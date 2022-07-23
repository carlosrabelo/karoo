// Package routing handles message routing between clients and upstream
package routing

import (
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/carlosrabelo/karoo/karoo/internal/connection"
	"github.com/carlosrabelo/karoo/karoo/internal/metrics"
	"github.com/carlosrabelo/karoo/karoo/internal/stratum"
)

// Config holds proxy configuration (subset needed for routing)
type Config struct {
	Upstream struct {
		User string `json:"user"`
	} `json:"upstream"`
	Compat struct {
		StrictBroadcast bool `json:"strict_broadcast"`
		// LocalAuthorize answers mining.authorize locally and never
		// forwards miner passwords upstream (pool auth uses upstream.user/pass).
		LocalAuthorize bool `json:"local_authorize"`
	} `json:"compat"`
	// VarDiffEnabled suppresses upstream mining.set_difficulty broadcasts
	// so the local VarDiff controller owns client difficulty.
	VarDiffEnabled bool
	// OnShare is invoked after a mining.submit response is processed.
	OnShare func(cl Client, accepted bool, difficulty float64)
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
	GetDiff() int64
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

// SetVarDiffEnabled toggles whether upstream set_difficulty is broadcast.
func (r *Router) SetVarDiffEnabled(enabled bool) {
	r.cfg.VarDiffEnabled = enabled
}

// SetUpstreamUser updates the upstream worker template used for submits.
func (r *Router) SetUpstreamUser(user string) {
	r.cfg.Upstream.User = user
}

// UpdateCompat updates compatibility flags (strict broadcast / local authorize).
func (r *Router) UpdateCompat(strictBroadcast, localAuthorize bool) {
	r.cfg.Compat.StrictBroadcast = strictBroadcast
	r.cfg.Compat.LocalAuthorize = localAuthorize
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
func (r *Router) ForwardToUpstream(cl Client, method string, params any, id json.RawMessage) bool {
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
		if r.cfg.Compat.LocalAuthorize {
			// Authenticate miners locally; upstream already authorized with
			// upstream.user/pass during SubscribeAuthorize.
			if cl.GetUpUser() == "" {
				cl.SetUpUser(r.cfg.Upstream.User)
			}
			cl.SetHandshakeDone(true)
			r.writeClient(cl, stratum.NewSuccessResponse(msg.ID, true))
			return
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

	// Handle responses (including result:null with error payloads)
	if stratum.IDPresent(msg.ID) && (msg.Result != nil || msg.Error != nil) {
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
		// When VarDiff owns difficulty, do not forward pool set_difficulty.
		if !r.cfg.VarDiffEnabled {
			r.Broadcast(line)
		}

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
				diff := stratum.DiffFromBits(nbits)
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
	upID, ok := stratum.ParseIDInt64(msg.ID)
	if !ok {
		return
	}
	req, exists := r.up.RemovePendingRequest(upID)
	if !exists || req.Client == nil {
		return
	}

	msg.ID = stratum.CopyID(req.OrigID)
	client := req.Client.(Client)
	if err := client.WriteJSON(msg); err != nil {
		log.Printf("response write error to %s: %v", client.GetAddr(), err)
	}

	switch req.Method {
	case "mining.submit":
		r.handleSubmitResponse(req, msg)
	case "mining.authorize":
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
		status, worker, totalShares, totalOK, totalBad, stratum.FormatDuration(sincePrev), latency)

	if r.cfg.OnShare != nil {
		diff := float64(client.GetDiff())
		if diff <= 0 {
			diff = float64(r.mx.GetLastSetDifficulty())
		}
		r.cfg.OnShare(client, success, diff)
	}
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
