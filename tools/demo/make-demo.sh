#!/usr/bin/env bash
# Records demotermdevtools.gif, the README's animated demo: the real binary,
# driven by VHS (https://github.com/charmbracelet/vhs) in a real terminal,
# against the secured test cluster filled with a few made-up indices.
#
#   tools/testclusters.sh up secured-es-9.5.4
#   tools/demo/make-demo.sh [binary [gif]]   (defaults: dist/termdevtools-linux-amd64,
#                                             demotermdevtools.gif at the repository's root)
#
# Linux only (WSL included). Needs vhs, ttyd, ffmpeg and curl; VHS downloads
# the headless Chromium it renders with on its first run. Run from anywhere:
# paths are taken from this script's location. The indices are deleted and
# created again on every run, so every recording starts from the same data.
set -eu

repo=$(cd "$(dirname "$0")/../.." && pwd)
binary=$(realpath -m "${1:-$repo/dist/termdevtools-linux-amd64}")
output=$(realpath -m "${2:-$repo/demotermdevtools.gif}")

# The secured cluster of tools/testclusters.sh: a throwaway container on
# this machine's loopback interface, with a password that protects nothing.
cluster=http://localhost:19395
password=tdt-throwaway

for tool in vhs ttyd ffmpeg curl; do
  command -v "$tool" >/dev/null || { echo "$tool not found" >&2; exit 1; }
done
[ -x "$binary" ] || { echo "no termdevtools binary at $binary (bash build-release.sh)" >&2; exit 1; }

es() { # es METHOD PATH [BODY]: one request to the cluster, its answer dropped
  curl -fsS -u "elastic:$password" -X "$1" "$cluster/$2" \
    -H 'Content-Type: application/json' ${3:+--data-binary "$3"} >/dev/null
}
curl -fsS -u "elastic:$password" "$cluster" >/dev/null ||
  { echo "the cluster doesn't answer at $cluster (tools/testclusters.sh up secured-es-9.5.4)" >&2; exit 1; }

# --- Data: the same every time (bash's RANDOM, seeded) -----------------------
# RANDOM is only ever read here, never inside a $(...): bash reseeds it in a
# subshell, and the data would change from one recording to the next.
RANDOM=42
# pick LIST: one of LIST's items, in $picked.
pick() { local -n list=$1; picked=${list[RANDOM % ${#list[@]}]}; }
index() { # index NAME SETTINGS DOCUMENTS...: (re)create, fill, refresh
  local name=$1 settings=$2; shift 2
  es DELETE "$name" 2>/dev/null || true
  es PUT "$name" "$settings"
  # $(...) drops the final newline a bulk request must end with: put it back.
  es POST "$name/_bulk?refresh=true" "$(printf '{"index": {}}\n%s\n' "$@")"$'\n'
}
plain='{"settings": {"number_of_replicas": 0}}'

categories=(Electronics Books Garden Kitchen Sports Toys)
docs=()
for i in $(seq 1 21); do
  pick categories
  docs+=("{\"name\": \"Product $i\", \"category\": \"$picked\", \"price\": $((RANDOM % 300 + 5)).99}")
done
index products "$plain" "${docs[@]}"

statuses=(paid paid paid shipped shipped delivered cancelled)
docs=()
for i in $(seq 1 29); do
  pick statuses
  printf -v created '2026-10-%02d' $((RANDOM % 21 + 1))
  docs+=("{\"order_id\": \"ORD-$((1000 + i))\", \"status\": \"$picked\", \"total\": $((RANDOM % 900 + 10)).50, \"created_at\": \"$created\"}")
done
index orders "$plain" "${docs[@]}"

docs=()
for login in alice bob carol dave erin frank grace heidi ivan judy; do
  pick categories
  docs+=("{\"login\": \"$login\", \"team\": \"$picked\"}")
done
index users "$plain" "${docs[@]}"

levels=(INFO INFO INFO INFO INFO INFO WARN WARN ERROR ERROR)
services=(api auth billing search)
INFO=("request served" "user logged in" "cache refreshed" "index rotated")
WARN=("slow query: 2.4s" "retrying request" "disk usage above 80%")
ERROR=("upstream timeout after 30s" "connection refused by billing-db" "payment declined: card expired")
logs='{"settings": {"number_of_replicas": 0}, "mappings": {"properties": {"level": {"type": "keyword"}, "service": {"type": "keyword"}, "message": {"type": "text"}}}}'
for day in 20 21; do
  docs=()
  for i in $(seq 1 45); do
    pick levels; level=$picked
    pick "$level"; message=$picked
    pick services
    printf -v time '%02d:%02d' $((RANDOM % 24)) $((RANDOM % 60))
    docs+=("{\"@timestamp\": \"2026-10-${day}T$time:00Z\", \"level\": \"$level\", \"service\": \"$picked\", \"message\": \"$message\"}")
  done
  index "app-logs-2026.10.$day" "$logs" "${docs[@]}"
done

# --- A fresh configuration: one known cluster, its secret in ES_PW ------------
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin" "$work/config/termdevtools"
ln -s "$binary" "$work/bin/termdevtools"
cat > "$work/config/termdevtools/config.yaml" <<EOF
language: en
clusters:
  - url: $cluster
    auth_type: basic
    username: elastic
    secret_env: ES_PW
    tls:
      verify: true
EOF
cat > "$work/config/termdevtools/queries_http___localhost_19395.txt" <<'EOF'
# Ctrl+E runs the request under the cursor
GET _cluster/health


EOF

# --- The recording ----------------------------------------------------------
# VHS has no command for the function keys, nor for Alt with an arrow: their
# escape sequences are typed instead, fast enough to arrive as one key.
esc=$(printf '\033')
f4="${esc}OS" f8="${esc}[19~" end="${esc}[F" alt_right="${esc}[1;3C"

cat > "$work/demo.tape" <<EOF
Output "$output"
Set Shell bash
Set FontFamily "DejaVu Sans Mono"
Set FontSize 16
Set Width 1280
Set Height 720
Set Padding 12
Set TypingSpeed 45ms
Set WaitTimeout 20s
# Fewer frames than VHS's default 50 per second, which a slower machine
# can't capture in time: playback then runs faster than the pauses below.
# A busy machine can still cause it: the gif should last about 38 seconds.
Set Framerate 25

Hide
Type "export PATH=$work/bin:\$PATH XDG_CONFIG_HOME=$work/config ES_PW=$password PS1='$ '; clear"
Enter
Show

Sleep 600ms
Type "termdevtools"
Sleep 400ms
Enter
Wait+Screen /New connection/
Sleep 1500ms

# The known cluster: its form opens with the secret read from ES_PW.
Enter
Wait+Screen /Secret read from the ES_PW variable/
Sleep 2800ms
Enter
Wait+Screen /Requests/
Sleep 1200ms

# Run a request.
Down
Ctrl+E
Wait+Screen /"status": "green"/
Sleep 2000ms

# Endpoint and _cat column completion.
Down 2
Type "GET _cat/"
Sleep 300ms
Tab
Sleep 1200ms
Type "in"
Sleep 900ms
Enter
Sleep 900ms
Type "&h=index,docs.c"
Sleep 300ms
Tab
Sleep 900ms
Type ",store.size&s=docs.count:d"
Sleep 300ms
Tab
Sleep 900ms
Ctrl+E
Wait+Screen /app-logs-2026/
Sleep 2800ms

# The recipe catalog.
Type@1ms "$f8"
Sleep 1200ms
Type "disk space"
Sleep 1800ms
Enter
Sleep 900ms
Ctrl+E
Sleep 3000ms

# A search, its body reformatted, then searched in the result.
Type@1ms "$end"
Enter 2
Type 'GET app-logs-*/_search'
Enter
Type '{"query": {"term": {"level": "ERROR"}}}'
Sleep 500ms
Type@1ms "$f4"
Sleep 1200ms
Ctrl+E
Wait+Screen /"hits"/
Sleep 1500ms
Type@1ms "$alt_right"
Sleep 400ms
Ctrl+F
Sleep 300ms
Type "timeout"
Enter
Sleep 3500ms
EOF

cd "$work"
vhs demo.tape
ls -l "$output"
