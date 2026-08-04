#!/bin/bash

# Alcatraz Manager - control script for the secure sandbox
# Usage: ./alcatraz.sh [build|run|exec|clean] [command]

set -euo pipefail

# ===== CONFIG =====
DOCKER_COMPOSE_FILE="docker-compose.go.yml"
OVERRIDE_FILE="docker-compose.override.yml"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-300}"  # 5 min default
MAX_FILE_SIZE_MB=1000

# Directory of alcatraz.sh (base for the default path)
SCRIPT_DIR="$(cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")" && pwd)"

# State file persisting ALCATRAZ_WORKSPACE between invocations
STATE_FILE="$SCRIPT_DIR/.alcatraz-state"

# Detect Docker Compose V2 (plugin) or V1 (standalone)
# V1 (docker-compose) is buggy with Docker Engine 25+ - prefer V2
if docker compose version &>/dev/null 2>&1; then
    DC="docker compose"
elif command -v docker-compose &>/dev/null; then
    DC="docker-compose"
else
    echo "Docker Compose not found. Install with: sudo apt-get install docker-compose-plugin"
    exit 1
fi

# Output colors (ANSI-C quoting for real escape bytes)
RED=$'\033[0;31m'
GREEN=$'\033[0;32m'
YELLOW=$'\033[1;33m'
BLUE=$'\033[0;34m'
NC=$'\033[0m'

# ===== FUNCTIONS =====

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[✗]${NC} $1" >&2
}

check_docker() {
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed!"
        exit 1
    fi
    log_success "Docker detected ($DC)"
}

# Load ALCATRAZ_WORKSPACE from .env if present
load_env_workspace() {
    local env_file="$SCRIPT_DIR/.env"
    if [ -f "$env_file" ]; then
        local val
        val="$(grep '^ALCATRAZ_WORKSPACE=' "$env_file" | cut -d'=' -f2- | head -n1)"
        if [ -n "$val" ]; then
            if [[ "$val" != /* ]]; then
                val="$SCRIPT_DIR/$val"
            fi
            echo "$val"
        fi
    fi
}

# ===== MODULES =====
# Single resolution point (shared with the Go CLI's internal/modules). The core
# — sandbox + Lighthouse + Guard — is always on and never appears here.
# Precedence, highest first: process env var > .env line > built-in default.

# Default state per module: safety net on, opt-in off.
module_default() {
    case "$1" in
        checkpoints|sessions|stats) echo on ;;
        *)                          echo off ;;
    esac
}

# Normalize a raw toggle value to on/off, or empty if it isn't a boolean.
_norm_bool() {
    case "$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr -d '[:space:]')" in
        on|1|true|yes|enabled)    echo on ;;
        off|0|false|no|disabled)  echo off ;;
        *)                        echo "" ;;
    esac
}

# module_state <key> -> on|off, applying full precedence.
module_state() {
    local key="$1"
    local var="ALCATRAZ_MOD_$(printf '%s' "$key" | tr '[:lower:]' '[:upper:]')"
    local raw n
    # 1. process environment
    raw="${!var:-}"
    n="$(_norm_bool "$raw")"
    [ -n "$n" ] && { echo "$n"; return; }
    # 2. .env line (last wins; strip inline comment)
    if [ -f "$SCRIPT_DIR/.env" ]; then
        raw="$(grep "^${var}=" "$SCRIPT_DIR/.env" 2>/dev/null | tail -n1 | cut -d= -f2- | sed 's/#.*//')"
        n="$(_norm_bool "$raw")"
        [ -n "$n" ] && { echo "$n"; return; }
    fi
    # 3. default
    module_default "$key"
}

module_enabled() { [ "$(module_state "$1")" = on ]; }

module_off_notice() {
    local key="$1"
    local var="ALCATRAZ_MOD_$(printf '%s' "$key" | tr '[:lower:]' '[:upper:]')"
    log_warn "module '$key' is off — enable with ${var}=on (in .env or the TUI Modules screen)"
}

# Compute state once and export canonical ALCATRAZ_MOD_* so `docker compose up`
# (and thus the container's shims/hooks) see the exact values we resolved.
resolve_modules() {
    local key var
    for key in checkpoints sessions stats megabrain shakedown spawn websearch; do
        var="ALCATRAZ_MOD_$(printf '%s' "$key" | tr '[:lower:]' '[:upper:]')"
        export "$var=$(module_state "$key")"
    done
}

# Migration: installs predating modules get the default block injected once.
ensure_module_block() {
    local env_file="$SCRIPT_DIR/.env"
    [ -f "$env_file" ] || return 0
    grep -q '^ALCATRAZ_MOD_' "$env_file" && return 0
    {
        echo ""
        echo "# --- Modules (core is always ON and does not appear here) ---"
        echo "# Turn features on/off here or from the TUI Modules screen. An"
        echo "# ALCATRAZ_MOD_* set in the environment overrides the line below."
        echo "ALCATRAZ_MOD_CHECKPOINTS=on       # safety net (default on)"
        echo "ALCATRAZ_MOD_SESSIONS=on          # safety net (default on)"
        echo "ALCATRAZ_MOD_STATS=on             # safety net (default on)"
        echo "ALCATRAZ_MOD_MEGABRAIN=off        # opt-in"
        echo "ALCATRAZ_MOD_SHAKEDOWN=off        # opt-in"
        echo "ALCATRAZ_MOD_SPAWN=off            # opt-in"
        echo "ALCATRAZ_MOD_WEBSEARCH=off        # opt-in"
    } >> "$env_file"
    log_info "Module toggles added to .env — Mega Brain, shakedown and spawn are now opt-in (ALCATRAZ_MOD_<NAME>=on to re-enable)."
}

# Resolve and export ALCATRAZ_WORKSPACE from an optional path.
# Priority:
#   1. Passed argument
#   2. ALCATRAZ_WORKSPACE in .env
#   3. Last saved workspace (.alcatraz-state)
#   4. Default: ./project
resolve_workspace() {
    local path="${1:-}"

    if [ -n "$path" ]; then
        path="$(realpath "$path" 2>/dev/null || readlink -f "$path" 2>/dev/null || echo "$path")"
        if [ ! -d "$path" ]; then
            log_error "Directory does not exist: $path"
            exit 1
        fi
    else
        path="$(load_env_workspace)"
        if [ -z "$path" ]; then
            load_workspace &>/dev/null
            path="${ALCATRAZ_WORKSPACE:-$SCRIPT_DIR/project}"
        fi
        if [[ "$path" != /* ]]; then
            path="$SCRIPT_DIR/$path"
        fi
        mkdir -p "$path"
    fi

    export ALCATRAZ_WORKSPACE="$path"
    export ALCATRAZ_PROJECT_NAME="$(basename "$ALCATRAZ_WORKSPACE")"
    save_workspace
    log_success "Project: $ALCATRAZ_WORKSPACE -> /workspace"
}

save_workspace() {
    echo "ALCATRAZ_WORKSPACE=$ALCATRAZ_WORKSPACE" > "$STATE_FILE"
}

load_workspace() {
    if [ -f "$STATE_FILE" ]; then
        # shellcheck disable=SC1090
        source "$STATE_FILE"
    fi
    export ALCATRAZ_WORKSPACE="${ALCATRAZ_WORKSPACE:-$SCRIPT_DIR/project}"
    export ALCATRAZ_PROJECT_NAME="$(basename "$ALCATRAZ_WORKSPACE")"
}

# ===== FAVORITE WORKSPACES =====

WORKSPACES_FILE="$SCRIPT_DIR/.alcatraz-workspaces"

ensure_workspaces_file() {
    if [ ! -f "$WORKSPACES_FILE" ]; then
        touch "$WORKSPACES_FILE"
    fi
}

# Save a favorite workspace (alias -> absolute path)
save_workspace_alias() {
    local name="$1"
    local path="${2:-}"

    if [ -z "$name" ]; then
        log_error "Usage: ./alcatraz.sh save <name> [path]"
        return 1
    fi

    # Validate name: no spaces, = or #
    if [[ "$name" =~ [\ =#] ]]; then
        log_error "Invalid workspace name. Use only letters, numbers, hyphen and underscore."
        return 1
    fi

    if [ -n "$path" ]; then
        path="$(realpath "$path" 2>/dev/null || readlink -f "$path" 2>/dev/null || echo "$path")"
        if [ ! -d "$path" ]; then
            log_error "Directory does not exist: $path"
            return 1
        fi
    else
        load_workspace
        path="$ALCATRAZ_WORKSPACE"
    fi

    ensure_workspaces_file

    grep -v "^${name}=" "$WORKSPACES_FILE" > "$WORKSPACES_FILE.tmp" 2>/dev/null || true
    mv "$WORKSPACES_FILE.tmp" "$WORKSPACES_FILE"

    echo "${name}=${path}" >> "$WORKSPACES_FILE"
    log_success "Workspace '${name}' saved -> ${path}"
}

# Load a path from an alias. Empty if not found.
load_workspace_alias() {
    local name="$1"
    if [ -z "$name" ]; then
        return 1
    fi
    if [ -f "$WORKSPACES_FILE" ]; then
        # shellcheck disable=SC1090
        grep "^${name}=" "$WORKSPACES_FILE" | cut -d'=' -f2- | head -n1
    fi
}

# List all favorite workspaces
list_workspace_aliases() {
    ensure_workspaces_file
    if [ ! -s "$WORKSPACES_FILE" ]; then
        log_warn "No favorite workspaces saved."
        echo ""
        echo "  Use: ./alcatraz.sh save <name> [path]"
        return 0
    fi

    log_info "Favorite workspaces:"
    echo ""
    while IFS='=' read -r name path; do
        [ -z "$name" ] && continue
        local icon status_color
        if [ -d "$path" ]; then
            icon="${GREEN}✓${NC}"
            status_color="$NC"
        else
            icon="${YELLOW}⚠${NC}"
            status_color="$YELLOW"
        fi
        echo -e "  ${icon} $(printf '%-18s' "$name") ${status_color}${path}${NC}"
    done < "$WORKSPACES_FILE"
}

# Remove a favorite workspace
remove_workspace_alias() {
    local name="$1"
    if [ -z "$name" ]; then
        log_error "Usage: ./alcatraz.sh remove <name>"
        return 1
    fi

    if [ ! -f "$WORKSPACES_FILE" ] || ! grep -q "^${name}=" "$WORKSPACES_FILE"; then
        log_warn "Workspace '${name}' not found."
        return 1
    fi

    grep -v "^${name}=" "$WORKSPACES_FILE" > "$WORKSPACES_FILE.tmp"
    mv "$WORKSPACES_FILE.tmp" "$WORKSPACES_FILE"
    log_success "Workspace '${name}' removed."
}

# ===== WORKSPACE CHECKPOINTS =====
# Snapshots of the whole workspace stored on a shadow ref inside the project's
# own git repo. They never touch the user's branches, index or HEAD — so an AI
# session (any model) can be rolled back even after bash side effects, which
# Claude's /rewind can't do.

CHECKPOINT_REF="refs/alcatraz/checkpoints"

ws_git() {
    git -C "$ALCATRAZ_WORKSPACE" "$@"
}

# 0 = workspace is a git repo and git is available (messages on failure)
checkpoint_usable() {
    load_workspace
    if ! command -v git &>/dev/null; then
        log_warn "git not found on the host - checkpoints unavailable"
        return 1
    fi
    if ! ws_git rev-parse --git-dir &>/dev/null; then
        log_warn "Not a git repository: $ALCATRAZ_WORKSPACE (run 'git init' there to enable checkpoints)"
        return 1
    fi
}

# Snapshot the workspace (respecting .gitignore) into the shadow ref.
# Uses a temporary index so the user's staged changes are untouched.
create_checkpoint() {
    local msg="${1:-manual checkpoint}"
    checkpoint_usable || return 1

    local tmp_index tree parent commit
    tmp_index="$(mktemp -u)"
    if ws_git rev-parse -q --verify HEAD >/dev/null; then
        GIT_INDEX_FILE="$tmp_index" ws_git read-tree HEAD
    fi
    GIT_INDEX_FILE="$tmp_index" ws_git add -A
    tree=$(GIT_INDEX_FILE="$tmp_index" ws_git write-tree)
    rm -f "$tmp_index"

    parent=$(ws_git rev-parse -q --verify "$CHECKPOINT_REF" || true)
    if [ -n "$parent" ] && [ "$(ws_git rev-parse "$parent^{tree}")" = "$tree" ]; then
        log_info "Checkpoint skipped - no changes since the last one"
        return 0
    fi

    commit=$(GIT_AUTHOR_NAME=alcatraz GIT_AUTHOR_EMAIL=checkpoint@alcatraz.local \
             GIT_COMMITTER_NAME=alcatraz GIT_COMMITTER_EMAIL=checkpoint@alcatraz.local \
             ws_git commit-tree "$tree" ${parent:+-p "$parent"} -m "$msg")
    ws_git update-ref "$CHECKPOINT_REF" "$commit"
    log_success "Checkpoint created: ${commit:0:7} - $msg"
}

list_checkpoints() {
    checkpoint_usable || return 1
    if ! ws_git rev-parse -q --verify "$CHECKPOINT_REF" >/dev/null; then
        log_warn "No checkpoints yet. Create one with: ./alcatraz.sh checkpoint"
        return 0
    fi
    log_info "Checkpoints for $ALCATRAZ_WORKSPACE (1 = most recent):"
    ws_git log --format='%h  %ad  %s' --date='format:%Y-%m-%d %H:%M' "$CHECKPOINT_REF" \
        | nl -w3 -s'. '
}

# Restore the workspace's files to a checkpoint (1 = latest, or a hash).
# Only worktree files change - branches, index and git history stay intact.
# A safety checkpoint of the current state is taken first, so rollback is
# itself reversible.
rollback_checkpoint() {
    local sel="${1:-1}"
    checkpoint_usable || return 1
    if ! ws_git rev-parse -q --verify "$CHECKPOINT_REF" >/dev/null; then
        log_error "No checkpoints to roll back to."
        return 1
    fi

    local target
    if [[ "$sel" =~ ^[0-9]+$ ]]; then
        target=$(ws_git rev-parse -q --verify "$CHECKPOINT_REF~$((sel-1))" 2>/dev/null) || {
            log_error "Checkpoint #$sel not found. See: ./alcatraz.sh checkpoints"
            return 1
        }
    else
        target=$(ws_git rev-parse -q --verify "$sel^{commit}" 2>/dev/null) || {
            log_error "Commit not found: $sel"
            return 1
        }
    fi

    create_checkpoint "auto: before rollback to ${target:0:7}" >/dev/null
    local safety
    safety=$(ws_git rev-parse "$CHECKPOINT_REF")

    # Restore every file recorded in the target snapshot...
    ws_git restore --source="$target" --worktree -- :/
    # ...and delete files that exist now but weren't in it. The safety
    # checkpoint just captured "now", so its tree is the authoritative list —
    # untracked-but-not-ignored files included.
    comm -23 <(ws_git ls-tree -r --name-only "$safety" | sort) \
             <(ws_git ls-tree -r --name-only "$target" | sort) \
        | while IFS= read -r f; do rm -f "$ALCATRAZ_WORKSPACE/$f"; done

    log_success "Workspace restored to checkpoint ${target:0:7}"
    log_info "Undo with: ./alcatraz.sh rollback ${safety:0:7}"
}

# Auto-checkpoint hook (disable with ALCATRAZ_AUTO_CHECKPOINT=0). Quietly a
# no-op for non-git workspaces.
auto_checkpoint() {
    local msg="$1"
    module_enabled checkpoints || return 0
    [ "${ALCATRAZ_AUTO_CHECKPOINT:-1}" = "0" ] && return 0
    command -v git &>/dev/null || return 0
    git -C "${ALCATRAZ_WORKSPACE:-.}" rev-parse --git-dir &>/dev/null || return 0
    create_checkpoint "$msg" || true
}

# Resolve an argument as an alias; if not found, return the argument itself.
resolve_alias_or_path() {
    local arg="$1"
    local resolved
    resolved="$(load_workspace_alias "$arg" 2>/dev/null || true)"
    if [ -n "$resolved" ]; then
        echo "$resolved"
    else
        echo "$arg"
    fi
}

image_exists() {
    docker image inspect alcatraz:latest &>/dev/null
}

container_running() {
    $DC $(dc_flags) ps --status running 2>/dev/null | grep -q "alcatraz"
}

build_image() {
    log_info "Building Docker image..."
    $DC $(dc_flags) build --no-cache
    log_success "Image built"
}

# Ensure the memory vault dir (AI_CONTEXT_PATH) exists and is owned by the user.
# Otherwise Docker creates the bind mount as root and the container's uid 1000
# cannot write (Mega Brain fails).
ensure_ai_context_dir() {
    local p="${AI_CONTEXT_PATH:-}"
    if [ -z "$p" ] && [ -f "$SCRIPT_DIR/.env" ]; then
        p="$(grep '^AI_CONTEXT_PATH=' "$SCRIPT_DIR/.env" 2>/dev/null | cut -d'=' -f2- | head -n1 || true)"
    fi
    p="${p:-./.ai-context}"
    case "$p" in
        /*) : ;;                       # absolute
        *)  p="$SCRIPT_DIR/${p#./}" ;; # relative to the compose dir
    esac
    mkdir -p "$p" 2>/dev/null || true
}

start_container() {
    log_info "Starting container..."
    ensure_ai_context_dir
    # shellcheck disable=SC2046
    $DC $(dc_flags) up -d --no-build
    log_success "Container started"
}

stop_container() {
    log_info "Stopping container..."
    # shellcheck disable=SC2046
    $DC $(dc_flags) down
    log_success "Container stopped"
}

check_container_running() {
    if ! container_running; then
        log_warn "Container is not running, starting..."
        load_workspace
        start_container
        sleep 2
    fi
}

# Run a command inside the container with a timeout
run_command() {
    local cmd="$1"
    local -a env_args=()
    collect_api_env_args env_args

    check_container_running

    load_workspace
    auto_checkpoint "auto: before exec"

    log_info "Running: $cmd"
    log_info "Timeout: ${TIMEOUT_SECONDS}s"

    set +e
    timeout "$TIMEOUT_SECONDS" \
        $DC $(dc_flags) exec -T "${env_args[@]}" alcatraz bash -c '. ~/.nvm/nvm.sh 2>/dev/null; '"$cmd"
    local exit_code=$?
    set -e

    if [ $exit_code -eq 124 ]; then
        log_error "Command exceeded timeout of ${TIMEOUT_SECONDS}s!"
        return 124
    elif [ $exit_code -ne 0 ]; then
        log_error "Command failed with code $exit_code"
        return $exit_code
    fi

    log_success "Command executed"
}

# Check file size before running
check_file_size() {
    local file="$1"
    if [ -f "$file" ]; then
        local size_mb
        size_mb=$(du -m "$file" | cut -f1)
        if [ "$size_mb" -gt "$MAX_FILE_SIZE_MB" ]; then
            log_error "File too large: ${size_mb}MB (max: ${MAX_FILE_SIZE_MB}MB)"
            return 1
        fi
    fi
}

# Show container resources
check_resources() {
    log_info "Checking resources..."
    local container_id
    container_id=$($DC $(dc_flags) ps -q alcatraz 2>/dev/null | head -1)
    if [ -n "$container_id" ]; then
        docker stats --no-stream "$container_id" 2>/dev/null || true
    fi
}

# Collect -e flags for API keys present in the host environment
collect_api_env_args() {
    local -n _arr=$1
    for key in ANTHROPIC_API_KEY GOOGLE_API_KEY OPENAI_API_KEY OPENCODE_API_KEY; do
        local val="${!key:-}"
        if [ -n "$val" ]; then
            _arr+=(-e "$key=$val")
        fi
    done
}

check_credentials() {
    echo ""
    log_info "Detected credentials:"

    # Claude Code - via volume (OAuth)
    if [ -f "$HOME/.claude/.credentials.json" ]; then
        log_success "Claude Code  : OAuth via ~/.claude/.credentials.json"
    elif [ -n "${ANTHROPIC_API_KEY:-}" ]; then
        log_success "Claude Code  : ANTHROPIC_API_KEY set"
    else
        log_warn  "Claude Code  : no credentials (no ~/.claude/.credentials.json or ANTHROPIC_API_KEY)"
    fi

    # Gemini - OAuth or API key
    if [ -f "$HOME/.gemini/oauth_creds.json" ]; then
        log_success "Gemini CLI   : OAuth via ~/.gemini/oauth_creds.json"
    elif [ -n "${GOOGLE_API_KEY:-}" ]; then
        log_success "Gemini CLI   : GOOGLE_API_KEY set"
    else
        log_warn  "Gemini CLI   : no credentials (no ~/.gemini/oauth_creds.json or GOOGLE_API_KEY)"
    fi

    # OpenAI / Codex - API key only
    if [ -n "${OPENAI_API_KEY:-}" ]; then
        log_success "OpenAI/Codex : OPENAI_API_KEY set"
    else
        log_warn  "OpenAI/Codex : no credentials (export OPENAI_API_KEY)"
    fi

    # opencode - OPENCODE_API_KEY or provider keys
    if [ -n "${OPENCODE_API_KEY:-}" ]; then
        log_success "OpenCode    : OPENCODE_API_KEY set"
    elif [ -n "${ANTHROPIC_API_KEY:-}" ] || [ -n "${OPENAI_API_KEY:-}" ] || [ -n "${GOOGLE_API_KEY:-}" ]; then
        log_success "OpenCode    : provider credentials available"
    else
        log_warn  "OpenCode    : no credentials (export OPENCODE_API_KEY or ANTHROPIC_API_KEY/OPENAI_API_KEY/GOOGLE_API_KEY)"
    fi

    echo ""
}

# ===== MULTI-PROJECT OVERRIDE =====

# Generate docker-compose.override.yml mounting every project under
# /workspace/projects/<name>: the active ALCATRAZ_WORKSPACE first, then
# any extra paths from PROJECT_PATHS (.env). Removes the file when empty.
generate_projects_override() {
    local override="$SCRIPT_DIR/$OVERRIDE_FILE"

    local count=0
    local alcatraz_volumes=()
    local seen_names=()

    _already_seen() {
        local n="$1" s
        for s in "${seen_names[@]}"; do [ "$s" = "$n" ] && return 0; done
        return 1
    }

    _add_path() {
        local p="$1"
        [ -z "$p" ] && return
        p="$(realpath "$p" 2>/dev/null || readlink -f "$p" 2>/dev/null || echo "$p")"
        [ ! -d "$p" ] && return
        local name
        name="$(basename "$p")"
        _already_seen "$name" && return
        seen_names+=("$name")
        alcatraz_volumes+=("      - ${p}:/workspace/projects/${name}:rw")
        count=$((count + 1))
    }

    # Active workspace goes in first
    _add_path "${ALCATRAZ_WORKSPACE:-}"

    # Extra paths from PROJECT_PATHS
    local paths="${PROJECT_PATHS:-}"
    if [ -z "$paths" ] && [ -f "$SCRIPT_DIR/.env" ]; then
        paths="$(grep '^PROJECT_PATHS=' "$SCRIPT_DIR/.env" 2>/dev/null | cut -d'=' -f2- | head -n1 || true)"
    fi
    if [ -n "$paths" ]; then
        IFS=',' read -ra path_array <<< "$paths"
        for raw_path in "${path_array[@]}"; do
            local p
            p="${raw_path#"${raw_path%%[![:space:]]*}"}"
            p="${p%"${p##*[![:space:]]}"}"
            [ -z "$p" ] && continue
            _add_path "$p"
        done
    fi

    if [ "$count" -eq 0 ]; then
        rm -f "$override"
        return 0
    fi

    {
        echo "# Auto-generated by alcatraz.sh - do not edit manually"
        echo "services:"
        echo "  alcatraz:"
        echo "    volumes:"
        for v in "${alcatraz_volumes[@]}"; do echo "$v"; done
    } > "$override"
}

# Return the correct -f flags for docker compose (with override if present)
dc_flags() {
    local override="$SCRIPT_DIR/$OVERRIDE_FILE"
    if [ -f "$override" ]; then
        echo "-f $DOCKER_COMPOSE_FILE -f $override"
    else
        echo "-f $DOCKER_COMPOSE_FILE"
    fi
}

# ===== COMMON RUN FLOW =====

do_run() {
    local target_path="$1"
    local force_rebuild="${2:-}"

    resolve_workspace "$target_path"

    # Snapshot the workspace before an AI session can touch it
    auto_checkpoint "auto: session start"

    generate_projects_override

    # Ensure the memory vault dir exists (owned by the user)
    ensure_ai_context_dir

    # Stop the container if running (the mount will change). Snapshot every
    # mounted project's Mega Brain context first, so in-progress work in any open
    # shell survives the restart (resume with 'mega-brain resume').
    if container_running; then
        if module_enabled megabrain; then
            log_info "Saving context (mega-brain pause-all) before remounting..."
            $DC $(dc_flags) exec -T alcatraz mega-brain pause-all "auto: container restart" 2>/dev/null || true
        fi
        log_info "Stopping container to mount the new project..."
        # shellcheck disable=SC2046
        $DC $(dc_flags) down
    fi

    if [ -n "$force_rebuild" ]; then
        # Force a rebuild (e.g. after changing the Guard code or Dockerfile)
        log_info "Rebuilding the image (--rebuild)..."
        # shellcheck disable=SC2046
        $DC $(dc_flags) up -d --build
    else
        # Build the image only if it doesn't exist yet
        if ! image_exists; then
            log_info "Image not found - building for the first time (may take a few minutes)..."
            build_image
        fi
        # shellcheck disable=SC2046
        $DC $(dc_flags) up -d --no-build
    fi

    # Wait for the Guard (MITM proxy) to become healthy before
    # returning, so no command runs before the proxy is up.
    log_info "Waiting for the Guard (MITM proxy) to be ready..."
    for i in {1..30}; do
        if $DC $(dc_flags) ps guard 2>/dev/null | grep -q "healthy"; then
            break
        fi
        sleep 1
    done

    # Note: security tests are now manual (they don't block startup)
    log_info "Reminder: run './test-security.sh' to validate isolation whenever you want."

    check_credentials
    log_success "Alcatraz ready - project mounted at /workspace"
    log_success "Lighthouse active - LLM requests pass through MITM sanitization"
    echo ""
    echo "  ./alcatraz.sh exec 'npm install'"
    echo "  ./alcatraz.sh exec 'claude \"refactor src/index.ts\"'"
    echo "  ./alcatraz.sh shell"
}

# ===== UNIFIED FLOW: JAIL + PLATFORM =====

# ===== MAIN COMMAND =====

main() {
    local action="${1:-help}"

    # Single resolution point: migrate an old .env, then compute + export module
    # state so gating below and the container both see identical values.
    ensure_module_block
    resolve_modules

    case "$action" in
        build)
            check_docker
            build_image
            ;;

        run)
            check_docker
            local target_path="" force_rebuild=""
            shift || true
            for arg in "$@"; do
                case "$arg" in
                    --rebuild|-b) force_rebuild=1 ;;
                    *)            target_path="$arg" ;;
                esac
            done
            target_path="$(resolve_alias_or_path "$target_path")"
            do_run "$target_path" "$force_rebuild"
            ;;

        save)
            if [ $# -lt 2 ]; then
                log_error "Usage: ./alcatraz.sh save <name> [path]"
                exit 1
            fi
            save_workspace_alias "$2" "${3:-}"
            ;;

        list|ls)
            list_workspace_aliases
            ;;

        remove|rm)
            if [ $# -lt 2 ]; then
                log_error "Usage: ./alcatraz.sh remove <name>"
                exit 1
            fi
            remove_workspace_alias "$2"
            ;;

        exec)
            if [ $# -lt 2 ]; then
                log_error "Usage: ./alcatraz.sh exec 'command'"
                exit 1
            fi
            local cmd="${2}"
            check_docker
            run_command "$cmd"
            ;;

        shell)
            check_docker
            check_container_running
            local -a env_args=()
            collect_api_env_args env_args
            local workdir="/workspace"
            load_workspace &>/dev/null
            if [ -n "${ALCATRAZ_WORKSPACE:-}" ]; then
                workdir="/workspace/projects/$(basename "$ALCATRAZ_WORKSPACE")"
            fi
            # A running container can't gain a new bind mount. If the saved
            # workspace isn't mounted (state/container out of sync), landing in
            # $workdir would fail with a chdir error — fall back to /workspace.
            if [ "$workdir" != "/workspace" ] && \
               ! $DC $(dc_flags) exec -T alcatraz test -d "$workdir" 2>/dev/null; then
                log_warn "Project not mounted in the running container: $workdir"
                log_warn "Run './alcatraz.sh run <path>' (or use the TUI) to mount it. Falling back to /workspace."
                workdir="/workspace"
            fi
            log_info "Opening a shell in the container..."
            # -it is required so opencode/claude/gemini/codex get a real TTY and
            # raw input mode; without it, Enter and other interactive keys are
            # swallowed or ignored.
            $DC $(dc_flags) exec -it "${env_args[@]}" --workdir "$workdir" alcatraz bash
            ;;

        stop)
            check_docker
            stop_container
            ;;

        clean)
            log_info "Cleaning up..."
            $DC $(dc_flags) down -v
            log_success "Cleanup complete"
            ;;

        test-guard)
            check_docker
            log_info "Running automated Guard tests..."
            if [ ! -f "$SCRIPT_DIR/test-guard.sh" ]; then
                log_error "Test script not found: test-guard.sh"
                exit 1
            fi
            "$SCRIPT_DIR/test-guard.sh"
            ;;

        sessions)
            module_enabled sessions || { module_off_notice sessions; exit 0; }
            check_docker
            check_container_running
            # Session/history data lives in named volumes, so it survives
            # stop/run cycles - this lists what each CLI can resume.
            $DC $(dc_flags) exec -T alcatraz bash -s << 'EOS'
echo "Resumable AI sessions (survive stop/run cycles; wiped by 'clean'):"
echo ""

echo "Claude Code   resume with: claude --continue (latest) | claude --resume (picker)"
found=0
for d in "$HOME/.claude/projects"/*/; do
    [ -d "$d" ] || continue
    n=$(ls "$d"*.jsonl 2>/dev/null | wc -l)
    [ "$n" -eq 0 ] && continue
    last=$(ls -t "$d"*.jsonl 2>/dev/null | head -1)
    when=$(date -r "$last" "+%Y-%m-%d %H:%M" 2>/dev/null || echo "?")
    printf '  %-40s %s session(s), last %s\n' "$(basename "$d")" "$n" "$when"
    found=1
done
[ "$found" -eq 0 ] && echo "  (none)"
echo ""

echo "Codex         resume with: codex resume"
n=$(find "$HOME/.codex/sessions" -name 'rollout-*.jsonl' 2>/dev/null | wc -l)
if [ "$n" -gt 0 ]; then
    last=$(find "$HOME/.codex/sessions" -name 'rollout-*.jsonl' -printf '%T@ %p\n' 2>/dev/null | sort -rn | head -1 | cut -d' ' -f2-)
    when=$(date -r "$last" "+%Y-%m-%d %H:%M" 2>/dev/null || echo "?")
    echo "  $n session(s), last $when"
else
    echo "  (none)"
fi
echo ""

echo "Gemini CLI    resume with: gemini, then /chat resume <tag> (save with /chat save <tag>)"
n=$(find "$HOME/.gemini/tmp" -name 'checkpoint-*.json' 2>/dev/null | wc -l)
if [ "$n" -gt 0 ]; then
    find "$HOME/.gemini/tmp" -name 'checkpoint-*.json' -printf '  %f\n' 2>/dev/null \
        | sed 's/^  checkpoint-/  /; s/\.json$//' | sort | head -20
else
    echo "  (none - save one inside gemini with /chat save <tag>)"
fi
echo ""

echo "opencode      resume with: opencode --continue (latest) | opencode --session <id>"
n=$(find "$HOME/.local/state/opencode" -name 'ses_*' -type f 2>/dev/null | wc -l)
if [ "$n" -gt 0 ]; then
    echo "  $n session file(s) in ~/.local/state/opencode"
else
    echo "  (none)"
fi
EOS
            ;;

        sessions-data)
            # Machine-readable session list for the TUI. One TSV row per
            # resumable session: TOOL \t ID \t CWD \t EPOCH \t LABEL \t RESUME_CMD
            module_enabled sessions || exit 0
            check_docker
            check_container_running
            $DC $(dc_flags) exec -T alcatraz bash -s << 'EOS'
# Claude: one .jsonl per session, grouped by project dir. List each; pull the
# real project cwd (and a first-message snippet) from the file so resume runs
# in the right directory with the right session id.
for d in "$HOME/.claude/projects"/*/; do
    [ -d "$d" ] || continue
    for f in "$d"*.jsonl; do
        [ -e "$f" ] || continue
        id=$(basename "$f" .jsonl)
        epoch=$(stat -c %Y "$f" 2>/dev/null || echo 0)
        cwd=$(grep -m1 -o '"cwd":"[^"]*"' "$f" 2>/dev/null | sed 's/"cwd":"//; s/"$//')
        [ -z "$cwd" ] && cwd="/workspace"
        label=$(basename "$cwd")
        printf 'Claude\t%s\t%s\t%s\t%s\tcd %q 2>/dev/null; claude --resume %s\n' \
            "$id" "$cwd" "$epoch" "$label" "$cwd" "$id"
    done
done
# Codex: flat rollout files; resume via its native picker.
n=$(find "$HOME/.codex/sessions" -name 'rollout-*.jsonl' 2>/dev/null | wc -l)
if [ "$n" -gt 0 ]; then
    last=$(find "$HOME/.codex/sessions" -name 'rollout-*.jsonl' -printf '%T@\n' 2>/dev/null | sort -rn | head -1 | cut -d. -f1)
    printf 'Codex\t\t\t%s\t%s session(s) — native picker\tcodex resume\n' "${last:-0}" "$n"
fi
# Gemini: saved chat tags.
if [ -d "$HOME/.gemini/tmp" ]; then
    find "$HOME/.gemini/tmp" -name 'checkpoint-*.json' -printf '%T@\t%f\n' 2>/dev/null | while IFS=$'\t' read -r ep fn; do
        tag=$(printf '%s' "$fn" | sed 's/^checkpoint-//; s/\.json$//')
        printf 'Gemini\t%s\t\t%s\ttag: %s\tgemini\n' "$tag" "${ep%.*}" "$tag"
    done
fi
# opencode: session state files; resume the latest.
n=$(find "$HOME/.local/state/opencode" -name 'ses_*' -type f 2>/dev/null | wc -l)
if [ "$n" -gt 0 ]; then
    last=$(find "$HOME/.local/state/opencode" -name 'ses_*' -type f -printf '%T@\n' 2>/dev/null | sort -rn | head -1 | cut -d. -f1)
    printf 'opencode\t\t\t%s\t%s session(s) — latest\topencode --continue\n' "${last:-0}" "$n"
fi
EOS
            ;;

        checkpoint)
            module_enabled checkpoints || { module_off_notice checkpoints; exit 0; }
            create_checkpoint "${2:-manual checkpoint}"
            ;;

        checkpoints)
            module_enabled checkpoints || { module_off_notice checkpoints; exit 0; }
            list_checkpoints
            ;;

        rollback)
            module_enabled checkpoints || { module_off_notice checkpoints; exit 0; }
            rollback_checkpoint "${2:-1}"
            ;;

        stats)
            module_enabled stats || { module_off_notice stats; exit 0; }
            check_docker
            if ! $DC $(dc_flags) ps --status running guard 2>/dev/null | grep -q "guard"; then
                log_error "Guard is not running. Start the stack first: ./alcatraz.sh run"
                exit 1
            fi
            # Aggregation runs inside the Guard container, where the stats
            # JSONL lives (audit volume). Same binary, -stats mode.
            $DC $(dc_flags) exec -T guard /alcatraz -stats
            ;;

        status)
            check_docker
            $DC $(dc_flags) ps
            # Show which project is mounted, if the container is running
            if container_running; then
                local container_id
                container_id=$($DC $(dc_flags) ps -q alcatraz 2>/dev/null | head -1)
                if [ -n "$container_id" ]; then
                    local mounted
                    mounted=$(docker inspect "$container_id" \
                        --format '{{range .Mounts}}{{if eq .Destination "/workspace"}}{{.Source}}{{end}}{{end}}' 2>/dev/null || true)
                    [ -n "$mounted" ] && log_info "Mounted project: $mounted -> /workspace"
                fi
            fi
            ;;

        resources)
            check_docker
            check_container_running
            check_resources
            ;;

        logs)
            check_docker
            local svc="alcatraz"
            case "${2:-}" in
                guard|backend|audit) svc="guard" ;;
                squid|proxy)            svc="lighthouse" ;;
            esac
            log_info "Tailing logs for '$svc' (Ctrl+C to exit)..."
            $DC $(dc_flags) logs -f "$svc"
            ;;

        *)
            cat << EOF
${BLUE}Alcatraz Manager${NC}
Isolated Docker sandbox for AI tools (Claude Code, Gemini CLI, Codex, opencode)

${YELLOW}Usage:${NC}
  ./alcatraz.sh [ACTION] [OPTIONS]

${YELLOW}Main actions:${NC}
  build                  Build the Docker image
  run [PATH|ALIAS] [-b]  Bring up the stack (Squid + Guard + jail) and mount PATH as /workspace
                         No argument: uses ./project (created if missing)
                         With PATH: uses the given directory (absolute or relative)
                         With ALIAS: uses a saved favorite workspace
                         If already running, restarts with the new path
                         Builds the image only if missing; waits for the Guard to be ready
                         --rebuild (-b): force an image rebuild (after changing Guard/Dockerfile)
  exec CMD               Run a command in the container
  shell                  Open an interactive shell
  stop                   Stop everything (Squid + Guard + jail)
  clean                  Remove container and volumes

${YELLOW}Workspace checkpoints:${NC}
  checkpoint [msg]   Snapshot the workspace (shadow git ref; auto on run/exec)
  checkpoints        List checkpoints (1 = most recent)
  rollback [N|HASH]  Restore workspace files to a checkpoint (default: latest)
                     Only files change - branches/index/history are untouched
                     Disable auto snapshots with ALCATRAZ_AUTO_CHECKPOINT=0

${YELLOW}Favorite workspaces:${NC}
  save <name> [path] Save the current workspace (or path) under a short name
  list (or ls)       List all favorite workspaces
  remove <name>      Remove a favorite workspace
                     (start one with: run <name>)

${YELLOW}Modules:${NC}
   Core (sandbox + Lighthouse + Guard) is always on. Everything else is a
   module toggled in the .env 'ALCATRAZ_MOD_*' block or the TUI Modules screen.
   Safety net (checkpoints, sessions, stats) is on by default; Mega Brain,
   shakedown, spawn and websearch are opt-in. Manage from the TUI or
   './alcatraz modules'.

${YELLOW}Utilities:${NC}
   status             Show status and which project is mounted
   stats              Token usage/cost report metered by the Guard (per day/model)  [module]
   sessions           List resumable AI sessions per model (claude --continue etc.) [module]
   test-guard      Run automated Guard tests (regression)
   resources          Live CPU/memory usage
   logs [SERVICE]     Tail logs. Default: the jail.
                      'logs guard' for the Guard, 'logs squid' for the proxy

${YELLOW}Examples:${NC}
   ./alcatraz.sh build
   ./alcatraz.sh run                          # mounts ./project
   ./alcatraz.sh run /home/user/my-project    # mounts a specific path
   ./alcatraz.sh run tetris                    # mounts favorite workspace "tetris"
   ./alcatraz.sh run --rebuild                 # force an image rebuild
   ./alcatraz.sh stop                          # stop everything
   ./alcatraz.sh save tetris                   # saves current workspace as "tetris"
   ./alcatraz.sh save tetris /path/to/tetris    # saves a specific path as "tetris"
   ./alcatraz.sh list                         # lists favorites
   ./alcatraz.sh remove tetris                 # removes a favorite
   ./alcatraz.sh exec 'npm install'
   ./alcatraz.sh exec 'claude "refactor src/index.ts"'
   ./alcatraz.sh shell
   ./alcatraz.sh test-guard           # checks the Guard hasn't regressed
   ./alcatraz.sh status

${YELLOW}Environment variables:${NC}
   TIMEOUT_SECONDS     Per-command timeout (default: 300s)
   MAX_FILE_SIZE_MB    Max file size (default: 1000MB)
   ANTHROPIC_API_KEY   Injected automatically into exec/shell if set
   GOOGLE_API_KEY      Injected automatically into exec/shell if set
   OPENAI_API_KEY      Injected automatically into exec/shell if set

${YELLOW}Enforced limits:${NC}
  - CPU: max 1.5 cores
  - Memory: max 4GB
  - Network: Guard (MITM + sanitize) -> Squid (domain whitelist)
  - Filesystem: only /workspace is accessible
  - Sensitive data: sanitized automatically before reaching LLMs
  - User: runs as non-root (uid 1000)

EOF
            ;;
    esac
}

# ===== EXECUTION =====
main "$@"
