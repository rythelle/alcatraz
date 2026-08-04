package tui

import (
	"fmt"

	"github.com/alcatraz/alcatraz/cli/internal/modules"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The Modules screen is a visual editor of the .env module block: every toggle
// here writes straight back to .env, so env and the TUI can never diverge. Core
// features (sandbox, Lighthouse, Guard) are always on and never shown.

func (a *App) handleModulesKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if a.ModulesCursor > 0 {
			a.ModulesCursor--
		}
		return true, nil
	case "down", "j":
		if a.ModulesCursor < len(modules.All)-1 {
			a.ModulesCursor++
		}
		return true, nil
	case "enter", " ":
		m := modules.All[a.ModulesCursor]
		newState := !a.ModuleState[m.Key]
		if err := modules.SetInEnv(a.ProjectRoot, m.Key, newState); err != nil {
			a.ModulesNotice = "✗ failed to write .env: " + err.Error()
			return true, nil
		}
		if a.ModuleState == nil {
			a.ModuleState = map[string]bool{}
		}
		a.ModuleState[m.Key] = newState
		a.rebuildMenu()
		word := "off"
		if newState {
			word = "on"
		}
		if m.Layer == modules.LayerOptin {
			a.ModulesNotice = fmt.Sprintf("%s is now %s — takes effect on the next Run/Rebuild.", m.Title, word)
		} else {
			a.ModulesNotice = fmt.Sprintf("%s is now %s.", m.Title, word)
		}
		return true, nil
	}
	return false, nil
}

func (a *App) viewModules() string {
	var b []string
	b = append(b, "")
	b = append(b, a.Styles.Title.Render("  🧩  Modules"))
	b = append(b, "")
	b = append(b, a.Styles.Hint.Render("  Core — sandbox · Lighthouse · Guard — is always on and not shown here."))
	b = append(b, "")

	lastLayer := modules.Layer("")
	rowIdx := 0
	for _, m := range modules.All {
		if m.Layer != lastLayer {
			heading := "  Opt-in (off by default)"
			if m.Layer == modules.LayerSafety {
				heading = "  Safety net (on by default)"
			}
			b = append(b, a.Styles.Subtitle.Render(heading))
			lastLayer = m.Layer
		}

		on := a.ModuleState[m.Key]
		toggle := a.Styles.StatusError.Render("○ off")
		if on {
			toggle = a.Styles.StatusOK.Render("● on ")
		}
		title := fmt.Sprintf("%-12s", m.Title)
		desc := a.Styles.Hint.Render(m.Desc)
		line := fmt.Sprintf("%s  %s  %s", toggle, title, desc)

		if rowIdx == a.ModulesCursor {
			b = append(b, a.Styles.MenuSelected.Render("  > "+line))
		} else {
			b = append(b, a.Styles.MenuItem.Render("    "+line))
		}
		rowIdx++
	}

	b = append(b, "")
	if a.ModulesNotice != "" {
		b = append(b, a.Styles.StatusInfo.Render("  "+a.ModulesNotice))
		b = append(b, "")
	}
	b = append(b, a.Styles.Hint.Render("  space/enter toggle · esc back — changes are written to .env immediately"))

	return lipgloss.JoinVertical(lipgloss.Left, b...)
}
