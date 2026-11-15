# Design

## Problem

`ANTHROPIC_BASE_URL` is global for a Claude Code process. A single process therefore talks to one endpoint, and `/model` only changes the `model` field of the request. To reach Claude and OpenCode models from one session, the split has to happen between Claude Code and the network.

## Architecture

```text
Claude Code (terminal)
   │  ANTHROPIC_BASE_URL=http://127.0.0.1:<port>
   ▼
tsunagi router (localhost only)
   ├── model starts with "claude-"  ──►  https://api.anthropic.com        (pass-through)
   └── any other model              ──►  OpenCode endpoint                (key swapped in)
```

- Listens on `127.0.0.1` only. Never binds a public interface.
- Reads the JSON body once to get `model`, then streams the body onward unchanged.
- Responses are streamed back as they arrive (server-sent events must not be buffered).

### Claude branch

- Forward method, path, headers and body exactly as received.
- Do not read, store, log or alter `Authorization`, `x-api-key`, session or beta headers.
- Without a gateway credential set, Claude Code keeps using the saved claude.ai login, so the request carries the user's own credential straight to Anthropic.

### OpenCode branch

- Base URLs: `https://opencode.ai/inference/anthropic` (Console) or `https://opencode.ai/inference/go/anthropic` (Go).
- Replace the credential with the OpenCode key (`oc_sk_...`), read from the user's local settings or the macOS keychain. Never from a shell profile, never from chat.
- Optionally strip Anthropic-only beta headers on this branch only, if OpenCode rejects them.

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

- Does typing multi-line text into the TUI submit early on the first newline? Long plans may need collapsing into one paragraph or a file reference.
- Does the OpenCode model reliably follow the "write the done file" instruction?
- Do Claude "thinking" blocks cause errors after switching to an OpenCode model mid-conversation?
- Does OpenCode accept the beta headers Claude Code sends, or must they be stripped?
- Which OpenCode plan (Console or Go) supports which models over the Anthropic-compatible endpoint? An older post said Go covers only Anthropic-compatible MiniMax and Qwen models. Check the current model table.
- Does the desktop app honor a project-level `.claude/settings.local.json` env block, as an alternative to a terminal launch?
