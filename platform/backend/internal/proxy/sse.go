package proxy

import (
	"bytes"
	"encoding/json"
	"io"
)

// SSE-aware detokenization.
//
// The plain byte-level detokenizer (detokReader) cannot restore a token the
// model echoes because a streaming response splits that token across several
// SSE events — "[[", "ALCZ", "-9f…", each inside its own `data: {…"text":"…"}`
// event, with JSON/SSE framing in between. A byte regex on the wire never sees
// the whole token.
//
// sseDetok reassembles the model's output text ACROSS events: it pulls the text
// fragment out of each delta event, feeds it through a text-level detokenizer
// that holds back only a trailing run that could still become a token, restores
// any complete token in the assembled text, and re-emits the (possibly shorter
// or expanded) text in place — keeping every other field and the event framing
// intact. Held-back text is flushed before a block/message terminator and at
// EOF, so nothing is ever dropped. Non-text events pass through untouched.
type sseDetok struct {
	src     io.ReadCloser
	vault   *Vault
	in      bytes.Buffer // raw bytes not yet split into whole events
	out     bytes.Buffer // rewritten bytes ready to emit
	pending string       // assembled text held back (possible partial token)
	tmplPre []byte       // bytes of the last text event before its text value
	tmplSuf []byte       // bytes of the last text event after its text value
	hasTmpl bool
	eof     bool
}

// NewSSEDetokReader wraps a decompressed text/event-stream body so vault tokens
// the model echoed — even split across SSE deltas — are restored before the
// AI CLI reads them.
func (v *Vault) NewSSEDetokReader(src io.ReadCloser) io.ReadCloser {
	return &sseDetok{src: src, vault: v}
}

func (r *sseDetok) Read(p []byte) (int, error) {
	for r.out.Len() == 0 && !r.eof {
		buf := make([]byte, 32*1024)
		n, err := r.src.Read(buf)
		if n > 0 {
			r.in.Write(buf[:n])
			r.process(false)
		}
		if err == io.EOF {
			r.eof = true
			r.process(true)
		} else if err != nil {
			if r.out.Len() == 0 {
				return 0, err
			}
			break
		}
	}
	if r.out.Len() > 0 {
		return r.out.Read(p)
	}
	if r.eof {
		return 0, io.EOF
	}
	return 0, nil
}

func (r *sseDetok) Close() error { return r.src.Close() }

// process consumes every complete event currently buffered. On final it also
// handles a trailing event with no delimiter and flushes any held-back text.
func (r *sseDetok) process(final bool) {
	data := r.in.Bytes()
	consumed := 0
	for {
		end, ok := nextEventBoundary(data[consumed:])
		if !ok {
			break
		}
		r.out.Write(r.handleEvent(data[consumed : consumed+end]))
		consumed += end
	}
	rem := append([]byte(nil), data[consumed:]...)
	r.in.Reset()
	r.in.Write(rem)

	if final {
		if r.in.Len() > 0 {
			r.out.Write(r.handleEvent(r.in.Bytes()))
			r.in.Reset()
		}
		if r.pending != "" {
			r.out.Write(r.flushPending())
			r.pending = ""
		}
	}
}

// handleEvent rewrites one SSE event (including its trailing delimiter). Text
// deltas get their text reassembled/detokenized; other events pass through, but
// a terminator first flushes any held-back text so the block's text is complete.
func (r *sseDetok) handleEvent(ev []byte) []byte {
	valStart, valEnd, ok := findTextValue(ev)
	if !ok {
		if r.pending != "" && isTerminator(ev) {
			flushed := r.flushPending()
			r.pending = ""
			return append(flushed, ev...)
		}
		return append([]byte(nil), ev...)
	}

	frag := jsonUnescapeInner(string(ev[valStart:valEnd]))
	safe := r.pushText(frag)

	r.tmplPre = append(r.tmplPre[:0], ev[:valStart]...)
	r.tmplSuf = append(r.tmplSuf[:0], ev[valEnd:]...)
	r.hasTmpl = true

	newEsc := jsonEscapeInner(safe)
	out := make([]byte, 0, valStart+len(newEsc)+len(ev)-valEnd)
	out = append(out, ev[:valStart]...)
	out = append(out, newEsc...)
	out = append(out, ev[valEnd:]...)
	return out
}

// pushText appends a fragment, restores complete tokens in the assembled text,
// and returns the part safe to emit now, holding back a suffix that could still
// grow into a token.
func (r *sseDetok) pushText(frag string) string {
	r.pending += frag
	det := r.vault.Detokenize(r.pending)
	keep := incompleteTokenSuffix(det)
	safe := det[:len(det)-keep]
	r.pending = det[len(det)-keep:]
	return safe
}

// flushPending emits the held-back text as a synthetic copy of the last text
// event (same framing/fields, replaced text). Falls back to raw bytes if no
// text event was seen yet (cannot happen when pending is non-empty).
func (r *sseDetok) flushPending() []byte {
	det := r.vault.Detokenize(r.pending)
	if !r.hasTmpl {
		return []byte(det)
	}
	esc := jsonEscapeInner(det)
	out := make([]byte, 0, len(r.tmplPre)+len(esc)+len(r.tmplSuf))
	out = append(out, r.tmplPre...)
	out = append(out, esc...)
	out = append(out, r.tmplSuf...)
	return out
}

// ── helpers ──────────────────────────────────────────────────────────────────

// nextEventBoundary returns the index just past the first SSE event delimiter
// (a blank line, either "\n\n" or "\r\n\r\n"), or ok=false if none yet.
func nextEventBoundary(b []byte) (int, bool) {
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '\n':
			if i+1 < len(b) && b[i+1] == '\n' {
				return i + 2, true
			}
		case '\r':
			if i+3 < len(b) && b[i+1] == '\n' && b[i+2] == '\r' && b[i+3] == '\n' {
				return i + 4, true
			}
		}
	}
	return 0, false
}

// findTextValue locates the model-text JSON string inside a delta event and
// returns the byte span of its (still-escaped) contents. It keys off a
// provider-specific discriminator so it never rewrites a non-text field.
func findTextValue(ev []byte) (start, end int, ok bool) {
	switch {
	case bytes.Contains(ev, []byte("text_delta")): // Anthropic content_block_delta
		return jsonStringSpan(ev, `"text":"`)
	case bytes.Contains(ev, []byte("output_text.delta")): // OpenAI Responses API
		return jsonStringSpan(ev, `"delta":"`)
	case bytes.Contains(ev, []byte(`"choices"`)): // OpenAI Chat Completions
		return jsonStringSpan(ev, `"content":"`)
	case bytes.Contains(ev, []byte(`"parts"`)): // Gemini
		return jsonStringSpan(ev, `"text":"`)
	}
	return 0, 0, false
}

// jsonStringSpan finds key (which must end at the opening quote of the value)
// and returns the span of the JSON-escaped string contents that follow, up to
// the matching unescaped closing quote.
func jsonStringSpan(ev []byte, key string) (start, end int, ok bool) {
	i := bytes.Index(ev, []byte(key))
	if i < 0 {
		return 0, 0, false
	}
	j := i + len(key)
	for k := j; k < len(ev); k++ {
		if ev[k] == '\\' {
			k++
			continue
		}
		if ev[k] == '"' {
			return j, k, true
		}
	}
	return 0, 0, false
}

// isTerminator reports whether an event ends a content block or the message, at
// which point held-back text must be flushed.
func isTerminator(ev []byte) bool {
	return bytes.Contains(ev, []byte("content_block_stop")) ||
		bytes.Contains(ev, []byte("message_stop")) ||
		bytes.Contains(ev, []byte("[DONE]")) ||
		bytes.Contains(ev, []byte(`"finish_reason":"`)) ||
		bytes.Contains(ev, []byte("finishReason"))
}

// incompleteTokenSuffix returns the length of the longest suffix of s that is a
// proper (incomplete) prefix of a vault token — the run to hold back so a token
// split across events is not emitted half-formed.
func incompleteTokenSuffix(s string) int {
	max := vaultTokenLen - 1
	if max > len(s) {
		max = len(s)
	}
	for l := max; l >= 1; l-- {
		if isTokenPrefix(s[len(s)-l:]) {
			return l
		}
	}
	return 0
}

// isTokenPrefix reports whether s matches the leading characters of a vault
// token and is shorter than a full one.
func isTokenPrefix(s string) bool {
	if len(s) >= vaultTokenLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !tokenCharOK(i, s[i]) {
			return false
		}
	}
	return true
}

func tokenCharOK(pos int, c byte) bool {
	switch {
	case pos <= 1:
		return c == '['
	case pos == 2:
		return c == 'A'
	case pos == 3:
		return c == 'L'
	case pos == 4:
		return c == 'C'
	case pos == 5:
		return c == 'Z'
	case pos == 6:
		return c == '-'
	case pos >= 7 && pos <= 22:
		return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
	case pos == 23:
		return c == ']'
	}
	return false
}

func jsonUnescapeInner(esc string) string {
	var s string
	if err := json.Unmarshal([]byte(`"`+esc+`"`), &s); err != nil {
		return esc
	}
	return s
}

func jsonEscapeInner(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // don't turn < > & into \u00xx — keep bytes minimal
	if err := enc.Encode(s); err != nil {
		return s
	}
	b := bytes.TrimRight(buf.Bytes(), "\n")
	if len(b) < 2 {
		return s
	}
	return string(b[1 : len(b)-1])
}
