package proxy

import (
	"fmt"
	"math/rand"
	"regexp/syntax"
	"strings"
	"testing"
)

// Does the obfuscator still obfuscate?
//
// Three changes landed in the redaction path: dynamic references are protected
// before the patterns run, a split-literal pass was added after them, and the
// pattern sweep now skips patterns whose literals are absent. Each one could
// in principle stop a secret from being redacted, and hand-picked examples
// would only prove the cases someone thought to write down.
//
// So instead of trusting a sample list, this generates a matching value for
// EVERY pattern in the table straight from that pattern's own regex, and
// checks the whole pipeline still removes it. A pattern that goes silently
// dead — because its prefilter is wrong, or because an earlier step consumed
// its input — fails here.

// sampleGen builds strings that match a parsed regex. It is deliberately
// simple: the pattern table is made of format regexes (literals, classes,
// counted repeats), which is exactly what it handles. Anything it cannot
// build is reported rather than silently skipped.
type sampleGen struct{ rnd *rand.Rand }

// build returns a string matching re, or ok=false for a construct it does not
// model. Repetition is kept minimal so the result stays a tight match.
func (g *sampleGen) build(re *syntax.Regexp, depth int) (string, bool) {
	if depth > 40 {
		return "", false
	}
	switch re.Op {
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine,
		syntax.OpBeginText, syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return "", true

	case syntax.OpLiteral:
		return string(re.Rune), true

	case syntax.OpCharClass:
		r, ok := g.pickRune(re.Rune)
		if !ok {
			return "", false
		}
		return string(r), true

	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		return "a", true

	case syntax.OpCapture:
		return g.build(re.Sub[0], depth+1)

	case syntax.OpStar, syntax.OpQuest:
		// Vary the count: some patterns (jwt_token's `eyJ[\w-]*\.`) only match
		// when the starred part is non-empty, others only when it is absent.
		n := g.rnd.Intn(4)
		if re.Op == syntax.OpQuest && n > 1 {
			n = 1
		}
		var sb strings.Builder
		for i := 0; i < n; i++ {
			s, ok := g.build(re.Sub[0], depth+1)
			if !ok {
				return "", false
			}
			sb.WriteString(s)
		}
		return sb.String(), true

	case syntax.OpPlus:
		var sb strings.Builder
		for i := 0; i <= g.rnd.Intn(3); i++ {
			s, ok := g.build(re.Sub[0], depth+1)
			if !ok {
				return "", false
			}
			sb.WriteString(s)
		}
		return sb.String(), true

	case syntax.OpRepeat:
		var sb strings.Builder
		for i := 0; i < re.Min; i++ {
			s, ok := g.build(re.Sub[0], depth+1)
			if !ok {
				return "", false
			}
			sb.WriteString(s)
		}
		return sb.String(), true

	case syntax.OpConcat:
		var sb strings.Builder
		for _, sub := range re.Sub {
			s, ok := g.build(sub, depth+1)
			if !ok {
				return "", false
			}
			sb.WriteString(s)
		}
		return sb.String(), true

	case syntax.OpAlternate:
		// Random branch, so repeated attempts explore the whole alternation.
		order := g.rnd.Perm(len(re.Sub))
		for _, i := range order {
			if s, ok := g.build(re.Sub[i], depth+1); ok {
				return s, true
			}
		}
		return "", false
	}
	return "", false
}

// pickRune chooses from a char-class range table, preferring characters that
// keep the sample readable and word-like (so \b boundaries still hold).
func (g *sampleGen) pickRune(ranges []rune) (rune, bool) {
	var pool []rune
	for i := 0; i+1 < len(ranges); i += 2 {
		lo, hi := ranges[i], ranges[i+1]
		if lo > 0x7e {
			continue
		}
		if hi > 0x7e {
			hi = 0x7e
		}
		for r := lo; r <= hi && r-lo < 96; r++ {
			if r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
				pool = append(pool, r)
			}
		}
	}
	if len(pool) == 0 {
		// No alphanumeric option: fall back to the first printable rune.
		for i := 0; i+1 < len(ranges); i += 2 {
			if ranges[i] >= 0x20 && ranges[i] <= 0x7e {
				return ranges[i], true
			}
		}
		return 0, false
	}
	return pool[g.rnd.Intn(len(pool))], true
}

// sampleForPattern keeps generating until it produces a value the pattern
// genuinely matches AND its structural validator accepts. Checksum-backed
// patterns (CPF, CNPJ, Luhn, IBAN, Verhoeff…) only redact when the check
// digits are right, so a random draw is retried until one lands — which is
// also why no check digit is hand-written anywhere in this file.
func sampleForPattern(sp SensitivePattern, seed int64) (string, bool) {
	parsed, err := syntax.Parse(sp.Regex.String(), syntax.Perl)
	if err != nil {
		return "", false
	}
	parsed = parsed.Simplify()
	g := &sampleGen{rnd: rand.New(rand.NewSource(seed))}
	validate := patternValidators[sp.Name]

	for attempt := 0; attempt < 200000; attempt++ {
		s, ok := g.build(parsed, 0)
		if !ok || s == "" {
			continue
		}
		m := sp.Regex.FindString(s)
		if m == "" {
			continue
		}
		if validate != nil && !validate(m) {
			continue
		}
		return s, true
	}
	return "", false
}

// generatedSamples builds one sample per pattern once, for reuse across tests.
func generatedSamples(t *testing.T) map[string]string {
	t.Helper()
	out := make(map[string]string, len(SensitivePatterns))
	var missing []string
	for i, sp := range SensitivePatterns {
		if s, ok := sampleForPattern(sp, int64(i)+1); ok {
			out[sp.Name] = s
		} else {
			missing = append(missing, sp.Name)
		}
	}
	// Every pattern in the table is currently reachable by the generator. If a
	// new pattern uses a construct it can't build, extend build() rather than
	// lowering this bar — an ungenerated pattern is an untested pattern.
	if len(missing) > 0 {
		t.Fatalf("no sample generated for %d of %d pattern(s): %v",
			len(missing), len(SensitivePatterns), missing)
	}
	return out
}

// Every pattern must still be reachable end to end. This is the direct answer
// to "did the prefilter switch anything off": a pattern whose literals were
// derived wrongly never runs, so its sample survives and this fails.
func TestEveryPatternStillRedacts(t *testing.T) {
	samples := generatedSamples(t)

	for _, sp := range SensitivePatterns {
		sample, ok := samples[sp.Name]
		if !ok {
			continue
		}
		t.Run(sp.Name, func(t *testing.T) {
			// The detector must see it.
			if !HasSensitiveContent(sample) {
				t.Fatalf("HasSensitiveContent missed %s: %q (prefilter: %q)",
					sp.Name, sample, patternPrefilter[sp.Name])
			}
			// The prefilter must not exclude it.
			if canSkip(sp.Name, strings.ToLower(sample)) {
				t.Fatalf("prefilter skips %s on its own sample %q (literals: %q)",
					sp.Name, sample, patternPrefilter[sp.Name])
			}
			// And the pipeline must remove it.
			got, dets, _ := sanitizePipeline(sample, nil, nil, false)
			if strings.Contains(got, sample) {
				t.Fatalf("%s leaked through the pipeline\n  sample: %q\n  output: %q\n  detections: %v",
					sp.Name, sample, got, dets)
			}
		})
	}
}

// The same values have to survive the trip through a realistic request body,
// with surrounding prose — the shape the Guard actually sees.
func TestEveryPatternRedactedInContext(t *testing.T) {
	samples := generatedSamples(t)

	for _, sp := range SensitivePatterns {
		sample, ok := samples[sp.Name]
		if !ok {
			continue
		}
		validate := patternValidators[sp.Name]

		t.Run(sp.Name, func(t *testing.T) {
			for _, tmpl := range []string{
				"Here is the config value: %s — please review it.",
				"line one\n%s\nline three",
				"{\"role\":\"user\",\"content\":%q}",
			} {
				body := fmt.Sprintf(tmpl, sample)

				// Some patterns are anchored to a line start (env_secret) or
				// to a word boundary, so wrapping can legitimately stop them
				// matching. Ask the raw regex — deliberately bypassing the
				// prefilter, which is the thing under test — whether this body
				// still contains something the pattern claims.
				matched := false
				for _, m := range sp.Regex.FindAllString(body, -1) {
					if validate == nil || validate(m) {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}

				res := SanitizeJSONWithVault(body, nil, NewVault(0), false)
				if strings.Contains(res.Output, sample) {
					t.Errorf("%s leaked in context %q\n  output: %s", sp.Name, tmpl, res.Output)
				}
			}
		})
	}
}

// Redaction must stay reversible for every pattern, or the workflow breaks and
// the agent goes looking for a way around the Guard again.
func TestEveryPatternRoundTrips(t *testing.T) {
	for name, sample := range generatedSamples(t) {
		t.Run(name, func(t *testing.T) {
			v := NewVault(0)
			got, _, _ := sanitizePipeline(sample, nil, v, false)
			if strings.Contains(got, sample) {
				t.Fatalf("%s was not redacted at all", name)
			}
			if back := v.Detokenize(got); back != sample {
				t.Errorf("%s did not round-trip\n want: %q\n got:  %q", name, sample, back)
			}
		})
	}
}
