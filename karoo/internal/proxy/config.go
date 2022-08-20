package proxy

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadConfig reads and validates the proxy configuration from path.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
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

func validateUpstream(u *UpstreamConfig) error {
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
