package proxy

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
)

type Proxy struct {
	mitm  *MITMProxy
	ca    *CA
	audit *AuditLogger
	log   zerolog.Logger
}

type ProxyConfig struct {
	Port         int
	Upstream     string
	CertDir      string
	AuditLogPath string
	DryRun       bool
}

func NewProxy(cfg ProxyConfig, log zerolog.Logger) (*Proxy, error) {
	ca, err := NewCA(cfg.CertDir)
	if err != nil {
		return nil, fmt.Errorf("ca: %w", err)
	}

	audit, err := NewAuditLogger(cfg.AuditLogPath, cfg.DryRun)
	if err != nil {
		return nil, fmt.Errorf("audit: %w", err)
	}

	mitm := NewMITMProxy(ca, audit, cfg.Upstream, cfg.Port, cfg.DryRun, log)

	return &Proxy{
		mitm:  mitm,
		ca:    ca,
		audit: audit,
		log:   log,
	}, nil
}

func (p *Proxy) Start(ctx context.Context) error {
	p.log.Info().Msg("MITM proxy + sanitizer starting")
	return p.mitm.Start(ctx)
}

func (p *Proxy) Close() {
	p.audit.Close()
}
