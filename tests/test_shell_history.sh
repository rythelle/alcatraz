#!/usr/bin/env bash
# Regression tests for the persistent shell history block in
# docker-init/opencode-tui-shim.sh (mounted as ~/.local/state/ai-env.sh and
# sourced by ~/.bashrc in every interactive container shell).
#
# Why this matters: the sandbox home IS a named volume, so ~/.bash_history
# already survives restarts — but bash only flushes it on a CLEAN exit. A shell
# killed by `alcatraz stop`, a closed terminal or a container restart lost every
# command typed in it. The block below flushes after each command instead, which
# is what makes the history actually persist.
#
# The tests source the file in a bare bash and inspect the resulting shell
# options — the real container is never involved.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHIM="$SCRIPT_DIR/../docker-init/opencode-tui-shim.sh"

pass=0
fail=0

# probe <expression> [extra-sources] — sources the shim in a clean bash and
# echoes the value of <expression>.
probe() {
    local expr="$1" times="${2:-1}"
    HOME=/tmp/alcatraz-history-test ALCATRAZ_NO_AUTOLOAD=1 \
    bash --norc --noprofile -c '
        for _ in $(seq "$2"); do source "$1"; done
        eval "echo \"$3\""
    ' _ "$SHIM" "$times" "$expr" 2>/dev/null
}

expect() {
    local desc="$1" want="$2" got="$3"
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

contains() {
    local desc="$1" needle="$2" hay="$3"
    case "$hay" in
        *"$needle"*)
            echo "  ✓ $desc"
            pass=$((pass + 1))
            ;;
        *)
            echo "  ✗ $desc"
            echo "      expected to contain: '$needle'"
            echo "      got               : '$hay'"
            fail=$((fail + 1))
            ;;
    esac
}

echo "shell history — persistence"

# The history file must live in $HOME, which is the persisted alcatraz-home
# volume; anywhere else and it dies with the container.
expect "HISTFILE lives in the persisted home" \
    "/tmp/alcatraz-history-test/.bash_history" "$(probe '$HISTFILE')"

# The whole point: flush after every command, so a killed shell keeps its
# history instead of losing the session.
contains "every command is appended immediately" "history -a" "$(probe '$PROMPT_COMMAND')"

# Sourcing twice (bashrc + a nested shell) must not stack duplicate hooks.
expect "the flush hook is registered only once" "1" \
    "$(probe '$(echo "$PROMPT_COMMAND" | grep -c "history -a")' 3)"

# The Mega Brain autoload hook shares PROMPT_COMMAND; neither may evict the other.
contains "the Mega Brain autoload hook survives" "__alcatraz_mb_autoload" "$(probe '$PROMPT_COMMAND')"

# Concurrent shells (a jail shell beside an AI CLI) must not truncate each
# other's history — that needs histappend, not overwrite.
expect "history is appended, never overwritten" "shopt -s histappend" "$(probe '$(shopt -p histappend)')"

# 1000 entries (the Debian default) is a couple of days of work; the point of
# persisting history is being able to look further back than that.
history_size="$(probe '$HISTSIZE')"
if [ "${history_size:-0}" -ge 10000 ]; then
    echo "  ✓ history keeps more than the stock 1000 entries ($history_size)"
    pass=$((pass + 1))
else
    echo "  ✗ history keeps more than the stock 1000 entries"
    echo "      got HISTSIZE: '$history_size'"
    fail=$((fail + 1))
fi

echo ""
echo "passed: $pass   failed: $fail"
[ "$fail" -eq 0 ]
