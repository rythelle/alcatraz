package tui

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/alcatraz/alcatraz/cli/internal/guard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// guardStep tracks the multi-step add flow / test prompt on the Guard
// screen. gIdle means the rule list is shown and its shortcuts are active.
type guardStep int

const (
	gIdle guardStep = iota
	gAddName
	gAddValue
	gAddReplace
	gTest
)

type (
	guardRule = guard.Rule
	guardFile = guard.File
)

// guardInputActive reports whether the Guard screen is currently
// capturing text (so keystrokes route to the input, not the list shortcuts).
func (a *App) guardInputActive() bool {
	return a.GuardStep != gIdle
}

// enterGuard resets the screen state and loads the rules file.
func (a *App) enterGuard() {
	a.GuardStep = gIdle
	a.GuardCursor = 0
	a.GuardNotice = ""
	a.GuardInput.Blur()
	a.loadGuard()
}

func (a *App) loadGuard() {
	f, err := guard.Load()
	a.GuardFile = f
	a.GuardLoadErr = err
	if f != nil && a.GuardCursor >= len(f.Redact) {
		a.GuardCursor = len(f.Redact) - 1
	}
	if a.GuardCursor < 0 {
		a.GuardCursor = 0
	}
}

func (a *App) startGuardInput(step guardStep, prompt string) {
	a.GuardStep = step
	a.GuardInput.SetValue("")
	a.GuardInput.Prompt = prompt
	a.GuardInput.Focus()
}

func (a *App) handleGuardKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if a.GuardStep != gIdle {
		return a.handleGuardInput(msg)
	}

	rules := a.guardRules()
	switch msg.String() {
	case "up", "k":
		if a.GuardCursor > 0 {
			a.GuardCursor--
		}
		return true, nil
	case "down", "j":
		if a.GuardCursor < len(rules)-1 {
			a.GuardCursor++
		}
		return true, nil
	case "a":
		a.GuardNotice = ""
		a.GuardDraft = guardRule{}
		a.startGuardInput(gAddName, "rule name › ")
		return true, nil
	case "t":
		a.GuardNotice = ""
		a.startGuardInput(gTest, "test text › ")
		return true, nil
	case "u":
		if !a.Compose.IsRunning("guard") {
			a.GuardNotice = "Backend not running — start Alcatraz first."
			return true, nil
		}
		return true, a.doGuardAudit()
	case "d":
		if len(rules) == 0 {
			a.GuardNotice = "No custom rules to delete."
			return true, nil
		}
		rule := rules[a.GuardCursor]
		a.ConfirmTitle = "Delete rule"
		a.ConfirmText = fmt.Sprintf("Delete custom rule %q?", rule.Name)
		a.ConfirmCursor = 1
		a.ConfirmAction = func() tea.Cmd {
			if err := guard.DeleteRule(rule.Name); err != nil {
				a.GuardNotice = "Delete failed: " + err.Error()
			} else {
				a.GuardNotice = fmt.Sprintf("Rule %q deleted.", rule.Name)
			}
			a.loadGuard()
			a.Screen = ScreenGuard
			return nil
		}
		a.Screen = ScreenConfirm
		return true, nil
	case "m":
		next := "strict"
		if a.GuardFile != nil && a.GuardFile.Mode == "strict" {
			next = "balanced"
		}
		if err := guard.SetMode(next); err != nil {
			a.GuardNotice = "Mode change failed: " + err.Error()
		} else {
			a.GuardNotice = fmt.Sprintf("Mode set to %s — backend reloads within ~1s.", next)
		}
		a.loadGuard()
		return true, nil
	case "r":
		a.loadGuard()
		a.GuardNotice = "Reloaded."
		return true, nil
	}
	return false, nil
}

// handleGuardInput advances the add/test flows. Non-enter keys are left for
// the text input (updated earlier in App.Update).
func (a *App) handleGuardInput(msg tea.KeyMsg) (bool, tea.Cmd) {
	if msg.String() != "enter" {
		return false, nil
	}
	val := strings.TrimSpace(a.GuardInput.Value())
	switch a.GuardStep {
	case gAddName:
		if val == "" {
			a.GuardNotice = "Name cannot be empty."
			return true, nil
		}
		a.GuardDraft.Name = val
		a.startGuardInput(gAddValue, "value (prefix 'regex:' for a pattern) › ")
		return true, nil
	case gAddValue:
		if val == "" {
			a.GuardNotice = "Value cannot be empty."
			return true, nil
		}
		if rest, ok := trimPrefixFold(val, "regex:"); ok {
			a.GuardDraft.Regex = strings.TrimSpace(rest)
		} else if rest, ok := trimPrefixFold(val, "re:"); ok {
			a.GuardDraft.Regex = strings.TrimSpace(rest)
		} else {
			a.GuardDraft.Literal = val
		}
		a.startGuardInput(gAddReplace, "replacement (blank = default) › ")
		return true, nil
	case gAddReplace:
		a.GuardDraft.Replace = val
		a.GuardStep = gIdle
		a.GuardInput.Blur()
		if err := guard.AddRule(a.GuardDraft); err != nil {
			a.GuardNotice = "Add failed: " + err.Error()
		} else {
			a.GuardNotice = fmt.Sprintf("Rule %q added — backend reloads within ~1s.", a.GuardDraft.Name)
		}
		a.loadGuard()
		return true, nil
	case gTest:
		a.GuardStep = gIdle
		a.GuardInput.Blur()
		if val == "" {
			return true, nil
		}
		if !a.Compose.IsRunning("guard") {
			a.GuardNotice = "Backend not running — start Alcatraz first."
			return true, nil
		}
		return true, a.doGuardTest(val)
	}
	return true, nil
}

func (a *App) guardRules() []guardRule {
	if a.GuardFile == nil {
		return nil
	}
	return a.GuardFile.Redact
}

// doGuardTest pipes text through the live backend engine (/alcatraz -check)
// and shows the before/after on the Output screen.
func (a *App) doGuardTest(text string) tea.Cmd {
	a.OutputTitle = "🛡  Guard test"
	a.OutputText = ""
	a.Loading = true
	a.LoadingText = "Running through the live engine..."
	a.Screen = ScreenOutput
	compose := a.Compose
	return func() tea.Msg {
		c := compose.ExecRaw("guard", "/alcatraz", "-check")
		c.Stdin = strings.NewReader(text)
		var out, errb bytes.Buffer
		c.Stdout = &out
		c.Stderr = &errb
		if err := c.Run(); err != nil {
			return CmdOutputMsg{Output: fmt.Sprintf("Error: %v\n%s", err, errb.String())}
		}
		result := out.String()
		body := fmt.Sprintf("Input:\n  %s\n\nSanitized:\n  %s", text, result)
		if result == text {
			body += "\n\n(no redactions — nothing matched)"
		}
		return CmdOutputMsg{Output: body}
	}
}

// doGuardAudit reads and summarizes the backend audit log on the Output
// screen.
func (a *App) doGuardAudit() tea.Cmd {
	a.OutputTitle = "🛡  Guard audit"
	a.OutputText = ""
	a.Loading = true
	a.LoadingText = "Reading audit log..."
	a.Screen = ScreenOutput
	compose := a.Compose
	return func() tea.Msg {
		c := compose.ExecRaw("guard", "cat", "/var/log/alcatraz/audit.log")
		var out bytes.Buffer
		c.Stdout = &out
		if err := c.Run(); err != nil {
			return CmdOutputMsg{Output: fmt.Sprintf("Error reading audit log: %v", err)}
		}
		return CmdOutputMsg{Output: guard.SummarizeAudit(out.Bytes())}
	}
}

func (a *App) viewGuard() string {
	title := a.Styles.Title.Render("🛡  Guard")

	if a.guardInputActive() {
		return a.viewGuardInput(title)
	}

	hint := a.Styles.Hint.Render("  a add  •  d delete  •  m mode  •  t test  •  u audit  •  r reload  •  esc back")

	// Status header.
	var status []string
	status = append(status, fmt.Sprintf("  %s %s", a.Styles.Key.Render("Rules file:"), a.Styles.Hint.Render(guard.Path())))
	if a.GuardLoadErr != nil {
		status = append(status, "  "+a.Styles.Error.Render("⚠  YAML error: "+a.GuardLoadErr.Error()))
	}
	f := a.GuardFile
	if f == nil {
		f = &guardFile{}
	}
	mode := f.Mode
	if mode == "" {
		mode = "balanced"
	}
	backend := a.Styles.StatusError.Render("● stopped")
	if a.Compose.IsRunning("guard") {
		backend = a.Styles.StatusOK.Render("● running")
	}
	status = append(status,
		fmt.Sprintf("  %s %d    %s %d    %s %v    %s %s    %s %s",
			a.Styles.Key.Render("rules:"), len(f.Redact),
			a.Styles.Key.Render("allow:"), len(f.Allow),
			a.Styles.Key.Render("markers:"), f.Markers.Enabled,
			a.Styles.Key.Render("mode:"), mode,
			a.Styles.Key.Render("backend:"), backend,
		))

	// Rule list.
	var list []string
	list = append(list, a.Styles.PanelTitle.Render("  Custom redactions"))
	if len(f.Redact) == 0 {
		list = append(list, a.Styles.Hint.Render("    none yet — press 'a' to add one"))
	} else {
		for i, r := range f.Redact {
			kind, val := "literal", r.Literal
			if r.Regex != "" {
				kind, val = "regex", r.Regex
			}
			repl := r.Replace
			if repl == "" {
				repl = "[REDACTED_BY_ALCATRAZ_CUSTOM]"
			}
			line := fmt.Sprintf("  %-18s %s=%s → %s", r.Name, kind, guard.Mask(val), a.Styles.Hint.Render(repl))
			if i == a.GuardCursor {
				list = append(list, a.Styles.MenuSelected.Render("> "+line))
			} else {
				list = append(list, a.Styles.MenuItem.Render("  "+line))
			}
		}
	}

	panel := a.Styles.Panel.Render(lipgloss.JoinVertical(lipgloss.Left,
		append(append([]string{}, status...), append([]string{""}, list...)...)...))

	var notice string
	if a.GuardNotice != "" {
		notice = "  " + a.Styles.StatusWarn.Render(a.GuardNotice)
	}

	tip := a.Styles.Hint.Render("  Values are masked — the file is mounted read-only into the backend and hot-reloaded on save.")

	return lipgloss.JoinVertical(lipgloss.Left, "", title, hint, "", panel, notice, "", tip)
}

func (a *App) viewGuardInput(title string) string {
	var heading string
	switch a.GuardStep {
	case gAddName, gAddValue, gAddReplace:
		heading = "  Add a custom redaction rule"
	case gTest:
		heading = "  Test text against the live Guard engine"
	}
	input := a.Styles.InputFocused.Render(a.GuardInput.View())
	steps := a.Styles.Hint.Render("  enter to continue • esc to cancel")

	var extra string
	if a.GuardStep == gAddValue {
		extra = a.Styles.Hint.Render("  Tip: start with 'regex:' for a Go RE2 pattern, otherwise it is an exact substring.")
	}

	return lipgloss.JoinVertical(lipgloss.Left, "", title, a.Styles.Subtitle.Render(heading), "", input, extra, "", steps)
}

// trimPrefixFold trims a case-insensitive prefix, reporting whether it matched.
func trimPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return s, false
}
