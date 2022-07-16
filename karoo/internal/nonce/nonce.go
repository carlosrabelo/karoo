// Package nonce manages extranonce allocation and subscription handling
package nonce

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"github.com/carlosrabelo/karoo/karoo/internal/connection"
	"github.com/carlosrabelo/karoo/karoo/internal/stratum"
)

// Client represents a mining client interface for nonce package
type Client interface {
	GetExtraNoncePrefix() string
	GetExtraNonceTrim() int
	SetExtraNoncePrefix(string)
	SetExtraNonceTrim(int)
	WriteJSON(stratum.Message) error
}

// Manager handles extranonce allocation and subscription queue
type Manager struct {
	up *connection.Upstream

	// upstream readiness for client subscribe responses
	upReady uint32 // 0/1
	readyMu sync.Mutex
	readyCh chan struct{}

	subMu       sync.Mutex
	pendingSubs map[Client]json.RawMessage

	// extranonce prefix allocation
	prefixCounter uint64
}

// NewManager creates a new nonce manager
func NewManager(up *connection.Upstream) *Manager {
	return &Manager{
		up:          up,
		readyCh:     make(chan struct{}),
		pendingSubs: make(map[Client]json.RawMessage),
	}
}

// UpstreamReady checks if upstream is ready for subscriptions
func (m *Manager) UpstreamReady() bool {
	ex1, ex2Size := m.up.GetExtranonce()
	return atomic.LoadUint32(&m.upReady) == 1 && ex2Size > 0 && ex1 != ""
}

// EnqueuePendingSubscribe adds client to pending subscribe queue
func (m *Manager) EnqueuePendingSubscribe(cl Client, id json.RawMessage) {
	copyID := stratum.CopyID(id)

	// Check readiness first to avoid unnecessary locking in common case
	if m.UpstreamReady() {
		m.RespondSubscribeIfReady(cl, copyID)
		return
	}

	m.subMu.Lock()

	// Double-check readiness while holding lock to prevent TOCTOU race
	if m.UpstreamReady() {
		m.subMu.Unlock()
		m.RespondSubscribeIfReady(cl, copyID)
		return
	}

	if m.pendingSubs == nil {
		m.pendingSubs = make(map[Client]json.RawMessage)
	}
	// single pending subscribe per client; latest ID wins
	m.pendingSubs[cl] = copyID
	m.subMu.Unlock()
}

// RemovePendingSubscribe removes client from pending subscribe queue
func (m *Manager) RemovePendingSubscribe(cl Client) {
	m.subMu.Lock()
	defer m.subMu.Unlock()
	delete(m.pendingSubs, cl)
}

// FlushPendingSubscribes responds to all pending subscribes
func (m *Manager) FlushPendingSubscribes() {
	m.subMu.Lock()
	if len(m.pendingSubs) == 0 {
		m.subMu.Unlock()
		return
	}
	pending := make(map[Client]json.RawMessage, len(m.pendingSubs))
	for cl, id := range m.pendingSubs {
		pending[cl] = id
	}
	// reset map so new subscribers can queue while we reply
	m.pendingSubs = make(map[Client]json.RawMessage)
	m.subMu.Unlock()

	for cl, id := range pending {
		m.RespondSubscribe(cl, id)
	}
}

// RespondSubscribe responds to mining.subscribe request
// If upstream is not ready, enqueues the request
func (m *Manager) RespondSubscribe(cl Client, id json.RawMessage) {
	if !m.UpstreamReady() {
		m.EnqueuePendingSubscribe(cl, id)
		return
	}
	m.RespondSubscribeIfReady(cl, id)
}

// RespondSubscribeIfReady responds immediately without checking readiness
// Used when caller has already verified upstream is ready
func (m *Manager) RespondSubscribeIfReady(cl Client, id json.RawMessage) {
	m.AssignNoncePrefix(cl)
	ex1Resp, ex2Resp := m.GetClientExtranonce(cl)
	resp := stratum.NewSuccessResponse(id, []interface{}{[]interface{}{}, ex1Resp, ex2Resp})
	m.WriteClient(cl, resp)
}

// AssignNoncePrefix assigns a unique extranonce prefix to client
func (m *Manager) AssignNoncePrefix(cl Client) {
	if cl.GetExtraNoncePrefix() != "" {
		return
	}
	const extraNoncePrefixBytes = 1
	if extraNoncePrefixBytes <= 0 {
		return
	}
	_, ex2Size := m.up.GetExtranonce()
	if ex2Size <= extraNoncePrefixBytes {
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
	val := atomic.AddUint64(&m.prefixCounter, 1) & mask
	prefix := fmt.Sprintf("%0*X", extraNoncePrefixBytes*2, val)
	cl.SetExtraNoncePrefix(prefix)
	cl.SetExtraNonceTrim(extraNoncePrefixBytes)
}

// GetClientExtranonce returns the extranonce values for a specific client
func (m *Manager) GetClientExtranonce(cl Client) (string, int) {
	ex1, ex2Size := m.up.GetExtranonce()
	ex1Resp := ex1
	ex2Resp := ex2Size

	if cl.GetExtraNoncePrefix() != "" && cl.GetExtraNonceTrim() > 0 {
		if ex2Size > cl.GetExtraNonceTrim() {
			ex1Resp = ex1Resp + cl.GetExtraNoncePrefix()
			ex2Resp = ex2Size - cl.GetExtraNonceTrim()
		} else {
			cl.SetExtraNoncePrefix("")
			cl.SetExtraNonceTrim(0)
		}
	}
	return ex1Resp, ex2Resp
}

// SetUpstreamReady marks upstream as ready and flushes pending subscribes
func (m *Manager) SetUpstreamReady(ready bool) {
	m.readyMu.Lock()
	defer m.readyMu.Unlock()
	if ready && atomic.LoadUint32(&m.upReady) == 0 {
		atomic.StoreUint32(&m.upReady, 1)
		close(m.readyCh)
		m.FlushPendingSubscribes()
	} else if !ready {
		atomic.StoreUint32(&m.upReady, 0)
		m.readyCh = make(chan struct{})
	}
}

// GetReadyChannel returns the ready channel for waiting
func (m *Manager) GetReadyChannel() <-chan struct{} {
	m.readyMu.Lock()
	defer m.readyMu.Unlock()
	return m.readyCh
}

// ProcessSubscribeResult processes the result from upstream subscribe
func (m *Manager) ProcessSubscribeResult(result interface{}) {
	info := stratum.ParseExtranonceResult(result)
	if info.Valid {
		m.up.SetExtranonce(info.Extranonce1, info.Extranonce2Size)
		m.SetUpstreamReady(true)
		log.Printf("upstream extranonce: ex1=%s ex2_size=%d", info.Extranonce1, info.Extranonce2Size)
	} else if atomic.LoadUint32(&m.upReady) == 0 {
		log.Printf("warning: invalid subscribe result from upstream")
	}
}

// WriteClient writes a message to a client
func (m *Manager) WriteClient(cl Client, msg stratum.Message) {
	if err := cl.WriteJSON(msg); err != nil {
		log.Printf("nonce: write error to client: %v", err)
	}
}

// Reset resets the nonce manager state for a new upstream session.
// The prefix counter is intentionally kept so reconnects do not reissue
// prefixes already assigned to live clients.
func (m *Manager) Reset() {
	m.SetUpstreamReady(false)

	m.subMu.Lock()
	m.pendingSubs = make(map[Client]json.RawMessage)
	m.subMu.Unlock()
}
