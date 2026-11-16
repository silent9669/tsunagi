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
- [ ] Verify with a real claude.ai login that the OAuth `Authorization` header reaches Anthropic through the router
- [ ] `/model` switch in a live session (Claude to non-Claude and back)

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
- [ ] Packaging (Homebrew tap)

## Test checklist

- Claude request through the router is byte-identical to a direct request.
- A non-Claude request carries that Provider's Key and never the Claude credential.
- Mid-session model switch does not break the conversation (thinking blocks, tool calls).
- Router refuses connections from anything but localhost.
