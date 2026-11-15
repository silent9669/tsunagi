# Policy and risk

This is a design stance, not legal advice. Re-read the sources before building or using the tool.

## What the sources say

- Anthropic's legal and compliance page allows the end user to sign in to the unmodified Claude Code binary with their own subscription. It forbids third-party developers from routing requests through Free, Pro, Max or Ultra plan credentials on users' behalf, and from collecting or intermediating claude.ai credentials or session tokens. Anthropic may enforce without notice.
- The LLM gateway docs state that routing Claude Code to non-Claude models through any gateway is not supported. That wording means unsupported, not forbidden.
- OpenCode documents Claude Code as a supported harness through `ANTHROPIC_BASE_URL` and a `oc_sk_...` key.

Sources:

- https://code.claude.com/docs/en/legal-and-compliance
- https://code.claude.com/docs/en/llm-gateway
- https://opencode.ai/docs/console/harnesses/claude-code/

## Where tsunagi sits

tsunagi is a local pass-through for the user's own, unmodified Claude Code. It is closer to the allowed case than a cloaking proxy, but it is still a grey area and not guaranteed safe.

Design rules that keep it on the safe side:

1. Localhost only. One user, one machine.
2. The Claude branch forwards requests unchanged. No credential extraction, no header rewriting, no identity spoofing.
3. Never store, log or transmit Claude credentials or session tokens.
4. Never pool a subscription or serve another person's requests.
5. Keys and Sign-in tokens for other Providers live in the macOS keychain, not in shell profiles, repositories or chat.

Sign-in for other Providers follows each Provider's own rules, not Anthropic's. OpenAI runs an official "Sign in with ChatGPT" program that covers open-source and local personal apps. Google forbids third-party use of Gemini and Antigravity OAuth, so tsunagi never implements it. Where a Provider's stance is unconfirmed, Sign-in is opt-in and can be removed.

Patterns that were flagged or banned elsewhere and are out of scope: relays that present subscription credentials on behalf of other clients, and proxies that disguise non-Claude-Code traffic as Claude Code.

## Residual risk

- Anthropic can change enforcement or terms at any time.
- A gateway in front of Claude Code is an unsupported setup, so issues are on the user.
- Use at your own risk. If the Claude account matters more than the extra models, do not run the Claude branch through the router. Use a separate terminal profile for OpenCode only.
