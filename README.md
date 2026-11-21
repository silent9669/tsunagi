# tsunagi (繋ぎ)

Run Claude Code against Claude and many other providers' models from one terminal session, using your own keys and without touching your Claude subscription credentials.

> **Status: Phase 2 built, not yet checked against the live services.** Router, launcher, keychain login, provider list, live watch, the models.dev catalogue, opencode import, Responses and chat translators, and Sign in with ChatGPT are implemented and covered by tests with fake upstreams. Nothing has yet been run end to end against a real provider, and Grok sign-in is not built.

## Why

Claude Code reads one `ANTHROPIC_BASE_URL` per process, so `/model` cannot switch between providers. Subagent-only tools such as cameo let other models appear as subagents, but a delegated model cannot own a whole task end to end.

tsunagi is a small local router. Claude Code talks to it, and it forwards each request by its `model` field:

| Model | Destination | Credential |
| --- | --- | --- |
| Claude (`claude-*`) | `https://api.anthropic.com`, request forwarded untouched | Your own Claude Code login, as sent by the unmodified client |
| `<provider>/<model>` | That provider (OpenCode, OpenRouter, DeepSeek, Kimi, GLM, MiniMax, Qwen, Fireworks, local) | Your key for it, from the macOS keychain |
| Any other name | The default provider | Its key |

`/model` then switches providers like it switches Claude models.

## Goals

- One Claude Code session, one `/model` picker, two providers.
- The Claude branch forwards the request body and headers unchanged. No header rewriting, no token handling, no logging of credentials.
- Scoped to a terminal process. Your normal Claude desktop app and plain `claude` runs stay untouched.
- macOS only for now.
- Hand off a plan from a normal Claude session to a tsunagi session, and report back when done.

## Non-goals

- Pooling or sharing Claude subscription credentials.
- Serving other people or other tools from the same proxy.
- Impersonating Claude Code or rewriting its identity headers.

## How it works

See [docs/design.md](docs/design.md) for the router, the model mapping and the plan hand-off loop. Read [docs/policy.md](docs/policy.md) before using it: routing Claude Code through a gateway is unsupported by Anthropic and carries account risk.

## Install

macOS only (arm64 and amd64).

```bash
brew install silent9669/tap/tsunagi
# or
curl -fsSL https://raw.githubusercontent.com/silent9669/tsunagi/main/install.sh | sh
# or from source (Go 1.27+)
go build -o tsunagi . && mv tsunagi /usr/local/bin/
```

Pushing a `v*` tag runs CI; when it passes, GoReleaser publishes the GitHub release and adds the cask to the Homebrew tap (`silent9669/homebrew-tap`) through a deploy key limited to that repo (secret `HOMEBREW_TAP_DEPLOY_KEY`).

## Usage

```bash
tsunagi login opencode        # key is typed hidden, saved in the keychain
tsunagi login chatgpt         # or sign in with your ChatGPT plan (browser, no key)
tsunagi                       # menu: Start Claude Code, Providers, Skills, Free models, Settings, Status (first run: setup wizard)
tsunagi setup                 # run the setup wizard again
tsunagi claude [args]         # skip the menu: start the router and launch claude with args
tsunagi import opencode       # reuse the providers and API keys you set up in opencode
tsunagi sync                  # fetch the models.dev catalogue (about 300 providers)
tsunagi providers [--all]     # providers and key status
tsunagi models deepseek       # model names to type after /model
tsunagi login openai --as work   # a second account; requests fail over when one is rate limited
tsunagi accounts              # every key and sign-in
tsunagi add mygw http://127.0.0.1:4000/v1   # your own gateway (LiteLLM, vLLM, LM Studio, Ollama, a company proxy)
tsunagi sessions              # running sessions; `tsunagi handoff` shows how to pass a plan to one
tsunagi usage                 # tokens per account, plus balances where the provider shows them
tsunagi ping --tool <model>   # one real test request through the router (add --tool to test tool calls)
tsunagi route <model>         # how a name would route: combo targets, skips, circuit state (--tools --tokens N --json)
tsunagi free [--all]          # providers with a verified free offer, limits and data policy (--json)
tsunagi free <provider>       # its free models, then signup/key/source facts
tsunagi free probe|combo      # probe candidates, then write the free/auto combo (--no-probe --dry-run)
tsunagi free refresh          # re-read the public free-model lists
tsunagi free allow|deny <p>   # opt a "trains" provider into free/auto or out
tsunagi skills                # install status of tsunagi's agent skills per target (--json)
tsunagi skills install        # write them for Claude Code and/or Codex, Cursor, opencode (--target claude|agents|all)
tsunagi uninstall             # list and remove what tsunagi installed (--keys --config --yes)

tsunagi watch                 # live traffic from every session: accounts, tokens, failovers, a sparkline; run it in a second tab
```

Inside the session, tsunagi adds slash commands (loaded with `--plugin-dir`, nothing is installed in your Claude Code setup). They take no arguments, only you can run them, and each is allowed exactly one command line; use the CLI for `models <provider>` and `ping`:

```text
/tsunagi:usage        /tsunagi:accounts     /tsunagi:providers
/tsunagi:models       /tsunagi:sessions     /tsunagi:handoff
```

And the model switch:

```text
/model claude-sonnet-5-5       # Claude, billed to your subscription
/model opencode/minimax-m3     # OpenCode model, billed to your OpenCode plan
/model deepseek/deepseek-v4-pro
/model openai@work/gpt-5.5          # pin one account; without @label the pool picks and fails over
/model chatgpt/gpt-6.1-sol        # your ChatGPT plan; names from `tsunagi models chatgpt`
/model free/auto                  # a combo: tries its targets in order until one answers
```

Claude Code prints a "does not recognize the model" notice for non-Claude names and assumes a 200k context window. The notice is cosmetic.

Claude Code saves every `/model` choice as your default in `~/.claude/settings.json`, which Claude Desktop and your other sessions share. While a tsunagi session runs, tsunagi puts that one value back whenever a non-Claude model is written there (the running session keeps its model), so your default never ends up pointing at another provider. It edits only the `model` member in place. `TSUNAGI_NO_MODEL_GUARD=1` turns it off.

Display modes: `--plain`, `--no-color` and `--no-anim` before a command; `--json` on `providers`, `accounts`, `usage`, `sessions` and `models`; `tsunagi watch` is a dashboard, `watch --log` prints one line per request (pipeable), `watch --json` streams events. `TSUNAGI_STATUSLINE=1` also puts a one-line tsunagi status in Claude Code's status line (it replaces your own). `tsunagi doctor` checks the setup, `tsunagi look [--style <look>] [--theme <theme>]` previews the art style and color theme (`--static` prints one frame), `tsunagi completion zsh|bash|fish` prints a completion script, `make e2e` runs a real Claude Code against a fake gateway. `TSUNAGI_NO_HOOK=1` and `TSUNAGI_NO_PLUGIN=1` turn off the Stop hook and the slash commands.

Any key skips the intro animation. Set `TSUNAGI_NO_ANIM=1` to turn animation off, or `NO_COLOR=1` for plain text. Extra providers go in `~/.config/tsunagi/config.json` (`name`, `base_url`, `dialect`). The same file takes `"models": {"claude-haiku-5-5": "opencode/glm-5.3", "fast": "deepseek/deepseek-v4-flash"}` to rewrite a model name before routing, for example to send Claude Code's background haiku calls to a cheaper provider. `"combos"` names a set of `provider/model` targets one model name fans out to — `"free/auto": {"targets": ["opencode/big-pickle", "openrouter/qwen/qwen3-coder:free"], "strategy": "priority"}` with `priority`, `fastest` or `round_robin` — and `"tiers"` maps `opus`, `sonnet` or `haiku` to a model or combo. Combo targets that cannot serve a request (no key, no tool calling, context too small, circuit open) are skipped with a reason; `tsunagi route` shows the plan without sending anything. `tsunagi free` lists providers with a verified free offer — see [docs/free.md](docs/free.md) — and `tsunagi free combo` builds `free/auto` from probed candidates; providers whose free tier may train on prompts need `tsunagi free allow <provider>` first. Any provider in the catalogue works as `/model <provider>/<model>` once it has a key; OAuth logins in opencode cannot be imported.

## Documents

- [docs/design.md](docs/design.md): architecture, routing rules, hand-off loop, open questions
- [docs/providers.md](docs/providers.md): which providers speak what
- [GLOSSARY.md](GLOSSARY.md): project vocabulary
- [docs/policy.md](docs/policy.md): compliance stance and known risks
- [docs/roadmap.md](docs/roadmap.md): build order and test checklist
