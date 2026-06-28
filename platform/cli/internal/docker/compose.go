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
	ProjectRoot string
	DC          string // "docker compose" or "docker-compose"
	ComposeFile string
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

// Logs tails logs for a service.
func (c *Compose) Logs(service string, follow bool) *exec.Cmd {
	args := append(c.Flags(), "logs")
	if follow {
		args = append(args, "-f")
	}
	args = append(args, service)
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

// ExecInteractive opens an interactive shell.
func (c *Compose) ExecInteractive(service string, envArgs ...string) *exec.Cmd {
	args := append(c.Flags(), "exec")
	for _, e := range envArgs {
		args = append(args, "-e", e)
	}
	args = append(args, service, "bash")
	return c.exec(args...)
}

// IsRunning checks if a service is running.
func (c *Compose) IsRunning(service string) bool {
	cmd := c.PsService(service)
	out, _ := cmd.Output()
	return strings.Contains(string(out), "running")
}

func (c *Compose) exec(args ...string) *exec.Cmd {
	parts := strings.Fields(c.DC)
	cmd := exec.Command(parts[0], append(parts[1:], args...)...)
	cmd.Dir = c.ProjectRoot
	return cmd
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
