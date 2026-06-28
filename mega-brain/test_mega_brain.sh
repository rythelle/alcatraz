#!/usr/bin/env bash
# Automated tests for mega-brain/mega-brain.sh
#   Run: bash mega-brain/test_mega_brain.sh
# mega-brain.sh uses 'exit' in some functions, so cmd_* calls are wrapped in subshells ().

set -uo pipefail

BRAIN_SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/mega-brain.sh"

if [ ! -f "$BRAIN_SCRIPT" ]; then
    echo "ERROR: mega-brain.sh not found at $BRAIN_SCRIPT"
    exit 1
fi

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
    if echo "$haystack" | grep -qF "$needle" 2>/dev/null; then _ok "$desc"
    else _fail "$desc -> '$needle' not found in output"; fi
}

assert_file_exists() {
    local desc="$1" file="$2"
    if [ -f "$file" ]; then _ok "$desc"
    else _fail "$desc -> file missing: $file"; fi
}

assert_dir_exists() {
    local desc="$1" dir="$2"
    if [ -d "$dir" ]; then _ok "$desc"
    else _fail "$desc -> dir missing: $dir"; fi
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
    else _fail "$desc -> expected exit != 0, got 0"; fi
}

suite() { echo ""; echo "> $1"; }

# Setup: source the script, then override CONTEXT_BASE + detect_project.
TMPDIR_BRAIN="$(mktemp -d)"
trap 'rm -rf "$TMPDIR_BRAIN"' EXIT

set +e
source "$BRAIN_SCRIPT" __noop__ 2>/dev/null
set +e

CONTEXT_BASE="$TMPDIR_BRAIN"
DATE="2026-05-30"
detect_project() { echo "testproject"; }

# --- Suite 1: to_kebab ---
suite "to_kebab"
assert_eq "lowercase"            "$(to_kebab 'hello world')"     "hello-world"
assert_eq "uppercase converted"  "$(to_kebab 'Hello World')"     "hello-world"
assert_eq "underscore to hyphen" "$(to_kebab 'my_task')"         "my-task"
assert_eq "already kebab"        "$(to_kebab 'already-kebab')"   "already-kebab"
assert_eq "empty string"         "$(to_kebab '')"                ""
assert_eq "double spaces"        "$(to_kebab 'a  b')"            "a-b"
assert_eq "punctuation"          "$(to_kebab 'Refactor: Done!')" "refactor-done"
assert_eq "leading hyphen"       "$(to_kebab ':start')"          "start"
assert_eq "trailing hyphen"      "$(to_kebab 'end!')"            "end"

# --- Suite 2: count_files ---
suite "count_files"
_d1="$(mktemp -d)"
assert_eq "empty dir"            "$(count_files "$_d1")" "0"
touch "$_d1/a.md" "$_d1/b.md"
assert_eq "two .md"              "$(count_files "$_d1")" "2"
touch "$_d1/readme.txt"
assert_eq "ignores non-.md"      "$(count_files "$_d1")" "2"
assert_eq "missing dir -> 0"     "$(count_files "/no/such/xyz")" "0"
rm -rf "$_d1"

# --- Suite 3: get_context_path (default + optional prefix grouping) ---
suite "get_context_path"
assert_eq "normal project"       "$(get_context_path "myproj")"    "$TMPDIR_BRAIN/myproj"
assert_eq "no grouping by default" "$(get_context_path "org-front")" "$TMPDIR_BRAIN/org-front"
assert_eq "no arg uses detect"   "$(get_context_path)"             "$TMPDIR_BRAIN/testproject"

GROUP_PREFIX="org-"; GROUP_DIR="ORG"
assert_eq "grouping: matching prefix" "$(get_context_path "org-front")" "$TMPDIR_BRAIN/ORG/org-front"
assert_eq "grouping: non-matching"    "$(get_context_path "other")"     "$TMPDIR_BRAIN/other"
GROUP_PREFIX=""; GROUP_DIR=""

# --- Suite 4: ensure_structure ---
suite "ensure_structure"
_sp="$TMPDIR_BRAIN/struct-test"
ensure_structure "$_sp"
assert_dir_exists "Context"          "$_sp/Context"
assert_dir_exists "Memory/patterns"  "$_sp/Memory/patterns"
assert_dir_exists "Memory/decisions" "$_sp/Memory/decisions"
assert_dir_exists "Memory/gotchas"   "$_sp/Memory/gotchas"
assert_dir_exists "Tasks/active"     "$_sp/Tasks/active"
assert_dir_exists "Tasks/done"       "$_sp/Tasks/done"
assert_dir_exists "Tasks/backlog"    "$_sp/Tasks/backlog"
assert_dir_exists "Logs"             "$_sp/Logs"
ensure_structure "$_sp"
assert_dir_exists "idempotent" "$_sp/Context"

# --- Suite 5: cmd_init ---
suite "cmd_init"
out_init="$(cmd_init 2>&1)"
_ip="$TMPDIR_BRAIN/testproject"
assert_dir_exists  "structure created"        "$_ip"
assert_file_exists "INDEX.md created"         "$_ip/INDEX.md"
assert_file_exists "timeline.md created"      "$_ip/Logs/timeline.md"
assert_file_contains "INDEX has project"      "$_ip/INDEX.md" "testproject"
assert_file_contains "INDEX has Navigation"   "$_ip/INDEX.md" "Navigation"
assert_contains    "output says Initialized"  "$out_init" "Initialized"

echo "PREVIOUS CONTENT" >> "$_ip/INDEX.md"
( cmd_init 2>/dev/null )
assert_file_contains "INDEX not overwritten" "$_ip/INDEX.md" "PREVIOUS CONTENT"

# --- Suite 6: cmd_task ---
suite "cmd_task"
out_task="$(cmd_task "My Feature" 2>&1)"
_tp="$TMPDIR_BRAIN/testproject"
assert_file_exists "task file created"        "$_tp/Tasks/active/my-feature.md"
assert_file_exists "current-task.md created"  "$_tp/Context/current-task.md"
assert_file_contains "task file has title"    "$_tp/Tasks/active/my-feature.md" "My Feature"
assert_file_contains "task file has date"     "$_tp/Tasks/active/my-feature.md" "2026-05-30"
assert_file_contains "current-task has link"  "$_tp/Context/current-task.md" "my-feature"
assert_contains "output confirms task"        "$out_task" "my-feature"

echo "EXISTING CONTENT" > "$_tp/Tasks/active/my-feature.md"
out_task2="$(cmd_task "My Feature" 2>&1)"
assert_contains "existing task loaded"        "$out_task2" "Existing"
assert_file_contains "task not overwritten"   "$_tp/Tasks/active/my-feature.md" "EXISTING CONTENT"

( cmd_task "" 2>/dev/null ); rc_noname=$?
assert_exit_nonzero "task without name -> exit != 0" "$rc_noname"

# --- Suite 7: cmd_remember (also covers auto-created singular dir) ---
suite "cmd_remember"
_mp="$TMPDIR_BRAIN/testproject"
out_rem="$(cmd_remember "pattern" "Use Composition" "prefer composition" 2>&1)"
assert_file_exists "pattern created"          "$_mp/Memory/pattern/use-composition.md"
assert_file_contains "pattern has type"       "$_mp/Memory/pattern/use-composition.md" "pattern"
assert_file_contains "pattern has date"       "$_mp/Memory/pattern/use-composition.md" "2026-05-30"
assert_file_contains "pattern has project"    "$_mp/Memory/pattern/use-composition.md" "testproject"
assert_contains "output confirms Saved"       "$out_rem" "Saved"

( cmd_remember "decision" "REST over GraphQL" "" 2>/dev/null )
assert_file_exists "decision created" "$_mp/Memory/decision/rest-over-graphql.md"

( cmd_remember "gotcha" "Null pointer in loop" "" 2>/dev/null )
assert_file_exists "gotcha created" "$_mp/Memory/gotcha/null-pointer-in-loop.md"

( cmd_remember "note" "Deploy on friday" "never" 2>/dev/null )
assert_file_exists "note created" "$_mp/Memory/note/deploy-on-friday.md"

( cmd_remember "bad_type" "x" "" 2>/dev/null ); rc_inv=$?
assert_exit_nonzero "invalid type -> exit != 0" "$rc_inv"

( cmd_remember "pattern" "" "" 2>/dev/null ); rc_noname=$?
assert_exit_nonzero "name required" "$rc_noname"

( cmd_remember "" "x" "" 2>/dev/null ); rc_notype=$?
assert_exit_nonzero "type required" "$rc_notype"

echo "ORIGINAL CONTENT" > "$_mp/Memory/pattern/use-composition.md"
( cmd_remember "pattern" "Use Composition" "new content" 2>/dev/null )
assert_file_not_contains "remember does not overwrite" \
    "$_mp/Memory/pattern/use-composition.md" "new content"

# --- Suite 8: cmd_done ---
suite "cmd_done"
_tp="$TMPDIR_BRAIN/testproject"
( cmd_task "Done Feature" 2>/dev/null )
out_done="$(cmd_done "learning A; learning B" 2>&1)"
assert_file_exists "task moved to done"     "$_tp/Tasks/done/done-feature.md"
assert_file_contains "done has 100%"        "$_tp/Tasks/done/done-feature.md" "100%"
assert_file_contains "timeline has entry"   "$_tp/Logs/timeline.md" "Done Feature"
assert_file_contains "current-task cleared" "$_tp/Context/current-task.md" "No active task"
assert_contains "output confirms completed" "$out_done" "completed"
assert_file_exists "note: learning A" "$_tp/Memory/note/learning-a.md"
assert_file_exists "note: learning B" "$_tp/Memory/note/learning-b.md"

rm -f "$_tp/Context/current-task.md"
( cmd_done "" 2>/dev/null ); rc_notask=$?
assert_exit_nonzero "done without current-task -> exit != 0" "$rc_notask"

# --- Suite 9: cmd_context ---
suite "cmd_context"
( cmd_task "Context Task" 2>/dev/null )
out_ctx="$(cmd_context 2>&1)"
assert_contains "context shows project"  "$out_ctx" "testproject"
assert_contains "context shows Patterns" "$out_ctx" "Patterns"
assert_contains "context shows path"     "$out_ctx" "$TMPDIR_BRAIN"

detect_project() { echo "brand-new-project"; }
out_ctx2="$(cmd_context 2>&1)"
assert_contains "new project: auto-init hint" "$out_ctx2" "automatically"
detect_project() { echo "testproject"; }

# --- Suite 10: subprocess dispatch ---
suite "subprocess (dispatch)"
out_path="$(bash "$BRAIN_SCRIPT" path "my-project" 2>&1)" || true
assert_contains "path includes name" "$out_path" "my-project"
out_help="$(bash "$BRAIN_SCRIPT" unknown-cmd-xyz 2>&1)"
assert_contains "unknown shows help" "$out_help" "brain"
assert_contains "help lists init"     "$out_help" "init"
assert_contains "help lists remember" "$out_help" "remember"
assert_contains "help lists task"     "$out_help" "task"
assert_contains "help lists done"     "$out_help" "done"

# --- Suite 11: cmd_remember auto-creates target dir ---
suite "cmd_remember auto-creates dir"
detect_project() { echo "proj-no-dirs"; }
( cmd_remember "pattern" "Auto Dir" "no pre-created dir" 2>/dev/null )
assert_file_exists "remember creates Memory/pattern itself" \
    "$TMPDIR_BRAIN/proj-no-dirs/Memory/pattern/auto-dir.md"
detect_project() { echo "testproject"; }

# --- Suite 12: global preferences ---
suite "preference (global)"
out_pref="$(cmd_remember "preference" "Use Nvim" "preferred editor is nvim" 2>&1)"
assert_file_exists "preference goes to _global" \
    "$TMPDIR_BRAIN/_global/preferences/use-nvim.md"
assert_file_contains "preference has content" \
    "$TMPDIR_BRAIN/_global/preferences/use-nvim.md" "nvim"
assert_file_contains "preference marked global" \
    "$TMPDIR_BRAIN/_global/preferences/use-nvim.md" "(global)"
out_gmd="$(emit_global_md 2>&1)"
assert_contains "emit_global_md lists preference" "$out_gmd" "nvim"

# --- Suite 13: context-md + auto-init ---
suite "context-md + auto-init"
detect_project() { echo "proj-autoinit"; }
out_cmd_md="$(cmd_context_md 2>&1)"
assert_contains "context-md has header"           "$out_cmd_md" "Mega Brain"
assert_contains "context-md includes global pref" "$out_cmd_md" "nvim"
assert_dir_exists "auto-init created structure"   "$TMPDIR_BRAIN/proj-autoinit/Context"
assert_file_exists "auto-init created INDEX"      "$TMPDIR_BRAIN/proj-autoinit/INDEX.md"
detect_project() { echo "testproject"; }

# --- Suite 14: hook-session-end ---
suite "hook-session-end"
detect_project() { echo "proj-sessend"; }
( cmd_hook_session_end "claude" 2>/dev/null )
assert_file_contains "session-end records timeline" \
    "$TMPDIR_BRAIN/proj-sessend/Logs/timeline.md" "session ended (claude)"
detect_project() { echo "testproject"; }

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
