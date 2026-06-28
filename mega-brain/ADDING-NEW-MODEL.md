# Adding support for a new AI model

This guide covers everything needed for a new model to run in Alcatraz with a working
**Mega Brain**: CLI install, network, credentials, **auto-load via session-start hook**, and
memory **auto-save**.

---

## File overview

| File | Configures |
|---|---|
| `Dockerfile.alcatraz` | Installing the model CLI in the image |
| `squid.conf` | API domains allowed through the proxy |
| `docker-compose.go.yml` | Credentials (volumes/env) + Mega Brain mounts (command, hooks, instructions) |
| `docker-init/mega-brain-init.sh` | Injecting the `SessionStart`/`SessionEnd` hook into the model config at boot |
| `mega-brain/hooks/` | Hook adapters (wrap `mega-brain context-md` in the model's JSON) |
| `mega-brain/{model}/` | Instruction file in the model's native format |
| `alcatraz.sh` | Credential detection on `run` and API-key injection on `exec`/`shell` |
| `README.md` | Usage docs |

---

## 1. Dockerfile.alcatraz - install the CLI

Add the install in the AI tools block. Example (npm):

```dockerfile
RUN . "$NVM_DIR/nvm.sh" && \
    npm install -g @new-model/cli && npm cache clean --force || echo "new model install failed"
```

**Requires rebuild:** `./alcatraz.sh clean && ./alcatraz.sh build`

---

## 2. squid.conf - allow API domains

```
acl allowed_domains dstdomain .api.new-model.com
acl allowed_domains dstdomain .auth.new-model.com
```

Rebuild needed (`squid.conf` is copied at build): `./alcatraz.sh build`.

---

## 3. Credentials

- **OAuth** (browser login): credentials stored in named volume or tmpfs, authenticate inside
  the container. Do not bind mount host credential files - users authenticate once in the
  container and credentials persist in the volume.
- **API key**: nothing to mount. Add the key in `alcatraz.sh` -> `collect_api_env_args`
  (injected into every `exec`/`shell`).

---

## 4. Auto-load via hook - the core Mega Brain step

Mega Brain injects context automatically via a session-start hook. Two pieces:

### 4a. Hook adapter (`mega-brain/hooks/`)

Each CLI expects a different output format. The adapter calls `mega-brain context-md` and
wraps it in the expected JSON. Existing ones:

| Adapter | For | Output schema |
|---|---|---|
| `start-claude-codex.sh` | Claude Code, Codex | `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"..."}}` |
| `start-gemini.sh` | Gemini CLI | `{"hookSpecificOutput":{"additionalContext":"..."}}` |
| `session-end.sh` | session end (all) | runs `mega-brain hook-session-end <model>`; prints `{}` |

If the new model uses one of these schemas, reuse the adapter. Otherwise create a new one in
`mega-brain/hooks/` (start it with `export PATH=/home/alcatraz_runner/.local/bin:$PATH`).

### 4b. Config injection at boot (`docker-init/mega-brain-init.sh`)

Add a block that writes/merges the hook into the new model's config (settings.json,
config.toml, plugin, etc.), pointing at the adapter (absolute path, e.g.
`/home/alcatraz_runner/.local/bin/mb-hook-start-...`). Also disable the model's native
memory if configurable (e.g. Gemini -> `excludeTools: ["save_memory"]`).

### 4c. Mounts in `docker-compose.go.yml`

```yaml
volumes:
  # new model adapter (if you created one) - mounted as a bin
  - ./mega-brain/hooks/start-new-model.sh:/home/alcatraz_runner/.local/bin/mb-hook-start-new:ro
  # instruction file in the model's native format
  - ./mega-brain/new-model/FILE.md:/home/alcatraz_runner/.new-model/FILE.md:ro
```

> Models without a reliable native hook (e.g. opencode) use a plugin + AGENTS.md as a
> fallback, which tells the model to run `mega-brain context-md` on its first action.

---

## 5. mega-brain/{model}/ - instruction file

```bash
mkdir -p mega-brain/new-model
```

Find where the model reads instructions (`~/.{model}/`, `GEMINI.md`, `AGENTS.md`, etc.) and
adapt to the model's style. **Required content:**

1. **Auto-load**: context is already injected by the hook - the model need not run load.
2. **Auto-save without being asked**:

| Command | When to use |
|---|---|
| `mega-brain remember preference "name" ["txt"]` | User preference (GLOBAL partition) |
| `mega-brain remember <pattern\|decision\|gotcha\|note> "name" ["txt"]` | Project memory |
| `mega-brain task "name"` | Start/resume a task |
| `mega-brain done ["learnings separated by ;"]` | Finish a task |
| `mega-brain path` / `mega-brain global-path` | Vault paths |

3. **Do not use the model's native memory** - everything goes to Mega Brain.
4. **Constraints**: don't edit `~/.ai-context/` directly; don't produce markdown to copy/paste.

---

## 6. README.md - document it

Add the model to the credentials section and to the per-model auto-load table in the
"Mega Brain" section.

---

## Checklist

**Install/network/credentials:**
- [ ] CLI installed in the Dockerfile and image rebuilt; `--version` works
- [ ] API domains in `squid.conf`; `curl` to the API is not blocked
- [ ] Credentials (volume-stored OAuth **or** API key in `collect_api_env_args`)
- [ ] `./alcatraz.sh run` shows the correct credential status

**Mega Brain:**
- [ ] Hook adapter (reused or new) in `mega-brain/hooks/`
- [ ] Block in `docker-init/mega-brain-init.sh` injecting the hook + disabling native memory
- [ ] Mounts in `docker-compose.go.yml` (adapter as a bin + instruction file)
- [ ] Instruction file in `mega-brain/{model}/`
- [ ] Start a session: context appears **without** being asked (auto-load)
- [ ] The model saves a preference/learning **without** a manual command (auto-save)
- [ ] `mega-brain hook-session-end` records the timeline on session end

**Docs:**
- [ ] README updated (credentials + auto-load table)
- [ ] Supported-models table below updated

---

## Currently supported models

| Model | CLI | Credentials | Instructions | Auto-load |
|---|---|---|---|---|
| Claude Code | `@anthropic-ai/claude-code` | Named volume (auth inside container) | `claude/SKILL.md` -> `~/.claude/skills/mega-brain/` | `SessionStart` hook (`~/.claude/settings.json`) |
| Gemini CLI | `@google/gemini-cli` | tmpfs (auth inside container) | `gemini/GEMINI.md` -> `~/.gemini/GEMINI.md` | `SessionStart` hook (`~/.gemini/settings.json`) |
| OpenAI Codex | `@openai/codex` | `OPENAI_API_KEY` | `codex/AGENTS.md` -> `~/.codex/AGENTS.md` | `SessionStart` hook (`~/.codex/config.toml`) |
| opencode | `opencode` (script) | provider keys / `OPENCODE_API_KEY` | `opencode/AGENTS.md` -> `~/.config/opencode/AGENTS.md` | plugin `~/.config/opencode/plugin/mega-brain.js` (+ AGENTS.md fallback) |
