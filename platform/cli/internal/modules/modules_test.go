package modules

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnv(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDefaultsWhenNoEnv(t *testing.T) {
	dir := t.TempDir() // no .env at all
	if !Enabled(dir, "checkpoints") {
		t.Error("checkpoints should default on")
	}
	if Enabled(dir, "spawn") {
		t.Error("spawn should default off")
	}
}

func TestEnvFileOverridesDefault(t *testing.T) {
	dir := writeEnv(t, "ALCATRAZ_MOD_SPAWN=on\nALCATRAZ_MOD_STATS=off\n")
	if !Enabled(dir, "spawn") {
		t.Error(".env should turn spawn on")
	}
	if Enabled(dir, "stats") {
		t.Error(".env should turn stats off")
	}
}

func TestProcessEnvWinsOverFile(t *testing.T) {
	dir := writeEnv(t, "ALCATRAZ_MOD_SPAWN=on\n")
	t.Setenv("ALCATRAZ_MOD_SPAWN", "off")
	if Enabled(dir, "spawn") {
		t.Error("process env (off) must beat .env (on)")
	}
}

func TestInlineCommentIgnored(t *testing.T) {
	dir := writeEnv(t, "ALCATRAZ_MOD_MEGABRAIN=on   # opt-in\n")
	if !Enabled(dir, "megabrain") {
		t.Error("inline comment must not defeat the value")
	}
}

func TestEnsureBlockInjectsOnce(t *testing.T) {
	dir := writeEnv(t, "X=1\n")
	injected, err := EnsureBlock(dir)
	if err != nil || !injected {
		t.Fatalf("expected injection, got injected=%v err=%v", injected, err)
	}
	if !HasBlock(dir) {
		t.Error("block should be present after injection")
	}
	// Second call is a no-op.
	injected2, _ := EnsureBlock(dir)
	if injected2 {
		t.Error("second EnsureBlock should not re-inject")
	}
	// Injected safety net is on, opt-in off.
	if !Enabled(dir, "sessions") || Enabled(dir, "shakedown") {
		t.Error("injected defaults wrong")
	}
}

func TestEnsureBlockSkipsMissingEnv(t *testing.T) {
	dir := t.TempDir() // no .env
	injected, err := EnsureBlock(dir)
	if err != nil || injected {
		t.Errorf("no .env should mean no injection, got injected=%v err=%v", injected, err)
	}
}

func TestSetInEnvUpdatesAndAppends(t *testing.T) {
	dir := writeEnv(t, "ALCATRAZ_MOD_SPAWN=off\n")
	if err := SetInEnv(dir, "spawn", true); err != nil {
		t.Fatal(err)
	}
	if !Enabled(dir, "spawn") {
		t.Error("SetInEnv should have turned spawn on")
	}
	// megabrain line doesn't exist yet — should be appended.
	if err := SetInEnv(dir, "megabrain", true); err != nil {
		t.Fatal(err)
	}
	if !Enabled(dir, "megabrain") {
		t.Error("SetInEnv should append and enable megabrain")
	}
}
