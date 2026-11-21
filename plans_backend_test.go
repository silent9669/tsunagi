package main

import (
	"testing"

	"github.com/silent9669/tsunagi/internal/config"
	"github.com/silent9669/tsunagi/internal/tui/app"
)

// plansCatalog has kilo (keyless free), a catalogue twin of the new alibaba
// builtin and a catalogue-only coding plan.
const plansCatalog = `{"fetched":"2026-11-21T00:00:00Z","providers":[
	{"id":"kilo","name":"Kilo","api":"https://api.kilo.ai/api/gateway","dialect":"openai-chat","models":["m1"]},
	{"id":"alibaba-coding-plan","name":"Alibaba Plan","api":"https://catalog.example/v1","dialect":"openai-chat","models":["m1"]},
	{"id":"foo-coding-plan","name":"Foo Plan","api":"https://foo.example/v1","dialect":"openai-chat","models":["m1"]}
]}`

func snapRow(snap app.Snapshot, name string) (app.ProviderRow, bool) {
	for _, p := range snap.Providers {
		if p.Name == name {
			return p, true
		}
	}
	return app.ProviderRow{}, false
}

func TestSnapshotPlanAndKeylessRows(t *testing.T) {
	envKeyHome(t, `{"ui":{"onboarded":true}}`, plansCatalog)
	snap := (backend{}).Snapshot()

	kilo, ok := snapRow(snap, "kilo")
	if !ok || !kilo.Keyless || kilo.Ready || kilo.Group != "free" {
		t.Fatalf("keyless provider must be usable but not ready: %+v", kilo)
	}
	for _, id := range []string{"zai", "minimax", "opencode-go", "alibaba-coding-plan",
		"kimi-code-plan-global", "volcengine-coding-plan", "ollama-cloud"} {
		p, ok := snapRow(snap, id)
		if !ok || p.Group != "plan" || p.Plan == nil || !p.Plan.Verified {
			t.Fatalf("%s must be a verified plan: %+v", id, p)
		}
	}
	ali, _ := snapRow(snap, "alibaba-coding-plan")
	if ali.Plan.KeyHint != "sk-sp-" || ali.Plan.Price == "" {
		t.Fatalf("alibaba plan info = %+v", ali.Plan)
	}
	foo, ok := snapRow(snap, "foo-coding-plan")
	if !ok || foo.Group != "plan" || foo.Plan == nil || foo.Plan.Verified ||
		foo.Plan.Terms != "flat-fee plan, details not checked" {
		t.Fatalf("unverified catalogue plan = %+v", foo)
	}
	// zai is a plan now; the free registry entry is zai-api
	zai, _ := snapRow(snap, "zai")
	if zai.Group != "plan" || zai.Plan == nil || zai.Plan.Price == "" {
		t.Fatalf("zai = %+v", zai)
	}
	za, ok := snapRow(snap, "zai-api")
	if !ok || za.Group != "free" || !za.FreeEntry {
		t.Fatalf("zai-api = %+v", za)
	}
	chat, _ := snapRow(snap, "chatgpt")
	if chat.Group != "plan" || chat.Plan != nil {
		t.Fatalf("chatgpt keeps the sign-in plan row: %+v", chat)
	}
}

// The four new builtins share names with models.dev catalogue ids; the
// builtin must win so the Providers list shows one row each.
func TestBuiltinBeatsCatalogueTwin(t *testing.T) {
	envKeyHome(t, `{"ui":{"onboarded":true}}`, plansCatalog)
	snap := (backend{}).Snapshot()
	n := 0
	for _, p := range snap.Providers {
		if p.Name == "alibaba-coding-plan" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("alibaba-coding-plan rows = %d", n)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range cfg.Catalog {
		if p.Name == "alibaba-coding-plan" {
			t.Fatal("catalogue twin survived the dedupe")
		}
	}
}
