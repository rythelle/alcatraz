package proxy

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
)

// Differential check against the obfuscator as it stood before this work.
//
// The coverage test proves every pattern still fires on its own. This proves
// the stronger property: across arbitrary text, anything the OLD pipeline
// removed is still gone from the NEW pipeline's output. That is the whole
// question — "did the changes break the obfuscator" — stated as an assertion.
//
// Exactly one intentional exception is allowed: a value sitting inside a
// `{{resolve:...}}` dynamic reference, which is now deliberately preserved
// because it names a secret rather than being one.

// legacyRedactions returns every value the pre-change pipeline would have
// removed from text: all validated pattern matches, plus the two anti-evasion
// passes that existed then (separated digits and encoded blobs). It reproduces
// the old order — patterns first, each seeing the previous one's output.
func legacyRedactions(text string) []string {
	var removed []string
	cur := text

	for _, sp := range SensitivePatterns {
		validate := patternValidators[sp.Name]
		for _, m := range sp.Regex.FindAllString(cur, -1) {
			if validate == nil || validate(m) {
				removed = append(removed, m)
			}
		}
		// No prefilter, no dynamic-reference protection: the old behaviour.
		out, n := applyPattern(sp, cur, nil)
		if n > 0 {
			cur = out
		}
	}

	// The anti-evasion passes that predate the split-literal one.
	if out, n := redactSeparatedDigits(cur, nil); n > 0 {
		removed = append(removed, diffSpans(cur, out)...)
		cur = out
	}
	if out, n := redactEncoded(cur, 0, nil); n > 0 {
		removed = append(removed, diffSpans(cur, out)...)
	}
	return removed
}

// diffSpans recovers the source substrings a redaction pass replaced, by
// walking the common prefix and suffix around each change.
func diffSpans(before, after string) []string {
	if before == after {
		return nil
	}
	i := 0
	for i < len(before) && i < len(after) && before[i] == after[i] {
		i++
	}
	j := 0
	for j < len(before)-i && j < len(after)-i &&
		before[len(before)-1-j] == after[len(after)-1-j] {
		j++
	}
	return []string{before[i : len(before)-j]}
}

// explainedByDynamicRef reports whether a value the old pipeline removed owes
// its "sensitive" status entirely to a dynamic reference. Old matches land on
// either side of a reference boundary, so both shapes have to be recognised:
//
//	fragment — `token:SecretString}}`, a piece of the reference that
//	           generic_secret matched inside it;
//	wrapper  — `TOKEN: '{{resolve:ssm:/a/b:1}}'`, where env_secret swallowed
//	           the assignment around it.
//
// Anything else surviving is a genuine regression.
//
// Only a value that survives WHOLE reaches here: when a real secret sits next
// to a reference the new pipeline still redacts that part, so the old match no
// longer appears verbatim in the output.
func explainedByDynamicRef(text, value string) bool {
	refs := refSpans(text)
	if len(refs) == 0 {
		return false
	}

	// Ask the same question the pipeline asks, at each place the value occurs:
	// cut away whatever the value shares with a reference, and see whether what
	// remains is still sensitive. Every occurrence has to be explained.
	found := false
	for off := 0; off < len(text); {
		i := strings.Index(text[off:], value)
		if i < 0 {
			break
		}
		found = true
		at := off + i
		stripped, overlapped := cutRefSpans(text, []int{at, at + len(value)}, refs)
		if !overlapped || HasSensitiveContent(stripped) {
			return false
		}
		off = at + 1
	}
	return found
}

// assertNoRegression is the core assertion: nothing the old pipeline removed
// may survive the new one.
func assertNoRegression(t *testing.T, label, text string) {
	t.Helper()
	got, _, _ := sanitizePipeline(text, nil, nil, false)

	for _, secret := range legacyRedactions(text) {
		if len(secret) < 4 {
			continue // too short to identify anything
		}
		if !strings.Contains(got, secret) {
			continue
		}
		if explainedByDynamicRef(text, secret) {
			continue // the one intended difference
		}
		t.Errorf("%s: value redacted by the old pipeline survives the new one\n"+
			"  value: %q\n  input: %q\n  output: %q", label, secret, text, got)
	}
}

// Fixed corpora: the package's own fixtures and sources, which between them
// carry samples of most pattern families.
func TestNoRedactionRegressionOnCorpora(t *testing.T) {
	for name, body := range corpora(t) {
		assertNoRegression(t, name, body)
	}
}

// Every generated per-pattern sample, on its own and wrapped in the contexts
// the new passes care about — next to a dynamic reference, and inside a
// concatenation the split-literal pass rewrites.
func TestNoRedactionRegressionPerPattern(t *testing.T) {
	for name, sample := range generatedSamples(t) {
		t.Run(name, func(t *testing.T) {
			for i, tmpl := range []string{
				"%s",
				"value = '%s';",
				"{{resolve:ssm:/app/x:1}} %s",
				"%s {{resolve:secretsmanager:name:SecretString}}",
				"const a = 'x'; const b = 'y'; const c = a + b; // %s",
				"[ 'p', 'q' ].join('@') // %s",
				"prefix %s suffix",
			} {
				assertNoRegression(t, fmt.Sprintf("%s/tmpl%d", name, i), fmt.Sprintf(tmpl, sample))
			}
		})
	}
}

// Randomised mixtures: several secrets, references and concatenations in one
// body, in shuffled order. Catches interactions between the new passes that
// no single hand-written case would.
func TestNoRedactionRegressionOnMixtures(t *testing.T) {
	samples := generatedSamples(t)
	names := make([]string, 0, len(samples))
	for n := range samples {
		names = append(names, n)
	}
	// Stable order before shuffling, so failures reproduce.
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}

	noise := []string{
		"{{resolve:secretsmanager:app-${self:custom.stage}-token:SecretString}}",
		"const user = 'someone'; const domain = 'example.com';",
		"const email = [user, domain].join('@');",
		"// just a comment",
		"path = 'src' + '/' + 'index.ts';",
	}

	rnd := rand.New(rand.NewSource(20260728))
	for round := 0; round < 300; round++ {
		var parts []string
		for i := 0; i < 3; i++ {
			parts = append(parts, samples[names[rnd.Intn(len(names))]])
		}
		parts = append(parts, noise[rnd.Intn(len(noise))], noise[rnd.Intn(len(noise))])
		rnd.Shuffle(len(parts), func(i, j int) { parts[i], parts[j] = parts[j], parts[i] })

		sep := []string{"\n", " ", ", ", "\n  "}[rnd.Intn(4)]
		assertNoRegression(t, fmt.Sprintf("mixture/round%d", round), strings.Join(parts, sep))
	}
}

// Fuzzing, for the shapes nobody thought of. Seeded with the awkward ones.
// Run with: go test -fuzz FuzzNoRedactionRegression ./internal/proxy/
func FuzzNoRedactionRegression(f *testing.F) {
	f.Add("AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	f.Add("aws_secret: MnZ5gKfHDb83KcDjDg2IWZrdGRCLW8RCAAyy369N")
	f.Add("{{resolve:secretsmanager:x-${self:custom.stage}-y:SecretString}}")
	f.Add("const u = 'someone'; const d = 'example.com'; const e = [u, d].join('@');")
	f.Add("TOKEN: '{{resolve:ssm:/a/b:1}}'\nAWS_KEY: 'AKIAIOSFODNN7EXAMPLE'")
	f.Add("email: someone@example.com and cpf 111.444.777-35")
	f.Add("{{resolve:}}AKIAIOSFODNN7EXAMPLE")

	f.Fuzz(func(t *testing.T, in string) {
		if len(in) > 8192 {
			t.Skip()
		}
		got, _, _ := sanitizePipeline(in, nil, nil, false)
		for _, secret := range legacyRedactions(in) {
			if len(secret) < 8 || !strings.Contains(got, secret) {
				continue
			}
			if explainedByDynamicRef(in, secret) {
				continue
			}
			t.Fatalf("value redacted by the old pipeline survives\n value: %q\n input: %q\n output: %q",
				secret, in, got)
		}
	})
}
