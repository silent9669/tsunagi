# tsunagi (繋ぎ)

Run Claude Code against Claude and many other providers' models from one terminal session, using your own keys and without touching your Claude subscription credentials.

> **Status: design stage.** Nothing is built yet. This repository holds the design docs only.

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
- The Claude branch is a byte-for-byte pass-through. No header rewriting, no token handling, no logging of credentials.
- Scoped to a terminal process. Your normal Claude desktop app and plain `claude` runs stay untouched.
- macOS only for now.
- Hand off a plan from a normal Claude session to a tsunagi session, and report back when done.

## Non-goals

- Pooling or sharing Claude subscription credentials.
- Serving other people or other tools from the same proxy.
- Impersonating Claude Code or rewriting its identity headers.

## How it works

See [docs/design.md](docs/design.md) for the router, the model mapping and the plan hand-off loop. Read [docs/policy.md](docs/policy.md) before using it: routing Claude Code through a gateway is unsupported by Anthropic and carries account risk.

## Planned usage

```bash
tsunagi            # starts the local router and launches Claude Code through it
```

Inside the session:

```text
/model claude-sonnet-5-5     # Claude, billed to your subscription
/model opencode/minimax-m3   # OpenCode model, billed to your OpenCode plan
/model deepseek/deepseek-v4-pro
```

## Documents

- [docs/design.md](docs/design.md): architecture, routing rules, hand-off loop, open questions
- [docs/providers.md](docs/providers.md): which providers speak what
- [GLOSSARY.md](GLOSSARY.md): project vocabulary
- [docs/policy.md](docs/policy.md): compliance stance and known risks
- [docs/roadmap.md](docs/roadmap.md): build order and test checklist
