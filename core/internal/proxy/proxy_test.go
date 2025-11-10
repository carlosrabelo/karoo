package proxy

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/carlosrabelo/karoo/core/internal/connection"
	"github.com/carlosrabelo/karoo/core/internal/proxysocks"
	"github.com/carlosrabelo/karoo/core/internal/stratum"
)

func TestNewProxy(t *testing.T) {
	cfg := &Config{}
	p := NewProxy(cfg)

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
		}{
			ReadBuf:  4096,
			WriteBuf: 4096,
		},
		Upstream: struct {
			Host               string `json:"host"`
			Port               int    `json:"port"`
			User               string `json:"user"`
			Pass               string `json:"pass"`
			TLS                bool   `json:"tls"`
			InsecureSkipVerify bool   `json:"insecure_skip_verify"`
			BackoffMinMs       int    `json:"backoff_min_ms"`
			BackoffMaxMs       int    `json:"backoff_max_ms"`
			SocksProxy         struct {
				Enabled  bool   `json:"enabled"`
				Type     string `json:"type"`
				Host     string `json:"host"`
				Port     int    `json:"port"`
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"socks_proxy"`
		}{
			User: "testuser",
			Pass: "testpass",
			SocksProxy: struct {
				Enabled  bool   `json:"enabled"`
				Type     string `json:"type"`
				Host     string `json:"host"`
				Port     int    `json:"port"`
				Username string `json:"username"`
				Password string `json:"password"`
			}{
				Enabled: false,
			},
		},
	}

	// Create a mock connection
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

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
		}{
			ReadBuf:  4096,
			WriteBuf: 4096,
		},
	}

	// Create a client with a closed connection to test error handling
	server, client := net.Pipe()
	server.Close() // Close server side immediately
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

	client.Close()
}

func TestClientAtomicOperations(t *testing.T) {
	cfg := &Config{}
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	cl := NewClient(client, cfg)

	// Test atomic operations
	cl.last.Store(time.Now().UnixMilli())
	cl.ok.Store(10)
	cl.bad.Store(5)
	cl.diff.Store(1000)
	cl.handshakeDone.Store(true)

	if cl.ok.Load() != 10 {
		t.Errorf("Expected ok=10, got %d", cl.ok.Load())
	}
	if cl.bad.Load() != 5 {
		t.Errorf("Expected bad=5, got %d", cl.bad.Load())
	}
	if cl.diff.Load() != 1000 {
		t.Errorf("Expected diff=1000, got %d", cl.diff.Load())
	}
	if !cl.handshakeDone.Load() {
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
	p := NewProxy(cfg)

	// Test that metrics are properly initialized
	if p.mx.ClientsActive.Load() != 0 {
		t.Errorf("Expected 0 active clients initially, got %d", p.mx.ClientsActive.Load())
	}

	if p.mx.UpConnected.Load() != false {
		t.Error("Expected upstream not connected initially")
	}

	// Test atomic operations
	p.mx.ClientsActive.Add(1)
	if p.mx.ClientsActive.Load() != 1 {
		t.Errorf("Expected 1 active client after increment, got %d", p.mx.ClientsActive.Load())
	}

	p.mx.SharesOK.Add(5)
	p.mx.SharesBad.Add(2)

	if p.mx.SharesOK.Load() != 5 {
		t.Errorf("Expected 5 OK shares, got %d", p.mx.SharesOK.Load())
	}

	if p.mx.SharesBad.Load() != 2 {
		t.Errorf("Expected 2 bad shares, got %d", p.mx.SharesBad.Load())
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

	p := NewProxy(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should return immediately when disabled
	p.VarDiffLoop(ctx)

	// Test enabled case
	cfg.VarDiff.Enabled = true
	p2 := NewProxy(cfg)

	// Should run and be cancelled by context
	p2.VarDiffLoop(ctx)
}

// Test for difficulty adjustment has been moved to:
// - core/internal/vardiff/vardiff_test.go (where this functionality now resides)

func TestReportLoop(t *testing.T) {
	cfg := &Config{}
	p := NewProxy(cfg)

	// Test with zero interval (should return immediately)
	p.ReportLoop(context.Background(), 0)

	// Test with positive interval
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	p.ReportLoop(ctx, 50*time.Millisecond)
}

func TestUpstreamManager(t *testing.T) {
	cfg := &Config{}
	p := NewProxy(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should not panic even without real upstream loop
	p.UpstreamManager(ctx, 30*time.Second)
}
