// Package router forwards Claude Code requests to the provider named in the
// request's model field.
//
//	claude-* or no model   -> Anthropic, forwarded untouched
//	<provider>/<model>     -> that provider, prefix removed, key swapped in
//	anything else          -> the default provider
package router

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"

	"github.com/silent9669/tsunagi/internal/events"
	"github.com/silent9669/tsunagi/internal/provider"
)

const maxBody = 64 << 20

// KeyError carries a message about a key that must reach the user as is.
type KeyError struct{ Msg string }

func (e KeyError) Error() string { return e.Msg }

// keyMessage explains why no key could be used.
func keyMessage(name string, err error) string {
	var ke KeyError
	if errors.As(err, &ke) {
		return ke.Msg
	}
	return fmt.Sprintf("tsunagi: no key for %s. Run: tsunagi login %s", name, name)
}

type Router struct {
	// Host is the exact Host header the router accepts, e.g. "127.0.0.1:5000".
	Host string
	// Token is a per-session secret that must prefix every request path.
	Token     string
	Providers map[string]provider.Provider
	Default   string
	// Accounts lists the accounts (keys or sign-ins) of a provider, the one to
	// prefer first. Nil means one account named after the provider.
	Accounts func(provider string) []string
	// Headroom reports how much of an account's quota is left (0..1), when
	// known. It is used by the "usage" strategy and must not block.
	Headroom func(p provider.Provider, account string) (float64, bool)
	// Aliases rewrite a requested model name before routing.
	Aliases map[string]string
	// Combos maps a model name to an ordered set of targets tried in turn.
	Combos map[string]Combo
	// Tiers maps a Claude tier (opus, sonnet, haiku) to a model or combo.
	Tiers map[string]string
	// Caps reports what the catalogue knows about a provider's model.
	Caps    func(provider, model string) (Caps, bool)
	Key     func(p provider.Provider, account string) (string, error)
	Claude  *url.URL
	OnEvent func(events.Event)

	// now is the clock; nil means time.Now. Tests may replace it.
	now func() time.Time

	transport http.RoundTripper
	guarded   http.RoundTripper // for catalogue providers

	pool   pool
	cstate comboState
}

// transportFor picks the transport for a provider.
func (rt *Router) transportFor(p provider.Provider) http.RoundTripper {
	if p.Untrusted {
		return rt.guarded
	}
	return rt.transport
}

func New(host, token string, providers []provider.Provider, def string, key func(provider.Provider, string) (string, error)) *Router {
	rt := &Router{
		Host:      host,
		Token:     token,
		Providers: map[string]provider.Provider{},
		Default:   def,
		Key:       key,
		Claude:    &url.URL{Scheme: "https", Host: "api.anthropic.com"},
	}
	for _, p := range providers {
		rt.Providers[p.Name] = p
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DisableCompression = true // never alter Accept-Encoding or the body
	rt.transport = t
	rt.guarded = guardedTransport(t)
	return rt
}

func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// A browser page can reach loopback ports, so every request must prove it
	// came from the launched Claude Code: exact Host, no Origin, session token.
	if r.Host != rt.Host || r.Header.Get("Origin") != "" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	path, ok := rt.stripToken(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	r.URL.Path, r.URL.RawPath = path, ""

	var body []byte
	if r.Body != nil && r.Body != http.NoBody {
		b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil {
			writeErr(w, http.StatusRequestEntityTooLarge, "invalid_request_error", "tsunagi: request body unreadable or too large")
			return
		}
		body = b
	}
	model := peekModel(body)
	resolved, why := rt.resolve(model)

	rec := &recorder{ResponseWriter: w, status: http.StatusOK}
	name, via, evWhy := "", "", why
	if combo, ok := rt.Combos[resolved]; ok {
		via = resolved
		name, evWhy = rt.serveCombo(rec, r, body, model, resolved, combo, why)
	} else {
		d := rt.route(resolved)
		name = d.provider
		switch {
		case name != "":
			rt.forwardProvider(rec, r, body, model, d, false, "")
		default:
			name = "claude"
			if resolved != model {
				// An alias or tier pointed a custom name at a Claude model.
				nb, err := setModel(body, resolved)
				if err != nil {
					writeErr(rec, http.StatusBadRequest, "invalid_request_error", "tsunagi: cannot rewrite model")
					break
				}
				body = nb
				r.ContentLength = int64(len(nb))
				r.Header.Set("Content-Length", strconv.Itoa(len(nb)))
			}
			if rec.status == http.StatusOK && !rec.wrote {
				rt.forwardClaude(rec, r, body)
			}
		}
	}
	rec.tap.Done()
	if rt.OnEvent != nil {
		rt.OnEvent(events.Event{
			Time: start, Provider: name, Model: model, Path: r.URL.Path,
			Status: rec.status, Millis: time.Since(start).Milliseconds(),
			Account: rec.account, In: rec.tap.In, Out: rec.tap.Out, Skipped: rec.skipped,
			Via: via, Why: evWhy,
		})
	}
}

func (rt *Router) stripToken(p string) (string, bool) {
	prefix := "/" + rt.Token
	if len(p) < len(prefix) || subtle.ConstantTimeCompare([]byte(p[:len(prefix)]), []byte(prefix)) != 1 {
		return "", false
	}
	rest := p[len(prefix):]
	if rest == "" {
		return "/", true
	}
	if rest[0] != '/' {
		return "", false
	}
	return rest, true
}

func (rt *Router) forwardClaude(w http.ResponseWriter, r *http.Request, body []byte) {
	r.Body = io.NopCloser(bytes.NewReader(body))
	rt.proxy(rt.Claude, rt.transport, nil).ServeHTTP(w, r)
}

func (rt *Router) proxy(target *url.URL, tr http.RoundTripper, auth func(http.Header)) *httputil.ReverseProxy {
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			if auth != nil {
				auth(pr.Out.Header)
			}
		},
		Transport:     tr,
		FlushInterval: -1, // stream server-sent events as they arrive
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeErr(w, http.StatusBadGateway, "api_error", "tsunagi: upstream "+target.Host+" unreachable: "+err.Error())
		},
	}
}

func peekModel(body []byte) string {
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' {
		return ""
	}
	var m struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &m) != nil {
		return ""
	}
	return m.Model
}

// setModel changes only the model field; other fields keep their raw bytes.
func setModel(body []byte, model string) ([]byte, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	b, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	m["model"] = b
	return json.Marshal(m)
}

func writeErr(w http.ResponseWriter, status int, typ, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"type":  "error",
		"error": map[string]string{"type": typ, "message": msg},
	})
}

// recorder remembers the status code and keeps streaming working.
type recorder struct {
	http.ResponseWriter
	status  int
	wrote   bool
	tap     tap
	account string
	skipped int
}

func (r *recorder) WriteHeader(code int) {
	if !r.wrote {
		r.status, r.wrote = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.wrote = true
	r.tap.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *recorder) Flush()                      { http.NewResponseController(r.ResponseWriter).Flush() }
func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }
