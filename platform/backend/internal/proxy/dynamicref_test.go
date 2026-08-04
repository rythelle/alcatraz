package proxy

import (
	"strings"
	"testing"

	"github.com/alcatraz/alcatraz/internal/rules"
)

// A CloudFormation dynamic reference points at a secret store; the value is
// resolved at deploy time and is never in the file. Redacting it corrupted the
// serverless template and pushed the agent into building the string by parts.
const cfnRef = `{{resolve:secretsmanager:notifier-v2-${self:custom.stage}-ses-verification-callback-token:SecretString}}`

func TestDynamicReferenceSurvives(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{"bare", cfnRef},
		{"in assignment", "SES_VERIFICATION_CALLBACK_TOKEN: '" + cfnRef + "'"},
		{"json field", `{"SES_VERIFICATION_CALLBACK_TOKEN":"` + cfnRef + `"}`},
		{"ssm", "DB_PASSWORD: '{{resolve:ssm-secure:/notifier/${self:custom.stage}/db-password:1}}'"},
		{"plain ssm", "{{resolve:ssm:/app/config/token:3}}"},
		{"two refs on one line", cfnRef + " and " + cfnRef},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _, _ := sanitizePipeline(tc.text, nil, nil, false)
			if got != tc.text {
				t.Errorf("dynamic reference was rewritten\n want: %s\n got:  %s", tc.text, got)
			}
		})
	}
}

// The protection must not become a hiding place: a real secret next to a
// reference still gets redacted.
func TestDynamicReferenceDoesNotShieldRealSecrets(t *testing.T) {
	text := "TOKEN: '" + cfnRef + "'\nAWS_SECRET_ACCESS_KEY: 'wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY'"
	got, dets, _ := sanitizePipeline(text, nil, nil, false)

	if !strings.Contains(got, cfnRef) {
		t.Errorf("reference should have survived, got: %s", got)
	}
	if strings.Contains(got, "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY") {
		t.Errorf("real AWS secret leaked through: %s", got)
	}
	if len(dets) == 0 {
		t.Error("expected a detection for the AWS secret")
	}
}

// A `{{resolve:...}}`-lookalike wrapping a real secret value must not be
// treated as a reference — the protection is keyed on the syntax, so verify a
// secret placed just outside it is still caught.
func TestDynamicReferenceProtectionIsBounded(t *testing.T) {
	text := "{{resolve:ssm:/a/b:1}}sk-ant-api03-" + strings.Repeat("A", 95)
	got, _, _ := sanitizePipeline(text, nil, nil, false)
	if strings.Contains(got, "sk-ant-api03-AAAA") {
		t.Errorf("secret adjacent to a reference leaked: %s", got)
	}
}

// The placeholder used to shield a protected value must itself be inert.
// Before the `$` separators it matched env_secret/generic_secret, which
// redacted the placeholder and left step ⑥ with nothing to restore.
func TestAllowlistedValueSurvivesInsideAssignment(t *testing.T) {
	rs := &rules.RuleSet{Allow: []string{"deadbeefcafebabe"}}
	text := "API_TOKEN: 'deadbeefcafebabe'"

	got, _, _ := sanitizePipeline(text, rs, nil, false)
	if got != text {
		t.Errorf("allowlisted value in an assignment was redacted\n want: %s\n got:  %s", text, got)
	}
}
