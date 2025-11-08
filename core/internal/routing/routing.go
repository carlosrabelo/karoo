// Package routing handles message routing between clients and upstream
package routing

import (
	"encoding/json"
	"log"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/carlosrabelo/karoo/core/internal/connection"
	"github.com/carlosrabelo/karoo/core/internal/metrics"
	"github.com/carlosrabelo/karoo/core/internal/stratum"
)

// Config holds proxy configuration (subset needed for routing)
type Config struct {
	Upstream struct {
		User string `json:"user"`
	} `json:"upstream"`
	Compat struct {
		StrictBroadcast bool `json:"strict_broadcast"`
	} `json:"compat"`
}

// Client represents a mining client interface for routing package
type Client interface {
	GetAddr() string
	GetWorker() string
	GetUpUser() string
	SetWorker(string)
	SetUpUser(string)
	GetExtraNoncePrefix() string
	GetExtraNonceTrim() int
	GetLastAccept() int64
	UpdateLastAccept(int64)
	GetOK() uint64
	GetBad() uint64
	IncrementOK()
	IncrementBad()
	SetHandshakeDone(bool)
	WriteJSON(stratum.Message) error
	WriteLine(string) error
}

// Router manages message routing between upstream and downstream connections
type Router struct {
	cfg *Config
	up  *connection.Upstream
	mx  *metrics.Collector

	clMu    sync.RWMutex
	clients map[Client]struct{}
}

// NewRouter creates a new message router
func NewRouter(cfg *Config, up *connection.Upstream, mx *metrics.Collector) *Router {
	return &Router{
		cfg:     cfg,
		up:      up,
		mx:      mx,
		clients: make(map[Client]struct{}),
	}
}

// AddClient adds a client to the routing table
func (r *Router) AddClient(cl Client) {
	r.clMu.Lock()
	defer r.clMu.Unlock()
	r.clients[cl] = struct{}{}
}

// RemoveClient removes a client from the routing table
func (r *Router) RemoveClient(cl Client) {
	r.clMu.Lock()
	defer r.clMu.Unlock()
	delete(r.clients, cl)
}

// ForwardToUpstream forwards message to upstream with routing
func (r *Router) ForwardToUpstream(cl Client, method string, params any, id *int64) bool {
	if !r.up.IsConnected() {
		r.writeClient(cl, stratum.NewErrorResponse(id, -1, "Upstream down", nil))
		return false
	}
	origID := stratum.CopyID(id)
	upID, err := r.up.Send(stratum.Message{Method: method, Params: params})
	if err != nil {
		r.writeClient(cl, stratum.NewErrorResponse(id, -1, "Forward error", nil))
		return false
	}
	req := connection.PendingReq{
		Client: cl,
		Method: method,
		Sent:   time.Now(),
		OrigID: origID,
	}
	r.up.AddPendingRequest(upID, req)
	return true
}

// Broadcast sends message to all connected clients
func (r *Router) Broadcast(line string) {
	r.clMu.RLock()
	defer r.clMu.RUnlock()
	for cl := range r.clients {
		if err := cl.WriteLine(line); err != nil {
			log.Printf("broadcast write error to %s: %v", cl.GetAddr(), err)
		}
	}
}

// ProcessClientMessage processes a message from a client
func (r *Router) ProcessClientMessage(cl Client, msg stratum.Message) {
	switch msg.Method {
	case "mining.subscribe":
		// This will be handled by the nonce manager
		return

	case "mining.authorize":
		if arr, ok := msg.Params.([]any); ok && len(arr) > 0 {
			if s, ok := arr[0].(string); ok {
				cl.SetWorker(s)
			}
		}
		r.ForwardToUpstream(cl, msg.Method, msg.Params, msg.ID)

	case "mining.submit":
		r.processSubmit(cl, msg)

	default:
		// Generic pass-through for any mining.* call
		if strings.HasPrefix(msg.Method, "mining.") {
			r.ForwardToUpstream(cl, msg.Method, msg.Params, msg.ID)
		}
	}
}

// processSubmit processes mining.submit message with nonce transformation
func (r *Router) processSubmit(cl Client, msg stratum.Message) {
	if arr, ok := msg.Params.([]any); ok && len(arr) > 0 {
		if cl.GetUpUser() == "" {
			cl.SetUpUser(r.cfg.Upstream.User)
		}
		arr[0] = cl.GetUpUser()

		// Handle extranonce transformation
		if len(arr) > 2 && cl.GetExtraNoncePrefix() != "" && cl.GetExtraNonceTrim() > 0 {
			if s, ok := arr[2].(string); ok {
				sUp := strings.ToUpper(s)
				prefix := cl.GetExtraNoncePrefix()
				_, ex2Size := r.up.GetExtranonce()
				expectedLen := (ex2Size - cl.GetExtraNonceTrim()) * 2

				switch {
				case len(sUp) == expectedLen:
					sUp = prefix + sUp
				case len(sUp) == ex2Size*2:
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
	r.ForwardToUpstream(cl, "mining.submit", msg.Params, msg.ID)
}

// ProcessUpstreamMessage processes a message from upstream
func (r *Router) ProcessUpstreamMessage(line string) {
	var msg stratum.Message
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return
	}

	if msg.Method != "" {
		r.processUpstreamNotification(msg, line)
		return
	}

	// Handle responses
	if msg.Result != nil && msg.ID != nil {
		r.processUpstreamResponse(msg)
	}
}

// processUpstreamNotification handles notifications from upstream
func (r *Router) processUpstreamNotification(msg stratum.Message, line string) {
	switch msg.Method {
	case "mining.set_difficulty":
		// Store difficulty in metrics
		if arr, ok := msg.Params.([]any); ok && len(arr) > 0 {
			if v, ok := arr[0].(float64); ok {
				r.mx.SetLastSetDifficulty(int64(v))
			}
		}
		r.Broadcast(line)

	case "mining.notify":
		// Track notify timestamp in metrics
		r.mx.SetLastNotify(time.Now())

		if arr, ok := msg.Params.([]any); ok {
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
		r.Broadcast(line)

	default:
		// Compatibility mode: when strict is off, forward any unrecognized mining.*
		if !r.cfg.Compat.StrictBroadcast && strings.HasPrefix(msg.Method, "mining.") {
			r.Broadcast(line)
		}
	}
}

// processUpstreamResponse handles responses from upstream
func (r *Router) processUpstreamResponse(msg stratum.Message) {
	req, exists := r.up.RemovePendingRequest(*msg.ID)
	if !exists || req.Client == nil {
		return
	}

	if req.OrigID != nil {
		msg.ID = req.OrigID
	} else {
		msg.ID = nil
	}
	client := req.Client.(Client)
	if err := client.WriteJSON(msg); err != nil {
		log.Printf("response write error to %s: %v", client.GetAddr(), err)
	}

	if req.Method == "mining.submit" {
		r.handleSubmitResponse(req, msg)
	} else if req.Method == "mining.authorize" {
		r.handleAuthorizeResponse(req, msg)
	}
}

// handleSubmitResponse handles submit response from upstream
func (r *Router) handleSubmitResponse(req connection.PendingReq, msg stratum.Message) {
	client := req.Client.(Client)
	success := false
	if b, ok := msg.Result.(bool); ok {
		success = b
	}

	// Increment share counters
	if success {
		client.IncrementOK()
		r.mx.IncrementSharesOK()
	} else {
		client.IncrementBad()
		r.mx.IncrementSharesBad()
	}

	latency := time.Since(req.Sent)
	var sincePrev time.Duration
	if success {
		nowMs := time.Now().UnixMilli()
		prev := client.GetLastAccept()
		client.UpdateLastAccept(nowMs)
		if prev > 0 {
			sincePrev = time.Duration(nowMs-prev) * time.Millisecond
		}
	}

	totalOK := client.GetOK()
	totalBad := client.GetBad()
	totalShares := totalOK + totalBad
	status := "Rejected"
	if success {
		status = "Accepted"
	}
	worker := client.GetWorker()
	if worker == "" {
		worker = client.GetAddr()
	}
	log.Printf("share %s worker=%s share=%d ok=%d bad=%d since_prev=%s latency=%s",
		status, worker, totalShares, totalOK, totalBad, fmtDuration(sincePrev), latency)
}

// handleAuthorizeResponse handles authorize response from upstream
func (r *Router) handleAuthorizeResponse(req connection.PendingReq, msg stratum.Message) {
	client := req.Client.(Client)
	if res, ok := msg.Result.(bool); ok && res {
		client.SetHandshakeDone(true)
	}
}

// writeClient writes a message to a client
func (r *Router) writeClient(cl Client, msg stratum.Message) {
	if err := cl.WriteJSON(msg); err != nil {
		log.Printf("client write error to %s: %v", cl.GetAddr(), err)
	}
}

// diffFromBits converts Bitcoin-style difficulty bits (compact target) to difficulty value.
// The nBits format is a compact representation of a 256-bit target threshold.
//
// Format: 0x1d00ffff
//   - First byte (0x1d): exponent (number of bytes in target)
//   - Remaining 3 bytes (0x00ffff): mantissa (coefficient)
//
// Calculation:
//   1. Extract exponent and mantissa from compact format
//   2. Compute target = mantissa * 2^(8*(exponent-3))
//   3. Compute difficulty = difficulty_1_target / target
//
// Where difficulty_1_target = 0xFFFF * 2^(8*(0x1d-3))
//
// Returns 0 for invalid inputs.
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

// fmtDuration formats duration for logging with millisecond precision.
// Returns "-" for zero or negative durations.
// Example: 1.5s for 1500ms, 2m30s for 150 seconds.
func fmtDuration(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	d = d.Round(time.Millisecond)
	return d.String()
}
