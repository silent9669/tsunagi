package free

import (
	"sort"
	"time"
)

// ModelCand is one candidate model for free/auto.
type ModelCand struct {
	ID        string // model id as the provider names it
	Context   int
	Tools     *bool // nil = unknown
	Reasoning bool
}

// AutoInput is everything BuildAuto needs; the caller gathers it so this
// package stays free of router and config imports.
type AutoInput struct {
	// Models maps a registry provider id to its candidate models.
	Models map[string][]ModelCand
	// Probes holds the latest probe result per "provider/model" target.
	Probes map[string]Result
	// AllowTrains is the user's opt-in list for Trains providers.
	AllowTrains map[string]bool
	// HasKey reports whether the provider has a usable key (or is Local).
	HasKey func(provider string) bool
	// ProbeOnly requires a fresh successful probe per model (the default).
	// When false, unprobed candidates with Tools != false are included.
	ProbeOnly bool
	// Now is the time probe freshness is measured against; zero = time.Now.
	Now time.Time
}

// ProviderCands is one registry provider's gated candidate models.
type ProviderCands struct {
	Entry  Entry
	Models []ModelCand
}

// ComboSpec is the router-agnostic combo the caller maps into config.Combo.
type ComboSpec struct {
	Targets  []string
	Strategy string
}

// dataAllowed applies the data-policy gate: Private always, Trains only when
// the user allowed the provider, Unknown never.
func dataAllowed(e Entry, allow map[string]bool) bool {
	switch e.Data {
	case Private:
		return true
	case Trains:
		return allow[e.ID]
	}
	return false
}

// Candidates applies the registry-order, data-policy and key gates and returns
// each passing provider's models sorted for combo order (reasoning, then
// context, then id). No probe gating and no per-provider cap: callers that
// probe use this to pick what to test, BuildAuto caps at 3.
func (in AutoInput) Candidates() []ProviderCands {
	if in.HasKey == nil {
		return nil
	}
	var out []ProviderCands
	for _, e := range Registry() {
		if !dataAllowed(e, in.AllowTrains) || !in.HasKey(e.ID) {
			continue
		}
		ms := append([]ModelCand(nil), in.Models[e.ID]...)
		sort.Slice(ms, func(i, j int) bool {
			if ms[i].Reasoning != ms[j].Reasoning {
				return ms[i].Reasoning
			}
			if ms[i].Context != ms[j].Context {
				return ms[i].Context > ms[j].Context
			}
			return ms[i].ID < ms[j].ID
		})
		out = append(out, ProviderCands{Entry: e, Models: ms})
	}
	return out
}

// probeOK applies the probe gate for one model. Under ProbeOnly a fresh
// successful probe that saw a tool call is required. Without it, models known
// to lack tools are out and a fresh failure excludes the model, but unprobed
// and previously-OK candidates stay in.
func (in AutoInput) probeOK(provider string, m ModelCand) bool {
	now := in.Now
	if now.IsZero() {
		now = time.Now()
	}
	r, ok := in.Probes[provider+"/"+m.ID]
	fresh := ok && now.Sub(r.At) < ProbeMaxAge
	if in.ProbeOnly {
		return ok && fresh && r.OK && r.Tools
	}
	if m.Tools != nil && !*m.Tools {
		return false
	}
	// a fresh probe that failed or saw no tool call is disqualifying; stale
	// results and no result at all leave the candidate in
	return !(ok && fresh && (!r.OK || !r.Tools))
}

// BuildAuto picks the free/auto targets: gated providers in registry order,
// at most 3 models each, strategy priority.
func BuildAuto(in AutoInput) ComboSpec {
	var targets []string
	for _, pc := range in.Candidates() {
		n := 0
		for _, m := range pc.Models {
			if !in.probeOK(pc.Entry.ID, m) {
				continue
			}
			targets = append(targets, pc.Entry.ID+"/"+m.ID)
			if n++; n == 3 {
				break
			}
		}
	}
	return ComboSpec{Targets: targets, Strategy: "priority"}
}
