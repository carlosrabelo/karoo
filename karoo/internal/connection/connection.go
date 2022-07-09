// Package connection manages upstream and downstream network connections
package connection

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/carlosrabelo/karoo/karoo/internal/proxysocks"
	"github.com/carlosrabelo/karoo/karoo/internal/stratum"
)

const dialTimeout = 10 * time.Second

// Config holds proxy configuration (subset needed for connection)
type Config struct {
	Proxy struct {
		ReadBuf  int `json:"read_buf"`
		WriteBuf int `json:"write_buf"`
	} `json:"proxy"`
	Upstream struct {
		Host               string            `json:"host"`
		Port               int               `json:"port"`
		User               string            `json:"user"`
		Pass               string            `json:"pass"`
		TLS                bool              `json:"tls"`
		InsecureSkipVerify bool              `json:"insecure_skip_verify"`
		SocksProxy         proxysocks.Config `json:"socks_proxy"`
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

	// SOCKS proxy dialer
	proxyDialer *proxysocks.ProxyDialer

	// extranonce (guarded by mu)
	ex1     string
	ex2Size int

	// req id / handshake tracking (guarded by mu)
	reqID       int64
	subscribeID int64

	// response routing: upID -> client
	respMu  sync.Mutex
	pending map[int64]PendingReq
}

// PendingReq represents a pending upstream request
type PendingReq struct {
	Client interface{} // Will be routing.Client
	Method string
	Sent   time.Time
	OrigID json.RawMessage
}

// Downstream represents a downstream mining client connection
type Downstream struct {
	Conn   net.Conn
	Reader *bufio.Reader
	Writer *bufio.Writer
	Addr   string
}

// NewUpstream creates a new upstream connection manager
func NewUpstream(cfg *Config) (*Upstream, error) {
	proxyDialer, err := proxysocks.NewProxyDialer(&cfg.Upstream.SocksProxy)
	if err != nil {
		return nil, fmt.Errorf("failed to create proxy dialer: %w", err)
	}

	return &Upstream{
		cfg:         cfg,
		proxyDialer: proxyDialer,
		pending:     make(map[int64]PendingReq),
	}, nil
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
	// Drop any previous connection before dialing again.
	u.Close()

	addr := net.JoinHostPort(u.cfg.Upstream.Host, strconv.Itoa(u.cfg.Upstream.Port))
	var c net.Conn
	var err error

	if u.proxyDialer.IsEnabled() {
		// Use SOCKS proxy
		if u.cfg.Upstream.TLS {
			// First connect through SOCKS proxy, then wrap with TLS
			var rawConn net.Conn
			rawConn, err = u.proxyDialer.DialContext(ctx, "tcp", addr)
			if err != nil {
				return fmt.Errorf("SOCKS proxy connection failed: %w", err)
			}

			conf := &tls.Config{InsecureSkipVerify: u.cfg.Upstream.InsecureSkipVerify}
			tlsConn := tls.Client(rawConn, conf)
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				_ = rawConn.Close()
				return fmt.Errorf("TLS handshake through SOCKS proxy failed: %w", err)
			}
			c = tlsConn
		} else {
			// Direct SOCKS proxy connection
			c, err = u.proxyDialer.DialContext(ctx, "tcp", addr)
			if err != nil {
				return fmt.Errorf("SOCKS proxy connection failed: %w", err)
			}
		}
	} else {
		d := &net.Dialer{Timeout: dialTimeout}
		if u.cfg.Upstream.TLS {
			conf := &tls.Config{InsecureSkipVerify: u.cfg.Upstream.InsecureSkipVerify}
			c, err = tls.DialWithDialer(d, "tcp", addr, conf)
		} else {
			c, err = d.DialContext(ctx, "tcp", addr)
		}
		if err != nil {
			return err
		}
	}

	u.mu.Lock()
	u.conn = c
	u.br = bufio.NewReaderSize(c, u.cfg.Proxy.ReadBuf)
	u.bw = bufio.NewWriterSize(c, u.cfg.Proxy.WriteBuf)
	u.ex1 = ""
	u.ex2Size = 0
	u.subscribeID = 0
	u.mu.Unlock()
	u.respMu.Lock()
	u.pending = make(map[int64]PendingReq)
	u.respMu.Unlock()
	return nil
}

// UpdateTarget updates upstream connection details for failover, including SOCKS.
func (u *Upstream) UpdateTarget(host string, port int, user, pass string, useTLS, insecure bool, socks proxysocks.Config) error {
	dialer, err := proxysocks.NewProxyDialer(&socks)
	if err != nil {
		return fmt.Errorf("socks dialer: %w", err)
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	u.cfg.Upstream.Host = host
	u.cfg.Upstream.Port = port
	u.cfg.Upstream.User = user
	u.cfg.Upstream.Pass = pass
	u.cfg.Upstream.TLS = useTLS
	u.cfg.Upstream.InsecureSkipVerify = insecure
	u.cfg.Upstream.SocksProxy = socks
	u.proxyDialer = dialer
	return nil
}

// ApplyConfig replaces connection buffers and primary upstream settings.
func (u *Upstream) ApplyConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}
	dialer, err := proxysocks.NewProxyDialer(&cfg.Upstream.SocksProxy)
	if err != nil {
		return fmt.Errorf("socks dialer: %w", err)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.cfg = cfg
	u.proxyDialer = dialer
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
	u.mu.Lock()
	u.reqID++
	id := u.reqID
	u.mu.Unlock()

	msg.ID = stratum.NewIntID(id)
	b, err := msg.Marshal()
	if err != nil {
		return 0, fmt.Errorf("marshal upstream message: %w", err)
	}
	return id, u.SendRaw(string(b))
}

// SubscribeAuthorize sends subscribe and authorize messages
func (u *Upstream) SubscribeAuthorize() error {
	subID, err := u.Send(stratum.NewSubscribeMessage("karoo/v0.0.1"))
	if err != nil {
		return err
	}
	u.mu.Lock()
	u.subscribeID = subID
	u.mu.Unlock()

	_, err = u.Send(stratum.NewAuthorizeMessage(u.cfg.Upstream.User, u.cfg.Upstream.Pass))
	return err
}

// IsSubscribeResponse reports whether msg is the response to our upstream subscribe.
func (u *Upstream) IsSubscribeResponse(msg stratum.Message) bool {
	n, ok := stratum.ParseIDInt64(msg.ID)
	if !ok {
		return false
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.subscribeID != 0 && n == u.subscribeID
}

// SetExtranonce sets the extranonce values from upstream
func (u *Upstream) SetExtranonce(ex1 string, ex2Size int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.ex1 = ex1
	u.ex2Size = ex2Size
}

// GetExtranonce returns the current extranonce values
func (u *Upstream) GetExtranonce() (string, int) {
	u.mu.Lock()
	defer u.mu.Unlock()
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
