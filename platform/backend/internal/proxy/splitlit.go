package proxy

import (
	"regexp"
	"strings"
)

// Split-literal evasion.
//
// The literal patterns only ever see contiguous values, so a value written as
// pieces that the reader joins back together walks straight past them:
//
//	const user = 'someone';
//	const domain = 'example.com';
//	const email = [user, domain].join('@');
//
// Nothing there matches an email, yet the value is perfectly recoverable by
// whoever reads the code — which is the whole point of redacting it. This pass
// folds the common joining forms (`+` chains, `[...].join(sep)`,
// `sep.join([...])`, `.concat(...)`, and `${}` template literals) over string
// literals bound in the same body, and when the folded result is sensitive it
// redacts the PIECES rather than the expression: leaving `'someone'` and
// `'example.com'` in place would give the value away regardless.
//
// Scope is one body and simple `name = "literal"` bindings. A caller set on
// evading can still compute a value this doesn't model — fromCharCode,
// arithmetic, or splitting across two requests (see the note in evasion.go).
// This closes the cheap, obvious route, not the arms race.

const (
	// maxBindings caps how many `name = "literal"` pairs are tracked per body.
	maxBindings = 512
	// maxFolds caps how many expressions are folded per body.
	maxFolds = 256
	// maxFoldLen skips folds too long to be a secret. Checking one costs a
	// full pattern sweep, so long concatenations (SQL, HTML, prose) are not
	// worth the scan — and any secret inside them is caught piece by piece.
	maxFoldLen = 4096
)

const (
	patStrLit  = `'[^'\n\\]*'|"[^"\n\\]*"`
	patIdent   = `[A-Za-z_$][A-Za-z0-9_$]*`
	patOperand = `(?:` + patStrLit + `|` + patIdent + `)`
)

var (
	// `const user = 'someone'`, `user: "someone"`, `user := "someone"`.
	bindingRe = regexp.MustCompile(`(` + patIdent + `)\s*:?=\s*(` + patStrLit + `)`)

	// `a + 'b' + c` — at least two operands.
	plusChainRe = regexp.MustCompile(patOperand + `(?:\s*\+\s*` + patOperand + `)+`)

	// `[a, b].join('@')` — the separator is optional (JS defaults to ",").
	arrayJoinRe = regexp.MustCompile(
		`\[\s*` + patOperand + `(?:\s*,\s*` + patOperand + `)*\s*,?\s*\]\s*\.\s*join\(\s*(` + patStrLit + `)?\s*\)`)

	// `'@'.join([user, domain])` — Python, and the tuple form.
	pyJoinRe = regexp.MustCompile(
		`(` + patStrLit + `)\s*\.\s*join\(\s*[\[(]\s*` + patOperand + `(?:\s*,\s*` + patOperand + `)*\s*,?\s*[\])]\s*\)`)

	// `'a'.concat(b, 'c')`.
	concatRe = regexp.MustCompile(
		patOperand + `\s*\.\s*concat\(\s*` + patOperand + `(?:\s*,\s*` + patOperand + `)*\s*\)`)

	// A template literal and the `${name}` holes inside it.
	templateRe    = regexp.MustCompile("`[^`\n]*`")
	templateVarRe = regexp.MustCompile(`\$\{\s*(` + patIdent + `)\s*\}`)

	operandRe = regexp.MustCompile(patOperand)
)

// binding is one name bound to a string literal, plus the spans of every
// literal it is bound to (a name rebound twice contributes both).
type binding struct {
	val  string
	lits []byteSpan
}

// isStrLit reports whether tok is a quoted literal rather than an identifier.
func isStrLit(tok string) bool {
	return len(tok) >= 2 && (tok[0] == '\'' || tok[0] == '"')
}

// collectBindings maps identifiers to the literal they are assigned. Only the
// first binding of a name defines its value; later ones still register their
// literal span so every piece of a folded value gets redacted.
func collectBindings(text string) map[string]*binding {
	ms := bindingRe.FindAllStringSubmatchIndex(text, maxBindings)
	if len(ms) == 0 {
		return nil
	}
	binds := make(map[string]*binding, len(ms))
	for _, m := range ms {
		name := text[m[2]:m[3]]
		lit := byteSpan{m[4] + 1, m[5] - 1} // inner text, quotes excluded
		if b, ok := binds[name]; ok {
			b.lits = append(b.lits, lit)
			continue
		}
		binds[name] = &binding{val: text[lit.start:lit.end], lits: []byteSpan{lit}}
	}
	return binds
}

// resolve turns one operand into its value and the literal span backing it.
// ok is false for an identifier with no known binding — folding with a hole
// would invent a value that isn't in the text.
func resolve(text string, sp byteSpan, binds map[string]*binding) (val string, lits []byteSpan, ok bool) {
	tok := text[sp.start:sp.end]
	if isStrLit(tok) {
		inner := byteSpan{sp.start + 1, sp.end - 1}
		return text[inner.start:inner.end], []byteSpan{inner}, true
	}
	b := binds[tok]
	if b == nil {
		return "", nil, false
	}
	return b.val, b.lits, true
}

// foldJoin resolves every operand inside zone, joins the values with sep, and
// reports the literal spans that contributed. ok is false when any operand is
// unresolvable.
func foldJoin(text string, zone byteSpan, sep string, binds map[string]*binding) (string, []byteSpan, bool) {
	ops := operandRe.FindAllStringIndex(text[zone.start:zone.end], -1)
	if len(ops) == 0 {
		return "", nil, false
	}
	parts := make([]string, 0, len(ops))
	var lits []byteSpan
	for _, o := range ops {
		val, ls, ok := resolve(text, byteSpan{zone.start + o[0], zone.start + o[1]}, binds)
		if !ok {
			return "", nil, false
		}
		parts = append(parts, val)
		lits = append(lits, ls...)
	}
	return strings.Join(parts, sep), lits, true
}

// litText returns the inner text of a quoted literal matched at m (a submatch
// index pair), or "" when the group didn't participate.
func litText(text string, start, end int) string {
	if start < 0 || end <= start+1 {
		return ""
	}
	return text[start+1 : end-1]
}

// redactSplitLiterals folds every joining expression it recognises and, when
// the result is sensitive, redacts each literal that fed it. Returns the
// rewritten text and the number of expressions that tripped.
func redactSplitLiterals(text string, v *Vault) (string, int) {
	// Nothing to fold without a joining operator — skips the regex sweep for
	// prose bodies, which are most of the traffic.
	if !strings.Contains(text, "+") && !strings.Contains(text, ".join(") &&
		!strings.Contains(text, ".concat(") && !strings.Contains(text, "${") {
		return text, 0
	}

	binds := collectBindings(text)
	var spans []byteSpan
	folds := 0

	// take records the contributing literals when a folded value is sensitive.
	take := func(folded string, lits []byteSpan, ok bool) {
		if !ok || folds >= maxFolds || len(lits) == 0 || len(folded) > maxFoldLen {
			return
		}
		folds++
		if !HasSensitiveContent(folded) {
			return
		}
		for _, l := range lits {
			if l.end > l.start {
				spans = append(spans, l)
			}
		}
	}

	// `a + 'b' + c` — the operands are the whole match, joined by nothing.
	for _, m := range plusChainRe.FindAllStringIndex(text, maxFolds) {
		take(foldJoin(text, byteSpan{m[0], m[1]}, "", binds))
	}

	// `[a, b].join(sep)` — operands live inside the brackets; sep defaults to ",".
	for _, m := range arrayJoinRe.FindAllStringSubmatchIndex(text, maxFolds) {
		end := strings.Index(text[m[0]:m[1]], "]")
		if end < 0 {
			continue
		}
		sep := ","
		if m[2] >= 0 {
			sep = litText(text, m[2], m[3])
		}
		take(foldJoin(text, byteSpan{m[0] + 1, m[0] + end}, sep, binds))
	}

	// `sep.join([a, b])` — sep is the leading literal, operands are the args.
	for _, m := range pyJoinRe.FindAllStringSubmatchIndex(text, maxFolds) {
		open := strings.IndexAny(text[m[0]:m[1]], "[(")
		if open < 0 {
			continue
		}
		take(foldJoin(text, byteSpan{m[0] + open + 1, m[1] - 2}, litText(text, m[2], m[3]), binds))
	}

	// `'a'.concat(b, 'c')` — receiver plus every argument, concatenated. The
	// `.concat` token is excluded from both zones so it isn't read as an operand.
	for _, m := range concatRe.FindAllStringIndex(text, maxFolds) {
		zone := text[m[0]:m[1]]
		dot, open := strings.Index(zone, ".concat"), strings.Index(zone, "(")
		if dot < 0 || open < 0 {
			continue
		}
		head, headLits, headOK := foldJoin(text, byteSpan{m[0], m[0] + dot}, "", binds)
		tail, tailLits, tailOK := foldJoin(text, byteSpan{m[0] + open + 1, m[1] - 1}, "", binds)
		lits := make([]byteSpan, 0, len(headLits)+len(tailLits))
		lits = append(append(lits, headLits...), tailLits...)
		take(head+tail, lits, headOK && tailOK)
	}

	// `` `${user}@${domain}` `` — holes resolved, literal parts kept.
	for _, m := range templateRe.FindAllStringIndex(text, maxFolds) {
		body := text[m[0]+1 : m[1]-1]
		if !strings.Contains(body, "${") {
			continue
		}
		var sb strings.Builder
		var lits []byteSpan
		last, ok := 0, true
		for _, h := range templateVarRe.FindAllStringSubmatchIndex(body, -1) {
			sb.WriteString(body[last:h[0]])
			b := binds[body[h[2]:h[3]]]
			if b == nil {
				ok = false
				break
			}
			sb.WriteString(b.val)
			lits = append(lits, b.lits...)
			last = h[1]
		}
		if !ok {
			continue
		}
		sb.WriteString(body[last:])
		take(sb.String(), lits, true)
	}

	if len(spans) == 0 {
		return text, 0
	}
	return spliceRedactions(text, spans, func(orig string) string {
		return tokenOrMarker(v, orig, "[REDACTED_BY_ALCATRAZ_SPLIT]")
	}), len(spans)
}
