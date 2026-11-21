# Subscription coding plans

Flat-fee plans that include API access for coding agents. tsunagi shows a
plan only when the provider's own docs document it. The machine-readable
copy lives in `internal/plans/registry.go`; every fact below was checked on
the date shown.

These plans are for coding tools, and several providers do not address
whether a local proxy may sit in front of the tool. That does not matter in
practice here: tsunagi only ever sends requests that come from the real,
unmodified Claude Code, with its own user agent and session id.

| Provider | Plan | Price | Quota | Models | Signup | Key page | Terms | Source | Checked |
|---|---|---|---|---|---|---|---|---|---|
| opencode-go | OpenCode Go | $10/mo Go, $40/mo Go Plus | included usage on open coding models, extra usage from credit | see the Go models page | https://opencode.ai/console/go | https://opencode.ai/console | Claude Code is validated; the client must send its own user agent and a session ID (Claude Code does). Free-tier models are not available outside OpenCode | https://opencode.ai/docs/console/harnesses/ | 2026-11-21 |
| zai | Z.ai GLM Coding Plan | from $18/mo (Lite, Pro, Max) | 5-hour and weekly credit limits | GLM-5.3, GLM-5.3-Flash only | https://z.ai/subscribe | https://z.ai/manage-apikey/apikey-list | limited to officially supported tools; Claude Code is listed. The docs do not say whether a local proxy in front of the tool is allowed | https://docs.z.ai/devpack/faq | 2026-11-21 |
| minimax | MiniMax M Plan (Token Plan) | subscription, see the plan page | shared monthly quota | MiniMax-M3.1-Flash and the M series | https://www.minimax.io/m-plan | https://platform.minimax.io/docs/m-plan/quickstart | the plan key is separate from a pay-as-you-go key; Claude Code is documented | https://platform.minimax.io/docs/m-plan/claude-code | 2026-11-21 |
| kimi-code-plan-global | Kimi Code | included in Kimi membership | by membership tier | k3.1-pro, k3, k3-256k, kimi-for-coding (by tier) | https://www.kimi.com/code | https://www.kimi.com/code/docs/en/third-party-tools/claude-code | Kimi documents Claude Code as a supported third-party tool | https://www.kimi.com/code/docs/en/third-party-tools/claude-code | 2026-11-21 |
| alibaba-coding-plan | Alibaba Cloud Coding Plan | $50/mo (Pro) | 6,000 requests per 5 hours, 45,000 per week, 90,000 per month; slots are limited | qwen3.7-plus, qwen3.6-plus, kimi-k2.5, glm-5, MiniMax-M2.5 | https://www.alibabacloud.com/help/en/model-studio/coding-plan | https://www.alibabacloud.com/help/en/model-studio/coding-plan | only for interactive coding tools such as Claude Code; direct API calls are rejected | https://www.alibabacloud.com/help/en/model-studio/coding-plan | 2026-11-21 |
| volcengine-coding-plan | Volcengine Ark Coding Plan | Lite and Pro tiers | per-period request quota | Doubao-Seed, GLM, MiniMax, Kimi and DeepSeek series | https://www.volcengine.com/activity/codingplan | https://docs.volcengine.com/docs/ark/coding-plan-personal-get-started | Claude Code is listed; requests must use the /api/coding endpoint to draw on the plan. Needs a Volcengine account (mainland-China console, Chinese docs) | https://docs.volcengine.com/docs/ark/coding-plan-personal-plan-overview | 2026-11-21 |
| ollama-cloud | Ollama Cloud | pay as you go; Pro $20/mo with $60 of credits, Max $100/mo | monthly credits on paid plans | cloud models such as glm-5.3-flash | https://ollama.com/pricing | https://ollama.com/settings/keys | Claude Code is documented; prompts are not stored or used for training | https://docs.ollama.com/integrations/claude-code | 2026-11-21 |

Catalogue providers whose name contains `coding-plan`, `token-plan` or
`code-plan` but which are not in this table are shown in Providers as
`plan · unverified` — a flat-fee plan by their own name, details not
checked.

See [free.md](free.md) for the free registry and [policy.md](policy.md)
for the compliance stance.
