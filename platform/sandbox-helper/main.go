// Command alcatraz-helper is the sandbox's JSON plumbing: the small set of
// encode/decode chores the shell scripts inside the container can't do on
// their own (building request files, emitting hook payloads, merging the AI
// CLIs' settings).
//
// Why a compiled binary instead of the language runtime that happens to be
// installed:
//
//   - The sandbox's language runtime is a USER choice. Node ships today, but
//     the whole point of the LANGUAGE RUNTIME LAYER in Dockerfile.alcatraz is
//     that you can swap it for Java, Go, Python, … Alcatraz's own plumbing must
//     not break when you do, so it depends on nothing from that layer.
//   - It is narrower than an interpreter. `node -e '<program>'` hands arbitrary
//     source to a general-purpose runtime living inside the jail; this exposes a
//     fixed set of subcommands and nothing else.
//   - No toolchain ships in the image: a multi-stage build compiles it and only
//     the static binary is copied in — the same pattern platform/backend uses.
//
// Stdlib only, CGO disabled: the result is a ~2 MB static binary that runs on
// any base image.
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fail(errors.New("usage: alcatraz-helper <command> [args]\n" +
			"commands: bridge-request, hook-session-start, precompact-digest,\n" +
			"          precompact-trigger, settings-claude, settings-gemini"))
	}

	var err error
	switch os.Args[1] {
	case "bridge-request":
		err = cmdBridgeRequest(os.Args[2:])
	case "hook-session-start":
		err = cmdHookSessionStart(os.Args[2:])
	case "precompact-digest":
		err = cmdPrecompactDigest(os.Args[2:])
	case "precompact-trigger":
		err = cmdPrecompactTrigger(os.Args[2:])
	case "settings-claude":
		err = cmdSettingsClaude(os.Args[2:])
	case "settings-gemini":
		err = cmdSettingsGemini(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "alcatraz-helper:", err)
	os.Exit(1)
}

// writeJSON emits compact JSON, matching what the shell scripts produced
// before. HTML escaping is off so a query containing < or & stays readable in
// the request file (both forms decode identically).
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// ---------------------------------------------------------------------------
// bridge-request — the file the sandbox drops for the host-side watcher.
// ---------------------------------------------------------------------------

type spawnRequest struct {
	Task  string `json:"task"`
	Agent string `json:"agent"`
	Nonce string `json:"nonce"`
}

type searchRequest struct {
	Kind  string `json:"kind"`
	Query string `json:"query"`
	Nonce string `json:"nonce"`
}

func cmdBridgeRequest(args []string) error {
	fs := flag.NewFlagSet("bridge-request", flag.ContinueOnError)
	kind := fs.String("kind", "spawn", "request kind: spawn or search")
	task := fs.String("task", "", "task text (spawn only)")
	agent := fs.String("agent", "", "agent name (spawn only)")
	query := fs.String("query", "", "search query (search only)")
	nonce := fs.String("nonce", "", "hex nonce correlating request and result")
	out := fs.String("out", "", "file to write the request to")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *nonce == "" || *out == "" {
		return errors.New("bridge-request needs --nonce and --out")
	}

	// Keep the two kinds strictly separate. The host enforces this too; failing
	// here as well means a shim bug surfaces before it burns an approval prompt.
	var payload any
	switch *kind {
	case "spawn":
		if *query != "" {
			return errors.New("a spawn request must not carry --query")
		}
		if *task == "" || *agent == "" {
			return errors.New("a spawn request needs --task and --agent")
		}
		payload = spawnRequest{Task: *task, Agent: *agent, Nonce: *nonce}
	case "search":
		if *task != "" || *agent != "" {
			return errors.New("a search request must not carry --task or --agent")
		}
		if *query == "" {
			return errors.New("a search request needs --query")
		}
		payload = searchRequest{Kind: "search", Query: *query, Nonce: *nonce}
	default:
		return fmt.Errorf("unknown --kind %q (want spawn or search)", *kind)
	}

	f, err := os.Create(*out)
	if err != nil {
		return err
	}
	if err := writeJSON(f, payload); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// ---------------------------------------------------------------------------
// hook-session-start — the SessionStart payload the AI CLIs expect on stdout.
// ---------------------------------------------------------------------------

type hookOutput struct {
	HookSpecificOutput hookPayload `json:"hookSpecificOutput"`
}

type hookPayload struct {
	// Claude Code and Codex want the event name echoed back; Gemini does not
	// accept the field, so it is omitted when empty.
	HookEventName     string `json:"hookEventName,omitempty"`
	AdditionalContext string `json:"additionalContext"`
}

func cmdHookSessionStart(args []string) error {
	fs := flag.NewFlagSet("hook-session-start", flag.ContinueOnError)
	eventName := fs.String("event-name", "", "value for hookEventName (omitted when empty)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return writeJSON(os.Stdout, hookOutput{HookSpecificOutput: hookPayload{
		HookEventName:     *eventName,
		AdditionalContext: os.Getenv("MB_CTX"),
	}})
}

// ---------------------------------------------------------------------------
// precompact-* — read Claude Code's PreCompact payload.
// ---------------------------------------------------------------------------

type precompactPayload struct {
	TranscriptPath string `json:"transcript_path"`
	Trigger        string `json:"trigger"`
}

const (
	digestMessages   = 10      // how many recent turns to keep
	digestRunes      = 300     // per-turn truncation
	transcriptMaxLen = 8 << 20 // a single transcript line can be large
)

// cmdPrecompactDigest is best-effort by design: a malformed payload or an
// unreadable transcript yields an empty digest rather than a failed compaction.
func cmdPrecompactDigest(args []string) error {
	if len(args) < 1 {
		return errors.New("precompact-digest needs the hook payload as its argument")
	}
	var hook precompactPayload
	if err := json.Unmarshal([]byte(args[0]), &hook); err != nil {
		return nil // nothing to say; stay silent
	}

	f, err := os.Open(hook.TranscriptPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var msgs []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), transcriptMaxLen)
	for scanner.Scan() {
		line, ok := transcriptLine(scanner.Bytes())
		if ok {
			msgs = append(msgs, line)
		}
	}

	if len(msgs) > digestMessages {
		msgs = msgs[len(msgs)-digestMessages:]
	}
	fmt.Println(strings.Join(msgs, "\n"))
	return nil
}

// transcriptLine turns one JSONL entry into a "- role: text" digest line.
func transcriptLine(raw []byte) (string, bool) {
	var entry struct {
		Message struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		return "", false
	}
	role := entry.Message.Role
	if role != "user" && role != "assistant" {
		return "", false
	}

	text, ok := contentText(entry.Message.Content)
	if !ok {
		return "", false
	}
	// Collapse all whitespace so one turn stays on one digest line.
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "", false
	}
	if r := []rune(text); len(r) > digestRunes {
		text = string(r[:digestRunes])
	}
	return "- " + role + ": " + text, true
}

// contentText accepts both shapes the CLIs write: a plain string, or an array
// of blocks where only the "text" ones carry conversation.
func contentText(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, true
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", false
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, " "), true
}

func cmdPrecompactTrigger(args []string) error {
	trigger := "auto"
	if len(args) > 0 {
		var hook precompactPayload
		if err := json.Unmarshal([]byte(args[0]), &hook); err == nil && hook.Trigger != "" {
			trigger = hook.Trigger
		}
	}
	fmt.Println(trigger)
	return nil
}

// ---------------------------------------------------------------------------
// settings-* — merge the Mega Brain hooks into each CLI's settings file.
//
// Both merges preserve every key already in the file and are idempotent. A
// missing or corrupt file is treated as empty, so a bad settings.json costs
// the hooks, never the container's boot.
// ---------------------------------------------------------------------------

type hookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type hookGroup struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookCommand `json:"hooks"`
}

func command(cmd string) []hookGroup {
	return []hookGroup{{Hooks: []hookCommand{{Type: "command", Command: cmd}}}}
}

func matchedCommand(cmd string) []hookGroup {
	return []hookGroup{{Matcher: "*", Hooks: []hookCommand{{Type: "command", Command: cmd}}}}
}

type workspaceProject struct {
	AllowedTools                  []string       `json:"allowedTools"`
	McpServers                    map[string]any `json:"mcpServers"`
	EnabledMcpjsonServers         []string       `json:"enabledMcpjsonServers"`
	DisabledMcpjsonServers        []string       `json:"disabledMcpjsonServers"`
	HasTrustDialogAccepted        bool           `json:"hasTrustDialogAccepted"`
	ProjectOnboardingSeenCount    int            `json:"projectOnboardingSeenCount"`
	HasCompletedProjectOnboarding bool           `json:"hasCompletedProjectOnboarding"`
}

func cmdSettingsClaude(args []string) error {
	if len(args) < 4 {
		return errors.New("settings-claude needs <path> <hook-start> <hook-end> <hook-precompact>")
	}
	path, hStart, hEnd, hPrecompact := args[0], args[1], args[2], args[3]

	data := readSettings(path)

	projects := asMap(data["projects"])
	projects["/workspace"] = workspaceProject{
		AllowedTools: []string{"Read", "Glob", "Grep", "Bash", "WebFetch",
			"StrReplaceBasedEditTool", "Write"},
		McpServers:                    map[string]any{},
		EnabledMcpjsonServers:         []string{},
		DisabledMcpjsonServers:        []string{},
		HasTrustDialogAccepted:        true,
		ProjectOnboardingSeenCount:    0,
		HasCompletedProjectOnboarding: true,
	}
	data["projects"] = projects

	hooks := asMap(data["hooks"])
	hooks["SessionStart"] = command(hStart)
	hooks["SessionEnd"] = command(hEnd + " claude")
	// Snapshot the vault before Claude compresses the context window.
	hooks["PreCompact"] = command(hPrecompact)
	data["hooks"] = hooks

	if err := writeSettings(path, data); err != nil {
		return err
	}
	fmt.Println("[mega-brain-init] claude settings.json updated")
	return nil
}

func cmdSettingsGemini(args []string) error {
	if len(args) < 3 {
		return errors.New("settings-gemini needs <path> <hook-start> <hook-end>")
	}
	path, hStart, hEnd := args[0], args[1], args[2]

	data := readSettings(path)

	hooks := asMap(data["hooks"])
	hooks["SessionStart"] = matchedCommand(hStart)
	hooks["SessionEnd"] = matchedCommand(hEnd + " gemini")
	data["hooks"] = hooks

	// Disable Gemini's native memory: the vault is the single source of truth.
	data["excludeTools"] = addExcluded(data["excludeTools"], "save_memory")

	if err := writeSettings(path, data); err != nil {
		return err
	}
	fmt.Println("[mega-brain-init] gemini settings.json updated (excludeTools: save_memory)")
	return nil
}

func readSettings(path string) map[string]any {
	b, err := os.ReadFile(path)
	if err != nil {
		return map[string]any{}
	}
	var data map[string]any
	if err := json.Unmarshal(b, &data); err != nil || data == nil {
		return map[string]any{}
	}
	return data
}

func writeSettings(path string, data map[string]any) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

// asMap returns v as a JSON object, replacing anything that isn't one (missing,
// null, a string, …) with a fresh map so the merge can always proceed.
func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok && m != nil {
		return m
	}
	return map[string]any{}
}

// addExcluded unions one tool into an existing excludeTools list, sorted so the
// result is stable across runs.
func addExcluded(v any, tool string) []string {
	seen := map[string]bool{tool: true}
	if list, ok := v.([]any); ok {
		for _, item := range list {
			if s, ok := item.(string); ok && s != "" {
				seen[s] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
