*[English](spawn.md) · [Português](../pt-BR/modules/spawn.md)*

# spawn: disposable sibling sandboxes

> **Optional module (opt-in, off by default).** Enable it with
> `ALCATRAZ_MOD_SPAWN=on` in `.env`, or from the TUI **Modules** screen, then run
> `alcatraz run`. While it's off, `alcatraz spawn` and `spawn-watch` are hidden from
> `--help` and print a notice if you call them, and the in-shell `spawn` shim
> refuses.

Long agent sessions burn most of their context window on exploration: reading big
files, chasing call chains, trying approaches that get thrown away. `spawn` isolates
that noisy work in a throwaway sibling of the sandbox, runs one task
non-interactively, and returns only the conclusion. The main session, human or agent,
stays lean.

```bash
alcatraz spawn "Trace how auth tokens flow from login to the DB and summarize"
alcatraz spawn --agent codex "Find every call site of processPayment and list edge cases"
alcatraz spawn -a gemini -m gemini-2.5-pro "Audit this module for N+1 queries"
```

A spawn is a full sibling container, not a `docker exec` into the running sandbox, so
it gets the same protections:

- **Same egress control.** It joins the same internal network and reaches the outside
  world only through Guard (secret redaction) and then Lighthouse (domain whitelist).
  Direct egress has nowhere to go.
- **Read-only project.** Your files are mounted read-only, so exploration can't mutate
  them. Auth and credential volumes are read-only too, so a throwaway run can never
  race the main session's logged-in state.
- **One task, then exit.** The task is injected into a non-interactive CLI run
  (`claude -p`, `codex exec`, `gemini -p`, `opencode run`), and the container is
  removed on exit.
- **Report, don't leak.** The full output goes to `<project>/.alcatraz/spawn-<id>.md`
  for the caller to read, instead of streaming back into the main context.
- **Mega-Brain-free.** A disposable exploration never writes to the memory vault or
  its timeline.

| Flag | Default | Meaning |
|---|---|---|
| `-a, --agent` | `claude` | AI CLI to run: `claude`, `codex`, `gemini`, `opencode` |
| `-m, --model` | (none) | Model override passed to the agent |
| `-p, --project` | active workspace | Project to explore (path or alias) |
| `--max` | `3` | Max concurrent spawns (they share the host) |
| `--keep` | off | Keep the container after exit, for debugging. Skips `--rm` |

You can also trigger a spawn from the **TUI**. The `🧬 Spawn` entry on the main menu
takes a task, lets you pick the agent, and streams the result inline. The entry only
shows up when the module is on.

The egress stack (`guard` and `lighthouse`) has to be up, so run `alcatraz run` if it
isn't. The interactive sandbox itself doesn't need to be running.

## Spawning from inside a shell: the bridge

`alcatraz spawn` talks to Docker, so it runs on the **host**. An agent (or you)
working *inside* `alcatraz shell` has no Docker access, which is the whole point of the
sandbox, so it can't launch a spawn directly. The **bridge** closes that gap without
ever handing the sandbox any Docker.

1. Inside the shell there's a `spawn` shim on the PATH. It doesn't touch Docker. It
   just drops a request file into `.alcatraz/requests/` and returns:

   ```bash
   # inside alcatraz shell, in your project
   spawn "trace how auth tokens flow to the DB"
   # ✓ queued spawn (claude) [a3f2c1] → read .alcatraz/results/a3f2c1.md
   ```

2. On the host, you run the watcher in a spare terminal. It services requests and asks
   you to approve each one, since the sandbox is treated as untrusted input:

   ```bash
   alcatraz spawn-watch            # add --auto to skip the per-request prompt
   # → spawn request from sandbox [a3f2c1]
   #     agent: claude
   #     task:  trace how auth tokens flow to the DB
   #   Approve? [y/N]
   ```

3. The result lands in `.alcatraz/results/<id>.md`, which you read back from inside the
   shell.

Because the sandbox may be prompt-injected, the watcher hardens every request. The
sandbox **never** gets Docker. Requests carry only a task string plus an agent from a
fixed allowlist, with no arbitrary flags or paths. Symlinked, oversized, malformed or
unknown-field requests are rejected. Nothing the sandbox controls steers a host path.
Every request is approved by default and appended to `.alcatraz/spawn-audit.log`. And
the watcher only ever spawns against the single project it's serving.

The same watcher serves the [websearch](websearch.md) module's requests, with each kind
behind its own module gate.
