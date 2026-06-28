package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type CA struct {
	cert     *x509.Certificate
	privKey  *ecdsa.PrivateKey
	certPEM  []byte
	keyPEM   []byte
	tlsCert  tls.Certificate
	once     sync.Once
	certPath string
	keyPath  string
}

func NewCA(certDir string) (*CA, error) {
	ca := &CA{
		certPath: filepath.Join(certDir, "ca-cert.pem"),
		keyPath:  filepath.Join(certDir, "ca-key.pem"),
	}

	if err := ca.loadOrGenerate(); err != nil {
		return nil, fmt.Errorf("ca: %w", err)
	}

	return ca, nil
}

func (ca *CA) loadOrGenerate() error {
	if _, err := os.Stat(ca.certPath); err == nil {
		return ca.load()
	}
	return ca.generate()
}

func (ca *CA) load() error {
	certPEM, err := os.ReadFile(ca.certPath)
	if err != nil {
		return fmt.Errorf("read cert: %w", err)
	}
	keyPEM, err := os.ReadFile(ca.keyPath)
	if err != nil {
		return fmt.Errorf("read key: %w", err)
	}

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("parse keypair: %w", err)
	}

	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return fmt.Errorf("parse cert: %w", err)
	}

	ca.cert = cert
	ca.privKey = tlsCert.PrivateKey.(*ecdsa.PrivateKey)
	ca.certPEM = certPEM
	ca.keyPEM = keyPEM
	ca.tlsCert = tlsCert
	return nil
}

func (ca *CA) generate() error {
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   "Alcatraz Data Guardian CA",
			Organization: []string{"Alcatraz"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privKey.PublicKey, privKey)
	if err != nil {
		return fmt.Errorf("create cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("keypair: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("parse cert: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(ca.certPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	if err := os.WriteFile(ca.certPath, certPEM, 0o644); err != nil {
		return fmt.Errorf("write cert: %w", err)
	}
	if err := os.WriteFile(ca.keyPath, keyPEM, 0o600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}

	ca.cert = cert
	ca.privKey = privKey
	ca.certPEM = certPEM
	ca.keyPEM = keyPEM
	ca.tlsCert = tlsCert
	return nil
}

func (ca *CA) GetTLSConfig() *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{ca.tlsCert},
		MinVersion:   tls.VersionTLS12,
	}
}

func (ca *CA) GenerateCertForHost(host string) (*tls.Certificate, error) {
	// Strip port if present — DNSNames must be bare hostnames for TLS verification.
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("serial: %w", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: host,
		},
		NotBefore:   time.Now().Add(-time.Hour),
		NotAfter:    time.Now().Add(24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{host},
	}

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("gen key: %w", err)
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &privKey.PublicKey, ca.privKey)
	if err != nil {
		return nil, fmt.Errorf("create cert: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("keypair: %w", err)
	}

	return &tlsCert, nil
}

func (ca *CA) CertPEM() []byte {
	return ca.certPEM
}
