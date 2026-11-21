#!/bin/sh
# UI end to end: first-run wizard in a real tmux pane against the e2e build
# (file keychain, recorded browser opens, stub claude). Temp HOME only.
set -eu
cd "$(dirname "$0")/.."
command -v tmux >/dev/null 2>&1 || { echo "tmux not found: brew install tmux"; exit 1; }
OUT=docs/ui/captures
mkdir -p "$OUT"
rm -f "$OUT"/*.txt

mkdir -p .build
T=.build/tsunagi-e2e
go build -tags e2e -o "$T" .

HOME=$(mktemp -d)
E2E=$(mktemp -d)
export HOME TSUNAGI_E2E_DIR="$E2E" TSUNAGI_NO_ANIM=1
cat > "$E2E/claude-stub" <<'EOF'
#!/bin/sh
echo "STUB-CLAUDE-OK $ANTHROPIC_BASE_URL"
EOF
chmod +x "$E2E/claude-stub"
export TSUNAGI_E2E_CLAUDE="$E2E/claude-stub"

python3 e2e/fakegw.py & GW=$!
trap 'kill $GW 2>/dev/null || true; tmux kill-session -t tsunagi-e2e 2>/dev/null || true' EXIT
sleep 1

cap() { tmux capture-pane -p -t tsunagi-e2e > "$OUT/$1.txt"; }
key() { tmux send-keys -t tsunagi-e2e "$1"; sleep "${2:-0.4}"; }
text() { tmux send-keys -l -t tsunagi-e2e "$1"; sleep "${2:-0.4}"; }
downs() { i=0; while [ $i -lt "$1" ]; do key Down 0.06; i=$((i+1)); done; }

tmux new-session -d -s tsunagi-e2e -x 100 -y 30 \
  "env HOME='$HOME' TSUNAGI_E2E_DIR='$E2E' TSUNAGI_E2E_CLAUDE='$E2E/claude-stub' TSUNAGI_NO_ANIM=1 sh -c 'stty -a > \"$OUT/stty-before.txt\"; \"$T\"; echo APP-EXIT=$?; stty -a > \"$OUT/stty-after.txt\"; exec sh'"
sleep 1.5
cap 01-welcome

# welcome -> providers
key C-m 0.6
cap 02-providers

# the checklist has 23 items on a fresh HOME; "Add a gateway" is last
downs 22
sleep 0.5
key Space 0.3; key C-m 0.6
cap 03-gwname
text e2e 0.2
key C-m 0.5
cap 04-gwurl
text "http://127.0.0.1:59997/v1" 0.2
key C-m 1.0
cap 05-after-gw

# with "e2e" added the list has 24 items; "e2e" is second to last. Local
# providers verify directly, no key needed.
downs 22
sleep 0.5
cap 05-after-gw
key Space 0.3; key C-m 0.5
sleep 3
cap 06-verify-result
key C-m 1.0   # Next -> providers done -> skills screen
cap 07-skills
# install only the claude target: agents Pending -> Done -> None
key j 0.3; key Space 0.3; key Space 0.3; key k 0.3
key C-m 1.0
cap 08-skills-result
key C-m 1.0   # Done -> ready screen
cap 09-ready
key C-m 2.0   # Start Claude Code -> stub prints and exits
sleep 1
tmux capture-pane -p -S -100 -t tsunagi-e2e > "$OUT/10-after-start.txt"
cap 11-exit

# assertions
ok=1
chk() { grep -q "$2" "$OUT/$1.txt" || { echo "MISSING in $1: $2"; ok=0; }; }
chk 01-welcome "Set up tsunagi"
chk 03-gwname "Gateway name"
chk 05-after-gw "e2e"
chk 06-verify-result "e2e/"
chk 08-skills-result "claude/tsunagi"
chk 10-after-start "STUB-CLAUDE-OK"

# status, free and settings screens in one session (same HOME: onboarded -> menu)
M=tsunagi-e2e-more
mcap() { tmux capture-pane -p -t "$M" > "$OUT/$1.txt"; }
mkey() { tmux send-keys -t "$M" "$1"; sleep "${2:-0.4}"; }
tmux new-session -d -s "$M" -x 100 -y 30 \
  "env HOME='$HOME' TSUNAGI_E2E_DIR='$E2E' TSUNAGI_E2E_CLAUDE='$E2E/claude-stub' TSUNAGI_NO_ANIM=1 '$T'"
sleep 1.2
mkey 6;  mcap 12-status
mkey Enter
mkey 4;  mcap 13-free
mkey 3;  mcap 14-free-policy
mkey Escape; mkey Escape
mkey 5;  mcap 15-settings
mkey 1;  mcap 16-routing
mkey Escape
mkey 2;  mkey 1 0.6; mcap 17-appearance
mkey Escape; mkey Escape; mkey q 0.6
tmux kill-session -t "$M" 2>/dev/null || true
chk 12-status "claude"
chk 13-free "Build free/auto"
chk 15-settings "Appearance"
chk 16-routing "Haiku"
chk 17-appearance "dither"
grep -q '"look": "dither"' "$HOME/.config/tsunagi/config.json" \
  || { echo "look not saved to config.json"; ok=0; }

# menu-only captures at the other sizes (same HOME: onboarded -> menu)
for sz in "100 30" "60 20" "40 12"; do
  set -- $sz; W=$1; H=$2
  tmux new-session -d -s "tsunagi-e2e-$W" -x "$W" -y "$H" \
    "env HOME='$HOME' TSUNAGI_E2E_DIR='$E2E' TSUNAGI_E2E_CLAUDE='$E2E/claude-stub' TSUNAGI_NO_ANIM=1 '$T'"
  sleep 1.2
  tmux capture-pane -p -t "tsunagi-e2e-$W" > "$OUT/menu-${W}x${H}.txt"
  tmux kill-session -t "tsunagi-e2e-$W"
done

# ANSI captures of the menu in the colour and mono themes
for th in colour mono; do
  tmux new-session -d -s "tsunagi-e2e-$th" -x 100 -y 30 \
    "env HOME='$HOME' TSUNAGI_E2E_DIR='$E2E' TSUNAGI_E2E_CLAUDE='$E2E/claude-stub' TSUNAGI_NO_ANIM=1 '$T' --theme $th"
  sleep 1.2
  tmux capture-pane -p -e -t "tsunagi-e2e-$th" > "$OUT/menu-100x30-$th-ansi.txt"
  tmux kill-session -t "tsunagi-e2e-$th"
done

# captures are committed: drop temp paths and the per-run router address/token
sed -i '' -E 's#/[^ ]*/claude-stub#<claude-stub>#; s#127\.0\.0\.1:[0-9]+/[0-9a-f]{32}#127.0.0.1:PORT/TOKEN#' "$OUT/10-after-start.txt" "$OUT/11-exit.txt"

sed -i '' -E 's#/var/folders/[^ ]*#<tmp>#; s#/[^ ]*/bin/claude#<claude>#' "$OUT/12-status.txt"

# terminal state: stty before/after must match
diff "$OUT/stty-before.txt" "$OUT/stty-after.txt" >/dev/null \
  && echo "stty: identical" || { echo "stty DIFFERS"; ok=0; }

[ "$ok" = 1 ] && echo "e2e-ui: ok" || { echo "e2e-ui: FAILED"; exit 1; }
