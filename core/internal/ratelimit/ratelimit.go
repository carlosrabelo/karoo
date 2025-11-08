// Package ratelimit implements rate limiting for client connections
package ratelimit

import (
	"net"
	"sync"
	"time"
)

// Config holds rate limiting configuration
type Config struct {
	// Enabled indicates if rate limiting is active
	Enabled bool `json:"enabled"`
	// MaxConnectionsPerIP limits connections from a single IP
	MaxConnectionsPerIP int `json:"max_connections_per_ip"`
	// MaxConnectionsPerMinute limits new connections per minute from a single IP
	MaxConnectionsPerMinute int `json:"max_connections_per_minute"`
	// BanDurationSeconds how long to ban an IP that exceeds limits
	BanDurationSeconds int `json:"ban_duration_seconds"`
	// CleanupIntervalSeconds how often to cleanup old entries
	CleanupIntervalSeconds int `json:"cleanup_interval_seconds"`
}

// IPStats tracks connection statistics for an IP address
type IPStats struct {
	mu                sync.Mutex
	activeConnections int
	connectionTimes   []time.Time
	bannedUntil       time.Time
}

// Limiter implements rate limiting logic
type Limiter struct {
	cfg   *Config
	mu    sync.RWMutex
	stats map[string]*IPStats
}

// NewLimiter creates a new rate limiter
func NewLimiter(cfg *Config) *Limiter {
	if cfg == nil {
		cfg = &Config{
			Enabled:                 false,
			MaxConnectionsPerIP:     100,
			MaxConnectionsPerMinute: 60,
			BanDurationSeconds:      300,
			CleanupIntervalSeconds:  60,
		}
	}

	l := &Limiter{
		cfg:   cfg,
		stats: make(map[string]*IPStats),
	}

	// Start cleanup routine if enabled
	if cfg.Enabled && cfg.CleanupIntervalSeconds > 0 {
		go l.cleanupRoutine()
	}

	return l
}

// AllowConnection checks if a connection from the given address should be allowed
func (l *Limiter) AllowConnection(addr net.Addr) bool {
	if !l.cfg.Enabled {
		return true
	}

	ip := extractIP(addr)
	if ip == "" {
		return false
	}

	// Get or create stats for this IP
	l.mu.RLock()
	stats, exists := l.stats[ip]
	l.mu.RUnlock()

	if !exists {
		l.mu.Lock()
		// Double-check after acquiring write lock
		stats, exists = l.stats[ip]
		if !exists {
			stats = &IPStats{
				connectionTimes: make([]time.Time, 0, l.cfg.MaxConnectionsPerMinute),
			}
			l.stats[ip] = stats
		}
		l.mu.Unlock()
	}

	stats.mu.Lock()
	defer stats.mu.Unlock()

	now := time.Now()

	// Check if IP is banned
	if now.Before(stats.bannedUntil) {
		return false
	}

	// Check active connections limit
	if l.cfg.MaxConnectionsPerIP > 0 && stats.activeConnections >= l.cfg.MaxConnectionsPerIP {
		return false
	}

	// Check connections per minute limit
	if l.cfg.MaxConnectionsPerMinute > 0 {
		// Remove connection times older than 1 minute
		cutoff := now.Add(-time.Minute)
		newTimes := stats.connectionTimes[:0]
		for _, t := range stats.connectionTimes {
			if t.After(cutoff) {
				newTimes = append(newTimes, t)
			}
		}
		stats.connectionTimes = newTimes

		// Check if limit exceeded
		if len(stats.connectionTimes) >= l.cfg.MaxConnectionsPerMinute {
			// Ban this IP
			stats.bannedUntil = now.Add(time.Duration(l.cfg.BanDurationSeconds) * time.Second)
			return false
		}

		// Record this connection
		stats.connectionTimes = append(stats.connectionTimes, now)
	}

	// Allow connection
	stats.activeConnections++
	return true
}

// ReleaseConnection decrements the active connection count for an IP
func (l *Limiter) ReleaseConnection(addr net.Addr) {
	if !l.cfg.Enabled {
		return
	}

	ip := extractIP(addr)
	if ip == "" {
		return
	}

	l.mu.RLock()
	stats, exists := l.stats[ip]
	l.mu.RUnlock()

	if !exists {
		return
	}

	stats.mu.Lock()
	if stats.activeConnections > 0 {
		stats.activeConnections--
	}
	stats.mu.Unlock()
}

// IsBanned checks if an IP is currently banned
func (l *Limiter) IsBanned(addr net.Addr) bool {
	if !l.cfg.Enabled {
		return false
	}

	ip := extractIP(addr)
	if ip == "" {
		return false
	}

	l.mu.RLock()
	stats, exists := l.stats[ip]
	l.mu.RUnlock()

	if !exists {
		return false
	}

	stats.mu.Lock()
	defer stats.mu.Unlock()

	return time.Now().Before(stats.bannedUntil)
}

// GetStats returns current statistics for an IP
func (l *Limiter) GetStats(addr net.Addr) map[string]interface{} {
	ip := extractIP(addr)
	if ip == "" {
		return nil
	}

	l.mu.RLock()
	stats, exists := l.stats[ip]
	l.mu.RUnlock()

	if !exists {
		return map[string]interface{}{
			"ip":                  ip,
			"active_connections":  0,
			"connections_in_minute": 0,
			"banned":              false,
		}
	}

	stats.mu.Lock()
	defer stats.mu.Unlock()

	return map[string]interface{}{
		"ip":                    ip,
		"active_connections":    stats.activeConnections,
		"connections_in_minute": len(stats.connectionTimes),
		"banned":                time.Now().Before(stats.bannedUntil),
		"banned_until":          stats.bannedUntil,
	}
}

// GetGlobalStats returns global rate limiting statistics
func (l *Limiter) GetGlobalStats() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	totalIPs := len(l.stats)
	totalActive := 0
	bannedIPs := 0

	now := time.Now()
	for _, stats := range l.stats {
		stats.mu.Lock()
		totalActive += stats.activeConnections
		if now.Before(stats.bannedUntil) {
			bannedIPs++
		}
		stats.mu.Unlock()
	}

	return map[string]interface{}{
		"total_ips":          totalIPs,
		"total_active":       totalActive,
		"banned_ips":         bannedIPs,
		"max_per_ip":         l.cfg.MaxConnectionsPerIP,
		"max_per_minute":     l.cfg.MaxConnectionsPerMinute,
		"ban_duration_sec":   l.cfg.BanDurationSeconds,
	}
}

// cleanupRoutine periodically removes old entries
func (l *Limiter) cleanupRoutine() {
	interval := time.Duration(l.cfg.CleanupIntervalSeconds) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		l.cleanup()
	}
}

// cleanup removes entries with no active connections and no recent activity
func (l *Limiter) cleanup() {
	now := time.Now()
	cutoff := now.Add(-5 * time.Minute) // Keep entries from last 5 minutes

	l.mu.Lock()
	defer l.mu.Unlock()

	for ip, stats := range l.stats {
		stats.mu.Lock()

		// Remove if no active connections and not banned and no recent connections
		if stats.activeConnections == 0 &&
		   now.After(stats.bannedUntil) &&
		   (len(stats.connectionTimes) == 0 || stats.connectionTimes[len(stats.connectionTimes)-1].Before(cutoff)) {
			delete(l.stats, ip)
		}

		stats.mu.Unlock()
	}
}

// extractIP extracts the IP address from net.Addr
func extractIP(addr net.Addr) string {
	switch v := addr.(type) {
	case *net.TCPAddr:
		return v.IP.String()
	case *net.UDPAddr:
		return v.IP.String()
	default:
		// Try to parse as string
		host, _, err := net.SplitHostPort(addr.String())
		if err != nil {
			return addr.String()
		}
		return host
	}
}
