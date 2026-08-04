package proxy

import (
	"os"
	"strings"
	"testing"
)

// The Guard runs on every outbound request, so a body-sized regression here is
// felt as latency in every AI call. These benchmarks exist to make that
// visible; `go test -bench Pipeline -benchtime 10x ./internal/proxy/`.

func benchCorpus(tb testing.TB) string {
	raw, err := os.ReadFile("patterns.go")
	if err != nil {
		tb.Skip(err)
	}
	return strings.Repeat(string(raw), 4)
}

// Source code: keyword-dense, so many patterns legitimately have to run.
func BenchmarkPipelineSource(b *testing.B) {
	src := benchCorpus(b)
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sanitizePipeline(src, nil, nil, false)
	}
}

// Prose: the common case for chat messages, where the prefilter should skip
// nearly every pattern.
func BenchmarkPipelineProse(b *testing.B) {
	src := strings.Repeat("The quick brown fox jumps over the lazy dog. Please refactor the handler and add a test. ", 1300)
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sanitizePipeline(src, nil, nil, false)
	}
}

func BenchmarkPrefilterSweep(b *testing.B) {
	src := benchCorpus(b)
	lower := strings.ToLower(src)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, sp := range SensitivePatterns {
			canSkip(sp.Name, lower)
		}
	}
}
