#!/bin/bash
# Mega Brain - unified init, run at container boot.
# Injects the SessionStart/SessionEnd hooks (auto-load + auto-save) into each CLI's
# config and disables native memory where configurable. Idempotent.
set -u

HOME_DIR="/home/alcatraz_runner"
BIN="$HOME_DIR/.local/bin"
H_START_CC="$BIN/mb-hook-start-cc"
H_START_GEMINI="$BIN/mb-hook-start-gemini"
H_END="$BIN/mb-hook-end"

# Claude Code: merge hooks into settings.json, keeping projects['/workspace'].
CLAUDE_DIR="$HOME_DIR/.claude"
mkdir -p "$CLAUDE_DIR" 2>/dev/null || true
python3 - "$CLAUDE_DIR/settings.json" "$H_START_CC" "$H_END" << 'PY' || echo "[mega-brain-init] claude settings skipped"
import json, sys
path, h_start, h_end = sys.argv[1], sys.argv[2], sys.argv[3]
try:
    with open(path) as f: data = json.load(f)
except Exception:
    data = {}

data.setdefault("projects", {})
data["projects"]["/workspace"] = {
    "allowedTools": ["Read", "Glob", "Grep", "Bash", "WebFetch", "StrReplaceBasedEditTool", "Write"],
    "mcpServers": {}, "enabledMcpjsonServers": [], "disabledMcpjsonServers": [],
    "hasTrustDialogAccepted": True, "projectOnboardingSeenCount": 0,
    "hasCompletedProjectOnboarding": True,
}
data.setdefault("hooks", {})
data["hooks"]["SessionStart"] = [{"hooks": [{"type": "command", "command": h_start}]}]
data["hooks"]["SessionEnd"]   = [{"hooks": [{"type": "command", "command": h_end + " claude"}]}]

with open(path, "w") as f: json.dump(data, f, indent=2)
print("[mega-brain-init] claude settings.json updated")
PY

# Gemini CLI: hooks + excludeTools (disables save_memory). ~/.gemini is isolated from host.
GEMINI_DIR="$HOME_DIR/.gemini"
mkdir -p "$GEMINI_DIR" 2>/dev/null || true
python3 - "$GEMINI_DIR/settings.json" "$H_START_GEMINI" "$H_END" << 'PY' || echo "[mega-brain-init] gemini settings skipped"
import json, sys
path, h_start, h_end = sys.argv[1], sys.argv[2], sys.argv[3]
try:
    with open(path) as f: data = json.load(f)
except Exception:
    data = {}

data.setdefault("hooks", {})
data["hooks"]["SessionStart"] = [{"matcher": "*", "hooks": [{"type": "command", "command": h_start}]}]
data["hooks"]["SessionEnd"]   = [{"matcher": "*", "hooks": [{"type": "command", "command": h_end + " gemini"}]}]

excl = set(data.get("excludeTools", []))
excl.add("save_memory")
data["excludeTools"] = sorted(excl)

with open(path, "w") as f: json.dump(data, f, indent=2)
print("[mega-brain-init] gemini settings.json updated (excludeTools: save_memory)")
PY

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
