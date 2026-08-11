#!/bin/bash
# Codex skills - link the host's user skills into ~/.codex/skills at boot.
#
# ~/.codex itself is a named volume (Codex owns .codex/skills/.system and
# refreshes it against a version marker, so it cannot be a read-only mount).
# The host skills therefore arrive on a separate read-only mount and are
# symlinked in here - whatever the host has at boot, no per-skill wiring.
# Read-only for the same reason as ~/.claude/skills: a prompt-injected agent
# must not be able to edit a skill the HOST would later execute.
set -u

HOME_DIR="/home/alcatraz_runner"
SRC="$HOME_DIR/.codex-skills-host"
DST="$HOME_DIR/.codex/skills"

[ -d "$SRC" ] || exit 0
mkdir -p "$DST" 2>/dev/null || true

# Drop links whose host skill is gone (deleted/renamed since the last boot).
for link in "$DST"/*; do
    [ -L "$link" ] || continue
    case "$(readlink "$link")" in
        "$SRC"/*) [ -e "$link" ] || rm -f "$link" ;;
    esac
done

linked=0
# The glob skips dotfiles, so .system on the host is never mirrored - and the
# container's own .system, a real directory, is never replaced by a link.
for skill in "$SRC"/*/; do
    [ -d "$skill" ] || continue
    name="${skill%/}"; name="${name##*/}"
    if [ ! -f "$skill/SKILL.md" ]; then
        echo "[codex-skills-init] skipped $name (no SKILL.md)"
        continue
    fi
    if [ -e "$DST/$name" ] && [ ! -L "$DST/$name" ]; then
        echo "[codex-skills-init] skipped $name (already a real dir in the container)"
        continue
    fi
    ln -sfn "${skill%/}" "$DST/$name" && linked=$((linked + 1))
done

echo "[codex-skills-init] linked $linked host skill(s)"
