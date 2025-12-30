package proxysocks

import (
	"context"
	"testing"
	"time"
)

// TestNewProxyDialer_Disabled tests that a disabled proxy uses a direct net.Dialer
func TestNewProxyDialer_Disabled(t *testing.T) {
	cfg := &Config{
		Enabled: false,
	}

	dialer, err := NewProxyDialer(cfg)
	if err != nil {
		t.Fatalf("NewProxyDialer failed: %v", err)
	}

	if dialer == nil {
		t.Fatal("Expected non-nil dialer")
	}

	if dialer.IsEnabled() {
		t.Error("Proxy should not be enabled")
	}

	if dialer.GetType() != "" {
		t.Errorf("Expected empty type for disabled proxy, got %s", dialer.GetType())
	}

	if dialer.GetAddress() != "" {
		t.Errorf("Expected empty address for disabled proxy, got %s", dialer.GetAddress())
	}
}

// TestNewProxyDialer_SOCKS5_NoAuth tests SOCKS5 proxy without authentication
func TestNewProxyDialer_SOCKS5_NoAuth(t *testing.T) {
	cfg := &Config{
		Enabled:  true,
		Type:     "socks5",
		Host:     "127.0.0.1",
		Port:     1080,
		Username: "",
		Password: "",
	}

	dialer, err := NewProxyDialer(cfg)
	if err != nil {
		t.Fatalf("NewProxyDialer failed: %v", err)
	}

	if dialer == nil {
		t.Fatal("Expected non-nil dialer")
	}

	if !dialer.IsEnabled() {
		t.Error("Proxy should be enabled")
	}

	if dialer.GetType() != "socks5" {
		t.Errorf("Expected type 'socks5', got %s", dialer.GetType())
	}

	expectedAddr := "127.0.0.1:1080"
	if dialer.GetAddress() != expectedAddr {
		t.Errorf("Expected address %s, got %s", expectedAddr, dialer.GetAddress())
	}
}

// TestNewProxyDialer_SOCKS5_WithAuth tests SOCKS5 proxy with authentication
func TestNewProxyDialer_SOCKS5_WithAuth(t *testing.T) {
	cfg := &Config{
		Enabled:  true,
		Type:     "socks5",
		Host:     "127.0.0.1",
		Port:     1080,
		Username: "testuser",
		Password: "testpass",
	}

	dialer, err := NewProxyDialer(cfg)
	if err != nil {
		t.Fatalf("NewProxyDialer failed: %v", err)
	}

	if dialer == nil {
		t.Fatal("Expected non-nil dialer")
	}

	if !dialer.IsEnabled() {
		t.Error("Proxy should be enabled")
	}

	if dialer.GetType() != "socks5" {
		t.Errorf("Expected type 'socks5', got %s", dialer.GetType())
	}

	expectedAddr := "127.0.0.1:1080"
	if dialer.GetAddress() != expectedAddr {
		t.Errorf("Expected address %s, got %s", expectedAddr, dialer.GetAddress())
	}
}

// TestNewProxyDialer_SOCKS4_NotSupported tests that SOCKS4 is not supported
func TestNewProxyDialer_SOCKS4_NotSupported(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Type:    "socks4",
		Host:    "127.0.0.1",
		Port:    1080,
	}

	dialer, err := NewProxyDialer(cfg)
	if err == nil {
		t.Error("Expected error for SOCKS4 (not supported)")
	}

	if dialer != nil {
		t.Error("Expected nil dialer for unsupported proxy type")
	}
}

// TestNewProxyDialer_InvalidType tests invalid proxy type
func TestNewProxyDialer_InvalidType(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Type:    "invalid",
		Host:    "127.0.0.1",
		Port:    1080,
	}

	dialer, err := NewProxyDialer(cfg)
	if err == nil {
		t.Error("Expected error for invalid proxy type")
	}

	if dialer != nil {
		t.Error("Expected nil dialer for invalid config")
	}
}

// TestNewProxyDialer_MissingHost tests missing host configuration
func TestNewProxyDialer_MissingHost(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Type:    "socks5",
		Host:    "",
		Port:    1080,
	}

	dialer, err := NewProxyDialer(cfg)
	if err == nil {
		t.Error("Expected error for missing host")
	}

	if dialer != nil {
		t.Error("Expected nil dialer for invalid config")
	}
}

// TestNewProxyDialer_MissingPort tests missing port configuration
func TestNewProxyDialer_MissingPort(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		Type:    "socks5",
		Host:    "127.0.0.1",
		Port:    0,
	}

	dialer, err := NewProxyDialer(cfg)
	if err == nil {
		t.Error("Expected error for missing port")
	}

	if dialer != nil {
		t.Error("Expected nil dialer for invalid config")
	}
}

// TestProxyDialer_DialContext tests context support
func TestProxyDialer_DialContext(t *testing.T) {
	cfg := &Config{
		Enabled: false,
	}

	dialer, err := NewProxyDialer(cfg)
	if err != nil {
		t.Fatalf("NewProxyDialer failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Try to dial a non-existent address with a short timeout
	conn, err := dialer.DialContext(ctx, "tcp", "192.0.2.1:9999")
	if err == nil {
		defer func() { _ = conn.Close() }()
		t.Error("Expected error when dialing non-existent address")
	}
}

// TestProxyDialer_DialContext_Cancelled tests that context cancellation works
func TestProxyDialer_DialContext_Cancelled(t *testing.T) {
	cfg := &Config{
		Enabled: false,
	}

	dialer, err := NewProxyDialer(cfg)
	if err != nil {
		t.Fatalf("NewProxyDialer failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Try to dial with cancelled context
	conn, err := dialer.DialContext(ctx, "tcp", "192.0.2.1:9999")
	if err == nil {
		defer func() { _ = conn.Close() }()
		t.Error("Expected error when using cancelled context")
	}

	if err != context.Canceled {
		// Context error might be wrapped
		if ctx.Err() != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	}
}

// TestProxyDialer_GetType tests GetType method
func TestProxyDialer_GetType(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected string
	}{
		{
			name: "SOCKS5",
			config: Config{
				Enabled: true,
				Type:    "socks5",
				Host:    "127.0.0.1",
				Port:    1080,
			},
			expected: "socks5",
		},
		{
			name: "Disabled",
			config: Config{
				Enabled: false,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialer, err := NewProxyDialer(&tt.config)
			if err != nil {
				t.Fatalf("NewProxyDialer failed: %v", err)
			}

			if dialer.GetType() != tt.expected {
				t.Errorf("Expected type %s, got %s", tt.expected, dialer.GetType())
			}
		})
	}
}

// TestProxyDialer_GetAddress tests GetAddress method
func TestProxyDialer_GetAddress(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected string
	}{
		{
			name: "Enabled with standard port",
			config: Config{
				Enabled: true,
				Type:    "socks5",
				Host:    "127.0.0.1",
				Port:    1080,
			},
			expected: "127.0.0.1:1080",
		},
		{
			name: "Enabled with custom port",
			config: Config{
				Enabled: true,
				Type:    "socks5",
				Host:    "proxy.example.com",
				Port:    9050,
			},
			expected: "proxy.example.com:9050",
		},
		{
			name: "Disabled",
			config: Config{
				Enabled: false,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialer, err := NewProxyDialer(&tt.config)
			if err != nil {
				t.Fatalf("NewProxyDialer failed: %v", err)
			}

			if dialer.GetAddress() != tt.expected {
				t.Errorf("Expected address %s, got %s", tt.expected, dialer.GetAddress())
			}
		})
	}
}

// TestProxyDialer_IsEnabled tests IsEnabled method
func TestProxyDialer_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected bool
	}{
		{
			name: "Enabled SOCKS5",
			config: Config{
				Enabled: true,
				Type:    "socks5",
				Host:    "127.0.0.1",
				Port:    1080,
			},
			expected: true,
		},
		{
			name: "Disabled",
			config: Config{
				Enabled: false,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dialer, err := NewProxyDialer(&tt.config)
			if err != nil {
				t.Fatalf("NewProxyDialer failed: %v", err)
			}

			if dialer.IsEnabled() != tt.expected {
				t.Errorf("Expected IsEnabled %v, got %v", tt.expected, dialer.IsEnabled())
			}
		})
	}
}

// TestProxyDialer_Dial tests basic Dial method
func TestProxyDialer_Dial(t *testing.T) {
	cfg := &Config{
		Enabled: false,
	}

	dialer, err := NewProxyDialer(cfg)
	if err != nil {
		t.Fatalf("NewProxyDialer failed: %v", err)
	}

	// Try to dial a non-existent address
	conn, err := dialer.Dial("tcp", "192.0.2.1:9999")
	if err == nil {
		defer func() { _ = conn.Close() }()
		t.Error("Expected error when dialing non-existent address")
	}
}
