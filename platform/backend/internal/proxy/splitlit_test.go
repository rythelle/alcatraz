package proxy

import (
	"strings"
	"testing"
)

// leaks reports whether any piece of want survives in got. A split value is
// only contained if EVERY piece is gone.
func assertPiecesGone(t *testing.T, got string, pieces ...string) {
	t.Helper()
	for _, p := range pieces {
		if strings.Contains(got, p) {
			t.Errorf("piece %q survived redaction in: %s", p, got)
		}
	}
}

// The exact evasion from the report: the agent avoided email redaction by
// never letting a literal email appear in the source.
func TestSplitLiteralJoinBypass(t *testing.T) {
	src := "const user = 'someone';\nconst domain = 'example.com';\nconst email = [user, domain].join('@');"

	got, _, _ := sanitizePipeline(src, nil, nil, false)
	assertPiecesGone(t, got, "someone", "example.com")
}

func TestSplitLiteralForms(t *testing.T) {
	cases := []struct {
		name   string
		src    string
		pieces []string
	}{
		{
			"plus chain of literals",
			`const email = 'someone' + '@' + 'example.com';`,
			[]string{"someone", "example.com"},
		},
		{
			"plus chain through bindings",
			"const u = 'someone';\nconst d = 'example.com';\nconst e = u + '@' + d;",
			[]string{"someone", "example.com"},
		},
		{
			"template literal",
			"const u = 'someone';\nconst d = 'example.com';\nconst e = `${u}@${d}`;",
			[]string{"someone", "example.com"},
		},
		{
			"python join",
			"user = 'someone'\ndomain = 'example.com'\nemail = '@'.join([user, domain])",
			[]string{"someone", "example.com"},
		},
		{
			"concat method",
			`const e = 'someone'.concat('@', 'example.com');`,
			[]string{"someone", "example.com"},
		},
		{
			"array join with default separator",
			`const e = ['someone@example', '.com'].join('');`,
			[]string{"someone@example"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _, _ := sanitizePipeline(tc.src, nil, nil, false)
			assertPiecesGone(t, got, tc.pieces...)
		})
	}
}

// Splitting an API key must not get through either. The cut is inside the
// `sk-ant-api03-` prefix so neither half is detectable alone — the whole point
// of the fold. (A half that still matches on its own is caught by the literal
// patterns before this pass ever runs.)
func TestSplitLiteralAPIKey(t *testing.T) {
	key := "sk-ant-api03-" + strings.Repeat("A", 95)
	head, tail := key[:8], key[8:]

	if HasSensitiveContent(head) || HasSensitiveContent(tail) {
		t.Fatal("test setup: a half is detectable on its own")
	}

	src := "const a = '" + head + "';\nconst b = '" + tail + "';\nconst k = a + b;"
	got, dets, _ := sanitizePipeline(src, nil, nil, false)

	assertPiecesGone(t, got, head, tail)
	if len(dets) == 0 {
		t.Error("expected a detection")
	}
}

// The fold must not invent values: ordinary code that happens to concatenate
// strings has to survive untouched, or every codebase becomes unreadable.
func TestSplitLiteralLeavesOrdinaryCodeAlone(t *testing.T) {
	cases := []string{
		`const url = 'https://' + host + '/v1/users';`,
		`const msg = 'Hello, ' + name + '!';`,
		`const p = ['src', 'components', 'Button.tsx'].join('/');`,
		"const q = `SELECT * FROM ${table} WHERE id = $1`;",
		`const cls = 'btn' + ' ' + variant;`,
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			got, _, _ := sanitizePipeline(src, nil, nil, false)
			if got != src {
				t.Errorf("ordinary code was rewritten\n want: %s\n got:  %s", src, got)
			}
		})
	}
}

// An unresolvable operand means the value isn't actually in the text, so
// nothing should be redacted on a guess.
func TestSplitLiteralIgnoresUnknownOperands(t *testing.T) {
	src := `const e = someUnknownVar + '@' + 'example.com';`
	got, _, _ := sanitizePipeline(src, nil, nil, false)
	if got != src {
		t.Errorf("folded across an unbound identifier\n want: %s\n got:  %s", src, got)
	}
}

// With the vault on, the pieces come back on the response path, so the agent
// gets working code instead of a reason to route around the Guard.
func TestSplitLiteralIsReversible(t *testing.T) {
	v := NewVault(0)
	src := "const user = 'someone';\nconst domain = 'example.com';\nconst e = [user, domain].join('@');"

	got, _, _ := sanitizePipeline(src, nil, v, false)
	assertPiecesGone(t, got, "someone", "example.com")

	if back := v.Detokenize(got); back != src {
		t.Errorf("round-trip failed\n want: %s\n got:  %s", src, back)
	}
}
