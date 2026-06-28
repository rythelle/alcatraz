package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

type MITMProxy struct {
	ca       *CA
	audit    *AuditLogger
	log      zerolog.Logger
	upstream string
	port     int
	dryRun   bool
	reqID    uint64
	mu       sync.Mutex
}

func NewMITMProxy(ca *CA, audit *AuditLogger, upstream string, port int, dryRun bool, log zerolog.Logger) *MITMProxy {
	return &MITMProxy{
		ca:       ca,
		audit:    audit,
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

		p.sanitizeRequest(req, host)

		resp, err := client.Do(req)
		if err != nil {
			p.log.Error().Err(err).Str("host", host).Msg("Upstream HTTPS request failed")
			fmt.Fprintf(tlsConn, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
			break
		}

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

func (p *MITMProxy) sanitizeRequest(req *http.Request, host string) {
	if req.Body == nil {
		return
	}

	contentType := req.Header.Get("Content-Type")
	if !IsJSON(contentType) {
		return
	}

	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil || len(body) == 0 {
		return
	}

	originalLen := len(body)
	result := SanitizeJSON(string(body), p.dryRun)

	if result.Modified {
		req.Body = io.NopCloser(strings.NewReader(result.Output))
		req.ContentLength = int64(len(result.Output))
		req.Header.Set("Content-Length", fmt.Sprintf("%d", len(result.Output)))
		p.logSanitize(host, result.Detections, originalLen)
	} else {
		req.Body = io.NopCloser(strings.NewReader(string(body)))
	}
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
		Msg("DATA GUARDIAN sanitized")

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

