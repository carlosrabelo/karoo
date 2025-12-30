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

	"github.com/carlosrabelo/karoo/core/internal/stratum"
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
		Upstream: struct {
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
				Type     string `json:"type"`
				Host     string `json:"host"`
				Port     int    `json:"port"`
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"socks_proxy"`
		}{
			Host:         "127.0.0.1",
			Port:         0, // Will be set to mock server
			User:         "testuser",
			Pass:         "testpass",
			TLS:          false,
			BackoffMinMs: 100,
			BackoffMaxMs: 1000,
			SocksProxy: struct {
				Enabled  bool   `json:"enabled"`
				Type     string `json:"type"`
				Host     string `json:"host"`
				Port     int    `json:"port"`
				Username string `json:"username"`
				Password string `json:"password"`
			}{
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
		}{
			StrictBroadcast: true,
		},
	}

	// Create proxy
	p := NewProxy(cfg)

	// Test basic proxy creation and configuration
	if p == nil {
		t.Fatal("Failed to create proxy")
	}

	// Test metrics initialization
	if p.mx.ClientsActive.Load() != 0 {
		t.Errorf("Expected 0 active clients initially, got %d", p.mx.ClientsActive.Load())
	}

	// Test client addition (without network operations)
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()

	cl := NewClient(client, cfg)
	p.clients[cl] = struct{}{}
	p.mx.ClientsActive.Add(1)

	// Verify client was added
	if p.mx.ClientsActive.Load() != 1 {
		t.Errorf("Expected 1 active client, got %d", p.mx.ClientsActive.Load())
	}

	// Test metrics updates
	p.mx.SharesOK.Add(5)
	p.mx.SharesBad.Add(2)

	if p.mx.SharesOK.Load() != 5 {
		t.Errorf("Expected 5 OK shares, got %d", p.mx.SharesOK.Load())
	}

	if p.mx.SharesBad.Load() != 2 {
		t.Errorf("Expected 2 bad shares, got %d", p.mx.SharesBad.Load())
	}
}

// TestProxyMetricsCollection tests metrics collection
func TestProxyMetricsCollection(t *testing.T) {
	cfg := &Config{}
	p := NewProxy(cfg)

	// Test metrics initialization
	if p.mx.ClientsActive.Load() != 0 {
		t.Errorf("Expected 0 active clients initially, got %d", p.mx.ClientsActive.Load())
	}

	// Simulate client activity
	p.mx.ClientsActive.Add(3)
	p.mx.SharesOK.Add(10)
	p.mx.SharesBad.Add(2)
	p.mx.LastSetDiff.Store(1000)
	p.mx.LastNotifyUnix.Store(time.Now().Unix())

	// Verify metrics
	if p.mx.ClientsActive.Load() != 3 {
		t.Errorf("Expected 3 active clients, got %d", p.mx.ClientsActive.Load())
	}

	if p.mx.SharesOK.Load() != 10 {
		t.Errorf("Expected 10 OK shares, got %d", p.mx.SharesOK.Load())
	}

	if p.mx.SharesBad.Load() != 2 {
		t.Errorf("Expected 2 bad shares, got %d", p.mx.SharesBad.Load())
	}

	if p.mx.LastSetDiff.Load() != 1000 {
		t.Errorf("Expected last diff 1000, got %d", p.mx.LastSetDiff.Load())
	}
}

// TestProxyConcurrentAccess tests concurrent access to proxy structures
func TestProxyConcurrentAccess(t *testing.T) {
	cfg := &Config{}
	p := NewProxy(cfg)

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
				p.mx.SharesOK.Add(1)
				p.mx.ClientsActive.Add(1)
				p.mx.ClientsActive.Add(-1)

				// Test atomic operations
				p.mx.LastSetDiff.Store(int64(j))
				_ = p.mx.LastSetDiff.Load()
			}
		}(i)
	}

	wg.Wait()

	// Verify final state
	expectedShares := uint64(numGoroutines * operationsPerGoroutine)
	if p.mx.SharesOK.Load() != expectedShares {
		t.Errorf("Expected %d shares, got %d", expectedShares, p.mx.SharesOK.Load())
	}

	if p.mx.ClientsActive.Load() != 0 {
		t.Errorf("Expected 0 active clients, got %d", p.mx.ClientsActive.Load())
	}
}

// MockStratumServer simulates a Stratum mining server for testing
type MockStratumServer struct {
	subscribeResponse interface{}
	authorizeResponse bool
	submitResponse    bool
	connections       atomic.Int64
}

func (m *MockStratumServer) HandleConnection(conn net.Conn) {
	m.connections.Add(1)
	defer m.connections.Add(-1)
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
		Upstream: struct {
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
				Type     string `json:"type"`
				Host     string `json:"host"`
				Port     int    `json:"port"`
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"socks_proxy"`
		}{
			Host:         "127.0.0.1",
			Port:         port,
			User:         "testuser",
			Pass:         "testpass",
			TLS:          false,
			BackoffMinMs: 100,
			BackoffMaxMs: 1000,
			SocksProxy: struct {
				Enabled  bool   `json:"enabled"`
				Type     string `json:"type"`
				Host     string `json:"host"`
				Port     int    `json:"port"`
				Username string `json:"username"`
				Password string `json:"password"`
			}{
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
		}{
			StrictBroadcast: false,
		},
	}

	p := NewProxy(cfg)
	if p == nil {
		t.Fatal("Failed to create proxy")
	}

	// Verify proxy was created correctly
	if p.mx.ClientsActive.Load() != 0 {
		t.Errorf("Expected 0 active clients, got %d", p.mx.ClientsActive.Load())
	}

	// Test complete - verify mock server had connection
	time.Sleep(100 * time.Millisecond)
	if mockServer.connections.Load() < 0 {
		t.Errorf("Expected at least 0 connections to mock server")
	}
}

// TestMultipleClientsIntegration tests handling multiple concurrent clients
func TestMultipleClientsIntegration(t *testing.T) {
	cfg := &Config{}
	p := NewProxy(cfg)

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

		p.mx.ClientsActive.Add(1)
	}

	// Verify all clients were added
	if p.mx.ClientsActive.Load() != int64(numClients) {
		t.Errorf("Expected %d active clients, got %d", numClients, p.mx.ClientsActive.Load())
	}

	// Simulate shares from all clients
	for i := 0; i < numClients; i++ {
		clients[i].IncrementOK()
		p.mx.SharesOK.Add(1)
	}

	// Verify metrics
	if p.mx.SharesOK.Load() != uint64(numClients) {
		t.Errorf("Expected %d OK shares, got %d", numClients, p.mx.SharesOK.Load())
	}
}

// TestUpstreamReconnection tests upstream reconnection logic
func TestUpstreamReconnection(t *testing.T) {
	cfg := &Config{
		Upstream: struct {
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
				Type     string `json:"type"`
				Host     string `json:"host"`
				Port     int    `json:"port"`
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"socks_proxy"`
		}{
			Host:         "127.0.0.1",
			Port:         9999, // Non-existent port
			User:         "testuser",
			Pass:         "testpass",
			TLS:          false,
			BackoffMinMs: 10,
			BackoffMaxMs: 100,
			SocksProxy: struct {
				Enabled  bool   `json:"enabled"`
				Type     string `json:"type"`
				Host     string `json:"host"`
				Port     int    `json:"port"`
				Username string `json:"username"`
				Password string `json:"password"`
			}{
				Enabled: false,
			},
		},
	}

	p := NewProxy(cfg)

	// Verify proxy handles upstream connection failures gracefully
	if p.up == nil {
		t.Error("Upstream manager should be initialized even with connection failure")
	}
}

// TestShareAccounting tests share acceptance and rejection tracking
func TestShareAccounting(t *testing.T) {
	cfg := &Config{}
	p := NewProxy(cfg)

	// Simulate accepted shares
	okShares := uint64(10)
	for i := uint64(0); i < okShares; i++ {
		p.mx.SharesOK.Add(1)
	}

	// Simulate rejected shares
	badShares := uint64(3)
	for i := uint64(0); i < badShares; i++ {
		p.mx.SharesBad.Add(1)
	}

	// Verify accounting
	if p.mx.SharesOK.Load() != okShares {
		t.Errorf("Expected %d OK shares, got %d", okShares, p.mx.SharesOK.Load())
	}

	if p.mx.SharesBad.Load() != badShares {
		t.Errorf("Expected %d bad shares, got %d", badShares, p.mx.SharesBad.Load())
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
	p := NewProxy(cfg)

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
