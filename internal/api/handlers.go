package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/slyt3/Vouch/internal/assert"
	"github.com/slyt3/Vouch/internal/core"
	"github.com/slyt3/Vouch/internal/logging"
	"github.com/slyt3/Vouch/internal/pool"
)

type Handlers struct {
	Core *core.Engine
}

func NewHandlers(engine *core.Engine) *Handlers {
	return &Handlers{Core: engine}
}

func (h *Handlers) HandleRekey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	adminToken := os.Getenv("VOUCH_ADMIN_TOKEN")
	if adminToken != "" {
		if r.Header.Get("X-Admin-Token") != adminToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	oldPubKey, newPubKey, err := h.Core.Worker.GetSigner().RotateKey(".vouch_key")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintf(w, "Key rotated\nOld: %s\nNew: %s", oldPubKey, newPubKey); err != nil {
		logging.Error("rekey_response_write_failed", logging.Fields{Component: "api", Error: err.Error()})
	}
}

func (h *Handlers) HandleStats(w http.ResponseWriter, r *http.Request) {
	metrics := pool.GetMetrics()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(metrics); err != nil {
		logging.Error("stats_encode_failed", logging.Fields{Component: "api", Error: err.Error()})
	}
}

func (h *Handlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	if err := assert.NotNil(h, "handlers"); err != nil {
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

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
	w.Write([]byte("ready"))
}

func (h *Handlers) HandlePrometheus(w http.ResponseWriter, r *http.Request) {
	if err := assert.NotNil(h, "handlers"); err != nil {
		return
	}
	if err := assert.NotNil(h.Core, "core"); err != nil {
		return
	}
	poolMetrics := pool.GetMetrics()
	proc, drop := h.Core.Worker.Stats()
	queueDepth, queueCap := h.Core.Worker.QueueDepth()
	latency := h.Core.Worker.LatencyMetrics()
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

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP vouch_pool_event_hits_total Total hits on the event pool\n")
	fmt.Fprintf(w, "# TYPE vouch_pool_event_hits_total counter\n")
	fmt.Fprintf(w, "vouch_pool_event_hits_total %d\n", poolMetrics.EventHits)

	fmt.Fprintf(w, "# HELP vouch_pool_event_misses_total Total misses (allocations) in the event pool\n")
	fmt.Fprintf(w, "# TYPE vouch_pool_event_misses_total counter\n")
	fmt.Fprintf(w, "vouch_pool_event_misses_total %d\n", poolMetrics.EventMisses)

	fmt.Fprintf(w, "# HELP vouch_ledger_events_processed_total Total events successfully written to the ledger\n")
	fmt.Fprintf(w, "# TYPE vouch_ledger_events_processed_total counter\n")
	fmt.Fprintf(w, "vouch_ledger_events_processed_total %d\n", proc)

	fmt.Fprintf(w, "# HELP vouch_ledger_events_dropped_total Total events dropped due to backpressure\n")
	fmt.Fprintf(w, "# TYPE vouch_ledger_events_dropped_total counter\n")
	fmt.Fprintf(w, "vouch_ledger_events_dropped_total %d\n", drop)

	fmt.Fprintf(w, "# HELP vouch_engine_active_tasks_total Number of currently active causal tasks\n")
	fmt.Fprintf(w, "# TYPE vouch_engine_active_tasks_total gauge\n")
	fmt.Fprintf(w, "vouch_engine_active_tasks_total %d\n", tasks)

	fmt.Fprintf(w, "# HELP vouch_ledger_queue_depth Current queue depth\n")
	fmt.Fprintf(w, "# TYPE vouch_ledger_queue_depth gauge\n")
	fmt.Fprintf(w, "vouch_ledger_queue_depth %d\n", queueDepth)

	fmt.Fprintf(w, "# HELP vouch_ledger_queue_capacity Queue capacity\n")
	fmt.Fprintf(w, "# TYPE vouch_ledger_queue_capacity gauge\n")
	fmt.Fprintf(w, "vouch_ledger_queue_capacity %d\n", queueCap)

	fmt.Fprintf(w, "# HELP vouch_ledger_event_latency_seconds Event processing latency\n")
	fmt.Fprintf(w, "# TYPE vouch_ledger_event_latency_seconds histogram\n")
	for i := 0; i < len(latency.BoundsNs); i++ {
		upper := latency.BoundsNs[i]
		label := ""
		if upper == ^uint64(0) {
			label = "+Inf"
		} else {
			label = fmt.Sprintf("%.6f", float64(upper)/float64(time.Second))
		}
		fmt.Fprintf(w, "vouch_ledger_event_latency_seconds_bucket{le=\"%s\"} %d\n", label, latency.Counts[i])
	}
	fmt.Fprintf(w, "vouch_ledger_event_latency_seconds_sum %.6f\n", float64(latency.SumNs)/float64(time.Second))
	fmt.Fprintf(w, "vouch_ledger_event_latency_seconds_count %d\n", latency.Count)
}
