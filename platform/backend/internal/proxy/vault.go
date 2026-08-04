package proxy

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
	"regexp"
	"sync"
)

// Vault implements reversible tokenization. Instead of destroying a sensitive
// value with a static [REDACTED] marker, the Guard can swap it for an opaque,
// fixed-length token and remember the mapping HERE, inside the container. The
// model upstream only ever sees the token; when the model echoes that token
// back in its response, the Guard restores the real value before it reaches the
// AI CLI. The mapping never leaves the process and is never persisted — it is
// purely in-memory and dies with the container.
//
// A bounded map prevents a flood of unique secrets from exhausting memory: once
// full, Tokenize refuses new values so the caller falls back to destructive
// redaction — the Guard never leaks a value just because the vault is full.
type Vault struct {
	mu      sync.Mutex
	toValue map[string]string // token -> original value
	toToken map[string]string // original value -> token (stable dedup)
	max     int
}

const (
	vaultTokenPrefix = "[[ALCZ-"
	vaultTokenSuffix = "]]"
	// vaultTokenLen is fixed: prefix(7) + 16 hex + suffix(2) = 25.
	vaultTokenLen = len(vaultTokenPrefix) + 16 + len(vaultTokenSuffix)
	// vaultKeep is how many trailing bytes the streaming detokenizer holds back
	// so a token split across reads is still matched (any incomplete token is
	// shorter than a full one).
	vaultKeep = vaultTokenLen - 1
)

// vaultTokenRe matches a complete vault token.
var vaultTokenRe = regexp.MustCompile(`\[\[ALCZ-[0-9a-f]{16}\]\]`)

// DefaultVaultMax bounds distinct tokenized values per process.
const DefaultVaultMax = 50000

// NewVault returns an empty vault. max<=0 uses DefaultVaultMax.
func NewVault(max int) *Vault {
	if max <= 0 {
		max = DefaultVaultMax
	}
	return &Vault{
		toValue: make(map[string]string),
		toToken: make(map[string]string),
		max:     max,
	}
}

// Tokenize returns a stable token for value and records the mapping. ok is
// false only when the vault is full and value is new, signalling the caller to
// fall back to destructive redaction rather than leak the value.
func (v *Vault) Tokenize(value string) (token string, ok bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if t, seen := v.toToken[value]; seen {
		return t, true
	}
	if len(v.toValue) >= v.max {
		return "", false
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", false
	}
	t := vaultTokenPrefix + hex.EncodeToString(b[:]) + vaultTokenSuffix
	if _, clash := v.toValue[t]; clash {
		return "", false // astronomically rare; fall back to a marker
	}
	v.toValue[t] = value
	v.toToken[value] = t
	return t, true
}

// maxDetokenizeRounds bounds the nested-restore loop below.
const maxDetokenizeRounds = 8

// Detokenize replaces every known token in text with its original value.
// Unknown tokens (e.g. from a previous process) are left untouched.
//
// Redactions nest: patterns are applied in sequence, so a later one can match
// a region that already contains an earlier token and tokenize the lot. One
// substitution pass would then hand back a half-restored value with an inner
// token still in it, so this repeats until the text settles — bounded, so a
// self-referential mapping cannot spin.
func (v *Vault) Detokenize(text string) string {
	if v == nil || !vaultTokenRe.MatchString(text) {
		return text
	}
	v.mu.Lock()
	defer v.mu.Unlock()

	for i := 0; i < maxDetokenizeRounds; i++ {
		out := vaultTokenRe.ReplaceAllStringFunc(text, func(tok string) string {
			if orig, ok := v.toValue[tok]; ok {
				return orig
			}
			return tok
		})
		if out == text {
			break
		}
		text = out
	}
	return text
}

// Len reports how many distinct values are stored (for diagnostics/tests).
func (v *Vault) Len() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.toValue)
}

// tokenOrMarker returns a vault token for original when v is non-nil and has
// room, else the static marker (destructive redaction). This is the single
// decision point that makes redaction reversible.
func tokenOrMarker(v *Vault, original, marker string) string {
	if v != nil {
		if tok, ok := v.Tokenize(original); ok {
			return tok
		}
	}
	return marker
}

// NewDetokReader wraps src so vault tokens are replaced with their original
// values as bytes stream through. It holds back a small tail each read so a
// token straddling a read boundary is still matched. Output length may differ
// from input, so callers must not rely on a fixed Content-Length (use it only
// for streaming/chunked responses; buffer-and-replace for fixed-length ones).
func (v *Vault) NewDetokReader(src io.ReadCloser) io.ReadCloser {
	return &detokReader{src: src, vault: v}
}

type detokReader struct {
	src   io.ReadCloser
	vault *Vault
	carry []byte
	out   bytes.Buffer
	eof   bool
}

func (d *detokReader) Read(p []byte) (int, error) {
	for d.out.Len() == 0 && !d.eof {
		buf := make([]byte, 32*1024)
		n, err := d.src.Read(buf)
		if n > 0 {
			d.carry = append(d.carry, buf[:n]...)
			det := []byte(d.vault.Detokenize(string(d.carry)))
			if len(det) > vaultKeep {
				d.out.Write(det[:len(det)-vaultKeep])
				d.carry = append(d.carry[:0], det[len(det)-vaultKeep:]...)
			} else {
				d.carry = append(d.carry[:0], det...)
			}
		}
		if err == io.EOF {
			d.eof = true
			d.out.Write([]byte(d.vault.Detokenize(string(d.carry))))
			d.carry = nil
		} else if err != nil {
			if d.out.Len() == 0 {
				return 0, err
			}
			break
		}
	}
	if d.out.Len() > 0 {
		return d.out.Read(p)
	}
	if d.eof {
		return 0, io.EOF
	}
	return 0, nil
}

func (d *detokReader) Close() error { return d.src.Close() }
