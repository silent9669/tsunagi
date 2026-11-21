---
name: tsunagi-setup
description: Use when the user asks to add a provider, check whether `tsunagi` is configured correctly, list accounts or diagnose a broken session.
---

# Setup and diagnostics

- `tsunagi doctor` checks that everything is in place.
- `tsunagi providers` lists providers and key status; `tsunagi providers --all` includes catalogue entries.
- `tsunagi accounts` lists stored keys and sign-ins.
- `tsunagi add <name> <url>` adds a custom gateway or provider (`--dialect`, `--local`, `--header`, `--auth-header`, `--strategy` tune it).
- `tsunagi sync` refreshes the models.dev catalogue.

Keys: never ask for or accept a key in chat, and never run `tsunagi login` yourself — the prompt is hidden and only the user can type into it. Tell the user to run `tsunagi login <provider>` in their own terminal.
