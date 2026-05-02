package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP layer
	HttpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gossipdb_http_request_duration_seconds",
			Help:    "HTTP request latency broken down by method, path, and status.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)

	// Write throughput
	WritesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gossipdb_writes_total",
			Help: "Total number of write operations, by type (local | replicated).",
		},
		[]string{"type"},
	)

	// Replication lag: seconds between a value's original timestamp and when we applied it
	ReplicationLagSeconds = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "gossipdb_replication_lag_seconds",
			Help:    "Time between a value being written on the origin node and applied on this node.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5},
		},
	)

	// Number of active SSE watchers
	ActiveWatchers = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "gossipdb_active_watchers",
			Help: "Number of currently active SSE watch connections.",
		},
	)

	// Gossip rounds
	GossipRoundsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gossipdb_gossip_rounds_total",
			Help: "Total number of gossip sync rounds attempted.",
		},
	)

	GossipErrorsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gossipdb_gossip_errors_total",
			Help: "Total number of failed gossip sync rounds.",
		},
	)

	// Keys replicated per gossip round
	GossipKeysReplicated = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "gossipdb_gossip_keys_replicated_total",
			Help: "Total number of key updates applied via gossip.",
		},
	)
)
