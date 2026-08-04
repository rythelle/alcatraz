#!/usr/bin/env bash
# Automated tests for alcatraz.sh
# Tests pure functions (no docker): workspace management, env loading,
# log functions, check_file_size, collect_api_env_args.
#   Run: bash tests/test_ai_jail.sh
# Does not require Docker installed.

set -uo pipefail

SCRIPT_SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/alcatraz.sh"

if [ ! -f "$SCRIPT_SRC" ]; then
    echo "ERROR: alcatraz.sh not found at $SCRIPT_SRC"
    exit 1
fi

# Test framework
PASS=0
FAIL=0
ERRORS=()

_ok()   { echo "  [PASS] $1"; PASS=$((PASS+1)); }
_fail() { echo "  [FAIL] $1"; FAIL=$((FAIL+1)); ERRORS+=("$1"); }

assert_eq() {
    local desc="$1" got="$2" want="$3"
    if [ "$got" = "$want" ]; then _ok "$desc"
    else _fail "$desc -> got='$got' want='$want'"; fi
}

assert_contains() {
    local desc="$1" haystack="$2" needle="$3"
    if [[ "$haystack" == *"$needle"* ]]; then _ok "$desc"
    else _fail "$desc -> '$needle' not found"; fi
}

assert_file_exists() {
    local desc="$1" file="$2"
    if [ -f "$file" ]; then _ok "$desc"
    else _fail "$desc -> file missing: $file"; fi
}

assert_file_contains() {
    local desc="$1" file="$2" needle="$3"
    if grep -qF "$needle" "$file" 2>/dev/null; then _ok "$desc"
    else _fail "$desc -> '$needle' not found in $file"; fi
}

assert_file_not_contains() {
    local desc="$1" file="$2" needle="$3"
    if ! grep -qF "$needle" "$file" 2>/dev/null; then _ok "$desc"
    else _fail "$desc -> '$needle' found in $file (should not be)"; fi
}

assert_exit_nonzero() {
    local desc="$1" code="$2"
    if [ "$code" -ne 0 ]; then _ok "$desc"
    else _fail "$desc -> expected exit != 0"; fi
}

assert_exit_zero() {
    local desc="$1" code="$2"
    if [ "$code" -eq 0 ]; then _ok "$desc"
    else _fail "$desc -> expected exit 0, got $code"; fi
}

suite() { echo ""; echo "> $1"; }

# Setup: isolated test dir + docker mock + source the functions
TEST_ROOT="$(mktemp -d)"
trap 'rm -rf "$TEST_ROOT"' EXIT

cp "$SCRIPT_SRC" "$TEST_ROOT/alcatraz.sh"
touch "$TEST_ROOT/docker-compose.go.yml"
mkdir -p "$TEST_ROOT/project"

# docker mock that answers version checks without errors
mkdir -p "$TEST_ROOT/bin"
cat > "$TEST_ROOT/bin/docker" << 'MOCK'
#!/bin/bash
case "${1:-}" in
    compose)
        case "${2:-}" in
            version) echo "Docker Compose version v2.0.0"; exit 0 ;;
            ps)      exit 0 ;;
            *)       exit 0 ;;
        esac ;;
    image)
        case "${2:-}" in
            inspect) exit 1 ;;  # simulate "image does not exist"
        esac ;;
    *)  exit 0 ;;
esac
MOCK
chmod +x "$TEST_ROOT/bin/docker"
export PATH="$TEST_ROOT/bin:$PATH"

# Version without the 'main "$@"' line so we can source only the functions
grep -v '^main "\$@"' "$TEST_ROOT/alcatraz.sh" > "$TEST_ROOT/alcatraz-lib.sh"

set +e
source "$TEST_ROOT/alcatraz-lib.sh" 2>/dev/null
set +e

# Point state variables at the test dir
SCRIPT_DIR="$TEST_ROOT"
STATE_FILE="$SCRIPT_DIR/.alcatraz-state"
WORKSPACES_FILE="$SCRIPT_DIR/.alcatraz-workspaces"

# --- Suite 1: save_workspace_alias ---
suite "save_workspace_alias"
rm -f "$WORKSPACES_FILE"

save_workspace_alias "proj1" "$TEST_ROOT/project"
assert_file_exists "workspaces file created"         "$WORKSPACES_FILE"
assert_file_contains "alias saved"                   "$WORKSPACES_FILE" "proj1=$TEST_ROOT/project"

mkdir -p "$TEST_ROOT/other"
save_workspace_alias "other" "$TEST_ROOT/other"
assert_file_contains "second alias"                  "$WORKSPACES_FILE" "other=$TEST_ROOT/other"

# Overwrite existing alias (no duplicate)
save_workspace_alias "proj1" "$TEST_ROOT"
count_p1="$(grep -c '^proj1=' "$WORKSPACES_FILE")"
assert_eq "alias not duplicated"                     "$count_p1" "1"
assert_file_contains "alias updated"                 "$WORKSPACES_FILE" "proj1=$TEST_ROOT"

# Invalid name (with space) -> return 1
set +e; save_workspace_alias "name with space" "$TEST_ROOT" 2>/dev/null; rc_sp=$?; set +e
assert_exit_nonzero "name with space -> error" "$rc_sp"

# Invalid name (with =)
set +e; save_workspace_alias "name=bad" "$TEST_ROOT" 2>/dev/null; rc_eq=$?; set +e
assert_exit_nonzero "name with = -> error" "$rc_eq"

# Invalid name (with #)
set +e; save_workspace_alias "name#bad" "$TEST_ROOT" 2>/dev/null; rc_hash=$?; set +e
assert_exit_nonzero "name with # -> error" "$rc_hash"

# Non-existent path
set +e; save_workspace_alias "nopath" "/path/does/not/exist" 2>/dev/null; rc_np=$?; set +e
assert_exit_nonzero "non-existent path -> error" "$rc_np"

# Empty name
set +e; save_workspace_alias "" "$TEST_ROOT" 2>/dev/null; rc_empty=$?; set +e
assert_exit_nonzero "empty name -> error" "$rc_empty"

# --- Suite 2: load_workspace_alias ---
suite "load_workspace_alias"
rm -f "$WORKSPACES_FILE"
save_workspace_alias "alpha" "$TEST_ROOT/project" 2>/dev/null

result="$(load_workspace_alias "alpha")"
assert_eq "existing alias"      "$result" "$TEST_ROOT/project"

result_miss="$(load_workspace_alias "does-not-exist")"
assert_eq "missing alias"       "$result_miss" ""

result_empty="$(load_workspace_alias "")"
assert_eq "empty arg -> empty"  "$result_empty" ""

mkdir -p "$TEST_ROOT/p2"
save_workspace_alias "beta" "$TEST_ROOT/p2" 2>/dev/null
assert_eq "beta resolves" "$(load_workspace_alias "beta")" "$TEST_ROOT/p2"
assert_eq "alpha resolves" "$(load_workspace_alias "alpha")" "$TEST_ROOT/project"

# --- Suite 3: list_workspace_aliases ---
suite "list_workspace_aliases"
rm -f "$WORKSPACES_FILE"

out_empty="$(list_workspace_aliases 2>&1)"
assert_contains "empty list -> warning"  "$out_empty" "No favorite workspaces"
assert_contains "empty list -> hint"     "$out_empty" "save"

save_workspace_alias "dev"  "$TEST_ROOT/project" 2>/dev/null
save_workspace_alias "prod" "$TEST_ROOT" 2>/dev/null
out_list="$(list_workspace_aliases 2>&1)"
assert_contains "list shows dev"         "$out_list" "dev"
assert_contains "list shows prod"        "$out_list" "prod"
assert_contains "list shows dev path"    "$out_list" "$TEST_ROOT/project"

# --- Suite 4: remove_workspace_alias ---
suite "remove_workspace_alias"
rm -f "$WORKSPACES_FILE"
save_workspace_alias "rmtest" "$TEST_ROOT/project" 2>/dev/null

remove_workspace_alias "rmtest"
assert_file_not_contains "alias removed"         "$WORKSPACES_FILE" "rmtest"

set +e; remove_workspace_alias "does-not-exist" 2>/dev/null; rc_miss=$?; set +e
assert_exit_nonzero "remove missing -> error" "$rc_miss"

set +e; remove_workspace_alias "" 2>/dev/null; rc_empty=$?; set +e
assert_exit_nonzero "remove without name -> error" "$rc_empty"

# Remove one of two, the other stays
save_workspace_alias "keep1" "$TEST_ROOT/project" 2>/dev/null
save_workspace_alias "keep2" "$TEST_ROOT" 2>/dev/null
remove_workspace_alias "keep1"
assert_file_not_contains "keep1 removed"      "$WORKSPACES_FILE" "keep1"
assert_file_contains     "keep2 kept"         "$WORKSPACES_FILE" "keep2"

# --- Suite 5: resolve_alias_or_path ---
suite "resolve_alias_or_path"
rm -f "$WORKSPACES_FILE"
save_workspace_alias "myalias" "$TEST_ROOT/project" 2>/dev/null

resolved="$(resolve_alias_or_path "myalias")"
assert_eq "alias -> path"            "$resolved" "$TEST_ROOT/project"

passthrough="$(resolve_alias_or_path "/any/path")"
assert_eq "literal path passthrough"  "$passthrough" "/any/path"

empty_res="$(resolve_alias_or_path "")"
assert_eq "empty arg -> empty"        "$empty_res" ""

unknown="$(resolve_alias_or_path "missing-alias")"
assert_eq "missing alias -> passthrough" "$unknown" "missing-alias"

# --- Suite 6: load_env_workspace ---
suite "load_env_workspace"
rm -f "$SCRIPT_DIR/.env"

out_no="$(load_env_workspace)"
assert_eq "no .env -> empty" "$out_no" ""

echo "ALCATRAZ_WORKSPACE=$TEST_ROOT/project" > "$SCRIPT_DIR/.env"
out_abs="$(load_env_workspace)"
assert_eq "absolute path from .env" "$out_abs" "$TEST_ROOT/project"

echo "ALCATRAZ_WORKSPACE=project" > "$SCRIPT_DIR/.env"
out_rel="$(load_env_workspace)"
assert_eq "relative path resolved" "$out_rel" "$SCRIPT_DIR/project"

echo "OTHER_VAR=value" > "$SCRIPT_DIR/.env"
out_miss="$(load_env_workspace)"
assert_eq ".env without the key -> empty" "$out_miss" ""

rm -f "$SCRIPT_DIR/.env"

# --- Suite 7: save_workspace / load_workspace (state file) ---
suite "save_workspace / load_workspace"
rm -f "$STATE_FILE"

ALCATRAZ_WORKSPACE="$TEST_ROOT/project"
save_workspace
assert_file_exists   "state file created"        "$STATE_FILE"
assert_file_contains "state contains path"       "$STATE_FILE" "ALCATRAZ_WORKSPACE=$TEST_ROOT/project"

unset ALCATRAZ_WORKSPACE
load_workspace
assert_eq "path restored"                        "$ALCATRAZ_WORKSPACE" "$TEST_ROOT/project"

rm -f "$STATE_FILE"
unset ALCATRAZ_WORKSPACE
load_workspace
assert_eq "no state -> default"                  "$ALCATRAZ_WORKSPACE" "$SCRIPT_DIR/project"

# --- Suite 8: check_file_size ---
suite "check_file_size"
_tf="$(mktemp "$TEST_ROOT/testfile.XXXXXX")"

dd if=/dev/zero of="$_tf" bs=1024 count=1 2>/dev/null
set +e; check_file_size "$_tf"; rc_small=$?; set +e
assert_exit_zero "small file -> ok" "$rc_small"

set +e; check_file_size "/file/does/not/exist.db"; rc_nofile=$?; set +e
assert_exit_zero "missing file -> ok" "$rc_nofile"

rm -f "$_tf"

# --- Suite 9: collect_api_env_args ---
suite "collect_api_env_args"
for _var in ANTHROPIC_API_KEY GOOGLE_API_KEY OPENAI_API_KEY OPENCODE_API_KEY; do
    unset "$_var" 2>/dev/null || true
done

declare -a args_empty=()
collect_api_env_args args_empty
assert_eq "no vars -> empty array" "${#args_empty[@]}" "0"

ANTHROPIC_API_KEY="sk-test-123"
declare -a args_one=()
collect_api_env_args args_one
assert_contains "ANTHROPIC included" "${args_one[*]}" "ANTHROPIC_API_KEY=sk-test-123"
assert_contains "-e flag present"    "${args_one[*]}" "-e"
unset ANTHROPIC_API_KEY

ANTHROPIC_API_KEY="sk-a"
OPENAI_API_KEY="sk-b"
declare -a args_multi=()
collect_api_env_args args_multi
assert_contains "multiple: anthropic" "${args_multi[*]}" "ANTHROPIC_API_KEY=sk-a"
assert_contains "multiple: openai"    "${args_multi[*]}" "OPENAI_API_KEY=sk-b"
unset ANTHROPIC_API_KEY OPENAI_API_KEY

GOOGLE_API_KEY="gk-xyz"
declare -a args_google=()
collect_api_env_args args_google
assert_contains "google included" "${args_google[*]}" "GOOGLE_API_KEY=gk-xyz"
unset GOOGLE_API_KEY

# --- Suite 10: log functions ---
suite "log functions"

out_info="$(log_info "info message" 2>&1)"
assert_contains "log_info has INFO"     "$out_info" "INFO"
assert_contains "log_info has message"  "$out_info" "info message"

out_success="$(log_success "operation ok" 2>&1)"
assert_contains "log_success has ✓"     "$out_success" "✓"
assert_contains "log_success has msg"   "$out_success" "operation ok"

out_warn="$(log_warn "heads up" 2>&1)"
assert_contains "log_warn has WARN"     "$out_warn" "WARN"
assert_contains "log_warn has message"  "$out_warn" "heads up"

out_error="$(log_error "serious error" 2>&1)"
assert_contains "log_error has ✗"       "$out_error" "✗"
assert_contains "log_error has message" "$out_error" "serious error"

# --- Suite 11: alcatraz.sh subprocess - save / list / remove ---
suite "alcatraz.sh subprocess: save / list / remove"

JAIL="bash $TEST_ROOT/alcatraz.sh"
rm -f "$TEST_ROOT/.alcatraz-workspaces" "$TEST_ROOT/.alcatraz-state"

out_save="$(cd "$TEST_ROOT" && $JAIL save "sp1" "$TEST_ROOT/project" 2>&1)"
assert_contains "save via CLI: success"  "$out_save" "sp1"
assert_file_contains "sp1 in file"       "$TEST_ROOT/.alcatraz-workspaces" "sp1="

out_list="$(cd "$TEST_ROOT" && $JAIL list 2>&1)"
assert_contains "list via CLI: sp1"      "$out_list" "sp1"

out_ls="$(cd "$TEST_ROOT" && $JAIL ls 2>&1)"
assert_contains "ls alias works"         "$out_ls" "sp1"

# Save a second alias before removing: 'grep -v' in alcatraz.sh returns
# exit 1 when the alias is the ONLY entry (no lines left), which kills the
# script via set -e. With two entries, removing one leaves the other.
cd "$TEST_ROOT" && $JAIL save "sp1-extra" "$TEST_ROOT/project" 2>/dev/null
out_rm="$(cd "$TEST_ROOT" && $JAIL remove "sp1" 2>&1)"
assert_contains "remove via CLI: success" "$out_rm" "removed"
assert_file_not_contains "sp1 removed"   "$TEST_ROOT/.alcatraz-workspaces" "sp1="
assert_file_contains "sp1-extra kept"    "$TEST_ROOT/.alcatraz-workspaces" "sp1-extra="

mkdir -p "$TEST_ROOT/project"
cd "$TEST_ROOT" && $JAIL save "sp2" "$TEST_ROOT/project" 2>/dev/null
out_rm2="$(cd "$TEST_ROOT" && $JAIL rm "sp2" 2>&1)"
assert_contains "rm alias works"         "$out_rm2" "removed"

set +e; out_noname="$(cd "$TEST_ROOT" && $JAIL save 2>&1)"; rc_sn=$?; set +e
assert_exit_nonzero "save without name -> exit != 0" "$rc_sn"

set +e; out_rn="$(cd "$TEST_ROOT" && $JAIL remove 2>&1)"; rc_rn=$?; set +e
assert_exit_nonzero "remove without name -> exit != 0" "$rc_rn"

set +e; out_use="$(cd "$TEST_ROOT" && $JAIL use "missing-alias" 2>&1)"; rc_use=$?; set +e
assert_exit_nonzero "use missing alias -> exit != 0" "$rc_use"

set +e; out_exec="$(cd "$TEST_ROOT" && $JAIL exec 2>&1)"; rc_exec=$?; set +e
assert_exit_nonzero "exec without argument -> exit != 0" "$rc_exec"

# --- Suite 12: help (unknown and explicit action) ---
suite "alcatraz.sh help"

out_help="$(cd "$TEST_ROOT" && bash "$TEST_ROOT/alcatraz.sh" invalid-cmd-xyz 2>&1)"
assert_contains "help has 'Alcatraz'"   "$out_help" "Alcatraz"
assert_contains "help has 'run'"        "$out_help" "run"
assert_contains "help has 'exec'"       "$out_help" "exec"
assert_contains "help has 'platform'"   "$out_help" "platform"
assert_contains "help has 'save'"       "$out_help" "save"
assert_contains "help has 'list'"       "$out_help" "list"
assert_contains "help has 'remove'"     "$out_help" "remove"
assert_contains "help has TIMEOUT"      "$out_help" "TIMEOUT_SECONDS"
assert_contains "help has examples"     "$out_help" "Examples"

out_help2="$(cd "$TEST_ROOT" && bash "$TEST_ROOT/alcatraz.sh" help 2>&1)"
assert_contains "explicit help works"   "$out_help2" "Alcatraz"

# --- Result ---
echo ""
echo "========================================"
echo " Result: $PASS passed, $FAIL failed"
echo "========================================"

if [ ${#ERRORS[@]} -gt 0 ]; then
    echo ""
    echo "Failures:"
    for e in "${ERRORS[@]}"; do echo "  x $e"; done
fi

[ "$FAIL" -eq 0 ]
