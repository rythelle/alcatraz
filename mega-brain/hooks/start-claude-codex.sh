#!/bin/bash
# SessionStart hook adapter (Claude Code and Codex share the same schema).
# Emits JSON with the project context in hookSpecificOutput.additionalContext.
# Skip when this is a disposable spawn OR the Mega Brain module is off. The
# wired hooks live in a persisted settings volume, so an off toggle only takes
# effect because we self-guard here at run time.
case "${ALCATRAZ_MOD_MEGABRAIN:-on}" in off|0|false|no|disabled|OFF|FALSE|NO) MB_OFF=1;; esac
if [ -n "${ALCATRAZ_SPAWN:-}" ] || [ -n "${MB_OFF:-}" ]; then echo '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":""}}'; exit 0; fi
export PATH="/home/alcatraz_runner/.local/bin:$PATH"
export MB_CTX="$(mega-brain context-md 2>/dev/null)"
# alcatraz-helper JSON-escapes the context for us. It is a static binary rather
# than a script for the language runtime of the day: swapping the sandbox's
# stack (Node → Java, …) must not silently break the memory hooks.
exec "${ALCATRAZ_HELPER:-alcatraz-helper}" hook-session-start --event-name SessionStart
