package routing

import (
	"testing"
	"time"

	"github.com/carlosrabelo/karoo/core/internal/connection"
	"github.com/carlosrabelo/karoo/core/internal/metrics"
	"github.com/carlosrabelo/karoo/core/internal/stratum"
)

// mockClient implements the Client interface for testing
type mockClient struct {
	addr             string
	worker           string
	upUser           string
	extraNoncePrefix string
	extraNonceTrim   int
	lastAccept       int64
	ok               uint64
	bad              uint64
	handshakeDone    bool
	writeError       error
}

func (m *mockClient) GetAddr() string                  { return m.addr }
func (m *mockClient) GetWorker() string                { return m.worker }
func (m *mockClient) GetUpUser() string                { return m.upUser }
func (m *mockClient) SetWorker(w string)               { m.worker = w }
func (m *mockClient) SetUpUser(u string)               { m.upUser = u }
func (m *mockClient) GetExtraNoncePrefix() string      { return m.extraNoncePrefix }
func (m *mockClient) GetExtraNonceTrim() int           { return m.extraNonceTrim }
func (m *mockClient) GetLastAccept() int64             { return m.lastAccept }
func (m *mockClient) UpdateLastAccept(t int64)         { m.lastAccept = t }
func (m *mockClient) GetOK() uint64                    { return m.ok }
func (m *mockClient) GetBad() uint64                   { return m.bad }
func (m *mockClient) IncrementOK()                     { m.ok++ }
func (m *mockClient) IncrementBad()                    { m.bad++ }
func (m *mockClient) SetHandshakeDone(done bool)       { m.handshakeDone = done }
func (m *mockClient) WriteJSON(msg stratum.Message) error { return m.writeError }
func (m *mockClient) WriteLine(line string) error      { return m.writeError }

func createTestConfig() *Config {
	return &Config{
		Upstream: struct {
			User string `json:"user"`
		}{
			User: "testuser",
		},
		Compat: struct {
			StrictBroadcast bool `json:"strict_broadcast"`
		}{
			StrictBroadcast: false,
		},
	}
}

func createTestUpstream() *connection.Upstream {
	cfg := &connection.Config{
		Proxy: struct {
			ReadBuf  int `json:"read_buf"`
			WriteBuf int `json:"write_buf"`
		}{
			ReadBuf:  4096,
			WriteBuf: 4096,
		},
	}
	return connection.NewUpstream(cfg)
}

func TestNewRouter(t *testing.T) {
	cfg := createTestConfig()
	up := createTestUpstream()
	mx := metrics.NewCollector()
	r := NewRouter(cfg, up, mx)

	if r == nil {
		t.Fatal("NewRouter returned nil")
	}
	if r.cfg != cfg {
		t.Error("Config not set correctly")
	}
	if r.up != up {
		t.Error("Upstream not set correctly")
	}
	if r.mx != mx {
		t.Error("Metrics collector not set correctly")
	}
	if r.clients == nil {
		t.Error("Clients map not initialized")
	}
}

func TestAddClient(t *testing.T) {
	cfg := createTestConfig()
	up := createTestUpstream()
	mx := metrics.NewCollector()
	r := NewRouter(cfg, up, mx)

	cl := &mockClient{addr: "192.168.1.1:12345"}
	r.AddClient(cl)

	r.clMu.RLock()
	if len(r.clients) != 1 {
		t.Errorf("Expected 1 client, got %d", len(r.clients))
	}
	if _, exists := r.clients[cl]; !exists {
		t.Error("Client not found in clients map")
	}
	r.clMu.RUnlock()
}

func TestRemoveClient(t *testing.T) {
	cfg := createTestConfig()
	up := createTestUpstream()
	mx := metrics.NewCollector()
	r := NewRouter(cfg, up, mx)

	cl := &mockClient{addr: "192.168.1.1:12345"}
	r.AddClient(cl)
	r.RemoveClient(cl)

	r.clMu.RLock()
	if len(r.clients) != 0 {
		t.Errorf("Expected 0 clients, got %d", len(r.clients))
	}
	r.clMu.RUnlock()
}

func TestBroadcast(t *testing.T) {
	cfg := createTestConfig()
	up := createTestUpstream()
	mx := metrics.NewCollector()
	r := NewRouter(cfg, up, mx)

	cl1 := &mockClient{addr: "192.168.1.1:12345"}
	cl2 := &mockClient{addr: "192.168.1.2:12345"}
	r.AddClient(cl1)
	r.AddClient(cl2)

	line := `{"method":"mining.notify","params":[]}`
	r.Broadcast(line)

	// Should not error even if write fails
}

func TestProcessClientMessageAuthorize(t *testing.T) {
	cfg := createTestConfig()
	up := createTestUpstream()
	mx := metrics.NewCollector()
	r := NewRouter(cfg, up, mx)

	cl := &mockClient{addr: "192.168.1.1:12345"}

	msg := stratum.Message{
		Method: "mining.authorize",
		Params: []interface{}{"worker1", "password"},
		ID:     intPtr(1),
	}

	r.ProcessClientMessage(cl, msg)

	if cl.GetWorker() != "worker1" {
		t.Errorf("Expected worker 'worker1', got '%s'", cl.GetWorker())
	}
}

func TestWriteClient(t *testing.T) {
	cfg := createTestConfig()
	up := createTestUpstream()
	mx := metrics.NewCollector()
	r := NewRouter(cfg, up, mx)

	cl := &mockClient{addr: "192.168.1.1:12345"}
	msg := stratum.Message{Method: "test"}

	r.writeClient(cl, msg)
	// Should not panic
}

func TestDiffFromBits(t *testing.T) {
	tests := []struct {
		name  string
		bits  string
		valid bool
	}{
		{"valid with 0x", "0x1d00ffff", true},
		{"valid without 0x", "1d00ffff", true},
		{"empty string", "", false},
		{"invalid hex", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := diffFromBits(tt.bits)
			if tt.valid && diff == 0 {
				t.Errorf("diffFromBits(%s) returned 0 for valid input", tt.bits)
			}
			if !tt.valid && diff != 0 {
				t.Errorf("diffFromBits(%s) returned non-zero for invalid input", tt.bits)
			}
		})
	}
}

func TestFmtDuration(t *testing.T) {
	tests := []struct {
		name string
		dur  int64 // milliseconds
		want string
	}{
		{"zero", 0, "-"},
		{"negative", -1000, "-"},
		{"1 second", 1000, "1s"},
		{"1.5 seconds", 1500, "1.5s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fmtDuration(toDuration(tt.dur))
			if got != tt.want {
				t.Errorf("fmtDuration(%d) = %s, want %s", tt.dur, got, tt.want)
			}
		})
	}
}

// Helper functions
func intPtr(i int64) *int64 {
	return &i
}

func toDuration(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}
