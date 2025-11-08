// Package vardiff implements variable difficulty adjustment for mining clients
package vardiff

import (
	"context"
	"sync"
	"time"

	"github.com/carlosrabelo/karoo/core/internal/stratum"
)

const (
	// maxShareWindowSize limits the number of shares tracked per client
	// to prevent unbounded memory growth
	maxShareWindowSize = 100
	// maxShareWindowAge is the maximum age of shares to keep in the window
	maxShareWindowAge = 10 * time.Minute
)

// Client represents a mining client interface for vardiff package
type Client interface {
	WriteJSON(stratum.Message) error
}

// Config holds vardiff configuration
type Config struct {
	Enabled       bool `json:"enabled"`
	TargetSeconds int  `json:"target_seconds"`
	MinDiff       int  `json:"min_diff"`
	MaxDiff       int  `json:"max_diff"`
	AdjustEveryMs int  `json:"adjust_every_ms"`
}

// ClientStats tracks per-client statistics for vardiff calculations
type ClientStats struct {
	mu                sync.Mutex
	LastAdjustTime    time.Time
	ShareWindow       []ShareEntry
	CurrentDifficulty float64
	LastShareTime     time.Time
	SharesPerSecond   float64
	RetargetInterval  time.Duration
}

// ShareEntry represents a single share submission
type ShareEntry struct {
	Timestamp  time.Time
	Accepted   bool
	Difficulty float64
}

// Manager handles variable difficulty adjustment for all clients
type Manager struct {
	cfg *Config

	clientsMu sync.RWMutex
	clients   map[Client]*ClientStats
}

// NewManager creates a new vardiff manager
func NewManager(cfg *Config) *Manager {
	return &Manager{
		cfg:     cfg,
		clients: make(map[Client]*ClientStats),
	}
}

// AddClient adds a client to vardiff management
func (m *Manager) AddClient(cl Client) {
	if !m.cfg.Enabled {
		return
	}

	stats := &ClientStats{
		CurrentDifficulty: float64(m.cfg.MinDiff),
		LastAdjustTime:    time.Now(),
		LastShareTime:     time.Now(),
		RetargetInterval:  time.Duration(m.cfg.AdjustEveryMs) * time.Millisecond,
		ShareWindow:       make([]ShareEntry, 0, 100), // Keep last 100 shares
	}

	m.clientsMu.Lock()
	m.clients[cl] = stats
	m.clientsMu.Unlock()

	// Send initial difficulty
	m.sendDifficulty(cl, stats.CurrentDifficulty)
}

// RemoveClient removes a client from vardiff management
func (m *Manager) RemoveClient(cl Client) {
	m.clientsMu.Lock()
	delete(m.clients, cl)
	m.clientsMu.Unlock()
}

// RecordShare records a share submission for difficulty calculations
func (m *Manager) RecordShare(cl Client, accepted bool, difficulty float64) {
	if !m.cfg.Enabled {
		return
	}

	m.clientsMu.RLock()
	stats, exists := m.clients[cl]
	m.clientsMu.RUnlock()

	if !exists {
		return
	}

	stats.mu.Lock()
	defer stats.mu.Unlock()

	// Add share to window
	entry := ShareEntry{
		Timestamp:  time.Now(),
		Accepted:   accepted,
		Difficulty: difficulty,
	}
	stats.ShareWindow = append(stats.ShareWindow, entry)

	// Keep only recent shares (last retarget interval * 2 or maxShareWindowAge, whichever is shorter)
	maxAge := stats.RetargetInterval * 2
	if maxAge > maxShareWindowAge {
		maxAge = maxShareWindowAge
	}
	cutoff := time.Now().Add(-maxAge)

	// Remove old shares by timestamp
	for i, share := range stats.ShareWindow {
		if share.Timestamp.After(cutoff) {
			stats.ShareWindow = stats.ShareWindow[i:]
			break
		}
	}

	// Enforce maximum window size to prevent unbounded memory growth
	if len(stats.ShareWindow) > maxShareWindowSize {
		stats.ShareWindow = stats.ShareWindow[len(stats.ShareWindow)-maxShareWindowSize:]
	}

	// Update last share time
	if accepted {
		stats.LastShareTime = time.Now()
	}

	// Calculate shares per second
	m.calculateSharesPerSecond(stats)
}

// calculateSharesPerSecond calculates the current share rate
func (m *Manager) calculateSharesPerSecond(stats *ClientStats) {
	if len(stats.ShareWindow) < 2 {
		stats.SharesPerSecond = 0
		return
	}

	// Count accepted shares in the window
	acceptedShares := 0
	windowStart := stats.ShareWindow[0].Timestamp
	windowEnd := stats.ShareWindow[len(stats.ShareWindow)-1].Timestamp

	for _, share := range stats.ShareWindow {
		if share.Accepted {
			acceptedShares++
		}
	}

	duration := windowEnd.Sub(windowStart).Seconds()
	if duration > 0 {
		stats.SharesPerSecond = float64(acceptedShares) / duration
	}
}

// AdjustDifficulties performs difficulty adjustment for all clients
func (m *Manager) AdjustDifficulties() {
	if !m.cfg.Enabled {
		return
	}

	m.clientsMu.RLock()
	clients := make([]Client, 0, len(m.clients))
	for cl := range m.clients {
		clients = append(clients, cl)
	}
	m.clientsMu.RUnlock()

	for _, cl := range clients {
		m.adjustClientDifficulty(cl)
	}
}

// adjustClientDifficulty adjusts difficulty for a specific client
func (m *Manager) adjustClientDifficulty(cl Client) {
	m.clientsMu.RLock()
	stats, exists := m.clients[cl]
	if !exists {
		m.clientsMu.RUnlock()
		return
	}

	// Keep RLock while accessing stats to prevent use-after-free
	stats.mu.Lock()
	m.clientsMu.RUnlock()
	defer stats.mu.Unlock()

	now := time.Now()
	if now.Sub(stats.LastAdjustTime) < stats.RetargetInterval {
		return
	}

	// Calculate new difficulty
	newDiff := m.calculateNewDifficulty(stats)

	// Apply bounds
	if newDiff < float64(m.cfg.MinDiff) {
		newDiff = float64(m.cfg.MinDiff)
	} else if newDiff > float64(m.cfg.MaxDiff) {
		newDiff = float64(m.cfg.MaxDiff)
	}

	// Update if changed significantly (more than 10% difference)
	diffRatio := newDiff / stats.CurrentDifficulty
	if diffRatio < 0.9 || diffRatio > 1.1 {
		stats.CurrentDifficulty = newDiff
		stats.LastAdjustTime = now
		m.sendDifficulty(cl, newDiff)
	}
}

// calculateNewDifficulty calculates the optimal difficulty for a client
func (m *Manager) calculateNewDifficulty(stats *ClientStats) float64 {
	if stats.SharesPerSecond == 0 {
		// No shares recently, reduce difficulty
		return stats.CurrentDifficulty * 0.5
	}

	// Target shares per second based on current difficulty
	targetSharesPerSec := stats.CurrentDifficulty / float64(m.cfg.TargetSeconds)

	// Adjust difficulty to reach target
	if stats.SharesPerSecond > targetSharesPerSec*1.2 {
		// Too fast, increase difficulty
		return stats.CurrentDifficulty * 1.2
	} else if stats.SharesPerSecond < targetSharesPerSec*0.8 {
		// Too slow, decrease difficulty
		return stats.CurrentDifficulty * 0.8
	}

	// Within acceptable range, keep current difficulty
	return stats.CurrentDifficulty
}

// sendDifficulty sends a new difficulty to a client
func (m *Manager) sendDifficulty(cl Client, difficulty float64) {
	msg := stratum.Message{
		Method: "mining.set_difficulty",
		Params: []interface{}{difficulty},
	}
	_ = cl.WriteJSON(msg)
}

// Run starts the vardiff adjustment loop
func (m *Manager) Run(ctx context.Context) {
	if !m.cfg.Enabled {
		return
	}

	ticker := time.NewTicker(time.Duration(m.cfg.AdjustEveryMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.AdjustDifficulties()
		}
	}
}

// GetClientStats returns statistics for a client
func (m *Manager) GetClientStats(cl Client) *ClientStats {
	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()

	if stats, exists := m.clients[cl]; exists {
		// Return a copy to avoid race conditions
		stats.mu.Lock()
		copy := &ClientStats{
			CurrentDifficulty: stats.CurrentDifficulty,
			LastShareTime:     stats.LastShareTime,
			SharesPerSecond:   stats.SharesPerSecond,
			RetargetInterval:  stats.RetargetInterval,
		}
		stats.mu.Unlock()
		return copy
	}

	return nil
}

// Reset resets all client statistics
func (m *Manager) Reset() {
	m.clientsMu.Lock()
	defer m.clientsMu.Unlock()

	for _, stats := range m.clients {
		stats.mu.Lock()
		stats.ShareWindow = stats.ShareWindow[:0]
		stats.CurrentDifficulty = float64(m.cfg.MinDiff)
		stats.LastAdjustTime = time.Now()
		stats.LastShareTime = time.Now()
		stats.SharesPerSecond = 0
		stats.mu.Unlock()
	}
}

// GetStats returns global vardiff statistics
func (m *Manager) GetStats() map[string]interface{} {
	m.clientsMu.RLock()
	defer m.clientsMu.RUnlock()

	totalClients := len(m.clients)
	avgDifficulty := 0.0
	avgSharesPerSec := 0.0
	activeClients := 0

	for _, stats := range m.clients {
		stats.mu.Lock()
		if time.Since(stats.LastShareTime) < time.Minute {
			activeClients++
			avgDifficulty += stats.CurrentDifficulty
			avgSharesPerSec += stats.SharesPerSecond
		}
		stats.mu.Unlock()
	}

	if activeClients > 0 {
		avgDifficulty /= float64(activeClients)
		avgSharesPerSec /= float64(activeClients)
	}

	return map[string]interface{}{
		"total_clients":      totalClients,
		"active_clients":     activeClients,
		"avg_difficulty":     avgDifficulty,
		"avg_shares_per_sec": avgSharesPerSec,
		"target_seconds":     m.cfg.TargetSeconds,
		"min_difficulty":     m.cfg.MinDiff,
		"max_difficulty":     m.cfg.MaxDiff,
	}
}
