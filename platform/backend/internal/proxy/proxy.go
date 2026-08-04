package proxy

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alcatraz/alcatraz/internal/rules"
	"github.com/rs/zerolog"
)

type Proxy struct {
	mitm  *MITMProxy
	ca    *CA
	audit *AuditLogger
	stats *StatsLogger
	rules *rules.Watcher
	vault *Vault
	log   zerolog.Logger
}

// vaultEnabled reports whether reversible tokenization is on. It defaults to ON
// (global): the response path decompresses gzip and reassembles model text
// across SSE deltas, so a token the model echoes — even split into fragments by
// SSE framing — is restored reliably (verified end-to-end against a live model).
// Disable with ALCATRAZ_VAULT=0 to fall back to destructive [REDACTED] markers.
func vaultEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("ALCATRAZ_VAULT"))) {
	case "0", "false", "off", "no":
		return false
	}
	return true
}

type ProxyConfig struct {
	Port         int
	Upstream     string
	CertDir      string
	AuditLogPath string
	StatsLogPath string
	RulesPath    string
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

	// Telemetry is best-effort: a broken stats file must never take the
	// proxy (and with it all AI traffic) down.
	stats, err := NewStatsLogger(cfg.StatsLogPath)
	if err != nil {
		log.Warn().Err(err).Msg("token stats disabled")
		stats = nil
	}

	rulesWatcher := rules.NewWatcher(cfg.RulesPath, log)

	var vault *Vault
	if vaultEnabled() {
		vault = NewVault(0)
		log.Info().Int("max", DefaultVaultMax).Msg("reversible tokenization (vault) enabled")
	}

	mitm := NewMITMProxy(ca, audit, stats, rulesWatcher, vault, cfg.Upstream, cfg.Port, cfg.DryRun, log)

	return &Proxy{
		mitm:  mitm,
		ca:    ca,
		audit: audit,
		stats: stats,
		rules: rulesWatcher,
		vault: vault,
		log:   log,
	}, nil
}

func (p *Proxy) Start(ctx context.Context) error {
	p.log.Info().Msg("MITM proxy + sanitizer starting")
	go func() {
		if err := p.rules.Start(ctx); err != nil {
			p.log.Error().Err(err).Msg("guard rules watcher stopped")
		}
	}()
	return p.mitm.Start(ctx)
}

func (p *Proxy) Close() {
	p.audit.Close()
	if p.stats != nil {
		p.stats.Close()
	}
}
