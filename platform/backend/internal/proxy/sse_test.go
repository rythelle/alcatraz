package proxy

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// collectSSEText concatenates the model text from a rewritten SSE stream, the
// way a client would, so tests assert on the reassembled output.
func collectSSEText(out string) string {
	b := []byte(out)
	var sb strings.Builder
	for len(b) > 0 {
		var ev []byte
		if end, ok := nextEventBoundary(b); ok {
			ev, b = b[:end], b[end:]
		} else {
			ev, b = b, nil
		}
		if s, e, ok := findTextValue(ev); ok {
			sb.WriteString(jsonUnescapeInner(string(ev[s:e])))
		}
	}
	return sb.String()
}

func anthropicDelta(text string) string {
	esc, _ := json.Marshal(text)
	return "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":" + string(esc) + "}}\n\n"
}

func openaiDelta(text string) string {
	esc, _ := json.Marshal(text)
	return "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":" + string(esc) + "}}]}\n\n"
}

const (
	evMsgStart  = "event: message_start\ndata: {\"type\":\"message_start\"}\n\n"
	evPing      = "event: ping\ndata: {\"type\":\"ping\"}\n\n"
	evBlockStop = "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"
	evMsgStop   = "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	evDone      = "data: [DONE]\n\n"
)

// splitToken chops s into n roughly-equal pieces (to fragment a token across
// SSE deltas the way a model streams it).
func splitToken(s string, n int) []string {
	var parts []string
	step := (len(s) + n - 1) / n
	for i := 0; i < len(s); i += step {
		end := i + step
		if end > len(s) {
			end = len(s)
		}
		parts = append(parts, s[i:end])
	}
	return parts
}

func runSSE(v *Vault, stream string) string {
	r := v.NewSSEDetokReader(&dribble{data: []byte(stream)})
	out, _ := io.ReadAll(r)
	return collectSSEText(string(out))
}

func TestSSE_TokenSplitAcrossDeltas(t *testing.T) {
	v := NewVault(0)
	tok, _ := v.Tokenize("11.222.333/0001-81")

	var sb strings.Builder
	sb.WriteString(evMsgStart)
	sb.WriteString(anthropicDelta("your CNPJ is "))
	for _, part := range splitToken(tok, 5) { // token in 5 fragments
		sb.WriteString(anthropicDelta(part))
	}
	sb.WriteString(anthropicDelta(" ok"))
	sb.WriteString(evBlockStop)
	sb.WriteString(evMsgStop)

	got := runSSE(v, sb.String())
	if got != "your CNPJ is 11.222.333/0001-81 ok" {
		t.Fatalf("reassembly failed: %q", got)
	}
}

func TestSSE_TokenSplitWithPingBetween(t *testing.T) {
	v := NewVault(0)
	tok, _ := v.Tokenize("secret-cnpj-value")
	parts := splitToken(tok, 3)

	var sb strings.Builder
	sb.WriteString(evMsgStart)
	sb.WriteString(anthropicDelta(parts[0]))
	sb.WriteString(evPing) // ping in the middle of a token must not force a flush
	sb.WriteString(anthropicDelta(parts[1]))
	sb.WriteString(anthropicDelta(parts[2]))
	sb.WriteString(evBlockStop)
	sb.WriteString(evMsgStop)

	if got := runSSE(v, sb.String()); got != "secret-cnpj-value" {
		t.Fatalf("ping split reassembly failed: %q", got)
	}
}

func TestSSE_OpenAIChatSplit(t *testing.T) {
	v := NewVault(0)
	tok, _ := v.Tokenize("4111111111111111")

	var sb strings.Builder
	sb.WriteString(openaiDelta("card: "))
	for _, part := range splitToken(tok, 4) {
		sb.WriteString(openaiDelta(part))
	}
	sb.WriteString(evDone)

	if got := runSSE(v, sb.String()); got != "card: 4111111111111111" {
		t.Fatalf("openai reassembly failed: %q", got)
	}
}

func TestSSE_IncompleteTokenAtEOFFlushed(t *testing.T) {
	v := NewVault(0)
	// A partial token that never completes must be emitted literally, not lost.
	var sb strings.Builder
	sb.WriteString(anthropicDelta("value "))
	sb.WriteString(anthropicDelta("[[ALCZ-dead")) // looks like a token start, never closes
	// stream ends with no terminator

	if got := runSSE(v, sb.String()); got != "value [[ALCZ-dead" {
		t.Fatalf("incomplete token not flushed: %q", got)
	}
}

func TestSSE_PlainTextUnchanged(t *testing.T) {
	v := NewVault(0)
	var sb strings.Builder
	sb.WriteString(evMsgStart)
	sb.WriteString(anthropicDelta("hello, "))
	sb.WriteString(anthropicDelta("no secrets here"))
	sb.WriteString(evBlockStop)
	sb.WriteString(evMsgStop)

	if got := runSSE(v, sb.String()); got != "hello, no secrets here" {
		t.Fatalf("plain text altered: %q", got)
	}
}

func TestSSE_UnknownTokenLeftLiteral(t *testing.T) {
	v := NewVault(0)
	var sb strings.Builder
	sb.WriteString(anthropicDelta("[[ALCZ-deadbeefdeadbeef]] stays"))
	sb.WriteString(evMsgStop)
	if got := runSSE(v, sb.String()); got != "[[ALCZ-deadbeefdeadbeef]] stays" {
		t.Fatalf("unknown token should stay literal: %q", got)
	}
}
