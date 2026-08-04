package proxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseUsageAnthropicSSE(t *testing.T) {
	body := `event: message_start
data: {"type":"message_start","message":{"model":"claude-sonnet-4","usage":{"input_tokens":1200,"cache_creation_input_tokens":300,"cache_read_input_tokens":9000,"output_tokens":2}}}

event: content_block_delta
data: {"type":"content_block_delta","delta":{"text":"hello"}}

event: message_delta
data: {"type":"message_delta","usage":{"output_tokens":450}}
`
	u := ParseUsage(body)
	if u.InputTokens != 1200 {
		t.Errorf("input = %d, want 1200", u.InputTokens)
	}
	if u.OutputTokens != 450 {
		t.Errorf("output = %d, want 450 (last occurrence wins)", u.OutputTokens)
	}
	if u.CacheReadTokens != 9000 {
		t.Errorf("cache read = %d, want 9000", u.CacheReadTokens)
	}
	if u.CacheWriteTokens != 300 {
		t.Errorf("cache write = %d, want 300", u.CacheWriteTokens)
	}
}

func TestParseUsageOpenAI(t *testing.T) {
	body := `{"id":"chatcmpl-1","usage":{"prompt_tokens":800,"completion_tokens":120,"total_tokens":920,"prompt_tokens_details":{"cached_tokens":500}}}`
	u := ParseUsage(body)
	if u.InputTokens != 800 || u.OutputTokens != 120 || u.CacheReadTokens != 500 {
		t.Errorf("got %+v, want 800/120/500", u)
	}
}

func TestParseUsageGemini(t *testing.T) {
	body := `data: {"candidates":[{"content":{}}],"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":3}}
data: {"candidates":[{"content":{}}],"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":77,"cachedContentTokenCount":10}}
`
	u := ParseUsage(body)
	if u.InputTokens != 50 || u.OutputTokens != 77 || u.CacheReadTokens != 10 {
		t.Errorf("got %+v, want 50/77/10", u)
	}
}

func TestParseUsageCamelCase(t *testing.T) {
	body := `data: {"text":"hello","usage":{"promptTokens":123,"completionTokens":45,"totalTokens":168}}
`
	u := ParseUsage(body)
	if u.InputTokens != 123 || u.OutputTokens != 45 {
		t.Errorf("got %+v, want 123/45", u)
	}
}

func TestParseUsageNoUsage(t *testing.T) {
	if u := ParseUsage(`{"ok":true}`); !u.IsZero() {
		t.Errorf("expected zero usage, got %+v", u)
	}
}

func TestExtractModel(t *testing.T) {
	if m := ExtractModel([]byte(`{"model":"claude-sonnet-4-5","messages":[]}`)); m != "claude-sonnet-4-5" {
		t.Errorf("model = %q", m)
	}
	if m := ExtractModel([]byte(`not json`)); m != "" {
		t.Errorf("model = %q, want empty", m)
	}
}

func TestUsageScannerPassthroughAndCallback(t *testing.T) {
	body := `{"usage":{"input_tokens":10,"output_tokens":20}}`
	var got TokenUsage
	sc := newUsageScanner(io.NopCloser(strings.NewReader(body)), "", func(u TokenUsage) { got = u })

	out, err := io.ReadAll(sc)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != body {
		t.Errorf("body was altered: %q", out)
	}
	sc.Close()
	if got.InputTokens != 10 || got.OutputTokens != 20 {
		t.Errorf("callback usage = %+v", got)
	}
}

func TestUsageScannerCallbackFiresOnce(t *testing.T) {
	calls := 0
	sc := newUsageScanner(io.NopCloser(strings.NewReader(`{"input_tokens":1,"output_tokens":1}`)), "", func(TokenUsage) { calls++ })
	io.ReadAll(sc) // EOF fires the callback
	sc.Close()     // Close must not fire it again
	if calls != 1 {
		t.Errorf("callback fired %d times, want 1", calls)
	}
}

func TestUsageScannerGzip(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	gz.Write([]byte(`{"usage":{"input_tokens":33,"output_tokens":44}}`))
	gz.Close()

	var got TokenUsage
	sc := newUsageScanner(io.NopCloser(bytes.NewReader(buf.Bytes())), "gzip", func(u TokenUsage) { got = u })
	out, _ := io.ReadAll(sc)
	sc.Close()

	if !bytes.Equal(out, buf.Bytes()) {
		t.Error("gzip body was altered in transit")
	}
	if got.InputTokens != 33 || got.OutputTokens != 44 {
		t.Errorf("usage = %+v, want 33/44", got)
	}
}

func TestUsageScannerLongStreamKeepsTail(t *testing.T) {
	// Usage arrives at the very end of a stream larger than the head window.
	filler := strings.Repeat("data: {\"type\":\"content_block_delta\"}\n", 20000)
	body := filler + `data: {"type":"message_delta","usage":{"output_tokens":999}}` + "\n" +
		`data: {"type":"message_start_x","usage":{"input_tokens":5}}` + "\n"
	var got TokenUsage
	sc := newUsageScanner(io.NopCloser(strings.NewReader(body)), "", func(u TokenUsage) { got = u })
	io.Copy(io.Discard, sc)
	sc.Close()
	if got.OutputTokens != 999 {
		t.Errorf("output = %d, want 999 (tail window lost the final event)", got.OutputTokens)
	}
}

func TestStatsLoggerWritesJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.jsonl")
	sl, err := NewStatsLogger(path)
	if err != nil {
		t.Fatal(err)
	}
	sl.Log(StatsEntry{Provider: "anthropic", Model: "claude-sonnet-4", TokenUsage: TokenUsage{InputTokens: 1, OutputTokens: 2}})
	sl.Close()

	data, _ := os.ReadFile(path)
	var e StatsEntry
	if err := json.Unmarshal(bytes.TrimSpace(data), &e); err != nil {
		t.Fatalf("invalid JSONL: %v", err)
	}
	if e.Timestamp == "" || e.Model != "claude-sonnet-4" || e.OutputTokens != 2 {
		t.Errorf("entry = %+v", e)
	}
}

func TestPrintStatsAggregates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.jsonl")
	sl, _ := NewStatsLogger(path)
	sl.Log(StatsEntry{Timestamp: "2026-07-04T10:00:00Z", Provider: "anthropic", Model: "claude-sonnet-4", TokenUsage: TokenUsage{InputTokens: 1000, OutputTokens: 500}})
	sl.Log(StatsEntry{Timestamp: "2026-07-04T11:00:00Z", Provider: "anthropic", Model: "claude-sonnet-4", TokenUsage: TokenUsage{InputTokens: 2000, OutputTokens: 1500}})
	sl.Log(StatsEntry{Timestamp: "2026-07-03T09:00:00Z", Provider: "openai", Model: "", TokenUsage: TokenUsage{InputTokens: 10, OutputTokens: 20}})
	sl.Close()

	var out bytes.Buffer
	if err := PrintStats(path, &out); err != nil {
		t.Fatal(err)
	}
	report := out.String()
	for _, want := range []string{"claude-sonnet-4", "(openai)", "TOTAL", "2026-07-04", "2026-07-03"} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
	// 2 sonnet requests on the same day must aggregate into one row
	if strings.Count(report, "claude-sonnet-4") != 1 {
		t.Errorf("expected a single aggregated sonnet row:\n%s", report)
	}
}

func TestPrintStatsMissingFile(t *testing.T) {
	var out bytes.Buffer
	if err := PrintStats(filepath.Join(t.TempDir(), "nope.jsonl"), &out); err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if !strings.Contains(out.String(), "No usage recorded yet") {
		t.Errorf("unexpected output: %s", out.String())
	}
}
