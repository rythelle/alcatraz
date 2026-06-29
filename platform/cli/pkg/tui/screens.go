package tui

import (
	"fmt"

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
	status := a.Styles.Hint.Render("  Docker: ")
	if a.Compose != nil {
		status += a.Styles.Success.Render("✓ " + a.Compose.DC)
	} else {
		status += a.Styles.Error.Render("✗ not found")
	}
	items = append(items, status)

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
	hint := a.Styles.Hint.Render("  Enter a path, saved alias, or leave empty for ./project")

	var wsHints []string
	if len(a.WorkspaceList) > 0 {
		wsHints = append(wsHints, "  Saved workspaces:")
		for _, name := range a.WorkspaceList {
			path := a.Workspaces[name]
			wsHints = append(wsHints, fmt.Sprintf("    • %s → %s", a.Styles.Key.Render(name), a.Styles.Hint.Render(path)))
		}
	}

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
	hint := a.Styles.Hint.Render("  Type a command to run inside the Alcatraz container")

	input := a.Styles.Input.Render(a.CommandInput.View())
	if a.CommandInput.Focused() {
		input = a.Styles.InputFocused.Render(a.CommandInput.View())
	}

	examples := []string{
		"  Examples:",
		fmt.Sprintf("    %s  %s", a.Styles.Key.Render("•"), a.Styles.Hint.Render("npm install")),
		fmt.Sprintf("    %s  %s", a.Styles.Key.Render("•"), a.Styles.Hint.Render("claude \"refactor src/index.ts\"")),
		fmt.Sprintf("    %s  %s", a.Styles.Key.Render("•"), a.Styles.Hint.Render("pytest tests/")),
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		title,
		hint,
		"",
		input,
		"",
		lipgloss.JoinVertical(lipgloss.Left, examples...),
	)
}

