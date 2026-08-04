#!/bin/bash
# Mega Brain PreCompact adapter (Claude Code only - the other CLIs have no
# pre-compaction hook). Before Claude compresses the context window, extract a
# digest of the recent conversation from the transcript and snapshot it into
# the vault, so late-session decisions survive even without a handoff.
# Skip when this is a disposable spawn OR the Mega Brain module is off.
case "${ALCATRAZ_MOD_MEGABRAIN:-on}" in off|0|false|no|disabled|OFF|FALSE|NO) MB_OFF=1;; esac
if [ -n "${ALCATRAZ_SPAWN:-}" ] || [ -n "${MB_OFF:-}" ]; then echo '{}'; exit 0; fi
export PATH=/home/alcatraz_runner/.local/bin:$PATH

payload=$(cat)

# The digest is built by alcatraz-helper — a static binary, so this keeps
# working whichever language runtime the sandbox ships. Everything here is
# best-effort: a malformed payload or transcript yields an empty digest rather
# than a failed compaction.
HELPER="${ALCATRAZ_HELPER:-alcatraz-helper}"
digest=$("$HELPER" precompact-digest "$payload" 2>/dev/null)
trigger=$("$HELPER" precompact-trigger "$payload" 2>/dev/null || echo auto)

mega-brain precompact "$trigger" "$digest" >/dev/null 2>&1 || true
echo '{}'
