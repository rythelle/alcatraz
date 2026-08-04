package guard

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// SummarizeAudit parses the Guard audit log (one JSON object per line) and
// returns a human-readable summary: total sanitized requests, top redaction
// patterns, and a per-provider breakdown. Shared by the CLI and TUI so both
// render the same view.
func SummarizeAudit(data []byte) string {
	type entry struct {
		Provider   string `json:"provider"`
		Detections []struct {
			Pattern string `json:"pattern"`
			Count   int    `json:"count"`
		} `json:"detections"`
	}

	perPattern := map[string]int{}
	perProvider := map[string]int{}
	requests := 0

	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		var e entry
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		requests++
		if e.Provider != "" {
			perProvider[e.Provider]++
		}
		for _, d := range e.Detections {
			perPattern[d.Pattern] += d.Count
		}
	}

	if requests == 0 {
		return "No redactions recorded yet."
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "Sanitized requests: %d\n\n", requests)
	b.WriteString("Top redaction patterns:\n")
	for _, kv := range topN(perPattern, 15) {
		fmt.Fprintf(&b, "  %5d  %s\n", kv.v, kv.k)
	}
	b.WriteString("\nBy provider:\n")
	for _, kv := range topN(perProvider, 10) {
		fmt.Fprintf(&b, "  %5d  %s\n", kv.v, kv.k)
	}
	return b.String()
}

type kvPair struct {
	k string
	v int
}

func topN(m map[string]int, n int) []kvPair {
	pairs := make([]kvPair, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, kvPair{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].v != pairs[j].v {
			return pairs[i].v > pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})
	if len(pairs) > n {
		pairs = pairs[:n]
	}
	return pairs
}
