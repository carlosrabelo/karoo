package metrics

import (
	"testing"
	"time"
)

func TestCollector(t *testing.T) {
	c := NewCollector()

	// Test initial state
	if c.IsUpstreamConnected() {
		t.Error("Initial upstream state should be false")
	}
	if c.GetClientsActive() != 0 {
		t.Error("Initial clients should be 0")
	}
	if c.GetSharesOK() != 0 {
		t.Error("Initial shares OK should be 0")
	}
	if c.GetSharesBad() != 0 {
		t.Error("Initial shares bad should be 0")
	}
	if c.GetTotalShares() != 0 {
		t.Error("Initial total shares should be 0")
	}
	if c.GetAcceptanceRate() != 0 {
		t.Error("Initial acceptance rate should be 0")
	}
}

func TestCollectorUpstream(t *testing.T) {
	c := NewCollector()

	// Test upstream connection
	c.SetUpstreamConnected(true)
	if !c.IsUpstreamConnected() {
		t.Error("Upstream should be connected")
	}

	c.SetUpstreamConnected(false)
	if c.IsUpstreamConnected() {
		t.Error("Upstream should be disconnected")
	}
}

func TestCollectorClients(t *testing.T) {
	c := NewCollector()

	// Test client increment/decrement
	c.IncrementClients()
	if c.GetClientsActive() != 1 {
		t.Error("Should have 1 client")
	}

	c.IncrementClients()
	if c.GetClientsActive() != 2 {
		t.Error("Should have 2 clients")
	}

	c.DecrementClients()
	if c.GetClientsActive() != 1 {
		t.Error("Should have 1 client")
	}

	c.DecrementClients()
	if c.GetClientsActive() != 0 {
		t.Error("Should have 0 clients")
	}
}

func TestCollectorShares(t *testing.T) {
	c := NewCollector()

	// Test shares increment
	c.IncrementSharesOK()
	if c.GetSharesOK() != 1 {
		t.Error("Should have 1 OK share")
	}

	c.IncrementSharesBad()
	if c.GetSharesBad() != 1 {
		t.Error("Should have 1 bad share")
	}

	c.IncrementSharesOK()
	c.IncrementSharesOK()
	if c.GetSharesOK() != 3 {
		t.Error("Should have 3 OK shares")
	}

	if c.GetTotalShares() != 4 {
		t.Error("Should have 4 total shares")
	}

	// Test acceptance rate
	rate := c.GetAcceptanceRate()
	expected := 75.0 // 3/4 * 100
	if rate != expected {
		t.Errorf("Acceptance rate = %v, want %v", rate, expected)
	}
}

func TestCollectorTiming(t *testing.T) {
	c := NewCollector()

	// Test last notify
	now := time.Now()
	c.SetLastNotify(now)
	retrieved := c.GetLastNotify()
	// Compare only seconds since we store Unix timestamp
	if retrieved.Unix() != now.Unix() {
		t.Errorf("Last notify time mismatch: got %v, want %v", retrieved.Unix(), now.Unix())
	}

	// Test last set difficulty
	c.SetLastSetDifficulty(1024)
	if c.GetLastSetDifficulty() != 1024 {
		t.Error("Last set difficulty mismatch")
	}
}

func TestCollectorSnapshot(t *testing.T) {
	c := NewCollector()

	// Set some values
	c.SetUpstreamConnected(true)
	c.IncrementClients()
	c.IncrementSharesOK()
	c.IncrementSharesBad()
	now := time.Now()
	c.SetLastNotify(now)
	c.SetLastSetDifficulty(512)

	// Take snapshot
	snap := c.Snapshot()

	// Verify snapshot
	if !snap.UpConnected {
		t.Error("Snapshot upstream should be connected")
	}
	if snap.ClientsActive != 1 {
		t.Error("Snapshot should have 1 client")
	}
	if snap.SharesOK != 1 {
		t.Error("Snapshot should have 1 OK share")
	}
	if snap.SharesBad != 1 {
		t.Error("Snapshot should have 1 bad share")
	}
	if snap.TotalShares != 2 {
		t.Error("Snapshot should have 2 total shares")
	}
	if snap.AcceptanceRate != 50.0 {
		t.Error("Snapshot acceptance rate should be 50%")
	}
	if snap.LastNotify.Unix() != now.Unix() {
		t.Errorf("Snapshot last notify time mismatch: got %v, want %v", snap.LastNotify.Unix(), now.Unix())
	}
	if snap.LastSetDifficulty != 512 {
		t.Error("Snapshot last set difficulty mismatch")
	}
}

func TestCollectorReset(t *testing.T) {
	c := NewCollector()

	// Set some values
	c.SetUpstreamConnected(true)
	c.IncrementClients()
	c.IncrementSharesOK()
	c.SetLastNotify(time.Now())
	c.SetLastSetDifficulty(1024)

	// Reset
	c.Reset()

	// Verify reset
	if c.IsUpstreamConnected() {
		t.Error("Upstream should be false after reset")
	}
	if c.GetClientsActive() != 0 {
		t.Error("Clients should be 0 after reset")
	}
	if c.GetSharesOK() != 0 {
		t.Error("Shares OK should be 0 after reset")
	}
	if c.GetSharesBad() != 0 {
		t.Error("Shares bad should be 0 after reset")
	}
	if c.GetTotalShares() != 0 {
		t.Error("Total shares should be 0 after reset")
	}
	if c.GetAcceptanceRate() != 0 {
		t.Error("Acceptance rate should be 0 after reset")
	}
}

func TestClientMetrics(t *testing.T) {
	cm := NewClientMetrics()

	// Test initial state
	if cm.GetOK() != 0 {
		t.Error("Initial OK should be 0")
	}
	if cm.GetBad() != 0 {
		t.Error("Initial bad should be 0")
	}
	if cm.GetTotal() != 0 {
		t.Error("Initial total should be 0")
	}
	if cm.GetAcceptanceRate() != 0 {
		t.Error("Initial acceptance rate should be 0")
	}
}

func TestClientMetricsIncrement(t *testing.T) {
	cm := NewClientMetrics()

	// Test increments
	cm.IncrementOK()
	cm.IncrementOK()
	cm.IncrementBad()

	if cm.GetOK() != 2 {
		t.Error("Should have 2 OK shares")
	}
	if cm.GetBad() != 1 {
		t.Error("Should have 1 bad share")
	}
	if cm.GetTotal() != 3 {
		t.Error("Should have 3 total shares")
	}

	// Test acceptance rate
	rate := cm.GetAcceptanceRate()
	expected := 66.66666666666666 // 2/3 * 100
	if rate != expected {
		t.Errorf("Acceptance rate = %v, want %v", rate, expected)
	}
}

func TestClientMetricsReset(t *testing.T) {
	cm := NewClientMetrics()

	// Set some values
	cm.IncrementOK()
	cm.IncrementBad()

	// Reset
	cm.Reset()

	// Verify reset
	if cm.GetOK() != 0 {
		t.Error("OK should be 0 after reset")
	}
	if cm.GetBad() != 0 {
		t.Error("Bad should be 0 after reset")
	}
	if cm.GetTotal() != 0 {
		t.Error("Total should be 0 after reset")
	}
	if cm.GetAcceptanceRate() != 0 {
		t.Error("Acceptance rate should be 0 after reset")
	}
}
