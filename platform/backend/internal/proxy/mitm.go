package proxy

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/alcatraz/alcatraz/internal/rules"
	"github.com/rs/zerolog"
)

type MITMProxy struct {
	ca       *CA
	audit    *AuditLogger
	stats    *StatsLogger
	rules    *rules.Watcher
	vault    *Vault
	log      zerolog.Logger
	upstream string
	port     int
	dryRun   bool
	reqID    uint64
	mu       sync.Mutex
}

func NewMITMProxy(ca *CA, audit *AuditLogger, stats *StatsLogger, rulesWatcher *rules.Watcher, vault *Vault, upstream string, port int, dryRun bool, log zerolog.Logger) *MITMProxy {
	return &MITMProxy{
		ca:       ca,
		audit:    audit,
		stats:    stats,
		rules:    rulesWatcher,
		vault:    vault,
		log:      log,
		upstream: upstream,
		port:     port,
		dryRun:   dryRun,
	}
}

func (p *MITMProxy) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p.port))
	if err != nil {
		return fmt.Errorf("mitm listen: %w", err)
	}

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	p.log.Info().Int("port", p.port).Str("upstream", p.upstream).Msg("MITM proxy listening")

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				p.log.Error().Err(err).Msg("Accept failed")
				continue
			}
		}
		go p.handleConn(conn)
	}
}

func (p *MITMProxy) handleConn(clientConn net.Conn) {
	defer clientConn.Close()

	reader := bufio.NewReader(clientConn)
	req, err := http.ReadRequest(reader)
	if err != nil {
		return
	}

	if req.Method == http.MethodConnect {
		p.handleHTTPS(clientConn, reader, req)
	} else {
		p.handleHTTPRequest(clientConn, reader, req)
	}
}

func (p *MITMProxy) handleHTTPS(clientConn net.Conn, reader *bufio.Reader, connectReq *http.Request) {
	host := connectReq.Host
	p.log.Debug().Str("host", host).Msg("CONNECT")

	clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))

	tlsCert, err := p.ca.GenerateCertForHost(host)
	if err != nil {
		p.log.Error().Err(err).Str("host", host).Msg("Failed to generate cert")
		return
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*tlsCert},
		MinVersion:   tls.VersionTLS12,
	}

	tlsConn := tls.Server(clientConn, tlsConfig)
	if err := tlsConn.Handshake(); err != nil {
		return
	}
	// Close the TLS layer, not just the socket: tls.Conn.Close sends a
	// close_notify alert first. Without it the client sees the stream end
	// abruptly, and strict TLS stacks (rustls, as used by Codex) reject the
	// body instead of accepting it — "error decoding response body" on any
	// response whose length isn't known up front.
	defer tlsConn.Close()

	client := p.upstreamClient()
	tlsReader := bufio.NewReader(tlsConn)

	for {
		req, err := http.ReadRequest(tlsReader)
		if err != nil {
			break
		}

		req.URL.Scheme = "https"
		req.URL.Host = host
		req.RequestURI = ""

		model := p.sanitizeRequest(req, host)

		resp, err := client.Do(req)
		if err != nil {
			p.log.Error().Err(err).Str("host", host).Msg("Upstream HTTPS request failed")
			fmt.Fprintf(tlsConn, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
			break
		}

		p.meterResponse(resp, host, model)
		p.detokenizeResponse(resp, host)

		// Downgrade HTTP/2 responses to HTTP/1.1 — Node.js's HTTP parser
		// rejects "HTTP/2.0" in the status line as Malformed_HTTP_Response.
		resp.Proto = "HTTP/1.1"
		resp.ProtoMajor = 1
		resp.ProtoMinor = 1

		// For streaming responses (unknown Content-Length, no explicit
		// Transfer-Encoding), use Connection: close so the body is forwarded
		// directly without chunked encoding. Chunked encoding can delay SSE
		// event delivery when buffers fill slowly.
		streaming := resp.ContentLength < 0 && len(resp.TransferEncoding) == 0
		if streaming {
			resp.Close = true
		}

		writeErr := resp.Write(tlsConn)
		resp.Body.Close()
		if writeErr != nil || streaming {
			break
		}
	}
}

func (p *MITMProxy) handleHTTPRequest(clientConn net.Conn, reader *bufio.Reader, req *http.Request) {
	req.RequestURI = ""
	p.sanitizeRequest(req, req.Host)

	resp, err := p.upstreamClient().Do(req)
	if err != nil {
		p.log.Error().Err(err).Str("host", req.Host).Msg("Upstream HTTP request failed")
		fmt.Fprintf(clientConn, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}
	defer resp.Body.Close()
	resp.Write(clientConn)
}

// oauthTokenEndpoints maps auth hosts to path prefixes of their OAuth token
// endpoints. Their JSON bodies carry credentials by design (refresh_token /
// authorization_code grants); redacting them breaks token refresh and forces
// the AI CLIs to re-login after every token expiry.
var oauthTokenEndpoints = map[string][]string{
	"platform.claude.com":   {"/v1/oauth"},
	"console.anthropic.com": {"/v1/oauth"},
	"claude.ai":             {"/v1/oauth"},
	// Codex's device-code grant lives under /api/accounts/deviceauth, not
	// /oauth — same class of endpoint, same credential-carrying bodies.
	"auth.openai.com":       {"/oauth", "/api/accounts/deviceauth"},
	"oauth2.googleapis.com": {"/token"},
	"accounts.google.com":   {"/o/oauth2"},
}

func isOAuthTokenEndpoint(host, path string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	for _, prefix := range oauthTokenEndpoints[host] {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// sanitizeRequest scrubs the request body in place and returns the "model"
// field observed in it (empty when absent), which feeds token telemetry.
func (p *MITMProxy) sanitizeRequest(req *http.Request, host string) string {
	if req.Body == nil {
		return ""
	}

	if isOAuthTokenEndpoint(host, req.URL.Path) {
		p.log.Debug().Str("host", host).Str("path", req.URL.Path).Msg("OAuth token endpoint — skipping sanitization")
		return ""
	}

	contentType := req.Header.Get("Content-Type")
	if !IsJSON(contentType) {
		return ""
	}

	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil || len(body) == 0 {
		return ""
	}

	model := ExtractModel(body)
	originalLen := len(body)
	var rs *rules.RuleSet
	if p.rules != nil {
		rs = p.rules.Current()
	}
	result := SanitizeJSONWithVault(string(body), rs, p.vault, p.dryRun)

	if result.Modified {
		req.Body = io.NopCloser(strings.NewReader(result.Output))
		req.ContentLength = int64(len(result.Output))
		req.Header.Set("Content-Length", fmt.Sprintf("%d", len(result.Output)))
		p.logSanitize(host, result.Detections, originalLen)
	} else {
		req.Body = io.NopCloser(strings.NewReader(string(body)))
	}
	return model
}

// meterResponse wraps AI-provider response bodies so token usage is recorded
// once the response finishes streaming. Bytes pass through untouched.
func (p *MITMProxy) meterResponse(resp *http.Response, host, model string) {
	if p.stats == nil || resp.Body == nil {
		return
	}
	provider := DetectProvider(host)
	if provider == "unknown" {
		return
	}
	resp.Body = newUsageScanner(resp.Body, resp.Header.Get("Content-Encoding"), func(u TokenUsage) {
		p.stats.Log(StatsEntry{
			Provider:   provider,
			Model:      model,
			Host:       host,
			TokenUsage: u,
		})
	})
}

// detokenizeResponse restores vault tokens the model echoed back into their
// original values before the response reaches the AI CLI — so redaction is
// reversible and doesn't break workflows that need the real value. Only AI
// provider responses can contain tokens.
//
// Provider responses are usually gzip-compressed SSE, so a byte-level regex
// can't see the tokens: the body is decompressed first, detokenized, and
// re-sent as identity (the Content-Encoding header is dropped). Non-streaming
// bodies are buffered and length-corrected; streaming bodies flow through a
// boundary-safe wrapper. Unsupported encodings (br/zstd/deflate) are skipped —
// the token stays literal (degraded UX) but is never leaked.
func (p *MITMProxy) detokenizeResponse(resp *http.Response, host string) {
	if p.vault == nil || resp.Body == nil {
		return
	}
	if DetectProvider(host) == "unknown" {
		return
	}
	ce := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	gzipped := ce == "gzip"
	if ce != "" && ce != "identity" && !gzipped {
		return
	}
	isSSE := strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "event-stream")

	// SSE: tokens are split across delta events, so reassemble the model text
	// across events. Always stream (drop any fixed length) and send identity.
	if isSSE {
		src, ok := p.plaintextBody(resp, gzipped)
		if !ok {
			return
		}
		resp.Header.Del("Content-Length")
		resp.ContentLength = -1
		resp.Body = p.vault.NewSSEDetokReader(src)
		return
	}

	// Non-SSE, fixed length: buffer, restore, correct length.
	if resp.ContentLength >= 0 {
		raw, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			resp.Body = io.NopCloser(bytes.NewReader(raw))
			return
		}
		data := raw
		if gzipped {
			d, derr := gunzipAll(raw)
			if derr != nil {
				resp.Body = io.NopCloser(bytes.NewReader(raw))
				return
			}
			data = d
		}
		out := p.vault.Detokenize(string(data))
		if gzipped {
			resp.Header.Del("Content-Encoding")
		}
		resp.Body = io.NopCloser(strings.NewReader(out))
		resp.ContentLength = int64(len(out))
		resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(out)))
		return
	}

	// Non-SSE streaming: a contiguous token is caught by the byte-level wrapper.
	src, ok := p.plaintextBody(resp, gzipped)
	if !ok {
		return
	}
	resp.Body = p.vault.NewDetokReader(src)
}

// plaintextBody returns a reader over the (decompressed) response body, dropping
// the Content-Encoding header when it unwraps gzip. ok is false only when a
// declared gzip body can't be opened, in which case the caller leaves the
// response untouched.
func (p *MITMProxy) plaintextBody(resp *http.Response, gzipped bool) (io.ReadCloser, bool) {
	if !gzipped {
		return resp.Body, true
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		return nil, false
	}
	resp.Header.Del("Content-Encoding")
	return &gzReadCloser{zr: zr, under: resp.Body}, true
}

// gzReadCloser reads a gzip stream and closes both the decompressor and the
// underlying body.
type gzReadCloser struct {
	zr    *gzip.Reader
	under io.ReadCloser
}

func (g *gzReadCloser) Read(p []byte) (int, error) { return g.zr.Read(p) }
func (g *gzReadCloser) Close() error {
	g.zr.Close()
	return g.under.Close()
}

func gunzipAll(b []byte) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

func (p *MITMProxy) upstreamClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: func(_ *http.Request) (*url.URL, error) {
				return &url.URL{Scheme: "http", Host: p.upstream}, nil
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (p *MITMProxy) logSanitize(host string, detections []Detection, reqSize int) {
	if len(detections) == 0 {
		return
	}

	provider := DetectProvider(host)

	detStr := make([]string, len(detections))
	for i, d := range detections {
		detStr[i] = fmt.Sprintf("%s(%d)", d.Pattern, d.Count)
	}

	p.log.Info().
		Str("host", host).
		Str("provider", provider).
		Strs("detections", detStr).
		Int("request_size", reqSize).
		Msg("GUARD sanitized")

	p.mu.Lock()
	p.reqID++
	id := p.reqID
	p.mu.Unlock()

	p.audit.Log(AuditEntry{
		RequestID:   fmt.Sprintf("req-%d", id),
		Host:        host,
		Method:      "POST",
		Provider:    provider,
		Detections:  detections,
		RequestSize: reqSize,
	})
}
