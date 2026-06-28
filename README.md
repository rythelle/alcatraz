<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./logo.png">
    <source media="(prefers-color-scheme: light)" srcset="./logo-light.png">
    <img src="./logo-light.png" alt="Alcatraz" width="420">
  </picture>
</p>

# Alcatraz — Isolated Sandbox for AI Tools

Docker-based isolation to run AI coding agents — **Claude Code, Gemini CLI, OpenAI Codex, and opencode** — safely from the terminal. You can point Alcatraz at any project in three ways: drop your code in `./project/` (the default folder), pass a path directly (`alcatraz run ~/projects/my-app`), or save named aliases and switch with a short command (`alcatraz save myapp ~/projects/my-app` → `alcatraz run myapp`). After the first run, the last used project is remembered automatically — just `alcatraz run` brings it back. Projects are always mounted at `/workspace/projects/<folder-name>` inside the container, so every project has its own named path regardless of how it was started. Additional projects from `PROJECT_PATHS` are mounted alongside it under the same directory. All outbound traffic goes through a Go MITM proxy that scrubs sensitive data before it reaches any AI provider, then through a Squid proxy that enforces a domain whitelist. Includes **Mega Brain**: persistent, per-project memory that auto-loads and auto-saves across sessions and models.

---

## Motivations

AI coding agents are powerful — and by design they read your codebase, write files, and call external APIs. That power comes with real risks that are easy to overlook:

**Sensitive data leakage.** When an agent reads your project, it reads everything: `.env` files, config files, tokens, private keys, credentials. All of that ends up verbatim in the prompt payload sent to the provider's API. Alcatraz puts a MITM proxy (the **Data Guardian**) in the path of every outbound request and redacts ~90 categories of secrets — API keys, cloud credentials, PII, SSH keys, database URLs — before they leave your machine.

**Uncontrolled filesystem access.** Without limits, an agent can read, write, or delete anything your user account can touch. Alcatraz runs the agent inside a container with a read-only root filesystem. Only `/workspace` (your project) is writable, and only from inside the container.

**Supply chain attacks via package managers.** Recent compromised npm and PyPI packages have been used to exfiltrate environment variables and files by running malicious `postinstall` scripts. When `npm install` runs inside Alcatraz, the container has no direct internet access, can't reach arbitrary hosts, can't read your home directory, and can't execute host-level syscalls like `ptrace` or `mount`. A compromised package can at most damage `/workspace` — your host is untouched.

**Unrestricted network access.** By default, a process running on your machine can reach any host on the internet. Alcatraz routes all traffic through Squid with an explicit allowlist: only the domains the tools actually need (npm registry, PyPI, Claude, Gemini, OpenAI, GitHub) are reachable. Everything else is blocked at the proxy level.

The result is a controlled environment where the agent can do its job — read your code, install dependencies, call the AI provider — but can't exfiltrate secrets, can't touch the rest of your filesystem, and can't reach arbitrary external hosts.

---

## Table of contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Credentials](#credentials)
- [Mega Brain](#mega-brain)
- [Commands](#commands)
- [Technical reference](#technical-reference)
- [Contributing](#contributing)

---

## Prerequisites

- Docker 20.10+
- Docker Compose V2 (plugin — **not** the standalone `docker-compose` V1)
- Go 1.21+ (used by `install.sh` to compile the CLI)

```bash
# Ubuntu/Debian
sudo apt-get install -y docker.io docker-compose-plugin golang-go
sudo usermod -aG docker $USER && newgrp docker

# macOS
brew install docker docker-compose go

# Windows — install Docker Desktop, WSL2, and Go from https://go.dev/dl
```

> **Why V2?** Docker Compose V1 has incompatibilities with Docker Engine 25+: it treats `cpus` as a string instead of a float and fails on `up --no-build` without an explicit `image:`. This project requires V2 (`docker compose` as a plugin).

---

## Installation

```bash
git clone https://github.com/youruser/alcatraz
cd alcatraz
./install.sh
source ~/.zshrc   # or ~/.bashrc
```

`install.sh` checks dependencies (Docker, Go), compiles the Go CLI, creates a symlink at `~/.local/bin/alcatraz`, and adds it to your PATH. After that, `alcatraz` is available from anywhere.

To update later:

```bash
git -C ~/path/to/alcatraz pull && ~/path/to/alcatraz/install.sh
```

> **(Optional)** Set a custom memory vault path before running — see [Mega Brain](#mega-brain):
> ```bash
> cp .env.example .env
> # edit AI_CONTEXT_PATH in .env
> ```

The first `alcatraz run` builds the Docker image automatically (a few minutes). After that, starting the container takes a few seconds.

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

`alcatraz` is the primary CLI. It provides an interactive TUI when called with no arguments:

```bash
# Interactive TUI — easiest way to get started
alcatraz
```

---

## Credentials

OAuth credentials are stored in named Docker volumes and persist across sessions. Authenticate once, then forget about it.

| Tool               | How auth works                                        |
| ------------------ | ----------------------------------------------------- |
| **Claude Code**    | OAuth — run `claude` inside the container once        |
| **Gemini CLI**     | OAuth — run `gemini auth` inside the container once   |
| **OpenAI / Codex** | API key — set `OPENAI_API_KEY` in `.env`              |
| **opencode**       | Provider key — set `ANTHROPIC_API_KEY` or similar in `.env` |

**First-time OAuth setup:**

```bash
alcatraz shell
# Inside the container:
claude        # opens browser for OAuth (Claude Code)
gemini auth   # interactive auth flow (Gemini CLI)
exit
# Credentials persist across stop/run — no repeat needed
```

> **Note:** Run `alcatraz clean && alcatraz run <project>` once after installing or updating to create the home directory volume. After that, OAuth credentials survive `alcatraz stop`.

**API keys (OpenAI, opencode):**

```bash
# In .env:
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
```

---

## Mega Brain

Persistent, per-project memory stored on the host and synced across sessions and models. When you start a session, context is injected automatically. When the session ends, learnings are saved automatically. No manual `load` or `save` needed.

**Where memory lives:** controlled by `AI_CONTEXT_PATH` in `.env`. Defaults to `./.ai-context`. Point it at an Obsidian or OneDrive vault to sync across machines:

```bash
# .env
AI_CONTEXT_PATH=/mnt/c/Users/youruser/OneDrive/Documents/AIContext
```

**Available commands (run these inside the container or via `exec`):**

```bash
mega-brain load                                # inspect the current context
mega-brain task "name"                         # set the active task
mega-brain remember pattern "name"             # save a code/design pattern
mega-brain remember decision "name"            # save an architectural decision
mega-brain remember gotcha "name"              # save a gotcha or pitfall
mega-brain remember note "name"                # save a general note
mega-brain remember preference "name"          # save a preference (writes to global partition)
mega-brain done ["learning 1; learning 2"]     # finish a task and save learnings
mega-brain context                             # quick summary
```

Memory is per-project (routed by git repo name). The `preference` type writes to a global partition that applies across all projects.

---

## Commands

All commands go through `alcatraz`. The older `./alcatraz.sh` script still works as a fallback, but the CLI is the preferred interface.

```bash
alcatraz                          # Interactive TUI
alcatraz run [PATH|ALIAS]         # Start sandbox, mount PATH or saved alias (auto-saves as favorite)
alcatraz run --rebuild            # Start with a forced image rebuild
alcatraz shell [PATH|ALIAS]       # Open an interactive shell (starts if needed)
alcatraz exec 'COMMAND'           # Run a one-off command in the container
alcatraz stop                     # Stop all containers
alcatraz status                   # Show running status and current workspace
alcatraz logs [SERVICE]           # Tail logs: jail (default), guardian, squid
alcatraz save NAME [PATH]         # Save a workspace alias
alcatraz list                     # List saved aliases
alcatraz remove NAME              # Remove a saved alias
alcatraz clean                    # Stop + delete containers and volumes (destructive)
alcatraz resources                # Live Docker resource stats
alcatraz test-guardian            # Run Data Guardian sanitizer tests
alcatraz test-security            # Run full security isolation test suite
```

**Workspace aliases** let you switch between projects without typing full paths:

```bash
alcatraz save api ~/projects/my-api
alcatraz save web ~/projects/my-web

alcatraz run api    # mounts ~/projects/my-api
alcatraz stop
alcatraz run web    # mounts ~/projects/my-web
```

**`PROJECT_PATHS`** — set in `.env` to mount extra projects alongside the active one. All projects — the one started with `alcatraz run` and every path in `PROJECT_PATHS` — appear at `/workspace/projects/<folder-name>` inside the container:

```bash
# .env
PROJECT_PATHS=/home/you/projects/api,/home/you/projects/web
# Inside the container:
#   /workspace/projects/my-app   ← active project (from alcatraz run)
#   /workspace/projects/api      ← from PROJECT_PATHS
#   /workspace/projects/web      ← from PROJECT_PATHS
```

**What persists across `stop`/`run` cycles:**

| Data                        | Persists | Storage                          |
| --------------------------- | -------- | -------------------------------- |
| Your project files          | Yes      | Host bind mount (`/workspace/projects/<name>`) |
| Claude / opencode auth      | Yes      | Named volumes                    |
| npm / pip caches            | Yes      | Named volumes                    |
| Mega Brain memory           | Yes      | Host path (`AI_CONTEXT_PATH`)    |
| `/tmp`, shell history       | No       | tmpfs — cleared on stop          |

> `alcatraz clean` removes everything including named volumes. Use it only to fully reset state.

---

## Technical reference

### Architecture

```
Host
 └── isolated-network (bridge 172.30.0.0/16)
      ├── proxy-whitelist    (Squid, port 3128)
      ├── alcatraz-backend   (Go binary)
      │    └── :8080  Lighthouse — MITM proxy + Data Guardian
      └── alcatraz           (sandbox container)
           ├── /workspace/projects/<name>   <- active project, rw
           ├── /workspace/projects/<name>   <- PROJECT_PATHS entries, rw
           └── http_proxy -> alcatraz-backend:8080
```

The sandbox container has no direct internet access. Every HTTP/HTTPS request goes through Lighthouse (which scrubs secrets from JSON request bodies) and then Squid (which blocks non-whitelisted domains).

### Allowed domains

`github.com`, `githubusercontent.com`, `npmjs.com`, `pypi.org`, `pythonhosted.org`, `crates.io`, `ubuntu.com`, `claude.ai`, `anthropic.com`, `googleapis.com`, `openai.com`, `statsigapi.net`

To add a domain, edit `squid.conf` and run `alcatraz run --rebuild`.

### Security layers

| Layer        | Mechanism                                                                              |
| ------------ | -------------------------------------------------------------------------------------- |
| Network      | Squid proxy with allowlist; no direct internet access from the container               |
| Filesystem   | Root FS `read_only: true`; only `/workspace` and `/tmp` (tmpfs) are writable           |
| User         | Runs as `uid 1000` (`alcatraz_runner`), `no-new-privileges: true`                      |
| Capabilities | Drops `NET_RAW`, `NET_ADMIN`, `SYS_ADMIN`, `SYS_MODULE`, `SYS_BOOT`, `DAC_OVERRIDE`   |
| Syscalls     | Custom seccomp profile blocks `ptrace`, `mount`, `BPF`, `io_uring`, kernel modules     |
| Resources    | 1.5 CPUs, 4 GB RAM, swap disabled, 5-minute command timeout                            |

### Data Guardian — how sanitization works

Lighthouse acts as a MITM proxy and intercepts every JSON request body the AI tools send upstream. Before the payload reaches the provider, all matching secrets are replaced with `[REDACTED_BY_ALCATRAZ_*]` markers.

**What is and isn't touched:**

| Touched                                           | Not touched                                                    |
| ------------------------------------------------- | -------------------------------------------------------------- |
| JSON request bodies (`Content-Type: *json*`)      | Request/response headers — auth (`x-api-key`) is never broken  |
| The prompt/conversation payload sent to the model | Provider responses                                             |
| All proxied hosts                                 | Non-JSON bodies (npm/pip tarballs, binary downloads)           |

**Categories covered (~90 patterns):**

- API keys & tokens — OpenAI, Anthropic, Google, GitHub, Slack, Discord, AWS, Stripe, JWT
- AI/LLM providers — Groq, Perplexity, Replicate, HuggingFace, OpenRouter, Cohere, Mistral
- Cloud credentials — AWS (account/ARN/session), Azure (subscription/tenant/secret), GCP (service account/OAuth), Cloudflare, Firebase, DigitalOcean, Terraform, Kubernetes
- PII (Brazil) — CPF, CNPJ, RG, phone, PIX, bank account
- PII (global) — email, credit card, IP address, passport
- Cryptographic keys — SSH, PGP/GPG private keys
- Env vars — `*_SECRET`, `*_TOKEN`, `*_PASSWORD` patterns, SMTP/IMAP credentials
- Git/CI/packages — GitHub (all token formats), GitLab, npm tokens, Docker, Atlassian
- Email/SMS/monitoring — SendGrid, Mailgun, Twilio, Telegram, Sentry, New Relic

**Adding a custom pattern:**

Patterns are Go regexes (RE2) in [`platform/backend/internal/proxy/patterns.go`](platform/backend/internal/proxy/patterns.go):

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

Then rebuild: `alcatraz run --rebuild`.

### Customization

**Increase resources** — edit `docker-compose.go.yml`:

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

**Install additional tools** — add `RUN` steps to `Dockerfile.alcatraz`, then `alcatraz run --rebuild`.

### Verify isolation

```bash
alcatraz shell
# Inside the container:
curl https://example.com    # fails — domain not whitelisted
ping 8.8.8.8                # Network unreachable
whoami                      # alcatraz_runner
id                          # uid=1000

# Or run the full automated suite:
exit
alcatraz test-security
```

### Troubleshooting

**`'cpus' expected type 'float32', got unconvertible type 'string'`** — you're running Docker Compose V1. Install V2: `sudo apt-get install -y docker-compose-plugin`.

**`invalid service "alcatraz". Must specify either image or build`** — image was built with an old compose version. Fix: `docker tag alcatraz-alcatraz:latest alcatraz:latest && alcatraz run`.

**"Cannot connect to Docker daemon"** — `sudo usermod -aG docker $USER && newgrp docker`.

**Container won't start** — `alcatraz logs`, then `alcatraz clean && alcatraz run`.

**Command exceeds timeout** — `TIMEOUT_SECONDS=900 alcatraz exec 'long-command'`.

**Memory limit exceeded** — increase `mem_limit` in `docker-compose.go.yml`.

---

## Contributing

Contributions are welcome. The project is deliberately focused — it's a sandbox for AI tools, not a general-purpose container framework — so the best contributions stay within that scope.

**Good areas to contribute:**

- New Data Guardian patterns for secrets that aren't yet covered
- Support for additional AI tools (new CLI agents, new model providers)
- Improvements to Mega Brain (new memory types, better hook integration, new model support)
- Security hardening (tighter seccomp profiles, additional capability drops, network rules)
- Bug fixes and reliability improvements

**To add support for a new AI model in Mega Brain**, see `mega-brain/ADDING-NEW-MODEL.md` — the process is documented and designed to be straightforward.

**To contribute:**

1. Fork the repo and create a branch from `main`
2. Make your change with a clear commit message
3. If you're adding or modifying Data Guardian patterns, include test cases in `platform/backend/internal/proxy/`
4. Open a pull request describing what the change does and why

There's no formal style guide — follow the conventions in the files you're editing.
