package signin

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeIssuer struct {
	t          *testing.T
	srv        *httptest.Server
	key        *rsa.PrivateKey
	mu         sync.Mutex
	challenge  string
	nonce      string
	authQuery  url.Values
	scope      string // scope granted at code exchange
	idNonce    string // override the nonce put in the ID token ("" = use the real one)
	validRT    string
	refreshes  int
	revoked    string
	clientSeen []string
}

func newFake(t *testing.T) *fakeIssuer {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIssuer{t: t, key: key, scope: "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"issuer": f.srv.URL, "jwks_uri": f.srv.URL + "/jwks", "revocation_endpoint": f.srv.URL + "/revoke"})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "k1", "kty": "RSA",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/api/accounts/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.mu.Lock()
		defer f.mu.Unlock()
		f.clientSeen = append(f.clientSeen, r.Form.Get("client_id"))
		switch r.Form.Get("grant_type") {
		case "authorization_code":
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || r.Form.Get("code") != "code-1" || r.Form.Get("client_id") != "oaiapp_1" {
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}
			nonce := f.nonce
			if f.idNonce != "" {
				nonce = f.idNonce
			}
			f.validRT = "rt1"
			json.NewEncoder(w).Encode(map[string]any{
				"access_token": "at1", "refresh_token": "rt1", "expires_in": 3600, "scope": f.scope,
				"id_token": f.idToken(nonce),
			})
		case "refresh_token":
			if r.Form.Get("refresh_token") != f.validRT || r.Form.Get("client_id") != "oaiapp_1" {
				w.WriteHeader(400)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
				return
			}
			f.refreshes++
			f.validRT = "rt" + string(rune('1'+f.refreshes))
			json.NewEncoder(w).Encode(map[string]any{"access_token": "at-r" + string(rune('0'+f.refreshes)), "refresh_token": f.validRT, "expires_in": 3600})
		}
	})
	mux.HandleFunc("/revoke", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.revoked = r.Form.Get("token")
	})
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(401)
			return
		}
		w.Write([]byte(`{"models":[{"slug":"gpt-a","display_name":"A","visibility":"list"},{"slug":"hidden","visibility":"hide"}]}`))
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeIssuer) idToken(nonce string) string {
	b64 := func(v any) string { b, _ := json.Marshal(v); return base64.RawURLEncoding.EncodeToString(b) }
	head := b64(map[string]string{"alg": "RS256", "kid": "k1"})
	claims := b64(map[string]any{"iss": f.srv.URL, "aud": "oaiapp_1", "exp": time.Now().Add(time.Hour).Unix(), "nonce": nonce, "sub": "user-1", "email": "me@example.com"})
	d := sha256.Sum256([]byte(head + "." + claims))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, d[:])
	return head + "." + claims + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// browser plays the user: it reads the authorization URL and calls the callback.
func (f *fakeIssuer) browser() func(string) error {
	return func(authURL string) error {
		u, _ := url.Parse(authURL)
		q := u.Query()
		f.mu.Lock()
		f.authQuery, f.challenge, f.nonce = q, q.Get("code_challenge"), q.Get("nonce")
		f.mu.Unlock()
		cb := q.Get("redirect_uri") + "?code=code-1&state=" + url.QueryEscape(q.Get("state"))
		if q.Get("client_id") == dynamicClient {
			cb += "&client_id=oaiapp_1"
		}
		go http.Get(cb)
		return nil
	}
}

func (f *fakeIssuer) client(t *testing.T) *ChatGPT {
	c := New(t.TempDir())
	c.Issuer, c.Resource = f.srv.URL, f.srv.URL+"/v1"
	c.Listen = func() (net.Listener, error) { return net.Listen("tcp", "127.0.0.1:0") }
	c.Open = f.browser()
	return c
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestSignInRegistersThenReusesClientAndRefreshRotates(t *testing.T) {
	f := newFake(t)
	c := f.client(t)

	email, err := c.SignIn(ctx(t), nil)
	if err != nil || email != "me@example.com" {
		t.Fatalf("email=%q err=%v", email, err)
	}
	q := f.authQuery
	if q.Get("client_id") != dynamicClient || q.Get("agent_name_hint") != "tsunagi" || !strings.HasPrefix(q.Get("ext_agent_host_id"), "urn:uuid:") {
		t.Fatalf("first sign-in query = %v", q)
	}
	if q.Get("resource") != c.Resource || q.Get("code_challenge_method") != "S256" || !strings.Contains(q.Get("scope"), "chatgpt.tokens.use.direct") {
		t.Fatalf("query = %v", q)
	}
	if !strings.HasPrefix(q.Get("redirect_uri"), "http://127.0.0.1:") || !strings.HasSuffix(q.Get("redirect_uri"), "/auth/callback") {
		t.Fatalf("redirect_uri = %s", q.Get("redirect_uri"))
	}
	if !c.Signed() || c.Email() != "me@example.com" {
		t.Fatal("not recorded as signed in")
	}
	for _, name := range []string{"chatgpt-auth.json", "chatgpt-host.json", "chatgpt-client.json"} {
		if fi, err := os.Stat(filepath.Join(c.Dir, name)); err != nil || fi.Mode().Perm() != 0o600 {
			t.Fatalf("%s: %v %v", name, fi, err)
		}
	}

	// The access token from sign-in is reused, not refreshed.
	if tok, _ := c.AccessToken(ctx(t)); tok != "at1" || f.refreshes != 0 {
		t.Fatalf("token=%s refreshes=%d", tok, f.refreshes)
	}
	// After expiry it refreshes and stores the rotated refresh token.
	now := time.Now().Add(2 * time.Hour)
	c.Now = func() time.Time { return now }
	if tok, err := c.AccessToken(ctx(t)); err != nil || tok != "at-r1" {
		t.Fatalf("token=%s err=%v", tok, err)
	}
	now = now.Add(2 * time.Hour)
	if tok, err := c.AccessToken(ctx(t)); err != nil || tok != "at-r2" {
		t.Fatalf("second refresh: token=%s err=%v (the stored refresh token must be the rotated one)", tok, err)
	}

	c.Now = time.Now
	// A second sign-in reuses the issued client id and sends no agent name.
	if _, err := c.SignIn(ctx(t), nil); err != nil {
		t.Fatal(err)
	}
	if f.authQuery.Get("client_id") != "oaiapp_1" || f.authQuery.Get("agent_name_hint") != "" {
		t.Fatalf("returning query = %v", f.authQuery)
	}

	models, err := c.ListModels(ctx(t))
	if err != nil || len(models) != 1 || models[0].Slug != "gpt-a" {
		t.Fatalf("models=%v err=%v", models, err)
	}
}

func TestSignInRejectsBadIdentityAndMissingPlanScope(t *testing.T) {
	f := newFake(t)
	f.idNonce = "not-the-nonce"
	if _, err := f.client(t).SignIn(ctx(t), nil); err == nil || !strings.Contains(err.Error(), "nonce") {
		t.Fatalf("want nonce error, got %v", err)
	}
	f2 := newFake(t)
	f2.scope = "openid profile email offline_access resource.invoke"
	c := f2.client(t)
	if _, err := c.SignIn(ctx(t), nil); err == nil || !strings.Contains(err.Error(), "plan usage") {
		t.Fatalf("want plan-usage error, got %v", err)
	}
	if c.Signed() {
		t.Fatal("a refused sign-in must not be stored")
	}
}

func TestStrayCallbackWithWrongStateDoesNotEndSignIn(t *testing.T) {
	f := newFake(t)
	c := f.client(t)
	open := f.browser()
	c.Open = func(authURL string) error {
		u, _ := url.Parse(authURL)
		go http.Get(u.Query().Get("redirect_uri") + "?code=evil&state=wrong") // a local page poking the port
		time.Sleep(100 * time.Millisecond)
		return open(authURL)
	}
	if _, err := c.SignIn(ctx(t), nil); err != nil {
		t.Fatalf("sign-in failed after a stray request: %v", err)
	}
}

func TestSignOutRevokesAndKeepsRegistration(t *testing.T) {
	f := newFake(t)
	c := f.client(t)
	if _, err := c.SignIn(ctx(t), nil); err != nil {
		t.Fatal(err)
	}
	ok, err := c.SignOut(ctx(t))
	if err != nil || !ok || f.revoked != "rt1" {
		t.Fatalf("ok=%v err=%v revoked=%q", ok, err, f.revoked)
	}
	if c.Signed() {
		t.Fatal("still signed in")
	}
	if _, err := c.AccessToken(ctx(t)); err != ErrNotSignedIn {
		t.Fatalf("err = %v", err)
	}
	if c.savedClient() != "oaiapp_1" {
		t.Fatal("registration lost on sign-out")
	}
	c.NewRegistration()
	var h host
	c.readJSON("chatgpt-host.json", &h)
	if c.savedClient() != "" || h.ID == "" {
		t.Fatalf("NewRegistration should clear only the client id: %q %+v", c.savedClient(), h)
	}
}

func TestAccountsKeepSeparateFilesAndRegistrations(t *testing.T) {
	f := newFake(t)
	dir := t.TempDir()
	a, b := f.client(t), f.client(t)
	a.Dir, b.Dir = dir, dir
	b.Label = "work"
	if _, err := a.SignIn(ctx(t), nil); err != nil {
		t.Fatal(err)
	}
	if b.Signed() {
		t.Fatal("the work account must not see the default account's sign-in")
	}
	if _, err := b.SignIn(ctx(t), nil); err != nil {
		t.Fatal(err)
	}
	if f.authQuery.Get("client_id") != dynamicClient {
		t.Fatalf("each account registers on its own, got %v", f.authQuery.Get("client_id"))
	}
	for _, n := range []string{"chatgpt-auth.json", "chatgpt.work-auth.json", "chatgpt.work-client.json"} {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Fatalf("%s: %v", n, err)
		}
	}
	// the device identity is shared by both accounts
	var h1, h2 host
	a.readJSON("chatgpt-host.json", &h1)
	b.readJSON("chatgpt-host.json", &h2)
	if h1.ID == "" || h1 != h2 {
		t.Fatalf("host ids differ: %v %v", h1, h2)
	}
}
