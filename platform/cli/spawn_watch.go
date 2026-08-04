package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/alcatraz/alcatraz/cli/internal/modules"
	"github.com/spf13/cobra"
)

// The bridge lets someone working *inside* the sandbox shell ask the host for
// something the sandbox deliberately cannot do itself — without handing the
// sandbox any new power. It serves two kinds of request:
//
//	spawn  — delegate a heavy exploration to a disposable sibling sandbox,
//	         without ever giving the sandbox access to Docker.
//	search — one web lookup, performed by the host as a single https GET,
//	         because search engines are not on the Lighthouse allowlist.
//
// In both cases the in-container shim only writes a request file into
// <project>/.alcatraz/requests/ (the project is bind-mounted, so the file lands
// on the host); this host-side watcher picks it up and does the work.
//
// The sandbox is treated as fully untrusted (it may be prompt-injected). Every
// value in a request is validated before use, nothing from a request may steer
// a host filesystem path except the strictly-hex `nonce`, and the operator
// approves each request — always for a search, which sends data out of the box.
// This keeps the project's core guarantee — the sandbox cannot reach out of its
// box on its own — intact.
const (
	maxRequestBytes = 8 << 10 // 8 KiB: a request is a tiny JSON blob
	maxTaskLen      = 4000    // cap on the untrusted task string
)

// nonceRe bounds the only request-controlled value that touches a host path.
var nonceRe = regexp.MustCompile(`^[a-f0-9]{6,32}$`)

// bridgeRequest is the ONLY shape the host accepts from the sandbox. Unknown
// fields are rejected (DisallowUnknownFields), and each kind may populate only
// its own fields.
type bridgeRequest struct {
	Kind  string `json:"kind"` // "spawn" (default) | "search"
	Task  string `json:"task"`
	Agent string `json:"agent"`
	Query string `json:"query"`
	Nonce string `json:"nonce"`
}

// parseBridgeRequest strictly validates an untrusted request blob.
func parseBridgeRequest(data []byte) (bridgeRequest, error) {
	var req bridgeRequest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return req, fmt.Errorf("malformed json: %w", err)
	}
	if !nonceRe.MatchString(req.Nonce) {
		return req, fmt.Errorf("bad nonce")
	}
	if req.Kind == "" {
		req.Kind = "spawn" // requests predating the search kind
	}

	switch req.Kind {
	case "spawn":
		if req.Query != "" {
			return req, fmt.Errorf("spawn request must not carry a query")
		}
		req.Task = strings.TrimSpace(req.Task)
		switch {
		case req.Task == "":
			return req, fmt.Errorf("empty task")
		case len(req.Task) > maxTaskLen:
			return req, fmt.Errorf("task too long (%d > %d)", len(req.Task), maxTaskLen)
		}
		if req.Agent == "" {
			req.Agent = "claude"
		}
		if !isValidAgent(req.Agent) {
			return req, fmt.Errorf("agent not allowed: %q", req.Agent)
		}
	case "search":
		// A search runs no agent and no task — refuse anything that suggests
		// the caller expected it to.
		if req.Task != "" || req.Agent != "" {
			return req, fmt.Errorf("search request must not carry a task or agent")
		}
		q, err := validateSearchQuery(req.Query)
		if err != nil {
			return req, err
		}
		req.Query = q
	default:
		return req, fmt.Errorf("unknown request kind: %q", req.Kind)
	}
	return req, nil
}

func spawnWatchCmd() *cobra.Command {
	var project string
	var auto bool
	var maxSpawns int
	var interval int
	var searchPerHour int

	cmd := &cobra.Command{
		Use:     "spawn-watch",
		Aliases: []string{"bridge"},
		Short:   "Serve in-sandbox spawn and web-search requests from the host (the bridge)",
		Long: `Watch a project's .alcatraz/requests/ for requests written by the in-sandbox
shims, and service each one on the host.

Two kinds are served, each gated by its own module:
  spawn   ('spawn' shim)     → runs the task in a disposable sibling sandbox
  search  ('websearch' shim) → performs ONE https GET against a search endpoint

The sandbox has no Docker access and no route to the internet — it can only drop
a request file. This watcher runs on the host, treats every request as
untrusted, and asks you to approve it before acting. Search requests always
prompt (even with --auto): the query leaves the box, so a human sees it every
time. The result is written to .alcatraz/results/<nonce>.md, which the shim
inside the shell reads back.

Run this in a spare host terminal while you work in 'alcatraz shell'. Requires
the egress stack (guard + lighthouse) to be up.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := spawnEgressReady(); err != nil {
				return err
			}
			absPath, name, workdir, err := resolveSpawnProject(project)
			if err != nil {
				return err
			}
			base := filepath.Join(absPath, ".alcatraz")
			reqDir := filepath.Join(base, "requests")
			resDir := filepath.Join(base, "results")
			auditPath := filepath.Join(base, "spawn-audit.log")
			searchAuditPath := filepath.Join(base, "search-audit.log")
			for _, d := range []string{reqDir, resDir} {
				if err := os.MkdirAll(d, 0755); err != nil {
					return err
				}
			}

			w := &bridgeWatcher{
				reqDir: reqDir, resDir: resDir,
				auditPath: auditPath, searchAuditPath: searchAuditPath,
				absPath: absPath, name: name, workdir: workdir,
				auto: auto, maxSpawns: maxSpawns, searchPerHour: searchPerHour,
				spawnOn:  modules.Enabled(projectRoot, "spawn"),
				searchOn: modules.Enabled(projectRoot, "websearch"),
				reader:   bufio.NewReader(os.Stdin),
			}
			if !w.spawnOn && !w.searchOn {
				return fmt.Errorf("both 'spawn' and 'websearch' are off — nothing to serve (alcatraz modules <name> on)")
			}

			mode := "approval required (y/N per request)"
			if auto {
				mode = "AUTO for spawns — searches still prompt (cap + audit log)"
			}
			fmt.Printf("🛰  bridge — serving %s\n", name)
			fmt.Printf("    requests: %s\n", reqDir)
			fmt.Printf("    serving:  %s\n", strings.Join(w.servedKinds(), ", "))
			fmt.Printf("    mode:     %s\n", mode)
			fmt.Printf("    the sandbox has no Docker and no route out; every request is validated as untrusted input.\n")
			fmt.Printf("    Ctrl+C to stop.\n\n")

			ticker := time.NewTicker(time.Duration(interval) * time.Millisecond)
			defer ticker.Stop()
			for range ticker.C {
				for _, fn := range pendingRequests(reqDir) {
					w.serve(fn)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&project, "project", "p", "", "Project to serve (path or alias; default: active workspace)")
	cmd.Flags().BoolVar(&auto, "auto", false, "Run spawns without a per-request approval prompt (searches always prompt)")
	cmd.Flags().IntVar(&maxSpawns, "max", defaultMaxSpawns, "Max concurrent spawns")
	cmd.Flags().IntVar(&interval, "interval", 1000, "Poll interval in milliseconds")
	cmd.Flags().IntVar(&searchPerHour, "search-per-hour", defaultSearchPerHour, "Max approved web searches per rolling hour")
	return cmd
}

// bridgeWatcher carries the per-run configuration of the watcher loop so the
// two request kinds don't each need a ten-argument function.
type bridgeWatcher struct {
	reqDir, resDir             string
	auditPath, searchAuditPath string
	absPath, name, workdir     string
	auto                       bool
	maxSpawns, searchPerHour   int
	spawnOn, searchOn          bool
	reader                     *bufio.Reader
}

func (w *bridgeWatcher) servedKinds() []string {
	var kinds []string
	if w.spawnOn {
		kinds = append(kinds, "spawn")
	}
	if w.searchOn {
		kinds = append(kinds, "search")
	}
	return kinds
}

// confirm shows a y/N prompt and reports whether the operator approved.
func (w *bridgeWatcher) confirm() bool {
	fmt.Print("  Approve? [y/N] ")
	line, _ := w.reader.ReadString('\n')
	return strings.TrimSpace(strings.ToLower(line)) == "y"
}

// pendingRequests lists request files in deterministic order, skipping the
// shim's in-progress dotfile temporaries.
func pendingRequests(reqDir string) []string {
	entries, err := os.ReadDir(reqDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, ".") || !strings.HasSuffix(n, ".json") {
			continue
		}
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// serve validates one request file and dispatches it to its handler.
func (w *bridgeWatcher) serve(fn string) {
	reqDir, resDir, auditPath := w.reqDir, w.resDir, w.auditPath
	full := filepath.Join(reqDir, fn)

	// SECURITY: only ever touch a regular file. A hostile sandbox could plant a
	// symlink named <x>.json pointing elsewhere on the host to trick us into
	// reading (or, on removal, deleting) an arbitrary path.
	info, err := os.Lstat(full)
	if err != nil {
		return
	}
	if !info.Mode().IsRegular() {
		rejectRequest(full, auditPath, fn, "", "not a regular file")
		return
	}
	if info.Size() > maxRequestBytes {
		rejectRequest(full, auditPath, fn, "", "oversized request")
		return
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return
	}
	req, err := parseBridgeRequest(data)
	if err != nil {
		rejectRequest(full, auditPath, fn, "", err.Error())
		return
	}

	// SECURITY: the result path is derived ONLY from the validated hex nonce and
	// must stay inside resDir (defence in depth against path traversal).
	resPath := filepath.Join(resDir, req.Nonce+".md")
	if filepath.Dir(resPath) != filepath.Clean(resDir) {
		rejectRequest(full, auditPath, fn, req.Nonce, "path escape")
		return
	}

	// Consume the request before running so a looping producer can't get the
	// same task executed twice.
	_ = os.Remove(full)

	switch req.Kind {
	case "search":
		if !w.searchOn {
			w.refuseSearch(resPath, req, "module 'websearch' is off on the host")
			return
		}
		w.serveSearch(resPath, req)
	default:
		if !w.spawnOn {
			writeResult(resPath, fmt.Sprintf("# Spawn request %s refused\n\nThe 'spawn' module is off on the host.\n", req.Nonce))
			appendAudit(auditPath, req, "module-off", -1)
			fmt.Printf("  ✗ spawn request [%s] refused: module off\n", req.Nonce)
			return
		}
		w.serveSpawn(resPath, req)
	}
}

// serveSpawn runs an approved task in a disposable sibling sandbox.
func (w *bridgeWatcher) serveSpawn(resPath string, req bridgeRequest) {
	auditPath := w.auditPath

	fmt.Printf("→ spawn request from sandbox [%s]\n", req.Nonce)
	fmt.Printf("    agent: %s\n", req.Agent)
	fmt.Printf("    task:  %s\n", oneLine(req.Task, 500))

	decision := "auto"
	if !w.auto {
		if !w.confirm() {
			fmt.Println("  ✗ denied")
			writeResult(resPath, deniedResult(req))
			appendAudit(auditPath, req, "denied", -1)
			return
		}
		decision = "approved"
	}

	fmt.Printf("  ⏳ running %s…\n", req.Agent)
	out, err := runSpawnTask(req.Agent, "", req.Task, w.absPath, w.name, w.workdir, w.maxSpawns, false, os.Stderr)
	if err != nil {
		fmt.Printf("  ⚠ spawn failed: %v\n", err)
		writeResult(resPath, fmt.Sprintf("# Spawn request %s failed\n\n```\n%v\n```\n", req.Nonce, err))
		appendAudit(auditPath, req, decision+"/error", -1)
		return
	}
	writeResult(resPath, out.reportText)
	fmt.Printf("  ✓ done → .alcatraz/results/%s.md  (canonical: .alcatraz/%s)\n\n",
		req.Nonce, filepath.Base(out.reportPath))
	appendAudit(auditPath, req, decision, out.exitCode)
}

// serveSearch performs the one https GET an approved search is allowed.
//
// Order matters here, and every step before the fetch is a reason NOT to make
// it: rate limit, then the Guard sanitizer (a query carrying a secret is
// refused outright, never redacted-and-sent), then a human. Only then does
// anything leave the host.
func (w *bridgeWatcher) serveSearch(resPath string, req bridgeRequest) {
	fmt.Printf("→ web search request from sandbox [%s]\n", req.Nonce)
	fmt.Printf("    query: %s\n", req.Query)

	if n := recentSearchCount(w.searchAuditPath); n >= w.searchPerHour {
		w.refuseSearch(resPath, req, fmt.Sprintf("rate limit reached (%d searches in the last hour)", n))
		return
	}

	// The query is about to leave the box. Run it through the same engine that
	// scrubs outbound prompts; anything it would redact is a secret that must
	// not be turned into a search term.
	redacts, err := guardWouldRedact(req.Query)
	if err != nil {
		w.refuseSearch(resPath, req, err.Error())
		return
	}
	if redacts {
		w.refuseSearch(resPath, req, "the Guard engine found a secret in the query")
		return
	}

	// Always ask, even under --auto: this is the step where data leaves the box.
	fmt.Printf("    provider: %s — this sends the query above to the internet.\n", searchProvider())
	if !w.confirm() {
		fmt.Println("  ✗ denied")
		writeResult(resPath, searchRefusedReport(req.Nonce, req.Query, "the host operator declined this search"))
		appendSearchAudit(w.searchAuditPath, req, "denied", "", 0)
		return
	}

	fmt.Printf("  ⏳ fetching…\n")
	hits, provider, err := runWebSearch(req.Query)
	if err != nil {
		fmt.Printf("  ⚠ search failed: %v\n", err)
		writeResult(resPath, searchRefusedReport(req.Nonce, req.Query, "the lookup failed: "+err.Error()))
		appendSearchAudit(w.searchAuditPath, req, "error", provider, 0)
		return
	}
	writeResult(resPath, renderSearchReport(req.Nonce, req.Query, provider, hits))
	fmt.Printf("  ✓ %d hit(s) → .alcatraz/results/%s.md\n\n", len(hits), req.Nonce)
	appendSearchAudit(w.searchAuditPath, req, "fetched", provider, len(hits))
}

// refuseSearch writes the refusal back so the waiting shim always terminates
// with an explanation instead of hanging.
func (w *bridgeWatcher) refuseSearch(resPath string, req bridgeRequest, reason string) {
	fmt.Printf("  ✗ refused: %s\n\n", reason)
	writeResult(resPath, searchRefusedReport(req.Nonce, req.Query, reason))
	appendSearchAudit(w.searchAuditPath, req, "refused", "", 0)
}

func rejectRequest(full, auditPath, fn, nonce, reason string) {
	_ = os.Remove(full)
	fmt.Printf("  ✗ rejected %s: %s\n", fn, reason)
	appendLine(auditPath, fmt.Sprintf("%s\treject\tnonce=%s\treason=%s\tfile=%s\n",
		time.Now().Format(time.RFC3339), nonce, reason, fn))
}

// writeResult writes the result atomically so the shim never reads a partial file.
func writeResult(resPath, content string) {
	tmp := resPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return
	}
	_ = os.Rename(tmp, resPath)
}

func appendAudit(auditPath string, req bridgeRequest, decision string, exit int) {
	appendLine(auditPath, fmt.Sprintf("%s\t%s\tnonce=%s\tagent=%s\texit=%d\ttask=%q\n",
		time.Now().Format(time.RFC3339), decision, req.Nonce, req.Agent, exit, oneLine(req.Task, 200)))
}

// appendSearchAudit records every search decision — including the ones that
// never reached the network — in its own log. recentSearchCount reads it back
// for the rate limit, and it is the record of what left the box.
func appendSearchAudit(auditPath string, req bridgeRequest, decision, provider string, hits int) {
	appendLine(auditPath, fmt.Sprintf("%s\t%s\tnonce=%s\tprovider=%s\thits=%d\tquery=%q\n",
		time.Now().Format(time.RFC3339), decision, req.Nonce, provider, hits, oneLine(req.Query, 256)))
}

func appendLine(path, line string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}

func deniedResult(req bridgeRequest) string {
	return fmt.Sprintf("# Spawn request %s denied\n\nThe host operator declined this spawn.\n\n- agent: %s\n- task: %s\n",
		req.Nonce, req.Agent, req.Task)
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
