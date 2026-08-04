package proxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Token telemetry: Guard already MITMs every AI-provider response, so it can
// meter usage without any cooperation from the CLIs. Each completed response
// is parsed for the provider's usage fields and appended as one JSONL entry,
// which `alcatraz -stats` aggregates into a per-day/per-model report.

type TokenUsage struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	CacheReadTokens  int64 `json:"cache_read_tokens"`
	CacheWriteTokens int64 `json:"cache_write_tokens"`
}

func (u TokenUsage) IsZero() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 &&
		u.CacheReadTokens == 0 && u.CacheWriteTokens == 0
}

type StatsEntry struct {
	Timestamp string `json:"timestamp"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Host      string `json:"host"`
	TokenUsage
}

type StatsLogger struct {
	mu   sync.Mutex
	file *os.File
	path string
}

func NewStatsLogger(path string) (*StatsLogger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	return &StatsLogger{file: f, path: path}, nil
}

func (sl *StatsLogger) Log(entry StatsEntry) {
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	sl.mu.Lock()
	defer sl.mu.Unlock()
	sl.file.Write(data)
	sl.file.Write([]byte{'\n'})
}

func (sl *StatsLogger) Close() error {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.file.Close()
}

// ExtractModel pulls the "model" field from a request body without failing on
// anything else in the payload.
func ExtractModel(body []byte) string {
	var probe struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return ""
	}
	return probe.Model
}

// Usage fields appear near the start (Anthropic message_start) or near the end
// (message_delta, OpenAI/Gemini final chunk) of a response, so keeping a head
// and a tail window is enough even for long SSE streams.
const usageWindow = 256 * 1024

// gzip bodies can't be scanned in windows (the tail is useless without the
// head), so those are buffered whole up to this cap and skipped beyond it.
const gzipBufferCap = 8 * 1024 * 1024

// usageScanner wraps a response body, forwards every byte untouched, and on
// EOF/Close parses the observed bytes for token usage.
type usageScanner struct {
	inner   io.ReadCloser
	gzipped bool
	head    []byte
	tail    []byte // ring-less tail: trimmed to usageWindow as it grows
	gzBuf   []byte
	done    bool
	onUsage func(TokenUsage)
}

func newUsageScanner(inner io.ReadCloser, contentEncoding string, onUsage func(TokenUsage)) io.ReadCloser {
	return &usageScanner{
		inner:   inner,
		gzipped: strings.Contains(strings.ToLower(contentEncoding), "gzip"),
		onUsage: onUsage,
	}
}

func (s *usageScanner) Read(p []byte) (int, error) {
	n, err := s.inner.Read(p)
	if n > 0 {
		s.observe(p[:n])
	}
	if err == io.EOF {
		s.finish()
	}
	return n, err
}

func (s *usageScanner) observe(chunk []byte) {
	if s.gzipped {
		if len(s.gzBuf) < gzipBufferCap {
			s.gzBuf = append(s.gzBuf, chunk...)
		}
		return
	}
	if len(s.head) < usageWindow {
		room := usageWindow - len(s.head)
		if room > len(chunk) {
			room = len(chunk)
		}
		s.head = append(s.head, chunk[:room]...)
		chunk = chunk[room:]
	}
	if len(chunk) > 0 {
		s.tail = append(s.tail, chunk...)
		if len(s.tail) > usageWindow {
			s.tail = s.tail[len(s.tail)-usageWindow:]
		}
	}
}

func (s *usageScanner) Close() error {
	s.finish()
	return s.inner.Close()
}

func (s *usageScanner) finish() {
	if s.done {
		return
	}
	s.done = true

	var text []byte
	if s.gzipped {
		if len(s.gzBuf) >= gzipBufferCap {
			return // too big to decompress safely; skip this response
		}
		gz, err := gzip.NewReader(bytes.NewReader(s.gzBuf))
		if err != nil {
			return
		}
		text, err = io.ReadAll(io.LimitReader(gz, gzipBufferCap))
		if err != nil && len(text) == 0 {
			return
		}
	} else {
		text = append(s.head, s.tail...)
	}

	usage := ParseUsage(string(text))
	if !usage.IsZero() && s.onUsage != nil {
		s.onUsage(usage)
	}
}

// Per-provider usage fields. Values are taken from the LAST occurrence: in
// SSE streams the final event carries the definitive counts (Anthropic
// message_delta, OpenAI/Gemini final chunk).
var (
	reInputTokens      = regexp.MustCompile(`"input_tokens"\s*:\s*(\d+)`)
	reOutputTokens     = regexp.MustCompile(`"output_tokens"\s*:\s*(\d+)`)
	reCacheReadTokens  = regexp.MustCompile(`"cache_read_input_tokens"\s*:\s*(\d+)`)
	reCacheWriteTokens = regexp.MustCompile(`"cache_creation_input_tokens"\s*:\s*(\d+)`)

	rePromptTokens     = regexp.MustCompile(`"prompt_tokens"\s*:\s*(\d+)`)
	reCompletionTokens = regexp.MustCompile(`"completion_tokens"\s*:\s*(\d+)`)
	reCachedTokens     = regexp.MustCompile(`"cached_tokens"\s*:\s*(\d+)`)

	rePromptTokenCount     = regexp.MustCompile(`"promptTokenCount"\s*:\s*(\d+)`)
	reCandidatesTokenCount = regexp.MustCompile(`"candidatesTokenCount"\s*:\s*(\d+)`)
	reCachedContentTokens  = regexp.MustCompile(`"cachedContentTokenCount"\s*:\s*(\d+)`)

	// Vercel AI SDK / opencode-go camelCase usage fields.
	rePromptTokensCamel     = regexp.MustCompile(`"promptTokens"\s*:\s*(\d+)`)
	reCompletionTokensCamel = regexp.MustCompile(`"completionTokens"\s*:\s*(\d+)`)
	reTotalTokensCamel      = regexp.MustCompile(`"totalTokens"\s*:\s*(\d+)`)
)

func lastMatch(re *regexp.Regexp, text string) int64 {
	matches := re.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return 0
	}
	v, _ := strconv.ParseInt(matches[len(matches)-1][1], 10, 64)
	return v
}

// ParseUsage extracts token usage from a response body (plain JSON or SSE),
// covering the Anthropic, OpenAI, Gemini, and Vercel AI SDK (camelCase) field names.
func ParseUsage(text string) TokenUsage {
	u := TokenUsage{
		InputTokens:      lastMatch(reInputTokens, text),
		OutputTokens:     lastMatch(reOutputTokens, text),
		CacheReadTokens:  lastMatch(reCacheReadTokens, text),
		CacheWriteTokens: lastMatch(reCacheWriteTokens, text),
	}
	if u.InputTokens == 0 && u.OutputTokens == 0 {
		u.InputTokens = lastMatch(rePromptTokens, text)
		u.OutputTokens = lastMatch(reCompletionTokens, text)
		u.CacheReadTokens = lastMatch(reCachedTokens, text)
	}
	if u.InputTokens == 0 && u.OutputTokens == 0 {
		u.InputTokens = lastMatch(rePromptTokenCount, text)
		u.OutputTokens = lastMatch(reCandidatesTokenCount, text)
		u.CacheReadTokens = lastMatch(reCachedContentTokens, text)
	}
	if u.InputTokens == 0 && u.OutputTokens == 0 {
		u.InputTokens = lastMatch(rePromptTokensCamel, text)
		u.OutputTokens = lastMatch(reCompletionTokensCamel, text)
	}
	return u
}

// ===== aggregation (the `-stats` flag) =====

// Cost in currency is deliberately NOT reported: only the provider knows the
// real price of a request (live pricing, volume/batch discounts, promos,
// per-provider cache multipliers). Token counts below are the source of
// truth — they are parsed verbatim from the provider's own response.

type statsKey struct {
	Date  string
	Model string
}

type statsAgg struct {
	Requests int64
	Usage    TokenUsage
}

// PrintStats reads the JSONL stats file and prints a per-day/per-model
// aggregate table followed by an all-time total.
func PrintStats(path string, w io.Writer) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(w, "No usage recorded yet. Stats accumulate as AI CLIs make requests through the Guard.")
			return nil
		}
		return err
	}

	agg := map[statsKey]*statsAgg{}
	total := statsAgg{}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e StatsEntry
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		model := e.Model
		if model == "" {
			model = "(" + e.Provider + ")"
		}
		date := e.Timestamp
		if len(date) >= 10 {
			date = date[:10]
		}
		k := statsKey{Date: date, Model: model}
		a := agg[k]
		if a == nil {
			a = &statsAgg{}
			agg[k] = a
		}
		for _, dst := range []*statsAgg{a, &total} {
			dst.Requests++
			dst.Usage.InputTokens += e.InputTokens
			dst.Usage.OutputTokens += e.OutputTokens
			dst.Usage.CacheReadTokens += e.CacheReadTokens
			dst.Usage.CacheWriteTokens += e.CacheWriteTokens
		}
	}

	if len(agg) == 0 {
		fmt.Fprintln(w, "No usage recorded yet. Stats accumulate as AI CLIs make requests through the Guard.")
		return nil
	}

	keys := make([]statsKey, 0, len(agg))
	for k := range agg {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Date != keys[j].Date {
			return keys[i].Date < keys[j].Date
		}
		return keys[i].Model < keys[j].Model
	})

	fmt.Fprintf(w, "%-10s  %-34s  %8s  %12s  %12s  %12s  %12s\n",
		"DATE", "MODEL", "REQS", "INPUT", "OUTPUT", "CACHE READ", "CACHE WRITE")
	for _, k := range keys {
		a := agg[k]
		fmt.Fprintf(w, "%-10s  %-34s  %8d  %12s  %12s  %12s  %12s\n",
			k.Date, truncate(k.Model, 34), a.Requests,
			fmtTokens(a.Usage.InputTokens), fmtTokens(a.Usage.OutputTokens),
			fmtTokens(a.Usage.CacheReadTokens), fmtTokens(a.Usage.CacheWriteTokens))
	}
	fmt.Fprintf(w, "%-10s  %-34s  %8d  %12s  %12s  %12s  %12s\n",
		"TOTAL", "", total.Requests,
		fmtTokens(total.Usage.InputTokens), fmtTokens(total.Usage.OutputTokens),
		fmtTokens(total.Usage.CacheReadTokens), fmtTokens(total.Usage.CacheWriteTokens))
	fmt.Fprintln(w, "\nToken counts are actual values parsed from provider responses. Cost is intentionally not shown — only the provider can price a request accurately.")
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func fmtTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	default:
		return strconv.FormatInt(n, 10)
	}
}

