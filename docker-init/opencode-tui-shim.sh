# Alcatraz — opencode shim (sourced by ~/.bashrc via ~/.local/state/ai-env.sh)
#
# History: this used to force the interactive TUI to `--mini`, on the theory
# that opentui's startup handshake (Kitty keyboard protocol, modifyOtherKeys,
# bracketed paste) broke under Alcatraz's nested PTY chain and swallowed Enter.
# That diagnosis was wrong. Driving the real chain — host terminal ->
# `docker exec -it` -> interactive bash -> opencode — shows the full TUI
# rendering and Enter submitting normally: the PTY relays the terminal's query
# replies just fine. What opentui actually needs is a terminal that ANSWERS
# those queries; when nothing answers it still starts, just several seconds
# later and with a sparser first frame. So the full TUI is the default again.
#
# Opt in to the line-mode interface (no alt-screen, no capability probing —
# handy over a dumb pipe or a very slow link):
#   ALCATRAZ_OPENCODE_MINI=1 opencode
# or just run `opencode --mini` directly.

opencode() {
    # Only the bare interactive TUI is affected; subcommands pass through.
    if [ -z "${ALCATRAZ_OPENCODE_MINI:-}" ]; then
        command opencode "$@"
        return
    fi

    local subcmds=" completion acp mcp attach run debug providers auth agent upgrade uninstall serve web models stats export import github pr session plugin plug db "
    local first="" a
    for a in "$@"; do
        case "$a" in
            -*) continue ;;
            *)  first="$a"; break ;;
        esac
    done
    if [ -n "$first" ] && [ "${subcmds#* $first }" != "$subcmds" ]; then
        command opencode "$@"
        return
    fi

    case " $* " in
        *" --mini "*|*" -h "*|*" --help "*|*" -v "*|*" --version "*)
            command opencode "$@"
            return
            ;;
    esac

    command opencode --mini "$@"
}

# ── Dynamic per-project Mega Brain context ──────────────────────────────────
# In multi-project mode every project is mounted at /workspace/projects/<name>.
# Load that project's Mega Brain context automatically the first time you enter
# it in a shell, and again whenever you cd into a DIFFERENT project — so context
# follows you around without restarting the container or running `mega-brain
# load` by hand. Disable with ALCATRAZ_NO_AUTOLOAD=1.
__alcatraz_mb_last=""
__alcatraz_mb_autoload() {
    [ -n "${ALCATRAZ_NO_AUTOLOAD:-}" ] && return 0
    local proj=""
    case "$PWD/" in
        /workspace/projects/*/)
            proj="${PWD#/workspace/projects/}"
            proj="${proj%%/*}"
            ;;
        *) return 0 ;;
    esac
    [ -z "$proj" ] && return 0
    [ "$proj" = "$__alcatraz_mb_last" ] && return 0
    __alcatraz_mb_last="$proj"
    command -v mega-brain >/dev/null 2>&1 || return 0
    printf '\033[2m── Mega Brain: %s ──\033[0m\n' "$proj" >&2
    mega-brain context 2>/dev/null || true
}
# Register once, preserving any existing PROMPT_COMMAND.
case "${PROMPT_COMMAND:-}" in
    *__alcatraz_mb_autoload*) : ;;
    "")  PROMPT_COMMAND="__alcatraz_mb_autoload" ;;
    *)   PROMPT_COMMAND="__alcatraz_mb_autoload; ${PROMPT_COMMAND}" ;;
esac

# ── Persistent shell history ────────────────────────────────────────────────
# $HOME is the alcatraz-home volume, so ~/.bash_history already outlives the
# container — but bash only writes it on a CLEAN exit. A shell killed by
# `alcatraz stop`, a closed terminal or a restart lost everything typed in it,
# which is why history looked like it wasn't kept at all. So: flush after every
# command (`history -a`), append instead of overwrite, and keep a window long
# enough to be worth searching. Only `alcatraz clean` (down -v) wipes it.
HISTFILE="$HOME/.bash_history"
HISTSIZE=50000
HISTFILESIZE=100000
HISTTIMEFORMAT='%F %T  '
shopt -s histappend
case "${PROMPT_COMMAND:-}" in
    *"history -a"*) : ;;
    "")  PROMPT_COMMAND="history -a" ;;
    *)   PROMPT_COMMAND="history -a; ${PROMPT_COMMAND}" ;;
esac
