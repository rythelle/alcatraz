# Agent Instructions - Mega Brain

## Persistent, dynamic context

This environment has Mega Brain, which keeps memory across sessions. The `mega-brain`
command (alias: `brain`) is on the PATH and manages all project memory.

## Auto-load

An opencode plugin tries to inject the project context automatically at session start.
If the context does not appear injected, load it yourself on your first action by running:

```bash
mega-brain context-md
```

Read and internalize the result before responding. New projects are auto-initialized.

## Auto-save - save without being asked

```bash
# User preference -> GLOBAL partition (applies to all projects)
mega-brain remember preference "name" "content"

# Current project memory
mega-brain remember pattern  "name" "description"
mega-brain remember decision "name" "decision and rationale"
mega-brain remember gotcha   "name" "problem and fix"
mega-brain remember note     "name" "content"
```

Start a task: `mega-brain task "name"`. Finish: `mega-brain done "learnings separated by ;"`.

## Do NOT use native memory

Do not rely on any internal model memory. All persistence goes to Mega Brain.

## Helpers

```bash
mega-brain path         # project path in the vault
mega-brain global-path  # global partition path
mega-brain project      # detected project name
```

## Constraints

- Don't edit `~/.ai-context/` directly without `mega-brain`.
- Don't produce context markdown for the user to copy - write it directly with `mega-brain remember`.
