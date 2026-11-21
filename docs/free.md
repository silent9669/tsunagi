# Free models

Some providers serve models at no cost. `tsunagi free` lists them, what the
free access costs in limits and in what happens to your prompts, and builds
the `free/auto` combo from the ones that pass every gate.

## The registry

Every fact below was checked on the date shown, from the provider's own
pages. The machine-readable copy lives in `internal/free/registry.go`.

| Provider | Mode | Limits | Data | Needs | Signup | Key | Source | Checked |
|---|---|---|---|---|---|---|---|---|
| OpenCode Zen | models | free models rotate; each is free for a limited time | trains — free models: provider may retain and train on data | key may need billing details on file; free models cost nothing | https://opencode.ai/auth | https://opencode.ai/auth | https://opencode.ai/docs/zen/ | 2026-11-21 |
| OpenRouter | models | 20 requests/min, 50/day; 1000/day after $10 of credits | trains — free variants can be served by providers that retain and train, see each model page | | https://openrouter.ai/sign-up | https://openrouter.ai/keys | https://openrouter.ai/docs/api_reference/limits | 2026-11-21 |
| Groq | tier | per-model request and token caps, see the limits page | private — inference requests are not retained or used for training by default | | https://console.groq.com/ | https://console.groq.com/keys | https://console.groq.com/docs/your-data | 2026-11-21 |
| Google AI Studio | tier | free tier on selected models, flagship models excluded | trains — free tier content is used to improve Google products | | https://aistudio.google.com/ | https://aistudio.google.com/apikey | https://ai.google.dev/gemini-api/docs/pricing | 2026-11-21 |
| SambaNova Cloud | tier | 20 requests/min, 20/day and 200,000 tokens/day per model; preview models, evaluation only | private — the data processing addendum forbids training on customer data; prompts are not stored by default | no payment method linked (a linked card ends the free tier) | https://cloud.sambanova.ai/ | https://cloud.sambanova.ai/apis | https://docs.sambanova.ai/docs/en/models/rate-limits | 2026-11-21 |
| Cohere (trial key) | tier | 20 requests/min, 1,000 calls/month; trial keys are not for production | trains — trial users' inputs and outputs may be used for research and to improve Cohere products | | https://dashboard.cohere.com/welcome/register | https://dashboard.cohere.com/api-keys | https://docs.cohere.com/docs/rate-limits | 2026-11-21 |
| NVIDIA NIM | tier | up to 40 requests/min, prototyping only | trains — trial terms: prompts and outputs may be collected to improve NVIDIA products | verified phone number | https://build.nvidia.com/ | https://build.nvidia.com/settings/api-keys | https://build.nvidia.com/search | 2026-11-21 |
| Cloudflare Workers AI | tier | 10,000 neurons/day; some models need a paid plan | private — inputs and outputs are not retained or used for training | account-id | https://dash.cloudflare.com/sign-up | https://dash.cloudflare.com/profile/api-tokens | https://developers.cloudflare.com/workers-ai/platform/data-usage/ | 2026-11-21 |
| Z.ai | models | GLM-4.7-Flash and GLM-4.5-Flash only; other models are paid | private — content is processed only to provide the API service and is not saved on Z.ai servers; processing is generally in Singapore | | https://z.ai/model-api | https://z.ai/manage-apikey/apikey-list | https://docs.z.ai/guides/overview/pricing | 2026-11-21 |
| Kilo Gateway | models | free models only; 200 requests/hour per IP, with or without an account | trains — free models may be routed to providers that log prompts and outputs to improve their services (NVIDIA's free endpoints are named) | | https://app.kilo.ai | keyless — https://app.kilo.ai optional | https://kilo.ai/docs/getting-started/using-kilo-for-free | 2026-11-21 |
| Reflection AI (beta) | models | public beta: free with daily token allowances that reset daily; Reflection may end the beta at any time | unknown — content is kept up to 30 days for safety monitoring; whether it trains models depends on the account terms | | https://platform.reflection.ai | https://developers.reflection.ai/quickstart | https://reflection.ai/legal/beta-pricing-information | 2026-11-21 |
| Requesty | models | free models only; new organizations 200 requests/day and 20/min, paying organizations 1,000/day and 60/min | unknown — prompts are not stored unless logging is enabled; each free model's upstream provider sets its own terms | | https://www.requesty.ai/free-models | https://www.requesty.ai/free-models | https://docs.requesty.ai/features/free-models | 2026-11-21 |

`models` mode means only some models are priced at zero and the list is read
live — from the provider's discovery endpoint, or from the synced catalogue's
zero-priced caps for `FromCatalog` entries (Z.ai, Kilo Gateway, Reflection
AI, Requesty). `tier` mode means every model the provider serves is usable on
a rate-limited free plan, so candidates come from the provider's live
`/models` list intersected with the catalogue's capability data.

Kilo Gateway is `keyless`: its free models answer with no account and no key
(the anonymous use is documented at
https://kilo.ai/docs/gateway/authentication). A stored key is still used when
one exists.

Offers that look free but are not are documented in `Excluded` and never
routed:

| Provider | Why excluded |
|---|---|
| github-models | retired on 2026-07-30 |
| cerebras | the $5 trial credit needs a payment method and expires in 30 days |
| gemini-cli | Google forbids third-party use of Gemini CLI and Antigravity sign-in |
| mistral | Free mode has no API access; API keys need billing activation (docs.mistral.ai, 2026-11) |
| ollama-cloud | pay as you go, no recurring free credits |
| huggingface | free accounts no longer get Inference Providers credits |
| modelscope | needs a ModelScope account bound to a real-name-verified Alibaba Cloud account; evaluation use only (modelscope.cn API-Inference docs) |
| poolside | hosted inference announced shut down on 2026-10-31 (Poolside's own sites) |
| poe | free accounts cannot call paid models through the API (creator.poe.com) |
| zenmux | the Free plan has no API access (zenmux.ai docs) |
| reka | no current recurring free credit, credits are prepaid (docs.reka.ai/pricing) |
| iteracompute | free promotion ends 2026-11-24 |
| qiniu-ai | free Conatus activity ends 2026-11-30 and needs mainland-China real-name verification |
| orcarouter | free tier depends on lifetime spend and has an unpublished prompt-size cap |
| llmgateway | free models are limited to 5 requests per 10 minutes without purchased credits (docs.llmgateway.io) |

## Data policy

The `data` column is what the provider's own documents say it does with
free-plan prompts:

- **private** — documented as not used for training. Always allowed in
  `free/auto`.
- **trains** — may be used for training or product improvement. Only enters
  `free/auto` after `tsunagi free allow <provider>`.
- **unknown** — not checked. Never enters `free/auto`.

`tsunagi free --all` also lists catalogue providers that are not in the
registry but show zero cost on models.dev, marked `unverified`. They are
informational only and never enter a combo.

## Commands

```
tsunagi free [--all] [--json]   # the registry table; --all adds unverified rows
tsunagi free <provider>          # its free models, then signup/key/needs/source facts
tsunagi free refresh            # force a re-read of the public lists
tsunagi free probe [provider]   # probe the top candidates with a real request
tsunagi free combo [--no-probe] [--dry-run]
tsunagi free allow <provider>   # opt a trains provider into free/auto
tsunagi free deny <provider>    # opt it back out
```

The public lists (OpenRouter `/api/v1/models`, OpenCode Zen `/zen/v1/models`)
are cached in `~/Library/Caches/tsunagi/free.json` for 6 hours; `free` shows
`offline, cached <age>` when it falls back to the cache. Probe results live
in `probe.json` next to it and count for 7 days.

## How free/auto is built

`tsunagi free combo` gathers candidates per provider — discovered free models
for `models` providers, live `/models` intersected with catalogue
capabilities for `tier` providers — then applies, in order:

1. **Key gate**: the provider must have a usable key — a `keyless` provider
   (Kilo Gateway) passes it with none (Cloudflare additionally
   needs an `account-id`, which is step 4's job; until a `cloudflare`
   provider exists in config it is never a candidate).
2. **Data gate**: private always, trains only when allowed, unknown never.
3. **Probe gate** (default): the model's latest probe must be under 7 days
   old, successful, and must have seen a tool call. `--no-probe` skips
   probing and includes unprobed candidates whose tools flag is not `no`,
   dropping only fresh probe failures.
4. **Tools gate for tier providers**: catalogue caps must say the model
   supports tools; models unknown to the catalogue are shown as `untested`
   in `tsunagi free <provider>` but never enter the combo.

At most 3 models per provider, ordered by reasoning support, then context
size, then id; providers keep registry order. The result is written as
`"combos": {"free/auto": {..., "auto": true}}` via `config.SetKey`, merging
into existing combos without touching user-written ones. A user-written
`free/auto` without `"auto": true` makes the command refuse rather than
overwrite it.

## Caveats

- **models.dev cost 0 is not proof of free.** NVIDIA lists all 52 models at
  price 0, which is the trial tier, not a permanent free plan — that is why
  the registry exists and unverified rows never enter a combo.
- **token-plan and coding-plan ids are subscriptions**, never free, and are
  filtered out of candidates.
- Free tiers change without notice; `tsunagi free refresh` re-reads the
  public lists and the registry `Checked` dates say when the facts were
  last verified.
