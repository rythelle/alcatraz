#!/usr/bin/env bash
# Alcatraz installer
# Usage:
#   git clone https://github.com/USER/alcatraz && cd alcatraz && ./install.sh
#   To update: git pull && ./install.sh

set -euo pipefail

SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
BIN_LINK="$BIN_DIR/alcatraz"

RED=$'\033[0;31m'
GREEN=$'\033[0;32m'
YELLOW=$'\033[1;33m'
BLUE=$'\033[0;34m'
NC=$'\033[0m'

log_info()    { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[✓]${NC} $1"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error()   { echo -e "${RED}[✗]${NC} $1" >&2; }

check_deps() {
    local missing=()
    command -v git    &>/dev/null || missing+=("git")
    command -v docker &>/dev/null || missing+=("docker")
    command -v go     &>/dev/null || missing+=("go")

    if [ ${#missing[@]} -gt 0 ]; then
        log_error "Missing required dependencies: ${missing[*]}"
        echo ""
        [ -z "${missing[*]##*docker*}" ] && echo "  Docker:     https://docs.docker.com/engine/install/"
        [ -z "${missing[*]##*go*}" ]     && echo "  Go:         https://go.dev/doc/install"
        exit 1
    fi

    if ! docker compose version &>/dev/null 2>&1 && ! command -v docker-compose &>/dev/null; then
        log_error "Docker Compose not found."
        echo "  Install: sudo apt-get install docker-compose-plugin"
        exit 1
    fi

    log_success "Dependencies OK (git, docker, go)"
}

build_cli() {
    local cli_dir="$SOURCE_DIR/platform/cli"
    local cli_bin="$cli_dir/alcatraz-cli"

    if [ ! -d "$cli_dir" ]; then
        log_warn "Go CLI source not found at $cli_dir — skipping build"
        return
    fi

    if [ ! -f "$cli_bin" ] || [ "$cli_dir/main.go" -nt "$cli_bin" ]; then
        log_info "Building CLI..."
        (cd "$cli_dir" && go build -o alcatraz-cli .)
        log_success "CLI built"
    else
        log_success "CLI already up to date"
    fi
}

link_binary() {
    mkdir -p "$BIN_DIR"
    chmod +x "$SOURCE_DIR/alcatraz"
    ln -sf "$SOURCE_DIR/alcatraz" "$BIN_LINK"
    log_success "Linked: $BIN_LINK -> $SOURCE_DIR/alcatraz"
}

add_to_path() {
    if [[ ":$PATH:" == *":$BIN_DIR:"* ]]; then
        log_success "$BIN_DIR already in PATH"
        return
    fi

    local export_line="export PATH=\"\$PATH:$BIN_DIR\""
    local added=0

    for rc in "$HOME/.zshrc" "$HOME/.bashrc"; do
        [ -f "$rc" ] || continue
        if ! grep -qF "$BIN_DIR" "$rc"; then
            printf '\n# Alcatraz CLI\n%s\n' "$export_line" >> "$rc"
            log_success "Added $BIN_DIR to PATH in $(basename "$rc")"
            added=1
        fi
    done

    if [ "$added" -eq 0 ]; then
        log_warn "Could not detect .bashrc or .zshrc — add this line manually:"
        echo "  $export_line"
    fi
}

main() {
    echo ""
    echo -e "${BLUE}  Alcatraz Installer${NC}"
    echo -e "  Installed from: $SOURCE_DIR"
    echo ""

    check_deps
    build_cli
    link_binary
    add_to_path

    echo ""
    log_success "Installation complete!"
    echo ""
    echo "  Reload your shell:"
    echo "    source ~/.zshrc   # or ~/.bashrc"
    echo ""
    echo "  Then use from anywhere:"
    echo "    alcatraz run /path/to/your/project"
    echo "    alcatraz save myapp /path/to/your/project"
    echo "    alcatraz list"
    echo ""
    echo "  To update later:"
    echo "    git -C $SOURCE_DIR pull && $SOURCE_DIR/install.sh"
    echo ""
}

main "$@"
