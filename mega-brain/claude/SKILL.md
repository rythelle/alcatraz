---
name: mega-brain
description: Persistent, dynamic, per-project memory in Alcatraz. Context is loaded automatically at session start (SessionStart hook) and saved automatically as you learn. Works with any AI via the `mega-brain` command.
trigger_commands:
  - /mega-brain
  - /remember
  - /task
  - /done
---

# Mega Brain - Persistent, Dynamic, Per-Project Memory

Each project's context is stored as markdown files in the user's vault
(`/home/alcatraz_runner/.ai-context/`), persisted on the host (and syncable with
Obsidian/OneDrive). All operations go through the `mega-brain` command (alias: `brain`),
available on the container PATH.

**Routing:** projects live at `~/.ai-context/{project}/`; user preferences live in the
global partition `~/.ai-context/_global/`. The project is detected from the git repo root
in `/workspace`. (Optional prefix grouping can be enabled via env; see the docs.)

---

## Auto-load (you do NOT need to run load)

This project's context was **already injected** at session start by a `SessionStart` hook.
Read and internalize it before responding. Only run `mega-brain load` manually if you want
to reload/inspect it. New projects are initialized automatically on first load.

---

## Auto-save (save without being asked)

Be proactive. Whenever something relevant comes up, save it **immediately**, without asking:

```bash
# User preference (applies to ALL projects -> global partition)
mega-brain remember preference "uses-tabs-not-spaces" "User prefers tabs, width 4."

# Reusable pattern, decision, or gotcha (project memory)
mega-brain remember pattern  "retry-with-backoff" "..."
mega-brain remember decision "postgres-vs-mongo" "..."
mega-brain remember gotcha   "migrations-lock" "ALTER TABLE on big tables locks."
mega-brain remember note     "any-note" "..."
```

When you **finish a task**, complete it automatically (moves it to done + records the
timeline + creates memories):

```bash
mega-brain done "learning 1; learning 2"
```

Start/resume a task:

```bash
mega-brain task "task-name"
```

---

## Do NOT use the model's native memory

Never use Claude's internal memory (the `#` shortcut, memory `CLAUDE.md`, memory tool).
**All** persistence goes to Mega Brain via `mega-brain remember` / `mega-brain done`.

---

## Helper commands

```bash
mega-brain context       # quick summary
mega-brain path          # project path in the vault
mega-brain global-path   # global partition path (preferences)
mega-brain project       # detected project name
```

---

## Rules

1. **Don't run load** - context is already injected by the hook; just read it.
2. **Always auto-save** - record preferences/patterns/decisions/gotchas as they appear and
   complete tasks with `mega-brain done` without being asked.
3. **Write directly** via `mega-brain remember` - never produce markdown for the user to copy/paste.
4. **User preferences -> `preference`** (global). Project knowledge -> `pattern`/`decision`/`gotcha`/`note`.
5. **One memory per file**, always timestamped (the command fills in the date).
