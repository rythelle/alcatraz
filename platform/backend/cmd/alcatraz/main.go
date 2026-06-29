package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alcatraz/alcatraz/internal/proxy"
	"github.com/alcatraz/alcatraz/internal/shared"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg, err := shared.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	log := shared.NewLogger(cfg.LogLevel)
	log.Info().Msg("Alcatraz starting")

	log.Info().
		Int("proxy_port", cfg.ProxyPort).
		Bool("dry_run", cfg.DryRun).
		Msg("Configuration loaded")

	// MITM proxy + Data Guardian sanitizer
	proxyInstance, err := proxy.NewProxy(proxy.ProxyConfig{
		Port:         cfg.ProxyPort,
		Upstream:     "proxy-whitelist:3128",
		CertDir:      "/shared-certs",
		AuditLogPath: cfg.AuditLogPath,
		DryRun:       cfg.DryRun,
	}, log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create proxy")
	}
	defer proxyInstance.Close()

	if err := proxyInstance.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("Proxy failed")
	}

	log.Info().Msg("Alcatraz shutting down")
}
