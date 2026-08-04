#!/bin/bash
# Tests for tools/shakedown.sh (host-run, no docker needed).
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHAKEDOWN="$SCRIPT_DIR/../tools/shakedown.sh"
export SHAKEDOWN_LOG="$(mktemp)"
trap 'rm -f "$SHAKEDOWN_LOG"' EXIT

pass=0; fail=0
ok()   { pass=$((pass+1)); echo "  ✓ $1"; }
bad()  { fail=$((fail+1)); echo "  ✗ $1"; }
check() { # <desc> <expected-in-output>
    local desc="$1" needle="$2" out="$3"
    if echo "$out" | grep -qF -- "$needle"; then ok "$desc"; else bad "$desc (missing: $needle)"; fi
}

echo "== short output passes through untouched =="
out=$(bash "$SHAKEDOWN" echo "hello world")
check "content preserved" "hello world" "$out"
if ! echo "$out" | grep -q "shakedown:"; then ok "no banner on short output"; else bad "unexpected banner"; fi

echo "== long output is compressed =="
out=$(bash "$SHAKEDOWN" bash -c 'for i in $(seq 1 500); do echo "line $i"; done; echo "ERROR: broke at the middle" >&2; for i in $(seq 501 1000); do echo "line $i"; done')
check "head kept" "line 1" "$out"
check "tail kept" "line 1000" "$out"
check "omission banner" "lines omitted" "$out"
check "summary with recall hint" "shakedown last" "$out"
lines=$(echo "$out" | wc -l)
if [ "$lines" -lt 100 ]; then ok "output actually small ($lines lines for 1001)"; else bad "output too big: $lines lines"; fi

echo "== error lines from the omitted middle are surfaced =="
check "error surfaced" "ERROR: broke at the middle" "$out"

echo "== exit code is preserved =="
bash "$SHAKEDOWN" bash -c 'seq 1 200; exit 7' >/dev/null
[ $? -eq 7 ] && ok "exit 7 preserved" || bad "exit code lost"
bash "$SHAKEDOWN" true >/dev/null
[ $? -eq 0 ] && ok "exit 0 preserved" || bad "exit 0 lost"

echo "== shakedown last replays the full log =="
bash "$SHAKEDOWN" bash -c 'seq 1 300' >/dev/null
out=$(bash "$SHAKEDOWN" last)
[ "$(echo "$out" | wc -l)" -eq 300 ] && ok "full 300 lines recalled" || bad "full log truncated"

echo "== legacy SLIM_* env still honored =="
LEGACY_LOG="$(mktemp)"
SHAKEDOWN_LOG="" SLIM_LOG="$LEGACY_LOG" bash "$SHAKEDOWN" bash -c 'seq 1 5' >/dev/null
[ -s "$LEGACY_LOG" ] && ok "SLIM_LOG fallback works" || bad "SLIM_LOG fallback ignored"
rm -f "$LEGACY_LOG"

echo "== module gate: OFF refuses to run =="
out=$(ALCATRAZ_MOD_SHAKEDOWN=off bash "$SHAKEDOWN" echo hi 2>&1)
rc=$?
check "off notice printed" "module 'shakedown' is off" "$out"
[ "$rc" -ne 0 ] && ok "off exits non-zero" || bad "off should exit non-zero"

echo "== deprecated 'slim' alias runs but warns =="
LINK_DIR="$(mktemp -d)"; ln -s "$SHAKEDOWN" "$LINK_DIR/slim"
out=$(bash "$LINK_DIR/slim" echo "still works" 2>&1)
check "alias still runs" "still works" "$out"
check "rename notice shown" "renamed to 'shakedown'" "$out"
rm -rf "$LINK_DIR"

echo ""
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
