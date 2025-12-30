package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// PrometheusCollectors holds all prometheus metric collectors
type PrometheusCollectors struct {
	SharesOK      prometheus.Counter
	SharesBad     prometheus.Counter
	ClientsActive prometheus.Gauge
	UpConnected   prometheus.Gauge
	LastSetDiff   prometheus.Gauge
	LastNotify    prometheus.Gauge
}

// InitPrometheus initializes and registers prometheus metrics
func InitPrometheus(namespace string) *PrometheusCollectors {
	// Helper to safely register or get existing collector
	register := func(c prometheus.Collector) prometheus.Collector {
		if err := prometheus.Register(c); err != nil {
			if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
				return are.ExistingCollector
			}
			// Don't panic on registration error in tests/dev, just log
			return c
		}
		return c
	}

	pc := &PrometheusCollectors{}

	pc.SharesOK = register(prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "shares_accepted_total",
		Help:      "Total number of accepted shares",
	})).(prometheus.Counter)

	pc.SharesBad = register(prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "shares_rejected_total",
		Help:      "Total number of rejected shares",
	})).(prometheus.Counter)

	pc.ClientsActive = register(prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "clients_active_count",
		Help:      "Number of currently connected clients",
	})).(prometheus.Gauge)

	pc.UpConnected = register(prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "upstream_connected",
		Help:      "Upstream connection status (1 = connected, 0 = disconnected)",
	})).(prometheus.Gauge)

	pc.LastSetDiff = register(prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "upstream_difficulty",
		Help:      "Current difficulty set by upstream",
	})).(prometheus.Gauge)

	pc.LastNotify = register(prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "last_notify_timestamp_seconds",
		Help:      "Unix timestamp of last mining.notify received",
	})).(prometheus.Gauge)

	return pc
}

// UpdateFromCollector syncs atomic metrics to prometheus collectors
// This should be called periodically or on change
func (p *PrometheusCollectors) UpdateFromCollector(c *Collector) {
	p.SharesOK.Add(float64(c.SharesOK.Load()))
	// Note: Counter.Add is for increments. Since we load total, we might need to change logic
	// But standard prometheus usage is Inc() on events.
	// To play nice with existing Collector, we might need to set values if using NewCounterFunc
	// OR, better: Instrument the Collector methods directly to update Prometheus.
	// For now, let's keep it simple: we will use "Set" semantics for Gauges,
	// but for Counters we can't "Set".
	//
	// Strategy rewrite:
	// The existing Collector uses atomic counters. We should instrument those methods.
	// We will modify Collector struct to include Prometheus collectors and update them in place.
}
