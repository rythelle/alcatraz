package proxy

import (
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Anti-evasion catches sensitive data that has been rewritten into a form the
// literal patterns miss: base64/hex encoding, digits split by separators or
// zero-width characters, full-width / non-ASCII digits, and reversed digit
// strings. It is always on and runs AFTER the literal patterns, so contiguous,
// forward-valid values are already redacted and only transformed
// representations remain.
//
// Every defense here is still gated by the same structural validators as the
// bare patterns (correct CPF/CNPJ/card check digits) or by a successful decode,
// so fake data keeps passing through. The allowlist (`allow:` in the rules
// file) is the escape valve for values that legitimately need to leave.
//
// The one evasion a stateless proxy fundamentally cannot stop is splitting a
// value across SEPARATE requests (e.g. "give me the first 7 digits now, the
// rest later") — there is nothing to correlate across independent HTTP bodies.

const (
	// maxGap is the most connector characters tolerated between two digits
	// before they count as separate numbers. "12 34" and "1.2.3" join; a digit
	// far away does not.
	maxGap = 3
	// maxDecodeDepth bounds recursive decoding (base64 wrapping base64/hex).
	maxDecodeDepth = 3
	// maxCandidates caps how many encoded tokens we decode per pass, so a
	// prose-heavy body can't turn every long word into a decode+scan.
	maxCandidates = 512
	// maxTokenLen skips absurdly long encoded runs (likely real binary blobs).
	maxTokenLen = 1 << 16
)

var (
	base64Candidate = regexp.MustCompile(`[A-Za-z0-9+/=_-]{16,}`)
	hexCandidate    = regexp.MustCompile(`(?:[0-9A-Fa-f]{2}[ :]?){11,}`)
)

// applyAntiEvasion runs the separated-digit and encoded-blob passes over one
// string value, returning the rewritten text and any detections. A non-nil v
// makes each redaction a reversible vault token instead of a static marker.
func applyAntiEvasion(text string, v *Vault) (string, []Detection) {
	dets := make([]Detection, 0, 2)
	if out, n := redactSeparatedDigits(text, v); n > 0 {
		text = out
		dets = append(dets, Detection{Pattern: "evasion_digits", Count: n})
	}
	if out, n := redactEncoded(text, 0, v); n > 0 {
		text = out
		dets = append(dets, Detection{Pattern: "evasion_encoded", Count: n})
	}
	if out, n := redactSplitLiterals(text, v); n > 0 {
		text = out
		dets = append(dets, Detection{Pattern: "evasion_split", Count: n})
	}
	return text, dets
}

type byteSpan struct{ start, end int }

type digitRef struct {
	d          byte // '0'..'9' (already folded from Unicode)
	start, end int  // byte span of the source rune in text
}

// redactSeparatedDigits finds runs of digits joined by up to maxGap connector
// characters (spaces, common punctuation, zero-width marks), folds Unicode
// digits to ASCII, and redacts any window whose digits form a valid CPF, CNPJ,
// PIS, CNS, or payment card — testing the reversed order too, to defeat `rev`.
func redactSeparatedDigits(text string, v *Vault) (string, int) {
	var spans []byteSpan
	refs := make([]digitRef, 0, 32)
	gap := 0

	flush := func() {
		if len(refs) >= 11 {
			spans = append(spans, matchIDs(refs)...)
		}
		refs = refs[:0]
		gap = 0
	}

	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if d, ok := foldDigit(r); ok {
			refs = append(refs, digitRef{d: d, start: i, end: i + size})
			gap = 0
		} else if isConnector(r) {
			gap++
			if gap > maxGap {
				flush()
			}
		} else {
			flush()
		}
		i += size
	}
	flush()

	if len(spans) == 0 {
		return text, 0
	}
	return spliceRedactions(text, spans, func(orig string) string {
		return tokenOrMarker(v, orig, "[REDACTED_BY_ALCATRAZ_ID]")
	}), len(spans)
}

// matchIDs slides validator windows over a group's digits and returns the byte
// spans of every valid ID, longest/most-specific first, without overlap.
func matchIDs(refs []digitRef) []byteSpan {
	n := len(refs)
	buf := make([]byte, n)
	for i := range refs {
		buf[i] = refs[i].d
	}
	s := string(buf)
	used := make([]bool, n)
	var out []byteSpan

	consider := func(width int, ok func(string) bool) {
		if width > n {
			return
		}
		for i := 0; i+width <= n; i++ {
			free := true
			for j := i; j < i+width; j++ {
				if used[j] {
					free = false
					break
				}
			}
			if !free {
				continue
			}
			w := s[i : i+width]
			if ok(w) || ok(reverseASCII(w)) {
				out = append(out, byteSpan{refs[i].start, refs[i+width-1].end})
				for j := i; j < i+width; j++ {
					used[j] = true
				}
			}
		}
	}

	consider(14, validateCNPJ)
	for w := 19; w >= 13; w-- {
		consider(w, validateCard)
	}
	consider(15, validateCNS)
	consider(11, func(x string) bool { return validateCPF(x) || validatePIS(x) })
	return out
}

// redactEncoded decodes base64 and hex runs and, when the decoded bytes contain
// sensitive data (directly, as separated digits, or nested one more level),
// redacts the ENCODED token. depth bounds recursion.
func redactEncoded(text string, depth int, v *Vault) (string, int) {
	if depth >= maxDecodeDepth {
		return text, 0
	}
	var spans []byteSpan
	budget := maxCandidates

	scan := func(re *regexp.Regexp, pre func(string) bool, decode func(string) ([]byte, bool)) {
		for _, m := range re.FindAllStringIndex(text, -1) {
			if budget <= 0 {
				return
			}
			tok := text[m[0]:m[1]]
			if len(tok) > maxTokenLen || (pre != nil && !pre(tok)) {
				continue
			}
			budget--
			b, ok := decode(tok)
			if !ok || len(b) < 8 {
				continue
			}
			if decodedIsSensitive(string(b), depth) {
				spans = append(spans, byteSpan{m[0], m[1]})
			}
		}
	}

	scan(base64Candidate, looksBase64ish, decodeBase64Any)
	scan(hexCandidate, nil, decodeHexLoose)

	if len(spans) == 0 {
		return text, 0
	}
	return spliceRedactions(text, spans, func(orig string) string {
		return tokenOrMarker(v, orig, "[REDACTED_BY_ALCATRAZ_ENCODED]")
	}), len(spans)
}

// decodedIsSensitive reports whether decoded bytes carry sensitive data by any
// route the Guard understands, including one more layer of encoding. It is a
// detection-only probe (output discarded), so it never touches the vault.
func decodedIsSensitive(s string, depth int) bool {
	if HasSensitiveContent(s) {
		return true
	}
	if _, n := redactSeparatedDigits(s, nil); n > 0 {
		return true
	}
	if _, n := redactEncoded(s, depth+1, nil); n > 0 {
		return true
	}
	return false
}

func decodeBase64Any(tok string) ([]byte, bool) {
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		if b, err := enc.DecodeString(tok); err == nil {
			return b, true
		}
	}
	return nil, false
}

func decodeHexLoose(tok string) ([]byte, bool) {
	var sb strings.Builder
	sb.Grow(len(tok))
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			sb.WriteByte(c)
		}
	}
	h := sb.String()
	if len(h)%2 != 0 {
		h = h[:len(h)-1]
	}
	if len(h) < 16 {
		return nil, false
	}
	b, err := hex.DecodeString(h)
	if err != nil {
		return nil, false
	}
	return b, true
}

// looksBase64ish rejects single-case, all-letter runs — natural words and
// identifiers that decode to meaningless bytes — so prose doesn't flood the
// decoder. Real base64 of binary almost always mixes case, digits, or symbols.
func looksBase64ish(tok string) bool {
	hasLower, hasUpper, hasOther := false, false, false
	for i := 0; i < len(tok); i++ {
		c := tok[i]
		switch {
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		default:
			hasOther = true
		}
	}
	if !hasOther && (hasLower != hasUpper) {
		return false
	}
	return true
}

// foldDigit maps ASCII, full-width, and Arabic-Indic digit runes to their ASCII
// byte. The second return is false for non-digits.
func foldDigit(r rune) (byte, bool) {
	switch {
	case r >= '0' && r <= '9':
		return byte(r), true
	case r >= 0xFF10 && r <= 0xFF19: // full-width ０-９
		return byte('0' + (r - 0xFF10)), true
	case r >= 0x0660 && r <= 0x0669: // Arabic-Indic
		return byte('0' + (r - 0x0660)), true
	case r >= 0x06F0 && r <= 0x06F9: // Extended Arabic-Indic
		return byte('0' + (r - 0x06F0)), true
	}
	return 0, false
}

var zeroWidth = map[rune]bool{
	0x200B: true, 0x200C: true, 0x200D: true, 0x2060: true, 0xFEFF: true,
}

// isConnector reports whether r may sit between two digits of the same number
// without breaking the run (a single separator per digit is the usual evasion).
func isConnector(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '.', '-', '/', ',', '_':
		return true
	}
	return zeroWidth[r]
}

func reverseASCII(s string) string {
	b := []byte(s)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}

// spliceRedactions replaces each (possibly overlapping) span with repl(original),
// merging overlaps so no region is rewritten twice. repl receives the exact
// source substring being redacted, so a vault can tokenize it reversibly.
func spliceRedactions(text string, spans []byteSpan, repl func(orig string) string) string {
	if len(spans) == 0 {
		return text
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	var b strings.Builder
	last := 0
	for _, s := range spans {
		if s.start < last {
			if s.end > last {
				last = s.end
			}
			continue
		}
		b.WriteString(text[last:s.start])
		b.WriteString(repl(text[s.start:s.end]))
		last = s.end
	}
	b.WriteString(text[last:])
	return b.String()
}
