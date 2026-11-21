# Roadmap

## Phase 1: router

- [x] Local HTTP server on `127.0.0.1`, random free port
- [x] Route by request `model`: `claude-*` to the Claude provider, `<provider>/<model>` to that Provider, others to the default Provider
- [x] Built-in Providers with the `anthropic` Dialect (see providers.md), plus user-defined ones in a config file
- [x] Requests without a JSON `model` go to the Claude provider
- [x] Streaming pass-through for server-sent events in both directions
- [x] Keys in the macOS keychain, written by `tsunagi login <provider>`; macOS only
- [x] No credential logging, verified by a test that greps logs for headers
- [x] Launcher that starts the router and runs Claude Code with the base URL scoped to that process
- [x] Verified with a real claude.ai login on 2026-11-21: `tsunagi claude -p` answered through the router and every request was logged as provider `claude`, status 200
- [ ] `/model` switch in a live session: Claude to `openrouter/inclusionai/ling-3.2-flash:free` was accepted and a request reached OpenRouter (200); switching back to Claude and a tool call on the non-Claude model were not checked live
- [x] Claude Code saves a `/model` choice as the default in `~/.claude/settings.json`, which Desktop and other sessions share. `internal/claudeguard` restores the saved value within ~100 ms when a non-Claude model is written there, edits only the `model` member in place, and repairs after a killed tsunagi. Verified with real Claude Code on 2026-11-21: the session kept the OpenRouter model, `settings.json` stayed `sonnet`, byte-identical after exit

## Phase 2a: catalogue and opencode import

- [x] models.dev catalogue (`tsunagi sync`), merged behind built-in and user providers
- [x] `tsunagi import opencode`: custom providers and API keys from opencode config and auth.json
- [x] `tsunagi models <provider>`
- [ ] Import from other tools (Codex `config.toml`, Crush)

## Phase 2: Sign-in and Responses Translator

- [x] Translator for `openai-responses`: request, streamed and plain reply, tool calls, usage, errors (reasoning is dropped on purpose)
- [ ] Verify the translators with a successful reply from a real Responses and a real chat provider (`ping`): Ollama Cloud and Google answered with provider errors only (no credits, key rejected), so the success path is still checked by fake upstreams only
- [x] Sign-in flow with loopback callback, ID token verification, rotating refresh token with a cross-process lock (refresh token in an owner-only file, as OpenAI's guide describes; the keychain cannot hold long values through `security -i`)
- [x] ChatGPT Sign-in via OpenAI's "Sign in with ChatGPT" (official, open-source/local eligible); tested against a fake issuer, the authorize endpoint accepts our request (302 to log-in), a full browser sign-in is not yet verified
- [ ] xAI Grok Sign-in (device code), opt-in flag
- [ ] Verify 403 handling for standard SuperGrok accounts
- [ ] Read GitHub Copilot and MiniMax third-party terms before adding their Sign-in

## Phase 2b: accounts and usage

- [x] Several accounts per provider (`--as label`), keys and ChatGPT sign-ins
- [x] Failover on rate limit, empty balance and rejected key; `sequential` and `round_robin`; `provider@label/model` pins
- [x] Token counts per account from the reply stream; `tsunagi usage`
- [x] Live balances: OpenRouter, DeepSeek, Moonshot (checked live against OpenRouter only)
- [ ] Balances for Z.AI, OpenCode Go and ChatGPT once their endpoints are documented
- [x] Usage-aware ranking (`strategy: usage`)

## Phase 2c: gateways

- [x] `tsunagi add` / `remove` for personal gateways, headers, auth header, strategy
- [x] Built-in local `ollama`, `lmstudio`, `vllm`; live model lists (`models`)
- [x] Checked with a real binary against a local fake OpenAI-compatible server (plain reply and tool call)

## Phase 2d: complete CLI

- [x] Slash commands through `--plugin-dir` (checked with real Claude Code)
- [x] `--json`, `--plain`, `watch --log|--json`, `statusline`, `doctor`, `help <command>`, shell completion
- [ ] Press the keys in a live interactive session: `/model` switch, `/tsunagi:usage`

## Phase 3: hand-off

- [x] Session registry, per-session done file, `tsunagi sessions`, `wait`, `done`, `handoff`
- [x] Stop hook writes the done file, so completion does not depend on the model
- [x] Verified with real Claude Code against a fake gateway (`make e2e`)
- [ ] Test very long and multi-line plans typed into the TUI
- [ ] Test non-Claude model compliance with a manual `tsunagi done`

## Phase 4: Chat Translator and polish

- [x] Translator for `openai-chat` (Groq, Together, Mistral, any OpenAI-compatible endpoint); built early because the models.dev catalogue is mostly this Dialect
- [x] Model aliases (`models` in config.json) rewrite any model name before routing, including Claude Code's background haiku requests
- [x] `tsunagi ping [--tool] <model>` sends one real request through the router; verified against OpenRouter (passthrough, tool call) with a real key
- [ ] Clear error when a Provider rejects a request
- [x] Packaging config: GoReleaser (`.goreleaser.yaml`, validated and snapshot-built locally), CI + tag release (`.github/workflows/ci.yml`), Homebrew cask, `install.sh`
- [x] First release v0.3.0 (2026-11-21): CI, GitHub release with arm64 and amd64 archives plus checksums, cask committed to `silent9669/homebrew-tap`. Checked: `install.sh` installs a working `tsunagi 0.3.0` into a temp dir with checksum verification; the cask sha256 equals the arm64 archive. Not run: `brew install` itself (Homebrew refuses an untrusted tap until `brew trust silent9669/tap`)

## Phase 5: smart routing

- [x] Combos: named `provider/model` target sets with `priority`, `fastest` (EWMA time-to-headers) and `round_robin` strategies
- [x] Capability filter from the catalogue (tool calling, context size); unknown data is permissive
- [x] Soft fall-through between targets on retryable failures; the last target delivers as-is
- [x] Per-target health: 3 consecutive failures open a circuit (30s, doubling to 10min, one half-open probe)
- [x] Tiers map `opus`/`sonnet`/`haiku` to a model or combo; aliases still win
- [x] `tsunagi route <model> [--tools] [--tokens N] [--json]` previews the plan with no network
- [x] Events carry `via`/`why`; `watch --log` shows them

## Phase 6: free models

- [x] Provider registry of verified free offers with data-policy labels, sources and check dates (`internal/free/registry.go`)
- [x] Public free-model discovery for OpenRouter and OpenCode Zen, cached 6h (`tsunagi free`, `free refresh`)
- [x] Tier-mode candidates from the live `/models` list intersected with catalogue capabilities; unmapped providers (e.g. google) contribute caps without becoming routable
- [x] Probe results cache (7-day freshness) and the `free/auto` builder: data-policy gate, probe gate, max 3 per provider
- [x] `tsunagi free probe|combo|allow|deny`, combo `"auto"` flag, `free.allow_trains` config
- [x] Live check, 2026-11-21: `ping --tool openrouter/inclusionai/ling-3.2-flash:free` returned a tool call with the stored OpenRouter key
- [ ] Live check of the other providers' free paths (only OpenRouter, Google and Ollama Cloud keys exist on the test machine)

## Test checklist

- Claude request through the router is byte-identical to a direct request.
- A non-Claude request carries that Provider's Key and never the Claude credential.
- Mid-session model switch does not break the conversation (thinking blocks, tool calls).
- Router refuses connections from anything but localhost.

## Phase 7: agent skills and uninstall

- [x] Four embedded agent skills (`internal/skills/content/*/SKILL.md`) covering hand-off, models, setup and free-model safety
- [x] `tsunagi skills` status/install/remove for the `claude` (`~/.claude/skills`) and `agents` (`~/.agents/skills`) targets, manifest-tracked at `~/.config/tsunagi/skills.json`
- [x] `tsunagi uninstall [--keys] [--config] [--yes]` removes skills, the tsunagi cache subdir, optionally keychain entries/ChatGPT sign-ins and the config dir; dry-run exits 2 on non-TTY
- [x] Interface remake: Charm v2 app (`internal/tui/app`), intro, main menu (Start Claude Code, Providers, Skills, Free models, Settings, Status), first-run wizard (`tsunagi setup` reopens it), Settings (tier routing incl. Haiku to free/auto, route preview, look/theme/animation applied live, reset, rerun setup); real tmux pass in `make e2e-ui`, captures in `docs/ui/captures`
- [x] Watch dashboard and launch card redrawn on the shared sea scene (`internal/tui/dash`); the old `internal/ui` look remains only for plain command output

## Phase 8: interface v2, routing and free survey (2026-11-21)

- [x] Layout engine: header (wordmark and animated sea with a swimming eel, a small sea, or one line) by terminal size, a body block, a key legend row; every screen is swept at 9 widths by 11 heights (20 to 140 columns, 5 to 50 rows) so nothing overflows
- [x] Providers: type-to-search over the whole catalogue, a stepper per provider (Account, Key, Verify, Done), mouse clicks and wheel, visible hotkey digits
- [x] Survey of other routers (OmniRoute, claude-code-router, LiteLLM) and free-API lists: registry corrected (Mistral free mode has no API; Cerebras, Hugging Face, Ollama Cloud and GitHub Models are not free) and extended (SambaNova, Cohere trial key)
- [x] Routing: `claude_fallback` (Claude first, then a combo on 429 or 529, with a Retry-After pause), strategies `cheapest`, `context` and `least_used`
- [x] Claude Code settings read-only (`internal/claudecfg`): conflicts reported by doctor, Status and the launch card; a conflicting base URL is overridden per session through inline `--settings`
- [x] Scenario routing from claude-code-router (thinking, long context, web search to a chosen model): `scenarios` + `scenario_tokens` in config, detection from the request body, precedence long context > web search > thinking, editable in Settings > Routing
- [x] Per-target weights and a quota-aware strategy from OmniRoute and LiteLLM: `weighted` strategy with per-target `weights` (Efraimidis-Spirakis draw; `tsunagi route` shows weight order) and `headroom` (best remaining account quota first, unknown last)
- [x] Zhipu GLM-Flash and ModelScope as free providers: Z.ai added (the international platform, GLM-4.7-Flash and GLM-4.5-Flash free per its pricing page, models read from the catalogue's zero-priced caps). ModelScope excluded: the free API needs an account bound to a real-name-verified Alibaba Cloud account and is evaluation-only. Zhipu's China platform was not added
- [ ] Live check of the Z.ai free models (no Z.ai key on the test machine; the builtin `zai` provider uses the Anthropic-dialect endpoint, and `free probe zai` decides whether the free models answer there)

## Phase 9: wider free catalogue and easier setup (v0.6.0)

- [x] Registry widened with Kilo Gateway, Reflection AI (beta) and Requesty (all `FromCatalog`), and exclusions documented for poolside, poe, zenmux, reka, iteracompute, qiniu-ai, orcarouter and llmgateway (`internal/free/registry.go`)
- [x] Keyless providers: registry `Access: "keyless"`, `OptionalKey` on `provider.Provider` stamped in `newRouter`; a request to a keyless provider goes out with no credential when none is stored and with the stored key when there is one
- [x] Key-prefix detector (`internal/keyprefix`): suggests the provider for a pasted key, refuses Claude keys with `ErrClaudeKey`
- [x] Environment key import: `Backend.EnvKeys`/`ImportEnvKey`/`DetectKey` — names only, the value is never returned or logged
- [x] Interface rework: one always-focused search box over actions and every provider (ordered substring and word-start matching), a smart next-step row, a providers browser with All/Free/Ready/Paid/Local chips, aligned columns, wrapped detail cards, a vertical setup rail, paste-a-key detection, environment import, an ANSI-Shadow wordmark with gradient, and `colour` as the default theme (`NO_COLOR`, `--plain` and an explicit `ui.theme` still win)
- [ ] Free providers read but not added: LLM7 (free token, 100k tokens/day, data policy unpublished), SEA-LION (10 requests/min trial key), Pollinations (anonymous, one text request per 45 s), OVHcloud (anonymous, 2 requests/min), Amazon Nova API (docs not readable here)
