package proxy

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carlosrabelo/karoo/karoo/internal/connection"
	"github.com/carlosrabelo/karoo/karoo/internal/proxysocks"
	"github.com/carlosrabelo/karoo/karoo/internal/stratum"
)

func TestNewProxy(t *testing.T) {
	cfg := &Config{}
	p, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}

	if p == nil {
		t.Fatal("NewProxy returned nil")
	}

	// Test that proxy initializes with default values
	if p.up == nil {
		t.Error("Upstream not initialized")
	}
	if p.mx == nil {
		t.Error("Metrics collector not initialized")
	}
	if p.clients == nil {
		t.Error("Clients map not initialized")
	}
	if p.rt == nil {
		t.Error("Router not initialized")
	}
	if p.nm == nil {
		t.Error("Nonce manager not initialized")
	}
	if p.vd == nil {
		t.Error("VarDiff manager not initialized")
	}
}

func TestNewClient(t *testing.T) {
	cfg := &Config{
		Proxy: struct {
			Listen       string `json:"listen"`
			ClientIdleMs int    `json:"client_idle_ms"`
			MaxClients   int    `json:"max_clients"`
			ReadBuf      int    `json:"read_buf"`
			WriteBuf     int    `json:"write_buf"`
			TLS          struct {
				Enabled bool   `json:"enabled"`
				Cert    string `json:"cert_file"`
				Key     string `json:"key_file"`
			} `json:"tls"`
		}{
			ReadBuf:  4096,
			WriteBuf: 4096,
			TLS: struct {
				Enabled bool   `json:"enabled"`
				Cert    string `json:"cert_file"`
				Key     string `json:"key_file"`
			}{
				Enabled: false,
			},
		},
		Upstream: UpstreamConfig{
			User: "testuser",
			Pass: "testpass",
			SocksProxy: proxysocks.Config{
				Enabled: false,
			},
		},
	}

	// Create a mock connection
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	cl := NewClient(client, cfg)

	if cl == nil {
		t.Fatal("NewClient returned nil")
	}

	if cl.c != client {
		t.Error("Client connection not set correctly")
	}
	if cl.upUser != "testuser" {
		t.Errorf("Expected upstream user 'testuser', got '%s'", cl.upUser)
	}
	if cl.addr == "" {
		t.Error("Client address not set")
	}
	if cl.clientMetrics == nil {
		t.Error("Client metrics not initialized")
	}
}

// Tests for upstream ready, pending subscribes, and nonce assignment
// have been moved to their respective internal packages:
// - nonce.Manager tests in core/internal/nonce/nonce_test.go
// - connection.Upstream tests in core/internal/connection/connection_test.go

func TestUpstreamDial(t *testing.T) {
	connCfg := &connection.Config{
		Upstream: struct {
			Host               string            `json:"host"`
			Port               int               `json:"port"`
			User               string            `json:"user"`
			Pass               string            `json:"pass"`
			TLS                bool              `json:"tls"`
			InsecureSkipVerify bool              `json:"insecure_skip_verify"`
			SocksProxy         proxysocks.Config `json:"socks_proxy"`
		}{
			Host:       "test.pool.com",
			Port:       3333,
			User:       "test.user",
			Pass:       "test.pass",
			SocksProxy: proxysocks.Config{Enabled: false},
		},
		Proxy: struct {
			ReadBuf  int `json:"read_buf"`
			WriteBuf int `json:"write_buf"`
		}{
			ReadBuf:  4096,
			WriteBuf: 4096,
		},
	}

	up, err := connection.NewUpstream(connCfg)
	if err != nil {
		t.Fatalf("Failed to create upstream: %v", err)
	}
	ctx := context.Background()

	// Should fail to connect to non-existent server
	err = up.Dial(ctx)
	if err == nil {
		t.Error("Expected error when dialing non-existent server")
	}
}

func TestUpstreamClose(t *testing.T) {
	connCfg := &connection.Config{
		Proxy: struct {
			ReadBuf  int `json:"read_buf"`
			WriteBuf int `json:"write_buf"`
		}{
			ReadBuf:  4096,
			WriteBuf: 4096,
		},
	}

	up, err := connection.NewUpstream(connCfg)
	if err != nil {
		t.Fatalf("Failed to create upstream: %v", err)
	}

	// Close should not panic even when not connected
	up.Close()

	// After close, should not be connected
	if up.IsConnected() {
		t.Error("Should not be connected after close")
	}
}

func TestUpstreamIsConnected(t *testing.T) {
	connCfg := &connection.Config{
		Proxy: struct {
			ReadBuf  int `json:"read_buf"`
			WriteBuf int `json:"write_buf"`
		}{
			ReadBuf:  4096,
			WriteBuf: 4096,
		},
	}
	up, err := connection.NewUpstream(connCfg)
	if err != nil {
		t.Fatalf("Failed to create upstream: %v", err)
	}

	// Initially not connected
	if up.IsConnected() {
		t.Error("Should not be connected initially")
	}
}

func TestClientWriteOperations(t *testing.T) {
	cfg := &Config{
		Proxy: struct {
			Listen       string `json:"listen"`
			ClientIdleMs int    `json:"client_idle_ms"`
			MaxClients   int    `json:"max_clients"`
			ReadBuf      int    `json:"read_buf"`
			WriteBuf     int    `json:"write_buf"`
			TLS          struct {
				Enabled bool   `json:"enabled"`
				Cert    string `json:"cert_file"`
				Key     string `json:"key_file"`
			} `json:"tls"`
		}{
			ReadBuf:  4096,
			WriteBuf: 4096,
			TLS: struct {
				Enabled bool   `json:"enabled"`
				Cert    string `json:"cert_file"`
				Key     string `json:"key_file"`
			}{
				Enabled: false,
			},
		},
	}

	// Create a client with a closed connection to test error handling
	server, client := net.Pipe()
	_ = server.Close() // Close server side immediately
	cl := NewClient(client, cfg)

	// Test WriteLine with closed connection should return error
	err := cl.WriteLine("test line\n")
	if err == nil {
		t.Error("Expected error when writing to closed connection")
	}

	// Test WriteJSON with closed connection should return error
	msg := stratum.Message{
		Method: "test.method",
		Params: []interface{}{"param1", "param2"},
	}

	err = cl.WriteJSON(msg)
	if err == nil {
		t.Error("Expected error when writing JSON to closed connection")
	}

	_ = client.Close()
}

func TestClientAtomicOperations(t *testing.T) {
	cfg := &Config{}
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	cl := NewClient(client, cfg)

	// Test atomic operations
	atomic.StoreInt64(&cl.last, time.Now().UnixMilli())
	atomic.StoreUint64(&cl.ok, 10)
	atomic.StoreUint64(&cl.bad, 5)
	atomic.StoreInt64(&cl.diff, 1000)
	atomic.StoreUint32(&cl.handshakeDone, 1)

	if atomic.LoadUint64(&cl.ok) != 10 {
		t.Errorf("Expected ok=10, got %d", atomic.LoadUint64(&cl.ok))
	}
	if atomic.LoadUint64(&cl.bad) != 5 {
		t.Errorf("Expected bad=5, got %d", atomic.LoadUint64(&cl.bad))
	}
	if atomic.LoadInt64(&cl.diff) != 1000 {
		t.Errorf("Expected diff=1000, got %d", atomic.LoadInt64(&cl.diff))
	}
	if atomic.LoadUint32(&cl.handshakeDone) == 0 {
		t.Error("Expected handshakeDone=true")
	}
}

func TestBackoff(t *testing.T) {
	min := 100 * time.Millisecond
	max := 1000 * time.Millisecond

	// Test multiple calls to ensure variation
	for i := 0; i < 10; i++ {
		d := connection.Backoff(min, max)
		if d < min || d > max+250*time.Millisecond {
			t.Errorf("Backoff %v outside range [%v, %v]", d, min, max+250*time.Millisecond)
		}
	}
}

// Tests for diffFromBits and fmtDuration have been moved to:
// - core/internal/routing/routing_test.go (where these functions now reside)

func TestProxyMetricsIntegration(t *testing.T) {
	cfg := &Config{}
	p, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}

	// Test that metrics are properly initialized
	if atomic.LoadInt64(&p.mx.ClientsActive) != 0 {
		t.Errorf("Expected 0 active clients initially, got %d", atomic.LoadInt64(&p.mx.ClientsActive))
	}

	if p.mx.IsUpstreamConnected() != false {
		t.Error("Expected upstream not connected initially")
	}

	// Test atomic operations
	atomic.AddInt64(&p.mx.ClientsActive, 1)
	if atomic.LoadInt64(&p.mx.ClientsActive) != 1 {
		t.Errorf("Expected 1 active client after increment, got %d", atomic.LoadInt64(&p.mx.ClientsActive))
	}

	atomic.AddUint64(&p.mx.SharesOK, 5)
	atomic.AddUint64(&p.mx.SharesBad, 2)

	if atomic.LoadUint64(&p.mx.SharesOK) != 5 {
		t.Errorf("Expected 5 OK shares, got %d", atomic.LoadUint64(&p.mx.SharesOK))
	}

	if atomic.LoadUint64(&p.mx.SharesBad) != 2 {
		t.Errorf("Expected 2 bad shares, got %d", atomic.LoadUint64(&p.mx.SharesBad))
	}
}

func TestVarDiffLoop(t *testing.T) {
	cfg := &Config{
		VarDiff: struct {
			Enabled       bool `json:"enabled"`
			TargetSeconds int  `json:"target_seconds"`
			MinDiff       int  `json:"min_diff"`
			MaxDiff       int  `json:"max_diff"`
			AdjustEveryMs int  `json:"adjust_every_ms"`
		}{
			Enabled:       false,
			AdjustEveryMs: 1000,
		},
	}

	p, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should return immediately when disabled
	p.VarDiffLoop(ctx)

	// Test enabled case
	cfg.VarDiff.Enabled = true
	p2, err2 := NewProxy(cfg)
	if err2 != nil {
		t.Fatalf("NewProxy: %v", err2)
	}

	// Should run and be cancelled by context
	p2.VarDiffLoop(ctx)
}

// Test for difficulty adjustment has been moved to:
// - core/internal/vardiff/vardiff_test.go (where this functionality now resides)

func TestReportLoop(t *testing.T) {
	cfg := &Config{}
	p, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}

	// Test with zero interval (should return immediately)
	p.ReportLoop(context.Background(), 0)

	// Test with positive interval
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	p.ReportLoop(ctx, 50*time.Millisecond)
}

func TestUpstreamManager(t *testing.T) {
	cfg := &Config{}
	p, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should not panic even without real upstream loop
	p.UpstreamManager(ctx, 30*time.Second)
}
