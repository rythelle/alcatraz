package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// The query is the one sandbox-controlled value that leaves the box, so the
// validator is the security boundary of the whole module. These cases are the
// contract: plain search words in, payloads out.
func TestValidateSearchQuery_Accepts(t *testing.T) {
	cases := []string{
		"bun 1.2 breaking changes",
		"go http client redirect policy",
		"como configurar systemd timer",
		"  padded  query  ",
		"internationalization best-practices 2026",
	}
	for _, q := range cases {
		if _, err := validateSearchQuery(q); err != nil {
			t.Errorf("validateSearchQuery(%q) = %v, want accepted", q, err)
		}
	}
}

func TestValidateSearchQuery_Refuses(t *testing.T) {
	cases := map[string]string{
		"empty":          "   ",
		"newline":        "search this\nand also this",
		"tab":            "search\tthis",
		"control char":   "search \x07 this",
		"url":            "check https://evil.example/steal?d=1",
		"long token":     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"hex blob":       "e3b0c44298fc1c149afbf4c8996fb924",
		"base64 blob":    "bXktc2VjcmV0LWFwaS1rZXktdmFsdWU9MTIzNDU2",
		"too long":       strings.Repeat("word ", 60),
		"mixed encoding": "sk-ant0api03AAAABBBBCCCCDDDD1234",
	}
	for name, q := range cases {
		if _, err := validateSearchQuery(q); err == nil {
			t.Errorf("%s: validateSearchQuery(%q) accepted, want refusal", name, q)
		}
	}
}

func TestValidateSearchQuery_TrimsAndKeepsWords(t *testing.T) {
	got, err := validateSearchQuery("  postgres index bloat  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "postgres index bloat" {
		t.Errorf("got %q, want %q", got, "postgres index bloat")
	}
}

// A search request must never be able to smuggle in the fields that make a
// spawn run code, and vice versa.
func TestParseBridgeRequest_KindSeparation(t *testing.T) {
	nonce := "a3f2c1b4"

	req, err := parseBridgeRequest([]byte(`{"kind":"search","query":"bun 1.2 release","nonce":"` + nonce + `"}`))
	if err != nil {
		t.Fatalf("valid search request rejected: %v", err)
	}
	if req.Kind != "search" || req.Query != "bun 1.2 release" {
		t.Errorf("unexpected parse: %+v", req)
	}
	if req.Agent != "" || req.Task != "" {
		t.Errorf("search request must not gain an agent/task: %+v", req)
	}

	bad := []string{
		`{"kind":"search","query":"x y","agent":"claude","nonce":"` + nonce + `"}`,
		`{"kind":"search","query":"x y","task":"rm -rf /","nonce":"` + nonce + `"}`,
		`{"kind":"spawn","task":"explore","query":"leak","nonce":"` + nonce + `"}`,
		`{"kind":"exec","query":"x y","nonce":"` + nonce + `"}`,
		`{"kind":"search","query":"x y","nonce":"../../etc/passwd"}`,
		`{"kind":"search","query":"x y","nonce":"` + nonce + `","extra":1}`,
	}
	for _, body := range bad {
		if _, err := parseBridgeRequest([]byte(body)); err == nil {
			t.Errorf("request accepted, want rejection: %s", body)
		}
	}
}

// A request written by an older spawn shim carries no kind at all.
func TestParseBridgeRequest_DefaultsToSpawn(t *testing.T) {
	req, err := parseBridgeRequest([]byte(`{"task":"trace auth","agent":"codex","nonce":"aabbcc"}`))
	if err != nil {
		t.Fatalf("legacy spawn request rejected: %v", err)
	}
	if req.Kind != "spawn" || req.Agent != "codex" {
		t.Errorf("unexpected parse: %+v", req)
	}
}

// The shim encodes with node; make sure what it writes is what we parse.
func TestParseBridgeRequest_MatchesShimEncoding(t *testing.T) {
	body, err := json.Marshal(map[string]string{
		"kind": "search", "query": `quotes "and" backslash \ ok`, "nonce": "0f1e2d",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := parseBridgeRequest(body)
	if err != nil {
		t.Fatalf("shim-shaped request rejected: %v", err)
	}
	if req.Query != `quotes "and" backslash \ ok` {
		t.Errorf("query mangled: %q", req.Query)
	}
}

const ddgFixture = `<html><body>
<table>
<tr><td><a rel="nofollow" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fbun.sh%2Fblog%2Fbun-v1.2&amp;rut=x" class="result-link">Bun 1.2 &amp; what changed</a></td></tr>
<tr><td class="result-snippet">Bun 1.2 ships a Node-compatible <b>module</b> resolver.</td></tr>
<tr><td><a class="result-link" rel="nofollow" href="https://example.org/notes">Plain link</a></td></tr>
<tr><td class="result-snippet">Second snippet.</td></tr>
<tr><td><a href="/settings" class="nav-link">Settings</a></td></tr>
</table></body></html>`

func TestParseDuckDuckGoHTML(t *testing.T) {
	hits := parseDuckDuckGoHTML(ddgFixture)
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2: %+v", len(hits), hits)
	}
	if hits[0].URL != "https://bun.sh/blog/bun-v1.2" {
		t.Errorf("redirector not unwrapped: %q", hits[0].URL)
	}
	if hits[0].Title != "Bun 1.2 & what changed" {
		t.Errorf("title not cleaned: %q", hits[0].Title)
	}
	if hits[0].Snippet != "Bun 1.2 ships a Node-compatible module resolver." {
		t.Errorf("snippet not cleaned: %q", hits[0].Snippet)
	}
	if hits[1].URL != "https://example.org/notes" {
		t.Errorf("plain link mangled: %q", hits[1].URL)
	}
}

// The banner is what tells an agent the text below is web content, not orders.
func TestRenderSearchReport_MarksUntrusted(t *testing.T) {
	out := renderSearchReport("a1b2c3", "bun 1.2", "ddg", []searchHit{
		{Title: "Bun", URL: "https://bun.sh", Snippet: "runtime"},
	})
	if !strings.Contains(out, "UNTRUSTED CONTENT") {
		t.Error("report is missing the untrusted banner")
	}
	if !strings.Contains(out, "https://bun.sh") {
		t.Error("report is missing the hit URL")
	}
}

func TestRenderSearchReport_EmptyExplainsItself(t *testing.T) {
	out := renderSearchReport("a1b2c3", "bun 1.2", "ddg", nil)
	if !strings.Contains(out, "No results") || !strings.Contains(out, "BRAVE_SEARCH_API_KEY") {
		t.Errorf("empty report should explain the keyless fallback:\n%s", out)
	}
}
