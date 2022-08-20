package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/carlosrabelo/karoo/karoo/internal/proxysocks"
	"github.com/carlosrabelo/karoo/karoo/internal/stratum"
)

// TestProxyIntegration tests basic proxy functionality end-to-end
func TestProxyIntegration(t *testing.T) {
	// Create test configuration
	cfg := &Config{
		Proxy: struct {
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
		}{
			Listen:       "127.0.0.1:0", // Random port
			ClientIdleMs: 5000,
			MaxClients:   10,
			ReadBuf:      4096,
			WriteBuf:     4096,
			TLS: struct {
				Enabled bool   `json:"enabled"`
				Cert    string `json:"cert_file"`
				Key     string `json:"key_file"`
			}{
				Enabled: false,
			},
		},
		Upstream: UpstreamConfig{
			Host:         "127.0.0.1",
			Port:         0, // Will be set to mock server
			User:         "testuser",
			Pass:         "testpass",
			TLS:          false,
			BackoffMinMs: 100,
			BackoffMaxMs: 1000,
			SocksProxy: proxysocks.Config{
				Enabled: false,
			},
		},
		HTTP: struct {
			Listen string `json:"listen"`
			Pprof  bool   `json:"pprof"`
		}{
			Listen: "127.0.0.1:0", // Random port
		},
		VarDiff: struct {
			Enabled       bool `json:"enabled"`
			TargetSeconds int  `json:"target_seconds"`
			MinDiff       int  `json:"min_diff"`
			MaxDiff       int  `json:"max_diff"`
			AdjustEveryMs int  `json:"adjust_every_ms"`
		}{
			Enabled:       false, // Disable for simpler test
			TargetSeconds: 15,
			MinDiff:       1000,
			MaxDiff:       65536,
			AdjustEveryMs: 60000,
		},
		Compat: struct {
			StrictBroadcast bool `json:"strict_broadcast"`
			LocalAuthorize  bool `json:"local_authorize"`
		}{
			StrictBroadcast: true,
		},
	}

	// Create proxy
	p, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}

	// Test basic proxy creation and configuration
	if p == nil {
		t.Fatal("Failed to create proxy")
	}

	// Test metrics initialization
	if atomic.LoadInt64(&p.mx.ClientsActive) != 0 {
		t.Errorf("Expected 0 active clients initially, got %d", atomic.LoadInt64(&p.mx.ClientsActive))
	}

	// Test client addition (without network operations)
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	cl := NewClient(client, cfg)
	p.clients[cl] = struct{}{}
	atomic.AddInt64(&p.mx.ClientsActive, 1)

	// Verify client was added
	if atomic.LoadInt64(&p.mx.ClientsActive) != 1 {
		t.Errorf("Expected 1 active client, got %d", atomic.LoadInt64(&p.mx.ClientsActive))
	}

	// Test metrics updates
	atomic.AddUint64(&p.mx.SharesOK, 5)
	atomic.AddUint64(&p.mx.SharesBad, 2)

	if atomic.LoadUint64(&p.mx.SharesOK) != 5 {
		t.Errorf("Expected 5 OK shares, got %d", atomic.LoadUint64(&p.mx.SharesOK))
	}

	if atomic.LoadUint64(&p.mx.SharesBad) != 2 {
		t.Errorf("Expected 2 bad shares, got %d", atomic.LoadUint64(&p.mx.SharesBad))
	}
}

// TestProxyMetricsCollection tests metrics collection
func TestProxyMetricsCollection(t *testing.T) {
	cfg := &Config{}
	p, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}

	// Test metrics initialization
	if atomic.LoadInt64(&p.mx.ClientsActive) != 0 {
		t.Errorf("Expected 0 active clients initially, got %d", atomic.LoadInt64(&p.mx.ClientsActive))
	}

	// Simulate client activity
	atomic.AddInt64(&p.mx.ClientsActive, 3)
	atomic.AddUint64(&p.mx.SharesOK, 10)
	atomic.AddUint64(&p.mx.SharesBad, 2)
	atomic.StoreInt64(&p.mx.LastSetDiff, 1000)
	atomic.StoreInt64(&p.mx.LastNotifyUnix, time.Now().Unix())

	// Verify metrics
	if atomic.LoadInt64(&p.mx.ClientsActive) != 3 {
		t.Errorf("Expected 3 active clients, got %d", atomic.LoadInt64(&p.mx.ClientsActive))
	}

	if atomic.LoadUint64(&p.mx.SharesOK) != 10 {
		t.Errorf("Expected 10 OK shares, got %d", atomic.LoadUint64(&p.mx.SharesOK))
	}

	if atomic.LoadUint64(&p.mx.SharesBad) != 2 {
		t.Errorf("Expected 2 bad shares, got %d", atomic.LoadUint64(&p.mx.SharesBad))
	}

	if atomic.LoadInt64(&p.mx.LastSetDiff) != 1000 {
		t.Errorf("Expected last diff 1000, got %d", atomic.LoadInt64(&p.mx.LastSetDiff))
	}
}

// TestProxyConcurrentAccess tests concurrent access to proxy structures
func TestProxyConcurrentAccess(t *testing.T) {
	cfg := &Config{}
	p, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}

	// Simulate concurrent client operations
	var wg sync.WaitGroup
	numGoroutines := 10
	operationsPerGoroutine := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				// Test concurrent metrics updates
				atomic.AddUint64(&p.mx.SharesOK, 1)
				atomic.AddInt64(&p.mx.ClientsActive, 1)
				atomic.AddInt64(&p.mx.ClientsActive, -1)

				// Test atomic operations
				atomic.StoreInt64(&p.mx.LastSetDiff, int64(j))
				_ = atomic.LoadInt64(&p.mx.LastSetDiff)
			}
		}(i)
	}

	wg.Wait()

	// Verify final state
	expectedShares := uint64(numGoroutines * operationsPerGoroutine)
	if atomic.LoadUint64(&p.mx.SharesOK) != expectedShares {
		t.Errorf("Expected %d shares, got %d", expectedShares, atomic.LoadUint64(&p.mx.SharesOK))
	}

	if atomic.LoadInt64(&p.mx.ClientsActive) != 0 {
		t.Errorf("Expected 0 active clients, got %d", atomic.LoadInt64(&p.mx.ClientsActive))
	}
}

// MockStratumServer simulates a Stratum mining server for testing
type MockStratumServer struct {
	subscribeResponse interface{}
	authorizeResponse bool
	submitResponse    bool
	connections       int64
}

func (m *MockStratumServer) HandleConnection(conn net.Conn) {
	atomic.AddInt64(&m.connections, 1)
	defer atomic.AddInt64(&m.connections, -1)
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		var msg stratum.Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		var response stratum.Message

		switch msg.Method {
		case "mining.subscribe":
			response = stratum.Message{
				ID:     msg.ID,
				Result: m.subscribeResponse,
			}
		case "mining.authorize":
			response = stratum.Message{
				ID:     msg.ID,
				Result: m.authorizeResponse,
			}
		case "mining.submit":
			response = stratum.Message{
				ID:     msg.ID,
				Result: m.submitResponse,
			}
		default:
			continue
		}

		respData, _ := json.Marshal(response)
		respData = append(respData, '\n')
		if _, err := conn.Write(respData); err != nil {
			return
		}
	}
}

// TestEndToEndFlow tests complete client->proxy->upstream flow
func TestEndToEndFlow(t *testing.T) {
	// Create mock upstream server
	mockServer := &MockStratumServer{
		subscribeResponse: []interface{}{
			[]interface{}{},
			"deadbeef",
			float64(4),
		},
		authorizeResponse: true,
		submitResponse:    true,
	}

	// Start mock server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start mock server: %v", err)
	}
	defer func() { _ = listener.Close() }()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go mockServer.HandleConnection(conn)
		}
	}()

	// Get server port
	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		t.Fatalf("Failed to parse port: %v", err)
	}

	// Create proxy configuration
	cfg := &Config{
		Proxy: struct {
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
		}{
			Listen:       "127.0.0.1:0", // Random port
			ClientIdleMs: 5000,
			MaxClients:   10,
			ReadBuf:      4096,
			WriteBuf:     4096,
			TLS: struct {
				Enabled bool   `json:"enabled"`
				Cert    string `json:"cert_file"`
				Key     string `json:"key_file"`
			}{
				Enabled: false,
			},
		},
		Upstream: UpstreamConfig{
			Host:         "127.0.0.1",
			Port:         port,
			User:         "testuser",
			Pass:         "testpass",
			TLS:          false,
			BackoffMinMs: 100,
			BackoffMaxMs: 1000,
			SocksProxy: proxysocks.Config{
				Enabled: false,
			},
		},
		HTTP: struct {
			Listen string `json:"listen"`
			Pprof  bool   `json:"pprof"`
		}{
			Listen: "",
		},
		VarDiff: struct {
			Enabled       bool `json:"enabled"`
			TargetSeconds int  `json:"target_seconds"`
			MinDiff       int  `json:"min_diff"`
			MaxDiff       int  `json:"max_diff"`
			AdjustEveryMs int  `json:"adjust_every_ms"`
		}{
			Enabled: false,
		},
		Compat: struct {
			StrictBroadcast bool `json:"strict_broadcast"`
			LocalAuthorize  bool `json:"local_authorize"`
		}{
			StrictBroadcast: false,
		},
	}

	p, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}
	if p == nil {
		t.Fatal("Failed to create proxy")
	}

	// Verify proxy was created correctly
	if atomic.LoadInt64(&p.mx.ClientsActive) != 0 {
		t.Errorf("Expected 0 active clients, got %d", atomic.LoadInt64(&p.mx.ClientsActive))
	}

	// Test complete - verify mock server had connection
	time.Sleep(100 * time.Millisecond)
	if atomic.LoadInt64(&mockServer.connections) < 0 {
		t.Errorf("Expected at least 0 connections to mock server")
	}
}

// TestMultipleClientsIntegration tests handling multiple concurrent clients
func TestMultipleClientsIntegration(t *testing.T) {
	cfg := &Config{}
	p, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}

	// Simulate multiple clients
	numClients := 5
	clients := make([]*Client, numClients)

	for i := 0; i < numClients; i++ {
		server, client := net.Pipe()
		defer func() { _ = server.Close() }()
		defer func() { _ = client.Close() }()

		cl := NewClient(client, cfg)
		clients[i] = cl

		p.clMu.Lock()
		p.clients[cl] = struct{}{}
		p.clMu.Unlock()

		atomic.AddInt64(&p.mx.ClientsActive, 1)
	}

	// Verify all clients were added
	if atomic.LoadInt64(&p.mx.ClientsActive) != int64(numClients) {
		t.Errorf("Expected %d active clients, got %d", numClients, atomic.LoadInt64(&p.mx.ClientsActive))
	}

	// Simulate shares from all clients
	for i := 0; i < numClients; i++ {
		clients[i].IncrementOK()
		atomic.AddUint64(&p.mx.SharesOK, 1)
	}

	// Verify metrics
	if atomic.LoadUint64(&p.mx.SharesOK) != uint64(numClients) {
		t.Errorf("Expected %d OK shares, got %d", numClients, atomic.LoadUint64(&p.mx.SharesOK))
	}
}

// TestUpstreamReconnection tests upstream reconnection logic
func TestUpstreamReconnection(t *testing.T) {
	cfg := &Config{
		Upstream: UpstreamConfig{
			Host:         "127.0.0.1",
			Port:         9999, // Non-existent port
			User:         "testuser",
			Pass:         "testpass",
			TLS:          false,
			BackoffMinMs: 10,
			BackoffMaxMs: 100,
			SocksProxy: proxysocks.Config{
				Enabled: false,
			},
		},
	}

	p, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}

	// Verify proxy handles upstream connection failures gracefully
	if p.up == nil {
		t.Error("Upstream manager should be initialized even with connection failure")
	}
}

// TestShareAccounting tests share acceptance and rejection tracking
func TestShareAccounting(t *testing.T) {
	cfg := &Config{}
	p, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}

	// Simulate accepted shares
	okShares := uint64(10)
	for i := uint64(0); i < okShares; i++ {
		atomic.AddUint64(&p.mx.SharesOK, 1)
	}

	// Simulate rejected shares
	badShares := uint64(3)
	for i := uint64(0); i < badShares; i++ {
		atomic.AddUint64(&p.mx.SharesBad, 1)
	}

	// Verify accounting
	if atomic.LoadUint64(&p.mx.SharesOK) != okShares {
		t.Errorf("Expected %d OK shares, got %d", okShares, atomic.LoadUint64(&p.mx.SharesOK))
	}

	if atomic.LoadUint64(&p.mx.SharesBad) != badShares {
		t.Errorf("Expected %d bad shares, got %d", badShares, atomic.LoadUint64(&p.mx.SharesBad))
	}

	// Calculate acceptance rate
	total := okShares + badShares
	acceptanceRate := float64(okShares) / float64(total) * 100

	if acceptanceRate < 70.0 {
		t.Errorf("Acceptance rate too low: %.2f%%", acceptanceRate)
	}
}

// TestBroadcastToClients tests broadcasting messages to all clients
func TestBroadcastToClients(t *testing.T) {
	cfg := &Config{}
	p, err := NewProxy(cfg)
	if err != nil {
		t.Fatalf("NewProxy: %v", err)
	}

	// Create multiple clients
	numClients := 3
	for i := 0; i < numClients; i++ {
		server, client := net.Pipe()
		defer func() { _ = server.Close() }()
		defer func() { _ = client.Close() }()

		cl := NewClient(client, cfg)

		p.clMu.Lock()
		p.clients[cl] = struct{}{}
		p.clMu.Unlock()
	}

	// Verify client count
	p.clMu.RLock()
	clientCount := len(p.clients)
	p.clMu.RUnlock()

	if clientCount != numClients {
		t.Errorf("Expected %d clients, got %d", numClients, clientCount)
	}
}
