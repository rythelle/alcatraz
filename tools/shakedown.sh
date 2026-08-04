#!/bin/bash
# shakedown - compress verbose command output before it lands in an AI context
# window. (Renamed from 'slim'; run as 'slim' still works for one cycle.)
#
# Test runners, builds and installs routinely print thousands of lines the
# model doesn't need; shakedown keeps the head, the tail and the error/warning
# lines, and points at the full log for on-demand recall.
#
# Usage:
#   shakedown <command> [args...]    run a command, print a compressed view
#   shakedown last                   print the FULL output of the last run
#
# Tuning (env vars):
#   SHAKEDOWN_THRESHOLD  pass output through untouched below this many lines (60)
#   SHAKEDOWN_HEAD       lines kept from the start (15)
#   SHAKEDOWN_TAIL       lines kept from the end (25)
#   SHAKEDOWN_LOG        where the full log is written (/tmp/shakedown-last.log)
#
# Not for interactive commands: output is buffered, nothing streams live.
set -u

# ── Module gate ── the shim is always mounted, but the module can be off. Only
# refuse when the container explicitly set the flag off; unset (e.g. run by hand
# outside the sandbox) still works.
case "$(printf '%s' "${ALCATRAZ_MOD_SHAKEDOWN:-}" | tr '[:upper:]' '[:lower:]')" in
    off|0|false|no|disabled)
        echo "module 'shakedown' is off — enable with ALCATRAZ_MOD_SHAKEDOWN=on (in .env or the TUI Modules screen)" >&2
        exit 1 ;;
esac

# ── Deprecated-alias notice ── invoked as 'slim' still works this cycle.
if [ "$(basename "$0")" = "slim" ]; then
    echo "note: 'slim' was renamed to 'shakedown'; the old name will be removed next cycle." >&2
fi

# New env names, with the old SLIM_* honored as a fallback for one cycle.
LOG="${SHAKEDOWN_LOG:-${SLIM_LOG:-/tmp/shakedown-last.log}}"
THRESHOLD="${SHAKEDOWN_THRESHOLD:-${SLIM_THRESHOLD:-60}}"
HEAD_LINES="${SHAKEDOWN_HEAD:-${SLIM_HEAD:-15}}"
TAIL_LINES="${SHAKEDOWN_TAIL:-${SLIM_TAIL:-25}}"
MATCH_MAX=30

if [ $# -eq 0 ] || [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
    sed -n '2,19p' "$0" | sed 's/^# \{0,1\}//'
    exit 0
fi

if [ "$1" = "last" ]; then
    if [ -f "$LOG" ]; then
        cat "$LOG"
        exit 0
    fi
    echo "shakedown: no previous run (log not found: $LOG)" >&2
    exit 1
fi

"$@" > "$LOG" 2>&1
status=$?

total=$(wc -l < "$LOG")
if [ "$total" -le "$THRESHOLD" ]; then
    cat "$LOG"
    exit "$status"
fi

head -n "$HEAD_LINES" "$LOG"

# Error/warning lines from the omitted middle, deduplicated and capped.
# 'grep -n' keeps original line numbers so the model can navigate the full log.
middle_start=$((HEAD_LINES + 1))
middle_end=$((total - TAIL_LINES))
matches=$(sed -n "${middle_start},${middle_end}p" "$LOG" \
    | grep -inE '(^|[^a-z])(error|err!|fail|failed|failure|warn|warning|exception|traceback|fatal|panic|not ok|✗|✖)([^a-z]|$)' \
    | awk -F: '!seen[substr($0, index($0,":")+1)]++' \
    | head -n "$MATCH_MAX")

omitted=$((total - HEAD_LINES - TAIL_LINES))
echo "── shakedown: ${omitted} lines omitted ──"
if [ -n "$matches" ]; then
    echo "── error/warning lines from the omitted part (line: text) ──"
    echo "$matches" | while IFS= read -r m; do
        n="${m%%:*}"
        echo "$((n + middle_start - 1)): ${m#*:}"
    done
    echo "──"
fi

tail -n "$TAIL_LINES" "$LOG"

kept=$((HEAD_LINES + TAIL_LINES + $(echo -n "$matches" | grep -c . || true)))
echo "── shakedown: ${total} -> ~${kept} lines. exit ${status}. full output: 'shakedown last' (${LOG}) ──"
exit "$status"
