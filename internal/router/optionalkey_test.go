package router

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/silent9669/tsunagi/internal/provider"
)

// optionalEnv adds a keyless provider and a keyed provider to the test env.
// keys maps account name to key; anything missing is a key error.
func optionalEnv(t *testing.T, keys map[string]string) (*env, *stubUpstream) {
	t.Helper()
	e := setup(t)
	up, stub := newStub(t, 200, anthropicSSE)
	e.rt.Providers["kilo"] = provider.Provider{
		Name: "kilo", BaseURL: up.URL + "/anthropic", Dialect: provider.Anthropic, OptionalKey: true,
	}
	e.rt.Key = func(_ provider.Provider, acct string) (string, error) {
		if k, ok := keys[acct]; ok {
			return k, nil
		}
		return "", errors.New("none")
	}
	return e, stub
}

func TestKeylessSendsNoAuth(t *testing.T) {
	e, stub := optionalEnv(t, nil)
	resp := e.post(t, "/v1/messages", `{"model":"kilo/free-1"}`, map[string]string{
		"Authorization": "Bearer oauth-claude-login",
	})
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
	if stub.auth != "" || stub.xapikey != "" {
		t.Fatalf("keyless request carried auth=%q x-api-key=%q", stub.auth, stub.xapikey)
	}
}

func TestKeylessUsesStoredKey(t *testing.T) {
	e, stub := optionalEnv(t, map[string]string{"kilo": "kilo-key-1"})
	resp := e.post(t, "/v1/messages", `{"model":"kilo/free-1"}`, nil)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if stub.auth != "Bearer kilo-key-1" {
		t.Fatalf("auth = %q", stub.auth)
	}
}

func TestKeyedProviderWithoutKeyStillErrors(t *testing.T) {
	e, _ := optionalEnv(t, nil)
	e.rt.Providers["groq"] = provider.Provider{
		Name: "groq", BaseURL: "http://127.0.0.1:1", Dialect: provider.OpenAIChat,
	}
	resp := e.post(t, "/v1/messages", `{"model":"groq/m"}`, nil)
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 401 || !strings.Contains(string(b), "no key") {
		t.Fatalf("status %d body %s", resp.StatusCode, b)
	}
}

func TestPlanKeylessReadyKeyedSkipped(t *testing.T) {
	e, _ := optionalEnv(t, nil)
	e.rt.Combos = map[string]Combo{"free/auto": {Targets: []string{"kilo/free-1", "groq/m"}}}
	plan := e.rt.Plan("free/auto", Need{})
	got := map[string]Step{}
	for _, s := range plan.Steps {
		got[s.Target] = s
	}
	if s, ok := got["kilo/free-1"]; !ok || s.Status != "ready" {
		t.Fatalf("kilo step = %+v", s)
	}
	if s, ok := got["groq/m"]; !ok || s.Status != "skip" || s.Reason != "no key" {
		t.Fatalf("groq step = %+v", s)
	}
}
