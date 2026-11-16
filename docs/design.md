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

- Listens on `127.0.0.1` only, on a random port. Never binds a public interface.
- A browser page can reach loopback ports, so every request must carry a per-session random token as the first path segment (`http://127.0.0.1:<port>/<token>`), have exactly the bound `Host`, and have no `Origin` header. Anything else gets 404 or 403 before any key is touched. Claude Code keeps path prefixes in its base URL (checked).
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
- Dialect `anthropic` is forwarded as is. Dialects `openai-chat` and `openai-responses` need the Translator. ChatGPT and Grok Sign-in both end in `openai-responses`, so that Dialect came first and is built (`internal/translate`). It never emits thinking blocks, because their signatures only mean something to Anthropic and a fake one would break the session when you switch back to Claude; thinking blocks in the history are dropped on the way out. `count_tokens` is answered locally with a rough size. Server-side tools such as web search are dropped. `openai-chat` (Chat Completions) has its own translator with the same rules, so any OpenAI-compatible endpoint works.
- Strip prior `thinking` and `redacted_thinking` blocks and Anthropic-only beta headers on this branch only, and only after a test shows the Provider rejects them.
- The Claude-originated background requests to a Haiku-class model stay on the Claude provider.

## Terminal ownership

The launcher draws the splash, then starts `claude` as a child with the terminal inherited. While the child runs the parent writes nothing to the terminal and forwards only termination signals, because Ctrl+C reaches the child through the terminal itself. Router events (time, provider, model, path, status, latency, never headers or bodies) go to `events-<pid>.jsonl` in the user cache directory, mode 0600, which `tsunagi watch` tails from another tab.

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

- Claude Code passes `provider/model` through unchanged (checked with Claude Code 2.1.308). It prints an unrecognized-model notice and assumes a 200k context window.
- Does the claude.ai OAuth `Authorization` header reach the router? Not checked: the OAuth session in the test environment had expired.
- Does typing multi-line text into the TUI submit early on the first newline? Long plans may need collapsing into one paragraph or a file reference.
- Do non-Claude models reliably follow the "write the done file" instruction?
- Do Claude "thinking" blocks cause errors after switching to a non-Claude model mid-conversation?
- Does each non-Claude Provider accept the beta headers Claude Code sends, or must they be stripped?
- Which OpenCode plan (Console or Go) supports which models? Check the current Go model table.
- Which of Ollama, LM Studio and SiliconFlow serve the Anthropic Dialect natively?
- Does the desktop app honor a project-level `.claude/settings.local.json` env block, as an alternative to a terminal launch?

## Catalogue and import

The router resolves a provider name in this order: user config, built-in list, models.dev catalogue. The catalogue is fetched on demand by `tsunagi sync` into the user cache directory, never at launch, and reduced to name, endpoint, Dialect and model names. Dialect comes from the npm package models.dev lists (`openai-compatible` and similar to `openai-chat`, `anthropic` to `anthropic`, `@ai-sdk/openai` to `openai-responses`). Providers whose package has no known mapping (Google, Azure, Bedrock) are left out.

`tsunagi import opencode` reads opencode's `provider`/`providers` blocks and `auth.json`. It adds providers tsunagi does not know, never overrides known ones, and copies only API keys, directly into the keychain and never to the screen. OAuth logins are reported and skipped: a refresh token is single-use, so sharing one would sign opencode out. A small `keys.json` index of provider names (no secrets) lets `tsunagi providers` show keyed catalogue entries without probing the keychain per entry.

The catalogue is downloaded data, so it is treated as untrusted: only `https` endpoints with a real host are accepted (no user info, loopback or private addresses), the cache file is `0600`, and a key saved for a catalogue provider is pinned to the endpoint host it was saved for. If a later sync moves that provider to another host, the router refuses to send the key and tells the user to log in again. Built-in and user-configured providers are not pinned, because the user chose their endpoints.

## Sign in with ChatGPT

`tsunagi login chatgpt` follows OpenAI's published flow for open-source and local apps: a loopback listener on `127.0.0.1`, PKCE, a first-time dynamic client registration (`client_id=dynamic_agent_client`, the issued id is kept and reused), and verification of the ID token (RS256 signature against the published keys, issuer, audience, expiry, nonce). Plan usage needs the `chatgpt.tokens.use.direct` scope, and sign-in is refused without it. Requests go to the public Responses API with `store:false` and `stream:true`; a client that asked for a plain reply gets one assembled from the stream.

Only the refresh token, client id, email and subject stay on disk, in `~/.config/tsunagi/chatgpt-auth.json` with mode `0600`; access tokens live in memory. The refresh token rotates, so refreshes take a file lock and always start from the stored token, which keeps several sessions from racing. Logout revokes the refresh token at OpenAI and keeps the registration. `tsunagi login chatgpt --new` registers again, to pick another workspace.

## Chat endpoint quirks

Learned from opencode's `compatibility` settings. The chat translator detects them from the provider and model name, and a provider's `compat` block overrides each one: output limit field (`max_completion_tokens` for api.openai.com, `max_tokens` elsewhere), an empty reasoning field on prior assistant messages (DeepSeek-style thinking models), `stream_options` on or off, an assistant turn between a tool result and a user message and 9-character tool ids (Mistral family, also when served through a gateway), and Anthropic-style `cache_control` markers for Claude behind an OpenAI-compatible proxy.

## Several accounts per provider

A provider can have many accounts: the first is named after the provider (`openai`), later ones carry a label (`openai.work`, made with `tsunagi login openai --as work`). Keys and sign-ins are separate per account; a ChatGPT account has its own registration and refresh token, while the device id is shared.

`provider/model` lets the pool choose. The default strategy is `sequential`: stay on the first account until it is rate limited, which uses each plan window in turn (the lesson from oh-my-pi's issues about balanced rotation wasting windowed quota). `"strategy": "round_robin"` spreads requests instead. `provider@label/model` pins one account and never falls back.

Failover happens before anything reaches Claude Code, so it is invisible: on 429, 402 or a rejected key (401, 403) the account is set aside (Retry-After, else 5 minutes for a rate limit, 30 minutes for a bad key, an hour for an empty balance) and the same request goes to the next one. Other errors, such as a bad request, are returned as they are, since another account would fail the same way. Blocks live only as long as the session, and when every account is out the last upstream error is shown.

`tsunagi usage` sums tokens per account from the event files of all sessions (kept a week; tokens are read from the replies as they stream past, with no change to the bytes) and asks providers with a published balance endpoint what is left: OpenRouter, DeepSeek and Moonshot so far. ChatGPT and most other plans have no documented usage API, so only local counts show for them.
