package app

import (
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
)

// planSnap is a snapshot with a verified plan, an unverified catalogue plan
// and a keyless free provider.
func planSnap() Snapshot {
	s := snapBase()
	s.Providers = []ProviderRow{
		{Name: "groq", Group: "free", Tag: "private", FreeEntry: true},
		{Name: "opencode-go", Group: "plan", Tag: "plan", Plan: &PlanInfo{
			Price: "$10/mo Go, $40/mo Go Plus", Quota: "included usage on open coding models, extra usage from credit",
			Models: "see the Go models page", KeyHint: "oc_sk_",
			Terms:  "Claude Code is validated; the client must send its own user agent and a session ID (Claude Code does). Free-tier models are not available outside OpenCode",
			Source: "https://opencode.ai/docs/console/harnesses/", Checked: "2026-11-21",
			Signup: "https://opencode.ai/console/go", KeyURL: "https://opencode.ai/console", Verified: true,
		}},
		{Name: "alibaba-coding-plan", Group: "plan", Tag: "plan", Plan: &PlanInfo{
			Price: "$50/mo (Pro)", Quota: "6,000 requests per 5 hours, 45,000 per week, 90,000 per month; slots are limited",
			Models: "qwen3.7-plus, qwen3.6-plus, kimi-k2.5, glm-5, MiniMax-M2.5", KeyHint: "sk-sp-",
			Terms:  "only for interactive coding tools such as Claude Code; direct API calls are rejected",
			Source: "https://www.alibabacloud.com/help/en/model-studio/coding-plan", Checked: "2026-11-21",
			Signup:   "https://www.alibabacloud.com/help/en/model-studio/coding-plan",
			KeyURL:   "https://www.alibabacloud.com/help/en/model-studio/coding-plan",
			Verified: true,
		}},
		{Name: "foo-coding-plan", Group: "plan", Tag: "plan", Plan: &PlanInfo{
			Terms: "flat-fee plan, details not checked",
		}},
		{Name: "kilo", Group: "free", Tag: "trains", FreeEntry: true, Keyless: true},
		{Name: "anthropic", Group: "paid", Ready: true, Accounts: []string{"anthropic"}},
		{Name: "deepseek", Group: "paid"},
	}
	return s
}

func TestPlansChipAndSections(t *testing.T) {
	m := newTestApp(t, &fakeBackend{snap: planSnap()}, false)
	choose(m, "providers")
	v := viewText(m)
	// chip row shows Plans between Free and Ready
	if !strings.Contains(v, "Plans 3") {
		t.Fatalf("Plans chip missing:\n%s", v)
	}
	// All sections: the Plans section header comes before the Free one
	ip, ifree := -1, -1
	for i, l := range strings.Split(v, "\n") {
		switch strings.TrimSpace(l) {
		case "Plans":
			if ip < 0 {
				ip = i
			}
		case "Free":
			if ifree < 0 {
				ifree = i
			}
		}
	}
	if ip < 0 || ifree < 0 || ip > ifree {
		t.Fatalf("Plans section must come before Free:\n%s", v)
	}
	// a keyless provider is not counted Ready
	if strings.Contains(v, "Ready 2") {
		t.Fatalf("kilo must not count as ready:\n%s", v)
	}
	key(m, tea.KeyRight, "") // All -> Free
	key(m, tea.KeyRight, "") // Free -> Plans
	v = viewText(m)
	if !strings.Contains(v, "Alibaba Cloud Coding") || strings.Contains(v, "Groq") {
		t.Fatalf("Plans chip must show only plans:\n%s", v)
	}
	if !strings.Contains(v, "unverified") {
		t.Fatalf("unverified badge missing:\n%s", v)
	}
	// verified plan note is price · quota; unverified is the flat-fee line
	if !strings.Contains(v, "$50/mo (Pro)") || !strings.Contains(v, "flat-fee plan, details not checked") {
		t.Fatalf("plan notes wrong:\n%s", v)
	}
}

func TestPlanDetailCard(t *testing.T) {
	for _, size := range [][2]int{{110, 34}, {80, 24}} {
		m := newTestApp(t, &fakeBackend{snap: planSnap()}, false)
		send(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		choose(m, "providers")
		choose(m, "alibaba")
		v := viewText(m)
		for _, want := range []string{"price", "$50/mo (Pro)", "quota", "6,000 requests", "models", "terms",
			"calls are rejected", "key", "starts with sk-sp-", "source", "checked 2026-11-21",
			"Set up", "I have a key", "Open sign-up", "Open key page", "Back"} {
			if !strings.Contains(v, want) {
				t.Fatalf("%dx%d detail missing %q:\n%s", size[0], size[1], want, v)
			}
		}
		esc(m)
		// back to the list for the next size
	}
}

func TestUnverifiedPlanDetail(t *testing.T) {
	m := newTestApp(t, &fakeBackend{snap: planSnap()}, false)
	send(m, tea.WindowSizeMsg{Width: 110, Height: 34})
	choose(m, "providers")
	choose(m, "foo-coding")
	v := viewText(m)
	if !strings.Contains(v, "flat-fee plan, details not checked") {
		t.Fatalf("unverified terms missing:\n%s", v)
	}
	if strings.Contains(v, "Open sign-up") || strings.Contains(v, "Set up") {
		t.Fatalf("an unverified plan offers only a key:\n%s", v)
	}
	if !strings.Contains(v, "I have a key") {
		t.Fatalf("unverified menu wrong:\n%s", v)
	}
}

func TestPastePlanKeyCandidates(t *testing.T) {
	be := &fakeBackend{snap: planSnap()}
	be.detected = []string{"alibaba-coding-plan"}
	m := newTestApp(t, be, false)
	choose(m, "providers")
	paste(m, "sk-sp-"+strings.Repeat("x", 36))
	v := viewText(m)
	if !strings.Contains(v, "Looks like Alibaba") {
		t.Fatalf("alibaba candidate missing:\n%s", v)
	}
	if strings.Contains(v, strings.Repeat("x", 36)) {
		t.Fatalf("key rendered unmasked:\n%s", v)
	}
}

func TestPasteOcKeyTwoCandidates(t *testing.T) {
	be := &fakeBackend{snap: planSnap()}
	be.detected = []string{"opencode-go", "opencode"}
	m := newTestApp(t, be, false)
	choose(m, "providers")
	paste(m, "oc_sk_"+strings.Repeat("x", 36))
	v := viewText(m)
	if !strings.Contains(v, "Looks like OpenCode Go") || !strings.Contains(v, "Looks like opencode") {
		t.Fatalf("both oc_sk_ candidates should be offered:\n%s", v)
	}
}
