// Package proxysocks provides SOCKS5 proxy support for Karoo
package proxysocks

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"

	"golang.org/x/net/proxy"
)

// Config holds SOCKS proxy configuration
type Config struct {
	Enabled  bool   `json:"enabled"`
	Type     string `json:"type"` // must be "socks5"
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"` // optional authentication
	Password string `json:"password"` // optional authentication
}

// ProxyDialer wraps SOCKS proxy functionality
type ProxyDialer struct {
	config *Config
	dialer proxy.Dialer
}

// NewProxyDialer creates a new SOCKS proxy dialer
func NewProxyDialer(config *Config) (*ProxyDialer, error) {
	if !config.Enabled {
		return &ProxyDialer{
			config: config,
			dialer: &net.Dialer{
				Timeout: 10 * time.Second,
			},
		}, nil
	}

	if config.Type != "socks5" {
		return nil, fmt.Errorf("unsupported proxy type: %s (must be 'socks5')", config.Type)
	}

	if config.Host == "" || config.Port == 0 {
		return nil, fmt.Errorf("proxy host and port are required when proxy is enabled")
	}

	proxyAddr := fmt.Sprintf("%s:%d", config.Host, config.Port)

	var dialer proxy.Dialer
	var err error

	// SOCKS5 proxy configuration
	authURL := &url.URL{
		Scheme: "socks5",
		Host:   proxyAddr,
	}

	// Add authentication if provided
	if config.Username != "" {
		authURL.User = url.UserPassword(config.Username, config.Password)
	}

	dialer, err = proxy.FromURL(authURL, proxy.Direct)

	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS proxy dialer: %w", err)
	}

	return &ProxyDialer{
		config: config,
		dialer: dialer,
	}, nil
}

// Dial creates a network connection using the configured proxy or direct connection
func (p *ProxyDialer) Dial(network, address string) (net.Conn, error) {
	return p.dialer.Dial(network, address)
}

// DialContext creates a network connection with context using the configured proxy
func (p *ProxyDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	// Check if the underlying dialer supports context
	if dialerCtx, ok := p.dialer.(interface {
		DialContext(context.Context, string, string) (net.Conn, error)
	}); ok {
		return dialerCtx.DialContext(ctx, network, address)
	}

	// Fallback for dialers that don't support context
	done := make(chan struct{})
	var conn net.Conn
	var err error

	go func() {
		conn, err = p.dialer.Dial(network, address)
		close(done)
	}()

	select {
	case <-done:
		return conn, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// IsEnabled returns true if SOCKS proxy is configured and enabled
func (p *ProxyDialer) IsEnabled() bool {
	return p.config.Enabled
}

// GetType returns the proxy type (socks5)
func (p *ProxyDialer) GetType() string {
	return p.config.Type
}

// GetAddress returns the proxy address
func (p *ProxyDialer) GetAddress() string {
	if !p.config.Enabled {
		return ""
	}
	return fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)
}
