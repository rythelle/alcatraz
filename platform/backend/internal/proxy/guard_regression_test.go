package proxy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alcatraz/alcatraz/internal/rules"
)

// Regressions for the two Guard failures reported from a real session. Both
// are written from the symptom, not the implementation, so they keep holding
// if the internals move.

// ── 1. Dynamic references were being redacted ───────────────────────────────
//
// Symptom: writing a serverless function file, the Guard mangled
// `{{resolve:secretsmanager:...:SecretString}}` into a [REDACTED] marker. The
// file reached disk corrupted, and the agent started assembling the string
// from parts to get around it.

// serverlessFile is the shape of the file that triggered the report.
const serverlessFile = `import defaultIamRoleStatements from '../provider/iamRoleStatements';

const resources: AWS['functions'] = {
  lambda: {
    handler: 'src/handler.main',
    environment: {
      SES_VERIFICATION_CALLBACK_TOKEN:
        '{{resolve:secretsmanager:notifier-v2-${self:custom.stage}-ses-verification-callback-token:SecretString}}',
      DB_PASSWORD: '{{resolve:ssm-secure:/notifier/${self:custom.stage}/db-password:1}}',
    },
    events: [{ httpApi: { path: '/verify', method: 'get' } }],
  },
};

export default resources;
`

func TestServerlessFileReachesTheModelIntact(t *testing.T) {
	got, _, _ := sanitizePipeline(serverlessFile, nil, nil, false)
	if got != serverlessFile {
		t.Errorf("the serverless file was rewritten.\n--- want ---\n%s\n--- got ---\n%s", serverlessFile, got)
	}
}

// The same file inside a real request body, which is how it actually travels.
func TestServerlessFileIntactThroughJSONBody(t *testing.T) {
	body, err := json.Marshal(map[string]any{
		"model": "claude-opus-5",
		"messages": []any{
			map[string]any{"role": "user", "content": "Here is the file:\n" + serverlessFile},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	res := SanitizeJSONWithVault(string(body), nil, NewVault(0), false)
	if !strings.Contains(res.Output, "{{resolve:secretsmanager:") {
		t.Errorf("dynamic reference lost from the request body: %s", res.Output)
	}
	if strings.Contains(res.Output, "REDACTED") {
		t.Errorf("something in the file was redacted: %s", res.Output)
	}
}

// Guard rails on the fix: protecting references must not turn them into a
// smuggling channel for real secrets.
func TestDynamicRefProtectionIsNotASmugglingChannel(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		secret string
	}{
		{
			"secret after a reference",
			"a: '{{resolve:ssm:/x/y:1}}'\nAWS_SECRET_ACCESS_KEY: 'wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY'",
			"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		},
		{
			"secret before a reference",
			"key=AKIAIOSFODNN7EXAMPLE\nb: '{{resolve:ssm:/x/y:1}}'",
			"AKIAIOSFODNN7EXAMPLE",
		},
		{
			"unterminated reference does not swallow the rest",
			"{{resolve:ssm:/x/y AKIAIOSFODNN7EXAMPLE",
			"AKIAIOSFODNN7EXAMPLE",
		},
		{
			"reference-looking text is not a licence to leak",
			"{{resolve:}}AKIAIOSFODNN7EXAMPLE",
			"AKIAIOSFODNN7EXAMPLE",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _, _ := sanitizePipeline(tc.text, nil, nil, false)
			if strings.Contains(got, tc.secret) {
				t.Errorf("secret %q survived: %s", tc.secret, got)
			}
		})
	}
}

// ── 2. Redaction was avoidable by never writing the literal ─────────────────
//
// Symptom, quoting the session: "já resolvi contornando: monto o email por
// concatenação ([user, domain].join('@')), então nenhum literal de email
// aparece no fonte pra ser mascarado."

func TestReportedConcatenationWorkaroundIsCaught(t *testing.T) {
	// The workaround as the agent actually wrote it.
	src := `const user = 'rythelle20';
const domain = 'gmail.com';
const email = [user, domain].join('@');`

	got, dets, _ := sanitizePipeline(src, nil, nil, false)

	for _, piece := range []string{"rythelle20", "gmail.com"} {
		if strings.Contains(got, piece) {
			t.Errorf("piece %q survived, the address is still reconstructible:\n%s", piece, got)
		}
	}
	if !hasDetection(dets, "evasion_split") {
		t.Errorf("expected an evasion_split detection, got %v", dets)
	}
}

// The detection has to be reported, not silently applied — the audit log is
// how anyone finds out an evasion happened.
func TestSplitEvasionIsAudited(t *testing.T) {
	src := "const a = 'someone';\nconst b = 'example.com';\nconst e = a + '@' + b;"
	_, dets, modified := sanitizePipeline(src, nil, nil, false)

	if !modified {
		t.Fatal("pipeline reported no modification")
	}
	if !hasDetection(dets, "evasion_split") {
		t.Errorf("evasion not recorded in detections: %v", dets)
	}
}

func hasDetection(dets []Detection, name string) bool {
	for _, d := range dets {
		if d.Pattern == name && d.Count > 0 {
			return true
		}
	}
	return false
}

// ── 3. Allowlist round-trip ─────────────────────────────────────────────────
//
// Found while fixing the above: the placeholder that shields a protected value
// was itself matching the assignment patterns, so an allowlisted value in a
// `TOKEN: ...` line was redacted and then had nothing left to restore.

func TestAllowlistSurvivesEverySensitiveKeyword(t *testing.T) {
	const allowed = "build-agent-shared-token-v4"
	rs := &rules.RuleSet{Allow: []string{allowed}}

	for _, key := range []string{
		"API_TOKEN", "SECRET", "PASSWORD", "DATABASE_URL",
		"MY_API_KEY", "CONNECTION_STRING", "token", "password",
	} {
		t.Run(key, func(t *testing.T) {
			text := key + ": '" + allowed + "'"
			got, _, _ := sanitizePipeline(text, rs, nil, false)
			if got != text {
				t.Errorf("allowlisted value did not survive\n want: %s\n got:  %s", text, got)
			}
		})
	}
}

// A value that is merely adjacent to an allowlisted one is still redacted.
func TestAllowlistDoesNotCoverNeighbours(t *testing.T) {
	rs := &rules.RuleSet{Allow: []string{"safe-public-value"}}
	text := "PUBLIC: 'safe-public-value'\nAWS_KEY: 'AKIAIOSFODNN7EXAMPLE'"

	got, _, _ := sanitizePipeline(text, rs, nil, false)
	if !strings.Contains(got, "safe-public-value") {
		t.Errorf("allowlisted value was redacted: %s", got)
	}
	if strings.Contains(got, "AKIAIOSFODNN7EXAMPLE") {
		t.Errorf("neighbouring AWS key leaked: %s", got)
	}
}

// ── 4. The whole point: reversibility ───────────────────────────────────────
//
// Redaction that breaks the workflow is what pushed the agent to evade in the
// first place. Everything the new passes redact must come back.

func TestNewRedactionsAreReversible(t *testing.T) {
	cases := map[string]string{
		"split via join":     "const u = 'someone';\nconst d = 'example.com';\nconst e = [u, d].join('@');",
		"split via plus":     "const e = 'someone' + '@' + 'example.com';",
		"split via template": "const u = 'someone';\nconst d = 'example.com';\nconst e = `${u}@${d}`;",
		"ordinary secret":    "AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"serverless file":    serverlessFile,
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			v := NewVault(0)
			got, _, _ := sanitizePipeline(src, nil, v, false)
			if back := v.Detokenize(got); back != src {
				t.Errorf("value did not round-trip\n want: %s\n got:  %s", src, back)
			}
		})
	}
}
