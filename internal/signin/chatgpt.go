// Package signin implements "Sign in with ChatGPT": OpenAI's program that lets
// open-source and local tools use a user's ChatGPT plan through the public
// Responses API. It follows OpenAI's published flow: dynamic client
// registration, PKCE, a loopback callback, a verified ID token, rotating
// refresh tokens.
package signin

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	DefaultIssuer   = "https://auth.openai.com"
	DefaultResource = "https://api.openai.com/v1"
	scopes          = "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"
	planScope       = "chatgpt.tokens.use.direct"
	dynamicClient   = "dynamic_agent_client"
	agentName       = "tsunagi"
)

var ErrNotSignedIn = errors.New("not signed in to ChatGPT. Run: tsunagi login chatgpt")

type ChatGPT struct {
	// Dir holds the credential files (owner-only).
	Dir string
	// Label names the account ("" for the default one). Each account has its
	// own registration, tokens and refresh lock.
	Label    string
	HTTP     *http.Client
	Issuer   string
	Resource string
	// Open shows the authorization URL in the user's browser.
	Open func(url string) error
	Now  func() time.Time
	// Listen starts the loopback callback listener.
	Listen func() (net.Listener, error)

	mu        sync.Mutex
	access    string
	accessExp time.Time
}

func New(dir string) *ChatGPT {
	return &ChatGPT{
		Dir: dir, HTTP: &http.Client{Timeout: 30 * time.Second},
		Issuer: DefaultIssuer, Resource: DefaultResource, Now: time.Now,
		Listen: func() (net.Listener, error) {
			// 1455 is the port OpenAI's examples use; any port is allowed.
			if ln, err := net.Listen("tcp", "127.0.0.1:1455"); err == nil {
				return ln, nil
			}
			return net.Listen("tcp", "127.0.0.1:0")
		},
	}
}

// record is what stays on disk: enough to refresh, nothing short-lived.
type record struct {
	ClientID     string `json:"client_id"`
	RefreshToken string `json:"refresh_token"`
	Email        string `json:"email"`
	Subject      string `json:"subject"`
}

type host struct {
	ID string `json:"ext_agent_host_id"`
}

func (c *ChatGPT) path(name string) string { return filepath.Join(c.Dir, name) }

// file names a per-account file: chatgpt-auth.json, or chatgpt.work-auth.json.
func (c *ChatGPT) file(kind string) string {
	if c.Label == "" {
		return "chatgpt-" + kind
	}
	return "chatgpt." + c.Label + "-" + kind
}

type clientFile struct {
	ClientID string `json:"client_id"`
}

func (c *ChatGPT) savedClient() string {
	var cf clientFile
	c.readJSON(c.file("client.json"), &cf)
	return cf.ClientID
}

func (c *ChatGPT) writeJSON(name string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return err
	}
	tmp := c.path(name + ".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path(name))
}

func (c *ChatGPT) readJSON(name string, v any) error {
	b, err := os.ReadFile(c.path(name))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (c *ChatGPT) loadHost() (host, error) {
	var h host
	if c.readJSON("chatgpt-host.json", &h) == nil && h.ID != "" {
		return h, nil
	}
	id, err := uuid()
	if err != nil {
		return h, err
	}
	h = host{ID: "urn:uuid:" + id}
	return h, c.writeJSON("chatgpt-host.json", h)
}

// Signed reports whether a sign-in is stored.
func (c *ChatGPT) Signed() bool {
	var r record
	return c.readJSON(c.file("auth.json"), &r) == nil && r.RefreshToken != ""
}

func (c *ChatGPT) Email() string {
	var r record
	c.readJSON(c.file("auth.json"), &r)
	return r.Email
}

// NewRegistration forgets the saved client, so the next sign-in registers
// again and can pick another workspace.
func (c *ChatGPT) NewRegistration() {
	os.Remove(c.path(c.file("client.json")))
}

func randB64(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func uuid() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

type callback struct {
	code, clientID string
	err            error
}

// SignIn runs the browser flow and stores the refresh token. announce receives
// the authorization URL so it can be shown when the browser does not open.
func (c *ChatGPT) SignIn(ctx context.Context, announce func(authURL string)) (string, error) {
	h, err := c.loadHost()
	if err != nil {
		return "", err
	}
	state, err1 := randB64(32)
	nonce, err2 := randB64(32)
	verifier, err3 := randB64(32)
	if err := errors.Join(err1, err2, err3); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	ln, err := c.Listen()
	if err != nil {
		return "", fmt.Errorf("cannot listen for the sign-in callback: %w", err)
	}
	redirect := "http://127.0.0.1:" + fmt.Sprint(ln.Addr().(*net.TCPAddr).Port) + "/auth/callback"

	result := make(chan callback, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var cb callback
		switch {
		case subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1:
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return // a stray request must not end the attempt
		case q.Get("error") != "":
			cb.err = fmt.Errorf("sign-in refused: %s", q.Get("error"))
		default:
			cb.code, cb.clientID = q.Get("code"), q.Get("client_id")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if cb.err != nil {
			fmt.Fprint(w, "<h3>Sign-in was not completed.</h3>You can close this tab.")
		} else {
			fmt.Fprint(w, "<h3>Signed in.</h3>You can close this tab and return to the terminal.")
		}
		select {
		case result <- cb:
		default:
		}
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	defer srv.Close()

	clientID := c.savedClient()
	q := url.Values{
		"response_type":         {"code"},
		"redirect_uri":          {redirect},
		"scope":                 {scopes},
		"resource":              {c.Resource},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"ext_agent_host_id":     {h.ID},
	}
	if clientID == "" {
		q.Set("client_id", dynamicClient)
		q.Set("agent_name_hint", agentName)
	} else {
		q.Set("client_id", clientID)
	}
	authURL := c.Issuer + "/api/accounts/authorize?" + q.Encode()
	if announce != nil {
		announce(authURL)
	}
	if c.Open != nil {
		c.Open(authURL) // a failure is fine: the URL was announced
	}

	var cb callback
	select {
	case cb = <-result:
	case <-ctx.Done():
		return "", errors.New("sign-in timed out")
	}
	if cb.err != nil {
		return "", cb.err
	}
	if cb.code == "" {
		return "", errors.New("sign-in callback carried no code")
	}
	switch {
	case clientID == "" && cb.clientID == "":
		return "", errors.New("registration incomplete: OpenAI returned no client id")
	case clientID == "":
		clientID = cb.clientID
		// Keep the issued id before exchanging the code, so a failed exchange
		// does not force a second registration.
		if err := c.writeJSON(c.file("client.json"), clientFile{clientID}); err != nil {
			return "", err
		}
	case cb.clientID != "" && cb.clientID != clientID:
		return "", errors.New("callback carried a different client id; refusing it")
	}

	tok, err := c.token(ctx, url.Values{
		"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {cb.code},
		"code_verifier": {verifier}, "redirect_uri": {redirect}, "resource": {c.Resource},
	})
	if err != nil {
		return "", err
	}
	if !hasScope(tok.Scope, planScope) {
		return "", errors.New("ChatGPT plan usage was not granted. Sign in again and allow it")
	}
	if tok.RefreshToken == "" {
		return "", errors.New("OpenAI returned no refresh token")
	}
	id, err := c.verifyIDToken(ctx, tok.IDToken, clientID, nonce)
	if err != nil {
		return "", err
	}
	if err := c.writeJSON(c.file("auth.json"), record{
		ClientID: clientID, RefreshToken: tok.RefreshToken, Email: id.Email, Subject: id.Subject,
	}); err != nil {
		return "", err
	}
	c.mu.Lock()
	c.access, c.accessExp = tok.AccessToken, c.Now().Add(time.Duration(tok.ExpiresIn)*time.Second)
	c.mu.Unlock()
	return id.Email, nil
}

func hasScope(list, want string) bool {
	for _, s := range strings.Fields(list) {
		if s == want {
			return true
		}
	}
	return false
}

type tokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

type oauthError struct{ code, msg string }

func (e *oauthError) Error() string { return e.code + ": " + e.msg }

func (c *ChatGPT) token(ctx context.Context, form url.Values) (tokenResp, error) {
	var t tokenResp
	req, err := http.NewRequestWithContext(ctx, "POST", c.Issuer+"/api/accounts/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return t, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return t, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	json.Unmarshal(b, &t)
	if resp.StatusCode != 200 || t.AccessToken == "" {
		code := t.Error
		if code == "" {
			code = fmt.Sprintf("http_%d", resp.StatusCode)
		}
		return t, &oauthError{code, t.Description}
	}
	return t, nil
}

// AccessToken returns a valid access token, refreshing it when it is about to
// expire. The refresh token rotates, so refreshes are serialized across
// processes with a file lock and always start from the stored token.
func (c *ChatGPT) AccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.access != "" && c.Now().Add(time.Minute).Before(c.accessExp) {
		return c.access, nil
	}
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return "", err
	}
	lock, err := os.OpenFile(c.path(c.file("lock")), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return "", err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	var r record
	if err := c.readJSON(c.file("auth.json"), &r); err != nil || r.RefreshToken == "" {
		return "", ErrNotSignedIn
	}
	tok, err := c.token(ctx, url.Values{
		"grant_type": {"refresh_token"}, "client_id": {r.ClientID},
		"refresh_token": {r.RefreshToken}, "resource": {c.Resource},
	})
	if err != nil {
		var oe *oauthError
		if errors.As(err, &oe) && oe.code == "invalid_grant" {
			return "", errors.New("ChatGPT sign-in expired. Run: tsunagi login chatgpt")
		}
		return "", err
	}
	if tok.RefreshToken != "" {
		r.RefreshToken = tok.RefreshToken
		if err := c.writeJSON(c.file("auth.json"), r); err != nil {
			return "", err
		}
	}
	c.access, c.accessExp = tok.AccessToken, c.Now().Add(time.Duration(tok.ExpiresIn)*time.Second)
	return c.access, nil
}

// SignOut revokes the refresh token and removes the local record. It reports
// whether OpenAI confirmed the revocation. The registered client id is kept
// for the next sign-in.
func (c *ChatGPT) SignOut(ctx context.Context) (bool, error) {
	var r record
	if err := c.readJSON(c.file("auth.json"), &r); err != nil {
		return false, ErrNotSignedIn
	}
	revoked := false
	if disc, err := c.discovery(ctx); err == nil && disc.Revocation != "" {
		form := url.Values{"token": {r.RefreshToken}, "token_type_hint": {"refresh_token"}, "client_id": {r.ClientID}}
		req, _ := http.NewRequestWithContext(ctx, "POST", disc.Revocation, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if resp, err := c.HTTP.Do(req); err == nil {
			resp.Body.Close()
			revoked = resp.StatusCode == 200
		}
	}
	c.mu.Lock()
	c.access = ""
	c.mu.Unlock()
	return revoked, os.Remove(c.path(c.file("auth.json")))
}

type Model struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Visibility  string `json:"visibility"`
}

// ListModels returns the models this account can use.
func (c *ChatGPT) ListModels(ctx context.Context) ([]Model, error) {
	tok, err := c.AccessToken(ctx)
	if err != nil {
		return nil, err
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", c.Resource+"/models", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("model list: HTTP %d", resp.StatusCode)
	}
	var out struct {
		Models []Model `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, err
	}
	var list []Model
	for _, m := range out.Models {
		if m.Visibility == "list" {
			list = append(list, m)
		}
	}
	return list, nil
}

type discoveryDoc struct {
	Issuer     string `json:"issuer"`
	JWKSURI    string `json:"jwks_uri"`
	Revocation string `json:"revocation_endpoint"`
}

func (c *ChatGPT) getJSON(ctx context.Context, u string, v any) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}

func (c *ChatGPT) discovery(ctx context.Context) (d discoveryDoc, err error) {
	err = c.getJSON(ctx, c.Issuer+"/.well-known/openid-configuration", &d)
	return
}

type identity struct{ Email, Subject string }

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// verifyIDToken checks signature (RS256 against the published keys), issuer,
// audience, expiry and nonce, as OpenAI's guide requires.
func (c *ChatGPT) verifyIDToken(ctx context.Context, token, clientID, nonce string) (identity, error) {
	bad := func(why string) (identity, error) {
		return identity{}, errors.New("ChatGPT did not return a verifiable identity: " + why)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return bad("no ID token")
	}
	var hdr struct{ Alg, Kid string }
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(hb, &hdr) != nil || hdr.Alg != "RS256" {
		return bad("unsupported signature algorithm")
	}
	disc, err := c.discovery(ctx)
	if err != nil {
		return bad(err.Error())
	}
	var keys struct {
		Keys []jwk `json:"keys"`
	}
	if err := c.getJSON(ctx, disc.JWKSURI, &keys); err != nil {
		return bad(err.Error())
	}
	var pub *rsa.PublicKey
	for _, k := range keys.Keys {
		if k.Kty == "RSA" && (k.Kid == hdr.Kid || hdr.Kid == "") {
			nb, e1 := base64.RawURLEncoding.DecodeString(k.N)
			eb, e2 := base64.RawURLEncoding.DecodeString(k.E)
			if e1 == nil && e2 == nil {
				pub = &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: int(new(big.Int).SetBytes(eb).Int64())}
				break
			}
		}
	}
	if pub == nil {
		return bad("signing key not found")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return bad("bad signature encoding")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig) != nil {
		return bad("signature check failed")
	}
	pb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return bad("bad claims encoding")
	}
	var cl struct {
		Iss   string          `json:"iss"`
		Aud   json.RawMessage `json:"aud"`
		Exp   float64         `json:"exp"`
		Nonce string          `json:"nonce"`
		Sub   string          `json:"sub"`
		Email string          `json:"email"`
	}
	if json.Unmarshal(pb, &cl) != nil {
		return bad("bad claims")
	}
	var aud []string
	var one string
	if json.Unmarshal(cl.Aud, &one) == nil {
		aud = []string{one}
	} else {
		json.Unmarshal(cl.Aud, &aud)
	}
	audOK := false
	for _, a := range aud {
		audOK = audOK || a == clientID
	}
	switch {
	case cl.Iss != disc.Issuer && cl.Iss != c.Issuer:
		return bad("wrong issuer")
	case !audOK:
		return bad("wrong audience")
	case float64(c.Now().Unix()) > cl.Exp:
		return bad("token expired")
	case subtle.ConstantTimeCompare([]byte(cl.Nonce), []byte(nonce)) != 1:
		return bad("nonce mismatch")
	case cl.Sub == "":
		return bad("no subject")
	}
	return identity{Email: cl.Email, Subject: cl.Sub}, nil
}
