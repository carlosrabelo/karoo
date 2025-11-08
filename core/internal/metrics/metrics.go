// Package metrics provides collection and reporting of proxy metrics
package metrics

import (
	"sync/atomic"
	"time"
)

// Collector holds all proxy metrics
type Collector struct {
	// Connection metrics
	UpConnected   atomic.Bool
	ClientsActive atomic.Int64

	// Share metrics
	SharesOK  atomic.Uint64
	SharesBad atomic.Uint64

	// Timing metrics
	LastNotifyUnix atomic.Int64
	LastSetDiff    atomic.Int64
}

// NewCollector creates a new metrics collector
func NewCollector() *Collector {
	return &Collector{}
}

// SetUpstreamConnected sets the upstream connection status
func (m *Collector) SetUpstreamConnected(connected bool) {
	m.UpConnected.Store(connected)
}

// IsUpstreamConnected returns the upstream connection status
func (m *Collector) IsUpstreamConnected() bool {
	return m.UpConnected.Load()
}

// IncrementClients increments the active client count
func (m *Collector) IncrementClients() {
	m.ClientsActive.Add(1)
}

// DecrementClients decrements the active client count
func (m *Collector) DecrementClients() {
	m.ClientsActive.Add(-1)
}

// GetClientsActive returns the current number of active clients
func (m *Collector) GetClientsActive() int64 {
	return m.ClientsActive.Load()
}

// IncrementSharesOK increments the accepted shares counter
func (m *Collector) IncrementSharesOK() {
	m.SharesOK.Add(1)
}

// IncrementSharesBad increments the rejected shares counter
func (m *Collector) IncrementSharesBad() {
	m.SharesBad.Add(1)
}

// GetSharesOK returns the total accepted shares
func (m *Collector) GetSharesOK() uint64 {
	return m.SharesOK.Load()
}

// GetSharesBad returns the total rejected shares
func (m *Collector) GetSharesBad() uint64 {
	return m.SharesBad.Load()
}

// GetTotalShares returns the total shares (accepted + rejected)
func (m *Collector) GetTotalShares() uint64 {
	return m.SharesOK.Load() + m.SharesBad.Load()
}

// SetLastNotify updates the last notification timestamp
func (m *Collector) SetLastNotify(t time.Time) {
	m.LastNotifyUnix.Store(t.Unix())
}

// GetLastNotify returns the last notification timestamp
func (m *Collector) GetLastNotify() time.Time {
	unix := m.LastNotifyUnix.Load()
	return time.Unix(unix, 0)
}

// SetLastSetDifficulty updates the last set difficulty timestamp
func (m *Collector) SetLastSetDifficulty(difficulty int64) {
	m.LastSetDiff.Store(difficulty)
}

// GetLastSetDifficulty returns the last set difficulty
func (m *Collector) GetLastSetDifficulty() int64 {
	return m.LastSetDiff.Load()
}

// GetAcceptanceRate calculates the share acceptance rate as percentage
func (m *Collector) GetAcceptanceRate() float64 {
	total := m.GetTotalShares()
	if total == 0 {
		return 0
	}
	ok := m.GetSharesOK()
	return (float64(ok) / float64(total)) * 100
}

// Reset resets all metrics to zero values
func (m *Collector) Reset() {
	m.UpConnected.Store(false)
	m.ClientsActive.Store(0)
	m.SharesOK.Store(0)
	m.SharesBad.Store(0)
	m.LastNotifyUnix.Store(0)
	m.LastSetDiff.Store(0)
}

// Snapshot returns a snapshot of current metrics
func (m *Collector) Snapshot() Snapshot {
	return Snapshot{
		UpConnected:       m.IsUpstreamConnected(),
		ClientsActive:     m.GetClientsActive(),
		SharesOK:          m.GetSharesOK(),
		SharesBad:         m.GetSharesBad(),
		TotalShares:       m.GetTotalShares(),
		AcceptanceRate:    m.GetAcceptanceRate(),
		LastNotify:        m.GetLastNotify(),
		LastSetDifficulty: m.GetLastSetDifficulty(),
	}
}

// Snapshot represents a point-in-time view of metrics
type Snapshot struct {
	UpConnected       bool      `json:"upstream"`
	ClientsActive     int64     `json:"clients_active"`
	SharesOK          uint64    `json:"shares_ok"`
	SharesBad         uint64    `json:"shares_bad"`
	TotalShares       uint64    `json:"total_shares"`
	AcceptanceRate    float64   `json:"acceptance_rate"`
	LastNotify        time.Time `json:"last_notify"`
	LastSetDifficulty int64     `json:"last_set_difficulty"`
}

// ClientMetrics holds per-client metrics
type ClientMetrics struct {
	OK  atomic.Uint64
	Bad atomic.Uint64
}

// NewClientMetrics creates new client metrics
func NewClientMetrics() *ClientMetrics {
	return &ClientMetrics{}
}

// IncrementOK increments accepted shares for this client
func (c *ClientMetrics) IncrementOK() {
	c.OK.Add(1)
}

// IncrementBad increments rejected shares for this client
func (c *ClientMetrics) IncrementBad() {
	c.Bad.Add(1)
}

// GetOK returns accepted shares count
func (c *ClientMetrics) GetOK() uint64 {
	return c.OK.Load()
}

// GetBad returns rejected shares count
func (c *ClientMetrics) GetBad() uint64 {
	return c.Bad.Load()
}

// GetTotal returns total shares count
func (c *ClientMetrics) GetTotal() uint64 {
	return c.OK.Load() + c.Bad.Load()
}

// GetAcceptanceRate calculates acceptance rate for this client
func (c *ClientMetrics) GetAcceptanceRate() float64 {
	total := c.GetTotal()
	if total == 0 {
		return 0
	}
	ok := c.GetOK()
	return (float64(ok) / float64(total)) * 100
}

// Reset resets client metrics
func (c *ClientMetrics) Reset() {
	c.OK.Store(0)
	c.Bad.Store(0)
}
