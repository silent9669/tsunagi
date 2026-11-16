package opencode

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadConfigV1V2AndKeys(t *testing.T) {
	t.Setenv("ACME_KEY", "from-env-123456")
	dir := t.TempDir()
	cfg := filepath.Join(dir, "opencode.jsonc")
	os.WriteFile(cfg, []byte(`{
	  // custom providers
	  "provider": {
	    "acme": {"npm": "@ai-sdk/openai-compatible", "options": {"baseURL": "https://a.example/v1", "apiKey": "{env:ACME_KEY}"}},
	    "openai": {},
	  },
	  "providers": {
	    "beta": {"package": "@opencode/ai/providers/anthropic-compatible", "settings": {"baseURL": "https://b.example", "apiKey": "literal-key-1234"}},
	  },
	}`), 0o600)
	ps, found, err := ReadConfig([]string{filepath.Join(dir, "missing.json"), cfg})
	if err != nil || !found || len(ps) != 3 {
		t.Fatalf("ps=%v found=%v err=%v", ps, found, err)
	}
	by := map[string]Provider{}
	for _, p := range ps {
		by[p.ID] = p
	}
	if a := by["acme"]; a.BaseURL != "https://a.example/v1" || a.Key != "from-env-123456" || a.Package != "@ai-sdk/openai-compatible" {
		t.Fatalf("acme = %+v", a)
	}
	if b := by["beta"]; b.BaseURL != "https://b.example" || b.Key != "literal-key-1234" {
		t.Fatalf("beta = %+v", b)
	}
	if _, found, _ := ReadConfig([]string{filepath.Join(dir, "nope")}); found {
		t.Fatal("found a config that does not exist")
	}
}

func TestReadAuth(t *testing.T) {
	p := filepath.Join(t.TempDir(), "auth.json")
	os.WriteFile(p, []byte(`{"groq":{"type":"api","key":"gsk_123456789"},"openai":{"type":"oauth","refresh":"r","access":"a"}}`), 0o600)
	m, err := ReadAuth(p)
	if err != nil || m["groq"].Key != "gsk_123456789" || m["openai"].Type != "oauth" || m["openai"].Key != "" {
		t.Fatalf("m=%v err=%v", m, err)
	}
}
