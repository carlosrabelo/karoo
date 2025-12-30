// Package proxy implements the core Stratum proxy logic
package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/carlosrabelo/karoo/core/internal/connection"
	"github.com/carlosrabelo/karoo/core/internal/metrics"
	"github.com/carlosrabelo/karoo/core/internal/nonce"
	"github.com/carlosrabelo/karoo/core/internal/proxysocks"
	"github.com/carlosrabelo/karoo/core/internal/ratelimit"
	"github.com/carlosrabelo/karoo/core/internal/routing"
	"github.com/carlosrabelo/karoo/core/internal/stratum"
	"github.com/carlosrabelo/karoo/core/internal/vardiff"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Client represents a mining client connection
type Client struct {
	c                net.Conn
	br               *bufio.Reader
	bw               *bufio.Writer
	addr             string
	worker           string
	upUser           string
	handshakeDone    atomic.Bool
	last             atomic.Int64
	diff             atomic.Int64
	ok               atomic.Uint64
	bad              atomic.Uint64
	extraNoncePrefix string
	extraNonceTrim   int
	lastAccept       atomic.Int64
	clientMetrics    *metrics.ClientMetrics
}

// UpstreamConfig holds upstream connection details
type UpstreamConfig struct {
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
		Type     string `json:"type"` // "socks4" or "socks5"
		Host     string `json:"host"`
		Port     int    `json:"port"`
		Username string `json:"username"` // optional for SOCKS5
		Password string `json:"password"` // optional for SOCKS5
	} `json:"socks_proxy"`
}

// Config holds proxy configuration
type Config struct {
	Proxy struct {
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
	} `json:"proxy"`
	Upstream UpstreamConfig   `json:"upstream"`
	Backups  []UpstreamConfig `json:"backups"`
	HTTP     struct {
		Listen string `json:"listen"`
		Pprof  bool   `json:"pprof"`
	} `json:"http"`
	VarDiff struct {
		Enabled       bool `json:"enabled"`
		TargetSeconds int  `json:"target_seconds"`
		MinDiff       int  `json:"min_diff"`
		MaxDiff       int  `json:"max_diff"`
		AdjustEveryMs int  `json:"adjust_every_ms"`
	} `json:"vardiff"`
	RateLimit struct {
		Enabled                 bool `json:"enabled"`
		MaxConnectionsPerIP     int  `json:"max_connections_per_ip"`
		MaxConnectionsPerMinute int  `json:"max_connections_per_minute"`
		BanDurationSeconds      int  `json:"ban_duration_seconds"`
		CleanupIntervalSeconds  int  `json:"cleanup_interval_seconds"`
	} `json:"ratelimit"`
	Compat struct {
		StrictBroadcast bool `json:"strict_broadcast"`
	} `json:"compat"`
}

// Proxy represents the main proxy instance
type Proxy struct {
	cfg *Config
	up  *connection.Upstream
	mx  *metrics.Collector
	rt  *routing.Router
	nm  *nonce.Manager
	vd  *vardiff.Manager
	rl  *ratelimit.Limiter

	clMu    sync.RWMutex
	clients map[*Client]struct{}
}

// NewProxy creates a new proxy instance
func NewProxy(cfg *Config) *Proxy {
	// Convert config for connection package
	connCfg := &connection.Config{
		Proxy: struct {
			ReadBuf  int `json:"read_buf"`
			WriteBuf int `json:"write_buf"`
		}{
			ReadBuf:  cfg.Proxy.ReadBuf,
			WriteBuf: cfg.Proxy.WriteBuf,
		},
		Upstream: struct {
			Host               string            `json:"host"`
			Port               int               `json:"port"`
			User               string            `json:"user"`
			Pass               string            `json:"pass"`
			TLS                bool              `json:"tls"`
			InsecureSkipVerify bool              `json:"insecure_skip_verify"`
			SocksProxy         proxysocks.Config `json:"socks_proxy"`
		}{
			Host:               cfg.Upstream.Host,
			Port:               cfg.Upstream.Port,
			User:               cfg.Upstream.User,
			Pass:               cfg.Upstream.Pass,
			TLS:                cfg.Upstream.TLS,
			InsecureSkipVerify: cfg.Upstream.InsecureSkipVerify,
			SocksProxy:         cfg.Upstream.SocksProxy,
		},
	}
	// Convert config for routing package
	routingCfg := &routing.Config{
		Upstream: struct {
			User string `json:"user"`
		}{
			User: cfg.Upstream.User,
		},
		Compat: cfg.Compat,
	}

	up, err := connection.NewUpstream(connCfg)
	if err != nil {
		log.Fatalf("Failed to create upstream: %v", err)
	}
	mx := metrics.NewCollector()
	rt := routing.NewRouter(routingCfg, up, mx)
	nm := nonce.NewManager(up)

	vdCfg := &vardiff.Config{
		Enabled:       cfg.VarDiff.Enabled,
		TargetSeconds: cfg.VarDiff.TargetSeconds,
		MinDiff:       cfg.VarDiff.MinDiff,
		MaxDiff:       cfg.VarDiff.MaxDiff,
		AdjustEveryMs: cfg.VarDiff.AdjustEveryMs,
	}
	vd := vardiff.NewManager(vdCfg)

	rlCfg := &ratelimit.Config{
		Enabled:                 cfg.RateLimit.Enabled,
		MaxConnectionsPerIP:     cfg.RateLimit.MaxConnectionsPerIP,
		MaxConnectionsPerMinute: cfg.RateLimit.MaxConnectionsPerMinute,
		BanDurationSeconds:      cfg.RateLimit.BanDurationSeconds,
		CleanupIntervalSeconds:  cfg.RateLimit.CleanupIntervalSeconds,
	}
	rl := ratelimit.NewLimiter(rlCfg)

	return &Proxy{
		cfg:     cfg,
		up:      up,
		mx:      mx,
		rt:      rt,
		nm:      nm,
		vd:      vd,
		rl:      rl,
		clients: make(map[*Client]struct{}),
	}
}

// Reload updates proxy configuration at runtime
func (p *Proxy) Reload(newCfg *Config) {
	log.Println("Reloading configuration...")

	// Update Config (Struct copy)
	// We update the fields implementation pointers point to
	*p.cfg = *newCfg

	// Update specific managers that support reloading
	// VarDiff
	p.vd.UpdateConfig(&vardiff.Config{
		Enabled:       newCfg.VarDiff.Enabled,
		TargetSeconds: newCfg.VarDiff.TargetSeconds,
		MinDiff:       newCfg.VarDiff.MinDiff,
		MaxDiff:       newCfg.VarDiff.MaxDiff,
		AdjustEveryMs: newCfg.VarDiff.AdjustEveryMs,
	})

	// RateLimit
	p.rl.UpdateConfig(&ratelimit.Config{
		Enabled:                 newCfg.RateLimit.Enabled,
		MaxConnectionsPerIP:     newCfg.RateLimit.MaxConnectionsPerIP,
		MaxConnectionsPerMinute: newCfg.RateLimit.MaxConnectionsPerMinute,
		BanDurationSeconds:      newCfg.RateLimit.BanDurationSeconds,
		CleanupIntervalSeconds:  newCfg.RateLimit.CleanupIntervalSeconds,
	})

	log.Println("Configuration reloaded")
}

// NewClient creates a new client instance
func NewClient(conn net.Conn, cfg *Config) *Client {
	return &Client{
		c:             conn,
		br:            bufio.NewReaderSize(conn, cfg.Proxy.ReadBuf),
		bw:            bufio.NewWriterSize(conn, cfg.Proxy.WriteBuf),
		addr:          conn.RemoteAddr().String(),
		upUser:        cfg.Upstream.User,
		clientMetrics: metrics.NewClientMetrics(),
	}
}

// GetAddr returns the client address
func (c *Client) GetAddr() string {
	return c.addr
}

// GetWorker returns the worker name
func (c *Client) GetWorker() string {
	return c.worker
}

// GetUpUser returns the upstream user
func (c *Client) GetUpUser() string {
	return c.upUser
}

// SetWorker sets the worker name
func (c *Client) SetWorker(worker string) {
	c.worker = worker
}

// SetUpUser sets the upstream user
func (c *Client) SetUpUser(upUser string) {
	c.upUser = upUser
}

// GetExtraNoncePrefix returns the extranonce prefix
func (c *Client) GetExtraNoncePrefix() string {
	return c.extraNoncePrefix
}

// GetExtraNonceTrim returns the extranonce trim
func (c *Client) GetExtraNonceTrim() int {
	return c.extraNonceTrim
}

// SetExtraNoncePrefix sets the extranonce prefix
func (c *Client) SetExtraNoncePrefix(prefix string) {
	c.extraNoncePrefix = prefix
}

// SetExtraNonceTrim sets the extranonce trim
func (c *Client) SetExtraNonceTrim(trim int) {
	c.extraNonceTrim = trim
}

// GetLastAccept returns the last accept timestamp
func (c *Client) GetLastAccept() int64 {
	return c.lastAccept.Load()
}

// UpdateLastAccept updates the last accept timestamp
func (c *Client) UpdateLastAccept(timestamp int64) {
	c.lastAccept.Store(timestamp)
}

// GetOK returns the number of accepted shares
func (c *Client) GetOK() uint64 {
	return c.ok.Load()
}

// GetBad returns the number of rejected shares
func (c *Client) GetBad() uint64 {
	return c.bad.Load()
}

// IncrementOK increments the accepted shares counter
func (c *Client) IncrementOK() {
	c.ok.Add(1)
}

// IncrementBad increments the rejected shares counter
func (c *Client) IncrementBad() {
	c.bad.Add(1)
}

// SetHandshakeDone sets the handshake done flag
func (c *Client) SetHandshakeDone(done bool) {
	c.handshakeDone.Store(done)
}

// WriteJSON writes a JSON message to the client
func (c *Client) WriteJSON(msg stratum.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = c.bw.Write(data)
	if err != nil {
		return err
	}
	_, err = c.bw.WriteString("\n")
	if err != nil {
		return err
	}
	return c.bw.Flush()
}

// WriteLine writes a line to the client
func (c *Client) WriteLine(line string) error {
	_, err := c.bw.WriteString(line)
	if err != nil {
		return err
	}
	_, err = c.bw.WriteString("\n")
	if err != nil {
		return err
	}
	return c.bw.Flush()
}

// AcceptLoop accepts new client connections
func (p *Proxy) AcceptLoop(ctx context.Context) error {
	var ln net.Listener
	var err error

	if p.cfg.Proxy.TLS.Enabled {
		cert, err := tls.LoadX509KeyPair(p.cfg.Proxy.TLS.Cert, p.cfg.Proxy.TLS.Key)
		if err != nil {
			return fmt.Errorf("loading tls keys: %w", err)
		}
		tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}}
		ln, err = tls.Listen("tcp", p.cfg.Proxy.Listen, tlsCfg)
		log.Printf("proxy: listening on %s (TLS enabled)", p.cfg.Proxy.Listen)
	} else {
		ln, err = net.Listen("tcp", p.cfg.Proxy.Listen)
		log.Printf("proxy: listening on %s", p.cfg.Proxy.Listen)
	}

	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("accept err: %v", err)
			continue
		}

		// Check rate limiting
		if !p.rl.AllowConnection(conn.RemoteAddr()) {
			log.Printf("rejecting client %s: rate limit exceeded", conn.RemoteAddr())
			_ = conn.Close()
			continue
		}

		if p.mx.ClientsActive.Load() >= int64(p.cfg.Proxy.MaxClients) {
			log.Printf("rejecting client: max reached")
			p.rl.ReleaseConnection(conn.RemoteAddr())
			_ = conn.Close()
			continue
		}
		cli := NewClient(conn, p.cfg)
		cli.last.Store(time.Now().UnixMilli())
		cli.diff.Store(int64(p.cfg.VarDiff.MinDiff))

		p.clMu.Lock()
		p.clients[cli] = struct{}{}
		p.clMu.Unlock()

		// Add to all managers
		p.rt.AddClient(cli)
		p.vd.AddClient(cli)
		p.mx.ClientsActive.Add(1)
		log.Printf("client connected: %s", cli.addr)

		go p.ClientLoop(ctx, cli)
	}
}

// ClientLoop handles individual client communication
func (p *Proxy) ClientLoop(ctx context.Context, cl *Client) {
	startTime := time.Now()

	defer func() {
		p.nm.RemovePendingSubscribe(cl)
		p.rt.RemoveClient(cl)
		p.vd.RemoveClient(cl)
		p.rl.ReleaseConnection(cl.c.RemoteAddr())

		p.clMu.Lock()
		delete(p.clients, cl)
		p.clMu.Unlock()

		p.mx.ClientsActive.Add(-1)
		_ = cl.c.Close()

		// Log graceful disconnect with session statistics
		duration := time.Since(startTime)
		totalShares := cl.GetOK() + cl.GetBad()
		worker := cl.GetWorker()
		if worker == "" {
			worker = "unknown"
		}

		log.Printf("client closed: %s worker=%s duration=%s shares=%d (ok=%d bad=%d)",
			cl.addr, worker, duration.Round(time.Second), totalShares, cl.GetOK(), cl.GetBad())
	}()

	sc := bufio.NewScanner(cl.br)
	buf := make([]byte, 0, p.cfg.Proxy.ReadBuf)
	sc.Buffer(buf, 1024*1024)

	idle := p.cfg.Proxy.ClientIdleMs
	postHandshakeIdle := 30 * time.Minute // Timeout for authenticated clients
	for {
		if idle > 0 && !cl.handshakeDone.Load() {
			// Pre-handshake timeout (shorter)
			_ = cl.c.SetReadDeadline(time.Now().Add(time.Duration(idle) * time.Millisecond))
		} else if cl.handshakeDone.Load() {
			// Post-handshake timeout (longer, prevents resource leaks)
			_ = cl.c.SetReadDeadline(time.Now().Add(postHandshakeIdle))
		} else {
			_ = cl.c.SetReadDeadline(time.Time{})
		}
		if !sc.Scan() {
			if err := sc.Err(); err != nil && !isNetClosed(err) {
				log.Printf("client scan err %s: %v", cl.addr, err)
			}
			return
		}
		line := sc.Text()
		cl.last.Store(time.Now().UnixMilli())

		var msg stratum.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		switch msg.Method {
		case "mining.subscribe":
			p.nm.RespondSubscribe(cl, msg.ID)
			continue

		default:
			// Route all other messages through the router
			p.rt.ProcessClientMessage(cl, msg)
		}
	}
}

// UpstreamLoop manages upstream connection and message handling with failover support
func (p *Proxy) UpstreamLoop(ctx context.Context) {
	currentIdx := 0

	for ctx.Err() == nil {
		// Rebuild list of upstreams to try (Primary + Backups) on every iteration
		// This allows hot-reloading of upstream configuration
		configs := []UpstreamConfig{p.cfg.Upstream}
		configs = append(configs, p.cfg.Backups...)

		// Safety check if configs is empty (shouldn't happen with validation)
		if len(configs) == 0 {
			time.Sleep(1 * time.Second)
			continue
		}

		// Adjust index if out of bounds (can happen if backups removed)
		if currentIdx >= len(configs) {
			currentIdx = 0
		}

		activeCfg := configs[currentIdx]

		// Update upstream target
		p.up.UpdateTarget(
			activeCfg.Host,
			activeCfg.Port,
			activeCfg.User,
			activeCfg.Pass,
			activeCfg.TLS,
			activeCfg.InsecureSkipVerify,
		)

		min := time.Duration(activeCfg.BackoffMinMs) * time.Millisecond
		max := time.Duration(activeCfg.BackoffMaxMs) * time.Millisecond

		if err := p.up.Dial(ctx); err != nil {
			d := connection.Backoff(min, max)
			log.Printf("upstream dial fail (idx=%d): %v; retry in %s", currentIdx, err, d)

			// Failover logic: switch to next upstream
			currentIdx = (currentIdx + 1) % len(configs)
			if currentIdx != 0 {
				log.Printf("switching to backup upstream index %d", currentIdx)
			} else {
				log.Printf("cycled through all upstreams, back to primary")
			}

			time.Sleep(d)
			continue
		}

		p.mx.UpConnected.Store(true)
		log.Printf("upstream connected (idx=%d)", currentIdx)

		// handshake
		if err := p.up.SubscribeAuthorize(); err != nil {
			log.Printf("handshake err: %v", err)
			p.up.Close()
			p.mx.UpConnected.Store(false)

			// Try next upstream on handshake failure
			currentIdx = (currentIdx + 1) % len(configs)
			time.Sleep(1 * time.Second)
			continue
		}

		sc := bufio.NewScanner(p.up.GetReader())
		buf := make([]byte, 0, p.cfg.Proxy.ReadBuf)
		sc.Buffer(buf, 1024*1024)

		for sc.Scan() {
			line := sc.Text()
			p.rt.ProcessUpstreamMessage(line)

			// Handle subscribe result specially
			var msg stratum.Message
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}

			if msg.Result != nil && msg.ID != nil && *msg.ID == 1 {
				log.Printf("subscribe result: %v", msg.Result)
				p.nm.ProcessSubscribeResult(msg.Result)
			}
		}

		if err := sc.Err(); err != nil && !isNetClosed(err) {
			log.Printf("upstream read err: %v", err)
		}
		p.up.Close()
		p.mx.UpConnected.Store(false)
		p.nm.Reset()

		d := connection.Backoff(min, max)
		log.Printf("upstream disconnected; retry in %s", d)
		time.Sleep(d)

		// Try next upstream on disconnect
		currentIdx = (currentIdx + 1) % len(configs)
	}
}

// HttpServe starts HTTP server with status and health endpoints
func (p *Proxy) HttpServe(ctx context.Context) {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	})
	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		type clientView struct {
			IP     string `json:"ip"`
			Worker string `json:"worker"`
			UpUser string `json:"upstream_user"`
			OK     uint64 `json:"ok"`
			Bad    uint64 `json:"bad"`
		}
		p.clMu.RLock()
		var clv []clientView
		for cl := range p.clients {
			clv = append(clv, clientView{
				IP:     cl.addr,
				Worker: cl.worker,
				UpUser: cl.upUser,
				OK:     cl.ok.Load(),
				Bad:    cl.bad.Load(),
			})
		}
		p.clMu.RUnlock()

		ex1, ex2Size := p.up.GetExtranonce()
		out := map[string]interface{}{
			"upstream":         p.mx.UpConnected.Load(),
			"extranonce1":      ex1,
			"extranonce2_size": ex2Size,
			"last_notify_unix": p.mx.LastNotifyUnix.Load(),
			"last_diff":        p.mx.LastSetDiff.Load(),
			"shares_ok":        p.mx.SharesOK.Load(),
			"shares_bad":       p.mx.SharesBad.Load(),
			"clients":          clv,
			"vardiff":          p.vd.GetStats(),
			"ratelimit":        p.rl.GetGlobalStats(),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
	http.Handle("/metrics", promhttp.Handler())
	srv := &http.Server{Addr: p.cfg.HTTP.Listen}
	go func() {
		<-ctx.Done()
		ctx2, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx2)
	}()
	log.Printf("http: listening on %s", p.cfg.HTTP.Listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("http err: %v", err)
	}
}

// ReportLoop generates periodic reports about proxy performance
func (p *Proxy) ReportLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	start := time.Now()
	last := start
	lastOK := p.mx.SharesOK.Load()
	lastBad := p.mx.SharesBad.Load()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			totalOK := p.mx.SharesOK.Load()
			totalBad := p.mx.SharesBad.Load()
			deltaOK := totalOK - lastOK
			deltaBad := totalBad - lastBad
			submittedInterval := deltaOK + deltaBad
			submittedTotal := totalOK + totalBad
			intervalDur := now.Sub(last)
			totalDur := now.Sub(start)
			var rateInterval, rateTotal float64
			if minutes := intervalDur.Minutes(); minutes > 0 {
				rateInterval = float64(submittedInterval) / minutes
			}
			if minutes := totalDur.Minutes(); minutes > 0 {
				rateTotal = float64(submittedTotal) / minutes
			}
			var accInterval, accTotal float64
			if submittedInterval > 0 {
				accInterval = (float64(deltaOK) / float64(submittedInterval)) * 100
			}
			if submittedTotal > 0 {
				accTotal = (float64(totalOK) / float64(submittedTotal)) * 100
			}
			log.Printf("Periodic Report interval=%10s total=%10s | submitted %d/%d (acc %.1f%% / %.1f%%) | rejects %d/%d | rate %.2f/min (overall %.2f/min)", intervalDur.Round(time.Second), totalDur.Round(time.Second), deltaOK, totalOK, accInterval, accTotal, deltaBad, totalBad, rateInterval, rateTotal)
			last = now
			lastOK = totalOK
			lastBad = totalBad
		}
	}
}

// UpstreamManager manages upstream connection based on client activity
func (p *Proxy) UpstreamManager(ctx context.Context, idleGrace time.Duration) {
	var upCancel context.CancelFunc
	var upCtx context.Context
	upstreamRunning := false
	var graceTimer *time.Timer
	var graceTimerCh <-chan time.Time

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if upCancel != nil {
				upCancel()
			}
			if graceTimer != nil {
				graceTimer.Stop()
			}
			return

		case <-graceTimerCh:
			// Grace period expired, stop upstream if still no clients
			if p.mx.ClientsActive.Load() == 0 && upstreamRunning {
				if upCancel != nil {
					upCancel()
				}
				upstreamRunning = false
			}
			graceTimer = nil
			graceTimerCh = nil

		case <-ticker.C:
			hasClients := p.mx.ClientsActive.Load() > 0

			if hasClients && !upstreamRunning {
				// Cancel any pending grace period
				if graceTimer != nil {
					graceTimer.Stop()
					graceTimer = nil
					graceTimerCh = nil
				}
				// Start upstream
				upCtx, upCancel = context.WithCancel(ctx)
				go p.UpstreamLoop(upCtx)
				upstreamRunning = true

			} else if !hasClients && upstreamRunning && graceTimer == nil {
				// Start grace period timer (only if not already started)
				graceTimer = time.NewTimer(idleGrace)
				graceTimerCh = graceTimer.C

			} else if hasClients && graceTimer != nil {
				// Clients reconnected during grace period, cancel timer
				graceTimer.Stop()
				graceTimer = nil
				graceTimerCh = nil
			}
		}
	}
}

// VarDiffLoop starts variable difficulty adjustment
func (p *Proxy) VarDiffLoop(ctx context.Context) {
	p.vd.Run(ctx)
}

// isNetClosed checks if error is network closed error
func isNetClosed(err error) bool {
	return strings.Contains(err.Error(), "use of closed network connection") ||
		strings.Contains(err.Error(), "connection reset by peer")
}
