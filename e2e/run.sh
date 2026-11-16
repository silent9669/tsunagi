#!/bin/sh
# End to end: real Claude Code -> tsunagi -> translator -> a fake OpenAI-compatible
# gateway. Checks a plain reply, a tool call loop, and the hand-off done file.
# Needs `claude` on PATH. Uses an API key placeholder, so no login is touched.
set -eu
cd "$(dirname "$0")/.."
go build -o tsunagi .
T=$PWD/tsunagi
python3 e2e/fakegw.py & GW=$!
trap 'kill $GW 2>/dev/null; TSUNAGI_NO_ANIM=1 $T remove e2e-gw >/dev/null 2>&1 || true' EXIT
sleep 1
TSUNAGI_NO_ANIM=1 $T add e2e-gw http://127.0.0.1:59997/v1 >/dev/null
export ANTHROPIC_API_KEY=sk-ant-e2e-placeholder TSUNAGI_NO_ANIM=1
( sleep 0.6; $T wait --timeout 60s > /tmp/tsunagi-e2e-wait.txt 2>&1 ) &
W=$!
out=$($T -p "run the echo" --model e2e-gw/tiny-model --allowedTools Bash </dev/null 2>&1 | tail -1)
wait $W
case "$out" in *tsunagi-e2e*) echo "tool loop: ok";; *) echo "tool loop: FAILED ($out)"; exit 1;; esac
grep -q "finished (hook)" /tmp/tsunagi-e2e-wait.txt && echo "hand-off: ok" || { echo "hand-off: FAILED"; exit 1; }
