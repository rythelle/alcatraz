package proxy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/alcatraz/alcatraz/internal/rules"
)

type Detection struct {
	Pattern string `json:"pattern"`
	Count   int    `json:"count"`
}

type SanitizeResult struct {
	Output     string
	Detections []Detection
	Modified   bool
}

// dynamicRefRe matches a CloudFormation dynamic reference —
// `{{resolve:secretsmanager:<name>:SecretString}}`, `{{resolve:ssm:<name>:<v>}}`
// and friends. The inner `${...}` alternative lets a Serverless variable sit
// inside the name (`notifier-${self:custom.stage}-token`), which is the common
// shape. A reference is a pointer to a secret store, never a secret value.
var dynamicRefRe = regexp.MustCompile(`\{\{resolve:(?:[^{}]|\$\{[^{}]*\})*\}\}`)

// placeholder builds the stand-in for an allowlisted value, which the pipeline
// must leave alone.
//
// The `$` separators are load-bearing: the assignment patterns (env_secret,
// generic_secret) capture their value as a run of `[^\s\n;,$]{8,}`, which `$`
// cuts short. Without them an allowlisted value sitting in `API_TOKEN: <value>`
// would be redacted anyway — destroying the placeholder, so step ⑥ would find
// nothing to restore and the original value would be lost for good.
//
// The same trick deliberately is NOT used for dynamic references: hiding a
// span mid-value also hides whatever is glued to it, which turns protection
// into an evasion. Those are handled by position — see applyPatternOutsideRefs.
func placeholder(kind string, i int) string {
	return fmt.Sprintf("\x00ALCZ$%s$%d$\x00", kind, i)
}

// applyPattern replaces every valid match of sp in text with its replacement,
// honoring an optional structural validator. It returns the (possibly)
// rewritten text and the number of matches actually redacted. Matches that
// fail sp.Validate are left in place and not counted.
func applyPattern(sp SensitivePattern, text string, v *Vault) (string, int) {
	return applyPatternOutsideRefs(sp, text, v, nil)
}

// applyPatternOutsideRefs is applyPattern with dynamic-reference awareness.
// refs are the `{{resolve:...}}` spans in text; a match is left alone only
// when the reference is the sole reason it matched.
func applyPatternOutsideRefs(sp SensitivePattern, text string, v *Vault, refs [][]int) (string, int) {
	validate := patternValidators[sp.Name]
	locs := sp.Regex.FindAllStringIndex(text, -1)
	if len(locs) == 0 {
		return text, 0
	}

	var b strings.Builder
	last, count := 0, 0
	for _, loc := range locs {
		m := text[loc[0]:loc[1]]
		if validate != nil && !validate(m) {
			continue
		}
		if len(refs) > 0 && matchIsJustARef(sp, validate, text, loc, refs) {
			continue
		}
		b.WriteString(text[last:loc[0]])
		b.WriteString(tokenOrMarker(v, m, sp.Replacement))
		last = loc[1]
		count++
	}
	if count == 0 {
		return text, 0
	}
	b.WriteString(text[last:])
	return b.String(), count
}

// matchIsJustARef reports whether the match at loc only matched because of the
// dynamic reference(s) it covers. Cut the reference text out of the match and
// ask the pattern again: `TOKEN: '{{resolve:ssm:/a/b:1}}'` reduces to
// `TOKEN: ”`, which is not a secret, so the reference is kept. But
// `TOKEN: {{resolve:a}}hunter2secret` reduces to `TOKEN: hunter2secret`, which
// still matches — so it is redacted, reference and all. Without this second
// question, gluing a reference in front of a secret would suppress redaction.
func matchIsJustARef(sp SensitivePattern, validate func(string) bool, text string, loc []int, refs [][]int) bool {
	stripped, overlapped := cutRefSpans(text, loc, refs)
	if !overlapped {
		return false
	}
	for _, m := range sp.Regex.FindAllString(stripped, -1) {
		if validate == nil || validate(m) {
			return false
		}
	}
	return true
}

// cutRefSpans returns the match text with the parts covered by refs removed.
// overlapped is false when the match touches no reference at all.
func cutRefSpans(text string, loc []int, refs [][]int) (string, bool) {
	var b strings.Builder
	pos := loc[0]
	overlapped := false
	for _, r := range refs {
		if r[1] <= loc[0] || r[0] >= loc[1] {
			continue
		}
		overlapped = true
		s, e := max(r[0], loc[0]), min(r[1], loc[1])
		if pos < s {
			b.WriteString(text[pos:s])
		}
		if e > pos {
			pos = e
		}
	}
	if !overlapped {
		return "", false
	}
	if pos < loc[1] {
		b.WriteString(text[pos:loc[1]])
	}
	return b.String(), true
}

// refSpans locates the dynamic references in text, cheaply skipping the scan
// for the overwhelming majority of bodies that contain none.
func refSpans(text string) [][]int {
	if !strings.Contains(text, "{{resolve:") {
		return nil
	}
	return dynamicRefRe.FindAllStringIndex(text, -1)
}

func SanitizeText(text string, dryRun bool) SanitizeResult {
	detections := make([]Detection, 0)
	modified := false

	lower := strings.ToLower(text)
	for _, sp := range SensitivePatterns {
		if canSkip(sp.Name, lower) {
			continue
		}
		out, count := applyPattern(sp, text, nil)
		if count == 0 {
			continue
		}

		detections = append(detections, Detection{
			Pattern: sp.Name,
			Count:   count,
		})

		if !dryRun {
			text = out
			lower = strings.ToLower(text)
			modified = true
		}
	}

	return SanitizeResult{
		Output:     text,
		Detections: detections,
		Modified:   modified,
	}
}

// SanitizeJSON runs the built-in Guard patterns only (no user rules).
// Retained for callers and tests that don't have a RuleSet.
func SanitizeJSON(rawJSON string, dryRun bool) SanitizeResult {
	return SanitizeJSONWithRules(rawJSON, nil, dryRun)
}

// SanitizeJSONWithVault is SanitizeJSONWithRules with reversible tokenization:
// when v is non-nil, redactions become vault tokens (restored on the response
// path) instead of destructive markers. A nil v is exactly SanitizeJSONWithRules.
func SanitizeJSONWithVault(rawJSON string, rs *rules.RuleSet, v *Vault, dryRun bool) SanitizeResult {
	return sanitizeJSON(rawJSON, rs, v, dryRun)
}

// SanitizeJSONWithRules sanitizes a JSON body running the full Guard
// pipeline for every string value: ① protect allowlisted values, ② custom user
// rules, ③ inline code markers, ④ built-in patterns (with validators, dynamic-
// reference awareness and, in strict mode, the strict-only patterns), ⑤
// anti-evasion, ⑥ restore allowlisted values. A nil rs applies built-in
// patterns only. Non-JSON input is treated as a single text blob so the
// pipeline still applies (used by the `-check` CLI path).
func SanitizeJSONWithRules(rawJSON string, rs *rules.RuleSet, dryRun bool) SanitizeResult {
	return sanitizeJSON(rawJSON, rs, nil, dryRun)
}

func sanitizeJSON(rawJSON string, rs *rules.RuleSet, v *Vault, dryRun bool) SanitizeResult {
	var data interface{}
	if err := json.Unmarshal([]byte(rawJSON), &data); err != nil {
		out, detections, modified := sanitizePipeline(rawJSON, rs, v, dryRun)
		output := rawJSON
		if modified && !dryRun {
			output = out
		}
		return SanitizeResult{Output: output, Detections: detections, Modified: modified}
	}

	sanitized, detections, modified := sanitizeValue(data, rs, v, dryRun)

	output := rawJSON
	if modified && !dryRun {
		result, err := json.Marshal(sanitized)
		if err != nil {
			return SanitizeResult{Output: rawJSON}
		}
		output = string(result)
	}

	return SanitizeResult{
		Output:     output,
		Detections: detections,
		Modified:   modified,
	}
}

func sanitizeValue(obj interface{}, rs *rules.RuleSet, v *Vault, dryRun bool) (interface{}, []Detection, bool) {
	switch val := obj.(type) {
	case string:
		return sanitizePipeline(val, rs, v, dryRun)
	case map[string]interface{}:
		return sanitizeMap(val, rs, v, dryRun)
	case []interface{}:
		return sanitizeSlice(val, rs, v, dryRun)
	default:
		return obj, nil, false
	}
}

// sanitizePipeline runs the ordered six-step Guard pipeline over a single
// string value.
func sanitizePipeline(text string, rs *rules.RuleSet, v *Vault, dryRun bool) (string, []Detection, bool) {
	detections := make([]Detection, 0)

	// ① Protect user-allowlisted literals: swap each for an inert placeholder
	// so no later step redacts it, restored in step ⑥.
	var restore map[string]string
	if rs != nil {
		for i, lit := range rs.Allow {
			if lit == "" || !strings.Contains(text, lit) {
				continue
			}
			tok := placeholder("ALLOW", i)
			text = strings.ReplaceAll(text, lit, tok)
			if restore == nil {
				restore = make(map[string]string)
			}
			restore[tok] = lit
		}
	}

	// Infrastructure dynamic references. These name a secret, they never carry
	// one — the value is resolved at deploy time by CloudFormation, not stored
	// in the file — so redacting them corrupts the template. They are handled
	// by position in step ④ rather than swapped out here: a placeholder sitting
	// in the middle of a value would also hide whatever it is glued to.
	refs := refSpans(text)

	// ② Custom user rules (logged as custom:<name>).
	if rs != nil {
		for _, cr := range rs.Custom {
			count := 0
			text = cr.Re.ReplaceAllStringFunc(text, func(m string) string {
				count++
				return tokenOrMarker(v, m, cr.Replace)
			})
			if count > 0 {
				detections = append(detections, Detection{Pattern: "custom:" + cr.Name, Count: count})
			}
		}
	}

	// ③ Inline code markers.
	if rs != nil && rs.Markers.Enabled {
		out, count, unclosed := rs.Markers.Apply(text)
		if count > 0 {
			text = out
			detections = append(detections, Detection{Pattern: "marker", Count: count})
		}
		if unclosed {
			detections = append(detections, Detection{Pattern: "marker_unclosed", Count: 1})
		}
	}

	// ④ Built-in patterns (plus strict-only patterns in strict mode). The
	// lowercased copy backs the literal prefilter; it is only rebuilt after a
	// pattern actually rewrites the text, which is the rare case.
	lower := strings.ToLower(text)
	sweep := func(patterns []SensitivePattern) {
		for _, sp := range patterns {
			if canSkip(sp.Name, lower) {
				continue
			}
			out, count := applyPatternOutsideRefs(sp, text, v, refs)
			if count > 0 {
				detections = append(detections, Detection{Pattern: sp.Name, Count: count})
				text = out
				lower = strings.ToLower(text)
				refs = refSpans(text) // spans shifted with the rewrite
			}
		}
	}
	sweep(SensitivePatterns)
	if rs != nil && rs.Strict {
		sweep(StrictPatterns)
	}

	// ⑤ Anti-evasion: catch sensitive data hidden by encoding, digit
	// separators, Unicode digits, or reversal — forms the literal patterns
	// above miss. Runs after them so contiguous, forward-valid values are
	// already redacted and only transformed representations remain. Runs
	// before the allowlist restore so protected literals are never decoded.
	if out, dets := applyAntiEvasion(text, v); len(dets) > 0 {
		text = out
		detections = append(detections, dets...)
	}

	// ⑥ Restore allowlisted values.
	for token, lit := range restore {
		text = strings.ReplaceAll(text, token, lit)
	}

	return text, detections, len(detections) > 0
}

func sanitizeMap(m map[string]interface{}, rs *rules.RuleSet, v *Vault, dryRun bool) (map[string]interface{}, []Detection, bool) {
	allDetections := make([]Detection, 0)
	modified := false
	result := make(map[string]interface{}, len(m))

	for k, val := range m {
		newV, dets, mod := sanitizeValue(val, rs, v, dryRun)
		allDetections = append(allDetections, dets...)
		modified = modified || mod
		result[k] = newV
	}

	return result, allDetections, modified
}

func sanitizeSlice(s []interface{}, rs *rules.RuleSet, v *Vault, dryRun bool) ([]interface{}, []Detection, bool) {
	allDetections := make([]Detection, 0)
	modified := false
	result := make([]interface{}, len(s))

	for i, item := range s {
		newItem, dets, mod := sanitizeValue(item, rs, v, dryRun)
		allDetections = append(allDetections, dets...)
		modified = modified || mod
		result[i] = newItem
	}

	return result, allDetections, modified
}

func IsJSON(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "json")
}

func MergeDetections(all ...[]Detection) []Detection {
	merged := make(map[string]int)
	for _, dets := range all {
		for _, d := range dets {
			merged[d.Pattern] += d.Count
		}
	}
	result := make([]Detection, 0, len(merged))
	for name, count := range merged {
		result = append(result, Detection{Pattern: name, Count: count})
	}
	return result
}

func HasSensitiveContent(text string) bool {
	lower := strings.ToLower(text)
	for _, sp := range SensitivePatterns {
		if canSkip(sp.Name, lower) {
			continue
		}
		validate := patternValidators[sp.Name]
		if validate == nil {
			if sp.Regex.MatchString(text) {
				return true
			}
			continue
		}
		for _, m := range sp.Regex.FindAllString(text, -1) {
			if validate(m) {
				return true
			}
		}
	}
	return false
}

func CountMatches(text string) map[string]int {
	counts := make(map[string]int)
	lower := strings.ToLower(text)
	for _, sp := range SensitivePatterns {
		if canSkip(sp.Name, lower) {
			continue
		}
		validate := patternValidators[sp.Name]
		count := 0
		for _, m := range sp.Regex.FindAllString(text, -1) {
			if validate == nil || validate(m) {
				count++
			}
		}
		if count > 0 {
			counts[sp.Name] = count
		}
	}
	return counts
}
