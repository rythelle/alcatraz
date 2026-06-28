package proxy

import (
	"encoding/json"
	"strings"
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

func SanitizeText(text string, dryRun bool) SanitizeResult {
	detections := make([]Detection, 0)
	modified := false

	for _, sp := range SensitivePatterns {
		matches := sp.Regex.FindAllStringIndex(text, -1)
		if len(matches) == 0 {
			continue
		}

		detections = append(detections, Detection{
			Pattern: sp.Name,
			Count:   len(matches),
		})

		if !dryRun {
			text = sp.Regex.ReplaceAllString(text, sp.Replacement)
			modified = true
		}
	}

	return SanitizeResult{
		Output:     text,
		Detections: detections,
		Modified:   modified,
	}
}

func SanitizeJSON(rawJSON string, dryRun bool) SanitizeResult {
	var data interface{}
	if err := json.Unmarshal([]byte(rawJSON), &data); err != nil {
		return SanitizeResult{Output: rawJSON}
	}

	sanitized, detections, modified := sanitizeValue(data, dryRun)

	var output string
	if modified && !dryRun {
		result, err := json.Marshal(sanitized)
		if err != nil {
			return SanitizeResult{Output: rawJSON}
		}
		output = string(result)
	} else {
		output = rawJSON
	}

	return SanitizeResult{
		Output:     output,
		Detections: detections,
		Modified:   modified,
	}
}

func sanitizeValue(obj interface{}, dryRun bool) (interface{}, []Detection, bool) {
	switch v := obj.(type) {
	case string:
		return sanitizeString(v, dryRun)
	case map[string]interface{}:
		return sanitizeMap(v, dryRun)
	case []interface{}:
		return sanitizeSlice(v, dryRun)
	default:
		return obj, nil, false
	}
}

func sanitizeString(text string, dryRun bool) (string, []Detection, bool) {
	detections := make([]Detection, 0)
	modified := false

	for _, sp := range SensitivePatterns {
		matches := sp.Regex.FindAllStringIndex(text, -1)
		if len(matches) == 0 {
			continue
		}

		detections = append(detections, Detection{
			Pattern: sp.Name,
			Count:   len(matches),
		})
		modified = true

		if !dryRun {
			text = sp.Regex.ReplaceAllString(text, sp.Replacement)
		}
	}

	return text, detections, modified
}

func sanitizeMap(m map[string]interface{}, dryRun bool) (map[string]interface{}, []Detection, bool) {
	allDetections := make([]Detection, 0)
	modified := false
	result := make(map[string]interface{}, len(m))

	for k, v := range m {
		newV, dets, mod := sanitizeValue(v, dryRun)
		allDetections = append(allDetections, dets...)
		modified = modified || mod
		result[k] = newV
	}

	return result, allDetections, modified
}

func sanitizeSlice(s []interface{}, dryRun bool) ([]interface{}, []Detection, bool) {
	allDetections := make([]Detection, 0)
	modified := false
	result := make([]interface{}, len(s))

	for i, item := range s {
		newItem, dets, mod := sanitizeValue(item, dryRun)
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
	for _, sp := range SensitivePatterns {
		if sp.Regex.MatchString(text) {
			return true
		}
	}
	return false
}

func CountMatches(text string) map[string]int {
	counts := make(map[string]int)
	for _, sp := range SensitivePatterns {
		if n := len(sp.Regex.FindAllStringIndex(text, -1)); n > 0 {
			counts[sp.Name] = n
		}
	}
	return counts
}
