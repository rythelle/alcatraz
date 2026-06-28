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

    generate_projects_override

    # Ensure the memory vault dir exists (owned by the user)
    ensure_ai_context_dir

    # Stop the container if running (the mount will change)
    if container_running; then
        log_info "Stopping container to mount the new project..."
        # shellcheck disable=SC2046
        $DC $(dc_flags) down
    fi

    if [ -n "$force_rebuild" ]; then
        # Force a rebuild (e.g. after changing the Guardian code or Dockerfile)
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

    # Wait for the Data Guardian (MITM proxy) to become healthy before
    # returning, so no command runs before the proxy is up.
    log_info "Waiting for the Data Guardian (MITM proxy) to be ready..."
    for i in {1..30}; do
        if $DC $(dc_flags) ps alcatraz-backend 2>/dev/null | grep -q "healthy"; then
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
            log_info "Opening a shell in the container..."
            $DC $(dc_flags) exec "${env_args[@]}" --workdir "$workdir" alcatraz bash
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

        test-guardian)
            check_docker
            log_info "Running automated Data Guardian tests..."
            if [ ! -f "$SCRIPT_DIR/test-guardian.sh" ]; then
                log_error "Test script not found: test-guardian.sh"
                exit 1
            fi
            "$SCRIPT_DIR/test-guardian.sh"
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
                guardian|backend|audit) svc="alcatraz-backend" ;;
                squid|proxy)            svc="proxy-whitelist" ;;
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
  run [PATH|ALIAS] [-b]  Bring up the stack (Squid + Guardian + jail) and mount PATH as /workspace
                         No argument: uses ./project (created if missing)
                         With PATH: uses the given directory (absolute or relative)
                         With ALIAS: uses a saved favorite workspace
                         If already running, restarts with the new path
                         Builds the image only if missing; waits for the Guardian to be ready
                         --rebuild (-b): force an image rebuild (after changing Guardian/Dockerfile)
  exec CMD               Run a command in the container
  shell                  Open an interactive shell
  stop                   Stop everything (Squid + Guardian + jail)
  clean                  Remove container and volumes

${YELLOW}Favorite workspaces:${NC}
  save <name> [path] Save the current workspace (or path) under a short name
  list (or ls)       List all favorite workspaces
  remove <name>      Remove a favorite workspace
                     (start one with: run <name>)

${YELLOW}Utilities:${NC}
   status             Show status and which project is mounted
   test-guardian      Run automated Data Guardian tests (regression)
   resources          Live CPU/memory usage
   logs [SERVICE]     Tail logs. Default: the jail.
                      'logs guardian' for the Data Guardian, 'logs squid' for the proxy

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
   ./alcatraz.sh test-guardian           # checks the Guardian hasn't regressed
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
  - Network: Data Guardian (MITM + sanitize) -> Squid (domain whitelist)
  - Filesystem: only /workspace is accessible
  - Sensitive data: sanitized automatically before reaching LLMs
  - User: runs as non-root (uid 1000)

EOF
            ;;
    esac
}

# ===== EXECUTION =====
main "$@"
