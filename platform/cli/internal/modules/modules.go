// Package modules is the single resolution point for Alcatraz's optional
// modules. The core (sandbox + Lighthouse + Guard) is always on and never
// appears here; everything else is a module that can be toggled from the
// `.env` module block or the TUI.
//
// Precedence, highest first:
//  1. A process environment variable (ALCATRAZ_MOD_<KEY>) — lets CI/scripts
//     override without touching the file.
//  2. The matching line in `.env`.
//  3. The module's built-in default.
//
// Both the Go CLI and alcatraz.sh compute state the same way; nothing else
// checks the env on its own.
package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Layer groups modules by how safe they are to have on by default.
type Layer string

const (
	// LayerSafety modules are passive or protective: they ask nothing of the
	// user and only save them. On by default.
	LayerSafety Layer = "safety"
	// LayerOptin modules add a distinct capability the user opts into. Off by
	// default.
	LayerOptin Layer = "optin"
)

// Module describes one toggleable subsystem.
type Module struct {
	Key     string // lowercase identifier, e.g. "megabrain"
	Title   string // human-facing name, e.g. "Mega Brain"
	Layer   Layer
	Default bool
	Desc    string // one-line summary for help/TUI
}

// EnvVar is the environment/.env key that toggles the module.
func (m Module) EnvVar() string {
	return "ALCATRAZ_MOD_" + strings.ToUpper(m.Key)
}

// All is the canonical module list, in display order (safety net first).
// Core features (sandbox, Lighthouse, Guard) are intentionally absent: they
// are always on and not toggleable.
var All = []Module{
	{Key: "checkpoints", Title: "Checkpoints", Layer: LayerSafety, Default: true, Desc: "File undo — snapshot the workspace to a shadow git ref"},
	{Key: "sessions", Title: "Sessions", Layer: LayerSafety, Default: true, Desc: "Resume a previous AI conversation"},
	{Key: "stats", Title: "Stats", Layer: LayerSafety, Default: true, Desc: "Passive token/cost report metered by the Guard"},
	{Key: "megabrain", Title: "Mega Brain", Layer: LayerOptin, Default: false, Desc: "Per-project persistent memory"},
	{Key: "shakedown", Title: "Shakedown", Layer: LayerOptin, Default: false, Desc: "Command-output compression (formerly 'slim')"},
	{Key: "spawn", Title: "Spawn", Layer: LayerOptin, Default: false, Desc: "Disposable sibling sandboxes for exploration"},
	{Key: "websearch", Title: "Web search", Layer: LayerOptin, Default: false, Desc: "Read-only web lookups fetched by the host, one query at a time"},
}

// Get returns the module with the given key.
func Get(key string) (Module, bool) {
	for _, m := range All {
		if m.Key == key {
			return m, true
		}
	}
	return Module{}, false
}

// parseBool interprets an on/off value. Returns (value, ok); ok is false when
// the string doesn't look like a boolean toggle (so callers fall through to the
// next precedence level).
func parseBool(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "on", "1", "true", "yes", "enabled":
		return true, true
	case "off", "0", "false", "no", "disabled":
		return false, true
	default:
		return false, false
	}
}

// envFileValues parses the ALCATRAZ_MOD_* lines from .env into KEY→raw-value.
func envFileValues(projectRoot string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(filepath.Join(projectRoot, ".env"))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ALCATRAZ_MOD_") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := kv[1]
		// Strip an inline comment (e.g. "on     # safety net").
		if i := strings.Index(val, "#"); i >= 0 {
			val = val[:i]
		}
		out[key] = strings.TrimSpace(val)
	}
	return out
}

// Enabled reports whether a single module is on, applying full precedence.
func Enabled(projectRoot, key string) bool {
	m, ok := Get(key)
	if !ok {
		return false
	}
	envVar := m.EnvVar()
	// 1. Process environment wins.
	if v, ok := os.LookupEnv(envVar); ok {
		if b, ok := parseBool(v); ok {
			return b
		}
	}
	// 2. .env line.
	if v, ok := envFileValues(projectRoot)[envVar]; ok {
		if b, ok := parseBool(v); ok {
			return b
		}
	}
	// 3. Default.
	return m.Default
}

// Resolve computes the state of every module once.
func Resolve(projectRoot string) map[string]bool {
	fileVals := envFileValues(projectRoot)
	out := make(map[string]bool, len(All))
	for _, m := range All {
		val := m.Default
		if v, ok := fileVals[m.EnvVar()]; ok {
			if b, ok := parseBool(v); ok {
				val = b
			}
		}
		if v, ok := os.LookupEnv(m.EnvVar()); ok {
			if b, ok := parseBool(v); ok {
				val = b
			}
		}
		out[m.Key] = val
	}
	return out
}

// Export writes the resolved state of every module into the process
// environment as ALCATRAZ_MOD_<KEY>=on|off, so a child `docker compose up`
// (and thus the container) sees the exact same values the CLI resolved. This is
// the "compute once, pass down" step.
func Export(projectRoot string) {
	for key, on := range Resolve(projectRoot) {
		m, _ := Get(key)
		os.Setenv(m.EnvVar(), boolStr(on))
	}
}

func boolStr(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// OffMessage is the friendly notice shown when someone invokes a module that is
// currently off.
func OffMessage(key string) string {
	m, ok := Get(key)
	if !ok {
		return fmt.Sprintf("module %q is unknown", key)
	}
	return fmt.Sprintf("module '%s' is off — enable with %s=on (in .env or the TUI Modules screen)", m.Key, m.EnvVar())
}

// block renders the canonical .env module block.
func block() string {
	var b strings.Builder
	b.WriteString("# --- Modules (core is always ON and does not appear here) ---\n")
	b.WriteString("# Turn features on/off here or from the TUI Modules screen. An\n")
	b.WriteString("# ALCATRAZ_MOD_* set in the environment overrides the line below.\n")
	for _, m := range All {
		tag := "opt-in"
		if m.Layer == LayerSafety {
			tag = "safety net (default on)"
		}
		assign := m.EnvVar() + "=" + boolStr(m.Default)
		fmt.Fprintf(&b, "%-34s# %s\n", assign, tag)
	}
	return b.String()
}

// HasBlock reports whether .env already contains any module line.
func HasBlock(projectRoot string) bool {
	return len(envFileValues(projectRoot)) > 0
}

// EnsureBlock injects the default module block into an existing .env that has
// none (migration for installs predating modules). Returns true when it wrote
// the block. A .env that doesn't exist yet is left alone — a fresh install
// copies .env.example, which already carries the block.
func EnsureBlock(projectRoot string) (bool, error) {
	envPath := filepath.Join(projectRoot, ".env")
	data, err := os.ReadFile(envPath)
	if err != nil {
		return false, nil // no .env: nothing to migrate
	}
	if HasBlock(projectRoot) {
		return false, nil
	}
	content := string(data)
	sep := "\n"
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		sep = "\n\n"
	} else if len(content) > 0 {
		sep = "\n"
	}
	content += sep + block()
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		return false, err
	}
	return true, nil
}

// SetInEnv updates (or appends) a single module's line in .env. Used by the TUI
// so env and the file never diverge.
func SetInEnv(projectRoot, key string, on bool) error {
	m, ok := Get(key)
	if !ok {
		return fmt.Errorf("unknown module %q", key)
	}
	envPath := filepath.Join(projectRoot, ".env")
	data, _ := os.ReadFile(envPath)
	lines := strings.Split(string(data), "\n")
	prefix := m.EnvVar() + "="
	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			lines[i] = m.EnvVar() + "=" + boolStr(on)
			found = true
			break
		}
	}
	out := strings.Join(lines, "\n")
	if !found {
		if len(out) > 0 && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += m.EnvVar() + "=" + boolStr(on) + "\n"
	}
	return os.WriteFile(envPath, []byte(out), 0644)
}
