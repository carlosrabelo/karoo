package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/carlosrabelo/karoo/karoo/internal/proxy"
)

func run(cfgFile string) error {
	cfg, err := proxy.LoadConfig(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	p, err := proxy.NewProxy(cfg)
	if err != nil {
		return fmt.Errorf("create proxy: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	if cfg.HTTP.Listen != "" {
		go p.HttpServe(ctx)
	}

	go p.UpstreamManager(ctx, 30*time.Second)

	if cfg.VarDiff.Enabled {
		go p.VarDiffLoop(ctx)
	}

	go p.ReportLoop(ctx, 60*time.Second)

	go func() {
		if err := p.AcceptLoop(ctx); err != nil {
			log.Printf("Accept loop error: %v", err)
			cancel()
		}
	}()

	for {
		sig := <-sigCh
		if sig == syscall.SIGHUP {
			log.Printf("Received SIGHUP, reloading config...")
			newCfg, err := proxy.LoadConfig(cfgFile)
			if err != nil {
				log.Printf("Failed to reload config: %v", err)
				continue
			}
			if err := p.Reload(newCfg); err != nil {
				log.Printf("Failed to apply reload: %v", err)
			}
			continue
		}

		log.Printf("Shutting down...")
		cancel()
		p.CloseClients()
		time.Sleep(500 * time.Millisecond)
		log.Printf("Shutdown complete")
		return nil
	}
}
