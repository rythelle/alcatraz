package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected and returns what it printed.
func captureStdout(t *testing.T, fn func() error) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- sb.String()
	}()

	runErr := fn()
	w.Close()
	os.Stdout = orig
	out := <-done
	if runErr != nil {
		t.Fatalf("command failed: %v", runErr)
	}
	return out
}

// --- bridge-request --------------------------------------------------------

func TestBridgeRequestShapes(t *testing.T) {
	dir := t.TempDir()

	t.Run("spawn carries no query", func(t *testing.T) {
		out := filepath.Join(dir, "spawn.json")
		if err := cmdBridgeRequest([]string{
			"--kind", "spawn", "--task", `trace "auth" flow`, "--agent", "claude",
			"--nonce", "a1b2c3", "--out", out}); err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		readJSON(t, out, &got)
		if got["task"] != `trace "auth" flow` || got["agent"] != "claude" || got["nonce"] != "a1b2c3" {
			t.Fatalf("unexpected payload: %v", got)
		}
		if _, ok := got["query"]; ok {
			t.Fatal("spawn request must not carry a query")
		}
		if _, ok := got["kind"]; ok {
			t.Fatal("spawn request stays kind-less so legacy watchers accept it")
		}
	})

	t.Run("search carries no task or agent", func(t *testing.T) {
		out := filepath.Join(dir, "search.json")
		if err := cmdBridgeRequest([]string{
			"--kind", "search", "--query", "rust async traits",
			"--nonce", "ff00aa", "--out", out}); err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		readJSON(t, out, &got)
		if got["kind"] != "search" || got["query"] != "rust async traits" {
			t.Fatalf("unexpected payload: %v", got)
		}
		if _, ok := got["task"]; ok {
			t.Fatal("search request must not carry a task")
		}
		if _, ok := got["agent"]; ok {
			t.Fatal("search request must not carry an agent")
		}
	})
}

func TestBridgeRequestRefusals(t *testing.T) {
	out := filepath.Join(t.TempDir(), "r.json")
	cases := []struct {
		name string
		args []string
	}{
		{"spawn with a query", []string{"--kind", "spawn", "--task", "t", "--agent", "claude", "--query", "q", "--nonce", "aa", "--out", out}},
		{"search with a task", []string{"--kind", "search", "--query", "q", "--task", "t", "--nonce", "aa", "--out", out}},
		{"search with an agent", []string{"--kind", "search", "--query", "q", "--agent", "claude", "--nonce", "aa", "--out", out}},
		{"spawn without an agent", []string{"--kind", "spawn", "--task", "t", "--nonce", "aa", "--out", out}},
		{"search without a query", []string{"--kind", "search", "--nonce", "aa", "--out", out}},
		{"unknown kind", []string{"--kind", "exec", "--nonce", "aa", "--out", out}},
		{"missing nonce", []string{"--kind", "spawn", "--task", "t", "--agent", "claude", "--out", out}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := cmdBridgeRequest(tc.args); err == nil {
				t.Fatal("expected a refusal, got none")
			}
		})
	}
}

// Characters that matter for JSON must survive the round trip unmangled.
func TestBridgeRequestEscaping(t *testing.T) {
	out := filepath.Join(t.TempDir(), "r.json")
	query := `a "b" \c <d> & ação 🧠`
	if err := cmdBridgeRequest([]string{
		"--kind", "search", "--query", query, "--nonce", "aa", "--out", out}); err != nil {
		t.Fatal(err)
	}
	var got searchRequest
	readJSON(t, out, &got)
	if got.Query != query {
		t.Fatalf("query mangled:\n want %q\n got  %q", query, got.Query)
	}
}

// --- hook-session-start ----------------------------------------------------

func TestHookSessionStart(t *testing.T) {
	t.Setenv("MB_CTX", "linha \"um\"\nlinha dois — ação")

	claude := captureStdout(t, func() error {
		return cmdHookSessionStart([]string{"--event-name", "SessionStart"})
	})
	var got hookOutput
	if err := json.Unmarshal([]byte(claude), &got); err != nil {
		t.Fatalf("invalid JSON: %v (%q)", err, claude)
	}
	if got.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Fatal("Claude/Codex need hookEventName echoed back")
	}
	if got.HookSpecificOutput.AdditionalContext != os.Getenv("MB_CTX") {
		t.Fatal("context mangled")
	}

	// Gemini rejects the field entirely, so it must be absent — not empty.
	gemini := captureStdout(t, func() error { return cmdHookSessionStart(nil) })
	if strings.Contains(gemini, "hookEventName") {
		t.Fatalf("Gemini payload must omit hookEventName: %s", gemini)
	}
}

func TestHookSessionStartEmptyContext(t *testing.T) {
	t.Setenv("MB_CTX", "")
	out := captureStdout(t, func() error { return cmdHookSessionStart(nil) })
	if !strings.Contains(out, `"additionalContext":""`) {
		t.Fatalf("empty context must still emit the key: %s", out)
	}
}

// --- precompact ------------------------------------------------------------

func TestPrecompactDigest(t *testing.T) {
	dir := t.TempDir()
	transcript := filepath.Join(dir, "t.jsonl")
	lines := []string{
		`{"message":{"role":"user","content":"primeira   pergunta   com espacos"}}`,
		`{"message":{"role":"assistant","content":[{"type":"text","text":"resposta"},{"type":"tool_use","name":"Bash"},{"type":"text","text":"e mais"}]}}`,
		`{"message":{"role":"system","content":"ignorar"}}`,
		`nao e json`,
		`{"message":{"role":"user","content":""}}`,
		`{"message":{"role":"user","content":"ultima — ação"}}`,
	}
	write(t, transcript, strings.Join(lines, "\n"))

	out := captureStdout(t, func() error {
		return cmdPrecompactDigest([]string{payload(t, transcript, "manual")})
	})

	want := []string{
		"- user: primeira pergunta com espacos", // whitespace collapsed
		"- assistant: resposta e mais",          // only text blocks, joined
		"- user: ultima — ação",
	}
	got := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(got) != len(want) {
		t.Fatalf("want %d digest lines, got %d:\n%s", len(want), len(got), out)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d:\n want %q\n got  %q", i, want[i], got[i])
		}
	}
	if strings.Contains(out, "ignorar") {
		t.Error("non-conversation roles must be skipped")
	}
}

func TestPrecompactDigestKeepsLastTen(t *testing.T) {
	dir := t.TempDir()
	transcript := filepath.Join(dir, "t.jsonl")
	var lines []string
	for i := 0; i < 25; i++ {
		lines = append(lines, `{"message":{"role":"user","content":"turn `+string(rune('a'+i%26))+`"}}`)
	}
	write(t, transcript, strings.Join(lines, "\n"))

	out := captureStdout(t, func() error {
		return cmdPrecompactDigest([]string{payload(t, transcript, "auto")})
	})
	if n := len(strings.Split(strings.TrimRight(out, "\n"), "\n")); n != digestMessages {
		t.Fatalf("want %d lines, got %d", digestMessages, n)
	}
}

// Truncation counts runes, not bytes: a multibyte character must never be cut
// in half (that would put invalid UTF-8 into the vault).
func TestPrecompactDigestTruncatesOnRunes(t *testing.T) {
	dir := t.TempDir()
	transcript := filepath.Join(dir, "t.jsonl")
	long := strings.Repeat("é", 500)
	write(t, transcript, `{"message":{"role":"user","content":"`+long+`"}}`)

	out := captureStdout(t, func() error {
		return cmdPrecompactDigest([]string{payload(t, transcript, "auto")})
	})
	text := strings.TrimPrefix(strings.TrimRight(out, "\n"), "- user: ")
	if n := len([]rune(text)); n != digestRunes {
		t.Fatalf("want %d runes, got %d", digestRunes, n)
	}
	if !strings.HasPrefix(text, "é") || strings.Contains(text, "\uFFFD") {
		t.Fatal("truncation split a multibyte rune")
	}
}

// A broken payload or a missing transcript must cost the digest, never the
// compaction itself.
func TestPrecompactDegradesQuietly(t *testing.T) {
	for _, arg := range []string{`nao e json`, `{"transcript_path":"/nao/existe.jsonl"}`, `{}`} {
		out := captureStdout(t, func() error { return cmdPrecompactDigest([]string{arg}) })
		if strings.TrimSpace(out) != "" {
			t.Errorf("payload %q should yield an empty digest, got %q", arg, out)
		}
	}
}

func TestPrecompactTrigger(t *testing.T) {
	cases := map[string]string{
		`{"trigger":"manual"}`: "manual",
		`{"trigger":""}`:       "auto",
		`{}`:                   "auto",
		`nao e json`:           "auto",
	}
	for in, want := range cases {
		out := captureStdout(t, func() error { return cmdPrecompactTrigger([]string{in}) })
		if strings.TrimSpace(out) != want {
			t.Errorf("payload %q: want %q, got %q", in, want, strings.TrimSpace(out))
		}
	}
}

// --- settings merges -------------------------------------------------------

func TestSettingsClaudePreservesAndWires(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{"model":"opus","projects":{"/outro":{"x":1}},"hooks":{"Notification":[{"hooks":[]}]}}`)

	runSettings(t, func() error {
		return cmdSettingsClaude([]string{path, "/bin/start", "/bin/end", "/bin/pre"})
	})

	var got map[string]any
	readJSON(t, path, &got)
	if got["model"] != "opus" {
		t.Error("unrelated top-level keys must survive")
	}
	projects := got["projects"].(map[string]any)
	if _, ok := projects["/outro"]; !ok {
		t.Error("other projects must survive")
	}
	if _, ok := projects["/workspace"]; !ok {
		t.Error("/workspace must be wired")
	}
	hooks := got["hooks"].(map[string]any)
	if _, ok := hooks["Notification"]; !ok {
		t.Error("unrelated hooks must survive")
	}
	for _, key := range []string{"SessionStart", "SessionEnd", "PreCompact"} {
		if _, ok := hooks[key]; !ok {
			t.Errorf("hook %s not wired", key)
		}
	}
	if cmd := firstCommand(t, hooks["SessionEnd"]); cmd != "/bin/end claude" {
		t.Errorf("SessionEnd must name the agent, got %q", cmd)
	}
}

func TestSettingsGeminiExcludeTools(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{"theme":"dark","excludeTools":["web_fetch"]}`)

	runSettings(t, func() error {
		return cmdSettingsGemini([]string{path, "/bin/start", "/bin/end"})
	})

	var got map[string]any
	readJSON(t, path, &got)
	if got["theme"] != "dark" {
		t.Error("unrelated keys must survive")
	}
	excl := got["excludeTools"].([]any)
	if len(excl) != 2 || excl[0] != "save_memory" || excl[1] != "web_fetch" {
		t.Fatalf("excludeTools must be the sorted union, got %v", excl)
	}
	// Gemini needs the matcher; Claude must not have one.
	group := got["hooks"].(map[string]any)["SessionStart"].([]any)[0].(map[string]any)
	if group["matcher"] != "*" {
		t.Error("Gemini hooks need matcher:*")
	}
}

// Re-running init must converge, not accumulate.
func TestSettingsAreIdempotent(t *testing.T) {
	dir := t.TempDir()
	claude := filepath.Join(dir, "claude.json")
	gemini := filepath.Join(dir, "gemini.json")
	write(t, gemini, `{"excludeTools":["save_memory"]}`)

	for i := 0; i < 3; i++ {
		runSettings(t, func() error {
			return cmdSettingsClaude([]string{claude, "/bin/start", "/bin/end", "/bin/pre"})
		})
		runSettings(t, func() error {
			return cmdSettingsGemini([]string{gemini, "/bin/start", "/bin/end"})
		})
	}

	var got map[string]any
	readJSON(t, gemini, &got)
	if excl := got["excludeTools"].([]any); len(excl) != 1 {
		t.Fatalf("excludeTools grew across runs: %v", excl)
	}
	first := read(t, claude)
	runSettings(t, func() error {
		return cmdSettingsClaude([]string{claude, "/bin/start", "/bin/end", "/bin/pre"})
	})
	if read(t, claude) != first {
		t.Fatal("claude settings.json is not byte-stable across runs")
	}
}

// A corrupt settings file costs the hooks, never the container's boot.
func TestSettingsRecoverFromCorruptFile(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, body string }{
		{"garbage", "lixo{{{"},
		{"null", "null"},
		{"array", "[1,2,3]"},
		{"empty", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".json")
			write(t, path, tc.body)
			runSettings(t, func() error {
				return cmdSettingsClaude([]string{path, "/bin/start", "/bin/end", "/bin/pre"})
			})
			var got map[string]any
			readJSON(t, path, &got)
			if _, ok := got["hooks"].(map[string]any)["SessionStart"]; !ok {
				t.Fatal("hooks not wired after recovering the file")
			}
		})
	}
}

// hooks present but not an object (a hand-edited file) must not panic.
func TestSettingsSurviveWrongTypes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	write(t, path, `{"hooks":"oops","projects":42,"excludeTools":"nope"}`)
	runSettings(t, func() error {
		return cmdSettingsGemini([]string{path, "/bin/start", "/bin/end"})
	})
	var got map[string]any
	readJSON(t, path, &got)
	if excl := got["excludeTools"].([]any); len(excl) != 1 || excl[0] != "save_memory" {
		t.Fatalf("excludeTools not repaired: %v", got["excludeTools"])
	}
}

// --- helpers ---------------------------------------------------------------

func runSettings(t *testing.T, fn func() error) {
	t.Helper()
	captureStdout(t, fn) // the commands log a line we don't care about here
}

func payload(t *testing.T, transcript, trigger string) string {
	t.Helper()
	b, err := json.Marshal(map[string]string{"transcript_path": transcript, "trigger": trigger})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("invalid JSON in %s: %v\n%s", path, err, b)
	}
}

func firstCommand(t *testing.T, v any) string {
	t.Helper()
	group := v.([]any)[0].(map[string]any)
	return group["hooks"].([]any)[0].(map[string]any)["command"].(string)
}
