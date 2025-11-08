package connection

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/carlosrabelo/karoo/core/internal/stratum"
)

func TestNewUpstream(t *testing.T) {
	cfg := &Config{}
	u := NewUpstream(cfg)

	if u == nil {
		t.Fatal("NewUpstream returned nil")
	}
	if u.cfg != cfg {
		t.Error("Config not set correctly")
	}
	if u.pending == nil {
		t.Error("Pending requests map not initialized")
	}
}

func TestNewDownstream(t *testing.T) {
	cfg := &Config{
		Proxy: struct {
			ReadBuf  int `json:"read_buf"`
			WriteBuf int `json:"write_buf"`
		}{
			ReadBuf:  4096,
			WriteBuf: 4096,
		},
	}

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	d := NewDownstream(client, cfg)

	if d == nil {
		t.Fatal("NewDownstream returned nil")
	}
	if d.Conn != client {
		t.Error("Connection not set correctly")
	}
	if d.Reader == nil {
		t.Error("Reader not initialized")
	}
	if d.Writer == nil {
		t.Error("Writer not initialized")
	}
	if d.Addr == "" {
		t.Error("Address not set")
	}
}

func TestUpstreamDial(t *testing.T) {
	cfg := &Config{
		Upstream: struct {
			Host               string `json:"host"`
			Port               int    `json:"port"`
			User               string `json:"user"`
			Pass               string `json:"pass"`
			TLS                bool   `json:"tls"`
			InsecureSkipVerify bool   `json:"insecure_skip_verify"`
		}{
			Host: "127.0.0.1",
			Port: 9999, // Non-existent port
		},
		Proxy: struct {
			ReadBuf  int `json:"read_buf"`
			WriteBuf int `json:"write_buf"`
		}{
			ReadBuf:  4096,
			WriteBuf: 4096,
		},
	}

	u := NewUpstream(cfg)
	ctx := context.Background()

	// Should fail to connect to non-existent server
	err := u.Dial(ctx)
	if err == nil {
		t.Error("Expected error when dialing non-existent server")
	}
}

func TestUpstreamClose(t *testing.T) {
	cfg := &Config{}
	u := NewUpstream(cfg)

	// Close should not panic even when not connected
	u.Close()

	if u.conn != nil {
		t.Error("Connection should be nil after close")
	}
}

func TestUpstreamIsConnected(t *testing.T) {
	cfg := &Config{}
	u := NewUpstream(cfg)

	// Initially not connected
	if u.IsConnected() {
		t.Error("Should not be connected initially")
	}
}

func TestUpstreamExtranonce(t *testing.T) {
	cfg := &Config{}
	u := NewUpstream(cfg)

	// Test initial state
	ex1, ex2 := u.GetExtranonce()
	if ex1 != "" {
		t.Error("Initial extranonce1 should be empty")
	}
	if ex2 != 0 {
		t.Error("Initial extranonce2 size should be 0")
	}

	// Test setting extranonce
	u.SetExtranonce("deadbeef", 4)
	ex1, ex2 = u.GetExtranonce()
	if ex1 != "deadbeef" {
		t.Errorf("Expected extranonce1 'deadbeef', got '%s'", ex1)
	}
	if ex2 != 4 {
		t.Errorf("Expected extranonce2 size 4, got %d", ex2)
	}
}

func TestUpstreamPendingRequests(t *testing.T) {
	cfg := &Config{}
	u := NewUpstream(cfg)

	// Test adding and removing pending requests
	req := PendingReq{
		Method: "test.method",
		Sent:   time.Now(),
	}

	// Add request
	u.AddPendingRequest(123, req)

	// Remove request
	retrieved, exists := u.RemovePendingRequest(123)
	if !exists {
		t.Error("Request should exist")
	}
	if retrieved.Method != req.Method {
		t.Errorf("Expected method '%s', got '%s'", req.Method, retrieved.Method)
	}

	// Try to remove non-existent request
	_, exists = u.RemovePendingRequest(456)
	if exists {
		t.Error("Non-existent request should not exist")
	}
}

func TestUpstreamSend(t *testing.T) {
	cfg := &Config{}
	u := NewUpstream(cfg)

	// Test send when not connected
	msg := stratum.Message{
		Method: "test.method",
		Params: []interface{}{"param1"},
	}

	_, err := u.Send(msg)
	if err == nil {
		t.Error("Expected error when sending to disconnected upstream")
	}
}

func TestBackoff(t *testing.T) {
	min := 100 * time.Millisecond
	max := 1000 * time.Millisecond

	// Test multiple calls to ensure variation
	for i := 0; i < 10; i++ {
		d := Backoff(min, max)
		if d < min || d > max+250*time.Millisecond {
			t.Errorf("Backoff %v outside range [%v, %v]", d, min, max+250*time.Millisecond)
		}
	}

	// Test when min == max
	d := Backoff(min, min)
	if d < min || d > min+250*time.Millisecond {
		t.Errorf("Backoff %v outside range [%v, %v]", d, min, min+250*time.Millisecond)
	}
}
