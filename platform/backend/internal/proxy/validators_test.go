package proxy

import (
	"strings"
	"testing"
)

func TestValidateCPF(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"123.456.789-09", true},      // valid, formatted
		{"52998224725", true},         // valid, bare
		{"529.982.247-25", true},      // valid, formatted
		{"cpf: 529.982.247-25", true}, // valid, with context prefix
		{"123.456.789-10", false},     // wrong check digits
		{"111.111.111-11", false},     // all-same-digit
		{"000.000.000-00", false},     // all zeros
		{"123.456.789", false},        // too short
		{"12345678901234", false},     // too long
	}
	for _, c := range cases {
		if got := validateCPF(c.in); got != c.want {
			t.Errorf("validateCPF(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestValidateCNPJ(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"11.222.333/0001-81", true}, // valid, formatted
		{"11222333000181", true},     // valid, bare
		{"cnpj 11.222.333/0001-81", true},
		{"12.345.678/0001-90", false}, // wrong check digits
		{"11.111.111/1111-11", false}, // all-same-digit
		{"11.222.333/0001-80", false}, // wrong final digit
		{"11.222.333/0001", false},    // too short
	}
	for _, c := range cases {
		if got := validateCNPJ(c.in); got != c.want {
			t.Errorf("validateCNPJ(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestValidateCard(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"4111 1111 1111 1111", true}, // Visa test number, Luhn-valid
		{"4111111111111111", true},
		{"5500 0000 0000 0004", true},  // Mastercard test number
		{"3400 0000 0000 009", true},   // Amex test number
		{"6011 0000 0000 0004", true},  // Discover test number
		{"4111 1111 1111 1112", false}, // Luhn fails
		{"1234 5678 9012 3456", false}, // unknown issuer + Luhn fails
		{"1111 1111 1111 1111", false}, // no valid issuer prefix
		{"4111 1111 1111", false},      // too short
	}
	for _, c := range cases {
		if got := validateCard(c.in); got != c.want {
			t.Errorf("validateCard(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestValidatePIS(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"120.34567.89-9", true}, // valid, formatted
		{"12034567899", true},    // valid, bare
		{"12034567890", false},   // wrong check digit
		{"11111111111", false},   // all-same-digit
		{"1203456789", false},    // too short
	}
	for _, c := range cases {
		if got := validatePIS(c.in); got != c.want {
			t.Errorf("validatePIS(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestValidateCNS(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"700000000000005", true},  // valid provisional (starts 7)
		{"100000000000007", true},  // valid definitive (starts 1)
		{"700000000000004", false}, // wrong final digit
		{"300000000000000", false}, // invalid leading digit
		{"70000000000000", false},  // too short (14)
	}
	for _, c := range cases {
		if got := validateCNS(c.in); got != c.want {
			t.Errorf("validateCNS(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestValidateIBAN(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"GB82WEST12345698765432", true},      // canonical valid
		{"DE89370400440532013000", true},      // canonical valid
		{"GB82 WEST 1234 5698 7654 32", true}, // valid with spaces
		{"GB82WEST12345698765433", false},     // wrong checksum
		{"XX00", false},                       // too short
	}
	for _, c := range cases {
		if got := validateIBAN(c.in); got != c.want {
			t.Errorf("validateIBAN(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestBarePatterns_ValidatorGating verifies the checksum validator gates the
// aggressive context-free patterns end-to-end: valid documents are redacted,
// structurally invalid look-alikes pass through untouched.
func TestBarePatterns_ValidatorGating(t *testing.T) {
	// Valid bare CPF (no surrounding keyword) must be redacted.
	valid := SanitizeText("id 52998224725 done", false)
	if !strings.Contains(valid.Output, "[REDACTED_BY_ALCATRAZ_CPF]") {
		t.Errorf("valid bare CPF not redacted: %q", valid.Output)
	}
	// An 11-digit number with a bad check digit must survive.
	invalid := SanitizeText("id 52998224720 done", false)
	if strings.Contains(invalid.Output, "[REDACTED") {
		t.Errorf("invalid 11-digit number was redacted: %q", invalid.Output)
	}
}
