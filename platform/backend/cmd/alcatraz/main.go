package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/alcatraz/alcatraz/internal/proxy"
	"github.com/alcatraz/alcatraz/internal/rules"
	"github.com/alcatraz/alcatraz/internal/shared"
)

func main() {
	check := flag.Bool("check", false, "read a payload from stdin, write the sanitized result to stdout, and exit (uses the same engine and rules file as the live proxy)")
	stats := flag.Bool("stats", false, "print the aggregated token-usage report and exit (used by './alcatraz.sh stats' via docker exec)")
	flag.Parse()

	cfg, err := shared.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if *check {
		os.Exit(runCheck(cfg))
	}

	if *stats {
		if err := proxy.PrintStats(cfg.StatsLogPath, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "stats: %v\n", err)
			os.Exit(1)
		}
		return
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log := shared.NewLogger(cfg.LogLevel)
	log.Info().Msg("Alcatraz starting")

	log.Info().
		Int("proxy_port", cfg.ProxyPort).
		Bool("dry_run", cfg.DryRun).
		Msg("Configuration loaded")

	// MITM proxy + Guard sanitizer
	proxyInstance, err := proxy.NewProxy(proxy.ProxyConfig{
		Port:         cfg.ProxyPort,
		Upstream:     "lighthouse:3128",
		CertDir:      "/shared-certs",
		AuditLogPath: cfg.AuditLogPath,
		StatsLogPath: cfg.StatsLogPath,
		RulesPath:    cfg.GuardRulesPath,
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

// runCheck implements the `-check` flag: sanitize stdin using the exact
// production engine and the current rules file, then write the result to
// stdout. Used by the CLI/TUI `guard test` command (via docker exec) so
// tests exercise the same code path as live traffic. Returns a process exit
// code.
func runCheck(cfg *shared.Config) int {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
		return 1
	}
	rs, err := rules.Load(cfg.GuardRulesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "guard rules invalid: %v\n", err)
		return 1
	}
	result := proxy.SanitizeJSONWithRules(string(input), rs, false)
	fmt.Print(result.Output)
	return 0
}
