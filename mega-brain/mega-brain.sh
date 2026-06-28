#!/bin/bash
# Mega Brain - persistent, dynamic, per-project memory.
# Usage: mega-brain <command> [args]   (alias: brain)

set -euo pipefail

CONTEXT_BASE="/home/alcatraz_runner/.ai-context"
DATE=$(date +%Y-%m-%d)

# Optional prefix-based grouping (e.g. group org repos under a subfolder).
# Set both to enable: repos whose name starts with GROUP_PREFIX go to CONTEXT_BASE/GROUP_DIR/<name>.
GROUP_PREFIX="${MEGABRAIN_GROUP_PREFIX:-}"
GROUP_DIR="${MEGABRAIN_GROUP_DIR:-}"

detect_project() {
    local root
    root=$(git -C /workspace rev-parse --show-toplevel 2>/dev/null) || root=""
    if [ -n "$root" ]; then basename "$root"
    else basename /workspace
    fi
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
        "$path/Tasks/active" \
        "$path/Tasks/done" \
        "$path/Tasks/backlog" \
        "$path/Logs"
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
- Patterns : 0 files
- Decisions: 0 files
- Gotchas  : 0 files
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

    echo "-- MEMORY -------------------------------"
    echo "Patterns  : $(count_files "$path/Memory/patterns")"
    echo "Decisions : $(count_files "$path/Memory/decisions")"
    echo "Gotchas   : $(count_files "$path/Memory/gotchas")"
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
    echo "Commands: mega-brain task <name> | mega-brain remember <type> <name> | mega-brain done"
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

    echo "## Project memory"
    echo ""
    echo "- Patterns: $(count_files "$path/Memory/patterns")"
    echo "- Decisions: $(count_files "$path/Memory/decisions")"
    echo "- Gotchas: $(count_files "$path/Memory/gotchas")"
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
    for dir in patterns decisions gotchas; do
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
        dir="Memory/$type"
        file="$path/$dir/$slug.md"
    fi
    # ensure_structure creates the plural dirs; memory entries use the singular type
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
            local count
            count=$(count_files "$path/Memory/$type")
            sed -i "s/^- ${type^}s*:.*/- ${type^}s   : $count files/" "$path/INDEX.md" 2>/dev/null || true
        fi

        echo "Saved: $dir/$slug.md"
        echo "  Path: $file"
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

case "${1:-help}" in
    load)             cmd_load ;;
    context-md)       cmd_context_md ;;
    init)             cmd_init ;;
    task)             cmd_task "${2:-}" ;;
    remember)         cmd_remember "${2:-}" "${3:-}" "${4:-}" ;;
    done)             cmd_done "${2:-}" ;;
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
  mega-brain done [learnings]              Finish active task; learnings separated by ;
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
