package vardiff

import (
	"context"
	"testing"
	"time"

	"github.com/carlosrabelo/karoo/core/internal/stratum"
)

// mockClient implements the Client interface for testing
type mockClient struct {
	writeError error
	messages   []stratum.Message
}

func (m *mockClient) WriteJSON(msg stratum.Message) error {
	m.messages = append(m.messages, msg)
	return m.writeError
}

func TestNewManager(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		TargetSeconds: 15,
		MinDiff:       1000,
		MaxDiff:       100000,
		AdjustEveryMs: 60000,
	}

	mgr := NewManager(cfg)

	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}
	if mgr.cfg != cfg {
		t.Error("Config not set correctly")
	}
	if mgr.clients == nil {
		t.Error("Clients map not initialized")
	}
}

func TestAddRemoveClient(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		TargetSeconds: 15,
		MinDiff:       1000,
		MaxDiff:       100000,
		AdjustEveryMs: 60000,
	}

	mgr := NewManager(cfg)
	cl := &mockClient{}

	mgr.AddClient(cl)

	mgr.clientsMu.RLock()
	if len(mgr.clients) != 1 {
		t.Errorf("Expected 1 client, got %d", len(mgr.clients))
	}
	stats, exists := mgr.clients[cl]
	mgr.clientsMu.RUnlock()

	if !exists {
		t.Error("Client not found in manager")
	}
	if stats == nil {
		t.Fatal("Client stats not initialized")
	}
	if stats.CurrentDifficulty != float64(cfg.MinDiff) {
		t.Errorf("Expected initial difficulty %d, got %f", cfg.MinDiff, stats.CurrentDifficulty)
	}

	mgr.RemoveClient(cl)

	mgr.clientsMu.RLock()
	if len(mgr.clients) != 0 {
		t.Errorf("Expected 0 clients after removal, got %d", len(mgr.clients))
	}
	mgr.clientsMu.RUnlock()
}

func TestRecordShare(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		TargetSeconds: 15,
		MinDiff:       1000,
		MaxDiff:       100000,
		AdjustEveryMs: 60000,
	}

	mgr := NewManager(cfg)
	cl := &mockClient{}

	mgr.AddClient(cl)
	mgr.RecordShare(cl, true, 1000)

	mgr.clientsMu.RLock()
	stats := mgr.clients[cl]
	mgr.clientsMu.RUnlock()

	stats.mu.Lock()
	if len(stats.ShareWindow) != 1 {
		t.Errorf("Expected 1 share in window, got %d", len(stats.ShareWindow))
	}
	if !stats.ShareWindow[0].Accepted {
		t.Error("Share should be marked as accepted")
	}
	if stats.ShareWindow[0].Difficulty != 1000 {
		t.Errorf("Expected difficulty 1000, got %f", stats.ShareWindow[0].Difficulty)
	}
	stats.mu.Unlock()
}

func TestShareWindowBounding(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		TargetSeconds: 15,
		MinDiff:       1000,
		MaxDiff:       100000,
		AdjustEveryMs: 60000,
	}

	mgr := NewManager(cfg)
	cl := &mockClient{}

	mgr.AddClient(cl)

	// Add more than maxShareWindowSize shares
	for i := 0; i < 150; i++ {
		mgr.RecordShare(cl, true, 1000)
	}

	mgr.clientsMu.RLock()
	stats := mgr.clients[cl]
	mgr.clientsMu.RUnlock()

	stats.mu.Lock()
	windowSize := len(stats.ShareWindow)
	stats.mu.Unlock()

	if windowSize > maxShareWindowSize {
		t.Errorf("Share window exceeded max size: got %d, max %d", windowSize, maxShareWindowSize)
	}
}

func TestRun(t *testing.T) {
	cfg := &Config{
		Enabled:       false,
		TargetSeconds: 15,
		MinDiff:       1000,
		MaxDiff:       100000,
		AdjustEveryMs: 100, // Short for testing
	}

	mgr := NewManager(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Should return immediately when disabled
	mgr.Run(ctx)
}

func TestRunEnabled(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		TargetSeconds: 15,
		MinDiff:       1000,
		MaxDiff:       100000,
		AdjustEveryMs: 50, // Short for testing
	}

	mgr := NewManager(cfg)
	cl := &mockClient{}
	mgr.AddClient(cl)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	// Should run and cancel on context done
	mgr.Run(ctx)
}

func TestGetStats(t *testing.T) {
	cfg := &Config{
		Enabled:       true,
		TargetSeconds: 15,
		MinDiff:       1000,
		MaxDiff:       100000,
		AdjustEveryMs: 60000,
	}

	mgr := NewManager(cfg)
	cl1 := &mockClient{}
	cl2 := &mockClient{}

	mgr.AddClient(cl1)
	mgr.AddClient(cl2)

	stats := mgr.GetStats()

	// Stats is a map, just verify it's not empty
	if stats == nil {
		t.Error("GetStats returned nil")
	}

	// Verify we have stats (the actual format is map[string]interface{})
	if clientsCount, ok := stats["clients_count"]; ok {
		if count, ok := clientsCount.(int); ok && count != 2 {
			t.Errorf("Expected 2 clients, got %d", count)
		}
	}
}
