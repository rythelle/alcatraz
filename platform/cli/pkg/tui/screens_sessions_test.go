package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestParseSessionsTSV(t *testing.T) {
	out := "some stderr noise line\n" +
		"Claude\tabc123\t/workspace/projects/retro-job-hub\t1783218780\tretro-job-hub\tcd /workspace/projects/retro-job-hub; claude --resume abc123\n" +
		"Codex\t\t\t1783200000\t2 session(s) — native picker\tcodex resume\n" +
		"\t\t\t\t\t\n" // malformed row: dropped

	items := parseSessionsTSV(out)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(items), items)
	}
	// Newest first: Claude (1783218780) before Codex (1783200000).
	if items[0].Tool != "Claude" || items[1].Tool != "Codex" {
		t.Fatalf("wrong order: %s then %s", items[0].Tool, items[1].Tool)
	}
	c := items[0]
	if c.ID != "abc123" || c.Cwd != "/workspace/projects/retro-job-hub" || c.Label != "retro-job-hub" {
		t.Fatalf("Claude fields wrong: %+v", c)
	}
	if c.Resume != "cd /workspace/projects/retro-job-hub; claude --resume abc123" {
		t.Fatalf("Claude resume wrong: %q", c.Resume)
	}
	if c.Epoch != 1783218780 {
		t.Fatalf("Claude epoch wrong: %d", c.Epoch)
	}
}

func TestClip(t *testing.T) {
	if got := clip("hello", 10); got != "hello" {
		t.Errorf("clip short: %q", got)
	}
	if got := clip("hello world", 5); got != "hell…" {
		t.Errorf("clip long: %q", got)
	}
}

// The list used to render every item, so on a normal terminal the newest
// sessions were pushed off screen with no way to reach them.
func TestSessionScrollFollowsCursor(t *testing.T) {
	a := &App{Height: 24}
	for i := 0; i < 50; i++ {
		a.SessionItems = append(a.SessionItems, SessionItem{Tool: "Claude"})
	}
	rows := a.sessionRows()
	if rows != 24-sessionChrome {
		t.Fatalf("expected %d visible rows, got %d", 24-sessionChrome, rows)
	}

	// Moving down inside the window must not scroll it.
	a.SessionCursor = rows - 1
	a.clampSessionScroll()
	if a.SessionScroll != 0 {
		t.Errorf("scrolled early: %d", a.SessionScroll)
	}

	// One past the window scrolls by exactly one.
	a.SessionCursor = rows
	a.clampSessionScroll()
	if a.SessionScroll != 1 {
		t.Errorf("expected scroll 1, got %d", a.SessionScroll)
	}

	// The last item must be reachable and visible.
	a.SessionCursor = len(a.SessionItems) - 1
	a.clampSessionScroll()
	if a.SessionCursor < a.SessionScroll || a.SessionCursor >= a.SessionScroll+rows {
		t.Errorf("last item not visible: cursor=%d scroll=%d rows=%d", a.SessionCursor, a.SessionScroll, rows)
	}
	if a.SessionScroll != len(a.SessionItems)-rows {
		t.Errorf("expected scroll %d, got %d", len(a.SessionItems)-rows, a.SessionScroll)
	}

	// Scrolling back to the top must not leave a gap.
	a.SessionCursor = 0
	a.clampSessionScroll()
	if a.SessionScroll != 0 {
		t.Errorf("expected scroll 0, got %d", a.SessionScroll)
	}
}

// A list shorter than the window never scrolls.
func TestSessionScrollShortList(t *testing.T) {
	a := &App{Height: 40}
	a.SessionItems = []SessionItem{{Tool: "Claude"}, {Tool: "Codex"}}
	a.SessionCursor = 1
	a.clampSessionScroll()
	if a.SessionScroll != 0 {
		t.Errorf("short list scrolled: %d", a.SessionScroll)
	}
}

// Height is 0 until the first WindowSizeMsg; the fallback must still be usable.
func TestSessionRowsBeforeWindowSize(t *testing.T) {
	a := &App{}
	if got := a.sessionRows(); got < 1 {
		t.Errorf("unusable row count before resize: %d", got)
	}
	a.Height = 6 // tiny terminal
	if got := a.sessionRows(); got < 1 {
		t.Errorf("unusable row count on a tiny terminal: %d", got)
	}
}

// newSessionsApp builds the minimum App the sessions view needs.
func newSessionsApp(height, n int) *App {
	a := &App{Height: height, Width: 120, Styles: DefaultStyles()}
	for i := 0; i < n; i++ {
		a.SessionItems = append(a.SessionItems, SessionItem{
			Tool:   "Claude",
			ID:     fmt.Sprintf("sess%04d", i),
			Label:  fmt.Sprintf("project-%d", i),
			Epoch:  int64(1783218780 - i*3600),
			Resume: "claude --resume",
		})
	}
	return a
}

// Regression: the screen rendered every session, so on a real terminal the
// output was taller than the window. The terminal scrolled to the bottom,
// pushing the newest sessions (which sort first) off the top — "não consigo
// ver as sessions mais recentes". The view must now fit the window.
func TestSessionsViewFitsTheTerminal(t *testing.T) {
	// sessionChrome+1 is the smallest window the screen can physically fit in;
	// below that there isn't room for the title, header and footer at all.
	for _, height := range []int{sessionChrome + 1, 24, 40, 60} {
		for _, n := range []int{0, 1, 5, 50, 500} {
			a := newSessionsApp(height, n)
			lines := strings.Count(a.viewSessions(), "\n") + 1
			if lines > height {
				t.Errorf("height=%d items=%d: view is %d lines, taller than the terminal", height, n, lines)
			}
		}
	}
}

// The newest session sorts first, so it must be on screen without scrolling.
func TestNewestSessionVisibleOnOpen(t *testing.T) {
	a := newSessionsApp(24, 200)
	view := a.viewSessions()

	if !strings.Contains(view, "project-0") {
		t.Error("the newest session is not visible when the screen opens")
	}
	if !strings.Contains(view, "▶") {
		t.Error("no cursor marker rendered")
	}
}

// Every session must be reachable by paging down, and the cursor must always
// stay inside the rendered window.
func TestEverySessionIsReachable(t *testing.T) {
	a := newSessionsApp(24, 137)
	rows := a.sessionRows()

	seen := map[string]bool{}
	for step := 0; step < len(a.SessionItems)+rows; step++ {
		a.clampSessionScroll()
		if a.SessionCursor < a.SessionScroll || a.SessionCursor >= a.SessionScroll+rows {
			t.Fatalf("cursor %d outside window [%d,%d)", a.SessionCursor, a.SessionScroll, a.SessionScroll+rows)
		}
		seen[a.SessionItems[a.SessionCursor].ID] = true
		if a.SessionCursor < len(a.SessionItems)-1 {
			a.SessionCursor++
		}
	}
	if len(seen) != len(a.SessionItems) {
		t.Errorf("only %d of %d sessions were reachable", len(seen), len(a.SessionItems))
	}
}

// Paging keys must not run off either end of the list.
func TestSessionPagingStaysInBounds(t *testing.T) {
	keys := map[string]tea.KeyMsg{
		"pgdown": {Type: tea.KeyPgDown},
		"pgup":   {Type: tea.KeyPgUp},
		"home":   {Type: tea.KeyHome},
		"end":    {Type: tea.KeyEnd},
		"up":     {Type: tea.KeyUp},
		"down":   {Type: tea.KeyDown},
	}
	// Hammer the ends: repeated paging past both boundaries, then back.
	script := []string{
		"pgdown", "pgdown", "pgdown", "pgdown", "pgdown",
		"end", "pgdown", "down", "down",
		"home", "pgup", "up", "up",
		"pgdown", "up", "end", "home",
	}

	for _, n := range []int{0, 1, 7, 30, 200} {
		a := newSessionsApp(24, n)
		for _, k := range script {
			handled, _ := a.handleSessionsKeys(keys[k])
			if !handled {
				t.Fatalf("key %q was not handled", k)
			}
			a.clampSessionScroll()

			if n == 0 {
				continue
			}
			if a.SessionCursor < 0 || a.SessionCursor >= n {
				t.Fatalf("items=%d after %q: cursor out of range: %d", n, k, a.SessionCursor)
			}
			if a.SessionScroll < 0 {
				t.Fatalf("items=%d after %q: negative scroll: %d", n, k, a.SessionScroll)
			}
			rows := a.sessionRows()
			if a.SessionCursor < a.SessionScroll || a.SessionCursor >= a.SessionScroll+rows {
				t.Fatalf("items=%d after %q: cursor %d outside window [%d,%d)",
					n, k, a.SessionCursor, a.SessionScroll, a.SessionScroll+rows)
			}
			// Rendering must stay inside the terminal at every step.
			if lines := strings.Count(a.viewSessions(), "\n") + 1; lines > a.Height {
				t.Fatalf("items=%d after %q: view is %d lines for a %d-line terminal", n, k, lines, a.Height)
			}
		}
	}
}

// An empty list must not panic or produce a negative scroll.
func TestSessionsEmptyList(t *testing.T) {
	a := newSessionsApp(24, 0)
	a.clampSessionScroll()
	if a.SessionScroll != 0 || a.SessionCursor != 0 {
		t.Errorf("empty list: cursor=%d scroll=%d", a.SessionCursor, a.SessionScroll)
	}
	if view := a.viewSessions(); !strings.Contains(view, "No resumable sessions yet") {
		t.Error("empty state not rendered")
	}
}

// Regression: the highlighted row is re-styled character by character, so the
// dimmed ID cell nested inside it had its own escape sequence chopped up and
// printed as literal text — "[3;38;5;60m[0m" sat in the ID column of whichever
// row the cursor was on. The selected row must not nest a style inside itself.
func TestSelectedSessionRowHasNoLeakedEscapes(t *testing.T) {
	lipgloss.SetColorProfile(termenv.ANSI256)
	defer lipgloss.SetColorProfile(termenv.Ascii)

	ansiSeq := regexp.MustCompile("\x1b\\[[0-9;]*m")
	for _, id := range []string{"", "4e082cf1-aaaa-bbbb"} {
		a := &App{Height: 24, Width: 120, Styles: DefaultStyles()}
		a.SessionItems = []SessionItem{{Tool: "opencode", ID: id, Label: "sandbox", Epoch: 1786484867}}

		for _, line := range strings.Split(a.viewSessions(), "\n") {
			if !strings.Contains(ansiSeq.ReplaceAllString(line, ""), "opencode") {
				continue
			}
			visible := ansiSeq.ReplaceAllString(line, "")
			if strings.Contains(visible, "38;5") || strings.Contains(visible, "[0m") {
				t.Errorf("id=%q: escape sequence leaked into the visible row: %q", id, visible)
			}
		}
	}
}

// The counters tell the user there is more list off-screen.
func TestSessionsShowMoreMarkers(t *testing.T) {
	a := newSessionsApp(24, 100)
	if view := a.viewSessions(); !strings.Contains(view, "more below") {
		t.Error("expected a 'more below' marker at the top of a long list")
	}
	a.SessionCursor = 99
	a.clampSessionScroll()
	if view := a.viewSessions(); !strings.Contains(view, "more above") {
		t.Error("expected a 'more above' marker at the bottom of a long list")
	}
}
