package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alcatraz/alcatraz/cli/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Sessions ──
//
// A "session" is a saved AI conversation a CLI can pick up where it left off.
// They live in named volumes, so they survive stop/run cycles (only `clean`
// wipes them). This screen lists the actual sessions — one row each, newest
// first, only for tools that have any — and resumes the highlighted one in a
// shell. Claude sessions are listed individually (per project); the other CLIs
// expose only "continue latest / native picker", so they show a single row.

// SessionItem is one resumable session parsed from `alcatraz.sh sessions-data`.
type SessionItem struct {
	Tool   string // Claude / Codex / Gemini / opencode
	ID     string // session id or tag ("" when the tool has no stable id)
	Cwd    string // project path the session belongs to ("" when unknown)
	Epoch  int64  // last-modified time, for sorting/display
	Label  string // human summary (project name, tag, or "N session(s)")
	Resume string // exact command to run in the container to resume it
}

// parseSessionsTSV turns the tab-separated container output into sorted items,
// skipping any non-TSV noise (log/stderr lines mixed in).
func parseSessionsTSV(out string) []SessionItem {
	var items []SessionItem
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) < 6 || f[0] == "" || f[5] == "" {
			continue
		}
		ep, _ := strconv.ParseInt(strings.TrimSpace(f[3]), 10, 64)
		items = append(items, SessionItem{
			Tool: f[0], ID: f[1], Cwd: f[2], Epoch: ep, Label: f[4], Resume: f[5],
		})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Epoch > items[j].Epoch })
	return items
}

func (a *App) loadSessions() tea.Cmd {
	a.Screen = ScreenSessions
	a.OutputTitle = "💬  Resumable sessions"
	a.OutputText = ""
	a.Loading = true
	a.LoadingText = "Loading sessions..."
	root := a.ProjectRoot
	return func() tea.Msg {
		cmd := exec.Command("bash", filepath.Join(root, "alcatraz.sh"), "sessions-data")
		cmd.Dir = root
		out, _ := cmd.CombinedOutput()
		return SessionsLoadedMsg{Items: parseSessionsTSV(string(out))}
	}
}

// sessionChrome is everything the sessions screen draws besides the rows
// themselves: leading blank, title, hint, blank, column header, both "N more"
// markers, blank, footer. Reserving all of it — markers included, even when
// they aren't shown — keeps the view inside the window at every scroll
// position, which is the whole point of the viewport.
const sessionChrome = 11

// sessionRows returns how many session rows fit on screen. Height is 0 until
// the first WindowSizeMsg arrives, so fall back to a conservative window.
// Below sessionChrome+1 lines the screen cannot fit at all; one row is the
// floor so the list stays navigable rather than vanishing.
func (a *App) sessionRows() int {
	if a.Height <= 0 {
		return 10
	}
	if n := a.Height - sessionChrome; n > 0 {
		return n
	}
	return 1
}

// clampSessionScroll keeps the cursor inside the visible window, scrolling by
// the minimum needed so the list moves only when the cursor would leave it.
func (a *App) clampSessionScroll() {
	rows := a.sessionRows()
	if a.SessionCursor < a.SessionScroll {
		a.SessionScroll = a.SessionCursor
	}
	if a.SessionCursor >= a.SessionScroll+rows {
		a.SessionScroll = a.SessionCursor - rows + 1
	}
	if max := len(a.SessionItems) - rows; a.SessionScroll > max {
		a.SessionScroll = max
	}
	if a.SessionScroll < 0 {
		a.SessionScroll = 0
	}
}

func (a *App) handleSessionsKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if a.SessionCursor > 0 {
			a.SessionCursor--
		}
		a.clampSessionScroll()
		return true, nil
	case "down", "j":
		if a.SessionCursor < len(a.SessionItems)-1 {
			a.SessionCursor++
		}
		a.clampSessionScroll()
		return true, nil
	case "pgup":
		a.SessionCursor -= a.sessionRows()
		if a.SessionCursor < 0 {
			a.SessionCursor = 0
		}
		a.clampSessionScroll()
		return true, nil
	case "pgdown":
		a.SessionCursor += a.sessionRows()
		if a.SessionCursor > len(a.SessionItems)-1 {
			a.SessionCursor = len(a.SessionItems) - 1
		}
		if a.SessionCursor < 0 {
			a.SessionCursor = 0
		}
		a.clampSessionScroll()
		return true, nil
	case "home", "g":
		a.SessionCursor = 0
		a.clampSessionScroll()
		return true, nil
	case "end", "G":
		a.SessionCursor = len(a.SessionItems) - 1
		if a.SessionCursor < 0 {
			a.SessionCursor = 0
		}
		a.clampSessionScroll()
		return true, nil
	case "enter":
		if a.SessionCursor >= 0 && a.SessionCursor < len(a.SessionItems) {
			return true, a.ensureRunning(a.doSessionResumeQuit(a.SessionItems[a.SessionCursor].Resume))
		}
		return true, nil
	case "s":
		return true, a.ensureRunning(a.doShellQuit(a.State.GetWorkspace()))
	case "r":
		return true, a.ensureRunning(a.loadSessions)
	}
	return false, nil
}

// doSessionResumeQuit records a shell-run next-action (the resume command) and
// exits the TUI; the wrapper then opens a shell that runs it and stays open.
func (a *App) doSessionResumeQuit(command string) func() tea.Cmd {
	return func() tea.Cmd {
		return func() tea.Msg {
			config.WriteNextAction(a.ProjectRoot, "shell-run", command)
			return ShellQuitMsg{}
		}
	}
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func (a *App) viewSessions() string {
	title := a.Styles.Title.Render("💬  Resumable sessions")
	hint := a.Styles.Hint.Render("  Pick a saved AI conversation to reopen. Newest first; only tools that have sessions appear.")

	var body string
	switch {
	case a.Loading:
		body = fmt.Sprintf("\n  %s  %s\n", a.Spinner.View(), a.LoadingText)
	case len(a.SessionItems) == 0:
		body = a.Styles.Hint.Render("\n  No resumable sessions yet.\n  Open a shell (s), start an AI CLI, then come back and Refresh (r).")
	default:
		a.clampSessionScroll()
		start := a.SessionScroll
		end := start + a.sessionRows()
		if end > len(a.SessionItems) {
			end = len(a.SessionItems)
		}

		rows := []string{a.Styles.Hint.Render(fmt.Sprintf("  %-2s %-9s  %-26s  %-16s  %s", "", "TOOL", "PROJECT / TAG", "LAST USED", "ID"))}
		if start > 0 {
			rows = append(rows, a.Styles.Hint.Render(fmt.Sprintf("     ↑ %d more above", start)))
		}
		for i, it := range a.SessionItems[start:end] {
			when := "—"
			if it.Epoch > 0 {
				when = time.Unix(it.Epoch, 0).Format("2006-01-02 15:04")
			}
			id := clip(it.ID, 8)
			line := fmt.Sprintf("%-9s  %-26s  %-16s  %s", it.Tool, clip(it.Label, 26), when, a.Styles.Hint.Render(id))
			if start+i == a.SessionCursor {
				rows = append(rows, "  "+a.Styles.Key.Render("▶ ")+a.Styles.PanelTitle.Render(line))
			} else {
				rows = append(rows, "     "+line)
			}
		}
		if end < len(a.SessionItems) {
			rows = append(rows, a.Styles.Hint.Render(fmt.Sprintf("     ↓ %d more below", len(a.SessionItems)-end)))
		}
		body = strings.Join(rows, "\n")
	}

	footer := a.Styles.Hint.Render("  ↑/↓ select • pgup/pgdn page • g/G first/last • enter resume • s shell • r refresh • ESC back")
	if n := len(a.SessionItems); n > 0 {
		footer = a.Styles.Hint.Render(fmt.Sprintf("  %d/%d  •  ", a.SessionCursor+1, n)) + footer
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		title,
		hint,
		"",
		body,
		"",
		footer,
	)
}

// ── Checkpoints ──
//
// A checkpoint is a full snapshot of the workspace stored on a shadow git ref
// (refs/alcatraz/checkpoints) inside the project's own repo. It never touches
// your branches, index or HEAD, and one is taken automatically on every run/
// exec — so you can roll the working tree back to any snapshot, even after an
// AI made changes. Rolling back is itself undoable (a safety snapshot is taken
// first).

func (a *App) loadCheckpoints() tea.Cmd {
	a.Screen = ScreenCheckpoints
	a.OutputTitle = "📸  Workspace checkpoints"
	a.OutputText = ""
	a.RollbackInput.SetValue("")
	a.RollbackInput.Focus()
	a.Loading = true
	a.LoadingText = "Loading checkpoints..."
	root := a.ProjectRoot
	return func() tea.Msg {
		cmd := exec.Command("bash", filepath.Join(root, "alcatraz.sh"), "checkpoints")
		cmd.Dir = root
		out, _ := cmd.CombinedOutput()
		return CheckpointsLoadedMsg{Output: strings.TrimSpace(string(out))}
	}
}

func (a *App) handleCheckpointsKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if msg.String() == "enter" {
		sel := strings.TrimSpace(a.RollbackInput.Value())
		label := sel
		if label == "" {
			label = "the latest checkpoint"
		}
		a.ConfirmTitle = "Roll back workspace?"
		a.ConfirmText = fmt.Sprintf("Restore the workspace files to %s.\n"+
			"Only files change — your git branches, index and history stay intact,\n"+
			"and a safety snapshot of the current state is taken first (undoable).\n\nContinue?", label)
		a.ConfirmCursor = 1
		a.ConfirmAction = func() tea.Cmd { return a.doRollback(sel) }
		a.Screen = ScreenConfirm
		return true, nil
	}
	return false, nil
}

// doRollback restores the workspace to a checkpoint (empty sel = latest) and
// re-lists the checkpoints beneath the result.
func (a *App) doRollback(sel string) tea.Cmd {
	a.Screen = ScreenCheckpoints
	a.OutputTitle = "📸  Workspace checkpoints"
	a.OutputText = ""
	a.Loading = true
	a.LoadingText = "Rolling back workspace..."
	root := a.ProjectRoot
	return func() tea.Msg {
		args := []string{filepath.Join(root, "alcatraz.sh"), "rollback"}
		if sel != "" {
			args = append(args, sel)
		}
		cmd := exec.Command("bash", args...)
		cmd.Dir = root
		out, _ := cmd.CombinedOutput()

		list := exec.Command("bash", filepath.Join(root, "alcatraz.sh"), "checkpoints")
		list.Dir = root
		listOut, _ := list.CombinedOutput()

		combined := strings.TrimSpace(string(out)) + "\n\n" + strings.TrimSpace(string(listOut))
		return CheckpointsLoadedMsg{Output: combined}
	}
}

func (a *App) viewCheckpoints() string {
	title := a.Styles.Title.Render("📸  Workspace checkpoints")
	explain := []string{
		a.Styles.Hint.Render("  A checkpoint is a full snapshot of the workspace on a hidden git ref — taken"),
		a.Styles.Hint.Render("  automatically on every run/exec. Rolling back restores only files; your"),
		a.Styles.Hint.Render("  branches, index and history are untouched, and rollback is itself undoable."),
		"",
	}

	var content string
	if a.Loading {
		content = fmt.Sprintf("  %s  %s\n", a.Spinner.View(), a.LoadingText)
	} else if a.OutputText != "" {
		content = a.Styles.LogOutput.Render(a.OutputText)
	}

	input := a.Styles.Input.Render(a.RollbackInput.View())
	if a.RollbackInput.Focused() {
		input = a.Styles.InputFocused.Render(a.RollbackInput.View())
	}

	footer := []string{
		"",
		fmt.Sprintf("  %s Roll back to checkpoint # (from the list) or hash — leave empty for the latest:", a.Styles.Key.Render("▶")),
		"  " + input,
		"",
		a.Styles.Hint.Render("  enter roll back (asks to confirm)  •  ESC back to menu"),
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		"",
		title,
		lipgloss.JoinVertical(lipgloss.Left, explain...),
		content,
		lipgloss.JoinVertical(lipgloss.Left, footer...),
	)
}
