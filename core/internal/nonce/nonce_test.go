package nonce

import (
	"testing"
	"time"

	"github.com/carlosrabelo/karoo/core/internal/connection"
	"github.com/carlosrabelo/karoo/core/internal/stratum"
)

// mockClient implements the Client interface for testing
type mockClient struct {
	extraNoncePrefix string
	extraNonceTrim   int
	writeError       error
}

func (m *mockClient) GetExtraNoncePrefix() string { return m.extraNoncePrefix }
func (m *mockClient) GetExtraNonceTrim() int      { return m.extraNonceTrim }
func (m *mockClient) SetExtraNoncePrefix(p string) { m.extraNoncePrefix = p }
func (m *mockClient) SetExtraNonceTrim(t int)      { m.extraNonceTrim = t }
func (m *mockClient) WriteJSON(msg stratum.Message) error { return m.writeError }

func createTestUpstream() *connection.Upstream {
	cfg := &connection.Config{
		Proxy: struct {
			ReadBuf  int `json:"read_buf"`
			WriteBuf int `json:"write_buf"`
		}{
			ReadBuf:  4096,
			WriteBuf: 4096,
		},
	}
	up, err := connection.NewUpstream(cfg)
	if err != nil {
		panic(err)
	}
	return up
}

func TestNewManager(t *testing.T) {
	up := createTestUpstream()
	m := NewManager(up)

	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.up != up {
		t.Error("Upstream not set correctly")
	}
	if m.upReady.Load() {
		t.Error("Initial upstream ready state should be false")
	}
	if m.pendingSubs == nil {
		t.Error("Pending subscribes map not initialized")
	}
}

func TestUpstreamReady(t *testing.T) {
	up := createTestUpstream()
	m := NewManager(up)

	// Initially not ready
	if m.UpstreamReady() {
		t.Error("Should not be ready initially")
	}

	// Set upstream extranonce
	up.SetExtranonce("deadbeef", 4)
	m.SetUpstreamReady(true)

	if !m.UpstreamReady() {
		t.Error("Should be ready when upstream is configured")
	}
}

func TestEnqueuePendingSubscribe(t *testing.T) {
	up := createTestUpstream()
	m := NewManager(up)

	cl := &mockClient{}
	id := int64(123)

	// Test enqueue when not ready
	m.EnqueuePendingSubscribe(cl, &id)

	m.subMu.Lock()
	if len(m.pendingSubs) != 1 {
		t.Errorf("Expected 1 pending subscribe, got %d", len(m.pendingSubs))
	}
	if m.pendingSubs[cl] == nil {
		t.Error("Client not found in pending subscribes")
	}
	if *m.pendingSubs[cl] != id {
		t.Errorf("Expected ID %d, got %d", id, *m.pendingSubs[cl])
	}
	m.subMu.Unlock()
}

func TestRemovePendingSubscribe(t *testing.T) {
	up := createTestUpstream()
	m := NewManager(up)

	cl := &mockClient{}
	id := int64(123)

	// Add to pending
	m.EnqueuePendingSubscribe(cl, &id)

	// Remove
	m.RemovePendingSubscribe(cl)

	m.subMu.Lock()
	if len(m.pendingSubs) != 0 {
		t.Errorf("Expected 0 pending subscribes, got %d", len(m.pendingSubs))
	}
	m.subMu.Unlock()
}

func TestFlushPendingSubscribes(t *testing.T) {
	up := createTestUpstream()
	m := NewManager(up)

	cl1 := &mockClient{}
	cl2 := &mockClient{}

	id1 := int64(123)
	id2 := int64(456)

	// Add to pending
	m.EnqueuePendingSubscribe(cl1, &id1)
	m.EnqueuePendingSubscribe(cl2, &id2)

	// Verify they are pending
	m.subMu.Lock()
	if len(m.pendingSubs) != 2 {
		t.Errorf("Expected 2 pending subscribes, got %d", len(m.pendingSubs))
	}
	m.subMu.Unlock()

	// Set upstream as ready
	up.SetExtranonce("deadbeef", 4)
	m.SetUpstreamReady(true)

	// Flush (this will try to write to clients, but that's ok for this test)
	m.FlushPendingSubscribes()

	// All should be removed from pending
	m.subMu.Lock()
	if len(m.pendingSubs) != 0 {
		t.Errorf("Expected 0 pending subscribes after flush, got %d", len(m.pendingSubs))
	}
	m.subMu.Unlock()
}

func TestAssignNoncePrefix(t *testing.T) {
	up := createTestUpstream()
	m := NewManager(up)

	cl := &mockClient{}

	// Set upstream extranonce
	up.SetExtranonce("deadbeef", 8)

	// Assign prefix
	m.AssignNoncePrefix(cl)

	if cl.GetExtraNoncePrefix() == "" {
		t.Error("Extra nonce prefix not assigned")
	}
	if cl.GetExtraNonceTrim() == 0 {
		t.Error("Extra nonce trim not set")
	}

	// Test that calling again doesn't change
	oldPrefix := cl.GetExtraNoncePrefix()
	m.AssignNoncePrefix(cl)
	if cl.GetExtraNoncePrefix() != oldPrefix {
		t.Error("Extra nonce prefix should not change on second call")
	}
}

func TestGetClientExtranonce(t *testing.T) {
	up := createTestUpstream()
	m := NewManager(up)

	cl := &mockClient{}

	// Set upstream extranonce
	up.SetExtranonce("deadbeef", 8)

	// Test without client prefix
	ex1, ex2 := m.GetClientExtranonce(cl)
	if ex1 != "deadbeef" {
		t.Errorf("Expected extranonce1 'deadbeef', got '%s'", ex1)
	}
	if ex2 != 8 {
		t.Errorf("Expected extranonce2 size 8, got %d", ex2)
	}

	// Test with client prefix
	m.AssignNoncePrefix(cl)
	ex1, ex2 = m.GetClientExtranonce(cl)
	expectedEx1 := "deadbeef" + cl.GetExtraNoncePrefix()
	if ex1 != expectedEx1 {
		t.Errorf("Expected extranonce1 '%s', got '%s'", expectedEx1, ex1)
	}
	if ex2 != 7 { // 8 - 1 (trim)
		t.Errorf("Expected extranonce2 size 7, got %d", ex2)
	}
}

func TestSetUpstreamReady(t *testing.T) {
	up := createTestUpstream()
	m := NewManager(up)

	// Test setting ready
	m.SetUpstreamReady(true)
	if !m.upReady.Load() {
		t.Error("Upstream should be ready")
	}

	// Test setting not ready
	m.SetUpstreamReady(false)
	if m.upReady.Load() {
		t.Error("Upstream should not be ready")
	}
}

func TestGetReadyChannel(t *testing.T) {
	up := createTestUpstream()
	m := NewManager(up)

	ch := m.GetReadyChannel()
	if ch == nil {
		t.Error("Ready channel should not be nil")
	}

	// Channel should be open initially
	select {
	case <-ch:
		t.Error("Ready channel should not be closed initially")
	default:
		// Expected
	}

	// Set ready and check channel closes
	up.SetExtranonce("deadbeef", 4)
	m.SetUpstreamReady(true)

	// Wait a bit for async operation
	time.Sleep(10 * time.Millisecond)

	select {
	case <-ch:
		// Expected - channel should be closed
	default:
		t.Error("Ready channel should be closed when upstream is ready")
	}
}

func TestProcessSubscribeResult(t *testing.T) {
	up := createTestUpstream()
	m := NewManager(up)

	// Test valid result
	result := []interface{}{[]interface{}{}, "deadbeef", float64(4)}
	m.ProcessSubscribeResult(result)

	if !m.upReady.Load() {
		t.Error("Upstream should be ready after valid result")
	}

	ex1, ex2 := up.GetExtranonce()
	if ex1 != "deadbeef" {
		t.Errorf("Expected extranonce1 'deadbeef', got '%s'", ex1)
	}
	if ex2 != 4 {
		t.Errorf("Expected extranonce2 size 4, got %d", ex2)
	}

	// Reset and test invalid result
	m.Reset()
	result = []interface{}{[]interface{}{}, "deadbeef"} // Missing extranonce2_size
	m.ProcessSubscribeResult(result)

	if m.upReady.Load() {
		t.Error("Upstream should not be ready after invalid result")
	}
}

func TestReset(t *testing.T) {
	up := createTestUpstream()
	m := NewManager(up)

	// Set some state
	up.SetExtranonce("deadbeef", 4)
	m.SetUpstreamReady(true)

	cl := &mockClient{}
	m.EnqueuePendingSubscribe(cl, nil)

	// Reset
	m.Reset()

	// Verify reset
	if m.upReady.Load() {
		t.Error("Upstream ready should be false after reset")
	}
	if m.prefixCounter.Load() != 0 {
		t.Error("Prefix counter should be 0 after reset")
	}

	m.subMu.Lock()
	if len(m.pendingSubs) != 0 {
		t.Errorf("Pending subscribes should be empty after reset, got %d", len(m.pendingSubs))
	}
	m.subMu.Unlock()
}
