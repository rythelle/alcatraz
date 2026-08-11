package proxy

import (
	"strings"
	"testing"
)

// validCNPJ / validCPF are real check-digit-valid documents used across the
// evasion tests. Their contiguous forms are already caught by the bare
// patterns; the point here is the transformed representations.
const (
	validCNPJ = "11222333000181"
	validCPF  = "11144477735"
)

// pipe runs the full Guard pipeline over a JSON body with no user rules.
func pipe(t *testing.T, body string) SanitizeResult {
	t.Helper()
	return SanitizeJSONWithRules(body, nil, false)
}

func TestEvasion_SeparatedDigits(t *testing.T) {
	cases := map[string]string{
		"spaced CNPJ":     "1 1 2 2 2 3 3 3 0 0 0 1 8 1",
		"dotted CPF":      "1.1.1.4.4.4.7.7.7.3.5",
		"dashed CNPJ":     "1-1-2-2-2-3-3-3-0-0-0-1-8-1",
		"underscored CPF": "1_1_1_4_4_4_7_7_7_3_5",
	}
	for name, val := range cases {
		t.Run(name, func(t *testing.T) {
			res := pipe(t, `{"note":"`+val+`"}`)
			if !res.Modified {
				t.Fatalf("%s: expected redaction, got none", name)
			}
			if !strings.Contains(res.Output, "REDACTED_BY_ALCATRAZ") {
				t.Fatalf("%s: no redaction marker in %q", name, res.Output)
			}
		})
	}
}

func TestEvasion_ZeroWidthSeparated(t *testing.T) {
	// A zero-width space between every digit of a valid CNPJ.
	var b strings.Builder
	for i, r := range validCNPJ {
		if i > 0 {
			b.WriteRune('​')
		}
		b.WriteRune(r)
	}
	res := pipe(t, `{"x":"`+b.String()+`"}`)
	if !res.Modified || !strings.Contains(res.Output, "REDACTED_BY_ALCATRAZ") {
		t.Fatalf("zero-width separated CNPJ not redacted: %q", res.Output)
	}
}

func TestEvasion_FullWidthDigits(t *testing.T) {
	// validCNPJ rendered with full-width digit runes (U+FF10..U+FF19).
	var b strings.Builder
	for _, r := range validCNPJ {
		b.WriteRune(0xFF10 + (r - '0'))
	}
	res := pipe(t, `{"x":"`+b.String()+`"}`)
	if !res.Modified || !strings.Contains(res.Output, "REDACTED_BY_ALCATRAZ") {
		t.Fatalf("full-width CNPJ not redacted: %q", res.Output)
	}
}

func TestEvasion_ReversedContiguous(t *testing.T) {
	rev := reverseASCII(validCNPJ) // 18100033322211
	res := pipe(t, `{"x":"`+rev+`"}`)
	if !res.Modified || !strings.Contains(res.Output, "REDACTED_BY_ALCATRAZ") {
		t.Fatalf("reversed CNPJ not redacted: %q", res.Output)
	}
	if strings.Contains(res.Output, rev) {
		t.Fatalf("reversed CNPJ digits leaked: %q", res.Output)
	}
}

func TestEvasion_Base64(t *testing.T) {
	// base64("11222333000181")
	res := pipe(t, `{"x":"MTEyMjIzMzMwMDAxODE="}`)
	if !res.Modified || !strings.Contains(res.Output, "REDACTED_BY_ALCATRAZ_ENCODED") {
		t.Fatalf("base64 CNPJ not redacted: %q", res.Output)
	}
	if strings.Contains(res.Output, "MTEyMjIz") {
		t.Fatalf("base64 payload leaked: %q", res.Output)
	}
}

func TestEvasion_Hex(t *testing.T) {
	// hex("11222333000181")
	res := pipe(t, `{"x":"3131323232333333303030313831"}`)
	if !res.Modified || !strings.Contains(res.Output, "REDACTED_BY_ALCATRAZ") {
		t.Fatalf("hex CNPJ not redacted: %q", res.Output)
	}
}

func TestEvasion_NestedBase64(t *testing.T) {
	// base64(base64("11222333000181")) — one wrapping layer.
	res := pipe(t, `{"x":"TVRFeU1qSXpNek13TURBeE9ERT0="}`)
	if !res.Modified || !strings.Contains(res.Output, "REDACTED_BY_ALCATRAZ_ENCODED") {
		t.Fatalf("nested base64 CNPJ not redacted: %q", res.Output)
	}
}

func TestEvasion_NoFalsePositive_InvalidChecksum(t *testing.T) {
	// Spaced digits with no valid CPF/CNPJ/card window (forward or reversed)
	// must pass through.
	res := pipe(t, `{"x":"5 2 6 0 1 8 1 5 9 0 8 3 0 1"}`)
	if res.Modified {
		t.Fatalf("invalid-checksum digits were redacted: %q", res.Output)
	}
}

func TestEvasion_NoFalsePositive_Prose(t *testing.T) {
	// Ordinary long words must not be decoded and redacted.
	res := pipe(t, `{"x":"internationalization and containerization are long words"}`)
	if res.Modified {
		t.Fatalf("prose was redacted: %q", res.Output)
	}
}

func TestEvasion_NoFalsePositive_HashNotSensitive(t *testing.T) {
	// A 64-char hex SHA-256 that decodes to random bytes must pass through.
	res := pipe(t, `{"x":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}`)
	if res.Modified {
		t.Fatalf("random hash was redacted: %q", res.Output)
	}
}

// The next three guard the machine identifiers every AI CLI puts in its
// request bodies. Corrupting one is not a cosmetic false positive: the Codex
// CLI's POST /backend-api/codex/responses came back {"detail":"Bad Request"}
// because a UUID conversation id had been partially redacted, which stalled
// the session. Sensitive data must still be caught (the tests above), but a
// digit run that is merely long is not sensitive data.

func TestEvasion_NoFalsePositive_UUID(t *testing.T) {
	// UUIDs whose digit-only segments used to satisfy a card/CNS window. The
	// hex letters around each run are what marks it as an identifier.
	for _, id := range []string{
		"71b1300b-6146-7970-8433-7593144e95fe",
		"59478666-4043-7815-81b5-ca8f159fd3fd",
		"5a8fb074-1867-7954-8340-c2a225d9e178",
	} {
		res := pipe(t, `{"conversation_id":"`+id+`"}`)
		if res.Modified {
			t.Errorf("UUID %s was corrupted: %q", id, res.Output)
		}
	}
}

func TestEvasion_NoFalsePositive_NanosecondTimestamp(t *testing.T) {
	// 19-digit epoch-nanosecond stamps (OTLP telemetry, request ids). A run of
	// contiguous digits offers exactly one window — its full width — so a
	// shorter document must not be matched by sliding inside it.
	for _, ts := range []string{
		"1785779875135340916",
		"1785779875135341916",
		"1785779875135342916",
		"1754239838123456789",
	} {
		res := pipe(t, `{"timeUnixNano":"`+ts+`"}`)
		if res.Modified {
			t.Errorf("timestamp %s was redacted: %q", ts, res.Output)
		}
	}
}

func TestEvasion_SeparatedDigits_SurviveNeighbouringWords(t *testing.T) {
	// The hex-adjacency rule keys on a letter touching a digit directly. A
	// label ending in a hex letter but separated by punctuation (here "CPF:")
	// must NOT disarm the detector.
	res := pipe(t, `{"note":"CPF: 111.444.777-35"}`)
	if !res.Modified || !strings.Contains(res.Output, "REDACTED_BY_ALCATRAZ") {
		t.Fatalf("labelled CPF not redacted: %q", res.Output)
	}
}
