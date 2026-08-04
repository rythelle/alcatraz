*[English](shakedown.md) · [Português](../pt-BR/modules/shakedown.md)*

# shakedown: command-output compression

> **Optional module (opt-in, off by default).** Enable it with
> `ALCATRAZ_MOD_SHAKEDOWN=on` in `.env`, or from the TUI **Modules** screen, then
> run `alcatraz run`. While it's off, the `shakedown` command inside the container
> prints a notice and refuses to run.
>
> **Renamed from `slim`.** The old `slim` command still works for one cycle: it
> prints a deprecation notice and then runs `shakedown`. Update your scripts to
> `shakedown`, since the alias goes away next cycle.

Test runners, builds and installs routinely dump thousands of lines into the model's
context window, and paying tokens for all of it is one of the biggest hidden costs of
an agent session. `shakedown` is available inside the container, and every model is
instructed to wrap noisy commands with it:

```bash
shakedown npm test          # prints head + tail + error/warning lines (~60 lines)
shakedown last              # full output of the previous run, on demand
```

The full log is always saved to `/tmp/shakedown-last.log`, so nothing is lost. The
model goes and reads it only when the summary isn't enough. Exit codes pass through
unchanged. One thing to avoid: don't wrap interactive commands, because output is
buffered and nothing streams live.

## Tuning (env vars)

| Variable | Default | Meaning |
| -------- | ------- | ------- |
| `SHAKEDOWN_THRESHOLD` | `60` | Pass output through untouched below this many lines |
| `SHAKEDOWN_HEAD` | `15` | Lines kept from the start |
| `SHAKEDOWN_TAIL` | `25` | Lines kept from the end |
| `SHAKEDOWN_LOG` | `/tmp/shakedown-last.log` | Where the full log is written |

The legacy `SLIM_*` variants are still honored as a fallback for one cycle.
