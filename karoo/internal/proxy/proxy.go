// Package proxy implements the core Stratum proxy logic
package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/carlosrabelo/karoo/karoo/internal/connection"
	"github.com/carlosrabelo/karoo/karoo/internal/metrics"
	"github.com/carlosrabelo/karoo/karoo/internal/nonce"
	"github.com/carlosrabelo/karoo/karoo/internal/proxysocks"
	"github.com/carlosrabelo/karoo/karoo/internal/ratelimit"
	"github.com/carlosrabelo/karoo/karoo/internal/routing"
	"github.com/carlosrabelo/karoo/karoo/internal/stratum"
	"github.com/carlosrabelo/karoo/karoo/internal/vardiff"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Client represents a mining client connection
type Client struct {
	c     net.Conn
	br    *bufio.Reader
	bw    *bufio.Writer
	write sync.Mutex // serializes all writes to bw
	mu    sync.RWMutex
	addr  string

	worker           string
	upUser           string
	extraNoncePrefix string
	extraNonceTrim   int

	handshakeDone uint32 // 0/1 — classic atomic for Go 1.18
	last          int64
	diff          int64
	ok            uint64
	bad           uint64
	lastAccept    int64
	clientMetrics *metrics.ClientMetrics
}

// UpstreamConfig holds upstream connection details
type UpstreamConfig struct {
	Host               string            `json:"host"`
	Port               int               `json:"port"`
	User               string            `json:"user"`
	Pass               string            `json:"pass"`
	TLS                bool              `json:"tls"`
	InsecureSkipVerify bool              `json:"insecure_skip_verify"`
	BackoffMinMs       int               `json:"backoff_min_ms"`
	BackoffMaxMs       int               `json:"backoff_max_ms"`
	SocksProxy         proxysocks.Config `json:"socks_proxy"`
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
		LocalAuthorize  bool `json:"local_authorize"`
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
func NewProxy(cfg *Config) (*Proxy, error) {
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

	up, err := connection.NewUpstream(connCfg)
	if err != nil {
		return nil, fmt.Errorf("create upstream: %w", err)
	}
	mx := metrics.NewCollector()

	vdCfg := &vardiff.Config{
		Enabled:       cfg.VarDiff.Enabled,
		TargetSeconds: cfg.VarDiff.TargetSeconds,
		MinDiff:       cfg.VarDiff.MinDiff,
		MaxDiff:       cfg.VarDiff.MaxDiff,
		AdjustEveryMs: cfg.VarDiff.AdjustEveryMs,
	}
	vd := vardiff.NewManager(vdCfg)

	// Convert config for routing package
	routingCfg := &routing.Config{
		Upstream: struct {
			User string `json:"user"`
		}{
			User: cfg.Upstream.User,
		},
		Compat: struct {
			StrictBroadcast bool `json:"strict_broadcast"`
			LocalAuthorize  bool `json:"local_authorize"`
		}{
			StrictBroadcast: cfg.Compat.StrictBroadcast,
			LocalAuthorize:  cfg.Compat.LocalAuthorize,
		},
		VarDiffEnabled: cfg.VarDiff.Enabled,
		OnShare: func(cl routing.Client, accepted bool, difficulty float64) {
			if vc, ok := cl.(vardiff.Client); ok {
				vd.RecordShare(vc, accepted, difficulty)
			}
		},
	}
	rt := routing.NewRouter(routingCfg, up, mx)
	nm := nonce.NewManager(up)

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
	}, nil
}

// Reload updates proxy configuration at runtime and forces upstream reconnect
// so host/user/TLS/SOCKS changes take effect.
func (p *Proxy) Reload(newCfg *Config) error {
	log.Println("Reloading configuration...")

	connCfg := &connection.Config{
		Proxy: struct {
			ReadBuf  int `json:"read_buf"`
			WriteBuf int `json:"write_buf"`
		}{
			ReadBuf:  newCfg.Proxy.ReadBuf,
			WriteBuf: newCfg.Proxy.WriteBuf,
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
			Host:               newCfg.Upstream.Host,
			Port:               newCfg.Upstream.Port,
			User:               newCfg.Upstream.User,
			Pass:               newCfg.Upstream.Pass,
			TLS:                newCfg.Upstream.TLS,
			InsecureSkipVerify: newCfg.Upstream.InsecureSkipVerify,
			SocksProxy:         newCfg.Upstream.SocksProxy,
		},
	}
	if err := p.up.ApplyConfig(connCfg); err != nil {
		return fmt.Errorf("reload upstream: %w", err)
	}

	// Update Config (Struct copy)
	*p.cfg = *newCfg

	p.vd.UpdateConfig(&vardiff.Config{
		Enabled:       newCfg.VarDiff.Enabled,
		TargetSeconds: newCfg.VarDiff.TargetSeconds,
		MinDiff:       newCfg.VarDiff.MinDiff,
		MaxDiff:       newCfg.VarDiff.MaxDiff,
		AdjustEveryMs: newCfg.VarDiff.AdjustEveryMs,
	})
	p.rt.SetVarDiffEnabled(newCfg.VarDiff.Enabled)
	p.rt.SetUpstreamUser(newCfg.Upstream.User)
	p.rt.UpdateCompat(newCfg.Compat.StrictBroadcast, newCfg.Compat.LocalAuthorize)

	p.rl.UpdateConfig(&ratelimit.Config{
		Enabled:                 newCfg.RateLimit.Enabled,
		MaxConnectionsPerIP:     newCfg.RateLimit.MaxConnectionsPerIP,
		MaxConnectionsPerMinute: newCfg.RateLimit.MaxConnectionsPerMinute,
		BanDurationSeconds:      newCfg.RateLimit.BanDurationSeconds,
		CleanupIntervalSeconds:  newCfg.RateLimit.CleanupIntervalSeconds,
	})

	// Drop current upstream so UpstreamLoop redials with new settings.
	p.up.Close()
	p.mx.SetUpstreamConnected(false)
	p.nm.Reset()

	log.Println("Configuration reloaded")
	return nil
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
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.worker
}

// GetUpUser returns the upstream user
func (c *Client) GetUpUser() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.upUser
}

// SetWorker sets the worker name
func (c *Client) SetWorker(worker string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.worker = worker
}

// SetUpUser sets the upstream user
func (c *Client) SetUpUser(upUser string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.upUser = upUser
}

// GetExtraNoncePrefix returns the extranonce prefix
func (c *Client) GetExtraNoncePrefix() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.extraNoncePrefix
}

// GetExtraNonceTrim returns the extranonce trim
func (c *Client) GetExtraNonceTrim() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.extraNonceTrim
}

// SetExtraNoncePrefix sets the extranonce prefix
func (c *Client) SetExtraNoncePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.extraNoncePrefix = prefix
}

// SetExtraNonceTrim sets the extranonce trim
func (c *Client) SetExtraNonceTrim(trim int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.extraNonceTrim = trim
}

// GetDiff returns the client's current difficulty
func (c *Client) GetDiff() int64 {
	return atomic.LoadInt64(&c.diff)
}

// SetDiff sets the client's current difficulty
func (c *Client) SetDiff(d int64) {
	atomic.StoreInt64(&c.diff, d)
}

// GetLastAccept returns the last accept timestamp
func (c *Client) GetLastAccept() int64 {
	return atomic.LoadInt64(&c.lastAccept)
}

// UpdateLastAccept updates the last accept timestamp
func (c *Client) UpdateLastAccept(timestamp int64) {
	atomic.StoreInt64(&c.lastAccept, timestamp)
}

// GetOK returns the number of accepted shares
func (c *Client) GetOK() uint64 {
	return atomic.LoadUint64(&c.ok)
}

// GetBad returns the number of rejected shares
func (c *Client) GetBad() uint64 {
	return atomic.LoadUint64(&c.bad)
}

// IncrementOK increments the accepted shares counter
func (c *Client) IncrementOK() {
	atomic.AddUint64(&c.ok, 1)
}

// IncrementBad increments the rejected shares counter
func (c *Client) IncrementBad() {
	atomic.AddUint64(&c.bad, 1)
}

// SetHandshakeDone sets the handshake done flag
func (c *Client) SetHandshakeDone(done bool) {
	var v uint32
	if done {
		v = 1
	}
	atomic.StoreUint32(&c.handshakeDone, v)
}

// WriteJSON writes a JSON message to the client
func (c *Client) WriteJSON(msg stratum.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	c.write.Lock()
	defer c.write.Unlock()
	if _, err = c.bw.Write(data); err != nil {
		return err
	}
	if _, err = c.bw.WriteString("\n"); err != nil {
		return err
	}
	return c.bw.Flush()
}

// WriteLine writes a line to the client
func (c *Client) WriteLine(line string) error {
	c.write.Lock()
	defer c.write.Unlock()
	if _, err := c.bw.WriteString(line); err != nil {
		return err
	}
	if _, err := c.bw.WriteString("\n"); err != nil {
		return err
	}
	return c.bw.Flush()
}

// AcceptLoop accepts new client connections
func (p *Proxy) AcceptLoop(ctx context.Context) error {
	var ln net.Listener
	var err error

	if p.cfg.Proxy.TLS.Enabled {
		var cert tls.Certificate
		cert, err = tls.LoadX509KeyPair(p.cfg.Proxy.TLS.Cert, p.cfg.Proxy.TLS.Key)
		if err != nil {
			return fmt.Errorf("loading tls keys: %w", err)
		}
		tlsCfg := &tls.Config{Certificates: []tls.Certificate{cert}}
		ln, err = tls.Listen("tcp", p.cfg.Proxy.Listen, tlsCfg)
		if err != nil {
			return fmt.Errorf("tls listen: %w", err)
		}
		log.Printf("proxy: listening on %s (TLS enabled)", p.cfg.Proxy.Listen)
	} else {
		ln, err = net.Listen("tcp", p.cfg.Proxy.Listen)
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		log.Printf("proxy: listening on %s", p.cfg.Proxy.Listen)
	}

	go func() {
		<-ctx.Done()
		_ = ln.Close()
		p.CloseClients()
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

		if p.mx.GetClientsActive() >= int64(p.cfg.Proxy.MaxClients) {
			log.Printf("rejecting client: max reached")
			p.rl.ReleaseConnection(conn.RemoteAddr())
			_ = conn.Close()
			continue
		}
		cli := NewClient(conn, p.cfg)
		atomic.StoreInt64(&cli.last, time.Now().UnixMilli())
		atomic.StoreInt64(&cli.diff, int64(p.cfg.VarDiff.MinDiff))

		p.clMu.Lock()
		p.clients[cli] = struct{}{}
		p.clMu.Unlock()

		// Add to all managers
		p.rt.AddClient(cli)
		p.vd.AddClient(cli)
		p.mx.IncrementClients()
		log.Printf("client connected: %s", cli.addr)

		go p.ClientLoop(ctx, cli)
	}
}

// CloseClients closes all active miner connections (used on shutdown).
func (p *Proxy) CloseClients() {
	p.clMu.RLock()
	clients := make([]*Client, 0, len(p.clients))
	for cl := range p.clients {
		clients = append(clients, cl)
	}
	p.clMu.RUnlock()
	for _, cl := range clients {
		_ = cl.c.Close()
	}
}

// ClientLoop handles individual client communication
func (p *Proxy) ClientLoop(ctx context.Context, cl *Client) {
	startTime := time.Now()

	go func() {
		<-ctx.Done()
		_ = cl.c.Close()
	}()

	defer func() {
		p.nm.RemovePendingSubscribe(cl)
		p.rt.RemoveClient(cl)
		p.vd.RemoveClient(cl)
		p.rl.ReleaseConnection(cl.c.RemoteAddr())

		p.clMu.Lock()
		delete(p.clients, cl)
		p.clMu.Unlock()

		p.mx.DecrementClients()
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
		if idle > 0 && atomic.LoadUint32(&cl.handshakeDone) == 0 {
			// Pre-handshake timeout (shorter)
			_ = cl.c.SetReadDeadline(time.Now().Add(time.Duration(idle) * time.Millisecond))
		} else if atomic.LoadUint32(&cl.handshakeDone) == 1 {
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
		atomic.StoreInt64(&cl.last, time.Now().UnixMilli())

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

		// Update upstream target (including per-upstream SOCKS settings)
		if err := p.up.UpdateTarget(
			activeCfg.Host,
			activeCfg.Port,
			activeCfg.User,
			activeCfg.Pass,
			activeCfg.TLS,
			activeCfg.InsecureSkipVerify,
			activeCfg.SocksProxy,
		); err != nil {
			log.Printf("upstream target update fail (idx=%d): %v", currentIdx, err)
			currentIdx = (currentIdx + 1) % len(configs)
			time.Sleep(time.Second)
			continue
		}
		p.rt.SetUpstreamUser(activeCfg.User)

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

		p.mx.SetUpstreamConnected(true)
		log.Printf("upstream connected (idx=%d)", currentIdx)

		// handshake
		if err := p.up.SubscribeAuthorize(); err != nil {
			log.Printf("handshake err: %v", err)
			p.up.Close()
			p.mx.SetUpstreamConnected(false)

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

			// Handle subscribe result by tracked request ID (not hardcoded 1).
			var msg stratum.Message
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}

			if msg.Result != nil && p.up.IsSubscribeResponse(msg) {
				log.Printf("subscribe result: %v", msg.Result)
				p.nm.ProcessSubscribeResult(msg.Result)
			}
		}

		if err := sc.Err(); err != nil && !isNetClosed(err) {
			log.Printf("upstream read err: %v", err)
		}
		p.up.Close()
		p.mx.SetUpstreamConnected(false)
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
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		// Liveness only: upstream may be idle when no miners are connected.
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		type clientView struct {
			IP     string `json:"ip"`
			Worker string `json:"worker"`
			UpUser string `json:"upstream_user"`
			OK     uint64 `json:"ok"`
			Bad    uint64 `json:"bad"`
			Diff   int64  `json:"diff"`
		}
		p.clMu.RLock()
		var clv []clientView
		for cl := range p.clients {
			clv = append(clv, clientView{
				IP:     cl.addr,
				Worker: cl.GetWorker(),
				UpUser: cl.GetUpUser(),
				OK:     atomic.LoadUint64(&cl.ok),
				Bad:    atomic.LoadUint64(&cl.bad),
				Diff:   cl.GetDiff(),
			})
		}
		p.clMu.RUnlock()

		ex1, ex2Size := p.up.GetExtranonce()
		snap := p.mx.Snapshot()
		out := map[string]interface{}{
			"upstream":         snap.UpConnected,
			"extranonce1":      ex1,
			"extranonce2_size": ex2Size,
			"last_notify_unix": snap.LastNotify.Unix(),
			"last_diff":        snap.LastSetDifficulty,
			"shares_ok":        snap.SharesOK,
			"shares_bad":       snap.SharesBad,
			"clients":          clv,
			"vardiff":          p.vd.GetStats(),
			"ratelimit":        p.rl.GetGlobalStats(),
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.Handle("/metrics", promhttp.Handler())

	if p.cfg.HTTP.Pprof {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	srv := &http.Server{Addr: p.cfg.HTTP.Listen, Handler: mux}
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
	lastOK := p.mx.GetSharesOK()
	lastBad := p.mx.GetSharesBad()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			totalOK := p.mx.GetSharesOK()
			totalBad := p.mx.GetSharesBad()
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
			if p.mx.GetClientsActive() == 0 && upstreamRunning {
				if upCancel != nil {
					upCancel()
				}
				upstreamRunning = false
			}
			graceTimer = nil
			graceTimerCh = nil

		case <-ticker.C:
			hasClients := p.mx.GetClientsActive() > 0

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
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "connection reset by peer")
}
