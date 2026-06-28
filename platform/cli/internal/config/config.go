package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// State holds persistent state.
type State struct {
	ProjectRoot string
	StateFile   string
}

// NewState creates state manager.
func NewState(projectRoot string) *State {
	return &State{
		ProjectRoot: projectRoot,
		StateFile:   filepath.Join(projectRoot, ".alcatraz-state"),
	}
}

// GetWorkspace returns the saved workspace path.
func (s *State) GetWorkspace() string {
	data, err := os.ReadFile(s.StateFile)
	if err != nil {
		return filepath.Join(s.ProjectRoot, "project")
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "ALCATRAZ_WORKSPACE=") {
			return strings.TrimPrefix(line, "ALCATRAZ_WORKSPACE=")
		}
	}
	return filepath.Join(s.ProjectRoot, "project")
}

// SetWorkspace saves the workspace path.
func (s *State) SetWorkspace(path string) error {
	return os.WriteFile(s.StateFile, []byte(fmt.Sprintf("ALCATRAZ_WORKSPACE=%s\n", path)), 0644)
}

// LoadEnvWorkspace reads ALCATRAZ_WORKSPACE from .env.
func LoadEnvWorkspace(projectRoot string) string {
	envFile := filepath.Join(projectRoot, ".env")
	data, err := os.ReadFile(envFile)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "ALCATRAZ_WORKSPACE=") {
			v := strings.TrimPrefix(line, "ALCATRAZ_WORKSPACE=")
			v = strings.TrimSpace(v)
			if !filepath.IsAbs(v) {
				v = filepath.Join(projectRoot, v)
			}
			return v
		}
	}
	return ""
}

// LoadProjectPaths reads PROJECT_PATHS from .env (comma-separated).
func LoadProjectPaths(projectRoot string) []string {
	envFile := filepath.Join(projectRoot, ".env")
	data, err := os.ReadFile(envFile)
	if err != nil {
		return nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PROJECT_PATHS=") {
			v := strings.TrimPrefix(line, "PROJECT_PATHS=")
			v = strings.TrimSpace(v)
			if v == "" {
				return nil
			}
			var paths []string
			for _, p := range strings.Split(v, ",") {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				if !filepath.IsAbs(p) {
					p = filepath.Join(projectRoot, p)
				}
				if _, err := os.Stat(p); err == nil {
					paths = append(paths, p)
				}
			}
			return paths
		}
	}
	return nil
}

// CollectAPIEnvArgs collects API keys from environment.
func CollectAPIEnvArgs() []string {
	var args []string
	keys := []string{"ANTHROPIC_API_KEY", "GOOGLE_API_KEY", "OPENAI_API_KEY", "OPENCODE_API_KEY"}
	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			args = append(args, fmt.Sprintf("%s=%s", key, val))
		}
	}
	return args
}

// NextAction file for post-TUI actions.
const nextActionFile = ".alcatraz-next-action"

// ReadNextAction reads the pending post-TUI action.
func ReadNextAction(projectRoot string) (action string, target string) {
	path := filepath.Join(projectRoot, nextActionFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ""
	}
	os.Remove(path)
	parts := strings.SplitN(string(data), "|", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(string(data)), ""
}

// WriteNextAction writes a post-TUI action.
func WriteNextAction(projectRoot, action, target string) error {
	path := filepath.Join(projectRoot, nextActionFile)
	return os.WriteFile(path, []byte(fmt.Sprintf("%s|%s", action, target)), 0644)
}
