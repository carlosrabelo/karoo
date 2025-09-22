// Karoo (Go) - Stratum V1 Proxy
// Author: Carlos Rabelo <contato@carlosrabelo.com.br>

package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/big"
	"math/rand"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const proxyUserAgent = "karoo/v0.0.1"

// ===================== Config =====================

type Config struct {
	Proxy struct {
		Listen       string `json:"listen"`
		ClientIdleMs int    `json:"client_idle_ms"`
		MaxClients   int    `json:"max_clients"`
		ReadBuf      int    `json:"read_buf"`
		WriteBuf     int    `json:"write_buf"`
	} `json:"proxy"`
	Upstream struct {
		Host               string `json:"host"`
		Port               int    `json:"port"`
		User               string `json:"user"`
		Pass               string `json:"pass"`
		TLS                bool   `json:"tls"`
		InsecureSkipVerify bool   `json:"insecure_skip_verify"`
		BackoffMinMs       int    `json:"backoff_min_ms"`
		BackoffMaxMs       int    `json:"backoff_max_ms"`
	} `json:"upstream"`
	HTTP struct {
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
	Compat struct {
		StrictBroadcast bool `json:"strict_broadcast"`
	} `json:"compat"`
}

func loadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}
	// defaults
	if cfg.Proxy.Listen == "" {
		cfg.Proxy.Listen = ":3334"
	}
	if cfg.Proxy.ClientIdleMs == 0 {
		cfg.Proxy.ClientIdleMs = 180_000
	}
	if cfg.Proxy.MaxClients == 0 {
		cfg.Proxy.MaxClients = 512
	}
	if cfg.Proxy.ReadBuf == 0 {
		cfg.Proxy.ReadBuf = 2048
	}
	if cfg.Proxy.WriteBuf == 0 {
		cfg.Proxy.WriteBuf = 2048
	}
	if cfg.Upstream.BackoffMinMs == 0 {
		cfg.Upstream.BackoffMinMs = 1000
	}
	if cfg.Upstream.BackoffMaxMs == 0 {
		cfg.Upstream.BackoffMaxMs = 20000
	}
	if cfg.HTTP.Listen == "" {
		cfg.HTTP.Listen = ":8080"
	}
	if cfg.VarDiff.TargetSeconds == 0 {
		cfg.VarDiff.TargetSeconds = 18
	}
	if cfg.VarDiff.MinDiff == 0 {
		cfg.VarDiff.MinDiff = 8
	}
	if cfg.VarDiff.MaxDiff == 0 {
		cfg.VarDiff.MaxDiff = 16384
	}
	if cfg.VarDiff.AdjustEveryMs == 0 {
		cfg.VarDiff.AdjustEveryMs = 60_000
	}
	return &cfg, nil
}

// ===================== Protocol (Stratum V1) =====================

type JSONMsg struct {
	ID     *int64      `json:"id,omitempty"`
	Method string      `json:"method,omitempty"`
	Params interface{} `json:"params,omitempty"`
	Result interface{} `json:"result,omitempty"`
	Error  interface{} `json:"error,omitempty"`
}

// ===================== Metrics =====================

type Metrics struct {
	UpConnected    atomic.Bool
	SharesOK       atomic.Uint64
	SharesBad      atomic.Uint64
	ClientsActive  atomic.Int64
	LastNotifyUnix atomic.Int64
	LastSetDiff    atomic.Int64
}

// ===================== Upstream =====================

type Upstream struct {
	cfg *Config

	mu   sync.Mutex
	conn net.Conn
	br   *bufio.Reader
	bw   *bufio.Writer

	// extranonce
	ex1     string
	ex2Size int

	// req id para upstream
	reqID atomic.Int64

	// response routing: upID -> client
	respMu  sync.Mutex
	pending map[int64]pendingReq
}

type pendingReq struct {
	cl     *Client
	method string
	sent   time.Time
	origID *int64
}

func parseExtranonceResult(res interface{}) (string, int, bool) {
	switch v := res.(type) {
	case []interface{}:
		if len(v) < 3 {
			return "", 0, false
		}
		ex1, ok1 := v[1].(string)
		ex2, ok2 := parseExtranonceSize(v[2])
		if !ok1 || !ok2 {
			return "", 0, false
		}
		return ex1, ex2, ex1 != "" && ex2 > 0
	case map[string]interface{}:
		ex1Raw, ok1 := v["extranonce1"]
		ex2Raw, ok2 := v["extranonce2_size"]
		if !ok1 || !ok2 {
			return "", 0, false
		}
		ex1, ok1 := ex1Raw.(string)
		ex2, ok2 := parseExtranonceSize(ex2Raw)
		if !ok1 || !ok2 {
			return "", 0, false
		}
		return ex1, ex2, ex1 != "" && ex2 > 0
	default:
		return "", 0, false
	}
}

func parseExtranonceSize(v interface{}) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), int(t) > 0
	case string:
		if t == "" {
			return 0, false
		}
		n, err := strconv.Atoi(t)
		if err != nil {
			return 0, false
		}
		return n, n > 0
	default:
		return 0, false
	}
}

func fmtDuration(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	d = d.Round(time.Millisecond)
	return d.String()
}

func diffFromBits(bits string) float64 {
	bits = strings.TrimPrefix(bits, "0x")
	if bits == "" {
		return 0
	}
	val, err := strconv.ParseUint(bits, 16, 32)
	if err != nil {
		return 0
	}
	exponent := byte(val >> 24)
	mantissa := val & 0xFFFFFF
	if mantissa == 0 || exponent <= 3 {
		return 0
	}
	target := new(big.Int).Lsh(big.NewInt(int64(mantissa)), uint(8*(int(exponent)-3)))
	if target.Sign() <= 0 {
		return 0
	}
	diffOne := new(big.Int).Lsh(big.NewInt(0xFFFF), uint(8*(0x1d-3)))
	t := new(big.Float).SetInt(target)
	d := new(big.Float).SetInt(diffOne)
	res := new(big.Float).Quo(d, t)
	out, _ := res.Float64()
	return out
}

func (u *Upstream) dial(ctx context.Context) error {
	addr := net.JoinHostPort(u.cfg.Upstream.Host, strconv.Itoa(u.cfg.Upstream.Port))
	var c net.Conn
	var err error
	if u.cfg.Upstream.TLS {
		conf := &tls.Config{InsecureSkipVerify: u.cfg.Upstream.InsecureSkipVerify}
		c, err = tls.Dial("tcp", addr, conf)
	} else {
		c, err = net.DialTimeout("tcp", addr, 10*time.Second)
	}
	if err != nil {
		return err
	}
	u.mu.Lock()
	u.conn = c
	u.br = bufio.NewReaderSize(c, u.cfg.Proxy.ReadBuf)
	u.bw = bufio.NewWriterSize(c, u.cfg.Proxy.WriteBuf)
	u.mu.Unlock()
	u.respMu.Lock()
	u.pending = make(map[int64]pendingReq)
	u.respMu.Unlock()
	return nil
}

func (u *Upstream) close() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.conn != nil {
		_ = u.conn.Close()
		u.conn = nil
		u.br = nil
		u.bw = nil
	}
}

func (u *Upstream) sendRaw(line string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.conn == nil {
		return errors.New("upstream nil")
	}
	if _, err := u.bw.WriteString(line); err != nil {
		return err
	}
	return u.bw.Flush()
}

func (u *Upstream) send(msg JSONMsg) (int64, error) {
	i := u.reqID.Add(1)
	msg.ID = &i
	b, _ := json.Marshal(msg)
	b = append(b, '\n')
	return i, u.sendRaw(string(b))
}

func (u *Upstream) subscribeAuthorize() error {
	if _, err := u.send(JSONMsg{Method: "mining.subscribe", Params: []interface{}{proxyUserAgent}}); err != nil {
		return err
	}
	_, err := u.send(JSONMsg{Method: "mining.authorize", Params: []interface{}{u.cfg.Upstream.User, u.cfg.Upstream.Pass}})
	return err
}

func (u *Upstream) isConnected() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.conn != nil
}

// ===================== Clients =====================

type Client struct {
	c    net.Conn
	br   *bufio.Reader
	bw   *bufio.Writer
	addr string

	last atomic.Int64
	ok   atomic.Uint64
	bad  atomic.Uint64

	// desligar timeout depois do handshake
	handshakeDone atomic.Bool

	worker           string
	upUser           string
	lastAccept       atomic.Int64
	extraNoncePrefix string
	extraNonceTrim   int

	// vardiff (placeholder simples)
	diff atomic.Int64

	writeMu sync.Mutex
}

func (cl *Client) writeRaw(b []byte) error {
	cl.writeMu.Lock()
	defer cl.writeMu.Unlock()
	if cl.bw == nil {
		return errors.New("client writer nil")
	}
	if _, err := cl.bw.Write(b); err != nil {
		return err
	}
	return cl.bw.Flush()
}

func (cl *Client) writeJSON(obj JSONMsg) error {
	b, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return cl.writeRaw(b)
}

func (cl *Client) writeLine(line string) error {
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	return cl.writeRaw([]byte(line))
}

// ===================== Proxy Core =====================

type Proxy struct {
	cfg *Config
	up  *Upstream
	mx  *Metrics

	clMu          sync.RWMutex
	clients       map[*Client]struct{}
	prefixCounter atomic.Uint64

	// upstream readiness for client subscribe responses
	upReady atomic.Bool
	readyMu sync.Mutex
	readyCh chan struct{}

	subMu       sync.Mutex
	pendingSubs map[*Client]*int64
}

func NewProxy(cfg *Config) *Proxy {
	return &Proxy{
		cfg:         cfg,
		up:          &Upstream{cfg: cfg},
		mx:          &Metrics{},
		clients:     make(map[*Client]struct{}),
		readyCh:     make(chan struct{}),
		pendingSubs: make(map[*Client]*int64),
	}
}

func copyID(id *int64) *int64 {
	if id == nil {
		return nil
	}
	dup := new(int64)
	*dup = *id
	return dup
}

func (p *Proxy) upstreamReady() bool {
	return p.upReady.Load() && p.up.ex2Size > 0 && p.up.ex1 != ""
}

func (p *Proxy) enqueuePendingSubscribe(cl *Client, id *int64) {
	if p.upstreamReady() {
		p.respondSubscribe(cl, copyID(id))
		return
	}
	copy := copyID(id)
	p.subMu.Lock()
	defer p.subMu.Unlock()
	if p.pendingSubs == nil {
		p.pendingSubs = make(map[*Client]*int64)
	}
	// single pending subscribe per client; latest ID wins
	p.pendingSubs[cl] = copy
}

func (p *Proxy) removePendingSubscribe(cl *Client) {
	p.subMu.Lock()
	defer p.subMu.Unlock()
	delete(p.pendingSubs, cl)
}

func (p *Proxy) flushPendingSubscribes() {
	p.subMu.Lock()
	if len(p.pendingSubs) == 0 {
		p.subMu.Unlock()
		return
	}
	pending := make(map[*Client]*int64, len(p.pendingSubs))
	for cl, id := range p.pendingSubs {
		pending[cl] = id
	}
	// reset map so new subscribers can queue while we reply
	p.pendingSubs = make(map[*Client]*int64)
	p.subMu.Unlock()

	for cl, id := range pending {
		p.respondSubscribe(cl, id)
	}
}

func (p *Proxy) respondSubscribe(cl *Client, id *int64) {
	if !p.upstreamReady() {
		p.enqueuePendingSubscribe(cl, id)
		return
	}
	p.assignNoncePrefix(cl)
	ex1Resp := p.up.ex1
	ex2Resp := p.up.ex2Size
	if cl.extraNoncePrefix != "" && cl.extraNonceTrim > 0 {
		if p.up.ex2Size > cl.extraNonceTrim {
			ex1Resp = ex1Resp + cl.extraNoncePrefix
			ex2Resp = p.up.ex2Size - cl.extraNonceTrim
		} else {
			cl.extraNoncePrefix = ""
			cl.extraNonceTrim = 0
		}
	}
	resp := JSONMsg{ID: id, Result: []interface{}{[]interface{}{}, ex1Resp, ex2Resp}}
	p.writeClient(cl, resp)
}

// ====== Accept loop ======
func (p *Proxy) acceptLoop(ctx context.Context) error {
	ln, err := net.Listen("tcp", p.cfg.Proxy.Listen)
	if err != nil {
		return err
	}
	log.Printf("proxy: listening on %s", p.cfg.Proxy.Listen)
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
		if p.mx.ClientsActive.Load() >= int64(p.cfg.Proxy.MaxClients) {
			log.Printf("rejecting client: max reached")
			_ = conn.Close()
			continue
		}
		cli := &Client{
			c:      conn,
			br:     bufio.NewReaderSize(conn, p.cfg.Proxy.ReadBuf),
			bw:     bufio.NewWriterSize(conn, p.cfg.Proxy.WriteBuf),
			addr:   conn.RemoteAddr().String(),
			upUser: p.cfg.Upstream.User,
		}
		cli.last.Store(time.Now().UnixMilli())
		cli.diff.Store(int64(p.cfg.VarDiff.MinDiff))

		p.clMu.Lock()
		p.clients[cli] = struct{}{}
		p.clMu.Unlock()
		p.mx.ClientsActive.Add(1)
		log.Printf("client connected: %s", cli.addr)

		go p.clientLoop(ctx, cli)
	}
}

// ====== Client protocol ======
func (p *Proxy) clientLoop(ctx context.Context, cl *Client) {
	defer func() {
		p.removePendingSubscribe(cl)
		p.clMu.Lock()
		delete(p.clients, cl)
		p.clMu.Unlock()
		p.mx.ClientsActive.Add(-1)
		_ = cl.c.Close()
		log.Printf("client closed: %s", cl.addr)
	}()

	sc := bufio.NewScanner(cl.br)
	buf := make([]byte, 0, p.cfg.Proxy.ReadBuf)
	sc.Buffer(buf, 1024*1024)

	idle := p.cfg.Proxy.ClientIdleMs
	for {
		if idle > 0 && !cl.handshakeDone.Load() {
			_ = cl.c.SetReadDeadline(time.Now().Add(time.Duration(idle) * time.Millisecond))
		} else {
			_ = cl.c.SetReadDeadline(time.Time{})
		}
		if !sc.Scan() {
			if err := sc.Err(); err != nil && !errors.Is(err, net.ErrClosed) {
				log.Printf("client scan err %s: %v", cl.addr, err)
			}
			return
		}
		line := sc.Text()
		cl.last.Store(time.Now().UnixMilli())

		var msg JSONMsg
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		switch msg.Method {
		case "mining.subscribe":
			if p.upstreamReady() {
				p.respondSubscribe(cl, copyID(msg.ID))
				break
			}
			p.enqueuePendingSubscribe(cl, msg.ID)
			continue

		case "mining.authorize":
			if arr, ok := msg.Params.([]interface{}); ok && len(arr) > 0 {
				if s, ok := arr[0].(string); ok {
					cl.worker = s
				}
			}
			p.forwardToUpstream(cl, msg.Method, msg.Params, msg.ID)
			break

		case "mining.submit":
			if arr, ok := msg.Params.([]interface{}); ok && len(arr) > 0 {
				if cl.upUser == "" {
					cl.upUser = p.cfg.Upstream.User
				}
				arr[0] = cl.upUser
				if len(arr) > 2 && cl.extraNoncePrefix != "" && cl.extraNonceTrim > 0 {
					if s, ok := arr[2].(string); ok {
						sUp := strings.ToUpper(s)
						prefix := cl.extraNoncePrefix
						expectedLen := (p.up.ex2Size - cl.extraNonceTrim) * 2
						switch {
						case len(sUp) == expectedLen:
							sUp = prefix + sUp
						case len(sUp) == p.up.ex2Size*2:
							if !strings.HasPrefix(sUp, prefix) {
								sUp = prefix + sUp[len(prefix):]
							}
						default:
							if !strings.HasPrefix(sUp, prefix) {
								sUp = prefix + sUp
							}
						}
						arr[2] = sUp
					}
				}
				msg.Params = arr
			}
			p.forwardToUpstream(cl, "mining.submit", msg.Params, msg.ID)
			break
		default:
			// Generic pass-through for any mining.* call (e.g., mining.configure, extranonce.subscribe, suggest_*)
			if strings.HasPrefix(msg.Method, "mining.") {
				p.forwardToUpstream(cl, msg.Method, msg.Params, msg.ID)
				break
			}
			// Ignore anything that is not mining.* (or log if desired)
		}
	}
}

func (p *Proxy) writeClient(cl *Client, obj JSONMsg) {
	_ = cl.writeJSON(obj)
}

func (p *Proxy) forwardToUpstream(cl *Client, method string, params interface{}, id *int64) bool {
	if !p.up.isConnected() {
		p.writeClient(cl, JSONMsg{ID: id, Result: false, Error: []interface{}{-1, "Upstream down", nil}})
		return false
	}
	origID := copyID(id)
	upID, err := p.up.send(JSONMsg{Method: method, Params: params})
	if err != nil {
		p.writeClient(cl, JSONMsg{ID: id, Result: false, Error: []interface{}{-1, "Forward error", nil}})
		return false
	}
	p.up.respMu.Lock()
	p.up.pending[upID] = pendingReq{cl: cl, method: method, sent: time.Now(), origID: origID}
	p.up.respMu.Unlock()
	return true
}

// ====== Upstream loop ======
func (p *Proxy) upstreamLoop(ctx context.Context) {
	min := time.Duration(pcfg.Upstream.BackoffMinMs) * time.Millisecond
	max := time.Duration(pcfg.Upstream.BackoffMaxMs) * time.Millisecond

	for ctx.Err() == nil {
		if err := p.up.dial(ctx); err != nil {
			d := backoff(min, max)
			log.Printf("upstream dial fail: %v; retry in %s", err, d)
			time.Sleep(d)
			continue
		}
		p.mx.UpConnected.Store(true)
		log.Printf("upstream connected")

		// handshake
		if err := p.up.subscribeAuthorize(); err != nil {
			log.Printf("handshake err: %v", err)
			p.up.close()
			p.mx.UpConnected.Store(false)
			continue
		}

		sc := bufio.NewScanner(p.up.br)
		buf := make([]byte, 0, p.cfg.Proxy.ReadBuf)
		sc.Buffer(buf, 1024*1024)

		for sc.Scan() {
			line := sc.Text()

			var msg JSONMsg
			if err := json.Unmarshal([]byte(line), &msg); err != nil {
				continue
			}

			if msg.Method != "" {
				switch msg.Method {
				case "mining.set_difficulty":
					// guardar diff (opcional)
					if arr, ok := msg.Params.([]interface{}); ok && len(arr) > 0 {
						if v, ok := arr[0].(float64); ok {
							p.mx.LastSetDiff.Store(int64(v))
						}
					}
					p.broadcast(line)

				case "mining.notify":
					p.mx.LastNotifyUnix.Store(time.Now().Unix())
					if arr, ok := msg.Params.([]interface{}); ok {
						var jobID, nbits string
						var clean bool
						if len(arr) > 0 {
							if s, ok := arr[0].(string); ok {
								jobID = s
							}
						}
						if len(arr) > 6 {
							if s, ok := arr[6].(string); ok {
								nbits = s
							}
						}
						if len(arr) > 8 {
							switch v := arr[8].(type) {
							case bool:
								clean = v
							case string:
								clean = strings.EqualFold(v, "true")
							}
						}
						if clean {
							diff := diffFromBits(nbits)
							log.Printf("new job job=%s diff=%.6g", jobID, diff)
						}
					}
					p.broadcast(line)

				default:
					// Compatibility mode: when strict is off, forward any unrecognized mining.*
					if !p.cfg.Compat.StrictBroadcast && strings.HasPrefix(msg.Method, "mining.") {
						p.broadcast(line)
					}
				}
				continue
			}

			// Responses (submit/subscribe/authorize)
			if msg.Result != nil && msg.ID != nil {
				// Capturar extranonce do subscribe (id = 1)
				if *msg.ID == 1 {
					log.Printf("subscribe result: %v", msg.Result)
					ex1, ex2, ok := parseExtranonceResult(msg.Result)
					if ok {
						p.up.ex1 = ex1
						p.up.ex2Size = ex2
						// mark upstream as ready for subscribers
						p.readyMu.Lock()
						if !p.upReady.Load() {
							p.upReady.Store(true)
							close(p.readyCh)
						}
						p.readyMu.Unlock()
						p.flushPendingSubscribes()
					} else if !p.upReady.Load() {
						log.Printf("subscribe result missing extranonce fields: %v", msg.Result)
					}
				}

				// Roteamento de submit/config/etc.
				p.up.respMu.Lock()
				req := p.up.pending[*msg.ID]
				delete(p.up.pending, *msg.ID)
				p.up.respMu.Unlock()
				if req.cl != nil {
					if req.origID != nil {
						msg.ID = req.origID
					} else {
						msg.ID = nil
					}
					_ = req.cl.writeJSON(msg)
					if req.method == "mining.submit" {
						success := false
						if b, ok := msg.Result.(bool); ok {
							success = b
						}
						if success {
							p.mx.SharesOK.Add(1)
							req.cl.ok.Add(1)
						} else {
							p.mx.SharesBad.Add(1)
							req.cl.bad.Add(1)
						}
						latency := time.Since(req.sent)
						var sincePrev time.Duration
						if success {
							nowMs := time.Now().UnixMilli()
							prev := req.cl.lastAccept.Swap(nowMs)
							if prev > 0 {
								sincePrev = time.Duration(nowMs-prev) * time.Millisecond
							}
						}
						totalOK := req.cl.ok.Load()
						totalBad := req.cl.bad.Load()
						totalShares := totalOK + totalBad
						status := "Rejected"
						if success {
							status = "Accepted"
						}
						worker := req.cl.worker
						if worker == "" {
							worker = req.cl.addr
						}
						log.Printf("share %s worker=%s share=%d ok=%d bad=%d since_prev=%s latency=%s", status, worker, totalShares, totalOK, totalBad, fmtDuration(sincePrev), latency)
					}
					if req.method == "mining.authorize" {
						if res, ok := msg.Result.(bool); ok && res {
							req.cl.handshakeDone.Store(true)
						}
					}
				}
			}
		}

		if err := sc.Err(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("upstream read err: %v", err)
		}
		p.up.close()
		p.mx.UpConnected.Store(false)

		// reset readiness flags when upstream disconnects
		p.readyMu.Lock()
		p.upReady.Store(false)
		p.readyCh = make(chan struct{})
		p.readyMu.Unlock()

		// backoff antes de redial (se o manager ainda quiser upstream)
		d := backoff(min, max)
		log.Printf("upstream disconnected; retry in %s", d)
		time.Sleep(d)
	}
}

func (p *Proxy) broadcast(line string) {
	p.clMu.RLock()
	defer p.clMu.RUnlock()
	for cl := range p.clients {
		_ = cl.writeLine(line)
	}
}

// ===================== VarDiff (basic) =====================
func (p *Proxy) vardiffLoop(ctx context.Context) {
	if !p.cfg.VarDiff.Enabled {
		return
	}
	t := time.NewTicker(time.Duration(p.cfg.VarDiff.AdjustEveryMs) * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.adjustDiffOnce()
		}
	}
}

func (p *Proxy) adjustDiffOnce() {
	p.clMu.RLock()
	defer p.clMu.RUnlock()
	for cl := range p.clients {
		// Placeholder heuristic: emits the stored difficulty (room to evolve into moving window).
		d := cl.diff.Load()
		if d <= 0 {
			d = int64(p.cfg.VarDiff.MinDiff)
		}
		msg := JSONMsg{Method: "mining.set_difficulty", Params: []interface{}{float64(d)}}
		_ = cl.writeJSON(msg)
	}
}

// ===================== HTTP (status/health) =====================
func (p *Proxy) httpServe(ctx context.Context) {
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
		out := map[string]interface{}{
			"upstream":         p.mx.UpConnected.Load(),
			"extranonce1":      p.up.ex1,
			"extranonce2_size": p.up.ex2Size,
			"last_notify_unix": p.mx.LastNotifyUnix.Load(),
			"last_diff":        p.mx.LastSetDiff.Load(),
			"shares_ok":        p.mx.SharesOK.Load(),
			"shares_bad":       p.mx.SharesBad.Load(),
			"clients":          clv,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
	srv := &http.Server{Addr: p.cfg.HTTP.Listen}
	go func() {
		<-ctx.Done()
		ctx2, _ := context.WithTimeout(context.Background(), 2*time.Second)
		_ = srv.Shutdown(ctx2)
	}()
	log.Printf("http: listening on %s", p.cfg.HTTP.Listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("http err: %v", err)
	}
}

func (p *Proxy) reportLoop(ctx context.Context, interval time.Duration) {
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

// ===================== Upstream manager =====================
// Connect to the upstream ONLY while there is at least one active client.
// Disconnect after a configurable grace period without clients.
func (p *Proxy) upstreamManager(ctx context.Context, idleGrace time.Duration) {
	var upCancel context.CancelFunc
	var upCtx context.Context
	upstreamRunning := false

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			if upstreamRunning {
				upCancel()
			}
			return
		case <-ticker.C:
			haveClients := p.mx.ClientsActive.Load() > 0
			if haveClients && !upstreamRunning {
				// start upstream loop under a child context
				upCtx, upCancel = context.WithCancel(ctx)
				go p.upstreamLoop(upCtx)
				upstreamRunning = true
				log.Printf("up-manager: clients present -> upstream started")
			}
			if upstreamRunning && !haveClients {
				// grace period; abort if a client reappears
				deadline := time.Now().Add(idleGrace)
				for time.Now().Before(deadline) {
					if p.mx.ClientsActive.Load() > 0 || ctx.Err() != nil {
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
				if p.mx.ClientsActive.Load() == 0 && ctx.Err() == nil {
					upCancel()
					upstreamRunning = false
					p.up.close()
					p.mx.UpConnected.Store(false)

					// reset readiness when we deliberately stop the upstream
					p.readyMu.Lock()
					p.upReady.Store(false)
					p.readyCh = make(chan struct{})
					p.readyMu.Unlock()

					log.Printf("up-manager: no clients for %s -> upstream stopped", idleGrace)
				}
			}
		}
	}
}

// ===================== Utils/Globals =====================

var (
	pcfg *Config
)

const extraNoncePrefixBytes = 1

func (p *Proxy) assignNoncePrefix(cl *Client) {
	if cl.extraNoncePrefix != "" {
		return
	}
	if extraNoncePrefixBytes <= 0 {
		return
	}
	if p.up.ex2Size <= extraNoncePrefixBytes {
		return
	}
	bits := extraNoncePrefixBytes * 8
	if bits <= 0 || bits >= 64 {
		return
	}
	mask := (uint64(1) << bits) - 1
	if mask == 0 {
		return
	}
	val := p.prefixCounter.Add(1) & mask
	cl.extraNoncePrefix = fmt.Sprintf("%0*X", extraNoncePrefixBytes*2, val)
	cl.extraNonceTrim = extraNoncePrefixBytes
}

func backoff(min, max time.Duration) time.Duration {
	d := min
	if max > min {
		mul := 1 << (rand.Intn(4)) // 1,2,4,8
		d = time.Duration(int(min) * mul)
		if d > max {
			d = max
		}
	}
	return d + time.Duration(rand.Intn(250))*time.Millisecond
}

// ===================== main =====================
func main() {
	rand.Seed(time.Now().UnixNano())
	cfgPath := flag.String("config", "config.json", "config file path")
	idleGraceMs := flag.Int("idle_grace_ms", 15000, "milliseconds to keep upstream alive with zero clients")
	strictBroadcast := flag.Bool("strict_broadcast", false, "if true, only broadcast known mining methods (notify,set_difficulty)")
	subscribeWaitMs := flag.Int("subscribe_wait_ms", 0, "deprecated: retained for backwards compatibility")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	cfg.Compat.StrictBroadcast = *strictBroadcast
	pcfg = cfg
	if *subscribeWaitMs > 0 {
		log.Printf("subscribe_wait_ms is deprecated and no longer affects subscribe handling (ignored value: %dms)", *subscribeWaitMs)
	}

	px := NewProxy(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// signals
	go func() {
		ch := make(chan os.Signal, 2)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		log.Printf("signal: shutting down...")
		cancel()
	}()

	// Manager that controls the upstream lifecycle based on the number of clients
	go px.upstreamManager(ctx, time.Duration(*idleGraceMs)*time.Millisecond)

	// VarDiff & HTTP independem do estado do upstream
	go px.vardiffLoop(ctx)
	go px.httpServe(ctx)
	go px.reportLoop(ctx, 5*time.Minute)

	if err := px.acceptLoop(ctx); err != nil {
		log.Printf("accept loop end: %v", err)
	}
	log.Printf("bye")
}
