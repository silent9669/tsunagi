// Package plans lists the subscription coding plans tsunagi knows about:
// flat-fee provider plans meant for coding tools such as Claude Code.
// Every verified entry comes from the provider's own docs on the Checked
// date; Source says where.
package plans

import "strings"

// Entry is one coding plan. ID is the tsunagi provider name.
type Entry struct {
	ID, Name, Price, Quota, Models, Signup, KeyURL, KeyHint, Needs, Terms, Source, Checked string
}

// Registry is the list of plans tsunagi has checked against the provider's
// own documentation.
func Registry() []Entry {
	return []Entry{
		{
			ID: "opencode-go", Name: "OpenCode Go",
			Price:  "$10/mo Go, $40/mo Go Plus",
			Quota:  "included usage on open coding models, extra usage from credit",
			Models: "see the Go models page",
			Signup: "https://opencode.ai/console/go", KeyURL: "https://opencode.ai/console",
			KeyHint: "oc_sk_",
			Terms:   "Claude Code is validated; the client must send its own user agent and a session ID (Claude Code does). Free-tier models are not available outside OpenCode",
			Source:  "https://opencode.ai/docs/console/harnesses/", Checked: "2026-11-21",
		},
		{
			ID: "zai", Name: "Z.ai GLM Coding Plan",
			Price:  "from $18/mo (Lite, Pro, Max)",
			Quota:  "5-hour and weekly credit limits",
			Models: "GLM-5.3, GLM-5.3-Flash only",
			Signup: "https://z.ai/subscribe", KeyURL: "https://z.ai/manage-apikey/apikey-list",
			Terms:  "limited to officially supported tools; Claude Code is listed. The docs do not say whether a local proxy in front of the tool is allowed",
			Source: "https://docs.z.ai/devpack/faq", Checked: "2026-11-21",
		},
		{
			ID: "minimax", Name: "MiniMax M Plan (Token Plan)",
			Price:  "subscription, see the plan page",
			Quota:  "shared monthly quota",
			Models: "MiniMax-M3.1-Flash and the M series",
			Signup: "https://www.minimax.io/m-plan", KeyURL: "https://platform.minimax.io/docs/m-plan/quickstart",
			KeyHint: "sk-cp-",
			Terms:   "the plan key is separate from a pay-as-you-go key; Claude Code is documented",
			Source:  "https://platform.minimax.io/docs/m-plan/claude-code", Checked: "2026-11-21",
		},
		{
			ID: "kimi-code-plan-global", Name: "Kimi Code",
			Price:  "included in Kimi membership",
			Quota:  "by membership tier",
			Models: "k3.1-pro, k3, k3-256k, kimi-for-coding (by tier)",
			Signup: "https://www.kimi.com/code",
			KeyURL: "https://www.kimi.com/code/docs/en/third-party-tools/claude-code",
			Terms:  "Kimi documents Claude Code as a supported third-party tool",
			Source: "https://www.kimi.com/code/docs/en/third-party-tools/claude-code", Checked: "2026-11-21",
		},
		{
			ID: "alibaba-coding-plan", Name: "Alibaba Cloud Coding Plan",
			Price:   "$50/mo (Pro)",
			Quota:   "6,000 requests per 5 hours, 45,000 per week, 90,000 per month; slots are limited",
			Models:  "qwen3.7-plus, qwen3.6-plus, kimi-k2.5, glm-5, MiniMax-M2.5",
			Signup:  "https://www.alibabacloud.com/help/en/model-studio/coding-plan",
			KeyURL:  "https://www.alibabacloud.com/help/en/model-studio/coding-plan",
			KeyHint: "sk-sp-",
			Terms:   "only for interactive coding tools such as Claude Code; direct API calls are rejected",
			Source:  "https://www.alibabacloud.com/help/en/model-studio/coding-plan", Checked: "2026-11-21",
		},
		{
			ID: "volcengine-coding-plan", Name: "Volcengine Ark Coding Plan",
			Price:  "Lite and Pro tiers",
			Quota:  "per-period request quota",
			Models: "Doubao-Seed, GLM, MiniMax, Kimi and DeepSeek series",
			Signup: "https://www.volcengine.com/activity/codingplan",
			KeyURL: "https://docs.volcengine.com/docs/ark/coding-plan-personal-get-started",
			Needs:  "Volcengine account (mainland-China console, Chinese docs)",
			Terms:  "Claude Code is listed; requests must use the /api/coding endpoint to draw on the plan",
			Source: "https://docs.volcengine.com/docs/ark/coding-plan-personal-plan-overview", Checked: "2026-11-21",
		},
		{
			ID: "ollama-cloud", Name: "Ollama Cloud",
			Price:  "pay as you go; Pro $20/mo with $60 of credits, Max $100/mo",
			Quota:  "monthly credits on paid plans",
			Models: "cloud models such as glm-5.3-flash",
			Signup: "https://ollama.com/pricing", KeyURL: "https://ollama.com/settings/keys",
			Terms:  "Claude Code is documented; prompts are not stored or used for training",
			Source: "https://docs.ollama.com/integrations/claude-code", Checked: "2026-11-21",
		},
	}
}

// Find returns the registry entry for a provider name.
func Find(id string) (Entry, bool) {
	for _, e := range Registry() {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// LooksLikePlan reports whether a catalogue provider id names a flat-fee
// coding plan the registry has not checked.
func LooksLikePlan(id string) bool {
	return strings.Contains(id, "coding-plan") ||
		strings.Contains(id, "token-plan") ||
		strings.Contains(id, "code-plan")
}
