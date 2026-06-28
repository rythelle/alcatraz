package proxy

import (
	"encoding/json"
	"strings"
	"testing"
)

// ── Real-world payloads ───────────────────────────────────────────────────────
// Equivalent to TestRealWorldPayloads in data-guardian/test_guardian.py

func TestRealWorldPayload_ClaudeMessages(t *testing.T) {
	payload := map[string]any{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 8192,
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Leia o arquivo .env:\nDATABASE_URL=postgres://user:secret123@localhost:5432/db\nAWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			},
		},
		"system": "You are a helpful assistant.",
	}
	raw, _ := json.Marshal(payload)
	result := SanitizeJSON(string(raw), false)

	if !result.Modified {
		t.Fatal("expected modification")
	}
	var sanitized map[string]any
	if err := json.Unmarshal([]byte(result.Output), &sanitized); err != nil {
		t.Fatalf("sanitized JSON is invalid: %v", err)
	}
	msgs := sanitized["messages"].([]any)
	content := msgs[0].(map[string]any)["content"].(string)
	if strings.Contains(content, "secret123") {
		t.Error("DATABASE_URL password leaked")
	}
	if strings.Contains(content, "wJalrXUtnFEMI") {
		t.Error("AWS secret key leaked")
	}
	if !strings.Contains(content, "[REDACTED") {
		t.Error("expected redaction marker")
	}
	// System prompt should be untouched
	if sanitized["system"] != "You are a helpful assistant." {
		t.Errorf("system prompt modified: %v", sanitized["system"])
	}
}

func TestRealWorldPayload_GeminiGenerateContent(t *testing.T) {
	payload := map[string]any{
		"contents": []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": "My email is admin@example.com and my key is AIza" + "SyDdI0hCZtE6vySjMm-WEfRq3CPzqKqqsHI"},
				},
			},
		},
		"generationConfig": map[string]any{"temperature": 0.7},
	}
	raw, _ := json.Marshal(payload)
	result := SanitizeJSON(string(raw), false)

	if !result.Modified {
		t.Fatal("expected modification")
	}
	var sanitized map[string]any
	if err := json.Unmarshal([]byte(result.Output), &sanitized); err != nil {
		t.Fatalf("sanitized JSON is invalid: %v", err)
	}
	contents := sanitized["contents"].([]any)
	parts := contents[0].(map[string]any)["parts"].([]any)
	text := parts[0].(map[string]any)["text"].(string)
	if strings.Contains(text, "admin@example.com") {
		t.Error("email leaked")
	}
	if strings.Contains(text, "AIza"+"SyDdI0hCZtE6vySjMm") {
		t.Error("Google API key leaked")
	}
	if !strings.Contains(text, "[REDACTED") {
		t.Error("expected redaction marker")
	}
}

func TestRealWorldPayload_OpencodeCompletions(t *testing.T) {
	payload := map[string]any{
		"model":      "big-pickle",
		"max_tokens": 32000,
		"messages": []any{
			map[string]any{"role": "system", "content": "You are opencode..."},
			map[string]any{
				"role":    "user",
				"content": "Read test.txt:\nSTRIPE_SECRET_KEY=sk_" + "live" + "_51ABCDEF1234567890abcdefghijklmnopqrstuv\nJWT_SECRET=7b4f1d3a9e8c2f6d",
			},
		},
		"stream": true,
	}
	raw, _ := json.Marshal(payload)
	result := SanitizeJSON(string(raw), false)

	if !result.Modified {
		t.Fatal("expected modification")
	}
	var sanitized map[string]any
	if err := json.Unmarshal([]byte(result.Output), &sanitized); err != nil {
		t.Fatalf("sanitized JSON is invalid: %v", err)
	}
	msgs := sanitized["messages"].([]any)
	content := msgs[1].(map[string]any)["content"].(string)
	if strings.Contains(content, "sk_"+"live"+"_51ABCDEF") {
		t.Error("Stripe key leaked")
	}
	if strings.Contains(content, "7b4f1d3a9e8c2f6d") {
		t.Error("JWT secret leaked")
	}
	if !strings.Contains(content, "[REDACTED") {
		t.Error("expected redaction marker")
	}
	// System message should be untouched
	sysContent := msgs[0].(map[string]any)["content"].(string)
	if sysContent != "You are opencode..." {
		t.Errorf("system message modified: %v", sysContent)
	}
}

func TestRealWorldPayload_StripeKeyBug(t *testing.T) {
	// Regression: Stripe key passing through unsanitized when prefixed with env var name
	payload := map[string]any{
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "STRIPE_SECRET_KEY=sk_" + "live" + "_51QwXxYyZzAaBbCcDdEeFfGgHhIiJjKkLlMmNnOoPp",
			},
		},
	}
	raw, _ := json.Marshal(payload)
	result := SanitizeJSON(string(raw), false)

	if !result.Modified {
		t.Fatal("expected modification")
	}
	if strings.Contains(result.Output, "sk_"+"live"+"_51QwXxYyZzAaBbCc") {
		t.Error("Stripe key leaked")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_STRIPE_KEY]") &&
		!strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_ENV_SECRET]") {
		t.Error("expected Stripe or env-secret redaction")
	}
}

func TestRealWorldPayload_NestedMessagesWithPII(t *testing.T) {
	// Tests CPF and email inside nested messages (similar to Python's test_nested_json_payload)
	payload := map[string]any{
		"messages": []any{
			map[string]any{"role": "system", "content": "system prompt"},
			map[string]any{"role": "user", "content": "email: admin@example.com\nCPF: 529.982.247-25"},
			map[string]any{"role": "assistant", "content": "ok, received"},
		},
		"tools": []any{
			map[string]any{"name": "read_file"},
		},
	}
	raw, _ := json.Marshal(payload)
	result := SanitizeJSON(string(raw), false)

	if !result.Modified {
		t.Fatal("expected modification")
	}
	var sanitized map[string]any
	if err := json.Unmarshal([]byte(result.Output), &sanitized); err != nil {
		t.Fatalf("sanitized JSON is invalid: %v", err)
	}
	msgs := sanitized["messages"].([]any)
	userContent := msgs[1].(map[string]any)["content"].(string)
	if strings.Contains(userContent, "admin@example.com") {
		t.Error("email leaked in user message")
	}
	if strings.Contains(userContent, "529.982.247-25") {
		t.Error("CPF leaked in user message")
	}
	if !strings.Contains(userContent, "[REDACTED_BY_ALCATRAZ_EMAIL]") {
		t.Error("email not redacted")
	}
	if !strings.Contains(userContent, "[REDACTED_BY_ALCATRAZ_CPF]") {
		t.Error("CPF not redacted")
	}
	// Other messages must be untouched
	if msgs[0].(map[string]any)["content"].(string) != "system prompt" {
		t.Error("system message was modified")
	}
	if msgs[2].(map[string]any)["content"].(string) != "ok, received" {
		t.Error("assistant message was modified")
	}
}

func TestRealWorldPayload_ArrayChoices(t *testing.T) {
	// Equivalent to Python's test_json_array_sanitization with real-world shape
	payload := map[string]any{
		"choices": []any{
			map[string]any{"message": map[string]any{
				"content": "sk-" + "abcdefghijklmnopqrstuvwxyz0123456789ABCDEF0123456789ab",
			}},
			map[string]any{"message": map[string]any{
				"content": "innocent text",
			}},
		},
	}
	raw, _ := json.Marshal(payload)
	result := SanitizeJSON(string(raw), false)

	if !result.Modified {
		t.Fatal("expected modification")
	}
	var sanitized map[string]any
	if err := json.Unmarshal([]byte(result.Output), &sanitized); err != nil {
		t.Fatalf("sanitized JSON is invalid: %v", err)
	}
	choices := sanitized["choices"].([]any)
	c0 := choices[0].(map[string]any)["message"].(map[string]any)["content"].(string)
	c1 := choices[1].(map[string]any)["message"].(map[string]any)["content"].(string)
	if !strings.Contains(c0, "[REDACTED_BY_ALCATRAZ_OPENAI_KEY]") {
		t.Error("OpenAI key not redacted in first choice")
	}
	if c1 != "innocent text" {
		t.Errorf("clean content modified: %q", c1)
	}
}

// ── Regression bugs ───────────────────────────────────────────────────────────
// Equivalent to TestRegressionBugs in data-guardian/test_guardian.py

func TestRegressionNoEnvFileLinePattern(t *testing.T) {
	// The env_file_line pattern was removed because it corrupted JSON.
	// Ensure no pattern with that name exists.
	for _, sp := range SensitivePatterns {
		if sp.Name == "env_file_line" {
			t.Fatal("env_file_line pattern must not exist (corrupts JSON)")
		}
	}
}

func TestRegressionNoBrokenJSONAfterSanitization(t *testing.T) {
	tricky := []map[string]any{
		{"msg": "x-anthropic-billing-header is a reserved keyword"},
		{"msg": "Key=Value\nAnother=Secret\nJSON={\"a\":1}"},
		{"msg": "email@domain.com = password123"},
		{"msg": "sk-" + "ant-api03-xxxxxxxx\n" + "AIza" + "SyDdI0hCZtE6vySjMm" + "\n" + "AKIA" + "IOSFODNN7EXAMPLE"},
	}
	for _, payload := range tricky {
		raw, _ := json.Marshal(payload)
		result := SanitizeJSON(string(raw), false)
		// Must produce valid JSON
		var reparsed map[string]any
		if err := json.Unmarshal([]byte(result.Output), &reparsed); err != nil {
			t.Errorf("broken JSON after sanitizing %q: %v", payload["msg"], err)
		}
	}
}

func TestRegressionNewlineEscapesPreserved(t *testing.T) {
	// \n inside JSON strings must remain escaped after sanitization
	payload := map[string]any{"text": "line1\nSTRIPE_SECRET_KEY=sk_" + "live" + "_abcdefghijklmnopqrstuvwxyz\nline3"}
	raw, _ := json.Marshal(payload)
	result := SanitizeJSON(string(raw), false)

	if !result.Modified {
		return // nothing sanitized — skip newline check
	}
	// Re-serialized JSON must still contain \n escape sequences
	if !strings.Contains(result.Output, `\n`) {
		t.Error("\\n escape was lost after sanitization")
	}
	// Must be valid JSON and preserve newlines in parsed string
	var reparsed map[string]any
	if err := json.Unmarshal([]byte(result.Output), &reparsed); err != nil {
		t.Fatalf("invalid JSON after sanitization: %v", err)
	}
	text, _ := reparsed["text"].(string)
	if !strings.Contains(text, "\n") {
		t.Error("newline lost in reparsed string value")
	}
}

func TestRegressionCleanJSONUnchanged(t *testing.T) {
	// Sanitization must not modify clean JSON
	payload := map[string]any{"message": "hello world", "count": 42}
	raw, _ := json.Marshal(payload)
	input := string(raw)
	result := SanitizeJSON(input, false)
	if result.Modified {
		t.Fatal("clean JSON must not be modified")
	}
	if result.Output != input {
		t.Fatal("output differs from input for clean JSON")
	}
}

// ── No-secrets-leak ───────────────────────────────────────────────────────────
// Equivalent to TestNoSecretsLeak in data-guardian/test_guardian.py

func TestNoSecretsLeak(t *testing.T) {
	type sample struct {
		name           string
		message        string // full string placed in the JSON payload
		sensitiveValue string // substring that must NOT appear after sanitization
	}

	cases := []sample{
		{
			"openai_key",
			"The secret is: sk-" + "abcdefghijklmnopqrstuvwxyz1234567890123456789012345678",
			"sk-" + "abcdefghijklmnopqrstuvwxyz",
		},
		{
			"anthropic_key",
			"key: sk-" + "ant-api03xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			"sk-" + "ant-api03",
		},
		{
			"google_key",
			"AIza" + "SyDdI0hCZtE6vySjMm-WEfRq3CPzqKqqsHI",
			"AIza" + "SyDdI0hCZtE6vySjMm",
		},
		{
			"github_token",
			"ghp" + "_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			"ghp" + "_xxxxxxxxxxxxxxxxxx",
		},
		{
			"aws_access_key",
			"AKIA" + "IOSFODNN7EXAMPLE",
			"AKIA" + "IOSFODNN7EXAMPLE",
		},
		{
			"aws_secret_key",
			"aws_secret_access_key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			"wJalrXUtnFEMI",
		},
		{
			"stripe_secret",
			"sk_" + "live" + "_51ABCDEF1234567890abcdefghijklmnopqrstuv",
			"sk_" + "live" + "_51ABCDEF",
		},
		{
			"cpf",
			"CPF: 529.982.247-25",
			"529.982.247-25",
		},
		{
			"email",
			"contact: admin@example.com",
			"admin@example.com",
		},
		{
			"db_url",
			"DATABASE_URL=postgres://user:secret123@localhost:5432/db\nresto",
			"secret123",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := map[string]any{"message": tc.message}
			raw, _ := json.Marshal(payload)
			result := SanitizeJSON(string(raw), false)

			if strings.Contains(result.Output, tc.sensitiveValue) {
				t.Errorf("secret %q leaked in sanitized output", tc.sensitiveValue)
			}
		})
	}
}
