package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alcatraz/alcatraz/cli/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// workspaceEntry represents an item in the combined workspace list.
type workspaceEntry struct {
	Name     string
	Path     string
	Detected bool // true = from PROJECT_PATHS, false = saved favorite
}

func (a *App) workspaceEntries() []workspaceEntry {
	var entries []workspaceEntry
	// Saved favorites first
	for _, name := range a.WorkspaceList {
		entries = append(entries, workspaceEntry{Name: name, Path: a.Workspaces[name], Detected: false})
	}
	// Detected from PROJECT_PATHS
	for _, path := range a.DetectedProjects {
		name := filepath.Base(path)
		entries = append(entries, workspaceEntry{Name: name, Path: path, Detected: true})
	}
	return entries
}

// ── Workspaces ──

func (a *App) handleWorkspacesKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	entries := a.workspaceEntries()
	switch msg.String() {
	case "up", "k":
		if a.WSCursor > 0 {
			a.WSCursor--
		}
		return true, nil
	case "down", "j":
		if a.WSCursor < len(entries)-1 {
			a.WSCursor++
		}
		return true, nil
	case "n":
		a.AliasInput.SetValue("")
		a.AliasInput.Focus()
		return true, nil
	case "d":
		if len(entries) > 0 && a.WSCursor < len(entries) {
			entry := entries[a.WSCursor]
			if !entry.Detected {
				a.WorkspaceMgr.Remove(entry.Name)
				ws, _ := a.WorkspaceMgr.Load()
				a.Workspaces = ws
				a.WorkspaceList = make([]string, 0, len(ws))
				for n := range ws {
					a.WorkspaceList = append(a.WorkspaceList, n)
				}
				if a.WSCursor >= len(a.workspaceEntries()) {
					a.WSCursor = len(a.workspaceEntries()) - 1
					if a.WSCursor < 0 {
						a.WSCursor = 0
					}
				}
			}
		}
		return true, nil
	case "enter":
		if len(entries) > 0 && a.WSCursor < len(entries) {
			entry := entries[a.WSCursor]
			a.PathInput.SetValue(entry.Path)
			a.Screen = ScreenRun
		}
		return true, nil
	case "s":
		if len(entries) > 0 && a.WSCursor < len(entries) {
			entry := entries[a.WSCursor]
			config.WriteNextAction(a.ProjectRoot, "shell", entry.Path)
			return true, tea.Quit
		}
		return true, nil
	}
	return false, nil
}

func (a *App) viewWorkspaces() string {
	title := a.Styles.Title.Render("📁  Workspaces")
	hint := a.Styles.Hint.Render("  enter=run  s=run+shell  d=delete  ↑/↓=navigate")

	entries := a.workspaceEntries()
	var items []string

	if len(entries) == 0 {
		items = append(items, "  No workspaces found.")
		items = append(items, "")
		items = append(items, fmt.Sprintf("  Press %s to save the current project", a.Styles.Key.Render("n")))
	} else {
		// Favorites section
		if len(a.WorkspaceList) > 0 {
			items = append(items, a.Styles.PanelTitle.Render("  ⭐ Favorites"))
		}
		favCount := len(a.WorkspaceList)
		for i, entry := range entries {
			if entry.Detected && i == favCount {
				items = append(items, "")
				items = append(items, a.Styles.PanelTitle.Render("  🔍 Detected from PROJECT_PATHS"))
			}
			exists := "✓"
			if _, err := os.Stat(entry.Path); err != nil {
				exists = "⚠"
			}
			badge := ""
			if entry.Detected {
				badge = a.Styles.Hint.Render(" [auto]")
			}
			line := fmt.Sprintf("  %s %-16s %s%s", exists, a.Styles.Key.Render(entry.Name), a.Styles.Hint.Render(entry.Path), badge)
			if i == a.WSCursor {
				items = append(items, a.Styles.MenuSelected.Render("> "+line))
			} else {
				items = append(items, a.Styles.MenuItem.Render("  "+line))
			}
		}
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		title,
		hint,
		"",
		lipgloss.JoinVertical(lipgloss.Left, items...),
	)
}

// ── Status ──

func (a *App) handleStatusKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if msg.String() == "r" {
		return true, nil
	}
	return false, nil
}

func (a *App) viewStatus() string {
	title := a.Styles.Title.Render("ℹ  Status")
	hint := a.Styles.Hint.Render("  Press r to refresh")

	var lines []string
	lines = append(lines, fmt.Sprintf("  %s  %s", a.Styles.Key.Render("Project Root:"), a.ProjectRoot))
	lines = append(lines, fmt.Sprintf("  %s  %s", a.Styles.Key.Render("Docker Compose:"), a.Compose.DC))
	lines = append(lines, "")

	services := []struct {
		Name string
		Desc string
	}{
		{"alcatraz", "Sandbox container"},
		{"alcatraz-backend", "Data Guardian (MITM proxy)"},
		{"proxy-whitelist", "Squid proxy"},
	}

	for _, svc := range services {
		status := a.Styles.StatusError.Render("● stopped")
		if a.Compose.IsRunning(svc.Name) {
			status = a.Styles.StatusOK.Render("● running")
		}
		lines = append(lines, fmt.Sprintf("  %-22s %s  %s", svc.Desc, status, a.Styles.Hint.Render(svc.Name)))
	}

	lines = append(lines, "")
	ws := a.State.GetWorkspace()
	lines = append(lines, fmt.Sprintf("  %s  %s", a.Styles.Key.Render("Workspace:"), ws))
	lines = append(lines, fmt.Sprintf("  %s  %s", a.Styles.Key.Render("Mount:"), ws+" → /workspace"))

	lines = append(lines, "")
	lines = append(lines, a.Styles.Hint.Render("  Use 'Resources' from the main menu for live stats"))

	panel := a.Styles.Panel.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		title,
		hint,
		"",
		panel,
	)
}

// ── Logs ──

func (a *App) handleLogsKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "1":
		a.showLogsInstructions("alcatraz")
		return true, nil
	case "2":
		a.showLogsInstructions("alcatraz-backend")
		return true, nil
	case "3":
		a.showLogsInstructions("proxy-whitelist")
		return true, nil
	}
	return false, nil
}

func (a *App) viewLogs() string {
	title := a.Styles.Title.Render("📋  Logs")
	hint := a.Styles.Hint.Render("  Select a service to see the tail command")

	services := []struct {
		Key  string
		Name string
		Desc string
	}{
		{"1", "alcatraz", "Sandbox container (default)"},
		{"2", "alcatraz-backend", "Data Guardian / MITM proxy"},
		{"3", "proxy-whitelist", "Squid whitelist proxy"},
	}

	var items []string
	for _, svc := range services {
		line := fmt.Sprintf("  [%s] %-20s %s", a.Styles.Key.Render(svc.Key), svc.Name, a.Styles.Hint.Render(svc.Desc))
		items = append(items, line)
	}

	panel := a.Styles.Panel.Render(lipgloss.JoinVertical(lipgloss.Left, items...))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		title,
		hint,
		"",
		panel,
	)
}

func (a *App) showLogsInstructions(service string) {
	a.Screen = ScreenOutput
	a.OutputTitle = fmt.Sprintf("📋  Logs: %s", service)

	cmdLine := fmt.Sprintf("docker compose -f docker-compose.go.yml logs -f %s", service)

	a.OutputText = fmt.Sprintf(`Live log tailing cannot run inside the TUI because
'docker compose logs -f' needs control of the terminal.

Run this command in your regular terminal instead:

  %s

Or use the CLI directly:

  ./alcatraz logs %s

Press ESC to return to the menu.
`, a.Styles.Key.Render(cmdLine), service)
}

// ── Tests ──

func (a *App) handleTestsKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "1":
		return true, a.doTestGuardian()
	case "2":
		return true, a.doTestSecurity()
	}
	return false, nil
}

func (a *App) viewTests() string {
	title := a.Styles.Title.Render("🧪  Test Suites")
	hint := a.Styles.Hint.Render("  Select a test suite to run")

	items := []string{
		fmt.Sprintf("  [%s] %-24s %s", a.Styles.Key.Render("1"), "Data Guardian", a.Styles.Hint.Render("Go unit + real-world sanitizer tests")),
		fmt.Sprintf("  [%s] %-24s %s", a.Styles.Key.Render("2"), "Security", a.Styles.Hint.Render("Isolation validation (needs running containers)")),
	}

	panel := a.Styles.Panel.Render(lipgloss.JoinVertical(lipgloss.Left, items...))

	return lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		title,
		hint,
		"",
		panel,
	)
}

// ── Confirm ──

func (a *App) handleConfirmKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "left", "h":
		if a.ConfirmCursor > 0 {
			a.ConfirmCursor--
		}
		return true, nil
	case "right", "l":
		if a.ConfirmCursor < 1 {
			a.ConfirmCursor++
		}
		return true, nil
	case "enter":
		if a.ConfirmCursor == 0 {
			return true, a.ConfirmAction()
		}
		a.Screen = ScreenDashboard
		return true, nil
	}
	return false, nil
}

func (a *App) viewConfirm() string {
	title := a.Styles.DialogTitle.Render(a.ConfirmTitle)
	text := a.Styles.Value.Render(a.ConfirmText)

	yesBtn := a.Styles.Button.Render("  Yes  ")
	noBtn := a.Styles.Button.Render("  No  ")

	if a.ConfirmCursor == 0 {
		yesBtn = a.Styles.ButtonFocused.Render("  Yes  ")
	} else {
		noBtn = a.Styles.ButtonFocused.Render("  No  ")
	}

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, yesBtn, noBtn)

	dialog := a.Styles.Dialog.Render(
		lipgloss.JoinVertical(
			lipgloss.Center,
			title,
			"",
			text,
			"",
			buttons,
		),
	)

	return lipgloss.Place(
		a.Width, a.Height-4,
		lipgloss.Center, lipgloss.Center,
		dialog,
	)
}

func (a *App) doStop() tea.Cmd {
	cmd := a.Compose.Down(false)
	return a.runCmd(cmd, "Stopping containers...")
}

func (a *App) doClean() tea.Cmd {
	cmd := a.Compose.Down(true)
	return a.runCmd(cmd, "Cleaning up...")
}

// ── Output ──

func (a *App) handleOutputKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "esc":
		a.Screen = ScreenDashboard
		a.Loading = false
		return true, nil
	}
	return false, nil
}

func (a *App) viewOutput() string {
	title := a.Styles.Title.Render(a.OutputTitle)

	var content string
	if a.Loading {
		content = fmt.Sprintf("\n  %s  %s\n", a.Spinner.View(), a.LoadingText)
	}

	if a.OutputText != "" {
		lines := strings.Split(a.OutputText, "\n")
		maxLines := a.Height - 12
		if len(lines) > maxLines && maxLines > 0 {
			lines = lines[len(lines)-maxLines:]
			content += a.Styles.Hint.Render("  ... (output truncated) ...\n")
		}
		content += a.Styles.LogOutput.Render(strings.Join(lines, "\n"))
	}

	footer := a.Styles.Hint.Render("  Press ESC to return to menu")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		title,
		"",
		content,
		"",
		footer,
	)
}
