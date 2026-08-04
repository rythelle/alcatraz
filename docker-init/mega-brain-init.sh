#!/bin/bash
# Mega Brain - unified init, run at container boot.
# Injects the SessionStart/SessionEnd hooks (auto-load + auto-save) into each CLI's
# config and disables native memory where configurable. Idempotent.
set -u

# Module gate: when Mega Brain is off, don't wire any hooks. (The hooks also
# self-guard on this flag, so a persisted-volume install where they were wired
# previously still no-ops.)
case "${ALCATRAZ_MOD_MEGABRAIN:-on}" in
    off|0|false|no|disabled|OFF|FALSE|NO)
        echo "[mega-brain-init] module off (ALCATRAZ_MOD_MEGABRAIN) — hooks not wired"
        exit 0 ;;
esac

HOME_DIR="/home/alcatraz_runner"
BIN="$HOME_DIR/.local/bin"
H_START_CC="$BIN/mb-hook-start-cc"
H_START_GEMINI="$BIN/mb-hook-start-gemini"
H_END="$BIN/mb-hook-end"
H_PRECOMPACT="$BIN/mb-hook-precompact"

# The settings merges run through alcatraz-helper, a static binary shipped with
# the image. It deliberately does not depend on the sandbox's language runtime:
# swapping that stack (Node → Java, …) must not break Mega Brain's wiring.
HELPER="${ALCATRAZ_HELPER:-alcatraz-helper}"
if ! command -v "$HELPER" >/dev/null 2>&1; then
    echo "[mega-brain-init] alcatraz-helper not found — hooks not wired (rebuild the image)"
    exit 0
fi

# Claude Code: merge hooks into settings.json, keeping projects['/workspace'].
CLAUDE_DIR="$HOME_DIR/.claude"
mkdir -p "$CLAUDE_DIR" 2>/dev/null || true
"$HELPER" settings-claude "$CLAUDE_DIR/settings.json" "$H_START_CC" "$H_END" "$H_PRECOMPACT" \
    || echo "[mega-brain-init] claude settings skipped"

# Gemini CLI: hooks + excludeTools (disables save_memory). ~/.gemini is isolated from host.
GEMINI_DIR="$HOME_DIR/.gemini"
mkdir -p "$GEMINI_DIR" 2>/dev/null || true
"$HELPER" settings-gemini "$GEMINI_DIR/settings.json" "$H_START_GEMINI" "$H_END" \
    || echo "[mega-brain-init] gemini settings skipped"

# Codex: inline hooks in config.toml (SessionStart matcher startup|resume + Stop). Idempotent append.
CODEX_DIR="$HOME_DIR/.codex"
CODEX_CFG="$CODEX_DIR/config.toml"
mkdir -p "$CODEX_DIR" 2>/dev/null || true
if ! grep -q "mega-brain hooks" "$CODEX_CFG" 2>/dev/null; then
    cat >> "$CODEX_CFG" << EOF

# mega-brain hooks (auto-load + auto-save)
[[hooks.SessionStart]]
matcher = "startup|resume"
[[hooks.SessionStart.hooks]]
type = "command"
command = "$H_START_CC"

[[hooks.Stop]]
[[hooks.Stop.hooks]]
type = "command"
command = "$H_END codex"
EOF
    echo "[mega-brain-init] codex config.toml updated"
fi

# opencode uses the plugin mounted at ~/.config/opencode/plugin/ - no init needed.
echo "[mega-brain-init] done"
