package proxy

import "testing"

func TestIsOAuthTokenEndpoint(t *testing.T) {
	tests := []struct {
		host string
		path string
		want bool
	}{
		// Claude Code OAuth token refresh (with and without port)
		{"platform.claude.com:443", "/v1/oauth/token", true},
		{"platform.claude.com", "/v1/oauth/token", true},
		{"console.anthropic.com:443", "/v1/oauth/token", true},
		{"claude.ai:443", "/v1/oauth/authorize", true},
		// Codex / Gemini
		{"auth.openai.com:443", "/oauth/token", true},
		// Codex's device-code grant: not under /oauth, so it needs its own
		// prefix. Sanitizing these bodies breaks `codex login --device-auth`.
		{"auth.openai.com:443", "/api/accounts/deviceauth/usercode", true},
		{"auth.openai.com", "/api/accounts/deviceauth/token", true},
		{"auth.openai.com:443", "/api/accounts/deviceauth/callback", true},
		// Neighbouring paths on the same host are NOT exempt.
		{"auth.openai.com:443", "/api/accounts/profile", false},
		{"auth.openai.com:443", "/api/accounts", false},
		{"oauth2.googleapis.com:443", "/token", true},
		{"accounts.google.com:443", "/o/oauth2/token", true},
		// Regular API traffic must still be sanitized
		{"api.anthropic.com:443", "/v1/messages", false},
		{"platform.claude.com:443", "/v1/messages", false},
		{"api.openai.com:443", "/v1/responses", false},
		{"opencode.ai:443", "/api/chat", false},
		{"evil.com:443", "/v1/oauth/token", false},
	}

	for _, tt := range tests {
		if got := isOAuthTokenEndpoint(tt.host, tt.path); got != tt.want {
			t.Errorf("isOAuthTokenEndpoint(%q, %q) = %v, want %v", tt.host, tt.path, got, tt.want)
		}
	}
}

func TestShouldBypassMITM(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"auth.openai.com:443", true},
		{"auth.openai.com", true},
		// Model/code traffic must still be inspected.
		{"api.openai.com:443", false},
		{"chatgpt.com:443", false},
		{"api.anthropic.com:443", false},
		{"evil.com:443", false},
	}

	for _, tt := range tests {
		if got := shouldBypassMITM(tt.host); got != tt.want {
			t.Errorf("shouldBypassMITM(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}
}
