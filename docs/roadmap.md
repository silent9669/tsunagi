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

## Phase 2: Sign-in and Responses Translator

- [x] Translator for `openai-responses`: request, streamed and plain reply, tool calls, usage, errors (reasoning is dropped on purpose)
- [ ] Verify the translator against the real OpenAI API with a Claude Code session (tool loop, `/model` back to Claude)
- [ ] Sign-in flow with loopback callback, tokens in the keychain, automatic refresh
- [ ] ChatGPT Sign-in via OpenAI's "Sign in with ChatGPT" (official, open-source/local eligible)
- [ ] xAI Grok Sign-in (device code), opt-in flag
- [ ] Verify 403 handling for standard SuperGrok accounts
- [ ] Read GitHub Copilot and MiniMax third-party terms before adding their Sign-in

## Phase 3: hand-off

- [ ] Send a plan from a Claude session into the tsunagi terminal tab
- [ ] Done-file marker at a per-session path exported as `TSUNAGI_DONE`, plus background waiter
- [ ] Test long and multi-line plans against the TUI
- [ ] Test non-Claude model compliance with the done-file instruction

## Phase 4: Chat Translator and polish

- [x] Translator for `openai-chat` (Groq, Together, Mistral, any OpenAI-compatible endpoint); built early because the models.dev catalogue is mostly this Dialect
- [ ] Model mapping config for Opus, Sonnet and Haiku slots, including background requests
- [ ] Clear error when a Provider rejects a request
- [ ] Packaging (Homebrew tap)

## Test checklist

- Claude request through the router is byte-identical to a direct request.
- A non-Claude request carries that Provider's Key and never the Claude credential.
- Mid-session model switch does not break the conversation (thinking blocks, tool calls).
- Router refuses connections from anything but localhost.
