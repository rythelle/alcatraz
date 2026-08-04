#!/bin/bash
# Mega Brain - persistent, dynamic, per-project memory.
# Usage: mega-brain <command> [args]   (alias: brain)

set -euo pipefail

# MEGABRAIN_CONTEXT_BASE is a test/override seam; in production this is the
# bind-mounted vault at the default path.
CONTEXT_BASE="${MEGABRAIN_CONTEXT_BASE:-/home/alcatraz_runner/.ai-context}"
DATE=$(date +%Y-%m-%d)

# Optional prefix-based grouping (e.g. group org repos under a subfolder).
# Set both to enable: repos whose name starts with GROUP_PREFIX go to CONTEXT_BASE/GROUP_DIR/<name>.
GROUP_PREFIX="${MEGABRAIN_GROUP_PREFIX:-}"
GROUP_DIR="${MEGABRAIN_GROUP_DIR:-}"

# Resolve the real project name when the working tree sits directly at
# /workspace (basename would just be "workspace"). Host-provided name first,
# then the git remote repo name. Only returns the bare "workspace" placeholder
# when there is genuinely no other signal — that bucket merges unrelated
# projects, so we avoid it whenever possible.
resolve_workspace_name() {
    if [ -n "${ALCATRAZ_PROJECT_NAME:-}" ]; then
        echo "$ALCATRAZ_PROJECT_NAME"
        return
    fi
    local remote
    remote=$(git -C /workspace config --get remote.origin.url 2>/dev/null || true)
    if [ -n "$remote" ]; then
        basename "$remote" .git
        return
    fi
    echo "workspace"
}

detect_project() {
    local root name cwd
    # MB_DETECT_CWD is a test seam; in production this is just the real cwd.
    cwd="${MB_DETECT_CWD:-$(pwd 2>/dev/null)}" || cwd="/workspace"

    # Multi-project layout: every project is mounted at /workspace/projects/<name>.
    # Derive the bucket straight from the path so sibling projects never share
    # one directory, no matter the git state or whether ALCATRAZ_PROJECT_NAME is
    # set. This is what previously collapsed everything into "workspace".
    case "$cwd/" in
        /workspace/projects/*/)
            name="${cwd#/workspace/projects/}"
            name="${name%%/*}"
            if [ -n "$name" ]; then
                echo "$name"
                return
            fi
            ;;
    esac

    # Single-project layout: use the git toplevel of the CURRENT directory
    # (not a hardcoded /workspace) so detection follows where work is happening.
    root=$(git -C "$cwd" rev-parse --show-toplevel 2>/dev/null) || root=""
    if [ -n "$root" ]; then
        name=$(basename "$root")
        # A repo whose root IS /workspace reports basename "workspace"; resolve
        # the real name from the host or the git remote instead.
        if [ "$name" = "workspace" ]; then
            name="$(resolve_workspace_name)"
        fi
        echo "$name"
        return
    fi

    # Not a git repository anywhere above cwd: fall back to the host name.
    resolve_workspace_name
}

get_context_path() {
    local project="${1:-$(detect_project)}"
    if [ -n "$GROUP_PREFIX" ] && [ -n "$GROUP_DIR" ] && [[ "$project" == "$GROUP_PREFIX"* ]]; then
        echo "$CONTEXT_BASE/$GROUP_DIR/$project"
    else
        echo "$CONTEXT_BASE/$project"
    fi
}

get_global_path() {
    echo "$CONTEXT_BASE/_global"
}

ensure_structure() {
    local path="$1"
    mkdir -p \
        "$path/Context" \
        "$path/Memory/patterns" \
        "$path/Memory/decisions" \
        "$path/Memory/gotchas" \
        "$path/Memory/notes" \
        "$path/Tasks/active" \
        "$path/Tasks/done" \
        "$path/Tasks/backlog" \
        "$path/Logs"
    migrate_legacy_memory "$path"
}

# Older versions saved memories in singular dirs (Memory/pattern/...) that the
# loaders never read. Move any leftovers into the canonical plural dirs.
migrate_legacy_memory() {
    local path="$1" t
    for t in pattern decision gotcha note; do
        if [ -d "$path/Memory/$t" ]; then
            find "$path/Memory/$t" -maxdepth 1 -name "*.md" \
                -exec mv -n {} "$path/Memory/${t}s/" \; 2>/dev/null || true
            rmdir "$path/Memory/$t" 2>/dev/null || true
        fi
    done
}

type_dir() {
    echo "Memory/${1}s"
}

ensure_global_structure() {
    local GLOBAL_DIR
    GLOBAL_DIR=$(get_global_path)
    mkdir -p "$GLOBAL_DIR/preferences" "$GLOBAL_DIR/Memory"
    if [ ! -f "$GLOBAL_DIR/INDEX.md" ]; then
        cat > "$GLOBAL_DIR/INDEX.md" << EOF
# Mega Brain - Global

**Created:** $DATE

User preferences and global learnings (valid across all projects).
Loaded automatically alongside any project's context.
EOF
    fi
}

count_files() {
    local dir="$1"
    [ -d "$dir" ] && find "$dir" -maxdepth 1 -name "*.md" | wc -l | tr -d ' ' || echo 0
}

to_kebab() {
    echo "$*" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g; s/--*/-/g; s/^-//; s/-$//'
}

# Silently create the project skeleton (dynamic auto-init).
auto_init_project() {
    local path="$1" project="$2"
    ensure_structure "$path"
    if [ ! -f "$path/INDEX.md" ]; then
        cat > "$path/INDEX.md" << EOF
# $project

**Created:** $DATE
**Repository:** $project

## Navigation
- Current task -> [[Context/current-task]]
- Architecture -> [[Context/architecture]]
- Stack -> [[Context/stack]]
- Timeline -> [[Logs/timeline]]

## Memory
- Patterns: 0 files
- Decisions: 0 files
- Gotchas: 0 files
- Notes: 0 files
EOF
    fi
    [ -f "$path/Logs/timeline.md" ] || printf '# Timeline - %s\n\n' "$project" > "$path/Logs/timeline.md"
}

# Emit the global preferences block as markdown (no banner). Empty if none.
emit_global_md() {
    local GLOBAL_DIR
    GLOBAL_DIR=$(get_global_path)
    [ -d "$GLOBAL_DIR/preferences" ] || return 0
    local files
    files=$(find "$GLOBAL_DIR/preferences" -maxdepth 1 -name "*.md" 2>/dev/null | sort)
    [ -z "$files" ] && return 0
    echo "## User preferences (global)"
    echo ""
    while IFS= read -r f; do
        [ -f "$f" ] && cat "$f" && echo ""
    done <<< "$files"
}

# First meaningful line of a memory file (the line after "## Description").
memory_desc() {
    awk '/^## Description/{f=1;next} f&&NF{print;exit}' "$1" 2>/dev/null | cut -c1-120
}

# One line per memory (link + short description). The model loads the full
# file on demand instead of having every memory injected into context.
emit_memory_index() {
    local path="$1" dir f slug desc
    for dir in patterns decisions gotchas notes; do
        [ -d "$path/Memory/$dir" ] || continue
        while IFS= read -r f; do
            [ -f "$f" ] || continue
            slug=$(basename "$f" .md)
            desc=$(memory_desc "$f")
            echo "- [[Memory/$dir/$slug]]${desc:+ - $desc}"
        done < <(find "$path/Memory/$dir" -maxdepth 1 -name "*.md" 2>/dev/null | sort)
    done
}

cmd_load() {
    local project path
    project=$(detect_project)
    path=$(get_context_path "$project")

    ensure_global_structure
    [ -d "$path" ] || auto_init_project "$path" "$project"

    echo "========================================"
    echo " MEGA BRAIN - $project"
    echo "========================================"
    echo ""

    local global_md
    global_md=$(emit_global_md)
    if [ -n "$global_md" ]; then
        echo "-- GLOBAL PREFERENCES -------------------"
        echo "$global_md"
        echo ""
    fi

    if [ -f "$path/Context/last-session.md" ]; then
        echo "-- LAST SESSION HANDOFF -----------------"
        cat "$path/Context/last-session.md"
        echo ""
    fi

    if [ -f "$path/Context/current-task.md" ]; then
        echo "-- CURRENT TASK -------------------------"
        cat "$path/Context/current-task.md"
        echo ""
    fi

    if [ -f "$path/Logs/timeline.md" ]; then
        echo "-- RECENT HISTORY (last sessions) -------"
        grep -A 5 "^## " "$path/Logs/timeline.md" | tail -50
        echo ""
    fi

    echo "-- MEMORY INDEX (cat the file on demand) "
    local mem_index
    mem_index=$(emit_memory_index "$path")
    echo "${mem_index:-"(empty)"}"
    echo ""

    if [ -f "$path/Context/current-task.md" ]; then
        local linked
        linked=$(grep -oE '\[\[Memory/[^]]+\]\]' "$path/Context/current-task.md" \
                 | sed 's/\[\[//;s/\]\]//' || true)
        if [ -n "$linked" ]; then
            echo "-- MEMORY RELEVANT TO CURRENT TASK ------"
            while IFS= read -r rel_path; do
                local full="$path/$rel_path.md"
                [ -f "$full" ] && echo "--- $rel_path ---" && cat "$full" && echo ""
            done <<< "$linked"
        fi
    fi

    echo "========================================"
    echo "Commands: mega-brain task <name> | mega-brain remember <type> <name> | mega-brain search <term> | mega-brain done | mega-brain handoff <summary>"
}

# Plain-markdown context (payload for SessionStart hooks). No banners.
cmd_context_md() {
    local project path
    project=$(detect_project)
    path=$(get_context_path "$project")

    ensure_global_structure
    [ -d "$path" ] || auto_init_project "$path" "$project"

    echo "# Mega Brain - context loaded automatically ($project)"
    echo ""
    echo "This context was injected by Mega Brain. Internalize it before responding."
    echo "Save learnings/preferences with \`mega-brain remember ...\` and finish tasks"
    echo "with \`mega-brain done ...\` without being asked. Do not use the model's native memory."
    echo ""

    local global_md
    global_md=$(emit_global_md)
    [ -n "$global_md" ] && echo "$global_md" && echo ""

    if [ -f "$path/Context/last-session.md" ]; then
        echo "## Last session handoff"
        echo ""
        cat "$path/Context/last-session.md"
        echo ""
    fi

    if [ -f "$path/Context/current-task.md" ]; then
        echo "## Current task"
        echo ""
        cat "$path/Context/current-task.md"
        echo ""
    fi

    if [ -f "$path/Logs/timeline.md" ]; then
        echo "## Recent history"
        echo ""
        grep -A 5 "^## " "$path/Logs/timeline.md" | tail -30
        echo ""
    fi

    echo "## Project memory index"
    echo ""
    echo "Only titles are loaded. Read a memory on demand with"
    echo "\`cat \"$path/<link>.md\"\` or find one with \`mega-brain search <term>\`."
    echo ""
    local mem_index
    mem_index=$(emit_memory_index "$path")
    echo "${mem_index:-"(no memories yet)"}"
}

cmd_init() {
    local project path
    project=$(detect_project)
    path=$(get_context_path "$project")
    ensure_global_structure
    auto_init_project "$path" "$project"

    echo "Initialized mega brain for: $project"
    echo "Path: $path"
}

cmd_task() {
    local name="${1:-}"
    [ -z "$name" ] && echo "Usage: mega-brain task <name>" && exit 1

    local project path slug task_file
    project=$(detect_project)
    path=$(get_context_path "$project")
    slug=$(to_kebab "$name")
    task_file="$path/Tasks/active/$slug.md"

    ensure_structure "$path"

    if [ ! -f "$task_file" ]; then
        cat > "$task_file" << EOF
# $name

**Status:** 0%
**Started:** $DATE

## Goal
(describe the goal of this task)

## Subtasks
- [ ]

## Notes
EOF
        echo "Task created: Tasks/active/$slug.md"
    else
        echo "Existing task loaded: Tasks/active/$slug.md"
    fi

    cat > "$path/Context/current-task.md" << EOF
# Task: $name

**Status:** 0%
**Started:** $DATE
**File:** [[Tasks/active/$slug]]

## Goal
(fill in the goal)

## Relevant context
### Patterns
(add [[Memory/patterns/name]] links as relevant)

### Decisions
(add [[Memory/decisions/name]] links as relevant)

### Gotchas
(add [[Memory/gotchas/name]] links as relevant)

## Progress
- [ ]
EOF

    echo ""
    echo "Active task: $name"
    echo "File: $task_file"
    echo ""

    local keywords found=0
    keywords=$(echo "$name" | tr '-' ' ')
    for dir in patterns decisions gotchas notes; do
        if [ -d "$path/Memory/$dir" ]; then
            while IFS= read -r f; do
                local fname
                fname=$(basename "$f" .md)
                if echo "$fname $keywords" | tr ' ' '\n' | sort | uniq -d | grep -q .; then
                    [ $found -eq 0 ] && echo "-- Possibly relevant memory --------------"
                    echo "  [$dir] $fname"
                    found=1
                fi
            done < <(find "$path/Memory/$dir" -name "*.md" 2>/dev/null)
        fi
    done
    [ $found -eq 1 ] && echo ""

    cat "$task_file"
}

cmd_remember() {
    local type="${1:-}" name="${2:-}" content="${3:-}"
    if [ -z "$type" ] || [ -z "$name" ]; then
        echo "Usage: mega-brain remember <pattern|decision|gotcha|note|preference> <name> [content]"
        exit 1
    fi

    local project path slug file dir is_global=0
    project=$(detect_project)
    slug=$(to_kebab "$name")

    case "$type" in
        pattern|decision|gotcha|note) ;;
        preference) is_global=1 ;;
        *) echo "Invalid type: $type (use: pattern, decision, gotcha, note, preference)" && exit 1 ;;
    esac

    if [ "$is_global" -eq 1 ]; then
        ensure_global_structure
        path=$(get_global_path)
        dir="preferences"
        file="$path/preferences/$slug.md"
    else
        path=$(get_context_path "$project")
        ensure_structure "$path"
        dir=$(type_dir "$type")
        file="$path/$dir/$slug.md"
    fi
    mkdir -p "$(dirname "$file")"

    if [ ! -f "$file" ]; then
        cat > "$file" << EOF
# $name

**Type:** $type
**Date:** $DATE
**Project:** $([ "$is_global" -eq 1 ] && echo "(global)" || echo "$project")

## Description
${content:-"(describe here)"}

## When to apply
(usage context)

## References
EOF
        if [ "$is_global" -eq 0 ] && [ -f "$path/Context/current-task.md" ]; then
            local current_task
            current_task=$(grep "^\*\*File:\*\*" "$path/Context/current-task.md" \
                           | sed 's/.*\[\[//;s/\]\].*//' || true)
            [ -n "$current_task" ] && echo "- Discovered in: [[$current_task]]" >> "$file"
        fi

        if [ "$is_global" -eq 0 ] && [ -f "$path/INDEX.md" ]; then
            local count cap
            count=$(count_files "$path/$dir")
            cap="${type^}s"
            if grep -q "^- $cap" "$path/INDEX.md"; then
                sed -i "s/^- $cap.*/- $cap: $count files/" "$path/INDEX.md" 2>/dev/null || true
            else
                sed -i "/^## Memory/a - $cap: $count files" "$path/INDEX.md" 2>/dev/null || true
            fi
        fi

        echo "Saved: $dir/$slug.md"
        echo "  Path: $file"
    elif [ -n "$content" ]; then
        # Memory already exists: append a dated update instead of refusing,
        # so knowledge can evolve without manual file editing.
        printf '\n## Update %s\n\n%s\n' "$DATE" "$content" >> "$file"
        echo "Updated: $dir/$slug.md (appended '## Update $DATE')"
    else
        echo "Already exists: $file"
    fi

    echo ""
    cat "$file"
}

cmd_done() {
    local learnings="${1:-}"
    local project path
    project=$(detect_project)
    path=$(get_context_path "$project")

    if [ ! -f "$path/Context/current-task.md" ]; then
        echo "No active task found."
        exit 1
    fi

    local task_name slug
    task_name=$(grep "^# Task:" "$path/Context/current-task.md" | sed 's/# Task: //')
    slug=$(to_kebab "$task_name")

    if [ -f "$path/Tasks/active/$slug.md" ]; then
        sed -i "s/\*\*Status:\*\* .*/\*\*Status:\*\* 100%/" "$path/Tasks/active/$slug.md"
        echo "**Completed:** $DATE" >> "$path/Tasks/active/$slug.md"
        mv "$path/Tasks/active/$slug.md" "$path/Tasks/done/$slug.md"
        echo "Task moved to: Tasks/done/$slug.md"
    fi

    {
        echo ""
        echo "## $DATE - done: $task_name"
        [ -n "$learnings" ] && echo "**Learnings:** $learnings"
        echo ""
    } >> "$path/Logs/timeline.md"
    echo "Timeline updated"

    if [ -n "$learnings" ]; then
        IFS=';' read -ra items <<< "$learnings"
        for item in "${items[@]}"; do
            item=$(echo "$item" | sed 's/^[[:space:]]*//')
            [ -n "$item" ] && cmd_remember "note" "$item" "$item"
        done
    fi

    cat > "$path/Context/current-task.md" << EOF
# No active task

Last completed: $task_name ($DATE)

Use: mega-brain task <name>
EOF

    echo ""
    echo "Task '$task_name' completed."

    local next
    next=$(find "$path/Tasks/backlog" -name "*.md" 2>/dev/null | head -1)
    if [ -n "$next" ]; then
        echo "Next task in backlog: $(basename "$next" .md)"
    fi
}

# On-demand recall: grep the vault (project + global) so the model can find
# a memory without having everything injected into context.
cmd_search() {
    local term="${1:-}"
    [ -z "$term" ] && echo "Usage: mega-brain search <term>" && exit 1

    local project path GLOBAL_DIR base found=0
    project=$(detect_project)
    path=$(get_context_path "$project")
    GLOBAL_DIR=$(get_global_path)

    echo "=== Search: '$term' ($project + global) ==="
    for base in "$path" "$GLOBAL_DIR"; do
        [ -d "$base" ] || continue
        while IFS= read -r line; do
            echo "$line"
            found=1
        done < <(grep -rniI --include='*.md' -m 3 -- "$term" "$base" 2>/dev/null \
                 | sed "s|^$CONTEXT_BASE/||" | cut -c1-200 | head -40)
    done
    [ "$found" -eq 0 ] && echo "(no results)"
    return 0
}

# End-of-session protocol: persist what changed, decisions made and next steps.
# Overwrites Context/last-session.md (injected at the next session start) and
# appends to the timeline (permanent history).
cmd_handoff() {
    local summary="${1:-}"
    [ -z "$summary" ] && echo "Usage: mega-brain handoff \"what changed; decisions; next steps\"" && exit 1

    local project path
    project=$(detect_project)
    path=$(get_context_path "$project")
    ensure_global_structure
    [ -d "$path" ] || auto_init_project "$path" "$project"

    cat > "$path/Context/last-session.md" << EOF
# Last session - $DATE

$summary
EOF

    {
        echo ""
        echo "## $DATE - handoff"
        echo "$summary"
    } >> "$path/Logs/timeline.md"

    echo "Handoff saved: Context/last-session.md (loaded automatically next session)"
}

# Pre-compaction backstop (Claude Code PreCompact hook): snapshot a digest of
# the recent conversation before the context window is compressed, so work from
# late in a long session survives even if it never reaches a proper handoff.
# Writes the same Context/last-session.md that handoff uses (a later real
# handoff simply overwrites it).
cmd_precompact() {
    local trigger="${1:-auto}"
    local digest="${2:-}"

    local project path
    project=$(detect_project)
    path=$(get_context_path "$project")
    ensure_global_structure
    [ -d "$path" ] || auto_init_project "$path" "$project"

    {
        echo "# Last session - $DATE (pre-compaction snapshot, trigger: $trigger)"
        echo ""
        if [ -n "$digest" ]; then
            echo "Recent conversation before the context was compacted:"
            echo ""
            echo "$digest"
        else
            echo "Context was compacted mid-session; no digest captured."
        fi
    } > "$path/Context/last-session.md"

    printf '\n## %s - context compacted (%s)\n' "$DATE" "$trigger" >> "$path/Logs/timeline.md"

    echo "Pre-compaction snapshot saved: Context/last-session.md"
}

# Pause an in-progress task mid-flight, keeping ALL context so it can be resumed
# later WITHOUT the model's native --resume. Unlike `done`, the task stays open;
# unlike `handoff`, it records an explicit PAUSED state and what was in progress.
# Writes Context/last-session.md (auto-injected at the next SessionStart), so the
# next session picks up exactly where this one stopped.
cmd_pause() {
    local summary="${1:-}"
    local project path
    project=$(detect_project)
    path=$(get_context_path "$project")
    ensure_global_structure
    [ -d "$path" ] || auto_init_project "$path" "$project"

    local task_name="" task_slug="" task_file=""
    if [ -f "$path/Context/current-task.md" ]; then
        task_name=$(grep "^# Task:" "$path/Context/current-task.md" | sed 's/# Task: //')
        task_slug=$(to_kebab "$task_name")
        task_file="$path/Tasks/active/$task_slug.md"
    fi

    # Stamp the active task as paused and record the in-progress note in it.
    if [ -n "$task_file" ] && [ -f "$task_file" ]; then
        sed -i "s/\*\*Status:\*\* .*/\*\*Status:\*\* PAUSED ($DATE)/" "$task_file"
        {
            echo ""
            echo "## Paused $DATE"
            echo "${summary:-"(paused with no summary)"}"
        } >> "$task_file"
    fi

    {
        echo "# Paused session - $DATE"
        echo ""
        echo "**Status:** PAUSED - resume with \`mega-brain resume\`"
        [ -n "$task_name" ] && echo "**Task in progress:** $task_name ([[Tasks/active/$task_slug]])"
        echo ""
        if [ -n "$summary" ]; then
            echo "## What was in progress"
            echo ""
            echo "$summary"
        else
            echo "Paused with no summary - see the active task for the latest progress."
        fi
    } > "$path/Context/last-session.md"

    printf '\n## %s - paused%s\n%s\n' "$DATE" "${task_name:+: $task_name}" "$summary" >> "$path/Logs/timeline.md"

    echo "Paused. Context saved to Context/last-session.md (auto-loaded next session)."
    echo "Resume anytime with: mega-brain resume"
}

# Resume a paused task: print the saved context and clear the PAUSED marker so
# work continues from where it stopped - no native --resume needed.
cmd_resume() {
    local project path
    project=$(detect_project)
    path=$(get_context_path "$project")

    if [ ! -f "$path/Context/last-session.md" ]; then
        echo "Nothing to resume for '$project' (no Context/last-session.md)."
        exit 0
    fi

    echo "=== Resuming: $project ==="
    echo ""
    cat "$path/Context/last-session.md"
    echo ""

    if [ -f "$path/Context/current-task.md" ]; then
        local task_name task_slug task_file
        task_name=$(grep "^# Task:" "$path/Context/current-task.md" | sed 's/# Task: //')
        task_slug=$(to_kebab "$task_name")
        task_file="$path/Tasks/active/$task_slug.md"
        if [ -n "$task_name" ] && [ -f "$task_file" ]; then
            sed -i "s/\*\*Status:\*\* PAUSED.*/\*\*Status:\*\* in progress (resumed $DATE)/" "$task_file"
            echo "-- Active task ---------------------------"
            cat "$task_file"
        fi
    fi

    printf '\n## %s - resumed\n' "$DATE" >> "$path/Logs/timeline.md"
}

# Snapshot EVERY mounted project before the containers go down (best-effort file
# snapshot per project). The host restart flow calls this so in-progress work in
# any open shell survives a container restart. Iterates the multi-project layout
# (/workspace/projects/<name>); falls back to the current project otherwise.
cmd_pause_all() {
    local summary="${1:-auto: container restart}"
    local d found=0
    for d in /workspace/projects/*/; do
        [ -d "$d" ] || continue
        found=1
        ( cd "$d" 2>/dev/null && cmd_pause "$summary" >/dev/null 2>&1 ) || true
        echo "  paused: $(basename "$d")"
    done
    if [ "$found" -eq 0 ]; then
        cmd_pause "$summary" >/dev/null 2>&1 || true
        echo "  paused: $(detect_project)"
    fi
    echo "All mounted projects paused."
}

cmd_context() {
    local project path
    project=$(detect_project)
    path=$(get_context_path "$project")

    echo "=== Mega Brain: $project ==="
    echo "Path: $path"

    if [ ! -d "$path" ]; then
        echo "Status: new project - context is created automatically when an AI session starts."
        exit 0
    fi

    [ -f "$path/Context/current-task.md" ] \
        && echo "-- Current task --" \
        && head -6 "$path/Context/current-task.md"

    [ -f "$path/Logs/timeline.md" ] \
        && echo "-- Last sessions --" \
        && grep "^## " "$path/Logs/timeline.md" | tail -3

    echo "Patterns: $(count_files "$path/Memory/patterns") | Decisions: $(count_files "$path/Memory/decisions") | Gotchas: $(count_files "$path/Memory/gotchas")"
}

# Deterministic auto-save backstop: record session end in the timeline.
cmd_hook_session_end() {
    local model="${1:-?}"
    local project path
    project=$(detect_project)
    path=$(get_context_path "$project")

    ensure_global_structure
    [ -d "$path" ] || auto_init_project "$path" "$project"

    printf '\n## %s - session ended (%s)\n\n' "$DATE" "$model" >> "$path/Logs/timeline.md"
}

# Supervisor: run as the container's PID 1 so Mega Brain snapshots survive both
# a graceful stop (SIGTERM trap -> pause-all) and a sudden death (OOM/kill) via
# a periodic autosave that loses at most MEGABRAIN_AUTOSAVE_SECS of work
# (default 300s; set 0 to disable). Replaces a bare `exec bash` as the keepalive
# process, so a snapshot happens even when the host never asks for one.
cmd_supervise() {
    local interval="${MEGABRAIN_AUTOSAVE_SECS:-300}"
    case "$interval" in ''|*[!0-9]*) interval=0 ;; esac

    _mb_bg=""
    _mb_on_stop() {
        trap - TERM INT
        [ -n "${_mb_bg:-}" ] && kill "$_mb_bg" 2>/dev/null || true
        cmd_pause_all "auto: container stop" >/dev/null 2>&1 || true
        exit 0
    }
    trap _mb_on_stop TERM INT

    if [ "$interval" -gt 0 ]; then
        (
            while true; do
                sleep "$interval" || true
                cmd_pause_all "auto: periodic autosave" >/dev/null 2>&1 || true
            done
        ) &
        _mb_bg=$!
    fi

    # Block as PID 1 until a stop signal arrives; `wait`/`sleep` are
    # interruptible, so the trap fires promptly. With autosave disabled there is
    # no child to wait on, so idle-sleep in a signal-interruptible loop instead.
    while true; do
        if [ -n "${_mb_bg:-}" ]; then
            wait "$_mb_bg" || true
        else
            sleep 3 || true
        fi
    done
}

case "${1:-help}" in
    load)             cmd_load ;;
    supervise)        cmd_supervise ;;
    context-md)       cmd_context_md ;;
    init)             cmd_init ;;
    task)             cmd_task "${2:-}" ;;
    remember)         cmd_remember "${2:-}" "${3:-}" "${4:-}" ;;
    done)             cmd_done "${2:-}" ;;
    search)           cmd_search "${2:-}" ;;
    handoff)          cmd_handoff "${2:-}" ;;
    pause)            cmd_pause "${2:-}" ;;
    resume)           cmd_resume ;;
    pause-all)        cmd_pause_all "${2:-}" ;;
    precompact)       cmd_precompact "${2:-}" "${3:-}" ;;
    context)          cmd_context ;;
    hook-session-end) cmd_hook_session_end "${2:-}" ;;
    path)             get_context_path "${2:-}" ;;
    global-path)      get_global_path ;;
    project)          detect_project ;;
    *)
        cat << 'EOF'
mega-brain - persistent, dynamic, per-project memory (alias: brain)

Context is loaded AUTOMATICALLY when Claude/Gemini/Codex/opencode start
(SessionStart hooks). You don't need to run load manually.

Commands:
  mega-brain load                          Load full context (global + project)
  mega-brain context-md                    Plain-markdown context (hook payload)
  mega-brain init                          Initialize the current project
  mega-brain task <name>                   Create/load a task and set it active
  mega-brain remember <type> <name> [txt]  Save memory (pattern/decision/gotcha/note/preference)
                                           If it already exists, appends a dated update
  mega-brain done [learnings]              Finish active task; learnings separated by ;
  mega-brain search <term>                 Grep the vault (project + global) on demand
  mega-brain handoff <summary>             End-of-session handoff, injected next session
  mega-brain pause [summary]               Pause the active task mid-flight, keeping full
                                           context; resume later without native --resume
  mega-brain resume                        Reload a paused session and clear the PAUSED mark
  mega-brain pause-all [summary]           Pause every mounted project (used before restart)
  mega-brain supervise                     Run as PID 1: SIGTERM trap + periodic autosave backstop
  mega-brain precompact <trigger> [digest] Pre-compaction snapshot (PreCompact hook backstop)
  mega-brain context                       Quick summary (shown when the container opens)
  mega-brain path                          Project path in the vault
  mega-brain global-path                   Global partition path (preferences)
  mega-brain project                       Detected project name

Memory types:
  pattern | decision | gotcha | note  -> current project's memory
  preference                          -> GLOBAL partition (applies to all projects)
EOF
    ;;
esac
