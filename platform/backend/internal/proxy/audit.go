package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type AuditEntry struct {
	Timestamp    string      `json:"timestamp"`
	RequestID    string      `json:"request_id"`
	Host         string      `json:"host"`
	Path         string      `json:"path"`
	Method       string      `json:"method"`
	Provider     string      `json:"provider"`
	Detections   []Detection `json:"detections"`
	RequestSize  int         `json:"request_size"`
	Sanitized    bool        `json:"sanitized"`
	DryRun       bool        `json:"dry_run"`
}

type AuditLogger struct {
	path   string
	mu     sync.Mutex
	file   *os.File
	dryRun bool
}

func NewAuditLogger(path string, dryRun bool) (*AuditLogger, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	return &AuditLogger{
		path:   path,
		file:   f,
		dryRun: dryRun,
	}, nil
}

func (al *AuditLogger) Log(entry AuditEntry) {
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	entry.Sanitized = !al.dryRun
	entry.DryRun = al.dryRun

	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	al.mu.Lock()
	defer al.mu.Unlock()

	al.file.Write(data)
	al.file.Write([]byte{'\n'})
}

func (al *AuditLogger) Close() error {
	al.mu.Lock()
	defer al.mu.Unlock()
	return al.file.Close()
}
