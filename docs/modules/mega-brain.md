*[English](mega-brain.md) · [Português](../pt-BR/modules/mega-brain.md)*

# Mega Brain: per-project persistent memory

> **Optional module (opt-in, off by default).** Enable it with
> `ALCATRAZ_MOD_MEGABRAIN=on` in `.env`, or from the TUI **Modules** screen, then
> run `alcatraz run`. While it's off, no memory hooks are wired into any AI CLI
> and nothing is written to the vault.

Persistent, per-project memory that lives on the host and syncs across sessions and
models. Start a session and the context gets injected automatically. End it and the
learnings get saved automatically. There's no manual `load` or `save` to remember.

**Where memory lives** is controlled by `AI_CONTEXT_PATH` in `.env`, and defaults to
`./.ai-context`. Point it at an Obsidian or OneDrive vault to sync across machines:

```bash
# .env
AI_CONTEXT_PATH=/mnt/c/Users/youruser/OneDrive/Documents/AIContext
```

**Available commands** (run these inside the container, or through `exec`):

```bash
mega-brain load                                # inspect the current context
mega-brain task "name"                         # set the active task
mega-brain remember pattern "name"             # save a code/design pattern
mega-brain remember decision "name"            # save an architectural decision
mega-brain remember gotcha "name"              # save a gotcha or pitfall
mega-brain remember note "name"                # save a general note
mega-brain remember preference "name"          # save a preference (writes to global partition)
mega-brain done ["learning 1; learning 2"]     # finish a task and save learnings
mega-brain search "term"                       # grep the vault (project + global) on demand
mega-brain handoff "summary; next steps"       # end-of-session handoff (injected next session)
mega-brain pause "what's in progress"          # pause the active task mid-flight, keeping full context
mega-brain resume                              # reload a paused task and clear the PAUSED mark
mega-brain context                             # quick summary
```

**Pause instead of `--resume`.** `mega-brain pause` snapshots an in-progress task. It
marks the task `PAUSED` and writes `Context/last-session.md`, which gets injected at
the next session start, so you can stop mid-task and come back later with any model,
without relying on a tool's native `--resume`. `mega-brain resume` prints the saved
context and un-pauses the task.

**Autosave backstop.** The container's main process is a Mega Brain supervisor that
snapshots every mounted project automatically. On a graceful stop it traps `SIGTERM`
and runs `mega-brain pause-all` before exiting, and it also autosaves on a timer
(`MEGABRAIN_AUTOSAVE_SECS`, 300 seconds by default), so even a sudden `docker kill` or
an OOM loses at most that window. A `PreCompact` hook in Claude Code adds a third
checkpoint right before a long conversation gets compacted.

**Dynamic per-project context.** In a shell, the current project's context loads by
itself the first time you enter its `/workspace/projects/<name>` directory, and again
whenever you `cd` into a different project. No reload, no manual `mega-brain load`.
Turn it off with `ALCATRAZ_NO_AUTOLOAD=1`.

At session start only an **index** of memories gets injected, one title and one line
each. The AI reads the full memory files on demand, which keeps the context window
lean. Saving an existing memory again appends a dated `## Update` section rather than
refusing.

**Compaction backstop (Claude Code).** Right before Claude compresses a long
conversation, a `PreCompact` hook snapshots a digest of the recent messages into the
vault, at `Context/last-session.md`. If the session dies after compaction without a
proper handoff, the next one still starts out knowing what was going on.

Memory is per-project, routed by git repo name. The `preference` type is the exception:
it writes to a global partition that applies across all projects.

## Related `.env` settings

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `AI_CONTEXT_PATH` | `./.ai-context` | Host path for the memory vault. Point it at an Obsidian or OneDrive folder to sync across machines. |
| `MEGABRAIN_AUTOSAVE_SECS` | `300` | Periodic-autosave interval in seconds. `0` disables the timer, though the SIGTERM snapshot on graceful stop still runs. |
| `MEGABRAIN_GROUP_PREFIX` | (none) | Repos whose name starts with this prefix are grouped into a vault subfolder. Set it together with `MEGABRAIN_GROUP_DIR`. |
| `MEGABRAIN_GROUP_DIR` | (none) | Vault subfolder name used when a repo matches the prefix. For example, prefix `acme-` and dir `Acme` gives `{vault}/Acme/acme-web`. |
