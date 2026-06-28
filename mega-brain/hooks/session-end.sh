#!/bin/bash
# Session-end hook adapter (Claude SessionEnd / Gemini SessionEnd / Codex Stop).
# Auto-save backstop: records the session end in the project timeline.
# Takes the model name as the first argument; prints "{}" (valid JSON for any CLI).
export PATH="/home/alcatraz_runner/.local/bin:$PATH"
mega-brain hook-session-end "${1:-?}" >/dev/null 2>&1 || true
echo '{}'
exit 0
