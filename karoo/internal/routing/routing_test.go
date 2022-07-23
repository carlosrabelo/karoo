package routing

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/carlosrabelo/karoo/karoo/internal/connection"
	"github.com/carlosrabelo/karoo/karoo/internal/metrics"
	"github.com/carlosrabelo/karoo/karoo/internal/stratum"
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
	messages         []stratum.Message
}

func (m *mockClient) GetAddr() string             { return m.addr }
func (m *mockClient) GetWorker() string           { return m.worker }
func (m *mockClient) GetUpUser() string           { return m.upUser }
func (m *mockClient) SetWorker(w string)          { m.worker = w }
func (m *mockClient) SetUpUser(u string)          { m.upUser = u }
func (m *mockClient) GetExtraNoncePrefix() string { return m.extraNoncePrefix }
func (m *mockClient) GetExtraNonceTrim() int      { return m.extraNonceTrim }
func (m *mockClient) GetLastAccept() int64        { return m.lastAccept }
func (m *mockClient) UpdateLastAccept(t int64)    { m.lastAccept = t }
func (m *mockClient) GetOK() uint64               { return m.ok }
func (m *mockClient) GetBad() uint64              { return m.bad }
func (m *mockClient) GetDiff() int64              { return 1000 }
func (m *mockClient) IncrementOK()                { m.ok++ }
func (m *mockClient) IncrementBad()               { m.bad++ }
func (m *mockClient) SetHandshakeDone(done bool)  { m.handshakeDone = done }
func (m *mockClient) WriteJSON(msg stratum.Message) error {
	m.messages = append(m.messages, msg)
	return m.writeError
}
func (m *mockClient) WriteLine(line string) error { return m.writeError }

func createTestConfig() *Config {
	return &Config{
		Upstream: struct {
			User string `json:"user"`
		}{
			User: "testuser",
		},
		Compat: struct {
			StrictBroadcast bool `json:"strict_broadcast"`
			LocalAuthorize  bool `json:"local_authorize"`
		}{
			StrictBroadcast: false,
			LocalAuthorize:  false,
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
	up, err := connection.NewUpstream(cfg)
	if err != nil {
		panic(err)
	}
	return up
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
		ID:     stratum.NewIntID(1),
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
			diff := stratum.DiffFromBits(tt.bits)
			if tt.valid && diff == 0 {
				t.Errorf("DiffFromBits(%s) returned 0 for valid input", tt.bits)
			}
			if !tt.valid && diff != 0 {
				t.Errorf("DiffFromBits(%s) returned non-zero for invalid input", tt.bits)
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
			got := stratum.FormatDuration(toDuration(tt.dur))
			if got != tt.want {
				t.Errorf("FormatDuration(%d) = %s, want %s", tt.dur, got, tt.want)
			}
		})
	}
}

func TestProcessUpstreamErrorResponse(t *testing.T) {
	cfg := createTestConfig()
	up := createTestUpstream()
	mx := metrics.NewCollector()
	r := NewRouter(cfg, up, mx)

	cl := &mockClient{addr: "127.0.0.1:1"}
	id := int64(42)
	up.AddPendingRequest(id, connection.PendingReq{
		Client: cl,
		Method: "mining.submit",
		Sent:   time.Now(),
		OrigID: json.RawMessage(`"client-7"`),
	})

	line := `{"id":42,"result":null,"error":[21,"low-difficulty-share",null]}`
	r.ProcessUpstreamMessage(line)

	if cl.bad != 1 {
		t.Fatalf("expected rejected share counted, bad=%d", cl.bad)
	}
	if _, exists := up.RemovePendingRequest(id); exists {
		t.Fatal("pending request should have been consumed")
	}
}

func TestLocalAuthorize(t *testing.T) {
	cfg := createTestConfig()
	cfg.Compat.LocalAuthorize = true
	up := createTestUpstream()
	mx := metrics.NewCollector()
	r := NewRouter(cfg, up, mx)

	cl := &mockClient{addr: "127.0.0.1:1"}
	msg := stratum.Message{
		ID:     json.RawMessage(`"auth-1"`),
		Method: "mining.authorize",
		Params: []any{"wallet.worker", "secret-password"},
	}
	r.ProcessClientMessage(cl, msg)

	if cl.worker != "wallet.worker" {
		t.Fatalf("worker=%q", cl.worker)
	}
	if cl.upUser != "testuser" {
		t.Fatalf("upUser=%q want testuser", cl.upUser)
	}
	if !cl.handshakeDone {
		t.Fatal("expected handshake done")
	}
	if len(cl.messages) != 1 {
		t.Fatalf("expected one local response, got %d", len(cl.messages))
	}
	if ok, _ := cl.messages[0].Result.(bool); !ok {
		t.Fatalf("expected success result true, got %#v", cl.messages[0].Result)
	}
	// Password must never be forwarded; with local auth there is no upstream pending entry.
	if _, exists := up.RemovePendingRequest(1); exists {
		t.Fatal("local authorize must not create upstream pending requests")
	}
}

// Helper functions

func toDuration(ms int64) time.Duration {
	return time.Duration(ms) * time.Millisecond
}
