package proxy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alcatraz/alcatraz/internal/rules"
)

func mustRules(t *testing.T, yml string) *rules.RuleSet {
	t.Helper()
	rs, err := rules.Parse([]byte(yml))
	if err != nil {
		t.Fatalf("parse rules: %v", err)
	}
	return rs
}

func userContent(t *testing.T, output string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(output), &m); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	return m["msg"].(string)
}

// Allowlisted value must survive a built-in pattern that would otherwise
// redact it (step ① protect before ④ built-ins, ⑤ restore).
func TestPipeline_AllowlistSurvivesBuiltin(t *testing.T) {
	rs := mustRules(t, "allow:\n  - \"529.982.247-25\"\n")
	payload, _ := json.Marshal(map[string]any{"msg": "meu CPF é 529.982.247-25 ok"})
	res := SanitizeJSONWithRules(string(payload), rs, false)
	got := userContent(t, res.Output)
	if !strings.Contains(got, "529.982.247-25") {
		t.Errorf("allowlisted CPF was redacted: %q", got)
	}
	if strings.Contains(got, "[REDACTED") {
		t.Errorf("unexpected redaction: %q", got)
	}
}

// Custom rules run before built-ins and are labeled custom:<name>.
func TestPipeline_CustomRuleBeforeBuiltin(t *testing.T) {
	rs := mustRules(t, "redact:\n  - name: formula\n    literal: \"k = 1.4423\"\n    replace: \"[FORMULA]\"\n")
	payload, _ := json.Marshal(map[string]any{"msg": "the secret is k = 1.4423 today"})
	res := SanitizeJSONWithRules(string(payload), rs, false)
	got := userContent(t, res.Output)
	if !strings.Contains(got, "[FORMULA]") {
		t.Errorf("custom rule not applied: %q", got)
	}
	var found bool
	for _, d := range res.Detections {
		if d.Pattern == "custom:formula" {
			found = true
		}
	}
	if !found {
		t.Errorf("custom detection not labeled custom:formula: %+v", res.Detections)
	}
}

// An unclosed marker fail-closes: redact from the marker to the end of the value.
func TestPipeline_UnclosedMarkerRedactsToEnd(t *testing.T) {
	rs := mustRules(t, "markers:\n  enabled: true\n")
	payload, _ := json.Marshal(map[string]any{"msg": "public alcatraz:hide-start topsecret trailing text"})
	res := SanitizeJSONWithRules(string(payload), rs, false)
	got := userContent(t, res.Output)
	if strings.Contains(got, "topsecret") || strings.Contains(got, "trailing") {
		t.Errorf("unclosed marker leaked content: %q", got)
	}
	if !strings.HasPrefix(got, "public ") {
		t.Errorf("content before marker was altered: %q", got)
	}
}

// Strict mode enables strict-only patterns (bare SSN) that balanced mode skips.
func TestPipeline_StrictMode(t *testing.T) {
	payloadStr := func() string {
		b, _ := json.Marshal(map[string]any{"msg": "value 123-45-6789 here"})
		return string(b)
	}()

	balanced := SanitizeJSONWithRules(payloadStr, mustRules(t, "mode: balanced\n"), false)
	if strings.Contains(userContent(t, balanced.Output), "[REDACTED_BY_ALCATRAZ_SSN]") {
		t.Error("balanced mode should not redact a bare SSN")
	}

	strict := SanitizeJSONWithRules(payloadStr, mustRules(t, "mode: strict\n"), false)
	if !strings.Contains(userContent(t, strict.Output), "[REDACTED_BY_ALCATRAZ_SSN]") {
		t.Errorf("strict mode should redact a bare SSN: %q", userContent(t, strict.Output))
	}
}
