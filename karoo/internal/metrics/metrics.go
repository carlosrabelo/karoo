// Package metrics provides collection and reporting of proxy metrics
package metrics

import (
	"sync/atomic"
	"time"
)

// Collector holds all proxy metrics.
// Atomic fields use the classic sync/atomic API for Go 1.18 compatibility.
type Collector struct {
	// Connection metrics
	upConnected   uint32 // 0 or 1
	ClientsActive int64

	// Share metrics
	SharesOK  uint64
	SharesBad uint64

	// Timing metrics
	LastNotifyUnix int64
	LastSetDiff    int64

	// Prometheus collectors
	Prom *PrometheusCollectors
}

// NewCollector creates a new metrics collector
func NewCollector() *Collector {
	return &Collector{
		Prom: InitPrometheus("karoo"),
	}
}

// SetUpstreamConnected sets the upstream connection status
func (m *Collector) SetUpstreamConnected(connected bool) {
	var v uint32
	if connected {
		v = 1
	}
	atomic.StoreUint32(&m.upConnected, v)
	m.Prom.UpConnected.Set(float64(v))
}

// IsUpstreamConnected returns the upstream connection status
func (m *Collector) IsUpstreamConnected() bool {
	return atomic.LoadUint32(&m.upConnected) == 1
}

// IncrementClients increments the active client count
func (m *Collector) IncrementClients() {
	atomic.AddInt64(&m.ClientsActive, 1)
	m.Prom.ClientsActive.Inc()
}

// DecrementClients decrements the active client count
func (m *Collector) DecrementClients() {
	atomic.AddInt64(&m.ClientsActive, -1)
	m.Prom.ClientsActive.Dec()
}

// GetClientsActive returns the current number of active clients
func (m *Collector) GetClientsActive() int64 {
	return atomic.LoadInt64(&m.ClientsActive)
}

// IncrementSharesOK increments the accepted shares counter
func (m *Collector) IncrementSharesOK() {
	atomic.AddUint64(&m.SharesOK, 1)
	m.Prom.SharesOK.Inc()
}

// IncrementSharesBad increments the rejected shares counter
func (m *Collector) IncrementSharesBad() {
	atomic.AddUint64(&m.SharesBad, 1)
	m.Prom.SharesBad.Inc()
}

// GetSharesOK returns the total accepted shares
func (m *Collector) GetSharesOK() uint64 {
	return atomic.LoadUint64(&m.SharesOK)
}

// GetSharesBad returns the total rejected shares
func (m *Collector) GetSharesBad() uint64 {
	return atomic.LoadUint64(&m.SharesBad)
}

// GetTotalShares returns the total shares (accepted + rejected)
func (m *Collector) GetTotalShares() uint64 {
	return m.GetSharesOK() + m.GetSharesBad()
}

// SetLastNotify updates the last notification timestamp
func (m *Collector) SetLastNotify(t time.Time) {
	atomic.StoreInt64(&m.LastNotifyUnix, t.Unix())
	m.Prom.LastNotify.Set(float64(t.Unix()))
}

// GetLastNotify returns the last notification timestamp
func (m *Collector) GetLastNotify() time.Time {
	unix := atomic.LoadInt64(&m.LastNotifyUnix)
	return time.Unix(unix, 0)
}

// SetLastSetDifficulty updates the last set difficulty timestamp
func (m *Collector) SetLastSetDifficulty(difficulty int64) {
	atomic.StoreInt64(&m.LastSetDiff, difficulty)
	m.Prom.LastSetDiff.Set(float64(difficulty))
}

// GetLastSetDifficulty returns the last set difficulty
func (m *Collector) GetLastSetDifficulty() int64 {
	return atomic.LoadInt64(&m.LastSetDiff)
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
	atomic.StoreUint32(&m.upConnected, 0)
	atomic.StoreInt64(&m.ClientsActive, 0)
	atomic.StoreUint64(&m.SharesOK, 0)
	atomic.StoreUint64(&m.SharesBad, 0)
	atomic.StoreInt64(&m.LastNotifyUnix, 0)
	atomic.StoreInt64(&m.LastSetDiff, 0)
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
	OK  uint64
	Bad uint64
}

// NewClientMetrics creates new client metrics
func NewClientMetrics() *ClientMetrics {
	return &ClientMetrics{}
}

// IncrementOK increments accepted shares for this client
func (c *ClientMetrics) IncrementOK() {
	atomic.AddUint64(&c.OK, 1)
}

// IncrementBad increments rejected shares for this client
func (c *ClientMetrics) IncrementBad() {
	atomic.AddUint64(&c.Bad, 1)
}

// GetOK returns accepted shares count
func (c *ClientMetrics) GetOK() uint64 {
	return atomic.LoadUint64(&c.OK)
}

// GetBad returns rejected shares count
func (c *ClientMetrics) GetBad() uint64 {
	return atomic.LoadUint64(&c.Bad)
}

// GetTotal returns total shares count
func (c *ClientMetrics) GetTotal() uint64 {
	return c.GetOK() + c.GetBad()
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
	atomic.StoreUint64(&c.OK, 0)
	atomic.StoreUint64(&c.Bad, 0)
}
