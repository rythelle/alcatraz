package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// Regression: Guard used to close the client socket without shutting the TLS
// layer down, so every HTTPS response ended without a close_notify alert.
// curl tolerated it (exit 56, body still printed); Codex's rustls stack did
// not, and any response whose length wasn't known up front failed with
// "error decoding response body" — which is what broke `codex login`.
//
// Go's crypto/tls cannot be used to detect this from the API side: it reports
// a plain io.EOF whether or not close_notify arrived, which is exactly why the
// bug went unnoticed against Go clients while breaking Codex. So the test
// inspects the wire instead. The client is pinned to TLS 1.2, where the record
// content type stays in the clear, and asserts an alert record (type 21)
// actually reaches the client before the socket closes.

// freePort reserves and releases a port so the proxy can bind it.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// startTestMITM runs a Guard MITM proxy with a dead upstream, so requests take
// the 502 path — the same `break` out of the serve loop that a streaming
// response takes, and the one the close_notify fix guards.
func startTestMITM(t *testing.T) (addr string, caPEM []byte) {
	t.Helper()

	ca, err := NewCA(t.TempDir())
	if err != nil {
		t.Fatalf("ca: %v", err)
	}
	audit, err := NewAuditLogger(t.TempDir()+"/audit.log", false)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	t.Cleanup(func() { audit.Close() })

	// Upstream points at a closed port: the proxy answers 502 and tears the
	// connection down, which is exactly the path under test.
	port := freePort(t)
	dead := fmt.Sprintf("127.0.0.1:%d", freePort(t))

	p := NewMITMProxy(ca, audit, nil, nil, nil, dead, port, false, zerolog.Nop())
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = p.Start(ctx) }()

	addr = fmt.Sprintf("127.0.0.1:%d", port)
	waitForListener(t, addr)
	return addr, ca.CertPEM()
}

func waitForListener(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			c.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("proxy never listened on %s", addr)
}

// recordingConn notes the TLS record content types the server sends. In TLS
// 1.2 the type byte of each record header is not encrypted, so a close_notify
// is visible as a record of type 21 (alert).
type recordingConn struct {
	net.Conn
	pending []byte
	types   []byte
}

const recAlert = 21

func (r *recordingConn) Read(p []byte) (int, error) {
	n, err := r.Conn.Read(p)
	if n > 0 {
		r.pending = append(r.pending, p[:n]...)
		for len(r.pending) >= 5 {
			length := int(r.pending[3])<<8 | int(r.pending[4])
			if len(r.pending) < 5+length {
				break
			}
			r.types = append(r.types, r.pending[0])
			r.pending = r.pending[5+length:]
		}
	}
	return n, err
}

func (r *recordingConn) sawAlert() bool {
	for _, t := range r.types {
		if t == recAlert {
			return true
		}
	}
	return false
}

// connectThroughProxy performs the CONNECT handshake and returns a TLS client
// connection to the proxy's man-in-the-middle certificate, plus the recorder
// sitting under it.
func connectThroughProxy(t *testing.T, addr string, caPEM []byte, host string) (*tls.Conn, *recordingConn) {
	t.Helper()

	raw, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	t.Cleanup(func() { raw.Close() })

	if _, err := fmt.Fprintf(raw, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", host, host); err != nil {
		t.Fatalf("send CONNECT: %v", err)
	}
	br := bufio.NewReader(raw)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("CONNECT returned %s", resp.Status)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		t.Fatal("could not load the proxy CA")
	}
	name, _, _ := net.SplitHostPort(host)
	rec := &recordingConn{Conn: raw}
	tlsConn := tls.Client(rec, &tls.Config{
		RootCAs:    pool,
		ServerName: name,
		// Pinned so the record content type stays readable on the wire.
		MaxVersion: tls.VersionTLS12,
	})
	if err := tlsConn.Handshake(); err != nil {
		t.Fatalf("TLS handshake through proxy: %v", err)
	}
	return tlsConn, rec
}

func TestProxySendsCloseNotify(t *testing.T) {
	addr, caPEM := startTestMITM(t)
	conn, rec := connectThroughProxy(t, addr, caPEM, "api.example.test:443")

	if _, err := fmt.Fprint(conn, "GET /v1/thing HTTP/1.1\r\nHost: api.example.test\r\n\r\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Drain to the end of the connection so any trailing record is observed.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("expected a clean EOF after the response, got %v", err)
	}

	if !rec.sawAlert() {
		t.Fatalf("no close_notify alert on the wire (records seen: %v) — "+
			"the connection was torn down abruptly and Codex login is broken again", rec.types)
	}
}

// The MITM cert has to chain to Guard's CA, otherwise a strict client rejects
// the connection before any of the above matters.
//
// The host has to be one Guard actually inspects: auth.openai.com, used here
// before, is in mitmBypassHosts now and is tunnelled raw, so no cert of ours
// is ever presented for it. That bypass is asserted by TestShouldBypassMITM.
func TestProxyCertificateChainsToCA(t *testing.T) {
	addr, caPEM := startTestMITM(t)
	conn, _ := connectThroughProxy(t, addr, caPEM, "api.openai.com:443")

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		t.Fatal("no certificate presented")
	}
	if err := state.PeerCertificates[0].VerifyHostname("api.openai.com"); err != nil {
		t.Errorf("certificate is not valid for the requested host: %v", err)
	}
}

// A client that verifies the chain and is given the wrong root must fail —
// proof the handshake above succeeded on trust, not on a permissive config.
func TestProxyRejectedWithoutItsCA(t *testing.T) {
	addr, _ := startTestMITM(t)

	raw, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer raw.Close()
	fmt.Fprint(raw, "CONNECT api.example.test:443 HTTP/1.1\r\nHost: api.example.test:443\r\n\r\n")
	if _, err := http.ReadResponse(bufio.NewReader(raw), nil); err != nil {
		t.Fatalf("CONNECT: %v", err)
	}

	conn := tls.Client(raw, &tls.Config{RootCAs: x509.NewCertPool(), ServerName: "api.example.test"})
	if err := conn.Handshake(); err == nil {
		t.Error("handshake succeeded against an empty trust store")
	}
}
