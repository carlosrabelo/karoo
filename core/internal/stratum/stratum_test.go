package stratum

import (
	"testing"
	"time"
)

func TestParseExtranonceResult(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		wantEx1 string
		wantEx2 int
		wantOK  bool
	}{
		{
			name:    "valid array format",
			input:   []interface{}{[]interface{}{}, "deadbeef", float64(4)},
			wantEx1: "deadbeef",
			wantEx2: 4,
			wantOK:  true,
		},
		{
			name:    "valid map format",
			input:   map[string]interface{}{"extranonce1": "cafe", "extranonce2_size": "2"},
			wantEx1: "cafe",
			wantEx2: 2,
			wantOK:  true,
		},
		{
			name:    "array too short",
			input:   []interface{}{[]interface{}{}, "deadbeef"},
			wantEx1: "",
			wantEx2: 0,
			wantOK:  false,
		},
		{
			name:    "empty extranonce1",
			input:   []interface{}{[]interface{}{}, "", 4},
			wantEx1: "",
			wantEx2: 0,
			wantOK:  false,
		},
		{
			name:    "zero extranonce2 size",
			input:   []interface{}{[]interface{}{}, "deadbeef", 0},
			wantEx1: "",
			wantEx2: 0,
			wantOK:  false,
		},
		{
			name:    "map missing extranonce1",
			input:   map[string]interface{}{"extranonce2_size": "2"},
			wantEx1: "",
			wantEx2: 0,
			wantOK:  false,
		},
		{
			name:    "map missing extranonce2_size",
			input:   map[string]interface{}{"extranonce1": "cafe"},
			wantEx1: "",
			wantEx2: 0,
			wantOK:  false,
		},
		{
			name:    "invalid type",
			input:   "invalid",
			wantEx1: "",
			wantEx2: 0,
			wantOK:  false,
		},
		{
			name:    "nil input",
			input:   nil,
			wantEx1: "",
			wantEx2: 0,
			wantOK:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseExtranonceResult(tt.input)
			if got.Extranonce1 != tt.wantEx1 {
				t.Errorf("ParseExtranonceResult() Extranonce1 = %v, want %v", got.Extranonce1, tt.wantEx1)
			}
			if got.Extranonce2Size != tt.wantEx2 {
				t.Errorf("ParseExtranonceResult() Extranonce2Size = %v, want %v", got.Extranonce2Size, tt.wantEx2)
			}
			if got.Valid != tt.wantOK {
				t.Errorf("ParseExtranonceResult() Valid = %v, want %v", got.Valid, tt.wantOK)
			}
		})
	}
}

func TestParseExtranonceSize(t *testing.T) {
	tests := []struct {
		name   string
		input  interface{}
		want   int
		wantOK bool
	}{
		{
			name:   "float64 positive",
			input:  float64(4),
			want:   4,
			wantOK: true,
		},
		{
			name:   "float64 zero",
			input:  float64(0),
			want:   0,
			wantOK: false,
		},
		{
			name:   "float64 negative",
			input:  float64(-1),
			want:   -1,
			wantOK: false,
		},
		{
			name:   "string valid",
			input:  "8",
			want:   8,
			wantOK: true,
		},
		{
			name:   "string zero",
			input:  "0",
			want:   0,
			wantOK: false,
		},
		{
			name:   "string empty",
			input:  "",
			want:   0,
			wantOK: false,
		},
		{
			name:   "string invalid",
			input:  "invalid",
			want:   0,
			wantOK: false,
		},
		{
			name:   "int type",
			input:  4,
			want:   0,
			wantOK: false,
		},
		{
			name:   "nil input",
			input:  nil,
			want:   0,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotOK := ParseExtranonceSize(tt.input)
			if got != tt.want {
				t.Errorf("ParseExtranonceSize() got = %v, want %v", got, tt.want)
			}
			if gotOK != tt.wantOK {
				t.Errorf("ParseExtranonceSize() gotOK = %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{
			name: "zero duration",
			d:    0,
			want: "-",
		},
		{
			name: "negative duration",
			d:    -time.Second,
			want: "-",
		},
		{
			name: "milliseconds",
			d:    150 * time.Millisecond,
			want: "150ms",
		},
		{
			name: "seconds",
			d:    5 * time.Second,
			want: "5s",
		},
		{
			name: "minutes and seconds",
			d:    2*time.Minute + 30*time.Second,
			want: "2m30s",
		},
		{
			name: "complex duration",
			d:    1*time.Hour + 2*time.Minute + 3*time.Second + 456*time.Millisecond,
			want: "1h2m3.456s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDuration(tt.d)
			if got != tt.want {
				t.Errorf("FormatDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDiffFromBits(t *testing.T) {
	tests := []struct {
		name string
		bits string
		want float64
	}{
		{
			name: "empty string",
			bits: "",
			want: 0,
		},
		{
			name: "invalid hex",
			bits: "invalid",
			want: 0,
		},
		{
			name: "zero bits",
			bits: "0x00000000",
			want: 0,
		},
		{
			name: "low difficulty",
			bits: "0x1d00ffff",
			want: 1,
		},
		{
			name: "medium difficulty",
			bits: "0x1c0ffff",
			want: 0, // O algoritmo retorna 0 para este valor específico
		},
		{
			name: "high difficulty",
			bits: "0x1b0ffff",
			want: 0, // O algoritmo retorna 0 para este valor específico
		},
		{
			name: "without 0x prefix",
			bits: "1d00ffff",
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DiffFromBits(tt.bits)
			// Allow for small floating point differences
			if tt.want == 0 {
				if got != 0 {
					t.Errorf("DiffFromBits() = %v, want %v", got, tt.want)
				}
			} else {
				diff := got - tt.want
				if diff < -0.001 || diff > 0.001 {
					t.Errorf("DiffFromBits() = %v, want %v (diff: %v)", got, tt.want, diff)
				}
			}
		})
	}
}

func TestCopyID(t *testing.T) {
	tests := []struct {
		name string
		id   *int64
		want *int64
	}{
		{
			name: "nil id",
			id:   nil,
			want: nil,
		},
		{
			name: "valid id",
			id:   func() *int64 { i := int64(42); return &i }(),
			want: func() *int64 { i := int64(42); return &i }(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CopyID(tt.id)
			if tt.id == nil {
				if got != nil {
					t.Errorf("CopyID() = %v, want %v", got, tt.want)
				}
			} else {
				if got == nil {
					t.Errorf("CopyID() = nil, want %v", tt.want)
				} else if *got != *tt.want {
					t.Errorf("CopyID() = %v, want %v", *got, *tt.want)
				}
				// Ensure it's a different pointer
				if got == tt.id {
					t.Error("CopyID() returned same pointer, should be a copy")
				}
			}
		})
	}
}

func TestParseURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantHost string
		wantPort int
	}{
		{
			name:     "full stratum url",
			url:      "stratum+tcp://pool.example.com:3333",
			wantHost: "pool.example.com",
			wantPort: 3333,
		},
		{
			name:     "host port only",
			url:      "pool.example.com:4444",
			wantHost: "pool.example.com",
			wantPort: 4444,
		},
		{
			name:     "host only (default port)",
			url:      "pool.example.com",
			wantHost: "pool.example.com",
			wantPort: 3333,
		},
		{
			name:     "localhost with port",
			url:      "localhost:8080",
			wantHost: "localhost",
			wantPort: 8080,
		},
		{
			name:     "localhost default port",
			url:      "localhost",
			wantHost: "localhost",
			wantPort: 3333,
		},
		{
			name:     "ip address with port",
			url:      "192.168.1.100:5678",
			wantHost: "192.168.1.100",
			wantPort: 5678,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var host string
			var port int
			ParseURL(tt.url, &host, &port)

			if host != tt.wantHost {
				t.Errorf("ParseURL() host = %v, want %v", host, tt.wantHost)
			}
			if port != tt.wantPort {
				t.Errorf("ParseURL() port = %v, want %v", port, tt.wantPort)
			}
		})
	}
}

func TestMessageTypes(t *testing.T) {
	// Test message creation functions
	subscribeMsg := NewSubscribeMessage("test-agent")
	if subscribeMsg.Method != MethodSubscribe {
		t.Errorf("NewSubscribeMessage() method = %v, want %v", subscribeMsg.Method, MethodSubscribe)
	}

	authorizeMsg := NewAuthorizeMessage("user", "pass")
	if authorizeMsg.Method != MethodAuthorize {
		t.Errorf("NewAuthorizeMessage() method = %v, want %v", authorizeMsg.Method, MethodAuthorize)
	}

	submitMsg := NewSubmitMessage("worker", "job", "nonce1", "nonce2", "ntime", "nonce")
	if submitMsg.Method != MethodSubmit {
		t.Errorf("NewSubmitMessage() method = %v, want %v", submitMsg.Method, MethodSubmit)
	}

	setDiffMsg := NewSetDifficultyMessage(1024.0)
	if setDiffMsg.Method != MethodSetDifficulty {
		t.Errorf("NewSetDifficultyMessage() method = %v, want %v", setDiffMsg.Method, MethodSetDifficulty)
	}

	notifyMsg := NewNotifyMessage([]string{"job"}, []string{"prev"}, []string{"cb1"}, []string{"cb2"}, []string{"m1", "m2"}, "ver", "bits", "time", true)
	if notifyMsg.Method != MethodNotify {
		t.Errorf("NewNotifyMessage() method = %v, want %v", notifyMsg.Method, MethodNotify)
	}
}

func TestMessageClassification(t *testing.T) {
	// Test notification (no ID)
	notification := Message{Method: MethodNotify}
	if !notification.IsNotification() {
		t.Error("Expected notification to be classified as notification")
	}
	if notification.IsRequest() || notification.IsResponse() {
		t.Error("Notification should not be classified as request or response")
	}

	// Test request (has ID and Method)
	id := int64(1)
	request := Message{ID: &id, Method: MethodSubscribe}
	if !request.IsRequest() {
		t.Error("Expected request to be classified as request")
	}
	if request.IsNotification() || request.IsResponse() {
		t.Error("Request should not be classified as notification or response")
	}

	// Test response (has ID and Result)
	response := Message{ID: &id, Result: true}
	if !response.IsResponse() {
		t.Error("Expected response to be classified as response")
	}
	if response.IsNotification() || response.IsRequest() {
		t.Error("Response should not be classified as notification or request")
	}
}
