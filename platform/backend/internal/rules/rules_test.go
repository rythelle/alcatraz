package rules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse_Valid(t *testing.T) {
	yml := `
redact:
  - name: proprietary-formula
    literal: "correction_factor = 1.4423"
    replace: "[PROPRIETARY_FORMULA]"
  - name: acme-algo
    regex: 'AcmeAlgo(V[0-9]+)?'
allow:
  - "111.444.777-35"
markers:
  enabled: true
mode: strict
`
	rs, err := Parse([]byte(yml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rs.RuleCount() != 2 {
		t.Errorf("RuleCount = %d, want 2", rs.RuleCount())
	}
	if rs.Custom[0].Replace != "[PROPRIETARY_FORMULA]" {
		t.Errorf("custom replace = %q", rs.Custom[0].Replace)
	}
	// literal must be treated as an exact substring (metachars escaped)
	if !rs.Custom[0].Re.MatchString("x correction_factor = 1.4423 y") {
		t.Error("literal rule did not match its exact substring")
	}
	if rs.Custom[1].Replace != DefaultCustomReplace {
		t.Errorf("default replace not applied: %q", rs.Custom[1].Replace)
	}
	if len(rs.Allow) != 1 || rs.Allow[0] != "111.444.777-35" {
		t.Errorf("allow = %v", rs.Allow)
	}
	if !rs.Markers.Enabled || rs.Markers.Start != DefaultMarkerStart {
		t.Errorf("marker defaults not applied: %+v", rs.Markers)
	}
	if !rs.Strict {
		t.Error("expected strict mode")
	}
}

func TestParse_Invalid(t *testing.T) {
	cases := map[string]string{
		"missing name":       "redact:\n  - literal: x\n",
		"both literal+regex": "redact:\n  - name: a\n    literal: x\n    regex: y\n",
		"neither":            "redact:\n  - name: a\n",
		"bad regex":          "redact:\n  - name: a\n    regex: '('\n",
		"duplicate name":     "redact:\n  - name: a\n    literal: x\n  - name: a\n    literal: y\n",
		"bad mode":           "mode: paranoid\n",
		"broken yaml":        "redact: [oops\n",
	}
	for name, yml := range cases {
		if _, err := Parse([]byte(yml)); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}

func TestLoad_MissingFileIsEmpty(t *testing.T) {
	rs, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if rs.RuleCount() != 0 {
		t.Errorf("expected empty rule set, got %d rules", rs.RuleCount())
	}
}

func TestLoad_ValidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guard-rules.yml")
	if err := os.WriteFile(path, []byte("redact:\n  - name: a\n    literal: secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rs, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rs.RuleCount() != 1 {
		t.Errorf("RuleCount = %d, want 1", rs.RuleCount())
	}
}

func TestMarkersApply_Unclosed(t *testing.T) {
	rs, err := Parse([]byte("markers:\n  enabled: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	// Closed block redacted; content between markers gone.
	out, count, unclosed := rs.Markers.Apply("a alcatraz:hide-start SECRET alcatraz:hide-end b")
	if count != 1 || unclosed {
		t.Errorf("closed block: count=%d unclosed=%v", count, unclosed)
	}
	if out != "a "+DefaultMarkerReplace+" b" {
		t.Errorf("closed block output = %q", out)
	}
	// Unclosed marker redacts to end of text (fail-closed).
	out, count, unclosed = rs.Markers.Apply("keep alcatraz:hide-start LEAK trailing")
	if !unclosed || count != 1 {
		t.Errorf("unclosed: count=%d unclosed=%v", count, unclosed)
	}
	if out != "keep "+DefaultMarkerReplace {
		t.Errorf("unclosed output = %q", out)
	}
}
