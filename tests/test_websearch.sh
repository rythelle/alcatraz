#!/bin/bash
# Tests for tools/websearch.sh — the in-sandbox shim (host-run, no docker).
#
# The shim's job is narrow: refuse obvious junk locally, then publish a
# well-formed request atomically and wait for the host's answer. The host
# re-validates everything (see websearch_test.go) — these tests cover the shim.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SHIM="$SCRIPT_DIR/../tools/websearch.sh"

pass=0; fail=0
ok()  { pass=$((pass+1)); echo "  ✓ $1"; }
bad() { fail=$((fail+1)); echo "  ✗ $1"; }

# The shim insists on running inside a mounted project, so fake that layout.
ROOT="$(mktemp -d)"
PROJ="$ROOT/workspace/projects/demo"
mkdir -p "$PROJ/sub"
trap 'rm -rf "$ROOT"' EXIT

# The shim resolves the project from $PWD under the projects mount point; point
# that base at the fake tree so it works outside a container.
export ALCATRAZ_PROJECTS_DIR="$ROOT/workspace/projects"

# In the image the shim calls alcatraz-helper (a static binary, so the plumbing
# survives swapping the sandbox's language runtime). Build it here and point the
# shim at it.
if ! (cd "$SCRIPT_DIR/../platform/sandbox-helper" && \
      CGO_ENABLED=0 go build -o "$ROOT/alcatraz-helper" . ) 2>/dev/null; then
    echo "cannot build platform/sandbox-helper — is Go installed?" >&2
    exit 1
fi
export ALCATRAZ_HELPER="$ROOT/alcatraz-helper"

run_shim() { # <cwd> <args...>
    local cwd="$1"; shift
    (cd "$cwd" && bash "$SHIM" "$@" 2>&1)
}

echo "== module gate =="
out=$(cd "$PROJ" && ALCATRAZ_MOD_WEBSEARCH=off bash "$SHIM" "hello world" 2>&1)
echo "$out" | grep -q "is off" && ok "refuses when the module is off" || bad "module gate missing: $out"

echo "== local pre-flight refusals =="
out=$(run_shim "$PROJ" --async "check https://evil.example/steal?d=1")
echo "$out" | grep -q "URLs are not accepted" && ok "refuses a URL" || bad "URL accepted: $out"

long=$(head -c 300 < /dev/zero | tr '\0' 'a')
out=$(run_shim "$PROJ" --async "$long")
echo "$out" | grep -q "too long" && ok "refuses an oversized query" || bad "long query accepted: $out"

out=$(run_shim "$PROJ" --async "")
echo "$out" | grep -q "usage:" && ok "refuses an empty query" || bad "empty query accepted: $out"

out=$(run_shim "$PROJ" --bogus "x")
echo "$out" | grep -q "unknown flag" && ok "refuses an unknown flag" || bad "unknown flag accepted: $out"

echo "== outside a mounted project =="
out=$(cd "$ROOT" && bash "$SHIM" --async "hello world" 2>&1)
echo "$out" | grep -q "mounted project" && ok "refuses outside /workspace/projects" || bad "ran anywhere: $out"

echo "== queued request shape =="
rm -rf "$PROJ/.alcatraz"
out=$(run_shim "$PROJ" --async 'bun 1.2 "breaking" changes')
req=$(ls "$PROJ/.alcatraz/requests"/*.json 2>/dev/null | head -n1)
if [ -n "$req" ]; then
    ok "request file published"
    node -e '
      const fs = require("fs");
      const r = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
      const errs = [];
      if (r.kind !== "search") errs.push("kind=" + r.kind);
      if (r.query !== `bun 1.2 "breaking" changes`) errs.push("query=" + r.query);
      if (!/^[a-f0-9]{6,32}$/.test(r.nonce)) errs.push("nonce=" + r.nonce);
      if ("task" in r || "agent" in r) errs.push("carries task/agent");
      if (errs.length) { console.error(errs.join("; ")); process.exit(1); }
    ' "$req" && ok "request json is well-formed (kind/query/nonce, no task or agent)" \
             || bad "malformed request json"
    case "$(basename "$req")" in
        .*) bad "published a dotfile temporary" ;;
        *)  ok "atomic publish (no leftover .tmp)" ;;
    esac
    ls "$PROJ/.alcatraz/requests"/.*.json.tmp >/dev/null 2>&1 && bad "temp file left behind" || ok "no temp file left behind"
else
    bad "no request file written"
fi

echo "== from a subdirectory of the project =="
rm -rf "$PROJ/.alcatraz"
run_shim "$PROJ/sub" --async "postgres index bloat" >/dev/null
[ -n "$(ls "$PROJ/.alcatraz/requests"/*.json 2>/dev/null)" ] \
    && ok "resolves the project root from a subdir" || bad "did not resolve project root"

echo "== waiting mode prints the host's answer =="
rm -rf "$PROJ/.alcatraz"
mkdir -p "$PROJ/.alcatraz/results"
# Drop the result in as soon as the request appears, imitating the watcher.
(
  for _ in $(seq 1 50); do
      req=$(ls "$PROJ/.alcatraz/requests"/*.json 2>/dev/null | head -n1) || true
      if [ -n "${req:-}" ]; then
          nonce=$(basename "$req" .json)
          printf '# Web search\n\nUNTRUSTED CONTENT marker\n' > "$PROJ/.alcatraz/results/$nonce.md"
          exit 0
      fi
      sleep 0.1
  done
) &
out=$(run_shim "$PROJ" --timeout 15 "bun 1.2 release notes")
wait
echo "$out" | grep -q "UNTRUSTED CONTENT marker" \
    && ok "prints the result on stdout (lands in the agent's context)" \
    || bad "result not printed: $out"

echo "== timeout is bounded =="
rm -rf "$PROJ/.alcatraz"
start=$(date +%s)
out=$(run_shim "$PROJ" --timeout 2 "nobody is listening")
elapsed=$(( $(date +%s) - start ))
if echo "$out" | grep -q "timed out" && [ "$elapsed" -lt 10 ]; then
    ok "gives up after --timeout (${elapsed}s) and says where the result will land"
else
    bad "timeout not honored (${elapsed}s): $out"
fi

echo ""
echo "passed: $pass  failed: $fail"
[ "$fail" -eq 0 ]
