package free

import (
	"testing"
	"time"
)

func boolp(b bool) *bool { return &b }

func autoIn(models map[string][]ModelCand, probes map[string]Result, allow []string, keyed map[string]bool, probeOnly bool) AutoInput {
	al := map[string]bool{}
	for _, n := range allow {
		al[n] = true
	}
	return AutoInput{
		Models:      models,
		Probes:      probes,
		AllowTrains: al,
		HasKey:      func(p string) bool { return keyed[p] },
		ProbeOnly:   probeOnly,
		Now:         time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC),
	}
}

func TestDataPolicyGates(t *testing.T) {
	// Private needs no allow-list entry, Trains does, Unknown never passes
	if !dataAllowed(Entry{Data: Private}, nil) {
		t.Fatal("private must always be allowed")
	}
	if dataAllowed(Entry{ID: "x", Data: Trains}, nil) {
		t.Fatal("trains without allow-list entry must be denied")
	}
	if !dataAllowed(Entry{ID: "x", Data: Trains}, map[string]bool{"x": true}) {
		t.Fatal("trains with allow-list entry must pass")
	}
	if dataAllowed(Entry{ID: "x", Data: Unknown}, map[string]bool{"x": true}) {
		t.Fatal("unknown must never pass, even when listed")
	}
	if dataAllowed(Entry{ID: "x", Data: "bogus"}, map[string]bool{"x": true}) {
		t.Fatal("unknown data label must never pass")
	}
}

func TestBuildAutoGatesAndOrder(t *testing.T) {
	models := map[string][]ModelCand{
		// opencode is Trains: gated out without allow, in with it
		"opencode": {
			{ID: "small", Context: 32_000},
			{ID: "big", Context: 200_000, Reasoning: true},
			{ID: "mid", Context: 128_000, Reasoning: true},
			{ID: "fourth", Context: 64_000, Reasoning: true},
		},
		// groq is Private: always allowed, registry order puts it after
		// opencode and openrouter but before google/mistral/nvidia/cloudflare
		"groq": {{ID: "llama", Context: 128_000}},
		// openrouter is Trains too
		"openrouter": {{ID: "qwen/qwen3-coder:free", Context: 262_144, Tools: boolp(true)}},
	}
	keyed := map[string]bool{"opencode": true, "groq": true, "openrouter": true, "google": true}

	// without allow_trains only the Private provider contributes
	spec := BuildAuto(autoIn(models, nil, nil, keyed, false))
	if len(spec.Targets) != 1 || spec.Targets[0] != "groq/llama" {
		t.Fatalf("targets = %v", spec.Targets)
	}
	if spec.Strategy != "priority" {
		t.Fatalf("strategy = %q", spec.Strategy)
	}

	// with allow_trains, registry order is opencode, openrouter, groq;
	// within opencode reasoning-first then context then id, capped at 3
	spec = BuildAuto(autoIn(models, nil, []string{"opencode", "openrouter"}, keyed, false))
	want := []string{
		"opencode/big", "opencode/mid", "opencode/fourth", // reasoning desc, context desc, capped at 3
		"openrouter/qwen/qwen3-coder:free",
		"groq/llama",
	}
	if len(spec.Targets) != len(want) {
		t.Fatalf("targets = %v", spec.Targets)
	}
	for i := range want {
		if spec.Targets[i] != want[i] {
			t.Fatalf("targets = %v, want %v", spec.Targets, want)
		}
	}
}

func TestBuildAutoNoKeyNoCandidates(t *testing.T) {
	models := map[string][]ModelCand{
		"groq":     {{ID: "llama"}},
		"opencode": {{ID: "big-pickle"}},
	}
	// nothing keyed: even Private providers contribute nothing
	spec := BuildAuto(autoIn(models, nil, nil, map[string]bool{}, false))
	if len(spec.Targets) != 0 {
		t.Fatalf("targets = %v", spec.Targets)
	}
	// a provider missing from Models still passes gates but adds nothing
	spec = BuildAuto(autoIn(map[string][]ModelCand{}, nil, nil, map[string]bool{"groq": true}, false))
	if len(spec.Targets) != 0 {
		t.Fatalf("targets = %v", spec.Targets)
	}
}

func TestBuildAutoProbeGating(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	models := map[string][]ModelCand{
		"groq": {
			{ID: "fresh-ok", Context: 100},
			{ID: "stale-ok", Context: 90},
			{ID: "no-tools", Context: 80},
			{ID: "failed", Context: 70},
			{ID: "unprobed", Context: 60},
		},
	}
	keyed := map[string]bool{"groq": true}
	probes := map[string]Result{
		"groq/fresh-ok": {Target: "groq/fresh-ok", OK: true, Tools: true, At: now.Add(-time.Hour)},
		"groq/stale-ok": {Target: "groq/stale-ok", OK: true, Tools: true, At: now.Add(-8 * 24 * time.Hour)},
		"groq/no-tools": {Target: "groq/no-tools", OK: true, Tools: false, At: now.Add(-time.Hour)},
		"groq/failed":   {Target: "groq/failed", OK: false, Err: "HTTP 500", At: now.Add(-time.Hour)},
	}

	// ProbeOnly: only a fresh OK probe that saw tools qualifies
	spec := BuildAuto(autoIn(models, probes, nil, keyed, true))
	if len(spec.Targets) != 1 || spec.Targets[0] != "groq/fresh-ok" {
		t.Fatalf("probe-only targets = %v", spec.Targets)
	}

	// no-probe: unprobed and OK models with tools stay; known no-tools and
	// fresh failures drop out; the stale OK counts as unprobed-ish info
	spec = BuildAuto(autoIn(models, probes, nil, keyed, false))
	want := map[string]bool{
		"groq/fresh-ok": true, "groq/stale-ok": true, "groq/unprobed": true,
	}
	if len(spec.Targets) != len(want) {
		t.Fatalf("no-probe targets = %v", spec.Targets)
	}
	for _, tg := range spec.Targets {
		if !want[tg] {
			t.Fatalf("unexpected target %s in %v", tg, spec.Targets)
		}
	}
}

func TestBuildAutoNoProbeDropsKnownNoTools(t *testing.T) {
	models := map[string][]ModelCand{
		"groq": {
			{ID: "notools", Tools: boolp(false)},
			{ID: "maybe", Tools: nil},
			{ID: "tools", Tools: boolp(true)},
		},
	}
	spec := BuildAuto(autoIn(models, nil, nil, map[string]bool{"groq": true}, false))
	want := []string{"groq/maybe", "groq/tools"}
	// ordering: all Context 0, not reasoning -> id order: maybe < notools < tools;
	// notools is dropped by the tools gate
	if len(spec.Targets) != 2 || spec.Targets[0] != want[0] || spec.Targets[1] != want[1] {
		t.Fatalf("targets = %v", spec.Targets)
	}
}

func TestCandidatesPreservesRegistryOrder(t *testing.T) {
	models := map[string][]ModelCand{
		"cloudflare": {{ID: "x"}},
		"opencode":   {{ID: "a"}},
		"groq":       {{ID: "b"}},
	}
	in := autoIn(models, nil, []string{"opencode"}, map[string]bool{"opencode": true, "groq": true, "cloudflare": true}, false)
	cands := in.Candidates()
	// registry order: opencode, openrouter, groq, google, mistral, nvidia, cloudflare
	var ids []string
	for _, pc := range cands {
		ids = append(ids, pc.Entry.ID)
	}
	want := []string{"opencode", "groq", "cloudflare"}
	if len(ids) != len(want) {
		t.Fatalf("providers = %v", ids)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("providers = %v, want %v", ids, want)
		}
	}
}
