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

## Usage

```bash
go build -o tsunagi . && mv tsunagi /usr/local/bin/   # macOS, Go 1.27+
tsunagi login opencode        # key is typed hidden, saved in the keychain
tsunagi login chatgpt         # or sign in with your ChatGPT plan (browser, no key)
tsunagi                       # splash, starts the router, launches claude through it
tsunagi import opencode       # reuse the providers and API keys you set up in opencode
tsunagi sync                  # fetch the models.dev catalogue (about 300 providers)
tsunagi providers [--all]     # providers and key status
tsunagi models deepseek       # model names to type after /model
tsunagi login openai --as work   # a second account; requests fail over when one is rate limited
tsunagi accounts              # every key and sign-in
tsunagi add mygw http://127.0.0.1:4000/v1   # your own gateway (LiteLLM, vLLM, LM Studio, Ollama, a company proxy)
tsunagi usage                 # tokens per account, plus balances where the provider shows them
tsunagi ping --tool <model>   # one real test request through the router (add --tool to test tool calls)
tsunagi watch                 # live traffic, run it in a second terminal tab
```

Inside the session:

```text
/model claude-sonnet-5-5       # Claude, billed to your subscription
/model opencode/minimax-m3     # OpenCode model, billed to your OpenCode plan
/model deepseek/deepseek-v4-pro
/model openai@work/gpt-5.5          # pin one account; without @label the pool picks and fails over
/model chatgpt/gpt-6.1-sol        # your ChatGPT plan; names from `tsunagi models chatgpt`
```

Claude Code prints a "does not recognize the model" notice for non-Claude names and assumes a 200k context window. The notice is cosmetic.

Set `TSUNAGI_NO_ANIM=1` to skip animation, or `NO_COLOR=1` for plain text. Extra providers go in `~/.config/tsunagi/config.json` (`name`, `base_url`, `dialect`). The same file takes `"models": {"claude-haiku-5-5": "opencode/glm-5.3", "fast": "deepseek/deepseek-v4-flash"}` to rewrite a model name before routing, for example to send Claude Code's background haiku calls to a cheaper provider. Any provider in the catalogue works as `/model <provider>/<model>` once it has a key; OAuth logins in opencode cannot be imported.

## Documents

- [docs/design.md](docs/design.md): architecture, routing rules, hand-off loop, open questions
- [docs/providers.md](docs/providers.md): which providers speak what
- [GLOSSARY.md](GLOSSARY.md): project vocabulary
- [docs/policy.md](docs/policy.md): compliance stance and known risks
- [docs/roadmap.md](docs/roadmap.md): build order and test checklist
