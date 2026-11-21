---
name: tsunagi-models
description: Use when the user asks which model a request will use, wants to pick or check a model for a session, or asks about routing, combos, tiers or provider accounts.
---

# Models and routing

- `tsunagi route <model> [--tools]` shows which provider/model a name resolves to and why — combos, tiers, circuit state — without sending anything.
- `tsunagi models <provider>` lists a provider's model names; add `--live` for the provider's own list.
- Inside a launched session, `/model <provider>/<model>` switches the model for that session.
- `claude-*` model names always go to the user's own Claude login; other names route by provider.
- `tsunagi usage` shows token counts and balances per account.
- `tsunagi ping --tool <provider/model>` sends one real test request through the router.

Never read, print or ask for API keys. Key entry is a hidden prompt only the user can type into.
