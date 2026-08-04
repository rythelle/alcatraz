#!/bin/bash
# SessionStart hook adapter (Gemini CLI).
# Gemini requires a single JSON object on stdout: hookSpecificOutput.additionalContext.
# Skip when this is a disposable spawn OR the Mega Brain module is off.
case "${ALCATRAZ_MOD_MEGABRAIN:-on}" in off|0|false|no|disabled|OFF|FALSE|NO) MB_OFF=1;; esac
if [ -n "${ALCATRAZ_SPAWN:-}" ] || [ -n "${MB_OFF:-}" ]; then echo '{"hookSpecificOutput":{"additionalContext":""}}'; exit 0; fi
export PATH="/home/alcatraz_runner/.local/bin:$PATH"
export MB_CTX="$(mega-brain context-md 2>/dev/null)"
# alcatraz-helper JSON-escapes the context for us. It is a static binary rather
# than a script for the language runtime of the day: swapping the sandbox's
# stack (Node → Java, …) must not silently break the memory hooks.
# Gemini rejects hookEventName, so it is left out here.
exec "${ALCATRAZ_HELPER:-alcatraz-helper}" hook-session-start
