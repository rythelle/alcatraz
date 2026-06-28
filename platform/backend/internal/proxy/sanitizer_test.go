package proxy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeText_OpenAIKey(t *testing.T) {
	input := `{"prompt": "use key sk-` + `abcdefghijklmnopqrstuvwxyz0123456789ABCDEF0123456789ab"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if strings.Contains(result.Output, "sk-abc") {
		t.Fatal("OpenAI key not redacted")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_OPENAI_KEY]") {
		t.Fatal("wrong replacement")
	}
}

func TestSanitizeText_AnthropicKey(t *testing.T) {
	input := `sk-` + `ant-abcdefghijklmnopqrstuvwxyz0123456789AB`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_ANTHROPIC_KEY]") {
		t.Fatal("Anthropic key not redacted")
	}
}

func TestSanitizeText_GoogleKey(t *testing.T) {
	input := `{"key": "AIza` + `SyA1234567890abcdefghijklmnopqrstuv"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_GOOGLE_KEY]") {
		t.Fatal("Google key not redacted")
	}
}

func TestSanitizeText_GitHubToken(t *testing.T) {
	input := `ghp` + `_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghij`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_GITHUB_TOKEN]") {
		t.Fatal("GitHub token not redacted")
	}
}

func TestSanitizeText_AWSAccessKey(t *testing.T) {
	input := `{"key": "AKIA` + `IOSFODNN7EXAMPLE"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_AWS_KEY]") {
		t.Fatal("AWS key not redacted")
	}
}

func TestSanitizeText_JWT(t *testing.T) {
	input := `eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123def456ghi789`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_JWT]") {
		t.Fatal("JWT not redacted")
	}
}

func TestSanitizeText_StripeKey(t *testing.T) {
	input := `{"key": "sk_` + `live` + `_abcdefghijklmnopqrstuvwxyz"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_STRIPE_KEY]") {
		t.Fatal("Stripe key not redacted")
	}
}

func TestSanitizeText_CPF(t *testing.T) {
	input := `{"cpf": "123.456.789-09"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_CPF]") {
		t.Fatal("CPF not redacted")
	}
}

func TestSanitizeText_CNPJ(t *testing.T) {
	input := `{"cnpj": "12.345.678/0001-90"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_CNPJ]") {
		t.Fatal("CNPJ not redacted")
	}
}

func TestSanitizeText_EmailContext(t *testing.T) {
	input := `{"email": "user@example.com"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_EMAIL]") {
		t.Fatal("Email not redacted")
	}
}

func TestSanitizeText_EmailStrict(t *testing.T) {
	input := `Contact: user@example.com for info`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_EMAIL]") {
		t.Fatal("Email not redacted")
	}
}

func TestSanitizeText_CreditCard(t *testing.T) {
	input := `{"cartao": "4111 1111 1111 1111"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_CARTAO]") {
		t.Fatal("Credit card not redacted")
	}
}

func TestSanitizeText_IPAddress(t *testing.T) {
	input := `{"ip": "192.168.1.100"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_IP]") {
		t.Fatal("IP not redacted")
	}
}

func TestSanitizeText_SSHPrivateKey(t *testing.T) {
	input := `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA0Z3VS5JJcds3xfn/ygWyF8PbnGy0AHB7MhgHcTz6sE2I2yPB
aFDrBz9vFqU4yBBmYXBYnqM8nJk7VqKJqH6T
-----END RSA PRIVATE KEY-----`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_PRIVATE_KEY]") {
		t.Fatal("SSH key not redacted")
	}
}

func TestSanitizeText_EnvSecret(t *testing.T) {
	input := "DATABASE_URL=postgresql://user:pass@host:5432/db\n"
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_ENV_SECRET]") {
		t.Fatal("ENV secret not redacted")
	}
}

func TestSanitizeText_GenericSecret(t *testing.T) {
	input := `{"password": "mysecretpassword123"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_GENERIC_SECRET]") {
		t.Fatal("Generic secret not redacted")
	}
}

func TestSanitizeText_DigitalOceanToken(t *testing.T) {
	input := `dop_v1_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_DO_TOKEN]") {
		t.Fatal("DO token not redacted")
	}
}

func TestSanitizeJSON_PreservesStructure(t *testing.T) {
	input := `{"messages": [{"role": "user", "content": "hello sk-` + `abcdefghijklmnopqrstuvwxyz0123456789ABCDEF0123456789ab"}], "model": "gpt-4"}`
	result := SanitizeJSON(input, false)

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result.Output), &parsed); err != nil {
		t.Fatalf("Invalid JSON after sanitization: %v", err)
	}

	if parsed["model"] != "gpt-4" {
		t.Fatal("model field corrupted")
	}

	messages := parsed["messages"].([]interface{})
	msg := messages[0].(map[string]interface{})
	if msg["role"] != "user" {
		t.Fatal("role field corrupted")
	}
	if !strings.Contains(msg["content"].(string), "[REDACTED_BY_ALCATRAZ_OPENAI_KEY]") {
		t.Fatal("API key not redacted in nested content")
	}
}

func TestSanitizeJSON_NoFalsePositives(t *testing.T) {
	input := `{"prompt": "This is a normal message with no secrets", "model": "gpt-4"}`
	result := SanitizeJSON(input, false)
	if result.Modified {
		t.Fatal("should not modify clean input")
	}
	if result.Output != input {
		t.Fatal("output should match input")
	}
}

func TestSanitizeJSON_DryRun(t *testing.T) {
	input := `{"key": "sk-` + `abcdefghijklmnopqrstuvwxyz0123456789ABCDEF0123456789ab"}`
	result := SanitizeJSON(input, true)

	if !result.Modified {
		t.Fatal("should detect but not modify in dry run")
	}
	if strings.Contains(result.Output, "[REDACTED") {
		t.Fatal("should not replace in dry run")
	}
	if len(result.Detections) == 0 {
		t.Fatal("should have detections in dry run")
	}
}

func TestSanitizeJSON_Array(t *testing.T) {
	input := `["sk-` + `abcdefghijklmnopqrstuvwxyz0123456789ABCDEF0123456789ab", "safe value"]`
	result := SanitizeJSON(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_OPENAI_KEY]") {
		t.Fatal("API key in array not redacted")
	}
}

func TestSanitizeJSON_NestedObject(t *testing.T) {
	input := `{"outer": {"inner": {"key": "sk-` + `abcdefghijklmnopqrstuvwxyz0123456789ABCDEF0123456789ab"}}}`
	result := SanitizeJSON(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_OPENAI_KEY]") {
		t.Fatal("Nested API key not redacted")
	}
}

func TestSanitizeJSON_MultipleDetections(t *testing.T) {
	input := `{"openai": "sk-` + `abcdefghijklmnopqrstuvwxyz0123456789ABCDEF0123456789ab", "anthropic": "sk-` + `ant-abcdefghijklmnopqrstuvwxyz012345"}`
	result := SanitizeJSON(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if len(result.Detections) < 2 {
		t.Fatalf("expected at least 2 detections, got %d", len(result.Detections))
	}
}

func TestSanitizeJSON_InvalidJSON(t *testing.T) {
	input := `{not valid json}`
	result := SanitizeJSON(input, false)
	if result.Output != input {
		t.Fatal("should return original input for invalid JSON")
	}
}

func TestSanitizeJSON_IntValues(t *testing.T) {
	input := `{"count": 42, "name": "test"}`
	result := SanitizeJSON(input, false)
	if result.Modified {
		t.Fatal("should not modify safe JSON")
	}
}

func TestSanitizeJSON_BoolValues(t *testing.T) {
	input := `{"active": true, "debug": false}`
	result := SanitizeJSON(input, false)
	if result.Modified {
		t.Fatal("should not modify safe JSON")
	}
}

func TestSanitizeJSON_NullValues(t *testing.T) {
	input := `{"data": null, "key": "sk-` + `abcdefghijklmnopqrstuvwxyz0123456789ABCDEF0123456789ab"}`
	result := SanitizeJSON(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "null") {
		t.Fatal("null value should be preserved")
	}
}

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		host     string
		expected string
	}{
		{"api.openai.com", "openai"},
		{"api.anthropic.com", "anthropic"},
		{"generativelanguage.googleapis.com", "google"},
		{"api.opencode.ai", "opencode"},
		{"api.mistral.ai", "mistral"},
		{"unknown.example.com", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := DetectProvider(tt.host)
			if got != tt.expected {
				t.Errorf("DetectProvider(%q) = %q, want %q", tt.host, got, tt.expected)
			}
		})
	}
}

func TestSanitizeText_BearerToken(t *testing.T) {
	input := `Authorization: bearer abcdefghijklmnopqrstuvwxyz0123456789`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_BEARER]") {
		t.Fatal("Bearer token not redacted")
	}
}

func TestSanitizeText_AWSSecretKey(t *testing.T) {
	input := `{"aws_secret_access_key": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_AWS_SECRET]") {
		t.Fatal("AWS secret key not redacted")
	}
}

func TestSanitizeText_EmailCredential(t *testing.T) {
	input := `{"smtp_pass": "myemailpassword123"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_EMAIL_CRED]") {
		t.Fatal("Email credential not redacted")
	}
}

func TestSanitizeText_PGPPrivateKey(t *testing.T) {
	input := `-----BEGIN PGP PRIVATE KEY BLOCK-----
some private key data here that is long enough to match
-----END PGP PRIVATE KEY BLOCK-----`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_PRIVATE_KEY]") {
		t.Fatal("PGP private key not redacted")
	}
}

func TestSanitizeText_BRPhone(t *testing.T) {
	input := `{"telefone": "(11) 99999-1234"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_TELEFONE]") {
		t.Fatal("Phone not redacted")
	}
}

func TestSanitizeText_PixKey(t *testing.T) {
	input := `{"chave_pix": "usuario12345678"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_PIX]") {
		t.Fatal("PIX key not redacted")
	}
}

func TestSanitizeText_AzureSubscription(t *testing.T) {
	input := `{"subscription_id": "12345678-1234-1234-1234-123456789012"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_AZURE_SUB]") {
		t.Fatal("Azure subscription not redacted")
	}
}

func TestSanitizeText_TerraformToken(t *testing.T) {
	input := `tfrc_ABCDEFGHIJKLMN.abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZab`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_TF_TOKEN]") {
		t.Fatal("Terraform token not redacted")
	}
}

func TestSanitizeText_AWSARN(t *testing.T) {
	input := `{"arn": "arn:aws:s3:us-east-1:123456789012:my-bucket/path"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_AWS_ARN]") {
		t.Fatal("AWS ARN not redacted")
	}
}

func TestSanitizeText_K8sSecret(t *testing.T) {
	input := `{"kubeconfig": "abcdefghijklmnopqrstuvwxyz0123456789"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_K8S]") {
		t.Fatal("K8s secret not redacted")
	}
}

func TestSanitizeText_Passport(t *testing.T) {
	input := `{"passaporte": "AB1234567"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_PASSAPORTE]") {
		t.Fatal("Passport not redacted")
	}
}

func TestSanitizeText_BankAccount(t *testing.T) {
	input := `{"conta": "12345678901234"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_CONTA_BANCARIA]") {
		t.Fatal("Bank account not redacted")
	}
}

func TestSanitizeText_BRId(t *testing.T) {
	input := `{"rg": "12.345.678-9"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_RG]") {
		t.Fatal("RG not redacted")
	}
}

func TestSanitizeText_SlackToken(t *testing.T) {
	input := `xox` + `b-123456789012-1234567890123-abcdefghijklmnopqrstuvwx`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_SLACK_TOKEN]") {
		t.Fatal("Slack token not redacted")
	}
}

func TestSanitizeText_DiscordToken(t *testing.T) {
	input := `ABCDEFGHIJKLMNOPQRSTUVWX.ABCDEF.abcdefghijklmnopqrstuvwxyz0`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_DISCORD_TOKEN]") {
		t.Fatal("Discord token not redacted")
	}
}

func TestSanitizeText_OpenAIProjectKey(t *testing.T) {
	input := `{"key": "sk-proj-abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ0123"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_OPENAI_PROJ]") {
		t.Fatal("OpenAI project key not redacted")
	}
}

func TestSanitizeText_StripePublishable(t *testing.T) {
	input := `{"key": "pk_` + `test` + `_abcdefghijklmnopqrstuvwxyz"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_STRIPE_KEY]") {
		t.Fatal("Stripe publishable key not redacted")
	}
}

func TestSanitizeText_AzureClientSecret(t *testing.T) {
	input := `{"client_secret": "abcdefghijklmnopqrstuvwxyz0123456789"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_AZURE_SECRET]") {
		t.Fatal("Azure client secret not redacted")
	}
}

func TestSanitizeText_GCPServiceAccount(t *testing.T) {
	input := `{"private_key_id": "abcdefghijklmnopqrstuvwxyz0123456789"}`
	result := SanitizeText(input, false)
	if !result.Modified {
		t.Fatal("expected modification")
	}
	if !strings.Contains(result.Output, "[REDACTED_BY_ALCATRAZ_GCP]") {
		t.Fatal("GCP service account not redacted")
	}
}

func TestHasSensitiveContent(t *testing.T) {
	if !HasSensitiveContent("sk-" + "abcdefghijklmnopqrstuvwxyz0123456789ABCDEF0123456789ab") {
		t.Fatal("should detect sensitive content")
	}
	if HasSensitiveContent("hello world, this is safe") {
		t.Fatal("should not detect false positive")
	}
}

func TestCountMatches(t *testing.T) {
	input := `{"key1": "sk-` + `abcdefghijklmnopqrstuvwxyz0123456789ABCDEF0123456789ab", "key2": "sk-` + `ant-abcdefghijklmnopqrstuvwxyz012345"}`
	counts := CountMatches(input)
	if counts["openai_key"] != 1 {
		t.Errorf("expected 1 openai_key match, got %d", counts["openai_key"])
	}
	if counts["anthropic_key"] != 1 {
		t.Errorf("expected 1 anthropic_key match, got %d", counts["anthropic_key"])
	}
}
