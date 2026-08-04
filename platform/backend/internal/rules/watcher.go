package rules

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
)

const reloadDebounce = 500 * time.Millisecond

// Status is a snapshot of the watcher's health, for CLI/TUI reporting.
type Status struct {
	Path       string
	RuleCount  int
	AllowCount int
	Strict     bool
	LastReload time.Time
	LastError  string
}

// Watcher holds the current compiled RuleSet and hot-reloads it when the rules
// file changes. Reloads are fail-safe: a parse error keeps the last valid set
// and records the error rather than degrading to zero rules.
type Watcher struct {
	path    string
	log     zerolog.Logger
	current atomic.Pointer[RuleSet]

	mu         sync.Mutex
	lastReload time.Time
	lastError  string
}

// NewWatcher creates a watcher for path and loads the initial RuleSet
// synchronously. A missing or invalid file still yields a usable (empty)
// RuleSet so the Guard never starts with zero protection.
func NewWatcher(path string, log zerolog.Logger) *Watcher {
	w := &Watcher{path: path, log: log}
	rs, err := Load(path)
	if err != nil {
		log.Warn().Err(err).Str("path", path).Msg("guard rules: initial load failed, starting with empty rule set")
		rs = &RuleSet{}
		w.lastError = err.Error()
	}
	w.current.Store(rs)
	w.lastReload = time.Now()
	log.Info().Str("path", path).Int("rules", rs.RuleCount()).Bool("strict", rs.Strict).Msg("guard rules loaded")
	return w
}

// Current returns the active RuleSet. Never nil after NewWatcher.
func (w *Watcher) Current() *RuleSet {
	return w.current.Load()
}

// Status returns a health snapshot.
func (w *Watcher) Status() Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	rs := w.current.Load()
	return Status{
		Path:       w.path,
		RuleCount:  rs.RuleCount(),
		AllowCount: rs.AllowCount(),
		Strict:     rs.Strict,
		LastReload: w.lastReload,
		LastError:  w.lastError,
	}
}

// Start watches the rules file's directory for changes and reloads on save,
// with debouncing. It blocks until ctx is cancelled. Watching the directory
// (not the file) survives editor atomic-rename saves and file creation.
func (w *Watcher) Start(ctx context.Context) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()

	dir := filepath.Dir(w.path)
	if err := watcher.Add(dir); err != nil {
		// Directory may not exist (rules never configured). Not fatal —
		// the Guard runs on built-in patterns.
		w.log.Warn().Err(err).Str("dir", dir).Msg("guard rules: cannot watch directory, hot-reload disabled")
		<-ctx.Done()
		return nil
	}

	var timer *time.Timer
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if filepath.Clean(event.Name) != filepath.Clean(w.path) {
				continue
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(reloadDebounce, w.reload)
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			w.log.Warn().Err(err).Msg("guard rules: fsnotify error")
		}
	}
}

func (w *Watcher) reload() {
	rs, err := Load(w.path)
	w.mu.Lock()
	defer w.mu.Unlock()
	if err != nil {
		w.lastError = err.Error()
		w.log.Error().Err(err).Str("path", w.path).Msg("rules_reload_failed — keeping last valid rule set")
		return
	}
	w.current.Store(rs)
	w.lastReload = time.Now()
	w.lastError = ""
	w.log.Info().Str("path", w.path).Int("rules", rs.RuleCount()).Bool("strict", rs.Strict).Msg("rules_reloaded")
}
