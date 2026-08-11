package proxy

import (
	"strconv"
	"strings"
)

// Validators confirm that a regex match is structurally valid (correct check
// digits / checksum) before the Guard redacts it. Attaching a validator to
// a SensitivePattern lets the pattern match aggressively — bare, unformatted,
// context-free — while fake data with wrong check digits (common in test
// fixtures and documentation) passes through untouched.
//
// Each validator receives the FULL regex match (which may include a keyword
// prefix for context patterns) and extracts the digits it needs itself.

// onlyDigits returns the ASCII digits of s in order.
func onlyDigits(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteByte(byte(r))
		}
	}
	return b.String()
}

// allSameDigit reports whether every rune in s is the same digit (e.g.
// "00000000000"). Such sequences pass most mod-11 checks but are never real.
func allSameDigit(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] != s[0] {
			return false
		}
	}
	return true
}

// mod11DV computes a Brazilian mod-11 check digit for the given digit string
// using descending weights starting at startWeight. Returns 0 when the
// remainder is < 2, else 11-remainder.
func mod11DV(digits string, startWeight int) int {
	sum := 0
	w := startWeight
	for i := 0; i < len(digits); i++ {
		sum += int(digits[i]-'0') * w
		w--
		if w < 2 {
			w = 9
		}
	}
	rem := sum % 11
	if rem < 2 {
		return 0
	}
	return 11 - rem
}

// validateCPF validates an 11-digit Brazilian CPF via its two mod-11 check
// digits. The match may be formatted (123.456.789-09) or bare.
func validateCPF(match string) bool {
	d := onlyDigits(match)
	if len(d) != 11 || allSameDigit(d) {
		return false
	}
	dv1 := mod11DV(d[:9], 10)
	dv2 := mod11DV(d[:10], 11)
	return dv1 == int(d[9]-'0') && dv2 == int(d[10]-'0')
}

// cnpjWeights1/2 are the standard CNPJ mod-11 weight vectors.
var (
	cnpjWeights1 = []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	cnpjWeights2 = []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
)

func cnpjDV(digits string, weights []int) int {
	sum := 0
	for i := 0; i < len(weights); i++ {
		sum += int(digits[i]-'0') * weights[i]
	}
	rem := sum % 11
	if rem < 2 {
		return 0
	}
	return 11 - rem
}

// validateCNPJ validates a 14-digit Brazilian CNPJ via its two mod-11 check
// digits.
func validateCNPJ(match string) bool {
	d := onlyDigits(match)
	if len(d) != 14 || allSameDigit(d) {
		return false
	}
	dv1 := cnpjDV(d[:12], cnpjWeights1)
	dv2 := cnpjDV(d[:13], cnpjWeights2)
	return dv1 == int(d[12]-'0') && dv2 == int(d[13]-'0')
}

// luhnValid runs the Luhn (mod-10) checksum over a bare digit string.
func luhnValid(d string) bool {
	if len(d) == 0 {
		return false
	}
	sum := 0
	double := false
	for i := len(d) - 1; i >= 0; i-- {
		n := int(d[i] - '0')
		if double {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		double = !double
	}
	return sum%10 == 0
}

// cardIssuerValid reports whether a bare card number begins with a recognized
// major-issuer prefix. Combined with Luhn this rejects most random digit runs.
//
// Every range here is one an issuer actually assigns. The earlier rule ended
// in a catch-all for "50" and for any leading "6", which accepted a tenth of
// the whole number space on its own — so a UUID segment like 6146-7970-… only
// had to clear Luhn to be redacted as a payment card.
func cardIssuerValid(d string) bool {
	// inRange reports whether d starts inside the inclusive BIN range [lo,hi];
	// lo and hi must be the same width.
	inRange := func(lo, hi string) bool {
		w := len(lo)
		return len(d) >= w && d[:w] >= lo && d[:w] <= hi
	}
	switch {
	case strings.HasPrefix(d, "4"): // Visa
		return true
	case inRange("51", "55"), inRange("2221", "2720"): // Mastercard
		return true
	case inRange("34", "34"), inRange("37", "37"): // Amex
		return true
	case inRange("300", "305"), inRange("36", "36"), inRange("38", "39"): // Diners
		return true
	case inRange("6011", "6011"), inRange("644", "649"), inRange("65", "65"): // Discover
		return true
	case inRange("3528", "3589"): // JCB
		return true
	case inRange("622126", "622925"), inRange("624", "626"): // UnionPay
		return true
	case inRange("606282", "606282"), inRange("637095", "637095"),
		inRange("637568", "637568"), inRange("637599", "637599"),
		inRange("637609", "637609"), inRange("637612", "637612"): // Hipercard
		return true
	case inRange("438935", "438935"), inRange("451416", "451416"),
		inRange("457393", "457393"), inRange("504175", "504175"),
		inRange("506699", "506778"), inRange("509000", "509999"),
		inRange("627780", "627780"), inRange("636297", "636297"),
		inRange("636368", "636368"), inRange("650031", "650920"),
		inRange("651652", "651679"), inRange("655000", "655058"): // Elo
		return true
	}
	return false
}

// validateCard validates a payment card number: 13–19 digits, Luhn-valid, and
// a recognized issuer prefix.
func validateCard(match string) bool {
	d := onlyDigits(match)
	if len(d) < 13 || len(d) > 19 {
		return false
	}
	return luhnValid(d) && cardIssuerValid(d)
}

// pisWeights is the PIS/PASEP/NIS mod-11 weight vector over the first 10 digits.
var pisWeights = []int{3, 2, 9, 8, 7, 6, 5, 4, 3, 2}

// validatePIS validates an 11-digit Brazilian PIS/PASEP/NIS number.
func validatePIS(match string) bool {
	d := onlyDigits(match)
	if len(d) != 11 || allSameDigit(d) {
		return false
	}
	sum := 0
	for i := 0; i < 10; i++ {
		sum += int(d[i]-'0') * pisWeights[i]
	}
	rem := sum % 11
	dv := 11 - rem
	if dv >= 10 {
		dv = 0
	}
	return dv == int(d[10]-'0')
}

// validateCNS validates a 15-digit Brazilian CNS (Cartão Nacional de Saúde).
// Definitive cards start with 1 or 2, provisional with 7, 8 or 9; both satisfy
// a weighted sum (weights 15..1) divisible by 11.
func validateCNS(match string) bool {
	d := onlyDigits(match)
	if len(d) != 15 {
		return false
	}
	switch d[0] {
	case '1', '2', '7', '8', '9':
	default:
		return false
	}
	sum := 0
	for i := 0; i < 15; i++ {
		sum += int(d[i]-'0') * (15 - i)
	}
	return sum%11 == 0
}

// ── International national IDs (context-keyed) ───────────────────────────────
// Most national IDs carry only ONE check digit, so a bare match would redact
// ~10% of all numbers of that length. These validators are therefore attached
// to CONTEXT-keyed patterns (they fire only near a keyword like "SIN"/"BSN"),
// where the checksum is a false-positive filter rather than the sole signal.

// validateSIN validates a Canadian Social Insurance Number (9 digits, Luhn).
func validateSIN(match string) bool {
	d := onlyDigits(match)
	return len(d) == 9 && !allSameDigit(d) && luhnValid(d)
}

// validateIMEI validates a device IMEI (15 digits, Luhn).
func validateIMEI(match string) bool {
	d := onlyDigits(match)
	return len(d) == 15 && !allSameDigit(d) && luhnValid(d)
}

// bsnWeights is the Dutch BSN "elfproef" weight vector (last digit weighted -1).
var bsnWeights = []int{9, 8, 7, 6, 5, 4, 3, 2, -1}

// validateBSN validates a Dutch Burgerservicenummer (9 digits, 11-test).
func validateBSN(match string) bool {
	d := onlyDigits(match)
	if len(d) != 9 || allSameDigit(d) {
		return false
	}
	sum := 0
	for i := 0; i < 9; i++ {
		sum += int(d[i]-'0') * bsnWeights[i]
	}
	return sum%11 == 0
}

// validateNIF validates a Portuguese NIF (9 digits, mod-11 check digit).
func validateNIF(match string) bool {
	d := onlyDigits(match)
	if len(d) != 9 || allSameDigit(d) {
		return false
	}
	sum := 0
	for i := 0; i < 8; i++ {
		sum += int(d[i]-'0') * (9 - i)
	}
	chk := 11 - sum%11
	if chk >= 10 {
		chk = 0
	}
	return chk == int(d[8]-'0')
}

// dniLetters is the Spanish DNI control-letter table indexed by number mod 23.
const dniLetters = "TRWAGMYFPDXBNJZSQVHLCKE"

// validateDNI validates a Spanish DNI (8 digits + control letter). The letter
// is the last A–Z in the match, so a keyword prefix does not confuse it.
func validateDNI(match string) bool {
	d := onlyDigits(match)
	if len(d) != 8 {
		return false
	}
	var letter byte
	for i := len(match) - 1; i >= 0; i-- {
		c := match[i]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		if c >= 'A' && c <= 'Z' {
			letter = c
			break
		}
	}
	if letter == 0 {
		return false
	}
	n, err := strconv.Atoi(d)
	if err != nil {
		return false
	}
	return dniLetters[n%23] == letter
}

// Verhoeff multiplication (d) and permutation (p) tables for Aadhaar.
var (
	verhoeffD = [10][10]int{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{1, 2, 3, 4, 0, 6, 7, 8, 9, 5},
		{2, 3, 4, 0, 1, 7, 8, 9, 5, 6},
		{3, 4, 0, 1, 2, 8, 9, 5, 6, 7},
		{4, 0, 1, 2, 3, 9, 5, 6, 7, 8},
		{5, 9, 8, 7, 6, 0, 4, 3, 2, 1},
		{6, 5, 9, 8, 7, 1, 0, 4, 3, 2},
		{7, 6, 5, 9, 8, 2, 1, 0, 4, 3},
		{8, 7, 6, 5, 9, 3, 2, 1, 0, 4},
		{9, 8, 7, 6, 5, 4, 3, 2, 1, 0},
	}
	verhoeffP = [8][10]int{
		{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		{1, 5, 7, 6, 2, 8, 3, 0, 9, 4},
		{5, 8, 0, 3, 7, 9, 6, 1, 4, 2},
		{8, 9, 1, 6, 0, 4, 3, 5, 2, 7},
		{9, 4, 5, 3, 1, 2, 6, 8, 7, 0},
		{4, 2, 8, 6, 5, 7, 3, 9, 0, 1},
		{2, 7, 9, 3, 8, 0, 6, 4, 1, 5},
		{7, 0, 4, 6, 9, 1, 3, 2, 5, 8},
	}
)

// validateAadhaar validates an Indian Aadhaar (12 digits, Verhoeff checksum,
// first digit 2–9).
func validateAadhaar(match string) bool {
	d := onlyDigits(match)
	if len(d) != 12 || d[0] < '2' || allSameDigit(d) {
		return false
	}
	c := 0
	for i := 0; i < 12; i++ {
		digit := int(d[11-i] - '0')
		c = verhoeffD[c][verhoeffP[i%8][digit]]
	}
	return c == 0
}

// validateIBAN validates an International Bank Account Number via the ISO 13616
// mod-97 checksum. The match may contain spaces; letters are case-insensitive.
func validateIBAN(match string) bool {
	var sb strings.Builder
	for _, r := range match {
		switch {
		case r >= '0' && r <= '9':
			sb.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			sb.WriteRune(r)
		case r >= 'a' && r <= 'z':
			sb.WriteRune(r - 32)
		}
	}
	iban := sb.String()
	if len(iban) < 15 || len(iban) > 34 {
		return false
	}
	// Move the first four characters to the end.
	rearranged := iban[4:] + iban[:4]
	// Replace each letter with two digits: A=10 … Z=35, then take mod 97.
	rem := 0
	for i := 0; i < len(rearranged); i++ {
		c := rearranged[i]
		switch {
		case c >= '0' && c <= '9':
			rem = (rem*10 + int(c-'0')) % 97
		case c >= 'A' && c <= 'Z':
			v := int(c-'A') + 10
			rem = (rem*100 + v) % 97
		default:
			return false
		}
	}
	return rem == 1
}
