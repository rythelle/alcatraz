#!/bin/bash
# websearch — in-sandbox shim for the host-fetched web search bridge.
#
# The sandbox has NO route to the internet: every request goes Guard →
# Lighthouse, and search engines are deliberately NOT on the allowlist. This
# shim does not change that. It writes a request file into the project's
# .alcatraz/requests/ (bind-mounted to the host); the host-side
# `alcatraz spawn-watch` validates it, asks you to approve it, performs ONE
# https GET against a search endpoint, and writes the hits back to
# .alcatraz/results/<nonce>.md. No agent, no shell, no tools run on the host.
#
# By default this blocks until the answer lands and prints it on stdout — so
# when an agent runs it, the results land directly in its context.
#
# Usage:
#   websearch "<query>"              search and print the results here
#   websearch --async "<query>"      queue it and return immediately
#   websearch --timeout 60 "<query>" how long to wait (default 180s)
#
# The query leaves the jail, so it is treated as an outbound channel and kept
# deliberately narrow: one line, at most 256 characters, no URLs and no
# encoded blobs. The host also runs it through the Guard sanitizer and refuses
# anything carrying a secret. Keep it to plain search words.
set -euo pipefail

# Module gate: the shim is always mounted, but websearch can be off. Refuse only
# when the container explicitly set the flag off (unset = allow).
case "$(printf '%s' "${ALCATRAZ_MOD_WEBSEARCH:-}" | tr '[:upper:]' '[:lower:]')" in
    off|0|false|no|disabled)
        echo "module 'websearch' is off — enable with ALCATRAZ_MOD_WEBSEARCH=on (in .env or the TUI Modules screen)" >&2
        exit 1 ;;
esac

wait_for_result=1
timeout_s=180

while [ $# -gt 0 ]; do
    case "$1" in
        --async)   wait_for_result=0; shift ;;
        --timeout) timeout_s="${2:-180}"; shift 2 ;;
        --help|-h)
            echo "usage: websearch [--async] [--timeout N] \"<query>\""
            exit 0 ;;
        --*)
            echo "websearch: unknown flag $1" >&2
            exit 1 ;;
        *) break ;;
    esac
done

query="$*"
if [ -z "${query// /}" ]; then
    echo "usage: websearch [--async] [--timeout N] \"<query>\"" >&2
    exit 1
fi

case "$timeout_s" in
    ''|*[!0-9]*) echo "websearch: --timeout must be a number of seconds" >&2; exit 1 ;;
esac

# Local pre-flight so a bad query fails here instead of burning an approval
# prompt on the host. The host re-checks all of this — this is convenience,
# not the security boundary.
if [ "${#query}" -gt 256 ]; then
    echo "websearch: query too long (${#query} > 256 chars) — search terms, not payloads" >&2
    exit 1
fi
case "$query" in
    *://*) echo "websearch: URLs are not accepted — search for words, then read the hits" >&2; exit 1 ;;
esac

# Locate the project root: everything is mounted at /workspace/projects/<name>.
# The base is overridable so the shim can be exercised outside a container; in
# the sandbox it is always the real mount point.
projects_dir="${ALCATRAZ_PROJECTS_DIR:-/workspace/projects}"
case "$PWD/" in
    "$projects_dir"/*/*)
        proj="$projects_dir/$(printf '%s' "${PWD#"$projects_dir"/}" | cut -d/ -f1)" ;;
    *)
        echo "websearch: run this inside a mounted project ($projects_dir/<name>)" >&2
        exit 1 ;;
esac

reqdir="$proj/.alcatraz/requests"
resdir="$proj/.alcatraz/results"
mkdir -p "$reqdir"

# Random hex nonce (safe charset) correlates this request to its result file.
nonce="$(head -c6 /dev/urandom | od -An -tx1 | tr -d ' \n')"

tmp="$reqdir/.$nonce.json.tmp"
final="$reqdir/$nonce.json"

# Encode with alcatraz-helper so any characters in the query are safely
# JSON-escaped — the host reads this as untrusted input and validates it
# strictly. The helper is a static binary shipped with the image on purpose: it
# must keep working whichever language runtime you put in the sandbox.
helper="${ALCATRAZ_HELPER:-alcatraz-helper}"
if ! command -v "$helper" >/dev/null 2>&1; then
    echo "websearch: alcatraz-helper not found — rebuild the image ('alcatraz build')" >&2
    exit 1
fi
"$helper" bridge-request --kind search \
    --query "$query" --nonce "$nonce" --out "$tmp"
mv "$tmp" "$final"   # atomic publish — the watcher never sees a partial file

result="$resdir/$nonce.md"

if [ "$wait_for_result" -eq 0 ]; then
    echo "✓ queued web search [$nonce]"
    echo "  read the result at: .alcatraz/results/$nonce.md"
    echo "  (needs 'alcatraz spawn-watch' running on the host to service it)"
    exit 0
fi

echo "⏳ queued web search [$nonce] — waiting for the host to approve and fetch…" >&2
waited=0
while [ ! -f "$result" ]; do
    if [ "$waited" -ge "$timeout_s" ]; then
        echo "websearch: timed out after ${timeout_s}s." >&2
        echo "  The request is still queued; read .alcatraz/results/$nonce.md when it lands." >&2
        echo "  Is 'alcatraz spawn-watch' running on the host?" >&2
        exit 1
    fi
    sleep 1
    waited=$((waited + 1))
done

cat "$result"
