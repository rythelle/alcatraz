# Mega Brain - Persistent, Dynamic, Per-Project Memory

You have a persistent second brain (Mega Brain) that keeps context across sessions.
The `mega-brain` command (alias: `brain`) is on the PATH and manages all project memory.

## Auto-load - context comes pre-loaded

This project's context is **injected automatically** at session start by a `SessionStart`
hook. Do not run load manually - just read and internalize the context you received.
To re-inspect: `mega-brain load`. New projects are auto-initialized on first load.

## Auto-save - save without being asked

Be proactive and save immediately, without asking:

```bash
# User preference -> GLOBAL partition (applies to all projects)
mega-brain remember preference "name" "content"

# Current project memory
mega-brain remember pattern  "name" "content"
mega-brain remember decision "name" "content"
mega-brain remember gotcha   "name" "content"
mega-brain remember note     "name" "content"
```

When you finish a task, complete it automatically:

```bash
mega-brain done "learning 1; learning 2"
```

Start/resume a task: `mega-brain task "name"`.

## Do NOT use Gemini's native memory

Do not use the `save_memory` tool or `~/.gemini/GEMINI.md` to store memory (it is disabled).
All persistence goes to Mega Brain.

## Location and helpers

```bash
mega-brain path         # project path in the vault
mega-brain global-path  # global partition path
mega-brain project      # detected project name
```

Files live in `/home/alcatraz_runner/.ai-context/` (persisted on the host; syncable with Obsidian/OneDrive).

## When to use each command

| Situation | Command |
|---|---|
| Session start | (nothing - context already injected) |
| Learned a user preference | `mega-brain remember preference "name" "txt"` |
| Found a reusable pattern | `mega-brain remember pattern "name" "txt"` |
| Made an architectural decision | `mega-brain remember decision "name" "txt"` |
| Hit a gotcha | `mega-brain remember gotcha "name" "txt"` |
| Starting a feature/bug | `mega-brain task "name"` |
| Finished a task | `mega-brain done "learnings"` |

## Rules

- Don't run load - context is already injected by the hook.
- Always auto-save: record learnings and complete tasks without being asked.
- Write directly with `mega-brain remember` - don't produce markdown to copy/paste.
- Learnings in `mega-brain done` are separated by `;`.
