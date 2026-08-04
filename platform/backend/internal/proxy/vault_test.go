package proxy

import (
	"io"
	"strings"
	"testing"
)

// dribble is an io.ReadCloser that yields one byte per Read, exercising the
// worst case for the streaming detokenizer (a token split at every boundary).
type dribble struct {
	data []byte
	i    int
}

func (d *dribble) Read(p []byte) (int, error) {
	if d.i >= len(d.data) {
		return 0, io.EOF
	}
	p[0] = d.data[d.i]
	d.i++
	return 1, nil
}
func (d *dribble) Close() error { return nil }

func TestVault_RoundTripThroughPipeline(t *testing.T) {
	v := NewVault(0)
	body := `{"x":"` + validCNPJ + `"}`

	res := SanitizeJSONWithVault(body, nil, v, false)
	if !res.Modified {
		t.Fatal("expected the CNPJ to be tokenized")
	}
	if strings.Contains(res.Output, validCNPJ) {
		t.Fatalf("raw CNPJ leaked upstream: %q", res.Output)
	}
	if strings.Contains(res.Output, "REDACTED") {
		t.Fatalf("expected a vault token, got a destructive marker: %q", res.Output)
	}
	if !vaultTokenRe.MatchString(res.Output) {
		t.Fatalf("no vault token in output: %q", res.Output)
	}

	// On the way back, the model echoes the token → real value is restored.
	restored := v.Detokenize(res.Output)
	if !strings.Contains(restored, validCNPJ) {
		t.Fatalf("detokenize did not restore the CNPJ: %q", restored)
	}
}

func TestVault_StableTokenPerValue(t *testing.T) {
	v := NewVault(0)
	t1, ok1 := v.Tokenize("secret-value")
	t2, ok2 := v.Tokenize("secret-value")
	if !ok1 || !ok2 || t1 != t2 {
		t.Fatalf("same value must map to the same token: %q vs %q", t1, t2)
	}
	if v.Len() != 1 {
		t.Fatalf("expected 1 stored value, got %d", v.Len())
	}
}

func TestVault_FullFallsBackToMarker(t *testing.T) {
	v := NewVault(1)
	if _, ok := v.Tokenize("first"); !ok {
		t.Fatal("first value should tokenize")
	}
	// Vault is full; a new value must fall back to the destructive marker so it
	// is never leaked in cleartext.
	out := tokenOrMarker(v, "second", "[REDACTED_BY_ALCATRAZ_X]")
	if out != "[REDACTED_BY_ALCATRAZ_X]" {
		t.Fatalf("expected marker fallback when full, got %q", out)
	}
}

func TestVault_DetokReaderSplitToken(t *testing.T) {
	v := NewVault(0)
	tok, _ := v.Tokenize(validCNPJ)
	payload := "answer: " + tok + " end"

	r := v.NewDetokReader(&dribble{data: []byte(payload)})
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(out) != "answer: "+validCNPJ+" end" {
		t.Fatalf("streaming detok failed: %q", string(out))
	}
}

func TestVault_DetokReaderUnknownTokenUntouched(t *testing.T) {
	v := NewVault(0)
	payload := "prefix [[ALCZ-deadbeefdeadbeef]] suffix"
	r := v.NewDetokReader(&dribble{data: []byte(payload)})
	out, _ := io.ReadAll(r)
	if string(out) != payload {
		t.Fatalf("unknown token must pass through: %q", string(out))
	}
}

func TestVault_AntiEvasionTokenized(t *testing.T) {
	v := NewVault(0)
	// base64("11222333000181")
	res := SanitizeJSONWithVault(`{"x":"MTEyMjIzMzMwMDAxODE="}`, nil, v, false)
	if !vaultTokenRe.MatchString(res.Output) {
		t.Fatalf("base64 payload should tokenize, got %q", res.Output)
	}
	restored := v.Detokenize(res.Output)
	if !strings.Contains(restored, "MTEyMjIzMzMwMDAxODE=") {
		t.Fatalf("detok should restore the base64 blob: %q", restored)
	}
}
