// Package connection manages upstream and downstream network connections
package connection

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/carlosrabelo/karoo/core/internal/stratum"
)

// Config holds proxy configuration (subset needed for connection)
type Config struct {
	Proxy struct {
		ReadBuf  int `json:"read_buf"`
		WriteBuf int `json:"write_buf"`
	} `json:"proxy"`
	Upstream struct {
		Host               string `json:"host"`
		Port               int    `json:"port"`
		User               string `json:"user"`
		Pass               string `json:"pass"`
		TLS                bool   `json:"tls"`
		InsecureSkipVerify bool   `json:"insecure_skip_verify"`
	} `json:"upstream"`
}

// Client represents a mining client interface for connection package
type Client interface {
	GetAddr() string
	GetWorker() string
	GetUpUser() string
}

// Upstream manages connection to upstream pool
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
	reqID int64

	// response routing: upID -> client
	respMu  sync.Mutex
	pending map[int64]PendingReq
}

// PendingReq represents a pending upstream request
type PendingReq struct {
	Client interface{} // Will be routing.Client
	Method string
	Sent   time.Time
	OrigID *int64
}

// Downstream represents a downstream mining client connection
type Downstream struct {
	Conn   net.Conn
	Reader *bufio.Reader
	Writer *bufio.Writer
	Addr   string
}

// NewUpstream creates a new upstream connection manager
func NewUpstream(cfg *Config) *Upstream {
	return &Upstream{
		cfg:     cfg,
		pending: make(map[int64]PendingReq),
	}
}

// NewDownstream creates a new downstream connection wrapper
func NewDownstream(conn net.Conn, cfg *Config) *Downstream {
	return &Downstream{
		Conn:   conn,
		Reader: bufio.NewReaderSize(conn, cfg.Proxy.ReadBuf),
		Writer: bufio.NewWriterSize(conn, cfg.Proxy.WriteBuf),
		Addr:   conn.RemoteAddr().String(),
	}
}

// Dial establishes connection to upstream pool
func (u *Upstream) Dial(ctx context.Context) error {
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
	u.pending = make(map[int64]PendingReq)
	u.respMu.Unlock()
	return nil
}

// Close closes upstream connection
func (u *Upstream) Close() {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.conn != nil {
		_ = u.conn.Close()
		u.conn = nil
		u.br = nil
		u.bw = nil
	}
}

// IsConnected checks if upstream is connected
func (u *Upstream) IsConnected() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.conn != nil
}

// SendRaw sends raw data to upstream
func (u *Upstream) SendRaw(line string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.conn == nil {
		return fmt.Errorf("upstream nil")
	}
	if _, err := u.bw.WriteString(line); err != nil {
		return err
	}
	return u.bw.Flush()
}

// Send sends JSON message to upstream
func (u *Upstream) Send(msg stratum.Message) (int64, error) {
	u.reqID++
	id := u.reqID
	msg.ID = &id
	b, _ := msg.Marshal()
	return id, u.SendRaw(string(b))
}

// SubscribeAuthorize sends subscribe and authorize messages
func (u *Upstream) SubscribeAuthorize() error {
	if _, err := u.Send(stratum.NewSubscribeMessage("karoo/v0.0.1")); err != nil {
		return err
	}
	_, err := u.Send(stratum.NewAuthorizeMessage(u.cfg.Upstream.User, u.cfg.Upstream.Pass))
	return err
}

// SetExtranonce sets the extranonce values from upstream
func (u *Upstream) SetExtranonce(ex1 string, ex2Size int) {
	u.ex1 = ex1
	u.ex2Size = ex2Size
}

// GetExtranonce returns the current extranonce values
func (u *Upstream) GetExtranonce() (string, int) {
	return u.ex1, u.ex2Size
}

// AddPendingRequest adds a pending request to the routing table
func (u *Upstream) AddPendingRequest(id int64, req PendingReq) {
	u.respMu.Lock()
	defer u.respMu.Unlock()
	u.pending[id] = req
}

// RemovePendingRequest removes and returns a pending request
func (u *Upstream) RemovePendingRequest(id int64) (PendingReq, bool) {
	u.respMu.Lock()
	defer u.respMu.Unlock()
	req, exists := u.pending[id]
	if exists {
		delete(u.pending, id)
	}
	return req, exists
}

// GetReader returns the upstream reader
func (u *Upstream) GetReader() *bufio.Reader {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.br
}

// Backoff calculates backoff delay with jitter
func Backoff(min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	mul := 1 << (rand.Intn(4)) // 1,2,4,8
	d := time.Duration(int(min) * mul)
	if d > max {
		d = max
	}
	return d + time.Duration(rand.Intn(250))*time.Millisecond
}
