# Roadmap

## Phase 1: router

- [ ] Local HTTP server on `127.0.0.1`, random free port
- [ ] Route by request `model`: `claude-*` to Anthropic, others to OpenCode
- [ ] Streaming pass-through for server-sent events in both directions
- [ ] OpenCode key from local settings or macOS keychain
- [ ] No credential logging, verified by a test that greps logs for headers
- [ ] Launcher that starts the router and runs Claude Code with the base URL scoped to that process
- [ ] `/model` check: switch Claude to OpenCode and back in one session

## Phase 2: hand-off

- [ ] Send a plan from a Claude session into the tsunagi terminal tab
- [ ] Done-file marker plus background waiter
- [ ] Test long and multi-line plans against the TUI
- [ ] Test OpenCode model compliance with the done-file instruction

## Phase 3: polish

- [ ] Model mapping config for Opus, Sonnet and Haiku slots
- [ ] Clear error when OpenCode rejects a request
- [ ] Packaging (Homebrew tap)

## Test checklist

- Claude request through the router is byte-identical to a direct request.
- OpenCode request carries the OpenCode key and never the Claude credential.
- Mid-session model switch does not break the conversation (thinking blocks, tool calls).
- Router refuses connections from anything but localhost.
