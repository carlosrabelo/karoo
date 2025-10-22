package metrics

import (
	"sync/atomic"
	"time"
)

type Metrics struct {
	requestsTotal int64
	errorsTotal   int64
	lastRequest   int64
}

var Default = New()

func New() *Metrics {
	return &Metrics{}
}

func (m *Metrics) IncrementRequests() {
	atomic.AddInt64(&m.requestsTotal, 1)
	atomic.StoreInt64(&m.lastRequest, time.Now().Unix())
}

func (m *Metrics) IncrementErrors() {
	atomic.AddInt64(&m.errorsTotal, 1)
}

func (m *Metrics) GetRequests() int64 {
	return atomic.LoadInt64(&m.requestsTotal)
}

func (m *Metrics) GetErrors() int64 {
	return atomic.LoadInt64(&m.errorsTotal)
}

func (m *Metrics) GetLastRequest() int64 {
	return atomic.LoadInt64(&m.lastRequest)
}

func IncrementRequests() {
	Default.IncrementRequests()
}

func IncrementErrors() {
	Default.IncrementErrors()
}
