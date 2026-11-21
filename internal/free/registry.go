// Package free knows which providers give free access to models, what the
// free access costs the user in privacy and limits, and how to find the free
// models today.
package free

// Mode says how free access works for a provider.
const (
	// ModelsMode: only some models are priced at zero; the list changes and is
	// read live.
	ModelsMode = "models"
	// TierMode: every model the provider serves is usable on a rate-limited
	// free plan.
	TierMode = "tier"
)

// Data is what the provider's own documents say it does with free-plan
// prompts and answers.
const (
	Private = "private" // documented as not used for training
	Trains  = "trains"  // may be used for training or product improvement
	Unknown = "unknown" // not checked
)

// Entry is one provider with a verified free offer. Everything here comes
// from the provider's own pages on the Checked date; Source is where.
type Entry struct {
	ID       string // provider name in tsunagi
	Name     string
	Mode     string
	Signup   string // where to make the account
	KeyURL   string // where to create the key
	Limits   string // short, as the provider states them
	Needs    string // what the sign-up asks for besides an email, "" for nothing
	Data     string // Private, Trains or Unknown
	DataNote string
	Source   string // page the Data label and limits come from
	Checked  string
	// Setup names an extra value the provider needs before it can be added.
	Setup string
}

// Registry is the list of providers tsunagi treats as having a free offer.
// A provider missing here is never put in a free combo.
func Registry() []Entry {
	return []Entry{
		{
			ID: "opencode", Name: "OpenCode Zen", Mode: ModelsMode,
			Signup: "https://opencode.ai/auth", KeyURL: "https://opencode.ai/auth",
			Limits: "free models rotate; each is free for a limited time",
			Needs:  "key may need billing details on file; free models cost nothing",
			Data:   Trains, DataNote: "free models: provider may retain and train on data",
			Source: "https://opencode.ai/docs/zen/", Checked: "2026-11-21",
		},
		{
			ID: "openrouter", Name: "OpenRouter", Mode: ModelsMode,
			Signup: "https://openrouter.ai/sign-up", KeyURL: "https://openrouter.ai/keys",
			Limits: "20 requests/min, 50/day; 1000/day after $10 of credits",
			Data:   Trains, DataNote: "free variants can be served by providers that retain and train, see each model page",
			Source: "https://openrouter.ai/docs/api_reference/limits", Checked: "2026-11-21",
		},
		{
			ID: "groq", Name: "Groq", Mode: TierMode,
			Signup: "https://console.groq.com/", KeyURL: "https://console.groq.com/keys",
			Limits: "per-model request and token caps, see the limits page",
			Data:   Private, DataNote: "inference requests are not retained or used for training by default",
			Source: "https://console.groq.com/docs/your-data", Checked: "2026-11-21",
		},
		{
			ID: "google", Name: "Google AI Studio", Mode: TierMode,
			Signup: "https://aistudio.google.com/", KeyURL: "https://aistudio.google.com/apikey",
			Limits: "free tier on selected models, flagship models excluded",
			Data:   Trains, DataNote: "free tier content is used to improve Google products",
			Source: "https://ai.google.dev/gemini-api/docs/pricing", Checked: "2026-11-21",
		},
		{
			ID: "mistral", Name: "Mistral Studio (Experiment)", Mode: TierMode,
			Signup: "https://console.mistral.ai/", KeyURL: "https://console.mistral.ai/api-keys",
			Limits: "about 1 request/second, restrictive token caps",
			Needs:  "verified phone number",
			Data:   Trains, DataNote: "free plan may train on data, opt-out in the Admin Console; Labs models always may",
			Source: "https://help.mistral.ai/en/articles/347617-do-you-use-my-user-data-to-train-your-artificial-intelligence-models", Checked: "2026-11-21",
		},
		{
			ID: "nvidia", Name: "NVIDIA NIM", Mode: TierMode,
			Signup: "https://build.nvidia.com/", KeyURL: "https://build.nvidia.com/settings/api-keys",
			Limits: "up to 40 requests/min, prototyping only",
			Needs:  "verified phone number",
			Data:   Trains, DataNote: "trial terms: prompts and outputs may be collected to improve NVIDIA products",
			Source: "https://build.nvidia.com/search", Checked: "2026-11-21",
		},
		{
			ID: "cloudflare", Name: "Cloudflare Workers AI", Mode: TierMode,
			Signup: "https://dash.cloudflare.com/sign-up", KeyURL: "https://dash.cloudflare.com/profile/api-tokens",
			Limits: "10,000 neurons/day; some models need a paid plan",
			Data:   Private, DataNote: "inputs and outputs are not retained or used for training",
			Source: "https://developers.cloudflare.com/workers-ai/platform/data-usage/", Checked: "2026-11-21",
			Setup: "account-id",
		},
	}
}

// Excluded lists offers that look free but are not, or that the provider
// forbids using this way. They are documented, never routed.
var Excluded = map[string]string{
	"github-models": "retired on 2026-07-30",
	"cerebras":      "the $5 trial credit needs a payment method and expires in 30 days",
	"gemini-cli":    "Google forbids third-party use of Gemini CLI and Antigravity sign-in",
}
