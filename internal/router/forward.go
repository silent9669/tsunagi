package router

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/silent9669/tsunagi/internal/provider"
	"github.com/silent9669/tsunagi/internal/translate"
)

type decision struct {
	provider string // "" means Claude
	label    string // account label pinned by "provider@label/model", else ""
	model    string // model sent upstream
}

// route reads "provider/model" or "provider@label/model". Anything else that
// is not a Claude model goes to the default provider.
func (rt *Router) route(model string) decision {
	if i := strings.Index(model, "/"); i > 0 {
		prefix := model[:i]
		name, label, _ := strings.Cut(prefix, "@")
		if _, ok := rt.Providers[name]; ok {
			return decision{provider: name, label: label, model: model[i+1:]}
		}
	}
	if model == "" || strings.HasPrefix(model, "claude-") {
		return decision{model: model}
	}
	return decision{provider: rt.Default, model: model}
}

// Account is the key name for a provider and an optional label.
func Account(providerName, label string) string {
	if label == "" || label == "default" {
		return providerName
	}
	return providerName + "." + label
}

// pool remembers which accounts are temporarily out and spreads load.
type pool struct {
	mu      sync.Mutex
	blocked map[string]time.Time
	next    map[string]int
}

func (pl *pool) block(account string, d time.Duration) {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if pl.blocked == nil {
		pl.blocked = map[string]time.Time{}
	}
	pl.blocked[account] = time.Now().Add(d)
}

// order puts usable accounts first, in the given order (rotated for
// round_robin), then blocked ones by how soon they come back.
func (pl *pool) order(providerName string, accounts []string, roundRobin bool) []string {
	pl.mu.Lock()
	defer pl.mu.Unlock()
	if roundRobin && len(accounts) > 1 {
		if pl.next == nil {
			pl.next = map[string]int{}
		}
		k := pl.next[providerName] % len(accounts)
		pl.next[providerName]++
		accounts = append(append([]string{}, accounts[k:]...), accounts[:k]...)
	}
	now := time.Now()
	var ok, out []string
	for _, a := range accounts {
		if until, blocked := pl.blocked[a]; blocked && until.After(now) {
			out = append(out, a)
		} else {
			ok = append(ok, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return pl.blocked[out[i]].Before(pl.blocked[out[j]]) })
	return append(ok, out...)
}

func (rt *Router) candidates(p provider.Provider, label string) []string {
	if p.Local {
		return []string{p.Name}
	}
	if label != "" {
		return []string{Account(p.Name, label)}
	}
	all := []string{p.Name}
	if rt.Accounts != nil {
		if a := rt.Accounts(p.Name); len(a) > 0 {
			all = a
		}
	}
	if p.Strategy == "usage" && rt.Headroom != nil && len(all) > 1 {
		all = rankByHeadroom(all, func(a string) (float64, bool) { return rt.Headroom(p, a) })
	}
	return rt.pool.order(p.Name, all, p.Strategy == "round_robin")
}

// classify decides whether a failed reply means "this account is out, try the
// next one", and for how long to leave it alone. Rate limits, empty balances
// and rejected keys qualify; a bad request would fail the same way elsewhere.
func classify(status int, h http.Header) (bool, time.Duration) {
	switch status {
	case http.StatusTooManyRequests:
		if s := h.Get("Retry-After"); s != "" {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
				return true, min(time.Duration(n)*time.Second, time.Hour)
			}
		}
		return true, 5 * time.Minute
	case http.StatusPaymentRequired:
		return true, time.Hour
	case http.StatusUnauthorized, http.StatusForbidden:
		return true, 30 * time.Minute
	}
	return false, 0
}

var hopByHop = map[string]bool{
	"Connection": true, "Keep-Alive": true, "Proxy-Authenticate": true, "Proxy-Authorization": true,
	"Te": true, "Trailer": true, "Transfer-Encoding": true, "Upgrade": true, "Content-Length": true,
}

func setAuth(h http.Header, p provider.Provider, key string) {
	h.Del("Authorization")
	h.Del("X-Api-Key")
	for k, v := range p.Headers {
		h.Set(k, v)
	}
	if key == "" {
		return
	}
	if p.AuthHeader != "" {
		h.Set(p.AuthHeader, key)
	} else {
		h.Set("Authorization", "Bearer "+key)
	}
}

// withAccounts sends a request with each candidate account in turn until one
// is not rate limited, out of balance or rejected. Nothing reaches the client
// before that: a retry is invisible to Claude Code.
//
// In soft mode a retryable failure on the last account writes nothing and
// reports delivered=false, so a combo can try its next target. hk names the
// combo target for health tracking; "" disables it. The returned status is
// the upstream status of the final attempt, -1 for a transport error and -2
// when no key was usable.
func (rt *Router) withAccounts(rec *recorder, p provider.Provider, label string,
	send func(key string) (*http.Response, error), deliver func(*http.Response),
	soft bool, hk string) (bool, int) {

	cands := rt.candidates(p, label)
	for i, acct := range cands {
		last := i == len(cands)-1
		key := ""
		if !p.Local {
			k, err := rt.Key(p, acct)
			if err != nil || k == "" {
				if last && p.OptionalKey {
					// keyless provider: forward with no credential
				} else if last {
					rt.healthNote(hk, -2, 0)
					if soft {
						return false, -2
					}
					writeErr(rec, http.StatusUnauthorized, "authentication_error", keyMessage(acct, err))
					return true, http.StatusUnauthorized
				} else {
					rec.skipped++
					continue
				}
			} else {
				key = k
			}
		}
		t0 := rt.clock()
		resp, err := send(key)
		lat := rt.clock().Sub(t0).Milliseconds()
		if err != nil {
			if last {
				rt.healthNote(hk, -1, 0)
				if soft {
					return false, -1
				}
				writeErr(rec, http.StatusBadGateway, "api_error", "tsunagi: upstream "+p.Name+" unreachable: "+err.Error())
				return true, http.StatusBadGateway
			}
			rec.skipped++
			continue
		}
		retry, d := classify(resp.StatusCode, resp.Header)
		if retry && !last {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
			rt.pool.block(acct, d)
			rec.skipped++
			continue
		}
		if soft && last && retryable(resp.StatusCode, retry) {
			io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
			resp.Body.Close()
			rt.healthNote(hk, resp.StatusCode, float64(lat))
			return false, resp.StatusCode
		}
		rec.account = acct
		deliver(resp)
		resp.Body.Close()
		rt.healthNote(hk, resp.StatusCode, float64(lat))
		return true, resp.StatusCode
	}
	return true, 0 // cands is never empty
}

// retryable reports whether a failed reply means the target itself is at
// fault, so a combo may fall through to its next target: rate limits and
// rejected keys (classify), timeouts and server errors. Other 4xx are request
// faults and are delivered as they are.
func retryable(status int, classified bool) bool {
	return classified || status == http.StatusRequestTimeout || status >= 500
}

func (rt *Router) forwardProvider(rec *recorder, r *http.Request, body []byte, model string, d decision, soft bool, hk string) (bool, int) {
	p, ok := rt.Providers[d.provider]
	if !ok {
		writeErr(rec, http.StatusBadRequest, "invalid_request_error",
			fmt.Sprintf("tsunagi: unknown provider %q. Run: tsunagi providers", d.provider))
		return true, http.StatusBadRequest
	}
	if !p.Ready() {
		writeErr(rec, http.StatusBadRequest, "invalid_request_error",
			fmt.Sprintf("tsunagi: provider %s has unknown dialect %q (use anthropic, openai-chat or openai-responses)", p.Name, p.Dialect))
		return true, http.StatusBadRequest
	}
	if p.Dialect == provider.Anthropic {
		return rt.forwardAnthropic(rec, r, body, model, d, p, soft, hk)
	}
	return rt.forwardTranslated(rec, r, body, p, d, soft, hk)
}

// forwardAnthropic passes the request to a provider that speaks the Anthropic
// format. Only the model name and the credential change.
func (rt *Router) forwardAnthropic(rec *recorder, r *http.Request, body []byte, model string, d decision, p provider.Provider, soft bool, hk string) (bool, int) {
	target, err := url.Parse(p.BaseURL)
	if err != nil {
		writeErr(rec, http.StatusInternalServerError, "api_error", "tsunagi: bad base_url for "+p.Name)
		return true, http.StatusInternalServerError
	}
	if d.model != model {
		nb, err := setModel(body, d.model)
		if err != nil {
			writeErr(rec, http.StatusBadRequest, "invalid_request_error", "tsunagi: cannot rewrite model")
			return true, http.StatusBadRequest
		}
		body = nb
	}
	u := *target
	u.Path = strings.TrimRight(target.Path, "/") + r.URL.Path
	u.RawQuery = r.URL.RawQuery

	return rt.withAccounts(rec, p, d.label, func(key string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(r.Context(), r.Method, u.String(), bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		for k, v := range r.Header {
			if !hopByHop[http.CanonicalHeaderKey(k)] {
				req.Header[k] = v
			}
		}
		setAuth(req.Header, p, key)
		req.ContentLength = int64(len(body))
		return rt.transportFor(p).RoundTrip(req)
	}, func(resp *http.Response) { passThrough(rec, resp) }, soft, hk)
}

// passThrough copies a reply to the client, flushing each chunk as it arrives
// so server-sent events are not held back.
func passThrough(w http.ResponseWriter, resp *http.Response) {
	for k, v := range resp.Header {
		if !hopByHop[http.CanonicalHeaderKey(k)] {
			w.Header()[k] = v
		}
	}
	w.WriteHeader(resp.StatusCode)
	rc := http.NewResponseController(w)
	buf := make([]byte, 32<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			rc.Flush()
		}
		if err != nil {
			return
		}
	}
}

// forwardTranslated serves a request from a provider that does not speak the
// Anthropic format: translate the request, call the provider, translate the
// reply back.
func (rt *Router) forwardTranslated(rec *recorder, r *http.Request, body []byte, p provider.Provider, d decision, soft bool, hk string) (bool, int) {
	switch r.URL.Path {
	case "/v1/messages/count_tokens":
		// The translated APIs have no counting endpoint; a rough size is enough.
		fmt.Fprintf(rec, `{"input_tokens":%d}`, len(body)/4+1)
		return true, http.StatusOK
	case "/v1/messages":
	default:
		writeErr(rec, http.StatusNotFound, "not_found_error", "tsunagi: "+p.Name+" does not support "+r.URL.Path)
		return true, http.StatusNotFound
	}
	if r.Method != http.MethodPost {
		writeErr(rec, http.StatusMethodNotAllowed, "invalid_request_error", "tsunagi: POST only")
		return true, http.StatusMethodNotAllowed
	}
	endpoint, toReq, toStream, toReply := "/responses", translate.ToResponses, translate.StreamToAnthropic, translate.ReplyToAnthropic
	if p.Dialect == provider.OpenAIChat {
		opts := chatOptions(p, d.model)
		toReq = func(b []byte, m string) ([]byte, bool, error) { return translate.ToChatWith(b, m, opts) }
		endpoint, toStream, toReply = "/chat/completions", translate.ChatStreamToAnthropic, translate.ChatReplyToAnthropic
	}
	out, stream, err := toReq(body, d.model)
	if err != nil {
		writeErr(rec, http.StatusBadRequest, "invalid_request_error", "tsunagi: "+err.Error())
		return true, http.StatusBadRequest
	}
	// Sign-in (plan usage) accepts streaming requests only. A client that asked
	// for a plain reply gets one assembled from the stream.
	forced := p.Auth != "" && !stream
	if forced {
		if out, err = translate.ForceStream(out); err != nil {
			writeErr(rec, http.StatusInternalServerError, "api_error", "tsunagi: "+err.Error())
			return true, http.StatusInternalServerError
		}
	}
	target := strings.TrimRight(p.BaseURL, "/") + endpoint

	return rt.withAccounts(rec, p, d.label, func(key string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(out))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		setAuth(req.Header, p, key)
		if stream || forced {
			req.Header.Set("Accept", "text/event-stream")
		}
		return rt.transportFor(p).RoundTrip(req)
	}, func(resp *http.Response) {
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			rec.Header().Set("Content-Type", "application/json")
			rec.WriteHeader(resp.StatusCode)
			rec.Write(translate.ErrorToAnthropic(resp.StatusCode, b))
			return
		}
		if stream && strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
			rec.Header().Set("Content-Type", "text/event-stream")
			rec.Header().Set("Cache-Control", "no-cache")
			rec.WriteHeader(http.StatusOK)
			rc := http.NewResponseController(rec)
			toStream(resp.Body, rec, func() { rc.Flush() }, d.model)
			return
		}
		var b []byte
		var err error
		if forced {
			b, err = translate.CompletedResponse(resp.Body)
		} else {
			b, err = io.ReadAll(io.LimitReader(resp.Body, maxBody))
		}
		if err == nil {
			b, err = toReply(b, d.model)
		}
		if err != nil {
			writeErr(rec, http.StatusBadGateway, "api_error", "tsunagi: "+err.Error())
			return
		}
		rec.Header().Set("Content-Type", "application/json")
		rec.Write(b)
	}, soft, hk)
}

// GuardedTransport is the transport used for catalogue providers: it refuses
// loopback, private and link-local addresses. Other code that sends a key to
// such a provider (listing its models) must use it too.
func GuardedTransport() http.RoundTripper {
	return guardedTransport(http.DefaultTransport.(*http.Transport))
}

// rankByHeadroom puts the account with the most quota left first. An account
// whose quota is unknown counts as half full, so it neither jumps the queue
// nor sinks behind one that is nearly empty. Ties keep their given order.
func rankByHeadroom(accounts []string, headroom func(string) (float64, bool)) []string {
	score := map[string]float64{}
	for _, a := range accounts {
		if f, ok := headroom(a); ok {
			score[a] = f
		} else {
			score[a] = 0.5
		}
	}
	out := append([]string{}, accounts...)
	sort.SliceStable(out, func(i, j int) bool { return score[out[i]] > score[out[j]] })
	return out
}
