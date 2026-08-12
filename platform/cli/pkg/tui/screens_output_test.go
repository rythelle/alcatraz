package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newOutputApp builds the minimum App the output screen needs, filled with n
// numbered lines (line-0000 … line-NNNN) so any line can be located in a view.
func newOutputApp(height, width, n int) *App {
	a := &App{Height: height, Width: width, Styles: DefaultStyles(), Screen: ScreenOutput}
	a.OutputTitle = "📊  Token usage (Guard)"
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("line-%04d", i)
	}
	a.OutputText = strings.Join(lines, "\n")
	a.OutputFollow = true
	return a
}

// Regression: Stats (and every other snapshot) rendered the tail of its output
// with no way to move — "aqui no stats não tem scroll ali na listagem". The
// screen must stay inside the terminal AND let the user reach every line.
func TestOutputViewFitsTheTerminal(t *testing.T) {
	for _, height := range []int{outputChrome + 1, 24, 40, 60} {
		for _, n := range []int{0, 1, 5, 73, 500} {
			a := newOutputApp(height, 100, n)
			if lines := strings.Count(a.View(), "\n") + 1; lines > height {
				t.Errorf("height=%d lines=%d: view is %d rows, taller than the terminal", height, n, lines)
			}
		}
	}
}

// The spinner block costs rows too: a streaming build (Loading with output
// already accumulated) must not overflow either.
func TestOutputViewFitsWhileLoading(t *testing.T) {
	for _, height := range []int{outputChrome + 4, 24, 40} {
		a := newOutputApp(height, 100, 200)
		a.Loading = true
		a.LoadingText = "Rebuilding image and restarting..."
		if lines := strings.Count(a.View(), "\n") + 1; lines > height {
			t.Errorf("height=%d: loading view is %d rows, taller than the terminal", height, lines)
		}
	}
}

// Stats puts its TOTAL row last and streams put the newest line last, so a
// freshly loaded screen must open pinned to the bottom (the old behaviour).
func TestOutputOpensAtTheBottom(t *testing.T) {
	a := newOutputApp(24, 100, 200)
	view := a.viewOutput()
	if !strings.Contains(view, "line-0199") {
		t.Error("the last line is not visible when the screen opens")
	}
	if !strings.Contains(view, "more above") {
		t.Error("expected a 'more above' marker with the window at the bottom")
	}
}

// The whole point: the head of a long output has to be reachable.
func TestOutputScrollReachesTheTop(t *testing.T) {
	a := newOutputApp(24, 100, 200)
	a.handleOutputKeys(tea.KeyMsg{Type: tea.KeyHome})
	view := a.viewOutput()
	if !strings.Contains(view, "line-0000") {
		t.Error("the first line is unreachable with home/g")
	}
	if !strings.Contains(view, "more below") {
		t.Error("expected a 'more below' marker with the window at the top")
	}
}

// Every line must show up at some scroll position while paging through.
func TestEveryOutputLineIsReachable(t *testing.T) {
	a := newOutputApp(24, 100, 137)
	a.handleOutputKeys(tea.KeyMsg{Type: tea.KeyHome})

	seen := map[string]bool{}
	for step := 0; step < 200; step++ {
		view := a.viewOutput()
		for i := 0; i < 137; i++ {
			if strings.Contains(view, fmt.Sprintf("line-%04d", i)) {
				seen[fmt.Sprintf("line-%04d", i)] = true
			}
		}
		a.handleOutputKeys(tea.KeyMsg{Type: tea.KeyDown})
	}
	if len(seen) != 137 {
		t.Errorf("only %d of 137 output lines were reachable by scrolling", len(seen))
	}
}

// Paging keys must never run off either end, whatever the output size.
func TestOutputPagingStaysInBounds(t *testing.T) {
	keys := []tea.KeyMsg{
		{Type: tea.KeyPgDown}, {Type: tea.KeyPgDown}, {Type: tea.KeyEnd},
		{Type: tea.KeyDown}, {Type: tea.KeyDown}, {Type: tea.KeyPgDown},
		{Type: tea.KeyHome}, {Type: tea.KeyPgUp}, {Type: tea.KeyUp},
		{Type: tea.KeyUp}, {Type: tea.KeyPgUp}, {Type: tea.KeyDown},
	}
	for _, n := range []int{0, 1, 9, 73, 400} {
		a := newOutputApp(24, 100, n)
		for i, k := range keys {
			a.handleOutputKeys(k)
			a.clampOutputScroll()
			if a.OutputScroll < 0 {
				t.Fatalf("lines=%d after key %d: negative scroll %d", n, i, a.OutputScroll)
			}
			if total := len(a.outputDisplayLines()); a.OutputScroll > total {
				t.Fatalf("lines=%d after key %d: scroll %d past the end (%d)", n, i, a.OutputScroll, total)
			}
			if lines := strings.Count(a.View(), "\n") + 1; lines > a.Height {
				t.Fatalf("lines=%d after key %d: view is %d rows for a %d-row terminal", n, i, lines, a.Height)
			}
		}
	}
}

// A stats row is wider than a narrow terminal. The window has to be measured in
// the rows the panel actually DRAWS, or wrapping silently pushes the footer off
// screen — which is how the truncated stats table looked in the first place.
func TestOutputWrapsWideLines(t *testing.T) {
	a := &App{Height: 24, Width: 76, Styles: DefaultStyles(), Screen: ScreenOutput}
	a.OutputText = strings.Repeat("2026-08-11  claude-opus-5  181  22.1k  56.3k  15.6M  270.4k\n", 20)
	a.OutputFollow = true

	lines := a.outputDisplayLines()
	if len(lines) <= 20 {
		t.Errorf("wide lines were not wrapped: %d display rows for 20 wrapped rows", len(lines))
	}
	if lines := strings.Count(a.View(), "\n") + 1; lines > a.Height {
		t.Errorf("wrapped view is %d rows, taller than the %d-row terminal", lines, a.Height)
	}
}

// Scrolling up must stop following the tail; jumping to the end must resume it,
// so a streaming build keeps auto-scrolling unless the user took over.
func TestOutputFollowToggles(t *testing.T) {
	a := newOutputApp(24, 100, 200)
	a.handleOutputKeys(tea.KeyMsg{Type: tea.KeyUp})
	if a.OutputFollow {
		t.Error("scrolling up must stop following the tail")
	}
	// New content arriving must not yank the view back down.
	before := a.OutputScroll
	a.OutputText += "\nline-0200"
	a.clampOutputScroll()
	if a.OutputScroll != before {
		t.Errorf("view moved on new output while the user was scrolled up: %d → %d", before, a.OutputScroll)
	}

	a.handleOutputKeys(tea.KeyMsg{Type: tea.KeyEnd})
	if !a.OutputFollow {
		t.Error("end must resume following the tail")
	}
	if !strings.Contains(a.viewOutput(), "line-0200") {
		t.Error("following the tail must show the newest line")
	}
}

// ESC still leaves the screen, and 'r' still refreshes logs — scrolling keys
// must not have stolen them.
func TestOutputEscapeStillWorks(t *testing.T) {
	a := newOutputApp(24, 100, 50)
	handled, _ := a.handleOutputKeys(tea.KeyMsg{Type: tea.KeyEsc})
	if !handled || a.Screen != ScreenDashboard {
		t.Errorf("ESC did not return to the dashboard: handled=%v screen=%v", handled, a.Screen)
	}
}
