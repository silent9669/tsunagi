package router

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// errClaudeBusy is what the Claude transport returns, before anything has
// reached the client, when Anthropic answers with a limit or overload.
var errClaudeBusy = errors.New("claude is limited")

const (
	claudeBlockMin = 30 * time.Second
	claudeBlockMax = 5 * time.Minute
	claudeBlockDef = time.Minute
)

type busyState struct {
	status     int
	retryAfter time.Duration
}

// busyTransport passes Claude's requests through untouched and turns a 429 or
// 529 into errClaudeBusy so the router can fall back instead of showing it.
type busyTransport struct {
	next http.RoundTripper
	st   *busyState
}

func (b busyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := b.next.RoundTrip(req)
	if err != nil || (resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode != 529) {
		return resp, err
	}
	b.st.status = resp.StatusCode
	if n, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && n > 0 {
		b.st.retryAfter = time.Duration(n) * time.Second
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	resp.Body.Close()
	return nil, errClaudeBusy
}

// wantsFallback reports whether a Claude-bound request may fall back: only a
// chat request, and only when the user set ClaudeFallback.
func (rt *Router) wantsFallback(r *http.Request) bool {
	return rt.ClaudeFallback != "" && r.Method == http.MethodPost &&
		strings.HasSuffix(r.URL.Path, "/v1/messages")
}

func (rt *Router) claudeBlocked() bool {
	rt.fbMu.Lock()
	defer rt.fbMu.Unlock()
	return rt.clock().Before(rt.claudeUntil)
}

func (rt *Router) blockClaude(d time.Duration) {
	if d <= 0 {
		d = claudeBlockDef
	}
	d = max(claudeBlockMin, min(d, claudeBlockMax))
	rt.fbMu.Lock()
	rt.claudeUntil = rt.clock().Add(d)
	rt.fbMu.Unlock()
}

// forwardClaudeThenFallback sends the request to Anthropic untouched. If
// Anthropic answers 429 or 529 it sends the same conversation to the
// user's fallback combo or model instead, and leaves Claude alone for the
// time Anthropic asked for. It returns the provider that answered and the
// Why text.
func (rt *Router) forwardClaudeThenFallback(rec *recorder, r *http.Request, body []byte, model, why string) (name, via, evWhy string) {
	join := func(parts ...string) string {
		var out []string
		for _, p := range parts {
			if p != "" {
				out = append(out, p)
			}
		}
		return strings.Join(out, "; ")
	}
	if rt.claudeBlocked() {
		return rt.serveFallback(rec, r, body, model, join(why, "claude limited, using "+rt.ClaudeFallback))
	}
	st := &busyState{}
	px := rt.proxy(rt.Claude, busyTransport{next: rt.transport, st: st}, nil)
	plain := px.ErrorHandler
	px.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		if errors.Is(err, errClaudeBusy) {
			return
		}
		plain(w, req, err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	px.ServeHTTP(rec, r)
	if st.status == 0 || rec.wrote {
		return "claude", "", why
	}
	rt.blockClaude(st.retryAfter)
	return rt.serveFallback(rec, r, body, model, join(why, "claude "+strconv.Itoa(st.status)))
}

// serveFallback delivers the request to the configured combo, or to one
// provider/model when ClaudeFallback is not a combo name.
func (rt *Router) serveFallback(rec *recorder, r *http.Request, body []byte, model, why string) (name, via, evWhy string) {
	if combo, ok := rt.Combos[rt.ClaudeFallback]; ok {
		n, w := rt.serveCombo(rec, r, body, model, rt.ClaudeFallback, combo, why)
		return n, rt.ClaudeFallback, w
	}
	d := rt.route(rt.ClaudeFallback)
	if d.provider == "" {
		writeErr(rec, http.StatusBadGateway, "api_error", "tsunagi: claude_fallback "+rt.ClaudeFallback+" is not a combo or a provider/model")
		return "claude", "", why
	}
	rt.forwardProvider(rec, r, body, model, d, false, "")
	return d.provider, "", why
}
