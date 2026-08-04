<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./logo.png">
    <source media="(prefers-color-scheme: light)" srcset="./logo-light.png">
    <img src="./logo-light.png" alt="Alcatraz" width="420">
  </picture>
</p>

<p align="center">
  <b>English</b> · <a href="README.pt-BR.md">Português</a>
</p>

# Alcatraz: an isolated sandbox for AI tools

Alcatraz runs AI coding agents (Claude Code, Gemini CLI, OpenAI Codex and opencode) inside a Docker container, so they can work on your code without having the run of your machine.

You can point it at a project in three ways: drop the code into `./project/`, pass a path directly (`alcatraz run ~/projects/my-app`), or save an alias once and use the short name from then on (`alcatraz save myapp ~/projects/my-app`, then `alcatraz run myapp`). After the first run it remembers the last project you used, so a bare `alcatraz run` picks up where you left off. However you start it, the project lands at `/workspace/projects/<folder-name>` inside the container, and anything you list in `PROJECT_PATHS` is mounted right next to it.

Everything the container sends out passes through two proxies. First comes **Guard**, a Go MITM proxy that strips secrets from the payload before any AI provider sees them. It does that reversibly: the model only ever gets an opaque token, and the real value is put back on the way home. Then comes **Lighthouse**, a Squid proxy that only lets through domains on an explicit list.

That trio (sandbox, Guard, Lighthouse) is the **core**, and it is always on. Everything else is a module you switch on or off from `.env` or the TUI: a safety net of checkpoints, sessions and stats that ships enabled, plus opt-in extras like [Mega Brain](docs/modules/mega-brain.md) memory, [shakedown](docs/modules/shakedown.md) output compression, [spawn](docs/modules/spawn.md) and [websearch](docs/modules/websearch.md). See [The core, and the modules around it](#the-core-and-the-modules-around-it).

---

## Motivations

AI coding agents are powerful, and by design they read your codebase, write files and call external APIs. That power comes with risks that are easy to overlook.

**Sensitive data leaks.** When an agent reads your project, it reads everything: `.env` files, configs, tokens, private keys, credentials. All of it goes into the prompt payload sent to the provider's API, verbatim. Alcatraz puts a MITM proxy (**Guard**) in front of every outbound request and redacts roughly 100 categories of secrets before they leave your machine: API keys, cloud credentials, PII, national IDs, SSH keys, database URLs. It catches them even when the data is base64 or hex encoded, or split up by separators. And the redaction is reversible, so the value is swapped for an opaque token and restored on the response. The model never sees it, and your workflow doesn't break.

**Unlimited filesystem access.** With nothing in the way, an agent can read, write or delete anything your user account can touch. Alcatraz runs it inside a container with a read-only root filesystem. Only `/workspace`, your project, is writable, and only from inside the container.

**Supply chain attacks through package managers.** Compromised npm and PyPI packages have been used to exfiltrate environment variables and files via malicious `postinstall` scripts. Run `npm install` inside Alcatraz and the container can't read your home directory, can't touch the host filesystem, and can't make host-level syscalls like `ptrace` or `mount`. Its outbound traffic goes through a proxy allowlist, so the worst a compromised package can do is damage `/workspace`. Your host stays untouched.

**Unrestricted network access.** By default, a process on your machine can reach any host on the internet. Alcatraz puts the sandbox on an internal-only network with no route out, and forces every request through **Lighthouse** with an explicit allowlist: only the domains the tools actually need (the npm registry, Claude, Gemini, OpenAI, GitHub) are reachable, and everything else is denied. Since this is enforced at the network layer, it holds even against a process that ignores the proxy. See [Security layers](#security-layers).

What you get is a controlled environment where the agent can still do its job (read your code, install dependencies, call the AI provider) while its filesystem access stays confined to `/workspace` and its outbound traffic is filtered and scrubbed of secrets before it leaves your machine.

---

## The core, and the modules around it

Alcatraz is one idea, running an AI agent safely, plus optional features you turn on
when you want them. Everything falls into three layers.

**Core: always on, not toggleable.** This *is* Alcatraz:

- **Sandbox**, a read-only container where only `/workspace` is writable.
- **Lighthouse**, an internal-only network plus a domain allowlist.
- **Guard**, reversible secret redaction before anything reaches an AI provider.

**Safety net: on by default, toggleable.** Passive and protective. They ask nothing
of you and only save you: **checkpoints** (file undo), **sessions** (resume a
conversation), **stats** (token and cost report).

**Optional modules: off by default, toggleable.** Turn these on when you want the
capability: **[Mega Brain](docs/modules/mega-brain.md)** (per-project memory),
**[shakedown](docs/modules/shakedown.md)** (command-output compression),
**[spawn](docs/modules/spawn.md)** (disposable sibling sandboxes),
**[websearch](docs/modules/websearch.md)** (host-fetched web lookups).

Toggle them from the TUI **Modules** screen, the `alcatraz modules` command, or the
`.env` module block. All three edit the same source of truth:

```env
# --- Modules (core is always ON and does not appear here) ---
ALCATRAZ_MOD_CHECKPOINTS=on     # safety net (default on)
ALCATRAZ_MOD_SESSIONS=on        # safety net (default on)
ALCATRAZ_MOD_STATS=on           # safety net (default on)
ALCATRAZ_MOD_MEGABRAIN=off      # opt-in
ALCATRAZ_MOD_SHAKEDOWN=off      # opt-in
ALCATRAZ_MOD_SPAWN=off          # opt-in
ALCATRAZ_MOD_WEBSEARCH=off      # opt-in
```

An `ALCATRAZ_MOD_*` set in the environment overrides the `.env` line, which is handy
in CI. A module that's off disappears from `--help` and the TUI, and its command
prints a one-line "enable with …" notice instead of running.

```bash
alcatraz modules                # list every module and its state
alcatraz modules spawn on       # turn one on (writes .env; applies next run)
```

---

## Table of contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Interactive TUI](#interactive-tui)
- [Credentials](#credentials)
- [Guard](#guard)
- [Modules](#the-core-and-the-modules-around-it)
  - [Mega Brain](docs/modules/mega-brain.md) · [shakedown](docs/modules/shakedown.md) · [spawn](docs/modules/spawn.md) · [websearch](docs/modules/websearch.md)
- [Commands](#commands)
- [Configuration (.env)](#configuration-env)
- [Upgrading](#upgrading)
- [Technical reference](#technical-reference)
- [Roadmap / ideas](#roadmap--ideas)
- [Contributing](#contributing)

---

## Prerequisites

- Docker 20.10+
- Docker Compose V2, the plugin, **not** the standalone `docker-compose` V1
- Go 1.22+, used by `install.sh` to compile the CLI. The backend image is built inside Docker and needs no Go on the host.

```bash
# Ubuntu/Debian
sudo apt-get install -y docker.io docker-compose-plugin golang-go
sudo usermod -aG docker $USER && newgrp docker

# macOS
brew install docker docker-compose go

# Windows: install Docker Desktop, WSL2, and Go from https://go.dev/dl
```

> **Why V2?** Docker Compose V1 doesn't get along with Docker Engine 25+. It treats `cpus` as a string instead of a float, and it fails on `up --no-build` without an explicit `image:`. This project needs V2 (`docker compose` as a plugin).

---

## Installation

```bash
git clone https://github.com/youruser/alcatraz
cd alcatraz
./install.sh
source ~/.zshrc   # or ~/.bashrc
```

`install.sh` checks your dependencies (Docker, Go), compiles the Go CLI, creates a symlink at `~/.local/bin/alcatraz` and adds it to your PATH. After that, `alcatraz` works from anywhere.

To update later:

```bash
git -C ~/path/to/alcatraz pull && ~/path/to/alcatraz/install.sh
```

> **(Optional)** Mega Brain memory is an opt-in module (`ALCATRAZ_MOD_MEGABRAIN=on`). Once it's enabled you can point it at a custom vault path. See [Mega Brain](docs/modules/mega-brain.md):
> ```bash
> cp .env.example .env
> # edit AI_CONTEXT_PATH in .env
> ```

The first `alcatraz run` builds the Docker image automatically, which takes a few minutes. After that, starting the container takes seconds.

---

## Quick start

```bash
# Start with your project
alcatraz run ~/projects/my-app

# Open a shell inside the sandbox
alcatraz shell

# Run a command directly
alcatraz exec 'claude "refactor the auth module"'

# Stop when you're done
alcatraz stop
```

`alcatraz` is the main CLI. Call it with no arguments and you get an interactive TUI:

```bash
# Interactive TUI, the easiest way to get started
alcatraz
```

---

## Interactive TUI

The TUI is the easiest way to drive Alcatraz. Launch it anytime with `alcatraz` (no arguments). Navigate with the arrow keys or `j`/`k`, press `Enter` to select, `q` to quit.

**Screens (press the key to jump):**

| Screen | Key | What it does |
| --- | --- | --- |
| **Dashboard** | `d` | Menu of every operation (the default on startup) |
| **Run** | `r` | Start the sandbox with a project path or a saved alias |
| **Exec** | `e` | Run a one-off command inside the container without opening a shell |
| **Shell** | `s` | Open an interactive bash/zsh shell (starts the container if needed) |
| **Workspaces** | `w` | View and switch between projects. `s` opens a shell in one, with no container restart if it's already mounted (via `PROJECT_PATHS`, for instance) |
| **Status** | `t` | Check whether containers are running, plus the current workspace and resource usage |
| **Stats** | n/a | Token usage and cost per day and model, metered by Guard |
| **Sessions** | n/a | Resumable AI conversations per model. Press `1` to `4` to reopen one in a shell (it runs `claude --continue` and friends), or `s` for a plain shell |
| **Checkpoints** | n/a | Browse workspace snapshots and roll back in place. Type a `#` or hash, leave it empty for the latest, and confirm |
| **Logs** | `l` | Tail live logs from the `alcatraz`, `guard` or `squid` services |
| **Tests** | `x` | Run `test-guard` (Guard pattern tests) or `test-security` (the isolation suite) |
| **Guard** | `g` | Manage Guard rules: add, list, test or audit redactions. It's the TUI version of `alcatraz guard` |

**Common workflows:**

- **Start a project:** press `r`, type a path or alias, hit `Enter`
- **Run a command:** press `e`, type the command, hit `Enter`. No shell prompt, it just runs
- **Check status:** press `t` to see running containers and the current workspace
- **Manage Guard rules:** press `g` to add custom redactions, test patterns and view the mode
- **View logs:** press `l`, pick a service (alcatraz/guard/squid), and watch the live output
- **Save a project shortcut:** in the Run screen, type a path and press `Enter`. You'll be offered the chance to save it as an alias for next time
- **Rebuild the image:** Dashboard → **Rebuild & Run** → confirm. You need this after changing the Guard code or `Dockerfile.alcatraz`, for example to pick up new features like `stats`. Nothing is lost: credentials, sessions, caches and Mega Brain memory all live in volumes or host paths, and only `/tmp` gets cleared.

**Inside the Guard screen (press `g`):**

- `a` adds a new rule, literal or regex
- `l` lists your custom rules with the values masked
- `t` runs a piece of text through the live redaction engine
- `s` shows the current mode (balanced or strict)
- `m` toggles the mode between balanced and strict
- `d` deletes a custom rule
- `u` shows the audit of what's been redacted since startup
- `r` shows the reload status of `guard-rules.yml`

**Tips:**

- If the container isn't running, most screens will start it for you.
- All menu navigation works without a mouse.
- Press `?` on any screen for context-sensitive help, where it's available.

---

## Credentials

OAuth credentials live in named Docker volumes and survive between sessions. Authenticate once, then forget about it.

| Tool               | How auth works                                        |
| ------------------ | ----------------------------------------------------- |
| **Claude Code**    | OAuth: run `claude` inside the container once         |
| **Gemini CLI**     | OAuth: run `gemini auth` inside the container once    |
| **OpenAI / Codex** | API key: set `OPENAI_API_KEY` in `.env`               |
| **opencode**       | Provider key: set `ANTHROPIC_API_KEY` or similar in `.env` |

**First-time OAuth setup:**

```bash
alcatraz shell
# Inside the container:
claude        # opens a browser for OAuth (Claude Code)
gemini auth   # interactive auth flow (Gemini CLI)
exit
# Credentials persist across stop/run, so you only do this once
```

> **Note:** run `alcatraz clean && alcatraz run <project>` once after installing or updating, to create the home directory volume. After that, OAuth credentials survive `alcatraz stop`.

**API keys (OpenAI, opencode):**

```bash
# In .env:
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
```

---

## Guard

Every JSON request the AI tools send upstream passes through a MITM proxy that redacts
secrets and PII **before** they reach the provider. The built-in coverage is on by
default and needs no configuration. On top of that, a single file on the host lets you
add your own redactions, hide proprietary code and tune how sensitive it is.

**Built-in coverage (roughly 100 categories, always on):** API keys and tokens, cloud
credentials, SSH and PGP private keys, database URLs, emails, credit cards, and
national ID or document numbers. The high-value document and card patterns are
**checksum-validated**: Luhn for cards, Canada's SIN and IMEI; mod-11 for Brazil's
CPF, CNPJ and PIS and Portugal's NIF; mod-97 for IBAN; the Dutch 11-test for BSN; the
Spanish DNI control letter; and Verhoeff for India's Aadhaar. Only structurally valid
numbers get redacted, so random look-alikes are left alone.

**Anti-evasion (always on).** Detection also catches data rewritten to slip past the
literal patterns: base64 and hex encoding (including one nested layer), digits split
by separators or zero-width characters, full-width and non-ASCII digits, and reversed
digit strings. The same checksums still apply, so fake data still passes through. The
one thing a stateless proxy can't catch is a value split across *separate* requests.

**Reversible tokenization (on by default).** Rather than destroying a value with a
`[REDACTED]` marker, Guard swaps it for an opaque token and keeps the mapping in memory
**inside the container**. The provider only ever sees the token. When the model echoes
it back, Guard restores the real value on the response path (gzip gets decompressed and
the text is reassembled across SSE deltas) before the AI CLI reads it. So redaction
stops breaking workflows that need the value, and nothing sensitive ever leaves the
box. Set `ALCATRAZ_VAULT=0` to fall back to destructive markers. The `allow` list is
unchanged, and it's still the opt-out for values that may legitimately leave in
cleartext.

### Configuration: `~/.alcatraz/guard-rules.yml`

Your custom rules live in `~/.alcatraz/guard-rules.yml`. The file is mounted
**read-only into the backend** and hot-reloaded within about a second of saving. It is
**never** mounted into the sandbox, so your list of secrets never travels with your
code. If the YAML fails to parse, the backend keeps the last valid version and logs the
problem, so protection is never dropped.

The file is created for you on first use. Edit it directly, or manage it from the CLI:

```bash
alcatraz guard add --name formula --literal "k = 1.4423" --replace "[FORMULA]"
alcatraz guard add --name acme --regex 'AcmeAlgo(V[0-9]+)?'
alcatraz guard list                 # show custom rules (values masked)
alcatraz guard mode strict          # balanced (default) | strict
alcatraz guard test "my CPF is 529.982.247-25"   # run text through the live engine
alcatraz guard status               # rule count, mode, backend reload state
alcatraz guard audit                # summary of what has been redacted (values never shown)
```

You can also use the Guard screen in the TUI: press `g` from the main menu.

**The file has four sections.**

**1. `redact`, your own rules** for hiding proprietary code, constants and internal names.

Each rule needs a `name` and exactly one of `literal` (an exact string match) or `regex` (Go RE2 syntax):

```yaml
redact:
  - name: proprietary-formula
    literal: "correction_factor = 1.4423"
    replace: "[PROPRIETARY_FORMULA]"
  
  - name: acme-algorithm
    regex: 'AcmeAlgo(V[0-9]+)?'
    replace: "[INTERNAL_ALGORITHM]"
  
  - name: customer-ids
    regex: 'customer_id["\']?\s*[:=]\s*["\']?([A-Z]{3}[0-9]{6})'
    replace: "[CUSTOMER_ID]"
```

Leave out `replace` and it defaults to `[REDACTED_BY_ALCATRAZ_CUSTOM]`.

**2. `allow`, values that should never be redacted.**

Handy for structurally valid fake data in test fixtures that would otherwise trip a built-in pattern:

```yaml
allow:
  - "111.444.777-35"              # fake Brazilian CPF (would trip the built-in pattern)
  - "4111 1111 1111 1111"         # fake credit card (would trip the Luhn validator)
  - "test.user@example.com"       # test fixture email to never redact
```

Only exact matches work here. No regex.

**3. `markers`, for hiding code inline** so it stays in the sandbox but never reaches the provider.

Wrap a block of code between the start and end markers. It gets replaced before the payload goes upstream, but still runs normally inside the container:

```yaml
markers:
  enabled: true
  start: "alcatraz:hide-start"
  end: "alcatraz:hide-end"
  replace: "[CODE_HIDDEN_BY_ALCATRAZ]"
```

In your code, using any comment style:

```python
# alcatraz:hide-start
SECRET_API_KEY = "sk-super-secret-key-12345"
INTERNAL_FORMULA = 42 * 1.4423
# alcatraz:hide-end
result = INTERNAL_FORMULA * input_value
```

The block shows up as `[CODE_HIDDEN_BY_ALCATRAZ]` in the prompt sent to the AI provider, but runs normally in the sandbox. **An unclosed start marker fails closed**, meaning everything from the marker to the end of the value gets redacted.

**4. `mode`, the sensitivity level.**

- `balanced` (the default) redacts well-structured secrets such as API keys and checksum-valid cards and documents, plus the anti-evasion transforms
- `strict` also redacts context-free look-alikes such as bare SSNs, Mercosul plates and hyphenated CEPs, at the cost of more false positives

```yaml
mode: balanced
```

Change it from the CLI with `alcatraz guard mode strict`, or press `m` in the TUI Guard screen.

### A full `~/.alcatraz/guard-rules.yml`

```yaml
redact:
  - name: datadog-key
    regex: 'dd_trace_id["\']?\s*[:=]\s*["\']?[a-f0-9]{32}'
    replace: "[DATADOG_KEY]"
  
  - name: internal-endpoint
    literal: "https://internal.acme.local/api"
    replace: "[INTERNAL_ENDPOINT]"

allow:
  - "529.982.247-25"  # fake CPF for testing

markers:
  enabled: true
  start: "alcatraz:hide-start"
  end: "alcatraz:hide-end"
  replace: "[CODE_HIDDEN]"

mode: balanced
```

### What Guard does and doesn't do

**✅ What it covers:**
- JSON request bodies, meaning the prompts and payloads sent to AI providers
- Every proxied host (Claude, Gemini, OpenAI, GitHub and the rest)
- Inline code markers, custom rules, and the anti-evasion transforms (base64, hex, split, reversed)
- Provider responses, but only to **restore** vault tokens. They are never redacted

**❌ What it doesn't cover:**
- Request and response headers, so auth headers like `x-api-key` are never broken
- Non-JSON uploads such as npm tarballs and binary blobs
- Files or code you run entirely inside the sandbox
- A value split across *separate* requests, since a stateless proxy has nothing to correlate

Guard is a best-effort content filter, not a hard egress control. See [Security layers](#security-layers) for the full defense-in-depth picture.

> **Tip:** run `alcatraz guard test "your text here"` to check whether something will be redacted before it reaches the provider.

### Token usage report (`alcatraz stats`)

Guard sits on the wire between every AI CLI and its provider, so it also meters token
usage. No cooperation from the CLIs is needed, and it works the same for Claude Code,
Gemini, Codex and opencode:

```bash
alcatraz stats
```

```
DATE        MODEL                  REQS       INPUT      OUTPUT  CACHE READ  CACHE WRITE
2026-07-04  claude-sonnet-4-5        42       81.3k       12.4k        1.2M        18.9k
2026-07-04  gemini-2.5-flash          5        10.2k        3.1k          0            0
TOTAL                                47       91.5k       15.5k        1.2M        18.9k
```

The `usage` fields from each response (in Anthropic, OpenAI or Gemini format) get
recorded to `stats.jsonl` in the audit volume and aggregated per day and model. **Only
token counts are reported.** They're parsed verbatim from the provider's own response,
so they're exact. There's deliberately no dollar figure: only the provider can price a
request accurately, given live pricing, volume and batch discounts, and per-provider
cache multipliers. Response bodies are scanned in transit and never stored.

---

## Optional modules

These are **off by default**. Turn one on from the TUI **Modules** screen, with
`alcatraz modules <name> on`, or in the `.env` module block, then run `alcatraz run`.
Each has its own page with the full story:

- **[Mega Brain](docs/modules/mega-brain.md)** gives you persistent, per-project memory
  that loads at session start and saves at session end, across sessions and models.
  `mega-brain pause` and `resume` let you stop a task mid-flight and pick it up later
  without a tool's native `--resume`. Vault path: `AI_CONTEXT_PATH`. Enable with
  `ALCATRAZ_MOD_MEGABRAIN=on`.

- **[shakedown](docs/modules/shakedown.md)** wraps noisy commands (`shakedown npm test`)
  and keeps only the head, the tail and the error and warning lines, saving the full log
  for on-demand recall. Builds and test runs stop burning the model's context window.
  Enable with `ALCATRAZ_MOD_SHAKEDOWN=on`.

- **[spawn](docs/modules/spawn.md)** offloads noisy exploration to a throwaway sibling
  sandbox (read-only project, same Guard and Lighthouse egress), runs one task
  non-interactively, and returns only the conclusion so the main session stays lean.
  Enable with `ALCATRAZ_MOD_SPAWN=on`.

- **[websearch](docs/modules/websearch.md)** lets the sandbox ask the *host* for one web
  search and prints the hits right in the shell, without putting a single search engine
  on the Lighthouse allowlist. The host runs no agent for it: an approved request is
  exactly one https GET. Every query is validated, checked by Guard, approved by a human
  and logged. Enable with `ALCATRAZ_MOD_WEBSEARCH=on`.

---

## Commands

Everything goes through `alcatraz`. The older `./alcatraz.sh` script still works as a fallback, but the CLI is the interface to use.

```bash
alcatraz                          # Interactive TUI
alcatraz run [PATH|ALIAS]         # Start sandbox, mount PATH or saved alias (auto-saves as favorite)
alcatraz run --rebuild            # Start with a forced image rebuild
alcatraz shell [PATH|ALIAS]       # Open a shell (starts, or restarts to mount the project, if needed)
alcatraz exec 'COMMAND'           # Run a one-off command in the container
alcatraz modules [NAME on|off]    # List/toggle optional modules (edits .env)
alcatraz spawn "<task>"           # [module] Run a task in a disposable sibling sandbox
alcatraz spawn-watch              # [module] Serve in-shell spawn/websearch requests from the host
alcatraz stop                     # Stop all containers
alcatraz status                   # Show running status and current workspace
alcatraz stats                    # [module] Token usage/cost report metered by the Guard
alcatraz sessions                 # [module] List resumable AI sessions per model
alcatraz logs [SERVICE]           # Tail logs: alcatraz (default), guard, squid
alcatraz save NAME [PATH]         # Save a workspace alias
alcatraz list                     # List saved aliases
alcatraz remove NAME              # Remove a saved alias
alcatraz clean                    # Stop + delete containers and volumes (destructive)
alcatraz checkpoint [MSG]         # [module] Snapshot the workspace (auto on run/exec)
alcatraz checkpoints              # [module] List workspace checkpoints
alcatraz rollback [N|HASH]        # [module] Restore workspace files to a checkpoint
alcatraz resources                # Live Docker resource stats
alcatraz guard ...                # Manage Guard rules (add/list/mode/test/status/audit)
alcatraz test-guard               # Run Guard sanitizer tests
alcatraz test-security            # Run full security isolation test suite
```

Commands marked `[module]` belong to an optional module. When the module is off they're
hidden from `--help` and print an "enable with `ALCATRAZ_MOD_…=on`" notice instead of
running. See [Modules](#the-core-and-the-modules-around-it).

The `guard` subcommand manages `~/.alcatraz/guard-rules.yml`. See [Guard](#guard):

```bash
alcatraz guard add --name <n> --literal <value>   # or --regex <pattern>
alcatraz guard list                               # custom rules (masked)
alcatraz guard mode balanced|strict               # show or set sensitivity
alcatraz guard test "<text>"                      # run text through the live engine
alcatraz guard status                             # rules, mode, reload state
alcatraz guard audit                              # what has been redacted
```

**Workspace aliases** let you switch between projects without typing full paths:

```bash
alcatraz save api ~/projects/my-api
alcatraz save web ~/projects/my-web

alcatraz run api    # mounts ~/projects/my-api
alcatraz stop
alcatraz run web    # mounts ~/projects/my-web
```

### Workspace checkpoints: an undo button for your files

A checkpoint is a snapshot of your **project files** that you can roll back to. Think of it
as a sandbox-level undo. It works for **every** model, not just Claude's `/rewind`, and it
even reverses **bash side effects** that a chat-level rewind can't touch: a script that
deleted files, a refactor that went sideways, an overwrite.

**How it stays out of your way.** Every `alcatraz run` and `exec` takes a snapshot
automatically, respecting `.gitignore`, and stores it on a *shadow git ref*
(`refs/alcatraz/checkpoints`) inside your project's own repo. It never touches your
branches, your staged changes, your working index or your commit history. It's a parallel
timeline only Alcatraz can see. The one requirement is that the project be a git repo, so
run `git init` if it isn't.

**From the TUI**, open **Checkpoints** on the main menu. It lists your snapshots with 1
being the most recent, you type a number or a hash, and it rolls back right there.

**From the CLI:**

```bash
alcatraz checkpoints                 # list them: 1. 03824f8  2026-07-04 20:50  auto: session start
alcatraz rollback                    # restore the latest snapshot
alcatraz rollback 3                  # ...or the 3rd one (or a commit hash)
alcatraz checkpoint "pre-refactor"   # take a manual snapshot with a label
```

**Rolling back is itself reversible.** Before restoring, it snapshots the current state and
prints that hash, so if you went back too far, `alcatraz rollback <hash>` brings you
forward again. Turn off the automatic snapshots with `ALCATRAZ_AUTO_CHECKPOINT=0`.

> **Checkpoints vs sessions.** They undo different things. A **checkpoint** restores your
> **files**, what's on disk. A **session** (below) restores an AI **conversation**, the
> model's context. They complement each other: you can reopen yesterday's conversation
> *and* roll the files back to a known-good point.

**`PROJECT_PATHS`** goes in `.env` and mounts extra projects alongside the active one. Every project, the one you started with `alcatraz run` and each path in `PROJECT_PATHS`, shows up at `/workspace/projects/<folder-name>` inside the container:

```bash
# .env
PROJECT_PATHS=/home/you/projects/api,/home/you/projects/web
# Inside the container:
#   /workspace/projects/my-app   ← active project (from alcatraz run)
#   /workspace/projects/api      ← from PROJECT_PATHS
#   /workspace/projects/web      ← from PROJECT_PATHS
```

**What survives `stop` and `run` cycles:**

| Data                        | Persists | Storage                          |
| --------------------------- | -------- | -------------------------------- |
| Your project files          | Yes      | Host bind mount (`/workspace/projects/<name>`) |
| Claude / opencode auth      | Yes      | Named volumes                    |
| AI sessions / chat history  | Yes      | Named volumes (`~/.claude`, `~/.codex`, `~/.gemini`, opencode state) |
| npm cache                   | Yes      | Named volumes                    |
| Mega Brain memory           | Yes      | Host path (`AI_CONTEXT_PATH`)    |
| `/tmp`, shell history       | No       | tmpfs, cleared on stop           |

> `alcatraz clean` removes everything, named volumes included. Use it only when you want a full reset.

### Resuming sessions: continue an AI conversation

A *session* is a saved AI conversation you can pick up where you left off. Each CLI stores
its history in a named volume, so sessions survive `stop` and `run` cycles and even an
image rebuild. Only `alcatraz clean` wipes them.

**What the Sessions screen is.** Open **Sessions** on the main menu and you get one
**navigable list, newest first**. Only tools that actually have saved sessions show up, so
there's no wall of empty rows. Each line is a resumable session:

```
   TOOL       PROJECT / TAG               LAST USED         ID
▶  Claude     retro-job-hub               2026-07-05 02:33  a1b2c3d4
   Claude     resume-adapter              2026-07-04 18:12  9f8e7d6c
   Codex      2 sessions (native picker)  2026-07-04 02:33
   opencode   1 session (latest)          2026-07-03 21:40

  ↑/↓ select • enter resume • s shell • r refresh • ESC back
```

Highlight one and press **enter**. Alcatraz opens a shell that resumes it and stays
interactive. Claude sessions are listed **individually**: each row carries its real project
directory and id, read from the session file, so enter runs `claude --resume <id>` in the
right project. Codex, Gemini and opencode get one row each, which hands off to their native
picker or continues the latest.

**Choosing *which* session from the CLI:**

| CLI | Resume the latest | Pick a specific one |
|---|---|---|
| Claude Code | `claude --continue` | `claude --resume` (picker), or `claude --resume <id>` |
| Codex | `codex resume` (picker) | `codex resume`, then choose |
| Gemini CLI | n/a | save with `/chat save <tag>`, later `/chat resume <tag>` |
| opencode | `opencode --continue` | `opencode --session <id>` |

**Sessions are per-project.** A session belongs to the project directory it was created in,
and resuming acts on the *active* project. To reach another project's sessions, switch the
active project in **Workspaces** first, then open Sessions. In a shell you can also just
`cd` into the project and run `claude --resume` there.

### `alcatraz spawn`: disposable sibling sandboxes

`spawn` offloads noisy exploration, such as reading big files or chasing call chains, to a
throwaway sibling of the sandbox. It gets a read-only project and the same Guard and
Lighthouse egress, runs one task non-interactively, and returns only the conclusion so the
main session stays lean. An in-shell bridge lets an agent request one without ever getting
Docker access.

It's an **optional module, off by default**. Enable it with `ALCATRAZ_MOD_SPAWN=on`, or
from the TUI Modules screen. For the full story, the flags and the request bridge, see
**[docs/modules/spawn.md](docs/modules/spawn.md)**.

### `websearch`: searching the web from inside the jail

The sandbox has no route to the internet, and search engines are deliberately kept off the
allowlist. `websearch` doesn't change that. It asks the **host** to run one lookup and
prints the hits back in the shell, so an agent's findings land straight in its context.

```bash
# inside alcatraz shell
websearch "bun 1.2 breaking changes"
```

On the host, `alcatraz spawn-watch` (alias `alcatraz bridge`) serves the request. No agent
and no shell runs for a search: an approved request is exactly one https GET. Because the
query itself leaves the box, it has to look like search terms (one line, 256 characters or
fewer, no URLs or encoded blobs), it's piped through the Guard engine and refused outright
if it carries a secret, a human approves every single one, and it's rate limited and
logged.

It's an **optional module, off by default**. Enable it with `ALCATRAZ_MOD_WEBSEARCH=on`.
For the full story and the threat model, see
**[docs/modules/websearch.md](docs/modules/websearch.md)**.

---

## Configuration (`.env`)

Copy `.env.example` to `.env` and adjust it. Docker Compose reads `.env` automatically at startup.

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `OPENAI_API_KEY` | (none) | API key for OpenAI / Codex. Injected into the container at runtime. |
| `ANTHROPIC_API_KEY` | (none) | API key for opencode (Anthropic backend) or other tools that read this var. |
| `GOOGLE_API_KEY` | (none) | API key for Google / Gemini (API key auth, an alternative to OAuth). |
| `AI_CONTEXT_PATH` | `./.ai-context` | Host path for the Mega Brain memory vault. Point it at an Obsidian or OneDrive folder to sync across machines. |
| `PROJECT_PATHS` | (none) | Comma-separated list of extra project paths to mount alongside the active workspace. Each one appears at `/workspace/projects/<folder-name>` inside the container. |
| `ALCATRAZ_VAULT` | `1` | Guard reversible tokenization. `1` means redactions become opaque tokens restored on the response path; `0` means destructive `[REDACTED]` markers. |
| `MEGABRAIN_AUTOSAVE_SECS` | `300` | Mega Brain periodic-autosave interval in seconds. `0` disables the timer, though the SIGTERM snapshot on graceful stop still runs. |
| `NODE_VERSION` | `22.19` | Node.js version pre-installed in the container via NVM. Change it before rebuilding (`alcatraz run --rebuild`). |
| `MEGABRAIN_GROUP_PREFIX` | (none) | Optional. Repos whose name starts with this prefix are grouped into a vault subfolder. Set it together with `MEGABRAIN_GROUP_DIR`. |
| `MEGABRAIN_GROUP_DIR` | (none) | Vault subfolder name used when a repo matches `MEGABRAIN_GROUP_PREFIX`. For example, prefix `acme-` and dir `Acme` gives `{vault}/Acme/acme-web`. |
| `COMPOSE_PROJECT_NAME` | `alcatraz` | Docker Compose project name. Controls the container name prefix. Change it only if you run multiple Alcatraz instances. |
| `ALCATRAZ_MOD_CHECKPOINTS` | `on` | Safety-net module: workspace file undo (checkpoints and rollback). |
| `ALCATRAZ_MOD_SESSIONS` | `on` | Safety-net module: list and resume AI conversations. |
| `ALCATRAZ_MOD_STATS` | `on` | Safety-net module: token and cost report metered by Guard. |
| `ALCATRAZ_MOD_MEGABRAIN` | `off` | Opt-in module: per-project persistent memory. See [docs/modules/mega-brain.md](docs/modules/mega-brain.md). |
| `ALCATRAZ_MOD_SHAKEDOWN` | `off` | Opt-in module: command-output compression (formerly `slim`). See [docs/modules/shakedown.md](docs/modules/shakedown.md). |
| `ALCATRAZ_MOD_SPAWN` | `off` | Opt-in module: disposable sibling sandboxes. See [docs/modules/spawn.md](docs/modules/spawn.md). |
| `ALCATRAZ_MOD_WEBSEARCH` | `off` | Opt-in module: web lookups fetched by the host. See [docs/modules/websearch.md](docs/modules/websearch.md). |
| `ALCATRAZ_SEARCH_PROVIDER` | auto | websearch, host side: `ddg` (keyless), `brave` or `searxng`. |
| `BRAVE_SEARCH_API_KEY` | (none) | websearch, host side: selects and authenticates the Brave provider. |
| `ALCATRAZ_SEARXNG_URL` | (none) | websearch, host side: base URL of a SearXNG instance returning JSON. |

The `ALCATRAZ_MOD_*` block is the single source of truth for module state, shared by the
CLI, the TUI Modules screen and `alcatraz.sh`. A value set in the environment overrides the
`.env` line. An existing install with no module block gets the defaults injected once, with
a notice, so nobody loses a feature silently.

**Example `.env`:**

```bash
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
AI_CONTEXT_PATH=/mnt/c/Users/youruser/OneDrive/Documents/AIContext
PROJECT_PATHS=/home/youruser/projects/api,/home/youruser/projects/shared-lib
```

---

## Upgrading

Pulling a new version takes three commands. The rebuild is the one people skip, and it's
the one that matters:

```bash
cp -r .ai-context ~/alcatraz-vault-backup   # cheap insurance (see below)
git pull
alcatraz run --rebuild                      # or -b
```

**`--rebuild` is not optional.** Parts of Alcatraz are baked into the image, notably
`alcatraz-helper`, the static binary the shims and Mega Brain hooks call. The scripts
themselves are bind-mounted and update the instant you `git pull`. Skip the rebuild and the
updated scripts go looking for a binary that isn't there yet. They fail loudly rather than
silently (`alcatraz-helper not found — rebuild the image`), but Mega Brain won't record
anything until you do.

### What survives, and what doesn't

Nothing in the update path touches your data. It's worth knowing *why*, so you can tell a
safe command from a destructive one:

| What | Where it lives | Survives |
|---|---|---|
| Mega Brain vault | host directory: `.ai-context/`, or `AI_CONTEXT_PATH` | `git pull`, rebuild, `down` |
| `.env` | host, git-ignored | `git pull`, rebuild, `down` |
| Agent logins (Claude, Gemini, Codex, opencode) | Docker named volumes | rebuild, `down` |
| Your projects | bind-mounted from the host | everything |

The vault and `.env` are in `.gitignore`, so `git pull` can't clobber them. The backup in
the snippet above is for the other scenario: cloning Alcatraz fresh somewhere else instead
of pulling. Then you copy `.ai-context/` across yourself, or point `AI_CONTEXT_PATH` at the
old location.

> **Never run `docker compose down -v`.** The `-v` deletes the named volumes and takes
> every agent login with it. Plain `down` is safe, and so is `alcatraz stop`.

### If something looks wrong afterwards

```bash
alcatraz status              # containers, modules, egress stack
alcatraz modules             # which modules resolved on/off, and from where
alcatraz logs proxy          # TCP_DENIED lines name the domain you're missing
ls .ai-context/              # the vault should be right where it was
```

---

## Technical reference

### Architecture

```
Host
 └── isolated-network (bridge 172.30.0.0/16)
      ├── lighthouse    (Squid, port 3128)
      ├── guard   (Go binary)
      │    └── :8080  Guard: MITM secret redaction
      └── alcatraz           (sandbox container)
           ├── /workspace/projects/<name>   <- active project, rw
           ├── /workspace/projects/<name>   <- PROJECT_PATHS entries, rw
           └── http_proxy -> guard:8080
```

The sandbox has **no route to the internet**. It lives on an `internal: true` Docker network. Its only path out is Guard, which scrubs secrets from JSON request bodies, and then Lighthouse, which blocks non-whitelisted domains. Lighthouse is the sole container bridged to the outside. Because the boundary is enforced at the network layer, it holds even against code that tries to ignore the proxy. See [Security layers](#security-layers) for details.

### Allowed domains

`github.com`, `githubusercontent.com`, `npmjs.com`, `npmjs.org`, `archive.ubuntu.com`, `security.ubuntu.com`, `claude.ai`, `claude.com`, `claudecode.com`, `claudeusercontent.com`, `anthropic.com`, `googleapis.com`, `openai.com`, `statsigapi.net`, `opencode.ai`, `sentry.io`, `models.dev`

Lighthouse (Squid) also explicitly blocks known DNS-over-HTTPS endpoints, to stop tools from resolving DNS out-of-band, along with the `PUT`, `DELETE` and `PATCH` methods on plain-HTTP requests. To add a domain, edit `squid.conf` and restart with `alcatraz stop && alcatraz run`. No rebuild is needed, since the config is bind-mounted.

### Security layers

| Layer        | Mechanism                                                                              |
| ------------ | -------------------------------------------------------------------------------------- |
| Network      | Sandbox on an **internal-only** Docker network with no route out. All egress forced through Guard and Lighthouse (allowlist). DoH blocked |
| Filesystem   | Root FS `read_only: true`. Only `/workspace` and a few tmpfs mounts are writable        |
| User         | Runs as `uid 1000` (`alcatraz_runner`), `no-new-privileges: true`                      |
| Capabilities | `cap_drop: ALL`, then adds back only `CHOWN`, `SETUID`, `SETGID`, `KILL`               |
| Syscalls     | **Default-deny** seccomp profile: only an explicit allowlist gets through. `ptrace`, `mount`, `BPF`, `io_uring`, `perf_event_open`, kernel modules, namespaces and so on are blocked |
| Resources    | 1.5 CPUs, 4 GB RAM, swap disabled, `pids_limit` 1024, 5-minute command timeout          |

> **How the network boundary works.** The sandbox and the backend sit on a Docker network
> marked `internal: true`, which has **no route to the internet**. Lighthouse is the only
> container attached to a second, external-facing network, so the *only* way out is
> sandbox → Guard (MITM redaction) → Lighthouse (domain allowlist). Ignoring the proxy
> with `curl --noproxy '*'` or a raw TCP socket to an IP has nowhere to route and fails,
> and raw packet sockets are blocked on top of that, since `NET_RAW` is dropped. This is
> enforced at the **network layer**, not merely via `http_proxy` environment variables, so
> it holds even against a process that deliberately tries to bypass the proxy.

### How Guard sanitization works

**Guard** is the MITM proxy. It intercepts every JSON request body the AI tools send
upstream and, before the payload reaches the provider, replaces each matching secret with
either a reversible vault token (the default) or a `[REDACTED_BY_ALCATRAZ_*]` marker (with
`ALCATRAZ_VAULT=0`). With tokens on, provider responses get processed on the way back too,
decompressed and reassembled across SSE deltas, so the original value can be **restored**.
Responses are never redacted.

**What is and isn't touched:**

| Touched                                           | Not touched                                                    |
| ------------------------------------------------- | -------------------------------------------------------------- |
| JSON request bodies (`Content-Type: *json*`)      | Request and response headers, so auth (`x-api-key`) is never broken |
| The prompt/conversation payload sent to the model | Non-JSON bodies (npm tarballs, binary downloads)               |
| Provider responses, only to restore vault tokens  | A value split across separate requests                         |

**Categories covered (roughly 100 patterns):**

- API keys and tokens: OpenAI, Anthropic, Google, GitHub, Slack, Discord, AWS, Stripe, JWT
- AI/LLM providers: Groq, Perplexity, Replicate, HuggingFace, OpenRouter, Cohere, Mistral
- Cloud credentials: AWS (account, ARN, session), Azure (subscription, tenant, secret), GCP (service account, OAuth), Cloudflare, Firebase, DigitalOcean, Terraform, Kubernetes
- PII (Brazil): CPF, CNPJ, PIS/NIS (all mod-11 checksum-validated), CNS/SUS, CNH, título eleitoral, RENAVAM, RG, CEP, phone, PIX, bank account, Mercosul and old vehicle plates
- National IDs (global, context-keyed plus checksum): Canada SIN, IMEI, Netherlands BSN, Portugal NIF, Spain DNI, India Aadhaar
- PII (global): email, credit card (Luhn plus issuer prefix), IBAN (mod-97), IP address, passport
- Cryptographic keys: SSH, PGP/GPG private keys
- Env vars: `*_SECRET`, `*_TOKEN`, `*_PASSWORD` patterns, SMTP and IMAP credentials
- Git, CI and packages: GitHub (all token formats), GitLab, npm tokens, Docker, Atlassian
- Email, SMS and monitoring: SendGrid, Mailgun, Twilio, Telegram, Sentry, New Relic

**Anti-evasion:** every string is also checked for values hidden by base64 or hex encoding
(one nested layer), separators or zero-width characters between digits, full-width and
non-ASCII digits, and reversed digit strings. All of it is still gated by the same
checksums.

**Adding your own redactions (no rebuild):** for project- or user-specific secrets, add a
rule to `~/.alcatraz/guard-rules.yml` with `alcatraz guard add`. It hot-reloads in about a
second and needs no code change. See [Guard](#guard).

**Adding a built-in pattern (for everyone):**

The shipped patterns are Go regexes (RE2, so linear-time with no catastrophic
backtracking) in [`platform/backend/internal/proxy/patterns.go`](platform/backend/internal/proxy/patterns.go).
Patterns whose shape isn't unique are gated by a checksum validator, so only valid
documents get redacted:

```go
// Fixed prefix (high precision):
{"my_service_key", re(`\bmysvc_[a-zA-Z0-9]{32}\b`), "[REDACTED_BY_ALCATRAZ_MYSVC]"},

// Context-gated (for secrets without a unique shape):
{"captcha_key", re(`(?i)(?:2captcha|capmonster)\s*[:=]\s*['"]?[a-zA-Z0-9]{20,}`), "[REDACTED_BY_ALCATRAZ_CAPTCHA]"},
```

Put specific rules above the generic catch-alls at the bottom of the file. Test before rebuilding:

```bash
cd platform/backend
go test ./internal/proxy/
go build ./internal/proxy/
```

Then rebuild with `alcatraz run --rebuild`.

### Customization

**Increase resources** by editing `docker-compose.go.yml`:

```yaml
cpus: 2.0       # float, no quotes
mem_limit: 8g
memswap_limit: 8g
```

**Add environment variables:**

```yaml
# docker-compose.go.yml
environment:
    - NODE_ENV=production
    - MY_VAR=value
```

**Mount an extra volume (read-only):**

```yaml
# docker-compose.go.yml
volumes:
    - /external/path:/workspace/data:ro
```

**Install additional tools** by adding `RUN` steps to `Dockerfile.alcatraz`, then running `alcatraz run --rebuild`.

### Swapping the language runtime

The sandbox deliberately ships **one** language runtime: Node. Nothing else, so no Python,
no JDK, no Rust toolchain. Every extra interpreter sitting in the image is another
ready-made tool for a compromised dependency or a prompt-injected agent to reach for, so
the image carries only what your project actually needs.

Which means that if you work in another stack, you change the image. It's three edits and a
rebuild.

**1. The runtime layer in `Dockerfile.alcatraz`.** It's marked with a banner comment,
`LANGUAGE RUNTIME LAYER` through `END LANGUAGE RUNTIME LAYER`. Add your stack inside it.
Java 21 and Maven, for example:

```dockerfile
# --- Add another stack here ---
USER root
RUN apt-get update && apt-get install -y --no-install-recommends \
        openjdk-21-jdk-headless maven \
    && rm -rf /var/lib/apt/lists/*
USER alcatraz_runner
ENV JAVA_HOME=/usr/lib/jvm/java-21-openjdk-amd64
```

Note the `USER root` and `USER alcatraz_runner` pair. Everything after the switch in the
Dockerfile runs as the unprivileged user, and it has to stay that way, because the
container itself never runs as root.

**Alcatraz itself doesn't care what you put here.** The layer is your development
environment, nothing more. The shims (`spawn`, `websearch`) and the Mega Brain hooks talk
to `alcatraz-helper`, a static, dependency-free binary compiled in a separate build stage
and installed *above* this layer, so replacing Node with a JDK breaks nothing in the
product.

The one thing you lose by removing Node is agents, because **the Gemini CLI and Codex are
npm packages**. Claude Code and opencode are standalone binaries and survive. For a Java
project you'd normally *add* the JDK and keep Node around for those two.

**2. The registry domains in `squid.conf`.** Lighthouse blocks everything not on the
allowlist, so a build that downloads dependencies fails until its registry is listed.
There's a marked block for exactly this:

```squid
acl allowed_domains dstdomain .repo.maven.apache.org
acl allowed_domains dstdomain .repo1.maven.org
```

Common ones: Java `repo.maven.apache.org`, `repo1.maven.org`, `plugins.gradle.org`,
`services.gradle.org` · Python `pypi.org`, `files.pythonhosted.org` · Rust
`crates.io`, `static.crates.io` · Go `proxy.golang.org`, `sum.golang.org` · .NET
`api.nuget.org` · PHP `repo.packagist.org`. Add only the ones you actually build against.
The allowlist is a security control, not a convenience list.

**3. The tool banner** (optional). The `init.sh` block at the end of
`Dockerfile.alcatraz` prints what's available at boot, so add a line for your stack and
both you and the agent can see it's there.

Then rebuild:

```bash
alcatraz run --rebuild
alcatraz shell
java -version    # inside the sandbox
```

If a build hangs or fails on a download, it's almost always the allowlist. Check with
`alcatraz logs proxy` and look for a `TCP_DENIED` line naming the host you forgot to add.

**Going Node-free.** To drop Node entirely, say for a pure Java or Go image, delete the
NVM/Node `RUN` block and the two `npm install -g` steps for the Gemini CLI and Codex, then
remove `.npmjs.com` and `.npmjs.org` from `squid.conf`. Alcatraz's own plumbing keeps
working, for the reason above: everything internal goes through `alcatraz-helper`, which
sits above the runtime layer and has no dependencies. The only real cost is losing the
Gemini CLI and Codex along with Node, while Claude Code and opencode keep working.

### Verify isolation

```bash
alcatraz shell
# Inside the container:
curl https://example.com          # fails: proxy denies the non-whitelisted domain
curl --noproxy '*' https://1.1.1.1   # also fails: no route out (internal network)
whoami                            # alcatraz_runner
id                                # uid=1000
touch /etc/test                   # fails: root filesystem is read-only

# Or run the full automated suite:
exit
alcatraz test-security
```

### Troubleshooting

**`'cpus' expected type 'float32', got unconvertible type 'string'`** means you're running Docker Compose V1. Install V2: `sudo apt-get install -y docker-compose-plugin`.

**`invalid service "alcatraz". Must specify either image or build`** means the image was built with an old compose version. Fix it with `docker tag alcatraz-alcatraz:latest alcatraz:latest && alcatraz run`.

**"Cannot connect to Docker daemon"**: run `sudo usermod -aG docker $USER && newgrp docker`.

**The container won't start**: check `alcatraz logs`, then try `alcatraz clean && alcatraz run`.

**A command exceeds the timeout**: raise it with `TIMEOUT_SECONDS=900 alcatraz exec 'long-command'`.

**Memory limit exceeded**: increase `mem_limit` in `docker-compose.go.yml`.

**opencode is slow to start.** Its TUI (opentui) probes the terminal on startup for cursor position, colour palette, Kitty keyboard support and bracketed paste, then waits for the replies. Terminals that answer promptly give you a UI in a second or two; ones that ignore some queries make it fall back to timeouts, and the first frame lands several seconds later. Alcatraz also sets `OPENCODE_DISABLE_AUTOUPDATE=1`, since the release check would otherwise travel through Guard and Lighthouse on every launch, and the sandbox can't self-update anyway with a read-only root and the binary baked into the image. The full TUI is the default and Enter works normally. If you'd rather have the line interface, with no alt-screen and no capability probing, run `ALCATRAZ_OPENCODE_MINI=1 opencode` or `opencode --mini`. Subcommands like `opencode run` and `opencode auth` are never modified. One tip: export a provider key (`ANTHROPIC_API_KEY`, say) on the host before `alcatraz run`, and opencode picks it up from the environment and skips the API-key prompt entirely.

---

## Roadmap / ideas

Ideas we'd like to see built but haven't gotten to. The project is open source, so feel free to pick one up and open a PR (see [Contributing](#contributing)), or open an issue to discuss an approach first.

### `mega-brain maintain`: scheduled vault maintenance

**Status:** an idea, up for grabs

Inspired by the "second brain" pattern of a vault that organizes itself overnight. The idea is a `mega-brain maintain` command that keeps the memory vault healthy without manual grooming, run periodically from cron inside the container, a host scheduler, or on container start.

What it might do:

- **Compress the timeline.** `Logs/timeline.md` grows forever, since every session end appends an entry. Archive entries older than N days into `Logs/archive/`, keeping a short summary
- **Archive stale tasks.** Move `Tasks/active/` entries untouched for weeks to `Tasks/backlog/`, or flag them in the injected context
- **Deduplicate memories.** Detect near-duplicate files across `Memory/*` (similar slugs or content) and suggest or perform merges
- **Prune empty boilerplate.** Remove memories that still contain only the `(describe here)` template with no real content
- **Rebuild INDEX.md.** Recount files and fix broken `[[links]]` after manual edits to the vault

Design notes: it has to be idempotent and safe to run unattended, so it should never delete content, only move, merge or archive. It should also print a short report of what it did, so the change history stays auditable in the timeline.

> **Shipped:** `alcatraz spawn`, disposable sibling sandboxes, used to live here as an idea
> and is now a real command. See [`alcatraz spawn`](#commands) under Commands.

## Contributing

Contributions are welcome. The project is deliberately focused, a sandbox for AI tools rather than a general-purpose container framework, so the best contributions stay within that scope.

**Good areas to contribute:**

- Anything in [Roadmap / ideas](#roadmap--ideas)
- New Guard patterns for secrets that aren't covered yet
- Support for additional AI tools (new CLI agents, new model providers)
- Improvements to Mega Brain (new memory types, better hook integration, new model support)
- Security hardening (tighter seccomp profiles, additional capability drops, network rules)
- Bug fixes and reliability improvements

**To add support for a new AI model in Mega Brain**, see `mega-brain/ADDING-NEW-MODEL.md`. The process is documented and designed to be straightforward.

**To contribute:**

1. Fork the repo and create a branch from `main`
2. Make your change with a clear commit message
3. If you're adding or modifying Guard patterns, include test cases in `platform/backend/internal/proxy/`
4. Open a pull request describing what the change does and why

There's no formal style guide, so follow the conventions in the files you're editing.
