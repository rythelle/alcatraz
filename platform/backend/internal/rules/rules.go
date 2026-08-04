// Package rules loads, validates, and compiles the Guard's user rules
// file (~/.alcatraz/guard-rules.yml, mounted read-only into the backend).
//
// A rules file lets users hide proprietary code / trade secrets from AI
// providers via custom literal/regex redactions and inline code markers, keep
// specific values from being redacted (allowlist), and toggle strict-mode
// built-in patterns.
//
// Compilation is atomic: any invalid rule rejects the whole file so a partial
// or malformed edit never silently disables protection.
package rules

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Defaults applied when a field is omitted.
const (
	DefaultCustomReplace = "[REDACTED_BY_ALCATRAZ_CUSTOM]"
	DefaultMarkerStart   = "alcatraz:hide-start"
	DefaultMarkerEnd     = "alcatraz:hide-end"
	DefaultMarkerReplace = "[CODE_HIDDEN_BY_ALCATRAZ]"
)

// CustomRule is a compiled user redaction rule.
type CustomRule struct {
	Name    string
	Re      *regexp.Regexp
	Replace string
}

// Markers configures inline code-hiding markers. A block between Start and End
// (in any comment style — only the tokens are matched) is replaced wholesale.
type Markers struct {
	Enabled bool
	Start   string
	End     string
	Replace string
	block   *regexp.Regexp
}

// Apply redacts marker blocks in text. It returns the rewritten text, the
// number of blocks redacted, and whether an unclosed marker was found. An
// unclosed start marker is fail-closed: everything from the marker to the end
// of text is redacted (a broken prompt beats leaking half a secret).
func (m Markers) Apply(text string) (string, int, bool) {
	if !m.Enabled || m.block == nil {
		return text, 0, false
	}
	count := 0
	out := m.block.ReplaceAllStringFunc(text, func(string) string {
		count++
		return m.Replace
	})
	unclosed := false
	if idx := strings.Index(out, m.Start); idx >= 0 {
		out = out[:idx] + m.Replace
		count++
		unclosed = true
	}
	return out, count, unclosed
}

// RuleSet is an immutable, compiled snapshot of a rules file. A nil *RuleSet is
// valid and means "no user rules" — built-in patterns still apply.
type RuleSet struct {
	Custom  []CustomRule
	Allow   []string
	Markers Markers
	Strict  bool
	// Source counts, for status reporting.
	rawRedactCount int
}

// RuleCount reports the number of custom redact rules.
func (rs *RuleSet) RuleCount() int {
	if rs == nil {
		return 0
	}
	return len(rs.Custom)
}

// AllowCount reports the number of allowlist entries.
func (rs *RuleSet) AllowCount() int {
	if rs == nil {
		return 0
	}
	return len(rs.Allow)
}

// ── YAML schema ────────────────────────────────────────────────────────────

type rawConfig struct {
	Redact  []rawRule  `yaml:"redact"`
	Allow   []string   `yaml:"allow"`
	Markers rawMarkers `yaml:"markers"`
	Mode    string     `yaml:"mode"`
}

type rawRule struct {
	Name    string `yaml:"name"`
	Literal string `yaml:"literal"`
	Regex   string `yaml:"regex"`
	Replace string `yaml:"replace"`
}

type rawMarkers struct {
	Enabled bool   `yaml:"enabled"`
	Start   string `yaml:"start"`
	End     string `yaml:"end"`
	Replace string `yaml:"replace"`
}

// Parse validates and compiles YAML rules-file bytes into a RuleSet. Any
// invalid rule returns an error and no RuleSet (atomic validation).
func Parse(data []byte) (*RuleSet, error) {
	var cfg rawConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	rs := &RuleSet{rawRedactCount: len(cfg.Redact)}

	seen := make(map[string]bool, len(cfg.Redact))
	for i, r := range cfg.Redact {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			return nil, fmt.Errorf("redact rule #%d: missing name", i+1)
		}
		if seen[name] {
			return nil, fmt.Errorf("redact rule %q: duplicate name", name)
		}
		seen[name] = true

		hasLiteral := r.Literal != ""
		hasRegex := r.Regex != ""
		if hasLiteral == hasRegex {
			return nil, fmt.Errorf("redact rule %q: exactly one of 'literal' or 'regex' is required", name)
		}

		var re *regexp.Regexp
		var err error
		if hasLiteral {
			re, err = regexp.Compile(regexp.QuoteMeta(r.Literal))
		} else {
			re, err = regexp.Compile(r.Regex)
		}
		if err != nil {
			return nil, fmt.Errorf("redact rule %q: invalid pattern: %w", name, err)
		}

		replace := r.Replace
		if replace == "" {
			replace = DefaultCustomReplace
		}
		rs.Custom = append(rs.Custom, CustomRule{Name: name, Re: re, Replace: replace})
	}

	for _, a := range cfg.Allow {
		if a = strings.TrimSpace(a); a != "" {
			rs.Allow = append(rs.Allow, a)
		}
	}

	rs.Markers = compileMarkers(cfg.Markers)

	switch strings.ToLower(strings.TrimSpace(cfg.Mode)) {
	case "", "balanced":
		rs.Strict = false
	case "strict":
		rs.Strict = true
	default:
		return nil, fmt.Errorf("invalid mode %q: expected 'balanced' or 'strict'", cfg.Mode)
	}

	return rs, nil
}

func compileMarkers(m rawMarkers) Markers {
	out := Markers{
		Enabled: m.Enabled,
		Start:   firstNonEmpty(m.Start, DefaultMarkerStart),
		End:     firstNonEmpty(m.End, DefaultMarkerEnd),
		Replace: firstNonEmpty(m.Replace, DefaultMarkerReplace),
	}
	if out.Enabled {
		out.block = regexp.MustCompile(`(?s)` + regexp.QuoteMeta(out.Start) + `.*?` + regexp.QuoteMeta(out.End))
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// Load reads and compiles a rules file. A missing file is not an error — it
// returns an empty RuleSet (built-in patterns still apply). A present but
// invalid file returns an error so the caller can keep the last valid set.
func Load(path string) (*RuleSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &RuleSet{}, nil
		}
		return nil, err
	}
	return Parse(data)
}
