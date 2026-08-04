package rules

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func waitFor(t *testing.T, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestWatcher_HotReloadAndFailSafe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "guard-rules.yml")
	if err := os.WriteFile(path, []byte("redact:\n  - name: a\n    literal: one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	w := NewWatcher(path, zerolog.Nop())
	if w.Current().RuleCount() != 1 {
		t.Fatalf("initial RuleCount = %d, want 1", w.Current().RuleCount())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Start(ctx)
	// Give the watcher goroutine time to register the inotify watch before we
	// edit the file, so the first WRITE event is not missed.
	time.Sleep(300 * time.Millisecond)

	// Concurrent readers to exercise the atomic pointer under -race.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				_ = w.Current().RuleCount()
			}
		}
	}()

	// Valid edit → hot reload picks up the new rule.
	if err := os.WriteFile(path, []byte("redact:\n  - name: a\n    literal: one\n  - name: b\n    literal: two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, func() bool { return w.Current().RuleCount() == 2 }) {
		t.Fatalf("hot reload did not apply: RuleCount = %d", w.Current().RuleCount())
	}

	// Broken edit → last valid set is kept, error recorded.
	if err := os.WriteFile(path, []byte("redact: [broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, func() bool { return w.Status().LastError != "" }) {
		t.Fatal("expected LastError to be set after broken edit")
	}
	if w.Current().RuleCount() != 2 {
		t.Errorf("broken edit degraded rule set: RuleCount = %d, want 2 (last good)", w.Current().RuleCount())
	}
}
