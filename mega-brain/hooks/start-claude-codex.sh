#!/bin/bash
# SessionStart hook adapter (Claude Code and Codex share the same schema).
# Emits JSON with the project context in hookSpecificOutput.additionalContext.
export PATH="/home/alcatraz_runner/.local/bin:$PATH"
export MB_CTX="$(mega-brain context-md 2>/dev/null)"
exec python3 - <<'PY'
import json, os
print(json.dumps({
    "hookSpecificOutput": {
        "hookEventName": "SessionStart",
        "additionalContext": os.environ.get("MB_CTX", ""),
    }
}))
PY
