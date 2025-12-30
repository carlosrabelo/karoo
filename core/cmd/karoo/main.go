// Karoo (Go) - Stratum V1 Proxy
// Author: Carlos Rabelo <contato@carlosrabelo.com.br>

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/carlosrabelo/karoo/core/internal/proxy"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	cfgFile := flag.String("config", "config.json", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("karoo %s (built %s)\n", version, buildTime)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := loadConfig(*cfgFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Create proxy instance
	p := proxy.NewProxy(cfg)

	// Setup context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Start HTTP server if enabled
	if cfg.HTTP.Listen != "" {
		go p.HttpServe(ctx)
	}

	// Start upstream manager
	go p.UpstreamManager(ctx, 30*time.Second)

	// Start VarDiff if enabled
	if cfg.VarDiff.Enabled {
		go p.VarDiffLoop(ctx)
	}

	// Start report loop
	go p.ReportLoop(ctx, 60*time.Second)

	// Start accept loop
	go func() {
		if err := p.AcceptLoop(ctx); err != nil {
			log.Printf("Accept loop error: %v", err)
			cancel()
		}
	}()

	// Wait for signal
	for {
		sig := <-sigCh
		if sig == syscall.SIGHUP {
			log.Printf("Received SIGHUP, reloading config...")
			newCfg, err := loadConfig(*cfgFile)
			if err != nil {
				log.Printf("Failed to reload config: %v", err)
				continue
			}
			p.Reload(newCfg)
			continue
		}

		// SIGINT/SIGTERM
		log.Printf("Shutting down...")
		cancel()
		time.Sleep(2 * time.Second)
		log.Printf("Shutdown complete")
		return
	}
}

func loadConfig(path string) (*proxy.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg proxy.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Set defaults if needed
	if cfg.Proxy.Listen == "" {
		cfg.Proxy.Listen = "0.0.0.0:3333"
	}
	if cfg.Proxy.MaxClients == 0 {
		cfg.Proxy.MaxClients = 1000
	}
	if cfg.Proxy.ReadBuf == 0 {
		cfg.Proxy.ReadBuf = 4096
	}
	if cfg.Proxy.WriteBuf == 0 {
		cfg.Proxy.WriteBuf = 4096
	}
	// Helper to set defaults and validate upstream config
	validateUpstream := func(u *proxy.UpstreamConfig) error {
		if u.Port == 0 {
			u.Port = 3333
		}
		if u.BackoffMinMs == 0 {
			u.BackoffMinMs = 1000
		}
		if u.BackoffMaxMs == 0 {
			u.BackoffMaxMs = 30000
		}

		if u.Host == "" {
			return fmt.Errorf("host is required")
		}
		if u.User == "" {
			return fmt.Errorf("user is required")
		}
		if u.BackoffMaxMs < u.BackoffMinMs {
			return fmt.Errorf("backoff_max_ms (%d) must be >= backoff_min_ms (%d)",
				u.BackoffMaxMs, u.BackoffMinMs)
		}
		return nil
	}

	// Set VarDiff defaults
	if cfg.VarDiff.MinDiff == 0 {
		cfg.VarDiff.MinDiff = 1
	}
	if cfg.VarDiff.MaxDiff == 0 {
		cfg.VarDiff.MaxDiff = 65536
	}
	if cfg.VarDiff.TargetSeconds == 0 {
		cfg.VarDiff.TargetSeconds = 15
	}
	if cfg.VarDiff.AdjustEveryMs == 0 {
		cfg.VarDiff.AdjustEveryMs = 60000
	}

	// Validate primary upstream
	if err := validateUpstream(&cfg.Upstream); err != nil {
		return nil, fmt.Errorf("upstream: %w", err)
	}

	// Validate backups
	for i := range cfg.Backups {
		if err := validateUpstream(&cfg.Backups[i]); err != nil {
			return nil, fmt.Errorf("backup[%d]: %w", i, err)
		}
	}

	return &cfg, nil
}
