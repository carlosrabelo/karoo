// Package stratum implements Stratum V1 protocol parsing and utilities
package stratum

import (
	"encoding/json"
	"math/big"
	"net"
	"strconv"
	"strings"
	"time"
)

// Message represents a Stratum V1 JSON message
type Message struct {
	ID     *int64      `json:"id,omitempty"`
	Method string      `json:"method,omitempty"`
	Params interface{} `json:"params,omitempty"`
	Result interface{} `json:"result,omitempty"`
	Error  interface{} `json:"error,omitempty"`
}

// ExtranonceInfo contains extranonce information from mining.subscribe response
type ExtranonceInfo struct {
	Extranonce1     string
	Extranonce2Size int
	Valid           bool
}

// ParseExtranonceResult extracts extranonce information from subscribe response
func ParseExtranonceResult(res interface{}) ExtranonceInfo {
	switch v := res.(type) {
	case []interface{}:
		if len(v) < 3 {
			return ExtranonceInfo{}
		}
		ex1, ok1 := v[1].(string)
		ex2, ok2 := ParseExtranonceSize(v[2])
		if !ok1 || !ok2 {
			return ExtranonceInfo{}
		}
		return ExtranonceInfo{
			Extranonce1:     ex1,
			Extranonce2Size: ex2,
			Valid:           ex1 != "" && ex2 > 0,
		}
	case map[string]interface{}:
		ex1Raw, ok1 := v["extranonce1"]
		ex2Raw, ok2 := v["extranonce2_size"]
		if !ok1 || !ok2 {
			return ExtranonceInfo{}
		}
		ex1, ok1 := ex1Raw.(string)
		ex2, ok2 := ParseExtranonceSize(ex2Raw)
		if !ok1 || !ok2 {
			return ExtranonceInfo{}
		}
		return ExtranonceInfo{
			Extranonce1:     ex1,
			Extranonce2Size: ex2,
			Valid:           ex1 != "" && ex2 > 0,
		}
	default:
		return ExtranonceInfo{}
	}
}

// ParseExtranonceSize parses extranonce2 size from various input types
func ParseExtranonceSize(v interface{}) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), int(t) > 0
	case string:
		if t == "" {
			return 0, false
		}
		n, err := strconv.Atoi(t)
		if err != nil {
			return 0, false
		}
		return n, n > 0
	default:
		return 0, false
	}
}

// FormatDuration formats a duration for logging, returns "-" for non-positive values
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	d = d.Round(time.Millisecond)
	return d.String()
}

// DiffFromBits converts mining difficulty bits to decimal difficulty
func DiffFromBits(bits string) float64 {
	bits = strings.TrimPrefix(bits, "0x")
	if bits == "" {
		return 0
	}
	val, err := strconv.ParseUint(bits, 16, 32)
	if err != nil {
		return 0
	}
	exponent := byte(val >> 24)
	mantissa := val & 0xFFFFFF
	if mantissa == 0 || exponent <= 3 {
		return 0
	}
	target := new(big.Int).Lsh(big.NewInt(int64(mantissa)), uint(8*(int(exponent)-3)))
	if target.Sign() <= 0 {
		return 0
	}
	diffOne := new(big.Int).Lsh(big.NewInt(0xFFFF), uint(8*(0x1d-3)))
	t := new(big.Float).SetInt(target)
	d := new(big.Float).SetInt(diffOne)
	res := new(big.Float).Quo(d, t)
	out, _ := res.Float64()
	return out
}

// CopyID creates a deep copy of an int64 pointer
func CopyID(id *int64) *int64 {
	if id == nil {
		return nil
	}
	dup := new(int64)
	*dup = *id
	return dup
}

// ParseURL parses a Stratum URL into host and port components
func ParseURL(url string, host *string, port *int) {
	// Parse stratum+tcp://host:port or just host:port
	if strings.HasPrefix(url, "stratum+tcp://") {
		url = strings.TrimPrefix(url, "stratum+tcp://")
	}

	h, p, err := net.SplitHostPort(url)
	if err != nil {
		// Try adding default port
		h = url
		p = "3333"
	}

	*host = h
	if pr, err := strconv.Atoi(p); err == nil {
		*port = pr
	}
}

// Message types for better type safety
const (
	MethodSubscribe     = "mining.subscribe"
	MethodAuthorize     = "mining.authorize"
	MethodSubmit        = "mining.submit"
	MethodSetDifficulty = "mining.set_difficulty"
	MethodNotify        = "mining.notify"
	MethodConfigure     = "mining.configure"
)

// NewSubscribeMessage creates a new mining.subscribe message
func NewSubscribeMessage(userAgent string) Message {
	return Message{
		Method: MethodSubscribe,
		Params: []interface{}{userAgent},
	}
}

// NewAuthorizeMessage creates a new mining.authorize message
func NewAuthorizeMessage(username, password string) Message {
	return Message{
		Method: MethodAuthorize,
		Params: []interface{}{username, password},
	}
}

// NewSubmitMessage creates a new mining.submit message
func NewSubmitMessage(worker, jobID, extraNonce1, extraNonce2, nTime, nonce string) Message {
	return Message{
		Method: MethodSubmit,
		Params: []interface{}{worker, jobID, extraNonce1, extraNonce2, nTime, nonce},
	}
}

// NewSetDifficultyMessage creates a new mining.set_difficulty notification
func NewSetDifficultyMessage(difficulty float64) Message {
	return Message{
		Method: MethodSetDifficulty,
		Params: []interface{}{difficulty},
	}
}

// NewNotifyMessage creates a new mining.notify notification
func NewNotifyMessage(jobID, prevHash, coinbase1, coinbase2 []string, merkleBranch []string, version, nBits, nTime string, cleanJobs bool) Message {
	return Message{
		Method: MethodNotify,
		Params: []interface{}{
			jobID,
			prevHash,
			coinbase1,
			coinbase2,
			merkleBranch,
			version,
			nBits,
			nTime,
			cleanJobs,
		},
	}
}

// NewErrorResponse creates a new error response
func NewErrorResponse(id *int64, code int, message string, details interface{}) Message {
	return Message{
		ID:    id,
		Error: []interface{}{code, message, details},
	}
}

// NewSuccessResponse creates a new success response
func NewSuccessResponse(id *int64, result interface{}) Message {
	return Message{
		ID:     id,
		Result: result,
	}
}

// IsNotification returns true if the message is a notification (no ID)
func (m *Message) IsNotification() bool {
	return m.ID == nil
}

// IsRequest returns true if the message is a request (has ID and Method)
func (m *Message) IsRequest() bool {
	return m.ID != nil && m.Method != ""
}

// IsResponse returns true if the message is a response (has ID and Result/Error)
func (m *Message) IsResponse() bool {
	return m.ID != nil && (m.Result != nil || m.Error != nil)
}

// Marshal implements json.Marshaler with newline for Stratum protocol
func (m *Message) Marshal() ([]byte, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Unmarshal implements json.Unmarshaler for Message
func (m *Message) Unmarshal(data []byte) error {
	return json.Unmarshal(data, m)
}
