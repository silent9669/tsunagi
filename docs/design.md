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

- Each launch registers itself (`tsunagi sessions`) and gets `TSUNAGI_SESSION` and `TSUNAGI_DONE` in its environment.
- The launcher adds a Stop hook through `--settings` (skipped when you pass your own `--settings` or set `TSUNAGI_NO_HOOK`). When the session ends a turn the hook runs `tsunagi done --hook`, which writes the last assistant message to the done file. The hook can fire before that message reaches the transcript, so it waits up to three seconds for it. Completion therefore does not depend on a non-Claude model obeying a closing instruction; a model can still call `tsunagi done "<summary>"` itself.
- The sending Claude session types the plan into the tsunagi terminal tab (the desktop app's terminal panel tool), then runs `tsunagi wait --session <id>` in the background. The command exits, and wakes the session, when the target finishes its next turn; the summary is its output. `tsunagi handoff` prints these steps with the right ids.
- The done file outlives its session so a waiter that started first can still read the last result; stale ones are cleaned up later.

tsunagi cannot type into another terminal itself: that needs a pty proxy, which would change who owns the terminal, so it stays with the terminal tool.

Checked end to end with real Claude Code 2.1.308 against a fake gateway (`make e2e`): a tool-call loop and a waiter woken by the hook.

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

## Your own gateway

Any provider tsunagi does not list can be added with `tsunagi add <name> <url>`: a LiteLLM or vLLM server, LM Studio, Ollama, a company proxy. The dialect defaults to `openai-chat` (or `anthropic` when the URL ends in `/anthropic`), a host on this machine is treated as needing no key, and `--header "Name: value"`, `--auth-header x-api-key` and `--strategy` cover gateways with their own conventions. `ollama`, `lmstudio` and `vllm` are built in at their default local ports. `tsunagi models <name>` asks the gateway what it serves, and `add` probes a local gateway right away. A gateway you add is your own choice, so it is not pinned and may point at loopback or private addresses; only endpoints from the downloaded catalogue are barred from those.

## Interface

The intro is the eel swimming through bubbles and revealing the name, then the boot lines, then a small map of where requests can go (Claude Code, tsunagi, Anthropic and every ready provider) with pulses running along the links. Any key skips the animations: the terminal is switched to a no-wait read mode and restored afterwards, with no background reader left to swallow a keystroke meant for Claude Code, and a signal restores it too. `tsunagi watch` follows every session's event file, shows account, tokens and failovers per request and a five-minute sparkline, and the eel swims faster while requests flow. `tsunagi usage` draws balances as `[####------]` gauges. Everything is plain ASCII, with color only where the terminal supports it.

## Account ranking by headroom

`"strategy": "usage"` orders accounts by the share of quota left, for providers with a balance endpoint (OpenRouter, DeepSeek, Moonshot). Lookups never delay a request: the first one answers from what is known and refreshes in the background (cached two minutes); an account of unknown headroom counts as half full. Failover still applies on top.

## Slash commands and display modes

The launcher writes a small plugin (`/tsunagi:usage`, `accounts`, `providers`, `models`, `sessions`, `handoff`) to the cache directory and passes it with `--plugin-dir`, so the user's own `~/.claude` is untouched. Each command runs one fixed command line of the tsunagi binary, granted as exactly that line (`allowed-tools: Bash(<path> usage)`, no wildcard), is marked `disable-model-invocation`, and takes no arguments: text typed after a slash command would be spliced into a shell line, so none is accepted. The binary path must need no quoting, otherwise the plugin is skipped. Checked with real Claude Code: `/tsunagi:accounts` ran the binary and its table reached the model.

Every command has the same data in several shapes: a table for people, `--json` for scripts, `watch --log` and `--json` for pipes, a one-line `statusline` for Claude Code's status bar (opt-in, since a status line replaces the user's own), and `--plain`/`--no-color`/`--no-anim` for terminals that cannot take the animation.

## Combos and smart routing

A Combo is a named set of `provider/model` targets one model name can fan out to, configured under `"combos"` in config.json:

```json
"combos": {"free/auto": {"targets": ["opencode/big-pickle", "openrouter/qwen/qwen3-coder:free"], "strategy": "priority"}}
```

A Tier maps a Claude tier to a model or combo, so Claude Code's own tier names can land on another provider:

```json
"tiers": {"haiku": "free/auto", "opus": "claude-opus-4-5"}
```

Resolution order: an exact `models` alias wins; then, for a `claude-*` name containing `-opus`, `-sonnet` or `-haiku`, the matching tier; then, when the resolved name equals a combo name, combo routing; otherwise the usual Route. A tier value may itself name a Claude model, which rewrites the request's model field on the Claude branch like an alias does.

A combo target is `provider[@label]/model`. Before a request is sent, each target is checked the same way `tsunagi route` checks it: the provider must exist and be ready, some account must have a usable key or sign-in, and the request must fit the model's capabilities from the catalogue — a request carrying `tools` skips models without tool calling, and an estimated size over the context window skips the target. Unknown capability data is permissive. A target that fails any check is skipped with a reason; when every target is skipped the client gets a 400 listing them all.

Targets are tried in order under the combo's strategy: `priority` keeps the written order, `fastest` sorts by an EWMA of time to response headers (unmeasured targets count as zero so they get measured), `round_robin` rotates the starting target per combo, `cheapest` sorts by the catalogue price per million tokens (input plus output, free models first, unknown prices last), `context` puts the largest context window first and `least_used` the target with the fewest attempts. Circuit-open targets always sort last.

Every target but the last deliverable one is tried soft: if its final account attempt ends in a retryable failure — 429, 402, 401 or 403, a timeout, a 5xx or a transport error — nothing has reached the client yet, the reply is drained and the next target is tried. The last deliverable target answers as-is, exactly like a direct route, including its errors. Within a target the per-account failover and `provider@label` pinning work unchanged. A non-retryable 4xx is a request fault and is delivered rather than retried.

Each target's health is tracked by consecutive failures, EWMA latency and a circuit. A failure is a transport error, 408, 429 or 5xx; success is under 400; other 4xx are neutral. Three consecutive failures open the circuit for 30 seconds, doubling on each re-open up to 10 minutes. After the cooldown exactly one probe is let through; a success closes the circuit and resets the backoff, a failure re-opens it doubled. All timing goes through the router's clock so tests can advance it.

Every routed request's event gains `via` (the combo name) and `why` (a compact trace such as `opencode/big-pickle 429 -> openrouter/qwen/qwen3-coder:free ok; skipped groq/x: no tool calling`, or `tier haiku -> free/auto` for a rewrite). `tsunagi route <model> [--tools] [--tokens N] [--json]` prints the plan the router would use without sending anything, and `tsunagi watch --log` shows `via=`/`why=` when present.

Claude models are not allowed as combo targets in v1: a combo exists to pick among non-Claude providers, and letting one name a Claude model would blur the credential boundary the router keeps.

## Agent skills

`tsunagi skills` installs four small agent skills (`tsunagi-handoff`, `tsunagi-models`, `tsunagi-setup`, `tsunagi-free`) so other coding agents know how to drive tsunagi: pass plans to a session, pick models, add providers, and use free models safely. Each skill is embedded in the binary and written as `<dir>/<name>/SKILL.md` with YAML frontmatter into exactly two targets: `claude` (`~/.claude/skills`, read by Claude Code) and `agents` (`~/.agents/skills`, read by Codex, Cursor, opencode and Pi). Global skill directories are used here — unlike the slash commands, which stay scoped to `--plugin-dir` — because skills are meant to be picked up by agents tsunagi never launches; Desktop sessions included, which tsunagi does not start.

A manifest at `~/.config/tsunagi/skills.json` (0600, atomic) records the tsunagi version and, per installed file, the target, skill, absolute path and sha256 of what was written, plus which target dirs tsunagi created. Install writes only missing or outdated files: a file whose on-disk hash still matches the manifest but differs from the embedded copy is updated; a file the user edited (hash differs from the manifest) is reported `modified` and left alone; a same-named directory tsunagi never wrote is `foreign` and skipped; a symlinked skill dir is refused. Remove deletes only files whose hash still matches the manifest, drops emptied skill dirs, removes a target skills dir only when tsunagi created it and it is now empty, and deletes the manifest when its last entry is gone. Nothing outside the two target dirs and the manifest is touched.

`tsunagi uninstall [--keys] [--config] [--yes]` reverses the footprint: it removes the manifest-owned skills, the whole `<UserCacheDir>/tsunagi` subdir (events, catalogue and probe caches, the generated plugin, handoff session files), and with `--keys` every keychain entry plus the ChatGPT sign-in files, and with `--config` the whole `~/.config/tsunagi` directory. It lists exactly what will go before removing anything; without `--yes` a non-TTY run prints the list and exits 2. It never edits `~/.claude/settings.json` and never removes the binary — it prints how (`brew uninstall tsunagi` or `rm <exe>`).

## Living next to other Claude Code sessions

tsunagi runs next to Claude Desktop and plain terminal sessions that use the same login, so a tsunagi session must not change what they do.

- The router address, the Stop hook (`--settings` inline) and the slash commands (`--plugin-dir`) reach only the child `claude` process. Nothing is written to `~/.claude` unless you install skills.
- Claude Code saves each `/model` choice as `model` in the user settings. `/model openrouter/x` in a tsunagi session would otherwise make that the default everywhere. `internal/claudeguard` polls the settings file every 100 ms while the session runs. When `model` becomes a tsunagi route (a `provider/model` pair, or one of your combo or alias names) it puts the saved value back, or removes the member if there was none. The running session keeps its model in memory, so it is unaffected.
- A Claude model picked in the session (`/model opus`) is left alone and becomes the value to keep: tsunagi cannot tell it from a change made in another session, and it is harmless to them.
- The edit replaces only the value of the top-level `model` member in the existing text, through a temp file and rename (through a symlink if the file is one), so formatting and every other key stay as they are. A file that is missing or not valid JSON is left alone.
- A state file per running tsunagi (`<cache>/tsunagi/guard-<pid>.json`) records the saved value. The next launch repairs the settings if a previous tsunagi was killed before it could restore them.
- `CLAUDE_CONFIG_DIR` is respected. A separate config directory is not used to isolate sessions: on macOS Claude Code keys its login to that directory, so tsunagi would need a second login, and it never handles Claude credentials.
- tsunagi reads Claude Code's settings files (user, project, local and managed) read-only through `internal/claudecfg`, and never writes them except for the one `model` member described above. It looks for what would get in the way of a session: an `env` block that sets `ANTHROPIC_BASE_URL` (Claude would bypass the router), `CLAUDE_CODE_USE_BEDROCK` and its siblings, an `availableModels` allow-list or a `PreModelSwitch` hook (either can refuse `/model provider/model`). `tsunagi doctor`, the Status screen and the launch output report them. A base-URL conflict in a user, project or local file is fixed for that session only: the session's own inline `--settings` carries `env.ANTHROPIC_BASE_URL`, which Claude Code ranks above those three files, so nothing on disk changes. Managed settings rank above `--settings`; a conflict there is reported but cannot be overridden. Checked with real Claude Code: with a project `.claude/settings.local.json` pointing at a dead address, plain `claude` failed with ECONNREFUSED and `tsunagi claude` answered.

## Claude first, then a fallback

Claude Code's own limits (a 429 when the plan's usage is spent, a 529 when Anthropic is overloaded) stop a session even though other providers could carry on. `"claude_fallback": "free/auto"` in the config (a combo name, or one `provider/model`) turns that into a cascade: a chat request still goes to Anthropic first, byte for byte and with your own login. Only when Anthropic answers 429 or 529, before any byte has reached Claude Code, the same conversation goes to the fallback, and Claude is skipped for the time of the `Retry-After` header (30 seconds to 5 minutes, one minute when absent). Everything else, `count_tokens` included, is passed through unchanged.

It is off by default, because it sends your conversation to another provider without asking each time. A combo still obeys its own gates: `free/auto` only holds providers whose data policy you accepted. The fallback is the same translation path as `/model provider/model`, so thinking blocks and tool calls survive. `tsunagi route claude-sonnet-5-5` shows the second step, and the event log records `claude 429 -> ...` in `why`.
