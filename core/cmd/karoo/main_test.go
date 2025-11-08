package main

import (
	"testing"

	"github.com/carlosrabelo/karoo/core/internal/proxy"
	"github.com/carlosrabelo/karoo/core/internal/stratum"
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
			name:    "invalid type",
			input:   "not array or map",
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
			got := stratum.ParseExtranonceResult(tt.input)
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
			name:   "string positive",
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
			name:   "string invalid",
			input:  "abc",
			want:   0,
			wantOK: false,
		},
		{
			name:   "empty string",
			input:  "",
			want:   0,
			wantOK: false,
		},
		{
			name:   "invalid type",
			input:  []int{},
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
			got, gotOK := stratum.ParseExtranonceSize(tt.input)
			if got != tt.want {
				t.Errorf("parseExtranonceSize() got = %v, want %v", got, tt.want)
			}
			if gotOK != tt.wantOK {
				t.Errorf("parseExtranonceSize() gotOK = %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

func TestProxyCreation(t *testing.T) {
	cfg := &proxy.Config{}
	p := proxy.NewProxy(cfg)

	if p == nil {
		t.Fatal("NewProxy returned nil")
	}

	// Test that proxy was created successfully
	// We can't test internal fields directly since they're not exported
	// but we can verify the proxy is not nil
}
