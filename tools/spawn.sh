#!/bin/bash
# spawn — in-sandbox shim for the disposable-spawn bridge.
#
# The sandbox has NO access to Docker (by design). This shim never touches
# Docker: it only drops a request file into the project's .alcatraz/requests/,
# which is bind-mounted to the host. A host-side `alcatraz spawn-watch` picks it
# up, runs the real disposable spawn, and writes the answer to
# .alcatraz/results/<nonce>.md — which you read back here.
#
# Fire-and-forget: this returns immediately. Read the result file when it lands.
#
# Usage:
#   spawn "<task>"                 delegate a read-only exploration task
#   AGENT=codex spawn "<task>"     pick the agent (claude|codex|gemini|opencode)
set -euo pipefail

# Module gate: the shim is always mounted, but spawn can be off. Refuse only
# when the container explicitly set the flag off (unset = allow).
case "$(printf '%s' "${ALCATRAZ_MOD_SPAWN:-}" | tr '[:upper:]' '[:lower:]')" in
    off|0|false|no|disabled)
        echo "module 'spawn' is off — enable with ALCATRAZ_MOD_SPAWN=on (in .env or the TUI Modules screen)" >&2
        exit 1 ;;
esac

task="$*"
if [ -z "${task// /}" ]; then
    echo "usage: spawn \"<task>\"   (set AGENT=codex|gemini|opencode to pick the agent)" >&2
    exit 1
fi

agent="${AGENT:-claude}"

# Locate the project root: everything is mounted at /workspace/projects/<name>.
# The base is overridable so the shim can be exercised outside a container; in
# the sandbox it is always the real mount point.
projects_dir="${ALCATRAZ_PROJECTS_DIR:-/workspace/projects}"
case "$PWD/" in
    "$projects_dir"/*/*)
        proj="$projects_dir/$(printf '%s' "${PWD#"$projects_dir"/}" | cut -d/ -f1)" ;;
    *)
        echo "spawn: run this inside a mounted project ($projects_dir/<name>)" >&2
        exit 1 ;;
esac

reqdir="$proj/.alcatraz/requests"
mkdir -p "$reqdir"

# Random hex nonce (safe charset) correlates this request to its result file.
nonce="$(head -c6 /dev/urandom | od -An -tx1 | tr -d ' \n')"

tmp="$reqdir/.$nonce.json.tmp"
final="$reqdir/$nonce.json"

# Encode with alcatraz-helper so any characters in the task are safely
# JSON-escaped — the host reads this as untrusted input and validates it
# strictly. The helper is a static binary shipped with the image on purpose: it
# must keep working whichever language runtime you put in the sandbox.
helper="${ALCATRAZ_HELPER:-alcatraz-helper}"
if ! command -v "$helper" >/dev/null 2>&1; then
    echo "spawn: alcatraz-helper not found — rebuild the image ('alcatraz build')" >&2
    exit 1
fi
"$helper" bridge-request --kind spawn \
    --task "$task" --agent "$agent" --nonce "$nonce" --out "$tmp"
mv "$tmp" "$final"   # atomic publish — the watcher never sees a partial file

echo "✓ queued spawn ($agent) [$nonce]"
echo "  read the result at: .alcatraz/results/$nonce.md"
echo "  (needs 'alcatraz spawn-watch' running on the host to service it)"
