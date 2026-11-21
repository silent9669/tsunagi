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
	// FromCatalog reads the free-model list from the synced catalogue's
	// zero-priced caps instead of the discovery snapshot.
	FromCatalog bool
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
			ID: "sambanova", Name: "SambaNova Cloud", Mode: TierMode,
			Signup: "https://cloud.sambanova.ai/", KeyURL: "https://cloud.sambanova.ai/apis",
			Limits: "20 requests/min, 20/day and 200,000 tokens/day per model; preview models, evaluation only",
			Needs:  "no payment method linked (a linked card ends the free tier)",
			Data:   Private, DataNote: "the data processing addendum forbids training on customer data; prompts are not stored by default",
			Source: "https://docs.sambanova.ai/docs/en/models/rate-limits", Checked: "2026-11-21",
		},
		{
			ID: "cohere", Name: "Cohere (trial key)", Mode: TierMode,
			Signup: "https://dashboard.cohere.com/welcome/register", KeyURL: "https://dashboard.cohere.com/api-keys",
			Limits: "20 requests/min, 1,000 calls/month; trial keys are not for production",
			Data:   Trains, DataNote: "trial users' inputs and outputs may be used for research and to improve Cohere products",
			Source: "https://docs.cohere.com/docs/rate-limits", Checked: "2026-11-21",
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
		{
			ID: "zai", Name: "Z.ai", Mode: ModelsMode,
			Signup: "https://z.ai/model-api", KeyURL: "https://z.ai/manage-apikey/apikey-list",
			Limits:   "GLM-4.7-Flash and GLM-4.5-Flash only; other models are paid",
			Data:     Private,
			DataNote: "content is processed only to provide the API service and is not saved on Z.ai servers; processing is generally in Singapore",
			Source:   "https://docs.z.ai/guides/overview/pricing", Checked: "2026-11-21",
			FromCatalog: true,
		},
	}
}

// Excluded lists offers that look free but are not, or that the provider
// forbids using this way. They are documented, never routed.
var Excluded = map[string]string{
	"github-models": "retired on 2026-07-30",
	"cerebras":      "the $5 trial credit needs a payment method and expires in 30 days",
	"gemini-cli":    "Google forbids third-party use of Gemini CLI and Antigravity sign-in",
	"mistral":       "Free mode has no API access; API keys need billing activation (docs.mistral.ai, 2026-11)",
	"ollama-cloud":  "pay as you go, no recurring free credits",
	"huggingface":   "free accounts no longer get Inference Providers credits",
	"modelscope":    "needs a ModelScope account bound to a real-name-verified Alibaba Cloud account; evaluation use only (modelscope.cn API-Inference docs)",
}
