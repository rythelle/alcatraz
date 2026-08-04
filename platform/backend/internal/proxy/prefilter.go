package proxy

import (
	"regexp/syntax"
	"strings"
)

// Literal prefiltering.
//
// The Guard runs ~120 patterns over every string value in a request body. None
// of them is individually slow, but each one scans the whole text, so a large
// file read costs ~120 full sweeps — most of them for secrets whose format
// isn't remotely present (Azure keys, IBANs, seed phrases…).
//
// Nearly every pattern requires some literal text to appear in a match:
// `AKIA`, `sk-ant-`, `password`, `xoxb-`. This derives that requirement from
// the pattern itself, at startup, and lets the scan skip any pattern whose
// literals are absent from the body — one cheap substring search instead of a
// regex sweep.
//
// The derivation is conservative in one direction only: it may fail to find a
// requirement (pattern always runs, no gain), but a set it does return is one
// that every possible match must satisfy. It can never skip a pattern that
// would have matched. `TestPrefilterNeverSkipsAMatch` holds that line.

// minPrefilterLen is the shortest literal worth filtering on. Below this the
// substring is common enough in ordinary text that it never skips anything,
// and keeping it would only add work.
const minPrefilterLen = 3

// patternPrefilter maps a pattern name to lowercase literals of which a match
// must contain at least one. Absent or empty means "always run this pattern".
var patternPrefilter map[string][]string

func init() {
	patternPrefilter = make(map[string][]string, len(SensitivePatterns)+len(StrictPatterns))
	for _, group := range [][]SensitivePattern{SensitivePatterns, StrictPatterns} {
		for _, sp := range group {
			if lits := requiredLiterals(sp.Regex.String()); len(lits) > 0 {
				patternPrefilter[sp.Name] = lits
			}
		}
	}
}

// canSkip reports whether the named pattern provably cannot match lowerText,
// which must be the lowercased body. A pattern with no derived literals is
// never skipped.
func canSkip(name, lowerText string) bool {
	lits := patternPrefilter[name]
	if len(lits) == 0 {
		return false
	}
	for _, l := range lits {
		if strings.Contains(lowerText, l) {
			return false
		}
	}
	return true
}

// requiredLiterals parses expr and returns lowercase literals such that every
// match contains at least one of them, or nil when no such set can be derived.
func requiredLiterals(expr string) []string {
	re, err := syntax.Parse(expr, syntax.Perl)
	if err != nil {
		return nil
	}
	lits := requiredSet(re.Simplify())
	if len(lits) == 0 {
		return nil
	}
	// The set is a disjunction: it only filters as well as its weakest member,
	// so one short or non-ASCII literal disqualifies the whole set. Non-ASCII
	// is excluded because the case folding below is ASCII-only.
	seen := make(map[string]bool, len(lits))
	out := make([]string, 0, len(lits))
	for _, l := range lits {
		if len(l) < minPrefilterLen || !isASCII(l) {
			return nil
		}
		l = strings.ToLower(l)
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	return out
}

// requiredSet walks the parsed regex and returns literals of which any match of
// re must contain at least one. nil means "nothing is required".
func requiredSet(re *syntax.Regexp) []string {
	switch re.Op {
	case syntax.OpLiteral:
		if len(re.Rune) == 0 {
			return nil
		}
		return []string{string(re.Rune)}

	case syntax.OpCapture:
		return requiredSet(re.Sub[0])

	case syntax.OpPlus:
		// One or more: whatever the body requires is required overall.
		return requiredSet(re.Sub[0])

	case syntax.OpRepeat:
		if re.Min >= 1 {
			return requiredSet(re.Sub[0])
		}
		return nil

	case syntax.OpConcat:
		// Every element must match, so any single element's requirement is a
		// valid requirement for the whole. Two candidates per position: the
		// run of adjacent fixed pieces starting there (which reassembles what
		// prefix factoring split apart — the parser rewrites `(?i)(cpf|cliente)`
		// as `c` followed by `(pf|liente)`, and only the join is worth
		// filtering on), and the element's own requirement.
		var best []string
		for i := range re.Sub {
			if c := fixedRunFrom(re.Sub[i:]); stronger(c, best) {
				best = c
			}
			if c := requiredSet(re.Sub[i]); stronger(c, best) {
				best = c
			}
		}
		return best

	case syntax.OpAlternate:
		// A match takes one branch, so every branch must contribute — if any
		// branch requires nothing, the alternation requires nothing.
		var out []string
		for _, sub := range re.Sub {
			c := requiredSet(sub)
			if len(c) == 0 {
				return nil
			}
			out = append(out, c...)
		}
		return out
	}

	// Star, quest, char classes, anchors, dot: nothing is required.
	return nil
}

// maxLiteralSet bounds the cross products below, so a pattern built from many
// small char classes can't blow up into thousands of combinations at startup.
const maxLiteralSet = 64

// fixedRunFrom walks subs from the front while each piece contributes a known
// leading string, and returns the concatenations. Every match of the run must
// begin with one of them, so they are required substrings of the whole match.
func fixedRunFrom(subs []*syntax.Regexp) []string {
	acc, exact := prefixSet(subs[0])
	if len(acc) == 0 {
		return nil
	}
	for k := 1; k < len(subs) && exact; k++ {
		next, nextExact := prefixSet(subs[k])
		if len(next) == 0 {
			break
		}
		crossed := cross(acc, next)
		if crossed == nil {
			break
		}
		acc, exact = crossed, nextExact
	}
	return allNonEmpty(acc)
}

// prefixSet returns strings such that every match of re starts with one of
// them. exact additionally reports that re matches those strings and nothing
// else, which is what lets fixedRunFrom keep extending through the next piece.
// A nil set means nothing can be said.
func prefixSet(re *syntax.Regexp) (set []string, exact bool) {
	switch re.Op {
	// Zero-width: matches the empty string exactly, so a run continues past it.
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine,
		syntax.OpBeginText, syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return []string{""}, true

	case syntax.OpLiteral:
		return []string{string(re.Rune)}, true

	case syntax.OpCapture:
		return prefixSet(re.Sub[0])

	case syntax.OpCharClass:
		// Only enumerate genuinely small classes — a case pair like [Cc] or a
		// short set. Anything wider is not worth the combinations.
		var out []string
		for i := 0; i+1 < len(re.Rune); i += 2 {
			lo, hi := re.Rune[i], re.Rune[i+1]
			if hi-lo > 3 {
				return nil, false
			}
			for r := lo; r <= hi; r++ {
				out = append(out, string(r))
				if len(out) > maxLiteralSet {
					return nil, false
				}
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true

	case syntax.OpConcat:
		return fixedRunFrom(re.Sub), false

	case syntax.OpAlternate:
		var out []string
		allExact := true
		for _, sub := range re.Sub {
			s, ex := prefixSet(sub)
			if len(s) == 0 {
				return nil, false
			}
			allExact = allExact && ex
			out = append(out, s...)
			if len(out) > maxLiteralSet {
				return nil, false
			}
		}
		return out, allExact

	case syntax.OpPlus:
		s, _ := prefixSet(re.Sub[0])
		return s, false

	case syntax.OpRepeat:
		if re.Min >= 1 {
			s, _ := prefixSet(re.Sub[0])
			return s, false
		}
		return nil, false
	}

	// Star, quest, dot, any: can match empty or anything.
	return nil, false
}

func cross(a, b []string) []string {
	if len(a)*len(b) > maxLiteralSet {
		return nil
	}
	out := make([]string, 0, len(a)*len(b))
	for _, x := range a {
		for _, y := range b {
			if len(x)+len(y) > 128 {
				return nil
			}
			out = append(out, x+y)
		}
	}
	return out
}

// allNonEmpty returns lits unchanged, or nil if any member is the empty
// string. An empty member means that alternative matches nothing at all —
// `(?:aws_secret_access_key|aws_secret)` parses to `aws_secret` followed by
// `(_access_key|<empty>)` — so no literal in the set is actually required.
// Dropping the empty and keeping the rest would claim `_access_key` is
// mandatory and skip the pattern on a plain `aws_secret: …`, which is a leak.
func allNonEmpty(lits []string) []string {
	for _, l := range lits {
		if l == "" {
			return nil
		}
	}
	if len(lits) == 0 {
		return nil
	}
	return lits
}

// stronger reports whether candidate filters better than best: a longer
// shortest-literal wins, ties go to the smaller set.
func stronger(candidate, best []string) bool {
	if len(candidate) == 0 {
		return false
	}
	if len(best) == 0 {
		return true
	}
	cm, bm := shortestLen(candidate), shortestLen(best)
	if cm != bm {
		return cm > bm
	}
	return len(candidate) < len(best)
}

func shortestLen(lits []string) int {
	min := -1
	for _, l := range lits {
		if min < 0 || len(l) < min {
			min = len(l)
		}
	}
	return min
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
