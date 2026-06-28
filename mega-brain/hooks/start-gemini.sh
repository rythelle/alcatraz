#!/bin/bash
# SessionStart hook adapter (Gemini CLI).
# Gemini requires a single JSON object on stdout: hookSpecificOutput.additionalContext.
export PATH="/home/alcatraz_runner/.local/bin:$PATH"
export MB_CTX="$(mega-brain context-md 2>/dev/null)"
exec python3 - <<'PY'
import json, os
print(json.dumps({
    "hookSpecificOutput": {
        "additionalContext": os.environ.get("MB_CTX", ""),
    }
}))
PY
