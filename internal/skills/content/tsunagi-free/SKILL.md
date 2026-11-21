---
name: tsunagi-free
description: Use when the user asks about free models, wants zero-cost models found or probed, or wants the free/auto combo built.
---

# Free models

- `tsunagi free` lists providers with a verified free offer; `tsunagi free <provider>` shows its free models plus signup and key links.
- `tsunagi free refresh` re-fetches the public model lists; `tsunagi free probe [provider]` sends real test requests and caches the results.
- `tsunagi free combo [--no-probe] [--dry-run]` builds the `free/auto` combo from probed candidates.
- `tsunagi free allow <provider>` / `tsunagi free deny <provider>` edit the data-policy allow list.

Data policy: providers labelled `private` are used automatically. Providers labelled `trains` are used only after the user explicitly agrees — ask before running `tsunagi free allow`. Providers labelled `unknown` are never used.
