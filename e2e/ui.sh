#!/bin/sh
# UI end to end: wizard + new chrome in real tmux panes against the e2e build
# (file keychain, recorded browser opens, stub claude). Temp HOME only.
set -eu
cd "$(dirname "$0")/.."
export LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8
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

# a small cached catalogue: real env names plus 40+ synthetic providers so
# the All view's More section and scrolling show up
mkdir -p "$HOME/Library/Caches/tsunagi"
{
  echo '{"providers":['
  echo '{"id":"groq","name":"Groq","api":"https://api.groq.com/openai/v1","dialect":"openai-chat","env":["GROQ_API_KEY"],"models":["llama-3.3-70b-versatile"]},'
  echo '{"id":"hyperbolic","name":"Hyperbolic","api":"https://api.hyperbolic.xyz/v1","dialect":"openai-chat","env":["HYPERBOLIC_API_KEY"],"models":["x"]},'
  for i in $(seq 1 42); do
    sep=,
    [ "$i" = 42 ] && sep=
    printf '{"id":"cat%02d","name":"Cat %02d","api":"https://cat%02d.example.com/v1","dialect":"openai-chat","models":["x"]}%s\n' "$i" "$i" "$i" "$sep"
  done
  echo ']}'
} > "$HOME/Library/Caches/tsunagi/models.json"

python3 e2e/fakegw.py & GW=$!
SESS=""
cleanup() {
  kill $GW 2>/dev/null || true
  for s in $SESS; do tmux kill-session -t "$s" 2>/dev/null || true; done
}
trap cleanup EXIT
sleep 1

ENVV="HOME='$HOME' TSUNAGI_E2E_DIR='$E2E' TSUNAGI_E2E_CLAUDE='$E2E/claude-stub' TSUNAGI_NO_ANIM=1"
# the renderer emits real tabs as cursor moves; expand them to the spacing
# the terminal showed so captures are what a user sees
cap() { tmux capture-pane -p -t "$1" | expand -t 8 > "$OUT/$2.txt"; }
key() { tmux send-keys -t "$1" "$2"; sleep "${3:-0.4}"; }
text() { tmux send-keys -l -t "$1" "$2"; sleep "${3:-0.4}"; }
ok=1
chk() { grep -q "$2" "$OUT/$1.txt" || { echo "MISSING in $1: $2"; ok=0; }; }
nope() { grep -q "$2" "$OUT/$1.txt" && { echo "LEAKED in $1: $2"; ok=0; } || true; }
scrollto() { # scroll a providers list until the pattern is visible
  for i in 1 2 3 4 5 6 7 8; do
    tmux capture-pane -p -t "$1" | grep -q "$2" && return 0
    tmux send-keys -t "$1" PgDn
    sleep 0.3
  done
  return 1
}

# --- session 1: first-run wizard ---------------------------------------------

S=tsunagi-e2e; SESS="$S"
tmux new-session -d -s $S -x 100 -y 30 \
  "env $ENVV sh -c 'stty -a > \"$OUT/stty-before.txt\"; \"$T\"; echo APP-EXIT=\$?; stty -a > \"$OUT/stty-after.txt\"; exec sh'"
sleep 1.5
cap $S 01-welcome
key $S C-m 0.8
cap $S 02-providers            # Free chip by default, nothing ready yet
text $S "add a" 0.4            # "add a" matches only the gateway action
key $S End 0.3; key $S C-m 0.5
cap $S 03-gwname               # rail: Name current
text $S e2e; key $S C-m 0.5
cap $S 04-gwurl
text $S "http://127.0.0.1:59997/v1"; key $S C-m 1.0
key $S Escape 0.3              # clear the search
key $S Left 0.4                # Free chip -> All so the local gateway shows
scrollto $S e2e || true        # gateway sits in the Local section below the fold
cap $S 05-after-gw
text $S e2e; key $S C-m 0.6    # gateway detail card
cap $S 06-gw-detail
key $S C-m 3.0                 # Test connection -> verify -> result rail
cap $S 07-result-rail
key $S C-m 1.0                 # Done -> list (search still "e2e")
key $S Escape 0.3              # clear the search
key $S End 0.3                 # Continue is the last row
key $S C-m 1.0
cap $S 08-skills
key $S j 0.3; key $S Space 0.3; key $S Space 0.3; key $S k 0.3
key $S C-m 1.0
cap $S 09-skills-result
key $S C-m 1.0
cap $S 10-ready
key $S C-m 2.0                 # Start Claude Code -> stub prints and exits
sleep 1
tmux capture-pane -p -J -S -100 -t $S > "$OUT/11-after-start.txt"
tmux capture-pane -p -J -t $S > "$OUT/12-exit.txt"
tmux kill-session -t $S 2>/dev/null || true

chk 01-welcome "Set up tsunagi"
chk 02-providers "Free"
chk 03-gwname "Name"
chk 05-after-gw "e2e"
chk 07-result-rail "e2e/"
chk 09-skills-result "claude/tsunagi"
chk 11-after-start "STUB-CLAUDE-OK"

# --- session 2: home palette + providers + env + paste (onboarded HOME) -------

M=tsunagi-e2e-more; SESS="$SESS $M"
tmux new-session -d -s $M -x 100 -y 30 \
  "env $ENVV GROQ_API_KEY=dummy-not-a-real-key '$T'"
sleep 1.5
# the first keystrokes must land in the search box: one send-keys, no intro
text $M gro 0.6
cap $M 21-home-gro
key $M Escape 0.3
cap $M 20-home
text $M providers; key $M C-m 0.8
cap $M 22-providers-free       # only kilo(keyless)+local ready -> Free chip
key $M Left 0.4                # -> All chip
cap $M 23-providers-all
text $M kilo; key $M C-m 0.6   # keyless detail card
cap $M 24-kilo-detail
key $M Escape 0.4              # back to list
key $M Escape 0.3              # clear "kilo"
text $M groq; key $M C-m 0.6   # groq detail card
cap $M 25-groq-detail
key $M 2 0.6                   # I have a key -> rail at Key
cap $M 26-rail-key
key $M Escape 0.4              # cancel -> detail
key $M Escape 0.4              # -> list
key $M Escape 0.3              # clear "groq" -> env rows on top
key $M Home 0.3                # first row: import groq $GROQ_API_KEY
key $M C-m 0.6
cap $M 27-env-import
chk 27-env-import 'GROQ_API_KEY'
nope 27-env-import 'dummy-not-a-real-key'
key $M 2 0.5                   # Skip -> hides for the session

# paste-a-key: tmux paste-buffer delivers the whole fake key at once
tmux set-buffer 'gsk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
tmux paste-buffer -t $M
sleep 0.7
cap $M 28-paste-masked
chk 28-paste-masked "gsk_"
chk 28-paste-masked "Looks like Groq"
nope 28-paste-masked 'aaaaaaaaaaaa'
key $M Escape 0.3              # drop the held key
key $M Escape 0.5              # leave -> home

text $M status; key $M C-m 0.6
cap $M 29-status
key $M C-m 0.5                 # Back
text $M "free mo"; key $M C-m 0.6
cap $M 30-free
key $M 3 0.5                   # Data policy
cap $M 31-free-policy
key $M Escape 0.3; key $M Escape 0.4   # -> free menu -> home
text $M settings; key $M C-m 0.6
cap $M 32-settings
key $M 1 0.5                   # Routing
cap $M 33-routing
key $M Escape 0.4
key $M 2 0.4; key $M 1 0.6     # Appearance -> Look cycle
cap $M 34-appearance
grep -q '"look": "dither"' "$HOME/.config/tsunagi/config.json" \
  || { echo "look not saved to config.json"; ok=0; }
key $M Escape 0.4; key $M 3 0.4; key $M 1 0.5   # Advanced -> Reset appearance
grep -q '"look"' "$HOME/.config/tsunagi/config.json" \
  && { echo "reset did not clear the look"; ok=0; } || true
key $M Escape 0.4; key $M Escape 0.4; key $M Escape 0.5   # -> home -> quit
tmux kill-session -t $M 2>/dev/null || true

chk 20-home "Start Claude Code"
chk 21-home-gro "Groq"
chk 22-providers-free "Groq"
chk 23-providers-all "Ready"
chk 24-kilo-detail "no key needed"
chk 25-groq-detail "imits"
chk 26-rail-key "Verify"
chk 29-status "claude"
chk 30-free "Build free/auto"
chk 32-settings "Appearance"
chk 33-routing "Haiku"
chk 34-appearance "dither"

# --- watch -------------------------------------------------------------------

mkdir -p "$HOME/Library/Caches/tsunagi"
python3 - "$HOME/Library/Caches/tsunagi/events-99999.jsonl" <<'PY'
import datetime, json, sys, time
rows = [("claude", "claude-sonnet-5-5", 200, 812, "", 0, "", 1200, 90),
        ("openrouter", "inclusionai/ling-3.2-flash:free", 429, 90, "openrouter", 1, "openrouter/x 429 -> groq/y ok", 0, 0),
        ("groq", "gpt-oss-120b", 200, 320, "groq", 0, "free/auto: groq/gpt-oss-120b ok", 800, 40),
        ("claude", "claude-haiku-5-5", 200, 410, "", 0, "", 300, 12)]
now = time.time()
with open(sys.argv[1], "w") as f:
    for i, (p, m, st, ms, a, sk, why, i_, o) in enumerate(rows):
        t = datetime.datetime.fromtimestamp(now - 3 * (len(rows) - i)).astimezone().isoformat()
        f.write(json.dumps({"t": t, "provider": p, "model": m, "path": "/v1/messages", "status": st, "ms": ms,
                            "account": a, "in": i_, "out": o, "skipped": sk, "why": why}) + "\n")
PY
W=tsunagi-e2e-watch; SESS="$SESS $W"
tmux new-session -d -s $W -x 110 -y 34 \
  "env $ENVV '$T' watch"
sleep 2
cap $W 40-watch
tmux kill-session -t $W 2>/dev/null || true
chk 40-watch "ling-3.2"
rm -f "$HOME/Library/Caches/tsunagi/events-99999.jsonl"

# --- real-size captures: wordmark 120x42, then 110x34 and 80x24 --------------

for sz in "120 42" "110 34" "80 24"; do
  set -- $sz; w=$1; h=$2
  s="tsunagi-e2e-cap$w"; SESS="$SESS $s"
  tmux new-session -d -s "$s" -x "$w" -y "$h" "env $ENVV '$T'"
  sleep 1.2
  cap "$s" "home-${w}x${h}"
  text "$s" gro 0.5
  cap "$s" "home-gro-${w}x${h}"
  key "$s" Escape 0.3
  text "$s" providers; key "$s" C-m 0.7
  cap "$s" "providers-free-${w}x${h}"
  key "$s" Left 0.3          # -> All
  scrollto "$s" "deepseek" || true
  cap "$s" "providers-paid-${w}x${h}"
  key "$s" End 0.4           # bottom: More + actions
  cap "$s" "providers-more-${w}x${h}"
  key "$s" Home 0.3
  text "$s" kilo; key "$s" C-m 0.6
  cap "$s" "detail-kilo-${w}x${h}"
  key "$s" Escape 0.4        # -> list
  key "$s" Escape 0.3        # clear "kilo"
  text "$s" groq; key "$s" C-m 0.6
  key "$s" 2 0.6             # I have a key -> rail at Key
  cap "$s" "rail-key-${w}x${h}"
  tmux kill-session -t "$s" 2>/dev/null || true
done

chk "home-120x42" "███"                    # wordmark at 120x42
chk "providers-paid-80x24" "deepseek"      # scrolling reached Paid
chk "providers-more-80x24" "type to search" # More is capped behind search
chk "detail-kilo-80x24" "with or without"  # limits wrapped, not truncated
chk "rail-key-80x24" "Verify"

# --- compact + theme + ascii captures ----------------------------------------

for sz in "100 30" "60 20" "40 12" "30 8"; do
  set -- $sz; w=$1; h=$2
  s="tsunagi-e2e-$w"; SESS="$SESS $s"
  tmux new-session -d -s "$s" -x "$w" -y "$h" "env $ENVV '$T'"
  sleep 1.2
  cap "$s" "home-${w}x${h}"
  text "$s" providers; key "$s" C-m 0.6
  cap "$s" "providers-${w}x${h}"
  tmux kill-session -t "$s" 2>/dev/null || true
done

for th in colour mono; do
  s="tsunagi-e2e-$th"; SESS="$SESS $s"
  tmux new-session -d -s "$s" -x 100 -y 30 "env $ENVV '$T' --theme $th"
  sleep 1.2
  tmux capture-pane -p -e -t "$s" | expand -t 8 > "$OUT/home-100x30-$th-ansi.txt"
  text "$s" groq; key "$s" C-m 0.6
  tmux capture-pane -p -e -t "$s" | expand -t 8 > "$OUT/detail-100x30-$th-ansi.txt"
  tmux kill-session -t "$s" 2>/dev/null || true
done

s=tsunagi-e2e-ascii; SESS="$SESS $s"
tmux new-session -d -s "$s" -x 80 -y 20 \
  "env $ENVV LANG=C LC_ALL=C '$T'"
sleep 1.2
cap "$s" home-ascii
tmux kill-session -t "$s" 2>/dev/null || true
nope home-ascii '❯'
nope home-ascii '◆'
nope home-ascii '╭'

# captures must not contain a literal tab (stty dumps legitimately do)
for f in "$OUT"/*.txt; do
  case "$f" in */stty-*) continue ;; esac
  grep -q "$(printf '\t')" "$f" && { echo "TAB in $(basename "$f")"; ok=0; } || true
done

# captures are committed: drop temp paths and the per-run router address/token
sed -i '' -E 's#/[^ ]*/claude-stub#<claude-stub>#; s#127\.0\.0\.1:[0-9]+/[0-9a-f]{32}#127.0.0.1:PORT/TOKEN#; s#127\.0\.0\.1:[0-9]+#127.0.0.1:PORT#g' "$OUT/11-after-start.txt" "$OUT/12-exit.txt"
sed -i '' -E 's#/var/folders/[^ ]*#<tmp>#; s#/[^ ]*/bin/claude#<claude>#' "$OUT/29-status.txt"

# terminal state: stty before/after must match
diff "$OUT/stty-before.txt" "$OUT/stty-after.txt" >/dev/null \
  && echo "stty: identical" || { echo "stty DIFFERS"; ok=0; }

[ "$ok" = 1 ] && echo "e2e-ui: ok" || { echo "e2e-ui: FAILED"; exit 1; }
