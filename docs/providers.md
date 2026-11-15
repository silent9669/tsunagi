# Provider survey

Checked 2026-11-15 from vendor docs and third-party guides. Re-verify an endpoint before shipping it in the built-in list.

Claude Code only speaks the Anthropic Messages API. A Provider is cheap to support when it already serves that Dialect, and costly when it does not.

## Dialect `anthropic` (no Translator)

Set the base URL to the value below. Claude Code appends `/v1/messages`.

| Provider | Base URL | Notes |
| --- | --- | --- |
| Anthropic | `https://api.anthropic.com` | Claude provider. Forwarded untouched. |
| OpenCode Console | `https://opencode.ai/inference/anthropic` | Translates GPT, Gemini, Grok and others itself, so one Key reaches every Dialect. |
| OpenCode Go | `https://opencode.ai/inference/go/anthropic` | Subscription. Smaller model list. |
| OpenRouter | `https://openrouter.ai/api` | About 400 models, one Key. Middleman sees prompts. |
| Z.ai (GLM) | `https://api.z.ai/api/anthropic` | China-hosted. |
| DeepSeek | `https://api.deepseek.com/anthropic` | China-hosted. |
| Moonshot (Kimi) | `https://api.moonshot.ai/anthropic` | China-hosted. |
| MiniMax | `https://api.minimax.io/anthropic` | China endpoint: `api.minimaxi.com/anthropic`. |
| Alibaba (Qwen) | `https://dashscope-intl.aliyuncs.com/apps/anthropic` | China-hosted. |
| Fireworks | `https://api.fireworks.ai/inference` | `count_tokens` returns 404. |
| SiliconFlow | not verified | Reported as Anthropic-compatible. |
| llama.cpp `llama-server` | `http://127.0.0.1:<port>` | Local. Anthropic Messages support added in 2026. |
| Ollama, LM Studio | not verified | Check whether the Anthropic route is native in the installed version. |

Credential: send as `Authorization: Bearer <Key>` (what `ANTHROPIC_AUTH_TOKEN` produces). Fireworks documents this explicitly.

## Dialect `openai-chat` or `openai-responses` (Translator needed)

| Provider | Dialect | Anthropic route |
| --- | --- | --- |
| OpenAI | `openai-responses` | None. `api.openai.com/anthropic` returns 404. |
| Google Gemini | native `generateContent`, plus OpenAI-compat | None found. |
| xAI | `openai-responses` / `openai-chat` | None found. |
| Groq | `openai-chat` | None found. |
| Together | `openai-chat` | None found. |
| Mistral | `openai-chat` | None found. |

Until tsunagi ships its own Translator, reach these through OpenCode Console or OpenRouter, which translate server-side.

## Sign-in (OAuth) instead of a Key

Checked 2026-11-15. Sign-in uses the user's own subscription. tsunagi stores the tokens in the macOS keychain and refreshes them itself. Only listed for Providers whose own docs permit third-party apps.

| Provider | Status | Notes |
| --- | --- | --- |
| OpenAI (ChatGPT Plus, Pro, Go, Business) | Allowed | Official "Sign in with ChatGPT" program. At launch it covers open-source projects and personal local projects. OpenAI registers the client dynamically during sign-in (PKCE, loopback callback on `127.0.0.1`, scopes `offline_access` and `resource.invoke`). Dialect after sign-in: `openai-responses`. Paid or hosted apps must apply through a waitlist; tsunagi is neither. |
| xAI (SuperGrok, X Premium+) | Works, sanction unconfirmed | Device-code flow against `accounts.x.ai`, API at `https://api.x.ai/v1`, Dialect `openai-responses`. Some standard SuperGrok accounts get HTTP 403, so access may depend on tier. xAI published a "Connect Grok to Hermes Agent" post, which suggests tolerance, but I found no written third-party policy. |
| GitHub Copilot | Works, sanction unconfirmed | Device OAuth. OpenCode ships it. GitHub's third-party terms not read yet. |
| MiniMax | Works, sanction unconfirmed | Hermes Agent signs in with a MiniMax account and uses the `anthropic` Dialect. |
| Google Gemini, Antigravity | Prohibited | Google forbids third-party tools from using Gemini CLI or Antigravity OAuth, and has disabled accounts over it. Use an API key, OpenRouter or OpenCode instead. Never implemented. |
| Anthropic | Out of scope | tsunagi never handles a Claude login. |

Rule: a Provider gets sign-in only when its own docs permit it. "Sanction unconfirmed" Providers ship behind an opt-in flag with a warning, and are removed if the Provider objects.

Sources:

- https://developers.openai.com/cookbook/articles/sign-in-with-chatgpt
- https://help.openai.com/en/articles/20001542-using-your-chatgpt-plan-in-other-apps-and-sites
- https://hermes-agent.nousresearch.com/docs/guides/xai-grok-oauth
- https://opencode.ai/docs/providers/
- https://github.com/can1357/oh-my-pi/issues/12487

## Findings that shape the design

1. OpenCode Console already translates every text model to the Anthropic Dialect. One Key covers OpenAI, Gemini, Grok, Qwen, DeepSeek, MiniMax, GLM, Kimi and Mistral models.
2. Seven direct Providers speak the Anthropic Dialect natively. No Translator needed for them.
3. A Translator is needed to use OpenAI, Gemini, xAI, Groq, Together or Mistral directly, with a Key or with sign-in.
3a. Sign-in does not remove the Translator. ChatGPT and Grok sign-in both end in the `openai-responses` Dialect, so one Translator covers both.
4. `count_tokens` is not universal. The Router must pass the upstream 404 through, not invent a reply.
5. Most Providers want `ANTHROPIC_AUTH_TOKEN`-style Bearer auth, not `x-api-key`.

Sources:

- https://opencode.ai/docs/console/harnesses/claude-code/
- https://opencode.ai/docs/console/models/
- https://docs.fireworks.ai/tools-sdks/anthropic-compatibility
- https://app.stationx.net/articles/claude-code-other-models
- https://github.com/Alorse/cc-compatible-models
- https://code.claude.com/docs/en/llm-gateway
