# tsunagi (繋ぎ)

Claude Code on your own Claude login **and** other providers' models, free or paid, in one terminal session. `/model` switches between them. Your Claude credentials are never touched, and your Claude Desktop and plain `claude` sessions are left exactly as they are.

```text
◆ tsunagi 0.6.0                                                                                         claude ✓ · 3 ready

                                ████████╗███████╗██╗   ██╗███╗   ██╗ █████╗  ██████╗ ██╗
                                ╚══██╔══╝██╔════╝██║   ██║████╗  ██║██╔══██╗██╔════╝ ██║
                                   ██║   ███████╗██║   ██║██╔██╗ ██║███████║██║  ███╗██║
                                   ██║   ╚════██║██║   ██║██║╚██╗██║██╔══██║██║   ██║██║
                                   ██║   ███████║╚██████╔╝██║ ╚████║██║  ██║╚██████╔╝██║
                                   ╚═╝   ╚══════╝ ╚═════╝ ╚═╝  ╚═══╝╚═╝  ╚═╝ ╚═════╝ ╚═╝

                        ⢤⣤⢤⣀⢀⣀⢀⣀⢀⣀⢀⣀⢀⣀⢀⣀⠀        ⣀⢀⣀⢠⣤⢤⣤⢤⣤⢤⣤⢄⣀⢀⣤⢤⣤⢤⣤⢤⣤⢤⣀⢀⣀⢀        ⣀⢀⣀⢀⣀⢀⣀⢀⣀⢀⣀⢀⣀
                        ⠝⠝⠝⠝⠝⠝⠝⠝⠝⠝⠝⠝⠝⠝⠝⢟⢟⢟⢟⢟⢿⢟⢟⢟⢟⠟⠝⠝⠝⠝⠝⠝⠝⠝⠝⠝⠝⠝⠝⠝⠝⠝⠝⢝⢙⢽⢝⣽⢝⣽⢟⢟⢟⢟⢿⢟⢟⢟⢟⢟⠟⠝⠝⠝⠝⠝⠝⠝⠟⠝⢿⠽
                        ⠅⠅⠅⠅⠅⠅⠅⠅⠅⠅⠅⢅⠁⢅⠁⢅⠁⢅⠁⢅⠁⢅⠁⢅⠁⠅⠅⠅⠅⠅⠅⠅⠅⠅⠅⢅⢅⢅⢅⣥⢵⢽⢿⢿⠟⢽⠝⢽⠝⢽⢿⢽⢿⢿⢿⢽⢽⢽⢽⢽⢽⢁⢿⢿⣅⣙⢛⠳⠁⢅⠑⢝
                        ⠁⠄⠁⠄⠁⢄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠍⠉⢍⠉⠍⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠅⠉⠍⠙⠝⠙⠝⠙⠌⠉⢍⠉⠄⠁⠄⠁⠄⠁⠄
                        ⠁⠄⠁⠄⠁⢴⠅⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⢴⠅⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⢙⠅⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⢙⠅⠁⠄⠁⠄⠁⠄⠁⠄⠁⠄⠁⢔⠕⠄⠁⠄⠁⠄⠁⠄

      ╭──────────────────────────────────────────────────────────────────────────────────────────────────────────╮
      │ › Search actions, providers…                                                                             │
      ╰──────────────────────────────────────────────────────────────────────────────────────────────────────────╯

      ❯   Start Claude Code                                                                        claude 2.1.311
          Add a free provider                                                                                next
          Providers                                                                                       3 ready
          Free models                                                                                           -
          Skills                                                                                              1/2
          Settings
          Status
          Quit

      type to search   ↑↓ move   ↵ open   esc quit
```

macOS only (arm64 and amd64). Open source, built on your own keys.

## Why

Claude Code reads one `ANTHROPIC_BASE_URL` per process, so `/model` cannot move between providers, and a usage limit stops the session even when other models could carry on. tsunagi is a small router on `127.0.0.1`. Claude Code talks to it and it forwards each request by its `model` field:

| Model name | Goes to | Credential |
| --- | --- | --- |
| `claude-*` | `api.anthropic.com`, request forwarded byte for byte | Your own Claude login, sent by the unmodified client |
| `<provider>/<model>` | That provider | Your key for it, from the macOS keychain |
| a combo or tier name | The combo's targets in order, with failover | Each target's own key |
| anything else | The default provider | Its key |

## Install

```bash
brew install silent9669/tap/tsunagi   # newer Homebrew asks you to trust a third-party tap first: brew trust silent9669/tap
# or
curl -fsSL https://raw.githubusercontent.com/silent9669/tsunagi/main/install.sh | sh
# or from source (Go 1.27+)
go build -o tsunagi . && mv tsunagi /usr/local/bin/
```

Update with `brew upgrade --cask tsunagi`. Remove with `tsunagi uninstall`, then `brew uninstall --cask tsunagi`.

## First run

Run `tsunagi`. The first time it opens a short setup:

1. **Providers**: filter the list (All, Free, Ready, Paid, Local) or type to search about 300 providers, press enter on one and follow its steps (account, key, verify, done). Paste a key anywhere in the search box and tsunagi recognises the provider from its prefix. Keys found in your environment (for example `GROQ_API_KEY`) are offered for import, names only until you agree. Kilo Gateway's free models need no account at all. Free offers show their limits and what the provider does with your prompts. Keys are typed hidden and saved in the macOS keychain.
2. **Skills**: optional skills so Claude Code, Codex, Cursor and opencode can hand off plans and manage tsunagi.
3. **Start Claude Code**.

After that `tsunagi` opens one search box over everything: type to filter actions and all providers (`gro` finds Groq), arrows and enter to pick, esc to clear or leave. With nothing typed, Start Claude Code is selected and a single `next` row suggests the next setup step. It also works with the mouse (click a row, scroll). `tsunagi claude [args]` skips the menu and starts Claude Code with your arguments, and so does running it without a terminal. The interface adapts to any terminal size, down to 20 columns.

In the session:

```text
/model claude-sonnet-5-5         # Claude, on your subscription
/model openrouter/inclusionai/ling-3.2-flash:free
/model deepseek/deepseek-v4-pro
/model openai@work/gpt-5.5       # pin one of several accounts
/model free/auto                 # a combo: tries its targets in order
```

Claude Code prints a "does not recognize the model" notice for non-Claude names and assumes a 200k window. The notice is cosmetic.

## Free models

`tsunagi free` lists providers with a documented, recurring free API tier, with their limits, what they need, and a data label from their own terms: `private` (not used for training) or `trains`. Only labelled providers can enter the `free/auto` combo, and `trains` ones only after `tsunagi free allow <provider>`. The list is checked against the providers' own pages and corrects itself: Mistral, Cerebras, Hugging Face, Ollama Cloud, GitHub Models and ModelScope are documented as not usable this way. Current entries: OpenCode Zen, OpenRouter, Groq, Google AI Studio, SambaNova, Cohere (trial key), NVIDIA NIM, Cloudflare Workers AI, Z.ai, Kilo Gateway (needs no key), Reflection AI (beta), Requesty. Details and sources: [docs/free.md](docs/free.md).

```bash
tsunagi free                 # the offers, their limits and data labels
tsunagi free openrouter      # its free models, signup and key links
tsunagi free probe           # send one tiny real request per candidate model
tsunagi free combo           # write free/auto from the probed models that pass every gate
```

## Routing

A **combo** is a named list of `provider/model` targets. The router skips targets that cannot serve a request (no key, no tool calling, context too small, circuit open), orders the rest by the strategy, and falls through on 429 and 5xx before anything reaches Claude Code.

```json
{
  "combos": { "free/auto": { "targets": ["groq/openai/gpt-oss-120b", "openrouter/qwen/qwen3-coder:free"], "strategy": "priority" } },
  "tiers": { "haiku": "free/auto" },
  "claude_fallback": "free/auto"
}
```

- **Strategies**: `priority` (as written), `fastest` (measured latency), `round_robin`, `cheapest` (catalogue price, free first), `context` (largest window first), `least_used`, `weighted` (a weighted draw per request; `weights` per target, missing counts as 1), `headroom` (most quota left first, unknown last).
- **Tiers**: `opus`, `sonnet` or `haiku` can point at a model or combo, for example Claude Code's background haiku calls on a free model.
- **Scenarios**: a `claude-*` request can be rerouted on its content: `"scenarios": {"think": ..., "long_context": ..., "web_search": ...}` each name a combo or `provider/model`. `long_context` triggers when the input tops `scenario_tokens` (default 60,000). Precedence is long context, then web search, then thinking; an exact alias still wins and a scenario beats a tier. Off by default, because it sends the request to another provider.
- **Claude first, then a fallback**: with `claude_fallback` set, a chat request goes to Anthropic first. Only if Anthropic answers 429 (usage limit) or 529 (overloaded) does the same conversation go to the fallback, and Claude is skipped for the `Retry-After` time. Off by default, because it sends your conversation to another provider.
- **Health**: a circuit opens after three failures, retries after a cooldown, and the soonest-ready target is probed when every one is cooling.
- **Explainable**: `tsunagi route <model> [--tools] [--think] [--web-search] [--tokens N]` shows the plan without sending anything, and `tsunagi watch` and the event log show why each request went where it did.

Settings (in the menu) edits tiers, previews a route, and changes the look and theme live. More in [docs/design.md](docs/design.md).

## Living next to Claude Code

tsunagi runs beside Claude Desktop and plain terminal sessions that use the same login, and does not change what they do:

- The router address, the Stop hook and the slash commands reach only the `claude` process tsunagi starts. Nothing is written to `~/.claude` unless you install skills.
- Claude Code saves every `/model` choice as your default in `~/.claude/settings.json`. While a session runs, tsunagi puts that one value back when a non-Claude model lands there (the running session keeps its model). It edits only that member, in place.
- tsunagi reads your Claude Code settings (read only) for things that would get in the way: an `env` block that points Claude somewhere else, a model allow-list, a `PreModelSwitch` hook. `tsunagi doctor`, Status and the launch card report them, and a conflicting base URL is overridden for that session only.

`TSUNAGI_NO_MODEL_GUARD=1` turns the default-model guard off. Checked with real Claude Code; see [docs/design.md](docs/design.md).

## Commands

```text
tsunagi                    menu (first run: setup)        tsunagi setup       the setup again
tsunagi claude [args]      start Claude Code, no menu     tsunagi watch       live dashboard of every session
tsunagi login <provider>   key (hidden) or ChatGPT        tsunagi accounts    keys and sign-ins
tsunagi providers [--all]  providers and key status       tsunagi models <p>  model names for /model
tsunagi add <name> <url>   your own gateway               tsunagi import opencode
tsunagi usage              tokens and balances            tsunagi sessions    running sessions
tsunagi free ...           free offers, probe, combo      tsunagi route <m>   how a name would route
tsunagi ping --tool <m>    one real test request          tsunagi doctor      what is in place
tsunagi skills ...         agent skills install/remove    tsunagi uninstall   remove what tsunagi added
tsunagi look               preview the art and themes     tsunagi completion  zsh, bash or fish
```

Flags before a command: `--plain`, `--no-color`, `--no-anim`, `--look <braille|dither|halftone|bricks|gradient>`, `--theme <mono|amber|matrix|newsprint|colour>`. Data commands take `--json`. `NO_COLOR=1`, `TSUNAGI_NO_ANIM=1` and `TSUNAGI_NO_MOUSE=1` are honoured. Plain output is used when there is no terminal.

In the session, tsunagi adds slash commands (`/tsunagi:usage`, `accounts`, `providers`, `models`, `sessions`, `handoff`) through `--plugin-dir`. They take no arguments and only you can run them.

## Policy

tsunagi never pools or shares Claude credentials, never rewrites Claude Code's identity headers, and only supports sign-in where a provider's own terms allow it (ChatGPT; not Google's Gemini CLI or Antigravity). Routing Claude Code through a gateway is unsupported by Anthropic. Read [docs/policy.md](docs/policy.md) before relying on it.

## Documents

- [docs/design.md](docs/design.md): architecture, routing, hand-off loop, living next to Claude Code
- [docs/free.md](docs/free.md): the free registry and its sources
- [docs/providers.md](docs/providers.md): which providers speak what
- [docs/policy.md](docs/policy.md): compliance stance and known risks
- [docs/roadmap.md](docs/roadmap.md): what is done and what is open
- [docs/ui/captures](docs/ui/captures): every screen, captured from a real terminal by `make e2e-ui`
- [GLOSSARY.md](GLOSSARY.md): project vocabulary
