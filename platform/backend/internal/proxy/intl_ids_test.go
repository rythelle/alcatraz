package proxy

import (
	"strings"
	"testing"
)

// Valid, check-digit-correct sample IDs (verified out-of-band). The IMEI here
// starts with 9 so it is NOT also a valid card issuer prefix, isolating the
// imei_context path from payment_card.
const (
	validSIN     = "046454286"
	validIMEI    = "087723956425717"
	validBSN     = "111222333"
	validNIF     = "288261062"
	validDNI     = "12345678Z"
	validAadhaar = "380524998934"
)

// Context patterns match a keyword and the value inside the SAME string (the
// realistic case: a value sits in prose / file content, e.g. "SIN: 046454286").
// JSON keys are not part of their string value, so context lives in the text.
func TestIntlIDs_RedactedNearKeyword(t *testing.T) {
	cases := []struct{ text, marker string }{
		{"customer SIN: " + validSIN, "SIN"},
		{"device imei = " + validIMEI, "IMEI"},
		{"klant BSN " + validBSN, "BSN"},
		{"NIF: " + validNIF, "NIF"},
		{"su DNI " + validDNI, "DNI"},
		{"aadhaar " + validAadhaar, "AADHAAR"},
	}
	for _, c := range cases {
		res := SanitizeJSONWithRules(`{"note":"`+c.text+`"}`, nil, false)
		if !res.Modified || !strings.Contains(res.Output, "REDACTED_BY_ALCATRAZ_"+c.marker) {
			t.Errorf("%s: expected %s redaction, got %q", c.marker, c.marker, res.Output)
		}
	}
}

func TestIntlIDs_NoKeyword_NotRedacted(t *testing.T) {
	// Without the keyword, these single-check-digit IDs must pass through
	// (a bare match would redact ~10% of all numbers of that length).
	for _, v := range []string{validSIN, validBSN, validNIF} {
		res := SanitizeJSONWithRules(`{"value": "`+v+`"}`, nil, false)
		if strings.Contains(res.Output, "REDACTED") {
			t.Errorf("bare %s was redacted without a keyword: %q", v, res.Output)
		}
	}
}

func TestIntlIDs_InvalidChecksum_NotRedacted(t *testing.T) {
	// Right length, near the keyword, but wrong check digit → left alone.
	cases := []string{
		"SIN: 046454287",       // last digit off
		"BSN 111222334",        // breaks 11-test
		"NIF: 288261063",       // wrong check digit
		"DNI 12345678A",        // wrong control letter
		"aadhaar 380524998935", // breaks Verhoeff
	}
	for _, c := range cases {
		res := SanitizeJSONWithRules(`{"note":"`+c+`"}`, nil, false)
		if strings.Contains(res.Output, "REDACTED") {
			t.Errorf("invalid checksum was redacted: %q → %q", c, res.Output)
		}
	}
}

func TestIntlIDs_Base64OfKeywordedID(t *testing.T) {
	// base64("imei: 962439091608378") — decode-rescan should catch it because
	// the decoded text carries both the keyword and the valid number.
	b64 := "aW1laTogMDg3NzIzOTU2NDI1NzE3"
	res := SanitizeJSONWithRules(`{"x":"`+b64+`"}`, nil, false)
	if !res.Modified || !strings.Contains(res.Output, "REDACTED_BY_ALCATRAZ_ENCODED") {
		t.Errorf("base64 of keyworded IMEI not redacted: %q", res.Output)
	}
}
