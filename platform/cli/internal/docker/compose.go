package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Compose wraps docker compose operations.
type Compose struct {
	ProjectRoot  string
	DC           string // "docker compose" or "docker-compose"
	ComposeFile  string
	OverrideFile string
}

// NewCompose auto-detects Docker Compose and returns a wrapper.
func NewCompose(projectRoot string) (*Compose, error) {
	// Try Docker Compose V2 first
	cmd := exec.Command("docker", "compose", "version")
	if err := cmd.Run(); err == nil {
		return &Compose{
			ProjectRoot:  projectRoot,
			DC:           "docker compose",
			ComposeFile:  filepath.Join(projectRoot, "docker-compose.go.yml"),
			OverrideFile: filepath.Join(projectRoot, "docker-compose.override.yml"),
		}, nil
	}

	// Fall back to V1
	if _, err := exec.LookPath("docker-compose"); err == nil {
		return &Compose{
			ProjectRoot:  projectRoot,
			DC:           "docker-compose",
			ComposeFile:  filepath.Join(projectRoot, "docker-compose.go.yml"),
			OverrideFile: filepath.Join(projectRoot, "docker-compose.override.yml"),
		}, nil
	}

	return nil, fmt.Errorf("docker compose not found")
}

// Flags returns the -f flags for docker compose.
func (c *Compose) Flags() []string {
	flags := []string{"-f", c.ComposeFile}
	if _, err := os.Stat(c.OverrideFile); err == nil {
		flags = append(flags, "-f", c.OverrideFile)
	}
	return flags
}

// Build builds the Docker image.
func (c *Compose) Build() *exec.Cmd {
	args := append(c.Flags(), "build", "--no-cache")
	return c.exec(args...)
}

// Up starts the containers.
func (c *Compose) Up(noBuild bool, build bool) *exec.Cmd {
	args := append(c.Flags(), "up", "-d")
	if noBuild {
		args = append(args, "--no-build")
	}
	if build {
		args = append(args, "--build")
	}
	return c.exec(args...)
}

// Down stops and removes containers.
func (c *Compose) Down(volumes bool) *exec.Cmd {
	args := append(c.Flags(), "down")
	if volumes {
		args = append(args, "-v")
	}
	return c.exec(args...)
}

// Ps lists containers.
func (c *Compose) Ps() *exec.Cmd {
	args := append(c.Flags(), "ps")
	return c.exec(args...)
}

// PsService lists a specific service.
func (c *Compose) PsService(service string) *exec.Cmd {
	args := append(c.Flags(), "ps", service)
	return c.exec(args...)
}

// Logs tails logs for a service. tail=0 means no limit; service="" means all services.
func (c *Compose) Logs(service string, follow bool, tail int) *exec.Cmd {
	args := append(c.Flags(), "logs")
	if follow {
		args = append(args, "-f")
	}
	if tail > 0 {
		args = append(args, fmt.Sprintf("--tail=%d", tail))
	}
	if service != "" {
		args = append(args, service)
	}
	return c.exec(args...)
}

// Exec runs a command in a container.
func (c *Compose) Exec(service string, cmd string, envArgs ...string) *exec.Cmd {
	args := append(c.Flags(), "exec", "-T")
	for _, e := range envArgs {
		args = append(args, "-e", e)
	}
	args = append(args, service, "bash", "-c", ". ~/.nvm/nvm.sh 2>/dev/null; "+cmd)
	return c.exec(args...)
}

// ExecRaw runs a command in a service without the interactive/bash wrapper,
// suitable for piping data to stdin (docker compose exec -T <service> <args…>).
func (c *Compose) ExecRaw(service string, cmdArgs ...string) *exec.Cmd {
	args := append(c.Flags(), "exec", "-T", service)
	args = append(args, cmdArgs...)
	return c.exec(args...)
}

// ExecInteractive opens an interactive shell, optionally starting in workdir.
// The -it flags are required so terminal programs (opencode, claude, etc.)
// get a real TTY and raw input mode; docker compose exec's auto-detection is
// unreliable when the Go exec.Command inherits stdin/stdout from a wrapper.
// stty sane is run first because the host TTY may be left in a state that
// breaks raw input mode for some TUIs (opencode ignores Enter otherwise).
func (c *Compose) ExecInteractive(service string, workdir string, envArgs ...string) *exec.Cmd {
	args := append(c.Flags(), "exec", "-it")
	if workdir != "" {
		args = append(args, "--workdir", workdir)
	}
	for _, e := range envArgs {
		args = append(args, "-e", e)
	}
	args = append(args, service, "bash", "-c", "stty sane 2>/dev/null || true; exec bash")
	return c.exec(args...)
}

// ExecInteractiveRun opens an interactive shell that first runs `runCmd`
// (e.g. `claude --continue` to resume a session) and then drops the user into a
// normal interactive bash, so they stay in the container after the command
// exits. Same TTY handling as ExecInteractive.
func (c *Compose) ExecInteractiveRun(service, workdir, runCmd string, envArgs ...string) *exec.Cmd {
	args := append(c.Flags(), "exec", "-it")
	if workdir != "" {
		args = append(args, "--workdir", workdir)
	}
	for _, e := range envArgs {
		args = append(args, "-e", e)
	}
	// Load nvm (needed by the AI CLIs), run the resume command, then exec bash.
	inner := "stty sane 2>/dev/null || true; . ~/.nvm/nvm.sh 2>/dev/null; " + runCmd + "; exec bash"
	args = append(args, service, "bash", "-c", inner)
	return c.exec(args...)
}

// IsRunning checks if a service is running.
// Handles both "running" (newer compose) and "Up" (older/alternate format).
func (c *Compose) IsRunning(service string) bool {
	cmd := c.PsService(service)
	out, _ := cmd.Output()
	s := strings.ToLower(string(out))
	return strings.Contains(s, "running") || strings.Contains(s, " up ")
}

// PathMounted reports whether dir exists inside the running alcatraz container
// (i.e. the project is already bind-mounted at /workspace/projects/<name>).
// Used to skip a needless restart when switching to a project that is already
// mounted — with PROJECT_PATHS multi-mount, that's the common case.
func (c *Compose) PathMounted(dir string) bool {
	if dir == "" || dir == "/workspace" {
		return true
	}
	return c.ExecRaw("alcatraz", "test", "-d", dir).Run() == nil
}

// PauseAll asks Mega Brain to snapshot every mounted project before the
// containers are brought down, so in-progress work in any open shell survives a
// restart (resume with `mega-brain resume`). Best-effort: callers ignore errors.
func (c *Compose) PauseAll() *exec.Cmd {
	return c.Exec("alcatraz", "mega-brain pause-all 'auto: container restart'")
}

func (c *Compose) exec(args ...string) *exec.Cmd {
	parts := strings.Fields(c.DC)
	cmd := exec.Command(parts[0], append(parts[1:], args...)...)
	cmd.Dir = c.ProjectRoot
	return cmd
}

// GenerateOverride writes docker-compose.override.yml that mounts all projects
// under /workspace/projects/<name>. activePath is the current workspace;
// extraPaths come from PROJECT_PATHS. Removes the file when there are no paths.
func (c *Compose) GenerateOverride(activePath string, extraPaths []string) error {
	seen := map[string]bool{}
	var volumes []string

	add := func(p string) {
		if p == "" {
			return
		}
		name := filepath.Base(p)
		if seen[name] {
			return
		}
		seen[name] = true
		volumes = append(volumes, fmt.Sprintf("      - %s:/workspace/projects/%s:rw", p, name))
	}

	add(activePath)
	for _, p := range extraPaths {
		add(p)
	}

	if len(volumes) == 0 {
		os.Remove(c.OverrideFile)
		return nil
	}

	lines := []string{
		"# Auto-generated by alcatraz-cli - do not edit manually",
		"services:",
		"  alcatraz:",
		"    volumes:",
	}
	lines = append(lines, volumes...)
	return os.WriteFile(c.OverrideFile, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

// EnsureContextDir ensures the AI_CONTEXT_PATH directory exists.
func EnsureContextDir(projectRoot string) (string, error) {
	envFile := filepath.Join(projectRoot, ".env")
	var p string
	if data, err := os.ReadFile(envFile); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "AI_CONTEXT_PATH=") {
				p = strings.TrimPrefix(line, "AI_CONTEXT_PATH=")
				p = strings.TrimSpace(p)
				break
			}
		}
	}
	if p == "" {
		p = filepath.Join(projectRoot, ".ai-context")
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(projectRoot, p)
	}
	return p, os.MkdirAll(p, 0755)
}
