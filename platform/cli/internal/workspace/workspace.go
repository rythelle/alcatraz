package workspace

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manager handles favorite workspaces.
type Manager struct {
	File string
}

// NewManager creates a workspace manager for the given project root.
func NewManager(projectRoot string) *Manager {
	return &Manager{
		File: filepath.Join(projectRoot, ".alcatraz-workspaces"),
	}
}

// Load reads all workspaces.
func (m *Manager) Load() (map[string]string, error) {
	workspaces := make(map[string]string)
	f, err := os.Open(m.File)
	if err != nil {
		if os.IsNotExist(err) {
			return workspaces, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			workspaces[parts[0]] = parts[1]
		}
	}
	return workspaces, scanner.Err()
}

// Save writes a workspace.
func (m *Manager) Save(name, path string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if strings.ContainsAny(name, " =#") {
		return fmt.Errorf("invalid workspace name")
	}
	workspaces, err := m.Load()
	if err != nil {
		return err
	}
	workspaces[name] = path
	return m.writeAll(workspaces)
}

// Remove deletes a workspace.
func (m *Manager) Remove(name string) error {
	workspaces, err := m.Load()
	if err != nil {
		return err
	}
	if _, ok := workspaces[name]; !ok {
		return fmt.Errorf("workspace '%s' not found", name)
	}
	delete(workspaces, name)
	return m.writeAll(workspaces)
}

// Resolve returns the path for an alias, or the input if not found.
func (m *Manager) Resolve(alias string) (string, bool) {
	workspaces, err := m.Load()
	if err != nil {
		return alias, false
	}
	if path, ok := workspaces[alias]; ok {
		return path, true
	}
	return alias, false
}

func (m *Manager) writeAll(workspaces map[string]string) error {
	f, err := os.Create(m.File)
	if err != nil {
		return err
	}
	defer f.Close()

	for name, path := range workspaces {
		fmt.Fprintf(f, "%s=%s\n", name, path)
	}
	return nil
}
