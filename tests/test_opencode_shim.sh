#!/usr/bin/env bash
# Regression tests for docker-init/opencode-tui-shim.sh
#
# The shim used to force the interactive TUI to `--mini`, on a wrong diagnosis
# (see the header of the shim itself). The full TUI is the default again, and
# `--mini` is opt-in. These tests pin that behaviour, and — just as important —
# pin that subcommands are never rewritten: an `opencode auth` or
# `opencode run` that silently gained a flag would be a nasty surprise.
#
# The real binary is never executed. A stub `command` builtin override records
# the argv the shim would have run.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHIM="$SCRIPT_DIR/../docker-init/opencode-tui-shim.sh"

pass=0
fail=0

# run_shim <env-assignments> -- <args...> ; echoes the resulting argv
run_shim() {
    local mini="$1"; shift
    [ "${1:-}" = "--" ] && shift
    ALCATRAZ_OPENCODE_MINI="$mini" \
    ALCATRAZ_NO_AUTOLOAD=1 \
    bash --norc --noprofile -c '
        # Stub the real binary: `command opencode …` records its arguments.
        command() {
            if [ "$1" = "opencode" ]; then
                shift
                printf "opencode %s\n" "$*"
                return 0
            fi
            builtin command "$@"
        }
        [ -n "${ALCATRAZ_OPENCODE_MINI:-}" ] || unset ALCATRAZ_OPENCODE_MINI
        source "$1" >/dev/null 2>&1
        shift
        opencode "$@"
    ' _ "$SHIM" "$@" 2>/dev/null
}

expect() {
    local desc="$1" want="$2" got="$3"
    # Collapse whitespace so trailing-arg spacing doesn't matter.
    want="$(echo "$want" | tr -s ' ' | sed 's/ *$//')"
    got="$(echo "$got" | tr -s ' ' | sed 's/ *$//')"
    if [ "$want" = "$got" ]; then
        echo "  ✓ $desc"
        pass=$((pass + 1))
    else
        echo "  ✗ $desc"
        echo "      want: '$want'"
        echo "      got : '$got'"
        fail=$((fail + 1))
    fi
}

echo "opencode shim — default (full TUI)"
expect "bare invocation runs the full TUI" \
    "opencode" "$(run_shim "" --)"
expect "flags are passed through untouched" \
    "opencode --model anthropic/claude-opus-5" \
    "$(run_shim "" -- --model anthropic/claude-opus-5)"
expect "an explicit --mini is still honoured" \
    "opencode --mini" "$(run_shim "" -- --mini)"

echo
echo "opencode shim — ALCATRAZ_OPENCODE_MINI=1 (opt-in line mode)"
expect "bare invocation gains --mini" \
    "opencode --mini" "$(run_shim 1 --)"
expect "--mini is not doubled" \
    "opencode --mini" "$(run_shim 1 -- --mini)"
expect "flags survive alongside --mini" \
    "opencode --mini --model x" "$(run_shim 1 -- --model x)"

echo
echo "opencode shim — subcommands are never rewritten"
for sub in run auth serve models upgrade agent mcp session export github; do
    expect "opencode $sub (mini off)" \
        "opencode $sub" "$(run_shim "" -- "$sub")"
    expect "opencode $sub (mini on)" \
        "opencode $sub" "$(run_shim 1 -- "$sub")"
done
expect "subcommand with arguments" \
    "opencode run fix the failing test" \
    "$(run_shim 1 -- run "fix the failing test")"
expect "auth login keeps both words" \
    "opencode auth login" "$(run_shim 1 -- auth login)"

echo
echo "opencode shim — help and version are never rewritten"
for flag in -h --help -v --version; do
    expect "$flag (mini on)" "opencode $flag" "$(run_shim 1 -- "$flag")"
done

echo
echo "-------------------------------------"
echo "passed: $pass   failed: $fail"
[ "$fail" -eq 0 ] || exit 1
