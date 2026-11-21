---
name: tsunagi-handoff
description: Use when the user wants to pass a finished plan to a running `tsunagi` session and get its result back, or asks how to continue work in another session.
---

# Handing a plan to a `tsunagi` session

1. Run `tsunagi sessions` to find the target session id. `tsunagi handoff --session <id>` prints the exact steps for that session.
2. If the plan is long or multi-line, write it to a file first. Multi-line typing into the session can submit early.
3. Send the target session a single line such as `Read <path> and implement it`.
4. Type that line into the session's terminal with whatever terminal-input tool is available. If there is none, show the line and ask the user to paste it into the terminal tab running the session.
5. Run `tsunagi wait --session <id> --timeout 30m` in the background. Its output is the target's last message; report it to the user.

Inside such a session, `tsunagi done "<summary>"` finishes a turn explicitly.
