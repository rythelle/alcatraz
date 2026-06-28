# Agent Instructions - Mega Brain

## Persistent, dynamic context

This environment has Mega Brain, which keeps memory across sessions. The `mega-brain`
command (alias: `brain`) is on the PATH and manages all project memory.

## Auto-load - don't run load

This project's context is **injected automatically** at session start by a `SessionStart`
hook (matcher `startup|resume`). Read and internalize it before acting. New projects are
auto-initialized. To re-inspect manually: `mega-brain load`.

## Auto-save - save without being asked

```bash
# User preference -> GLOBAL partition (applies to all projects)
mega-brain remember preference "name" "content"

# Current project memory
mega-brain remember pattern  "name" "description"     # reusable pattern
mega-brain remember decision "name" "decision and rationale"
mega-brain remember gotcha   "name" "problem and fix"
mega-brain remember note     "name" "content"
```

Start a task: `mega-brain task "task-name"`.

Finish a task (moves to done, records the timeline, creates memories):

```bash
mega-brain done "pattern X worked well; avoid Y in production"
```

## Do NOT use native memory

Do not rely on any internal model memory. All persistence goes to Mega Brain.

## Expected flow

1. Read the already-injected context (don't run load).
2. Work on the task.
3. `mega-brain remember` whenever you discover something relevant (without being asked).
4. `mega-brain done "learnings"` when finished.

## Helpers

```bash
mega-brain path         # project path in the vault
mega-brain global-path  # global partition path
mega-brain project      # detected project name
```

## Constraints

- Don't edit `~/.ai-context/` directly without `mega-brain` - it keeps INDEX.md and links consistent.
- Use `mega-brain path` to find paths; don't ask where files live.
- Don't produce context markdown for the user to copy - write it directly with `mega-brain remember`.
