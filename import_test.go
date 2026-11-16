package main

import (
	"testing"

	"github.com/silent9669/tsunagi/internal/opencode"
)

func TestPlanImport(t *testing.T) {
	known := map[string]bool{"opencode": true, "groq": true}
	ocs := []opencode.Provider{
		{ID: "acme", Package: "@ai-sdk/openai-compatible", BaseURL: "https://a.example/v1", Key: "acme-key-123456"},
		{ID: "opencode"},
		{ID: "nopkg", BaseURL: "https://x.example"},
		{ID: "nourl", Package: "@ai-sdk/openai-compatible"},
		{ID: "Bad_Name", Package: "@ai-sdk/openai-compatible", BaseURL: "https://x.example"},
	}
	auth := map[string]opencode.Auth{
		"groq":    {Type: "api", Key: "gsk_123456789"},
		"openai":  {Type: "oauth"},
		"mystery": {Type: "api", Key: "zzz-123456789"},
		"acme":    {Type: "api", Key: "other-key-123456"},
	}
	adds, keys, rows := planImport(known, ocs, auth)
	if len(adds) != 1 || adds[0].Name != "acme" || adds[0].Dialect != "openai-chat" {
		t.Fatalf("adds = %+v", adds)
	}
	if keys["groq"] != "gsk_123456789" || keys["acme"] != "acme-key-123456" || len(keys) != 2 {
		t.Fatalf("keys = %v (config literal must beat auth.json; unknown providers are dropped)", keys)
	}
	by := map[string]importRow{}
	for _, r := range rows {
		by[r.ID] = r
	}
	for id, want := range map[string]string{
		"acme": "added", "opencode": "known", "nopkg": "skip: no package", "nourl": "skip: no baseURL", "Bad_Name": "skip: bad name",
	} {
		if by[id].Config != want {
			t.Errorf("%s config = %q, want %q", id, by[id].Config, want)
		}
	}
	if by["openai"].Key != "skip: oauth login" || by["mystery"].Key != "skip: unknown provider" {
		t.Errorf("rows = %+v", by)
	}
}
