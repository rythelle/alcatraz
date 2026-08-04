package proxy

import (
	"os"
	"strings"
	"testing"
)

// corpora returns bodies to check the prefilter against: the package's own
// sources and test fixtures (which between them carry samples of nearly every
// pattern), plus the repo README, plus a synthetic kitchen sink.
func corpora(t *testing.T) map[string]string {
	out := map[string]string{
		"kitchen_sink": kitchenSink,
		"empty":        "",
		"prose":        "The quick brown fox jumps over the lazy dog. " + strings.Repeat("lorem ipsum dolor sit amet. ", 200),
	}
	for _, p := range []string{
		"sanitizer_test.go",
		"sanitizer_realworld_test.go",
		"intl_ids_test.go",
		"evasion_test.go",
		"patterns.go",
		"/home/ryth/projects/alcatraz/README.md",
	} {
		if b, err := os.ReadFile(p); err == nil {
			out[p] = string(b)
		}
	}
	return out
}

// kitchenSink carries at least one plausible sample per pattern family, so the
// prefilter is exercised against text that really does match.
const kitchenSink = `
AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE
AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
ANTHROPIC_API_KEY=sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijklmnopqrstuvwx
OPENAI_API_KEY=sk-proj-abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGH
GITHUB_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz0123456789AB
SLACK_TOKEN=xoxb-123456789012-1234567890123-abcdefghijklmnopqrstuvwx
STRIPE_KEY=sk_live_abcdefghijklmnopqrstuvwx
DATABASE_URL=postgres://user:hunter2secret@db.example.com:5432/app
email: someone@example.com
CPF: 111.444.777-35
CNPJ: 11.222.333/0001-81
IBAN: GB82WEST12345698765432
card: 4111 1111 1111 1111
ssn 123-45-6789
phone: +55 11 91234-5678
ip: 192.168.1.100
jwt: eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NSJ9.dBjftJeZ4CVPmB92K27uhbUJU1p1r
-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAx0000000000000000000000000000000000000000000000000
-----END RSA PRIVATE KEY-----
seed: abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about
Authorization: Bearer abcdefghijklmnopqrstuvwxyz0123456789
password = "correct horse battery staple"
AZURE_STORAGE_KEY=DefaultEndpointsProtocol=https;AccountName=x;AccountKey=abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789abcdefghijkl==;
`

// The load-bearing invariant: if the prefilter says a pattern cannot match a
// body, the regex must agree. A failure here means the Guard silently stops
// redacting something.
func TestPrefilterNeverSkipsAMatch(t *testing.T) {
	all := append(append([]SensitivePattern{}, SensitivePatterns...), StrictPatterns...)

	for name, body := range corpora(t) {
		lower := strings.ToLower(body)
		for _, sp := range all {
			if !canSkip(sp.Name, lower) {
				continue
			}
			if loc := sp.Regex.FindStringIndex(body); loc != nil {
				t.Errorf("pattern %q skipped on %s but matches at %d: %q\n  prefilter literals: %q",
					sp.Name, name, loc[0], body[loc[0]:loc[1]], patternPrefilter[sp.Name])
			}
		}
	}
}

// Same invariant at the pipeline level: prefiltering must not change output.
func TestPrefilterDoesNotChangeOutput(t *testing.T) {
	for name, body := range corpora(t) {
		got, _, _ := sanitizePipeline(body, nil, nil, false)
		want := sanitizeUnfiltered(body)
		if got != want {
			t.Errorf("%s: prefiltered output differs from unfiltered", name)
			for i := 0; i < len(got) && i < len(want); i++ {
				if got[i] != want[i] {
					lo := i - 60
					if lo < 0 {
						lo = 0
					}
					t.Errorf("  first difference at %d:\n   got: %q\n  want: %q", i, got[lo:min(i+60, len(got))], want[lo:min(i+60, len(want))])
					break
				}
			}
		}
	}
}

// sanitizeUnfiltered mirrors steps ④ and ⑤ of the pipeline with the prefilter
// switched off. It must stay in step with sanitizePipeline apart from that one
// difference, or the comparison stops meaning anything.
func sanitizeUnfiltered(text string) string {
	refs := refSpans(text)
	for _, sp := range SensitivePatterns {
		out, count := applyPatternOutsideRefs(sp, text, nil, refs)
		if count > 0 {
			text = out
			refs = refSpans(text)
		}
	}
	if out, dets := applyAntiEvasion(text, nil); len(dets) > 0 {
		text = out
	}
	return text
}

// A prefilter that covers nothing would be correct but pointless; this records
// how much of the table it actually covers so a regression is visible.
func TestPrefilterCoverage(t *testing.T) {
	covered := 0
	for _, sp := range SensitivePatterns {
		if len(patternPrefilter[sp.Name]) > 0 {
			covered++
		}
	}
	pct := 100 * covered / len(SensitivePatterns)
	t.Logf("prefilter covers %d/%d patterns (%d%%)", covered, len(SensitivePatterns), pct)
	if pct < 50 {
		t.Errorf("prefilter coverage collapsed to %d%%", pct)
	}
}

func TestRequiredLiterals(t *testing.T) {
	cases := []struct {
		expr string
		want []string
	}{
		{`AKIA[0-9A-Z]{16}`, []string{"akia"}},
		{`(?i)(?:password|secret)\s*[:=]\s*\S+`, []string{"password", "secret"}},
		{`\b\d{3}-\d{2}-\d{4}\b`, nil}, // no literal at all
		// The parser factors the shared `sk-` prefix out into `sk-` +
		// `(ant-|proj-)`; the extraction has to put it back together, or the
		// filter degrades to the weaker `ant-`/`proj-`.
		{`(?:sk-ant-|sk-proj-)\w+`, []string{"sk-ant-", "sk-proj-"}},
		// Case-insensitive alternations are the common shape in the table, and
		// they get the same factoring treatment.
		{`(?i)(?:cpf|cliente)\s*[:=]\s*\d+`, []string{"cpf", "cliente"}},
		{`(?:token|x)=\d+`, nil}, // one branch too short -> unusable
		{`-----BEGIN [A-Z ]+PRIVATE KEY-----`, []string{"private key-----"}},
	}
	for _, tc := range cases {
		got := requiredLiterals(tc.expr)
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("requiredLiterals(%q) = %q, want %q", tc.expr, got, tc.want)
		}
	}
}
