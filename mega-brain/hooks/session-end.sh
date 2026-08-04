#!/bin/bash
# Session-end hook adapter (Claude SessionEnd / Gemini SessionEnd / Codex Stop).
# Auto-save backstop: records the session end in the project timeline.
# Takes the model name as the first argument; prints "{}" (valid JSON for any CLI).
# Skip when this is a disposable spawn OR the Mega Brain module is off.
case "${ALCATRAZ_MOD_MEGABRAIN:-on}" in off|0|false|no|disabled|OFF|FALSE|NO) MB_OFF=1;; esac
if [ -n "${ALCATRAZ_SPAWN:-}" ] || [ -n "${MB_OFF:-}" ]; then echo '{}'; exit 0; fi
export PATH="/home/alcatraz_runner/.local/bin:$PATH"
mega-brain hook-session-end "${1:-?}" >/dev/null 2>&1 || true
echo '{}'
exit 0
