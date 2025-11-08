package ratelimit

import (
	"net"
	"testing"
	"time"
)

func TestNewLimiter(t *testing.T) {
	cfg := &Config{
		Enabled:                 true,
		MaxConnectionsPerIP:     10,
		MaxConnectionsPerMinute: 60,
		BanDurationSeconds:      300,
		CleanupIntervalSeconds:  60,
	}

	l := NewLimiter(cfg)

	if l == nil {
		t.Fatal("NewLimiter returned nil")
	}
	if l.cfg != cfg {
		t.Error("Config not set correctly")
	}
	if l.stats == nil {
		t.Error("Stats map not initialized")
	}
}

func TestNewLimiterWithNilConfig(t *testing.T) {
	l := NewLimiter(nil)

	if l == nil {
		t.Fatal("NewLimiter returned nil")
	}
	if l.cfg == nil {
		t.Error("Default config not created")
	}
	if l.cfg.Enabled {
		t.Error("Default config should have Enabled = false")
	}
}

func TestAllowConnectionDisabled(t *testing.T) {
	cfg := &Config{
		Enabled: false,
	}

	l := NewLimiter(cfg)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345}

	// Should always allow when disabled
	for i := 0; i < 100; i++ {
		if !l.AllowConnection(addr) {
			t.Errorf("Connection %d should be allowed when limiter is disabled", i)
		}
	}
}

func TestMaxConnectionsPerIP(t *testing.T) {
	cfg := &Config{
		Enabled:                 true,
		MaxConnectionsPerIP:     5,
		MaxConnectionsPerMinute: 0, // Disable this limit
		BanDurationSeconds:      300,
		CleanupIntervalSeconds:  0,
	}

	l := NewLimiter(cfg)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345}

	// Should allow up to MaxConnectionsPerIP
	for i := 0; i < cfg.MaxConnectionsPerIP; i++ {
		if !l.AllowConnection(addr) {
			t.Errorf("Connection %d should be allowed", i+1)
		}
	}

	// Should reject the next connection
	if l.AllowConnection(addr) {
		t.Error("Connection should be rejected when limit exceeded")
	}

	// Release one connection
	l.ReleaseConnection(addr)

	// Should allow one more connection now
	if !l.AllowConnection(addr) {
		t.Error("Connection should be allowed after releasing one")
	}
}

func TestMaxConnectionsPerMinute(t *testing.T) {
	cfg := &Config{
		Enabled:                 true,
		MaxConnectionsPerIP:     0, // Disable this limit
		MaxConnectionsPerMinute: 5,
		BanDurationSeconds:      1, // Short ban for testing
		CleanupIntervalSeconds:  0,
	}

	l := NewLimiter(cfg)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.2"), Port: 12345}

	// Should allow up to MaxConnectionsPerMinute
	for i := 0; i < cfg.MaxConnectionsPerMinute; i++ {
		if !l.AllowConnection(addr) {
			t.Errorf("Connection %d should be allowed", i+1)
		}
		// Release immediately to not hit MaxConnectionsPerIP
		l.ReleaseConnection(addr)
	}

	// Should reject and ban
	if l.AllowConnection(addr) {
		t.Error("Connection should be rejected when per-minute limit exceeded")
	}

	// Verify IP is banned
	if !l.IsBanned(addr) {
		t.Error("IP should be banned after exceeding limit")
	}

	// Wait for both ban to expire AND connection times to age out of window
	// Need to wait 1 minute + ban duration for clean slate
	time.Sleep(1200 * time.Millisecond)

	// At this point ban is expired but connection times may still be in window
	// Verify ban is expired
	if l.IsBanned(addr) {
		t.Error("IP should not be banned after ban duration")
	}
}

func TestReleaseConnection(t *testing.T) {
	cfg := &Config{
		Enabled:                 true,
		MaxConnectionsPerIP:     3,
		MaxConnectionsPerMinute: 0,
		BanDurationSeconds:      300,
		CleanupIntervalSeconds:  0,
	}

	l := NewLimiter(cfg)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.3"), Port: 12345}

	// Add 3 connections
	for i := 0; i < 3; i++ {
		if !l.AllowConnection(addr) {
			t.Fatalf("Connection %d should be allowed", i+1)
		}
	}

	// Should be at limit
	if l.AllowConnection(addr) {
		t.Error("Should be at connection limit")
	}

	// Release all connections
	for i := 0; i < 3; i++ {
		l.ReleaseConnection(addr)
	}

	// Should allow new connection
	if !l.AllowConnection(addr) {
		t.Error("Connection should be allowed after releasing all")
	}
}

func TestIsBanned(t *testing.T) {
	cfg := &Config{
		Enabled:                 true,
		MaxConnectionsPerIP:     0,
		MaxConnectionsPerMinute: 2,
		BanDurationSeconds:      1,
		CleanupIntervalSeconds:  0,
	}

	l := NewLimiter(cfg)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.4"), Port: 12345}

	// Not banned initially
	if l.IsBanned(addr) {
		t.Error("IP should not be banned initially")
	}

	// Exceed limit to trigger ban
	for i := 0; i < 3; i++ {
		l.AllowConnection(addr)
		l.ReleaseConnection(addr)
	}

	// Should be banned now
	if !l.IsBanned(addr) {
		t.Error("IP should be banned after exceeding limit")
	}

	// Wait for ban to expire
	time.Sleep(1200 * time.Millisecond)

	// Should not be banned anymore
	if l.IsBanned(addr) {
		t.Error("IP should not be banned after expiry")
	}
}

func TestGetStats(t *testing.T) {
	cfg := &Config{
		Enabled:                 true,
		MaxConnectionsPerIP:     10,
		MaxConnectionsPerMinute: 60,
		BanDurationSeconds:      300,
		CleanupIntervalSeconds:  0,
	}

	l := NewLimiter(cfg)
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.5"), Port: 12345}

	// Get stats for unknown IP
	stats := l.GetStats(addr)
	if stats == nil {
		t.Fatal("GetStats returned nil")
	}
	if stats["active_connections"] != 0 {
		t.Error("Active connections should be 0 for new IP")
	}

	// Add connections
	l.AllowConnection(addr)
	l.AllowConnection(addr)

	stats = l.GetStats(addr)
	if stats["active_connections"] != 2 {
		t.Errorf("Expected 2 active connections, got %v", stats["active_connections"])
	}
	if stats["connections_in_minute"] != 2 {
		t.Errorf("Expected 2 connections in minute, got %v", stats["connections_in_minute"])
	}
}

func TestGetGlobalStats(t *testing.T) {
	cfg := &Config{
		Enabled:                 true,
		MaxConnectionsPerIP:     10,
		MaxConnectionsPerMinute: 60,
		BanDurationSeconds:      300,
		CleanupIntervalSeconds:  0,
	}

	l := NewLimiter(cfg)

	addr1 := &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: 12345}
	addr2 := &net.TCPAddr{IP: net.ParseIP("192.168.1.11"), Port: 12345}

	l.AllowConnection(addr1)
	l.AllowConnection(addr2)
	l.AllowConnection(addr2)

	stats := l.GetGlobalStats()
	if stats == nil {
		t.Fatal("GetGlobalStats returned nil")
	}

	if stats["total_ips"] != 2 {
		t.Errorf("Expected 2 total IPs, got %v", stats["total_ips"])
	}
	if stats["total_active"] != 3 {
		t.Errorf("Expected 3 total active, got %v", stats["total_active"])
	}
	if stats["max_per_ip"] != 10 {
		t.Errorf("Expected max_per_ip 10, got %v", stats["max_per_ip"])
	}
}

func TestCleanup(t *testing.T) {
	cfg := &Config{
		Enabled:                 true,
		MaxConnectionsPerIP:     10,
		MaxConnectionsPerMinute: 60,
		BanDurationSeconds:      0,
		CleanupIntervalSeconds:  0,
	}

	l := NewLimiter(cfg)

	// Add and release connections
	addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.20"), Port: 12345}
	l.AllowConnection(addr)
	l.ReleaseConnection(addr)

	// Manually set old timestamp
	l.mu.Lock()
	if stats, exists := l.stats["192.168.1.20"]; exists {
		stats.mu.Lock()
		stats.connectionTimes[0] = time.Now().Add(-10 * time.Minute)
		stats.mu.Unlock()
	}
	l.mu.Unlock()

	// Run cleanup
	l.cleanup()

	// IP should be removed
	l.mu.RLock()
	_, exists := l.stats["192.168.1.20"]
	l.mu.RUnlock()

	if exists {
		t.Error("Old entry should be cleaned up")
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		name     string
		addr     net.Addr
		expected string
	}{
		{
			name:     "TCPAddr",
			addr:     &net.TCPAddr{IP: net.ParseIP("192.168.1.1"), Port: 12345},
			expected: "192.168.1.1",
		},
		{
			name:     "UDPAddr",
			addr:     &net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 12345},
			expected: "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := extractIP(tt.addr)
			if ip != tt.expected {
				t.Errorf("Expected IP %s, got %s", tt.expected, ip)
			}
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	cfg := &Config{
		Enabled:                 true,
		MaxConnectionsPerIP:     100,
		MaxConnectionsPerMinute: 1000,
		BanDurationSeconds:      60,
		CleanupIntervalSeconds:  0,
	}

	l := NewLimiter(cfg)

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			addr := &net.TCPAddr{IP: net.ParseIP("192.168.1.100"), Port: 12345 + id}
			for j := 0; j < 50; j++ {
				l.AllowConnection(addr)
				l.GetStats(addr)
				l.ReleaseConnection(addr)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic and should have stats
	stats := l.GetGlobalStats()
	if stats == nil {
		t.Error("GetGlobalStats returned nil after concurrent access")
	}
}
