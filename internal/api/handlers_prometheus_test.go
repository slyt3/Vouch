package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/slyt3/Logryph/internal/assert"
	"github.com/slyt3/Logryph/internal/core"
	"github.com/slyt3/Logryph/internal/ledger"
	"github.com/slyt3/Logryph/internal/ledger/store"
	"github.com/slyt3/Logryph/internal/pool"
)

const maxWaitTicks = 50

func TestHandlePrometheusIncludesQueueAndLatency(t *testing.T) {
	engine, worker, cleanup := setupTestEngine(t)
	defer cleanup()

	emitTestEvent(worker)
	waitForProcessed(t, worker, 1, 2*time.Second)

	body := fetchPrometheusBody(t, engine)
	if !strings.Contains(body, "logryph_ledger_queue_depth") {
		t.Fatalf("missing queue depth metric")
	}
	if !strings.Contains(body, "logryph_ledger_queue_capacity") {
		t.Fatalf("missing queue capacity metric")
	}
	if !strings.Contains(body, "logryph_ledger_event_latency_seconds_bucket") {
		t.Fatalf("missing latency histogram bucket")
	}
	if !strings.Contains(body, "logryph_ledger_events_blocked_total") {
		t.Fatalf("missing blocked events metric")
	}
	if !strings.Contains(body, "logryph_ledger_backpressure_mode") {
		t.Fatalf("missing backpressure mode metric")
	}
}

func TestHandlePrometheusLatencyCountAndSum(t *testing.T) {
	engine, worker, cleanup := setupTestEngine(t)
	defer cleanup()

	emitTestEvent(worker)
	waitForProcessed(t, worker, 1, 2*time.Second)

	body := fetchPrometheusBody(t, engine)
	if !strings.Contains(body, "logryph_ledger_event_latency_seconds_sum") {
		t.Fatalf("missing latency sum metric")
	}
	if !strings.Contains(body, "logryph_ledger_event_latency_seconds_count") {
		t.Fatalf("missing latency count metric")
	}
}

func setupTestEngine(t *testing.T) (*core.Engine, *ledger.Worker, func()) {
	tempDir := t.TempDir()
	if err := assert.Check(tempDir != "", "temp dir must not be empty"); err != nil {
		t.Fatalf("temp dir invalid: %v", err)
	}

	dbPath := filepath.Join(tempDir, "logryph_test.db")
	keyPath := filepath.Join(tempDir, "test.key")
	if err := assert.Check(dbPath != "" && keyPath != "", "paths must not be empty"); err != nil {
		t.Fatalf("paths invalid: %v", err)
	}

	dbStore, err := store.NewDB(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	worker, err := ledger.NewWorker(16, dbStore, keyPath)
	if err != nil {
		if err2 := dbStore.Close(); err2 != nil {
			t.Fatalf("failed to close db after worker init error: %v", err2)
		}
		t.Fatalf("failed to create worker: %v", err)
	}
	if err := worker.Start(); err != nil {
		_ = worker.Close()
		t.Fatalf("failed to start worker: %v", err)
	}

	engine := &core.Engine{Worker: worker, ActiveTasks: &sync.Map{}}
	if err := assert.NotNil(engine, "engine"); err != nil {
		_ = worker.Close()
		t.Fatalf("engine invalid: %v", err)
	}
	if err := assert.NotNil(engine.Worker, "worker"); err != nil {
		_ = worker.Close()
		t.Fatalf("worker invalid: %v", err)
	}

	cleanup := func() {
		if err := worker.Shutdown(2 * time.Second); err != nil {
			t.Fatalf("failed to shutdown worker: %v", err)
		}
	}

	return engine, worker, cleanup
}

func emitTestEvent(worker *ledger.Worker) {
	if err := assert.NotNil(worker, "worker"); err != nil {
		return
	}
	event := pool.GetEvent()
	event.ID = "evt-test"
	event.Timestamp = time.Now()
	event.Actor = "test"
	event.EventType = "tool_call"
	event.Method = "os.read"
	worker.Submit(event)
}

func fetchPrometheusBody(t *testing.T, engine *core.Engine) string {
	if err := assert.NotNil(engine, "engine"); err != nil {
		t.Fatalf("engine invalid: %v", err)
	}
	if err := assert.NotNil(engine.Worker, "worker"); err != nil {
		t.Fatalf("worker invalid: %v", err)
	}

	h := NewHandlers(engine)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	h.HandlePrometheus(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	return rec.Body.String()
}

func waitForProcessed(t *testing.T, worker *ledger.Worker, min uint64, timeout time.Duration) {
	if err := assert.NotNil(worker, "worker"); err != nil {
		t.Fatalf("worker invalid: %v", err)
	}
	if err := assert.Check(timeout > 0, "timeout must be positive"); err != nil {
		t.Fatalf("timeout invalid: %v", err)
	}

	step := timeout / maxWaitTicks
	if step == 0 {
		step = time.Millisecond
	}
	if err := assert.Check(step > 0, "step must be positive"); err != nil {
		t.Fatalf("step invalid: %v", err)
	}

	for i := 0; i < maxWaitTicks; i++ {
		processed, _ := worker.Stats()
		if processed >= min {
			return
		}
		time.Sleep(step)
	}
	processed, _ := worker.Stats()
	t.Fatalf("timeout waiting for processed events: %d", processed)
}
