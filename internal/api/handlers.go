package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/slyt3/Logryph/internal/assert"
	"github.com/slyt3/Logryph/internal/core"
	"github.com/slyt3/Logryph/internal/ledger"
	"github.com/slyt3/Logryph/internal/logging"
	"github.com/slyt3/Logryph/internal/pool"
)

// LatencySnapshot is aliased from ledger package for clarity
type LatencySnapshot = ledger.LatencySnapshot

// Handlers provides HTTP endpoints for admin operations, metrics, and health probes.
// All handlers are mounted on the admin server (default :9998).
type Handlers struct {
	Core *core.Engine
}

// NewHandlers creates a new handlers instance with the provided core engine.
// The engine must contain an initialized Worker for metrics and health checks.
func NewHandlers(engine *core.Engine) *Handlers {
	return &Handlers{Core: engine}
}

// HandleRekey rotates the Ed25519 signing key and returns the new public key.
// Requires POST method and X-Admin-Token header if LOGRYPH_ADMIN_TOKEN is set.
// Returns 405 for non-POST, 401 for missing/invalid token, 500 on rotation failure.
func (h *Handlers) HandleRekey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	adminToken := os.Getenv("LOGRYPH_ADMIN_TOKEN")
	if adminToken != "" {
		if r.Header.Get("X-Admin-Token") != adminToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	oldPubKey, newPubKey, err := h.Core.Worker.GetSigner().RotateKey(".logryph_key")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintf(w, "Key rotated\nOld: %s\nNew: %s", oldPubKey, newPubKey); err != nil {
		logging.Error("rekey_response_write_failed", logging.Fields{Component: "api", Error: err.Error()})
	}
}

// HandleStats returns pool metrics (event/buffer hits and misses) as JSON.
// Always returns 200 OK with pool statistics.
func (h *Handlers) HandleStats(w http.ResponseWriter, r *http.Request) {
	metrics := pool.GetMetrics()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(metrics); err != nil {
		logging.Error("stats_encode_failed", logging.Fields{Component: "api", Error: err.Error()})
	}
}

// HandleHealth is a Kubernetes liveness probe endpoint.
// Always returns 200 OK to indicate the process is alive.
func (h *Handlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if err := assert.NotNil(h, "handlers"); err != nil {
		return
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok")); err != nil {
		logging.Error("health_response_write_failed", logging.Fields{Component: "api", Error: err.Error()})
	}
}

// HandleReady is a Kubernetes readiness probe endpoint.
// Returns 200 OK only if worker is healthy, signer is available, and database is available.
// Returns 503 Service Unavailable if any dependency is not ready.
func (h *Handlers) HandleReady(w http.ResponseWriter, r *http.Request) {
	if err := assert.NotNil(h, "handlers"); err != nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := assert.NotNil(h.Core, "core"); err != nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := assert.NotNil(h.Core.Worker, "worker"); err != nil {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}

	if !h.Core.Worker.IsHealthy() {
		http.Error(w, "worker unhealthy", http.StatusServiceUnavailable)
		return
	}

	if h.Core.Worker.GetSigner() == nil {
		http.Error(w, "signer unavailable", http.StatusServiceUnavailable)
		return
	}

	if h.Core.Worker.GetDB() == nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ready")); err != nil {
		logging.Error("ready_response_write_failed", logging.Fields{Component: "api", Error: err.Error()})
	}
}

// HandlePrometheus exports metrics in Prometheus text format.
// Includes counters (events processed/dropped), gauges (queue depth, active tasks),
// and histograms (processing latency buckets).
func (h *Handlers) HandlePrometheus(w http.ResponseWriter, r *http.Request) {
	if err := assert.NotNil(h, "handlers"); err != nil {
		return
	}
	if err := assert.NotNil(h.Core, "core"); err != nil {
		return
	}

	metrics := h.collectMetrics()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	h.formatPrometheusText(w, metrics)
}

// prometheusMetrics holds all metrics for Prometheus export
type prometheusMetrics struct {
	PoolEventHits    uint64
	PoolEventMisses  uint64
	EventsProcessed  uint64
	EventsDropped    uint64
	EventsBlocked    uint64
	BackpressureMode string
	ActiveTasks      int
	QueueDepth       int
	QueueCapacity    int
	LatencyMetrics   LatencySnapshot
}

// collectMetrics gathers all metrics from the system
func (h *Handlers) collectMetrics() *prometheusMetrics {
	if err := assert.NotNil(h, "handlers"); err != nil {
		return &prometheusMetrics{}
	}
	if err := assert.NotNil(h.Core, "core"); err != nil {
		return &prometheusMetrics{}
	}
	if err := assert.NotNil(h.Core.Worker, "worker"); err != nil {
		return &prometheusMetrics{}
	}

	poolMetrics := pool.GetMetrics()
	proc, drop := h.Core.Worker.Stats()
	queueDepth, queueCap := h.Core.Worker.QueueDepth()
	latency := h.Core.Worker.LatencyMetrics()
	blocked := h.Core.Worker.BlockedSubmits()
	mode := h.Core.Worker.BackpressureMode()
	modeLabel := "drop"
	if mode == ledger.BackpressureBlock {
		modeLabel = "block"
	}

	if err := assert.Check(queueCap >= 0, "queue capacity must be non-negative"); err != nil {
		logging.Warn("queue_capacity_invalid", logging.Fields{Component: "api", Error: err.Error()})
	}

	tasks := 0
	const maxActiveTasks = 10000
	h.Core.ActiveTasks.Range(func(_, _ interface{}) bool {
		if tasks >= maxActiveTasks {
			return false
		}
		tasks++
		return true
	})
	if err := assert.Check(tasks <= maxActiveTasks, "active tasks exceeded cap: %d", tasks); err != nil {
		logging.Warn("active_tasks_exceeded", logging.Fields{Component: "api", Error: err.Error()})
	}

	return &prometheusMetrics{
		PoolEventHits:    poolMetrics.EventHits,
		PoolEventMisses:  poolMetrics.EventMisses,
		EventsProcessed:  proc,
		EventsDropped:    drop,
		EventsBlocked:    blocked,
		BackpressureMode: modeLabel,
		ActiveTasks:      tasks,
		QueueDepth:       queueDepth,
		QueueCapacity:    queueCap,
		LatencyMetrics:   latency,
	}
}

// formatPrometheusText writes metrics in Prometheus text format
func (h *Handlers) formatPrometheusText(w http.ResponseWriter, m *prometheusMetrics) {
	if err := assert.NotNil(m, "metrics"); err != nil {
		return
	}
	if err := assert.NotNil(w, "response writer"); err != nil {
		return
	}

	writef := func(format string, args ...interface{}) bool {
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			logging.Error("prometheus_write_failed", logging.Fields{Component: "api", Error: err.Error()})
			return false
		}
		return true
	}

	if !writef("# HELP logryph_pool_event_hits_total Total hits on the event pool\n") {
		return
	}
	if !writef("# TYPE logryph_pool_event_hits_total counter\n") {
		return
	}
	if !writef("logryph_pool_event_hits_total %d\n", m.PoolEventHits) {
		return
	}

	if !writef("# HELP logryph_pool_event_misses_total Total misses (allocations) in the event pool\n") {
		return
	}
	if !writef("# TYPE logryph_pool_event_misses_total counter\n") {
		return
	}
	if !writef("logryph_pool_event_misses_total %d\n", m.PoolEventMisses) {
		return
	}

	if !writef("# HELP logryph_ledger_events_processed_total Total events successfully written to the ledger\n") {
		return
	}
	if !writef("# TYPE logryph_ledger_events_processed_total counter\n") {
		return
	}
	if !writef("logryph_ledger_events_processed_total %d\n", m.EventsProcessed) {
		return
	}

	if !writef("# HELP logryph_ledger_events_dropped_total Total events dropped due to backpressure\n") {
		return
	}
	if !writef("# TYPE logryph_ledger_events_dropped_total counter\n") {
		return
	}
	if !writef("logryph_ledger_events_dropped_total %d\n", m.EventsDropped) {
		return
	}

	if !writef("# HELP logryph_ledger_events_blocked_total Total submit attempts blocked by backpressure\n") {
		return
	}
	if !writef("# TYPE logryph_ledger_events_blocked_total counter\n") {
		return
	}
	if !writef("logryph_ledger_events_blocked_total %d\n", m.EventsBlocked) {
		return
	}

	if !writef("# HELP logryph_ledger_backpressure_mode Current backpressure mode (drop|block)\n") {
		return
	}
	if !writef("# TYPE logryph_ledger_backpressure_mode gauge\n") {
		return
	}
	if !writef("logryph_ledger_backpressure_mode{mode=\"%s\"} 1\n", m.BackpressureMode) {
		return
	}

	if !writef("# HELP logryph_engine_active_tasks_total Number of currently active causal tasks\n") {
		return
	}
	if !writef("# TYPE logryph_engine_active_tasks_total gauge\n") {
		return
	}
	if !writef("logryph_engine_active_tasks_total %d\n", m.ActiveTasks) {
		return
	}

	if !writef("# HELP logryph_ledger_queue_depth Current queue depth\n") {
		return
	}
	if !writef("# TYPE logryph_ledger_queue_depth gauge\n") {
		return
	}
	if !writef("logryph_ledger_queue_depth %d\n", m.QueueDepth) {
		return
	}

	if !writef("# HELP logryph_ledger_queue_capacity Queue capacity\n") {
		return
	}
	if !writef("# TYPE logryph_ledger_queue_capacity gauge\n") {
		return
	}
	if !writef("logryph_ledger_queue_capacity %d\n", m.QueueCapacity) {
		return
	}

	h.formatLatencyHistogram(w, &m.LatencyMetrics)
}

// formatLatencyHistogram writes the latency histogram in Prometheus format
func (h *Handlers) formatLatencyHistogram(w http.ResponseWriter, latency *LatencySnapshot) {
	if err := assert.NotNil(latency, "latency histogram"); err != nil {
		return
	}
	if err := assert.NotNil(w, "response writer"); err != nil {
		return
	}

	writef := func(format string, args ...interface{}) bool {
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			logging.Error("prometheus_write_failed", logging.Fields{Component: "api", Error: err.Error()})
			return false
		}
		return true
	}

	if !writef("# HELP logryph_ledger_event_latency_seconds Event processing latency\n") {
		return
	}
	if !writef("# TYPE logryph_ledger_event_latency_seconds histogram\n") {
		return
	}

	const maxBuckets = 20
	for i := 0; i < maxBuckets; i++ {
		if i >= len(latency.BoundsNs) {
			break
		}
		upper := latency.BoundsNs[i]
		label := ""
		if upper == ^uint64(0) {
			label = "+Inf"
		} else {
			label = fmt.Sprintf("%.6f", float64(upper)/float64(time.Second))
		}
		if !writef("logryph_ledger_event_latency_seconds_bucket{le=\"%s\"} %d\n", label, latency.Counts[i]) {
			return
		}
	}
	if !writef("logryph_ledger_event_latency_seconds_sum %.6f\n", float64(latency.SumNs)/float64(time.Second)) {
		return
	}
	if !writef("logryph_ledger_event_latency_seconds_count %d\n", latency.Count) {
		return
	}
}
