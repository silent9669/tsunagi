package free

import (
	"os"
	"path/filepath"
	"testing"
)

// TestUnverified needs a catalogue cache: providers priced at zero on
// models.dev that are not registry providers show up as unverified, while
// registry providers, plan subscriptions and unroutable entries do not.
func TestUnverified(t *testing.T) {
	cacheHome(t)
	dir := filepath.Join(os.Getenv("HOME"), "Library", "Caches", "tsunagi")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cache := `{"fetched":"2026-11-21T00:00:00Z","providers":[
		{"id":"cheapco","name":"CheapCo","api":"https://cheap.example/v1","dialect":"openai-chat",
		 "models":["free-1","free-2","paid"],
		 "caps":{"free-1":{"tools":true,"in":0,"out":0},"free-2":{"in":0,"out":0},"paid":{"in":1,"out":1}}},
		{"id":"xiaomi-token-plan-ams","name":"Xiaomi Plan","api":"https://x.example/v1","dialect":"openai-chat",
		 "models":["m"],"caps":{"m":{"in":0,"out":0}}},
		{"id":"openrouter","name":"OpenRouter","api":"https://or.example/v1","dialect":"openai-chat",
		 "models":["m"],"caps":{"m":{"in":0,"out":0}}},
		{"id":"unroutable","name":"NoDialect","api":"","dialect":"",
		 "models":["m"],"caps":{"m":{"in":0,"out":0}}},
		{"id":"halfprice","name":"Half","api":"https://h.example/v1","dialect":"openai-chat",
		 "models":["m"],"caps":{"m":{"in":0}}}
	]}`
	if err := os.WriteFile(filepath.Join(dir, "models.json"), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	got := Unverified()
	if len(got) != 1 || got[0].Provider != "cheapco" || got[0].Models != 2 {
		t.Fatalf("unverified = %+v", got)
	}
}
