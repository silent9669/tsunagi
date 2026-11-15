# Design

## Problem

`ANTHROPIC_BASE_URL` is global for a Claude Code process. A single process therefore talks to one endpoint, and `/model` only changes the `model` field of the request. To reach Claude and OpenCode models from one session, the split has to happen between Claude Code and the network.

## Architecture

```text
Claude Code (terminal)
   │  ANTHROPIC_BASE_URL=http://127.0.0.1:<port>
   ▼
tsunagi router (localhost only)
   ├── model starts with "claude-"        ──►  Claude provider   (pass-through)
   ├── model "<provider>/<model>"         ──►  that provider     (Key swapped in)
   └── any other model                    ──►  default provider  (Key swapped in)
```

- Listens on `127.0.0.1` only. Never binds a public interface.
- Reads the JSON body once to get `model`, then streams the body onward unchanged.
- Responses are streamed back as they arrive (server-sent events must not be buffered).
- A request with no JSON `model` (model list, health, `count_tokens`) goes to the Claude provider.
- Upstream errors, including a 404 on `count_tokens`, are passed through as received.

### Claude provider

- Forward method, path, headers and body exactly as received.
- Do not read, store, log or alter `Authorization`, `x-api-key`, session or beta headers.
- Without a gateway credential set, Claude Code keeps using the saved claude.ai login, so the request carries the user's own credential straight to Anthropic.

### Other providers

- The Route is `claude-*` to the Claude provider, `<provider>/<model>` to the named Provider with the prefix removed, and any other name to the default Provider.
- Each Provider has a base URL, a Dialect and a Key. The built-in list is in [providers.md](providers.md); the user can add more in the config file.
- Replace the credential with the Provider's Key as `Authorization: Bearer`. The Claude credential is dropped on this branch and never forwarded.
- Credentials live in the macOS keychain, written by `tsunagi login <provider>`. That command stores a Key, or runs a Sign-in when the Provider supports one. Never in a shell profile, a plain file or chat.
- Sign-in is allowed only for Providers whose own docs permit third-party apps (see providers.md). The Router runs the OAuth flow, stores the tokens and refreshes them itself. Sign-in providers that are not explicitly sanctioned sit behind an opt-in flag.
- Gemini and Antigravity sign-in is never implemented: Google prohibits it.
- Dialect `anthropic` is forwarded as is. Dialects `openai-chat` and `openai-responses` need the Translator. ChatGPT and Grok Sign-in both end in `openai-responses`, so that Dialect comes first. Until then the user reaches those models through OpenCode Console or OpenRouter, which translate server-side.
- Strip prior `thinking` and `redacted_thinking` blocks and Anthropic-only beta headers on this branch only, and only after a test shows the Provider rejects them.
- The Claude-originated background requests to a Haiku-class model stay on the Claude provider.

## Implementation

Go, standard library only (`net/http`, `httputil.ReverseProxy`). One static binary. macOS only for now.

## Process scoping

The router and the base-URL override exist only inside the launched process:

```bash
ANTHROPIC_BASE_URL=http://127.0.0.1:<port> claude
```

Nothing is written to global `~/.claude/settings.json`. The desktop app and plain `claude` keep their default behavior.

Known side effects of a custom base URL, from Claude Code docs:

- Remote Control is disabled when `ANTHROPIC_BASE_URL` points to a host other than `api.anthropic.com`, localhost included.
- Tool search through a gateway may be affected.
- `ANTHROPIC_CUSTOM_MODEL_OPTION` adds one extra entry to the `/model` picker. Other models are typed by name.

## Plan hand-off loop

Goal: finish a plan in a normal Claude session, send it to a tsunagi session, get a message back when it is done.

1. The Claude session types the plan into the tsunagi terminal tab (terminal-panel tool of the desktop app).
2. The plan ends with: "when finished, write a short summary to `/tmp/tsunagi-done.txt`".
3. The Claude session starts a background waiter (`until [ -f /tmp/tsunagi-done.txt ]; do sleep 2; done`). Background shell commands re-invoke the session when they exit, so the result arrives without polling.
4. Fallback: read the terminal tab output and check whether the session is idle.

Plain text is enough. No Markdown plan file is required.

## Open questions

- Does Claude Code accept `/model provider/model` names with a slash? If not, pick another separator.
- Does typing multi-line text into the TUI submit early on the first newline? Long plans may need collapsing into one paragraph or a file reference.
- Do non-Claude models reliably follow the "write the done file" instruction?
- Do Claude "thinking" blocks cause errors after switching to a non-Claude model mid-conversation?
- Does each non-Claude Provider accept the beta headers Claude Code sends, or must they be stripped?
- Which OpenCode plan (Console or Go) supports which models? Check the current Go model table.
- Which of Ollama, LM Studio and SiliconFlow serve the Anthropic Dialect natively?
- Does the desktop app honor a project-level `.claude/settings.local.json` env block, as an alternative to a terminal launch?
