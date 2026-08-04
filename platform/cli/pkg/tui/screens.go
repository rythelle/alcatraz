package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Splash ──

func (a *App) viewSplash() string {
	logo := a.Styles.AsciiArt.Render(Logo())
	tagline := a.Styles.Subtitle.Render("Isolated Sandbox for AI Tools")
	loading := a.Spinner.View() + "  Initializing..."
	if !a.Loading {
		loading = ""
	}

	return lipgloss.Place(
		a.Width, a.Height-4,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(
			lipgloss.Center,
			logo,
			tagline,
			"",
			loading,
		),
	)
}

// ── Dashboard ──

func (a *App) handleDashboardKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if a.MenuCursor > 0 {
			a.MenuCursor--
		}
	case "down", "j":
		if a.MenuCursor < len(a.Menu)-1 {
			a.MenuCursor++
		}
	case "enter", " ":
		item := a.Menu[a.MenuCursor]
		if item.Title == "Quit" {
			return false, nil
		}
		if item.Title == "Open Shell" {
			ws := a.State.GetWorkspace()
			return true, a.ensureRunning(a.doShellQuit(ws))
		}
		if item.Title == "Stats" {
			return true, a.doStats()
		}
		if item.Title == "Sessions" {
			return true, a.ensureRunning(a.loadSessions)
		}
		if item.Title == "Checkpoints" {
			return true, a.loadCheckpoints()
		}
		if item.Title == "Modules" {
			a.ModulesCursor = 0
			a.ModulesNotice = ""
			a.Screen = ScreenModules
			return true, nil
		}
		if item.Title == "Rebuild & Run" {
			a.ConfirmTitle = "Rebuild Image"
			a.ConfirmText = "This rebuilds the sandbox image and restarts containers.\n" +
				"No data is lost: credentials, AI sessions, caches and Mega Brain\n" +
				"memory all persist (volumes/host paths). Only /tmp is cleared.\n" +
				"It can take several minutes. Continue?"
			a.ConfirmAction = a.doRebuild
			a.ConfirmCursor = 0
			a.Screen = ScreenConfirm
			return true, nil
		}
		if item.Title == "Stop" {
			a.ConfirmTitle = "Stop Containers"
			a.ConfirmText = "This will stop all Alcatraz containers. Continue?"
			a.ConfirmAction = a.doStop
			a.ConfirmCursor = 0
			a.Screen = ScreenConfirm
			return true, nil
		}
		if item.Title == "Clean" {
			a.ConfirmTitle = "Clean Everything"
			a.ConfirmText = "This will stop containers AND remove volumes.\nThis destroys all caches and configs. Continue?"
			a.ConfirmAction = a.doClean
			a.ConfirmCursor = 1
			a.Screen = ScreenConfirm
			return true, nil
		}
		a.Screen = item.Screen
		if item.Screen == ScreenRun {
			a.PathInput.SetValue("")
			a.PathInput.Focus()
		}
		if item.Screen == ScreenExec {
			a.CommandInput.SetValue("")
			a.CommandInput.Focus()
		}
		if item.Screen == ScreenSpawn {
			a.SpawnInput.SetValue("")
			a.SpawnInput.Focus()
			a.SpawnAgentIdx = 0
		}
		if item.Screen == ScreenGuard {
			a.enterGuard()
		}
		return true, nil
	}
	return false, nil
}

func (a *App) viewDashboard() string {
	var items []string

	items = append(items, "")
	items = append(items, a.Styles.Title.Render("  Main Menu"))
	items = append(items, "")

	for i, item := range a.Menu {
		icon := a.Styles.Key.Render(item.Icon)
		title := item.Title
		desc := a.Styles.Hint.Render(item.Desc)

		line := fmt.Sprintf("  %s  %-22s %s", icon, title, desc)

		if i == a.MenuCursor {
			items = append(items, a.Styles.MenuSelected.Render("> "+line))
		} else {
			items = append(items, a.Styles.MenuItem.Render("  "+line))
		}
	}

	items = append(items, "")

	// Status bar
	dockerStatus := a.Styles.Hint.Render("  Docker: ")
	if a.Compose != nil {
		dockerStatus += a.Styles.Success.Render("✓ " + a.Compose.DC)
	} else {
		dockerStatus += a.Styles.Error.Render("✗ not found")
	}

	containerStatus := ""
	if a.Compose != nil {
		if a.Compose.IsRunning("alcatraz") {
			containerStatus = "  " + a.Styles.StatusOK.Render("● containers running")
		} else {
			containerStatus = "  " + a.Styles.StatusError.Render("● containers stopped")
		}
	}

	ws := a.State.GetWorkspace()
	wsStatus := ""
	if ws != "" {
		wsStatus = fmt.Sprintf("  %s %s", a.Styles.Hint.Render("workspace:"), a.Styles.Key.Render(ws))
	}

	items = append(items, dockerStatus+containerStatus)
	if wsStatus != "" {
		items = append(items, wsStatus)
	}

	return lipgloss.JoinVertical(lipgloss.Left, items...)
}

// ── Run ──

func (a *App) handleRunKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return true, a.doRun(a.PathInput.Value())
	case "tab":
		if len(a.WorkspaceList) > 0 {
			a.PathInput.SetValue(a.WorkspaceList[0])
		}
		return true, nil
	}
	return false, nil
}

func (a *App) viewRun() string {
	title := a.Styles.Title.Render("▶  Run Project")
	hint := a.Styles.Hint.Render("  Enter an absolute path, saved alias, or leave empty for ./project")

	var wsHints []string
	if len(a.WorkspaceList) > 0 {
		wsHints = append(wsHints, "  Saved aliases:")
		for _, name := range a.WorkspaceList {
			path := a.Workspaces[name]
			wsHints = append(wsHints, fmt.Sprintf("    • %s → %s", a.Styles.Key.Render(name), a.Styles.Hint.Render(path)))
		}
		wsHints = append(wsHints, "")
	}

	warning := a.Styles.StatusWarn.Render("  ⚠  If the project changes, containers will restart and active shell sessions will close.")
	tip := a.Styles.Hint.Render("  Tip: to open a second project without restarting, add it to PROJECT_PATHS in .env and use Workspaces → s")

	input := a.Styles.Input.Render(a.PathInput.View())
	if a.PathInput.Focused() {
		input = a.Styles.InputFocused.Render(a.PathInput.View())
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		title,
		hint,
		"",
		input,
		"",
		lipgloss.JoinVertical(lipgloss.Left, wsHints...),
		warning,
		tip,
	)
}

// ── Exec ──

func (a *App) handleExecKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "enter":
		cmdStr := a.CommandInput.Value()
		if cmdStr != "" {
			return true, a.ensureRunning(func() tea.Cmd {
				return a.doExec(cmdStr)
			})
		}
	}
	return false, nil
}

func (a *App) viewExec() string {
	title := a.Styles.Title.Render("⚡  Execute Command")
	hint := a.Styles.Hint.Render("  Runs a one-off command inside the container and shows the output here")

	ws := a.State.GetWorkspace()
	var contextLine string
	if ws != "" {
		contextLine = fmt.Sprintf("  %s %s  %s /workspace/projects/%s",
			a.Styles.Hint.Render("workspace:"), a.Styles.Key.Render(ws),
			a.Styles.Hint.Render("→"), filepath.Base(ws))
	}

	input := a.Styles.Input.Render(a.CommandInput.View())
	if a.CommandInput.Focused() {
		input = a.Styles.InputFocused.Render(a.CommandInput.View())
	}

	examples := []string{
		"",
		"  Examples:",
		fmt.Sprintf("    %s  %s", a.Styles.Key.Render("•"), a.Styles.Hint.Render("npm install")),
		fmt.Sprintf("    %s  %s", a.Styles.Key.Render("•"), a.Styles.Hint.Render("claude \"refactor src/index.ts\"")),
		fmt.Sprintf("    %s  %s", a.Styles.Key.Render("•"), a.Styles.Hint.Render("pytest tests/")),
		"",
		a.Styles.Hint.Render("  Note: for interactive commands use Open Shell instead."),
	}

	lines := []string{"", title, hint}
	if contextLine != "" {
		lines = append(lines, contextLine)
	}
	lines = append(lines, "", input)
	lines = append(lines, examples...)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// ── Spawn (disposable sibling sandbox) ──

func (a *App) handleSpawnKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "tab":
		a.SpawnAgentIdx = (a.SpawnAgentIdx + 1) % len(spawnAgentChoices)
		return true, nil
	case "shift+tab":
		a.SpawnAgentIdx = (a.SpawnAgentIdx - 1 + len(spawnAgentChoices)) % len(spawnAgentChoices)
		return true, nil
	case "enter":
		task := strings.TrimSpace(a.SpawnInput.Value())
		if task != "" {
			agent := spawnAgentChoices[a.SpawnAgentIdx]
			// ensureRunning brings the egress stack up if it's down, so the spawn
			// always has Guard + Lighthouse to route through.
			return true, a.ensureRunning(func() tea.Cmd {
				return a.runSpawn(agent, task)
			})
		}
	}
	return false, nil
}

func (a *App) viewSpawn() string {
	title := a.Styles.Title.Render("🧬  Spawn — disposable sibling sandbox")
	hint := a.Styles.Hint.Render("  Runs one task in a throwaway sandbox (project mounted READ-ONLY) and saves the result")

	ws := a.State.GetWorkspace()
	var contextLine string
	if ws != "" {
		contextLine = fmt.Sprintf("  %s %s  %s .alcatraz/spawn-<id>.md",
			a.Styles.Hint.Render("project:"), a.Styles.Key.Render(ws),
			a.Styles.Hint.Render("→ report:"))
	}

	var agentParts []string
	for i, name := range spawnAgentChoices {
		if i == a.SpawnAgentIdx {
			agentParts = append(agentParts, a.Styles.MenuSelected.Render("["+name+"]"))
		} else {
			agentParts = append(agentParts, a.Styles.Hint.Render(" "+name+" "))
		}
	}
	agentLine := "  " + a.Styles.Hint.Render("agent (tab to change):") + " " + strings.Join(agentParts, " ")

	input := a.Styles.Input.Render(a.SpawnInput.View())
	if a.SpawnInput.Focused() {
		input = a.Styles.InputFocused.Render(a.SpawnInput.View())
	}

	notes := []string{
		"",
		fmt.Sprintf("    %s %s", a.Styles.Key.Render("•"), a.Styles.Hint.Render("Same egress control (Guard → Lighthouse); your files can't be mutated")),
		fmt.Sprintf("    %s %s", a.Styles.Key.Render("•"), a.Styles.Hint.Render("Enter to run · tab to switch agent · esc to go back")),
	}

	lines := []string{"", title, hint}
	if contextLine != "" {
		lines = append(lines, contextLine)
	}
	lines = append(lines, "", agentLine, "", input)
	lines = append(lines, notes...)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
