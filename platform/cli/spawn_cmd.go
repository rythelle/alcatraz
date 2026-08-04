package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alcatraz/alcatraz/cli/internal/config"
	"github.com/alcatraz/alcatraz/cli/internal/workspace"
	"github.com/spf13/cobra"
)

// The Docker names the main compose stack (name: alcatraz) creates. A spawn is
// a disposable sibling that attaches to the SAME internal network and shares
// the SAME auth/config volumes, so it inherits Guard + Lighthouse egress
// control and the CLIs' logged-in state without booting a stack of its own.
const (
	spawnNetwork   = "alcatraz_internal-net"
	spawnRoleLabel = "alcatraz.role=spawn"
	// Default cap so a burst of spawns can't exhaust the host — they all share
	// the same machine as the main sandbox.
	defaultMaxSpawns = 3
)

// spawnVolumes are the named volumes the main `alcatraz` service mounts that a
// headless spawn also needs: the MITM CA (to trust Guard), the per-CLI auth/
// config stores (so `claude`/`codex`/`gemini`/`opencode` are already logged in),
// and the home volume. Names are the compose-prefixed ones (project "alcatraz").
// Source of truth for these is docker-compose.go.yml — keep in sync.
//
// The credential/config stores are mounted READ-ONLY: a throwaway spawn must
// never mutate the shared auth state (e.g. rewrite .credentials.json on a token
// refresh) and race the main session or a sibling spawn. A valid token is only
// read; session/history writes fail silently, which is exactly what we want for
// something disposable. Only the home volume (node/nvm/binaries, plus small
// per-run state files at its root) stays writable.
var spawnVolumes = []struct {
	vol, dest string
	ro        bool
}{
	{"alcatraz_alcatraz-ca-certs", "/shared-certs", true},
	{"alcatraz_alcatraz-home", "/home/alcatraz_runner", false},
	{"alcatraz_alcatraz-claude-data", "/home/alcatraz_runner/.claude", true},
	{"alcatraz_alcatraz-codex-config", "/home/alcatraz_runner/.codex", true},
	{"alcatraz_alcatraz-gemini-data", "/home/alcatraz_runner/.gemini", true},
	{"alcatraz_alcatraz-opencode-config", "/home/alcatraz_runner/.config/opencode", true},
	{"alcatraz_alcatraz-opencode-state", "/home/alcatraz_runner/.local/state", false},
	{"alcatraz_alcatraz-node-cache", "/home/alcatraz_runner/.npm-global", true},
	{"alcatraz_alcatraz-pnpm-store", "/home/alcatraz_runner/.local/share", true},
}

// spawnEnv mirrors the security-relevant environment of the main sandbox: route
// all egress through Guard (MITM redaction) → Lighthouse (whitelist), and trust
// the Guard CA. Keep in sync with docker-compose.go.yml.
var spawnEnv = []string{
	"http_proxy=http://guard:8080",
	"https_proxy=http://guard:8080",
	"HTTP_PROXY=http://guard:8080",
	"HTTPS_PROXY=http://guard:8080",
	"no_proxy=localhost,127.0.0.1,lighthouse,guard",
	"NO_PROXY=localhost,127.0.0.1,lighthouse,guard",
	"SSL_CERT_FILE=/shared-certs/ca-cert.pem",
	"REQUESTS_CA_BUNDLE=/shared-certs/ca-cert.pem",
	"NODE_EXTRA_CA_CERTS=/shared-certs/ca-cert.pem",
	"CURL_CA_BUNDLE=/shared-certs/ca-cert.pem",
	"GIT_SSL_CAINFO=/shared-certs/ca-cert.pem",
	"CLAUDE_CODE_DISABLE_TELEMETRY=true",
	"TERM=xterm-256color",
	// Tells the Mega Brain hooks (baked into the shared settings) to no-op, so a
	// throwaway exploration never pollutes the vault or its timeline.
	"ALCATRAZ_SPAWN=1",
}

// spawnHookMounts bind-mounts the Mega Brain hook shims read-only at the paths
// the shared claude/gemini/codex settings reference. Without them the settings
// (baked into the auth volume by mega-brain-init) point at non-existent files
// and every session fails the hook; with ALCATRAZ_SPAWN=1 they self-guard to a
// no-op. Host paths are relative to projectRoot. Keep in sync with
// docker-compose.go.yml.
var spawnHookMounts = []struct{ src, dest string }{
	{"mega-brain/hooks/start-claude-codex.sh", "/home/alcatraz_runner/.local/bin/mb-hook-start-cc"},
	{"mega-brain/hooks/start-gemini.sh", "/home/alcatraz_runner/.local/bin/mb-hook-start-gemini"},
	{"mega-brain/hooks/session-end.sh", "/home/alcatraz_runner/.local/bin/mb-hook-end"},
	{"mega-brain/hooks/pre-compact.sh", "/home/alcatraz_runner/.local/bin/mb-hook-precompact"},
}

// isValidAgent is the allowlist of AI CLIs a spawn may run. Used both by the CLI
// flag and by the untrusted-request path in spawn-watch.
func isValidAgent(agent string) bool {
	switch agent {
	case "claude", "codex", "gemini", "opencode":
		return true
	}
	return false
}

// agentCommand maps an agent name to its non-interactive invocation. The task
// and optional model are appended as separate argv elements (no shell quoting),
// so the task string is passed to the CLI verbatim and can't break out.
func agentCommand(agent, task, model string) ([]string, error) {
	switch agent {
	case "claude":
		argv := []string{"claude", "-p"}
		if model != "" {
			argv = append(argv, "--model", model)
		}
		return append(argv, task), nil
	case "codex":
		argv := []string{"codex", "exec"}
		if model != "" {
			argv = append(argv, "-m", model)
		}
		return append(argv, task), nil
	case "gemini":
		argv := []string{"gemini"}
		if model != "" {
			argv = append(argv, "-m", model)
		}
		return append(argv, "-p", task), nil
	case "opencode":
		argv := []string{"opencode", "run"}
		if model != "" {
			argv = append(argv, "-m", model)
		}
		return append(argv, task), nil
	default:
		return nil, fmt.Errorf("unknown agent %q (want: claude, codex, gemini, opencode)", agent)
	}
}

func spawnID() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%06x", time.Now().UnixNano()&0xffffff)
	}
	return hex.EncodeToString(b)
}

// countRunningSpawns returns how many disposable spawn containers are alive.
func countRunningSpawns() int {
	out, err := exec.Command("docker", "ps", "-q", "--filter", "label="+spawnRoleLabel).Output()
	if err != nil {
		return 0
	}
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// spawnEgressReady verifies the egress stack the spawn depends on: a spawn has
// no route out except Guard → Lighthouse, and needs the CA volume they populate.
func spawnEgressReady() error {
	if !compose.IsRunning("guard") || !compose.IsRunning("lighthouse") {
		return fmt.Errorf("egress stack (guard + lighthouse) is not running — start it first: alcatraz run")
	}
	return nil
}

// resolveSpawnProject turns a project arg (path, saved alias, or empty for the
// active workspace) into its absolute host path, base name and in-container
// workdir.
func resolveSpawnProject(project string) (absPath, name, workdir string, err error) {
	path := project
	if path != "" {
		path = workspace.NormalizePath(path)
		if resolved, ok := wsMgr.Resolve(path); ok {
			path = resolved
		}
	} else {
		path = state.GetWorkspace()
	}
	absPath, err = filepath.Abs(path)
	if err != nil {
		return "", "", "", err
	}
	if _, statErr := os.Stat(absPath); statErr != nil {
		return "", "", "", fmt.Errorf("project directory does not exist: %s", absPath)
	}
	name = filepath.Base(absPath)
	workdir = "/workspace/projects/" + name
	return absPath, name, workdir, nil
}

type spawnOutcome struct {
	id         string
	reportPath string
	reportText string
	exitCode   int
	elapsed    time.Duration
}

// runSpawnTask runs one task in a disposable sibling sandbox and writes the
// canonical report to <absPath>/.alcatraz/spawn-<id>.md. This is the single
// vetted entrypoint shared by the CLI command, the TUI and the spawn-watch
// bridge — every caller goes through the same hardened `docker run`, so the
// isolation guarantees hold no matter who triggered the spawn. progress, if
// non-nil, receives the spawn's live stderr. Callers are responsible for the
// egress preflight and for treating `task` as untrusted input.
func runSpawnTask(agent, model, task, absPath, name, workdir string, maxSpawns int, keep bool, progress io.Writer) (spawnOutcome, error) {
	argv, err := agentCommand(agent, task, model)
	if err != nil {
		return spawnOutcome{}, err
	}
	if maxSpawns <= 0 {
		maxSpawns = defaultMaxSpawns
	}
	// Concurrency cap — spawns share the host with the main sandbox.
	if n := countRunningSpawns(); n >= maxSpawns {
		return spawnOutcome{}, fmt.Errorf("too many spawns running (%d/%d) — wait for one to finish or raise the cap", n, maxSpawns)
	}

	// The report lands where the caller reads it, host-side, so the container
	// itself needs nothing writable in the workspace.
	id := spawnID()
	reportDir := filepath.Join(absPath, ".alcatraz")
	if err := os.MkdirAll(reportDir, 0755); err != nil {
		return spawnOutcome{}, fmt.Errorf("cannot create %s: %w", reportDir, err)
	}
	reportPath := filepath.Join(reportDir, "spawn-"+id+".md")

	runArgs := buildSpawnRunArgs(id, absPath, name, workdir, keep)
	// Load nvm (node lives under it) then exec the agent with the task as a
	// positional arg — no shell interpolation of the task string. The entrypoint
	// is overridden to bash (the image bakes ENTRYPOINT ["/bin/bash"]); $0 is the
	// literal "bash" placeholder, $@ is argv.
	runArgs = append(runArgs, "alcatraz:latest",
		"-c", `. ~/.nvm/nvm.sh 2>/dev/null; exec "$@"`, "bash")
	runArgs = append(runArgs, argv...)

	start := time.Now()
	c := exec.Command("docker", runArgs...)
	var stdout, stderr strings.Builder
	c.Stdout = &stdout
	if progress != nil {
		c.Stderr = io.MultiWriter(progress, &stderr)
	} else {
		c.Stderr = &stderr
	}
	runErr := c.Run()
	elapsed := time.Since(start).Round(time.Second)

	exitCode := 0
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			return spawnOutcome{}, fmt.Errorf("failed to launch spawn: %w", runErr)
		}
	}

	report := renderSpawnReport(id, agent, model, task, absPath, elapsed, exitCode, stdout.String(), stderr.String())
	if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
		return spawnOutcome{}, fmt.Errorf("spawn finished but writing report failed: %w", err)
	}
	return spawnOutcome{id: id, reportPath: reportPath, reportText: report, exitCode: exitCode, elapsed: elapsed}, nil
}

func spawnCmd() *cobra.Command {
	var agent, model, project string
	var maxSpawns int
	var keep bool

	cmd := &cobra.Command{
		Use:   "spawn \"<task>\"",
		Short: "Run a task in a disposable sibling sandbox and save the result",
		Long: `Bring up a throwaway sibling of the sandbox, run one task
non-interactively, capture the result, and tear the container down.

The spawn joins the same internal network as the main stack, so it still goes
through Guard (secret redaction) and Lighthouse (domain whitelist). The project
is mounted READ-ONLY, so exploration can't mutate your files. The full output is
written to <project>/.alcatraz/spawn-<id>.md — the main session (human or agent)
reads that instead of burning its own context on the exploration.

Requires the egress stack (guard + lighthouse) to be up; start it with
'alcatraz run' if needed. The interactive sandbox itself need not be running.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			task := strings.Join(args, " ")
			if err := spawnEgressReady(); err != nil {
				return err
			}
			absPath, name, workdir, err := resolveSpawnProject(project)
			if err != nil {
				return err
			}

			fmt.Printf("⏳ spawn — %s exploring %s (read-only)…\n", agent, name)
			out, err := runSpawnTask(agent, model, task, absPath, name, workdir, maxSpawns, keep, os.Stderr)
			if err != nil {
				return err
			}

			rel, _ := filepath.Rel(absPath, out.reportPath)
			if rel == "" {
				rel = out.reportPath
			}
			if out.exitCode == 0 {
				fmt.Printf("✓ spawn %s done in %s → %s\n", out.id, out.elapsed, rel)
			} else {
				fmt.Printf("⚠ spawn %s exited %d after %s → %s\n", out.id, out.exitCode, out.elapsed, rel)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&agent, "agent", "a", "claude", "AI CLI to run: claude, codex, gemini, opencode")
	cmd.Flags().StringVarP(&model, "model", "m", "", "Model override passed to the agent")
	cmd.Flags().StringVarP(&project, "project", "p", "", "Project to explore (path or alias; default: active workspace)")
	cmd.Flags().IntVar(&maxSpawns, "max", defaultMaxSpawns, "Max concurrent spawns")
	cmd.Flags().BoolVar(&keep, "keep", false, "Keep the container after exit (debug; skips --rm)")
	return cmd
}

// buildSpawnRunArgs assembles the `docker run` flags for a disposable sandbox,
// mirroring the hardening of the main `alcatraz` service in docker-compose.go.yml
// with lighter resource caps and a READ-ONLY project mount.
func buildSpawnRunArgs(id, absPath, name, workdir string, keep bool) []string {
	args := []string{"run"}
	if !keep {
		args = append(args, "--rm")
	}
	args = append(args,
		"--name", "alcatraz-spawn-"+id,
		"--label", spawnRoleLabel,
		"--label", "alcatraz.spawn="+id,
		"--network", spawnNetwork,
		"--workdir", workdir,
		"--user", "1000:1000",
		// Override the image's baked ENTRYPOINT (/bin/bash) so the command args
		// below are the shell's own args, not doubled up under another bash.
		"--entrypoint", "bash",

		// Hardening — same posture as the main sandbox.
		"--cap-drop", "ALL",
		"--cap-add", "CHOWN",
		"--cap-add", "SETUID",
		"--cap-add", "SETGID",
		"--cap-add", "KILL",
		"--security-opt", "no-new-privileges:true",
		"--security-opt", "seccomp="+filepath.Join(projectRoot, "seccomp-profile.json"),
		"--read-only",

		// Writable scratch (root fs stays read-only).
		"--tmpfs", "/tmp:size=1G,mode=1777,exec",
		"--tmpfs", "/run:size=256M,noexec,mode=1777",
		"--tmpfs", "/home/alcatraz_runner/.config:size=256M,mode=1777",
		"--tmpfs", "/home/alcatraz_runner/.cache:size=512M,mode=1777",
		"--tmpfs", "/var/tmp:size=256M,noexec,mode=1777",

		// Lighter resource caps than the main sandbox — spawns are throwaway.
		"--memory", "2g",
		"--memory-swap", "2g",
		"--cpus", "1.0",
		"--pids-limit", "1024",
	)

	// Project mounted READ-ONLY so exploration can't mutate the files.
	args = append(args, "-v", absPath+":"+workdir+":ro")

	for _, m := range spawnVolumes {
		mount := m.vol + ":" + m.dest
		if m.ro {
			mount += ":ro"
		}
		args = append(args, "-v", mount)
	}
	for _, m := range spawnHookMounts {
		args = append(args, "-v", filepath.Join(projectRoot, m.src)+":"+m.dest+":ro")
	}
	for _, e := range spawnEnv {
		args = append(args, "-e", e)
	}
	for _, e := range config.CollectAPIEnvArgs() {
		args = append(args, "-e", e)
	}
	return args
}

func renderSpawnReport(id, agent, model, task, project string, elapsed time.Duration, exitCode int, stdout, stderr string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Spawn %s\n\n", id)
	fmt.Fprintf(&b, "- **agent:** %s\n", agent)
	if model != "" {
		fmt.Fprintf(&b, "- **model:** %s\n", model)
	}
	fmt.Fprintf(&b, "- **project:** %s (read-only)\n", project)
	fmt.Fprintf(&b, "- **task:** %s\n", task)
	fmt.Fprintf(&b, "- **finished:** %s (took %s, exit %d)\n\n", time.Now().Format(time.RFC3339), elapsed, exitCode)

	fmt.Fprintf(&b, "## Output\n\n")
	if s := strings.TrimRight(stdout, "\n"); s != "" {
		b.WriteString(s)
		b.WriteString("\n")
	} else {
		b.WriteString("_(no output on stdout)_\n")
	}

	if s := strings.TrimRight(stderr, "\n"); s != "" {
		fmt.Fprintf(&b, "\n## Diagnostics (stderr)\n\n```\n%s\n```\n", s)
	}
	return b.String()
}
