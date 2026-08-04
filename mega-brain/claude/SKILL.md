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

If a memory already exists, `remember` with content **appends a dated update**
(`## Update <date>`) instead of refusing - use it to evolve knowledge over time.

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

## Recall on demand (keep context lean)

The injected context contains only an **index** of memories (title + one line).
When an indexed memory looks relevant, read the full file; to find something
not in view, search the vault:

```bash
cat "$(mega-brain path)/Memory/gotchas/migrations-lock.md"   # read a specific memory
mega-brain search "migration"                                # grep project + global vault
```

---

## Verbose commands (keep output lean)

Wrap commands that print a lot (test runners, builds, installs) with `slim` — it keeps
the head, the tail and the error/warning lines, and saves the full log for recall:

```bash
slim npm test
slim npm run build
slim last            # full output of the previous slim run, when you need it
```

Don't use it for interactive commands (output is buffered, nothing streams).

---

## End-of-session handoff

When the user says goodbye, asks to stop, or the work is wrapping up, persist a
handoff **before ending** (it is injected at the start of the next session):

```bash
mega-brain handoff "what changed; decisions made and why; tech debt; next steps"
```

A `PreCompact` hook also snapshots the recent conversation automatically right before
the context window is compacted — but that digest is a backstop, not a substitute for
a real handoff written by you.

---

## Do NOT use the model's native memory

Never use Claude's internal memory (the `#` shortcut, memory `CLAUDE.md`, memory tool).
**All** persistence goes to Mega Brain via `mega-brain remember` / `mega-brain done`.

---

## Helper commands

```bash
mega-brain context       # quick summary
mega-brain search <term> # grep the vault (project + global)
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
6. **Recall on demand** - the injected context is an index; `cat` the memory file or
   `mega-brain search <term>` before assuming something isn't known.
7. **Handoff before ending** - when the session is wrapping up, run
   `mega-brain handoff "..."` so the next session starts with full context.
8. **Wrap verbose commands with `slim`** (tests, builds, installs) - `slim last`
   replays the full output when the summary isn't enough.
