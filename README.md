<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./logo.png">
    <source media="(prefers-color-scheme: light)" srcset="./logo-light.png">
    <img src="./logo-light.png" alt="Alcatraz" width="420">
  </picture>
</p>

# Alcatraz - Isolated Sandbox for AI Tools

Docker containerization with strong isolation to run command-line AI agents - **Claude Code, Gemini CLI, OpenAI Codex, and opencode** - safely, **straight from the terminal**. Your project lives in `./project/` (host) and is mounted as `/workspace` in the container. All HTTP/HTTPS traffic goes through a Go MITM proxy (**Data Guardian** / Lighthouse) that sanitizes sensitive data before it reaches the AI providers, then through a Squid proxy that restricts access to a domain whitelist. Includes the **Mega Brain**: persistent, dynamic per-project memory, with auto-load and auto-save across sessions and models.

## Table of contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Credentials](#credentials)
- [Mega Brain (persistent memory)](#mega-brain)
- [Commands](#commands)
  - [Alcatraz CLI (Go binary)](#alcatraz-cli-go-binary)
- [AI tools](#ai-tools)
- [Project structure](#project-structure)
- [Architecture and security](#architecture-and-security)
- [Customization](#customization)
- [Troubleshooting](#troubleshooting)

---

## Prerequisites

- Docker 20.10+
- Docker Compose V2 (plugin) - **do not use V1 (`docker-compose` standalone)**

```bash
# Ubuntu/Debian (installs the V2 plugin)
sudo apt-get install -y docker.io docker-compose-plugin
sudo usermod -aG docker $USER && newgrp docker

# macOS
brew install docker docker-compose

# Windows - install Docker Desktop and use WSL2
```

> **Why V2?** Docker Compose V1 (standalone `docker-compose`) has incompatibilities with Docker Engine 25+ - for example, it treats `cpus` as a string instead of a float and fails on `up --no-build` without an explicit `image:` in the service. This project uses V2 (`docker compose` as a plugin) to avoid those issues.

---

## Installation

```bash
# 1. Make the scripts executable
chmod +x alcatraz.sh test-security.sh

# 2. (Optional) Configure the memory vault
cp .env.example .env
# By default memory lives in ./.ai-context (local). To sync with
# Obsidian/OneDrive, set AI_CONTEXT_PATH in .env.

# 3. Start, pointing at your project
./alcatraz.sh run /path/to/your/project
```

The first `run` builds the image automatically (may take a few minutes). After that it just restarts the container - a few seconds.

To recreate everything from scratch (volumes included):

```bash
./alcatraz.sh clean && ./alcatraz.sh run /path/to/your/project
```

### Memory vault configuration

The vault path is set via the `AI_CONTEXT_PATH` environment variable. docker-compose reads `.env` automatically. The default is the local `./.ai-context`; point it at your Obsidian/OneDrive vault to sync:

```bash
# .env
AI_CONTEXT_PATH=/mnt/c/Users/your-user/OneDrive/Documents/AIContext
```

Or export it before running:

```bash
export AI_CONTEXT_PATH=/mnt/c/Users/your-user/OneDrive/Documents/AIContext
./alcatraz.sh run /my/project
```

**Common paths:**

| Environment      | Path                                               |
| ---------------- | -------------------------------------------------- |
| Local (default)  | `./.ai-context`                                    |
| Windows via WSL2 | `/mnt/c/Users/{user}/OneDrive/Documents/AIContext` |
| Native Linux     | `/home/{user}/Documents/AIContext`                 |
| macOS            | `/Users/{user}/Documents/AIContext`                |

---

## Credentials

OAuth credentials are stored in **named volumes** and persist across sessions. Authenticate once inside the container - you won't need to do it again.

### How it works per tool

| Tool               | Storage                              | First-time setup                                      |
| ------------------ | ------------------------------------ | ------------------------------------------------------ |
| **Claude Code**    | Named volume `alcatraz-claude-data`  | Run `claude` inside container, complete OAuth          |
| **Gemini CLI**     | Named volume `alcatraz-gemini-data`  | Run `gemini auth` inside container                     |
| **OpenAI / Codex** | env var (passed at `exec` time)      | `export OPENAI_API_KEY` before `./alcatraz.sh run`     |
| **opencode**       | env var (passed at `exec` time)      | `export OPENCODE_API_KEY` or provider keys             |

### First-time setup (Claude & Gemini)

Authenticate inside the container on first run:

```bash
./alcatraz.sh shell
# Inside container:

# Claude Code - opens browser for OAuth
claude

# Gemini CLI - interactive auth flow
gemini auth

# opencode - manage providers/credentials
opencode providers

# Exit after auth completes
exit
# Next sessions: credentials persist in volume
```

### OpenAI / Codex (API key)

OpenAI and Codex use API keys, not OAuth. Add them to `.env` for persistent access:

```bash
# In .env (copied from .env.example):
OPENAI_API_KEY=sk-...
```

Then run normally:

```bash
./alcatraz.sh exec 'codex "add error handling"'
```

### opencode

Same as OpenAI - add to `.env`:

```bash
# In .env:
OPENCODE_API_KEY=sk-...
# or use a provider key:
ANTHROPIC_API_KEY=sk-...
OPENAI_API_KEY=sk-...
GOOGLE_API_KEY=...
```

---

## Mega Brain

Persistent, **dynamic**, per-project memory, stored on the host (and syncable with Obsidian/OneDrive if you point `AI_CONTEXT_PATH` at your vault). Context survives across sessions and models. Each model gets instructions in its native format, but they all share the same backend (`mega-brain`, alias `brain`).

**Truly dynamic:**

- **Auto-load** - when you start Claude Code / Gemini CLI / Codex / opencode in the project folder, the context is injected automatically via a `SessionStart` hook. No need to run `load`.
- **Auto-save** - models are instructed to record preferences/learnings on the spot (`mega-brain remember ...`) and complete tasks (`mega-brain done ...`) without being asked. A session-end hook records the timeline as a backstop.
- **No native memory** - each LLM's internal memory is avoided/disabled (e.g. Gemini's `save_memory` tool); everything goes to Mega Brain.
- **Auto-init** - a new project is initialized on its own at first load.

### How it works

The vault set in `AI_CONTEXT_PATH` (`.env`) is mounted at `/home/alcatraz_runner/.ai-context` inside the container. Routing is automatic, by the git repo name in `/workspace`:

| Project                   | Vault path           |
| ------------------------- | -------------------- |
| Most repositories         | `{vault}/{project}/` |
| User preferences (global) | `{vault}/_global/`   |

> **Optional prefix grouping:** set `MEGABRAIN_GROUP_PREFIX` and `MEGABRAIN_GROUP_DIR` (in `.env`) to route repos whose name starts with the prefix into a subfolder. Example: `MEGABRAIN_GROUP_PREFIX=acme-` and `MEGABRAIN_GROUP_DIR=Acme` routes `acme-web` to `{vault}/Acme/acme-web/`. Disabled by default.

### Per-project context structure

```
{project}/
├── INDEX.md
├── Context/
│   ├── current-task.md    # active task
│   ├── architecture.md
│   └── stack.md
├── Memory/
│   ├── patterns/
│   ├── decisions/
│   └── gotchas/
├── Tasks/
│   ├── active/
│   ├── done/
│   └── backlog/
└── Logs/
    └── timeline.md
```

### `mega-brain` commands - work in any model

```bash
mega-brain context-md                                  # markdown context (hook payload)
mega-brain load                                        # load/inspect context manually
mega-brain init                                        # initialize a new project (usually automatic)
mega-brain task "name"                                 # create/load a task and set it active
mega-brain remember pattern|decision|gotcha|note|preference "name" ["content"]
mega-brain done ["learning 1; learning 2"]             # finish a task and save learnings
mega-brain context                                     # quick summary (shown when the container opens)
mega-brain path                                        # project path in the vault
mega-brain global-path                                 # global partition path (preferences)
```

> `preference` writes to the **global** partition (applies to all projects); the other types write to the current project's memory. The old `brain` command still works as an alias.

### Per model

Context is loaded **automatically** by session-start hooks. Each model has an instruction file in its native format under `mega-brain/`:

| Model        | Instruction file                | Auto-load via                                                           |
| ------------ | ------------------------------- | ----------------------------------------------------------------------- |
| Claude Code  | `mega-brain/claude/SKILL.md`    | `SessionStart` hook (`~/.claude/settings.json`)                         |
| Gemini CLI   | `mega-brain/gemini/GEMINI.md`   | `SessionStart` hook (`~/.gemini/settings.json`)                         |
| OpenAI Codex | `mega-brain/codex/AGENTS.md`    | `SessionStart` hook (`~/.codex/config.toml`)                            |
| opencode     | `mega-brain/opencode/AGENTS.md` | plugin `~/.config/opencode/plugin/mega-brain.js` (+ AGENTS.md fallback) |

Hooks are injected at container boot by `docker-init/mega-brain-init.sh`. No manual configuration is needed - context appears on its own and learnings are saved on their own.

To add support for a new model, see `mega-brain/ADDING-NEW-MODEL.md`.

---

## Commands

```bash
./alcatraz.sh build                       # Build the Docker image
./alcatraz.sh run [PATH|ALIAS]            # Start, mounting PATH (or ALIAS) as /workspace
                                          #   no argument: uses ./project (created if missing)
                                          #   with PATH: mounts the given directory
                                          #   with ALIAS: mounts a saved favorite workspace
                                          #   if already running: restarts with the new path
                                          #   builds only if the image is missing; waits for the Guardian
./alcatraz.sh run --rebuild               # Same as run, but forces an image rebuild
./alcatraz.sh save NAME [PATH]            # Save current workspace (or PATH) as a favorite
./alcatraz.sh list                        # List saved favorite workspaces
./alcatraz.sh remove NAME                 # Remove a favorite workspace
./alcatraz.sh exec 'COMMAND'              # Run a command in the container
./alcatraz.sh shell                       # Interactive shell
./alcatraz.sh stop                        # Stop everything (Squid + Guardian + jail)
./alcatraz.sh clean                       # Stop + remove container and volumes
./alcatraz.sh status                      # Status + currently mounted project
./alcatraz.sh resources                   # Live docker stats
./alcatraz.sh logs [SERVICE]              # Tail logs (default: jail; 'logs guardian' / 'logs squid')

# Security validation
./test-security.sh
```

### Alcatraz CLI (`./alcatraz`)

> **Prefer `./alcatraz` over `./alcatraz.sh`** — it's a smart wrapper around the native Go CLI that auto-builds if needed and provides an interactive TUI.

The `./alcatraz` wrapper detects if the Go binary is missing (or stale) and compiles it automatically. All commands are delegated to the Go CLI:

```bash
# Interactive TUI (no args) — easiest way to get started
./alcatraz

# Or use individual commands directly
./alcatraz build                    # Build the Docker image
./alcatraz run [PATH|ALIAS]         # Start the sandbox
./alcatraz run --rebuild            # Force rebuild
./alcatraz save <name> [path]       # Save a favorite workspace
./alcatraz list                     # List favorites + PROJECT_PATHS
./alcatraz remove <name>            # Remove a favorite
./alcatraz exec 'COMMAND'           # Run a command inside the container
./alcatraz shell [PATH|ALIAS]       # Interactive shell (starts if needed)
./alcatraz stop                     # Stop all containers
./alcatraz clean                    # Stop + remove volumes
./alcatraz status                   # Show status + workspace
./alcatraz resources                # Live Docker stats
./alcatraz logs [SERVICE]           # Tail logs (jail|guardian|squid)
./alcatraz test-guardian            # Run Data Guardian tests
./alcatraz test-security            # Run security isolation tests
./alcatraz tui                      # Launch TUI explicitly
```

**What `./alcatraz` adds over `./alcatraz.sh`:**

| Feature | Script (`./alcatraz.sh`) | CLI (`./alcatraz`) |
| ------- | ------------------------ | ------------------ |
| Auto-build Go CLI | ❌ | ✅ — compiles automatically if needed |
| Interactive TUI | ❌ | ✅ — Bubble Tea interface |
| `PROJECT_PATHS` auto-detect | ❌ | ✅ — lists detected projects in `list` |
| `run --rebuild` flag | ❌ | ✅ |
| `shell [PATH]` — auto-start | ❌ | ✅ |
| Alias `ls` for `list` | ❌ | ✅ |
| Alias `rm` for `remove` | ❌ | ✅ |

> **Tip:** Both tools share the same state files (`.alcatraz-state`, `.alcatraz-workspaces`), so you can mix them freely.

### Favorite workspaces (switch between projects)

Save workspaces under short names to switch quickly without typing the full path:

```bash
# Save the current project as "retro"
./alcatraz.sh run ~/projects/tetris
./alcatraz.sh save tetris

# Save a specific path as "api"
./alcatraz.sh save api ~/projects/my-api

# List favorites
./alcatraz.sh list

# Switch between projects
./alcatraz.sh stop
./alcatraz.sh run tetris

./alcatraz.sh stop
./alcatraz.sh run api

# Remove a favorite
./alcatraz.sh remove api
```

Favorites are stored in `.alcatraz-workspaces` (gitignored) as `name=/absolute/path`.

> **Tip:** `exec` and `shell` always reuse the last active workspace (saved in `.alcatraz-state`). Use `run` or `use` to switch explicitly.

### Context persistence across sessions

The container is **stateless** - everything outside `/workspace` is lost on stop. **What persists:**

| Type                           | Persists? | Where                                                              |
| ------------------------------ | --------- | ------------------------------------------------------------------ |
| **Your project files**         | Yes       | Host bind mount at `/workspace`                                    |
| **npm/pip caches**             | Yes       | Named volumes (`alcatraz-node-cache`, `alcatraz-pip-cache`)        |
| **Claude/opencode configs**    | Yes       | Named volumes (`alcatraz-claude-data`, `alcatraz-opencode-config`) |
| **/tmp, ~/.config, ~/.gemini** | No        | tmpfs - cleared on stop (Gemini config is regenerated at boot)     |
| **Shell history**              | No        | Not in a named volume                                              |

To keep context across stops and restarts:

```bash
# Stop (preserves volumes)
./alcatraz.sh stop

# Come back later - reopens the last project
./alcatraz.sh run
# or
./alcatraz.sh exec 'npm test'
# or
./alcatraz.sh shell
```

> **`clean` destroys everything** - volumes, configs, and caches. Use it only to fully reset state.

**Configurable environment variables:**

| Variable                 | Default | Description                                          |
| ------------------------ | ------- | ---------------------------------------------------- |
| `TIMEOUT_SECONDS`        | `300`   | Max timeout per command                              |
| `MAX_FILE_SIZE_MB`       | `1000`  | Max file size guard                                  |
| `MEGABRAIN_GROUP_PREFIX` | (unset) | Repo name prefix for vault grouping (see Mega Brain) |
| `MEGABRAIN_GROUP_DIR`    | (unset) | Subfolder for grouped repos                          |

```bash
# Example with a custom timeout
TIMEOUT_SECONDS=600 ./alcatraz.sh exec 'npm run build'
```

---

## AI tools

All tools come preinstalled in the image. See the [Credentials](#credentials) section for how authentication works.

### Claude Code

```bash
# Authenticate once inside container with ./alcatraz.sh shell
./alcatraz.sh exec 'claude "write a cache function"'
```

### Gemini CLI

```bash
# Authenticate inside container (config regenerates on boot unless bind mounted)
./alcatraz.sh exec 'gemini "generate code to validate email"'
```

### OpenAI / Codex

```bash
# API key passed to container if exported in your shell
export OPENAI_API_KEY=sk-...   # once per terminal session
./alcatraz.sh exec 'codex "add error handling in src/index.ts"'
```

### opencode

```bash
# Uses provider keys (ANTHROPIC_API_KEY / OPENAI_API_KEY / GOOGLE_API_KEY) or OPENCODE_API_KEY
export OPENCODE_API_KEY=...    # or export the desired provider key
./alcatraz.sh exec 'opencode run "write tests for src/index.ts"'
```

### Example - Node.js project

```bash
./alcatraz.sh run
./alcatraz.sh exec 'npm install'
./alcatraz.sh exec 'npm run build'
./alcatraz.sh exec 'npm test'

# Shell for interactive development
./alcatraz.sh shell
# Inside the container:
# $ npm run dev
# $ npm test -- --watch
```

### Example - Python

```bash
./alcatraz.sh exec 'pip install -r requirements.txt'
./alcatraz.sh exec 'python3 src/main.py'
./alcatraz.sh exec 'pytest tests/'
```

---

## Project structure

```
alcatraz/
├── Dockerfile.alcatraz          # Sandbox container image
├── docker-compose.go.yml        # Topology and resources
├── seccomp-profile.json         # Custom seccomp profile
├── squid.conf                   # Proxy whitelist
├── alcatraz.sh                  # Main CLI
├── test-security.sh             # Security validation suite
├── docker-init/                 # Boot-time init (Mega Brain hook injection)
├── platform/
│   └── backend/                 # Go: MITM proxy (Lighthouse) + Data Guardian sanitizer
├── mega-brain/                  # Persistent, dynamic memory
│   ├── mega-brain.sh
│   ├── hooks/                   # SessionStart/SessionEnd hook adapters
│   ├── claude/  codex/  gemini/  opencode/
└── project/                     # YOUR CODE (mounted at /workspace)
    ├── package.json
    ├── src/
    ├── tests/
    └── ...
```

**Volumes persisted across runs:**

- `alcatraz-node-cache` - node_modules (npm install is fast after the first time)
- `alcatraz-pip-cache` - pip cache

To clear the caches:

```bash
docker volume rm alcatraz-node-cache alcatraz-pip-cache
```

---

## Architecture and security

### Container topology

```
Host
 └── isolated-network (bridge 172.30.0.0/16)
      ├── proxy-whitelist    (Squid, port 3128)
      ├── alcatraz-backend   (Go binary)
      │    └── :8080  Lighthouse - MITM proxy + Data Guardian (sanitizes AI payloads)
      └── alcatraz           (sandbox container)
           ├── /workspace    <- ./project mounted rw
           └── http_proxy -> alcatraz-backend:8080
```

The `alcatraz` container has no direct internet access - all HTTP/HTTPS goes through Lighthouse (MITM + sanitization) and then the Squid proxy.

### Domains allowed by the proxy

`github.com`, `githubusercontent.com`, `npmjs.com`, `pypi.org`, `pythonhosted.org`, `crates.io`, `ubuntu.com`, `claude.ai`, `anthropic.com`, `googleapis.com`, `openai.com`, `statsigapi.net`

To add a domain: edit `squid.conf` and run `./alcatraz.sh build`.

### Data Guardian (sanitizer)

Lighthouse inspects every **JSON request body** that the AI tools send upstream and
replaces sensitive data with `[REDACTED_BY_ALCATRAZ_*]` markers _before_ it reaches the
provider. The detection rules live in
[`platform/backend/internal/proxy/patterns.go`](platform/backend/internal/proxy/patterns.go).

**Scope — what is and isn't touched:**

| Touched                                           | Not touched                                                                                        |
| ------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| Request bodies with `Content-Type: *json*`        | Request/response **headers** (your `x-api-key` / `Authorization` stays intact — auth never breaks) |
| The prompt/conversation payload sent to the model | **Responses** from the provider                                                                    |
| Any host the proxy intercepts                     | Non-JSON bodies and binary downloads (npm/pip tarballs, etc.)                                      |

Because only JSON request bodies are scrubbed, **package downloads and tool authentication
are never affected** — the only possible side effect is a false positive redacting a
legitimate string _inside a prompt_, which degrades context for that snippet but breaks nothing.

**Categories currently covered (~90 patterns):**

- **API keys & tokens** — OpenAI, Anthropic, Google, GitHub, Slack, Discord, AWS, Stripe, JWT, Bearer
- **AI / LLM providers** — Groq, Perplexity, Replicate, HuggingFace, OpenRouter, Cohere, Mistral
- **Captcha / automation** — 2captcha, anti-captcha, CapMonster, CapSolver, proxy credentials
- **Git / packages / CI** — GitHub (all token types), GitLab, npm, PyPI, Docker, Atlassian
- **Email / SMS / monitoring** — SendGrid, Mailgun, Mailchimp, Postmark, Twilio, Telegram, Sentry, New Relic
- **E-commerce / SaaS** — Shopify, Square, Linear, Notion, Supabase, PlanetScale, Databricks, Vault
- **Cloud credentials** — AWS (account/ARN/session), Azure (subscription/tenant/secret/storage), GCP (service account/OAuth), Cloudflare, Firebase, DigitalOcean, Terraform, Kubernetes
- **PII (BR)** — CPF, CNPJ, RG, phone, PIX, bank account
- **PII (global)** — email, credit card, IP, passport
- **Cryptographic keys** — SSH/PGP/GPG private keys, generic private/secret keys
- **Env & config** — `*_SECRET`/`*_TOKEN`/`*_PASSWORD` env vars, generic `key=value` secrets, SMTP/IMAP credentials

See [Add or remove Data Guardian patterns](#add-or-remove-data-guardian-patterns) to customize.

### Security layers

| Layer        | Mechanism                                                                           |
| ------------ | ----------------------------------------------------------------------------------- |
| Network      | Squid proxy with whitelist; no direct internet access                               |
| Filesystem   | Root FS `read_only: true`; writable areas are tmpfs                                 |
| User         | Runs as `uid 1000` (`alcatraz_runner`), `no-new-privileges: true`                   |
| Capabilities | Drops `NET_RAW`, `NET_ADMIN`, `SYS_ADMIN`, `SYS_MODULE`, `SYS_BOOT`, `DAC_OVERRIDE` |
| Syscalls     | seccomp profile blocks ptrace, mount, BPF, io_uring, kernel modules, etc.           |
| Resources    | 1.5 CPUs, 4 GB RAM, swap disabled, 5-min timeout                                    |

**Filesystem accessible inside the container:**

```
/workspace/      -> your project
/tmp/            -> temporary (tmpfs)
/dev/null, /dev/zero, /dev/random, /dev/urandom, /dev/pts/
```

---

## Customization

### Increase resources

```yaml
# docker-compose.go.yml
cpus: 2.0 # float - no quotes (required by Docker Compose V2)
mem_limit: 4g
memswap_limit: 4g
```

### Enable direct internet (no proxy)

```yaml
# docker-compose.go.yml
networks:
    isolated-network:
        driver_opts:
            com.docker.network.bridge.enable_ip_masquerade: "true"
```

### Add environment variables

```yaml
# docker-compose.go.yml
environment:
    - NODE_ENV=production
    - MY_VAR=value
```

### Mount an extra volume (read-only)

```yaml
# docker-compose.go.yml
volumes:
    - /external/path:/workspace/data:ro
```

### Install additional tools

Add `RUN` steps to `Dockerfile.alcatraz` and run `./alcatraz.sh build`.

### Add or remove Data Guardian patterns

All sanitization rules are Go regexes (RE2 syntax) in
[`platform/backend/internal/proxy/patterns.go`](platform/backend/internal/proxy/patterns.go),
inside the `SensitivePatterns` slice. Each entry is a `{Name, Regex, Replacement}` triple:

```go
{"my_service_key", re(`\bmysvc_[a-zA-Z0-9]{32}\b`), "[REDACTED_BY_ALCATRAZ_MYSVC]"},
```

- **`Name`** — shows up in the audit log (`./alcatraz.sh logs guardian`), e.g. `my_service_key(2)`.
- **`Regex`** — wrapped by the `re(...)` helper. RE2 has **no backreferences and no lookahead**.
- **`Replacement`** — the marker that takes the secret's place in the payload.

**Add a pattern** — two common styles:

```go
// 1. Fixed prefix (high precision, e.g. provider tokens):
{"linear_key", re(`\blin_api_[A-Za-z0-9]{40,}\b`), "[REDACTED_BY_ALCATRAZ_LINEAR_KEY]"},

// 2. Context-gated (for secrets with no unique shape — avoids false positives):
{
    "captcha_solver_key",
    re(`(?i)(?:"|')?(?:2captcha|capmonster|capsolver)(?:"|')?\s*['"]?\s*[:=]\s*['"]?[a-zA-Z0-9]{20,}['"]?`),
    "[REDACTED_BY_ALCATRAZ_CAPTCHA_KEY]",
},
```

> **Ordering matters.** Patterns run top-to-bottom and each replaces independently, so put
> **specific** rules above the generic catch-alls (`generic_secret`, `email_estrito`) near the
> end of the file. Once a specific rule redacts a token, the generic ones can't re-match it.
> Prefer context-gated rules for anything shorter than ~20 chars or without a unique prefix —
> a bare `[a-f0-9]{32}` will redact ordinary hashes/IDs in your prompts.

**Remove (or loosen) a pattern** — delete its line, or comment it out. If a rule is
over-matching legitimate content, tighten it (add a prefix/context) instead of removing it.

**Test your change** (the suite has real-world fixtures and false-positive guards):

```bash
cd platform/backend
go test ./internal/proxy/        # unit + real-world sanitizer tests
go build ./internal/proxy/       # confirm the regex compiles
```

A bad regex panics at startup via `regexp.MustCompile`, so a passing `go build`/`go test`
means it's valid. Rebuild the stack to apply: `./alcatraz.sh build`.

### Share the jail across projects

Use favorite workspaces to switch quickly without typing full paths:

```bash
# Save each project as a favorite
./alcatraz.sh save tetris ~/projects/tetris
./alcatraz.sh save api   ~/projects/my-api

# Switch with a short command
./alcatraz.sh run tetris
# ... work ...
./alcatraz.sh stop

./alcatraz.sh run api
# ... work ...
```

Or, without favorites, pass the path directly to `run`:

```bash
./alcatraz.sh run /path/to/another/project
```

---

## Troubleshooting

### `'cpus' expected type 'float32', got unconvertible type 'string'`

You're using Docker Compose V1 (standalone) with Docker Engine 25+. Migrate to V2:

```bash
sudo apt-get install -y docker-compose-plugin
# Verify: docker compose version
```

If you can't migrate, remove the quotes from `cpus` in `docker-compose.go.yml`:

```yaml
cpus: 1.5 # correct - no quotes
```

### `invalid service "alcatraz". Must specify either image or build`

Happens when `docker compose up --no-build` can't find an explicit `image:` in the service. A common cause is building the image with an old compose version that auto-names it `<project>_<service>`. To fix without rebuilding:

```bash
docker tag alcatraz-alcatraz:latest alcatraz:latest
./alcatraz.sh run
```

### "Cannot connect to Docker daemon" / "Permission denied"

```bash
sudo usermod -aG docker $USER
newgrp docker
# Or use sudo ./alcatraz.sh run
```

### Container won't start

```bash
./alcatraz.sh logs          # See the error
./alcatraz.sh clean
./alcatraz.sh build
./alcatraz.sh run
```

### Command exceeds the timeout

```bash
TIMEOUT_SECONDS=900 ./alcatraz.sh exec 'long-command'
```

### Memory limit exceeded

Increase `mem_limit` and `memswap_limit` in `docker-compose.go.yml` (see [Customization](#customization)).

### Project files don't show up in the container

```bash
ls -la project/            # Check there's content
./alcatraz.sh clean
./alcatraz.sh run
```

### Verify isolation manually

```bash
./alcatraz.sh shell
# Inside the container:
$ curl https://example.com      # should fail (domain not whitelisted)
$ ping 8.8.8.8                  # Network unreachable
$ whoami                        # alcatraz_runner (not root)
$ id                            # uid=1000
```

Or run the full suite:

```bash
./test-security.sh
```
