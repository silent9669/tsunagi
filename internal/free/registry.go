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

// Access is how the free offer is reached. "" means a key is needed;
// Keyless means the provider answers with no account and no key.
const Keyless = "keyless"

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
	// Access is "" when the offer needs a key, free.Keyless when it does not.
	Access string
	// Setup names an extra value the provider needs before it can be added.
	Setup string
	// FromCatalog reads the free-model list from the synced catalogue's
	// zero-priced caps instead of the discovery snapshot.
	FromCatalog bool
	// CatalogID is the models.dev catalogue entry the free models are read
	// from when it differs from the provider name ("" means use ID).
	CatalogID string
}

// CatID is the catalogue id behind the entry.
func (e Entry) CatID() string {
	if e.CatalogID != "" {
		return e.CatalogID
	}
	return e.ID
}

// Registry is the list of providers tsunagi treats as having a free offer.
// A provider missing here is never put in a free combo.
func Registry() []Entry {
	return []Entry{
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
			ID: "zai-api", Name: "Z.ai API (GLM Flash)", Mode: ModelsMode,
			Signup: "https://z.ai/model-api", KeyURL: "https://z.ai/manage-apikey/apikey-list",
			Limits:   "GLM-4.7-Flash and GLM-4.5-Flash only; other models are paid",
			Data:     Private,
			DataNote: "content is processed only to provide the API service and is not saved on Z.ai servers; processing is generally in Singapore",
			Source:   "https://docs.z.ai/guides/overview/pricing", Checked: "2026-11-21",
			FromCatalog: true, CatalogID: "zai",
		},
		{
			ID: "kilo", Name: "Kilo Gateway", Mode: ModelsMode,
			Signup: "https://app.kilo.ai", KeyURL: "https://app.kilo.ai",
			Limits:   "free models only, best effort: 200 requests/hour per IP, the list rotates and some models are rate limited upstream",
			Data:     Trains,
			DataNote: "free models may be routed to providers that log prompts and outputs to improve their services (NVIDIA's free endpoints are named)",
			Source:   "https://kilo.ai/docs/getting-started/using-kilo-for-free", Checked: "2026-11-21",
			FromCatalog: true, Access: Keyless,
		},
		{
			ID: "reflection", Name: "Reflection AI (beta)", Mode: ModelsMode,
			Signup: "https://platform.reflection.ai", KeyURL: "https://developers.reflection.ai/quickstart",
			Limits:   "public beta: free with daily token allowances that reset daily; Reflection may end the beta at any time",
			Data:     Unknown,
			DataNote: "content is kept up to 30 days for safety monitoring; whether it trains models depends on the account terms",
			Source:   "https://reflection.ai/legal/beta-pricing-information", Checked: "2026-11-21",
			FromCatalog: true,
		},
		{
			ID: "requesty", Name: "Requesty", Mode: ModelsMode,
			Signup: "https://www.requesty.ai/free-models", KeyURL: "https://www.requesty.ai/free-models",
			Limits:   "free models only; new organizations 200 requests/day and 20/min, paying organizations 1,000/day and 60/min",
			Data:     Unknown,
			DataNote: "prompts are not stored unless logging is enabled; each free model's upstream provider sets its own terms",
			Source:   "https://docs.requesty.ai/features/free-models", Checked: "2026-11-21",
			FromCatalog: true,
		},
	}
}

// Excluded lists offers that look free but are not, or that the provider
// forbids using this way. They are documented, never routed.
var Excluded = map[string]string{
	"opencode":      "free models are served only inside OpenCode: other tools get HTTP 403 or FreeUsageLimitError even with a key (OpenCode harness docs and a live check on 2026-11-21); its paid models and the Go plan work in any tool, see plans",
	"github-models": "retired on 2026-07-30",
	"cerebras":      "the $5 trial credit needs a payment method and expires in 30 days",
	"gemini-cli":    "Google forbids third-party use of Gemini CLI and Antigravity sign-in",
	"mistral":       "Free mode has no API access; API keys need billing activation (docs.mistral.ai, 2026-11)",
	"ollama-cloud":  "pay as you go, no recurring free credits",
	"huggingface":   "free accounts no longer get Inference Providers credits",
	"modelscope":    "needs a ModelScope account bound to a real-name-verified Alibaba Cloud account; evaluation use only (modelscope.cn API-Inference docs)",
	"poolside":      "hosted inference announced shut down on 2026-10-31 (Poolside's own sites)",
	"poe":           "free accounts cannot call paid models through the API (creator.poe.com)",
	"zenmux":        "the Free plan has no API access (zenmux.ai docs)",
	"reka":          "no current recurring free credit, credits are prepaid (docs.reka.ai/pricing)",
	"iteracompute":  "free promotion ends 2026-11-24",
	"qiniu-ai":      "free Conatus activity ends 2026-11-30 and needs mainland-China real-name verification",
	"orcarouter":    "free tier depends on lifetime spend and has an unpublished prompt-size cap",
	"llmgateway":    "free models are limited to 5 requests per 10 minutes without purchased credits (docs.llmgateway.io)",
}
