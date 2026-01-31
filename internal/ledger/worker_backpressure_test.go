package ledger

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/slyt3/Logryph/internal/assert"
	"github.com/slyt3/Logryph/internal/pool"
)

const maxBlockWait = 2 * time.Second

func TestBackpressureModeSetAndGet(t *testing.T) {
	oldStrictMode := assert.StrictMode
	oldSuppressLogs := assert.SuppressLogs
	assert.StrictMode = false
	assert.SuppressLogs = true
	defer func() {
		assert.StrictMode = oldStrictMode
		assert.SuppressLogs = oldSuppressLogs
	}()

	worker, cleanup := newTestWorker(t, 4)
	defer cleanup()

	if err := worker.SetBackpressureMode(BackpressureBlock); err != nil {
		t.Fatalf("failed to set backpressure mode: %v", err)
	}
	if err := assert.NotNil(worker, "worker"); err != nil {
		t.Fatalf("worker invalid: %v", err)
	}
	mode := worker.BackpressureMode()
	if err := assert.Check(mode == BackpressureBlock, "expected block mode"); err != nil {
		t.Fatalf("mode invalid: %v", err)
	}
}

func TestSubmitDropModeDropsOnFullBuffer(t *testing.T) {
	oldStrictMode := assert.StrictMode
	oldSuppressLogs := assert.SuppressLogs
	assert.StrictMode = false
	assert.SuppressLogs = true
	defer func() {
		assert.StrictMode = oldStrictMode
		assert.SuppressLogs = oldSuppressLogs
	}()

	worker, cleanup := newTestWorker(t, 1)
	defer cleanup()

	if err := worker.SetBackpressureMode(BackpressureDrop); err != nil {
		t.Fatalf("failed to set backpressure mode: %v", err)
	}
	if err := assert.NotNil(worker, "worker"); err != nil {
		t.Fatalf("worker invalid: %v", err)
	}

	event1 := pool.GetEvent()
	defer pool.PutEvent(event1)
	event1.ID = "evt-1"
	worker.Submit(event1)

	event2 := pool.GetEvent()
	defer pool.PutEvent(event2)
	event2.ID = "evt-2"
	worker.Submit(event2)

	_, dropped := worker.Stats()
	if err := assert.Check(dropped == 1, "expected 1 dropped event"); err != nil {
		t.Fatalf("drop count invalid: %v", err)
	}
}

func TestSubmitBlockModeBlocksAndDropsOnTimeout(t *testing.T) {
	oldStrictMode := assert.StrictMode
	oldSuppressLogs := assert.SuppressLogs
	assert.StrictMode = false
	assert.SuppressLogs = true
	defer func() {
		assert.StrictMode = oldStrictMode
		assert.SuppressLogs = oldSuppressLogs
	}()

	worker, cleanup := newTestWorker(t, 1)
	defer cleanup()

	if err := worker.SetBackpressureMode(BackpressureBlock); err != nil {
		t.Fatalf("failed to set backpressure mode: %v", err)
	}

	event1 := pool.GetEvent()
	defer pool.PutEvent(event1)
	event1.ID = "evt-1"
	worker.Submit(event1)

	event2 := pool.GetEvent()
	defer pool.PutEvent(event2)
	event2.ID = "evt-2"
	start := time.Now()
	worker.Submit(event2)
	elapsed := time.Since(start)

	if err := assert.Check(elapsed <= maxBlockWait, "block wait exceeded max: %v", elapsed); err != nil {
		t.Fatalf("block wait invalid: %v", err)
	}
	if err := assert.Check(worker.BlockedSubmits() > 0, "expected blocked submits > 0"); err != nil {
		t.Fatalf("blocked submits invalid: %v", err)
	}
	_, dropped := worker.Stats()
	if err := assert.Check(dropped == 1, "expected 1 dropped event"); err != nil {
		t.Fatalf("drop count invalid: %v", err)
	}
}

func newTestWorker(t *testing.T, bufferSize int) (*Worker, func()) {
	if err := assert.Check(t != nil, "test handle must not be nil"); err != nil {
		t.Fatalf("test handle invalid: %v", err)
	}
	if err := assert.Check(bufferSize > 0, "buffer size must be positive"); err != nil {
		t.Fatalf("buffer size invalid: %v", err)
	}

	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "test.key")
	repo := &mockEventRepository{}

	worker, err := NewWorker(bufferSize, repo, keyPath)
	if err != nil {
		t.Fatalf("failed to create worker: %v", err)
	}

	cleanup := func() {
		if err := os.Remove(keyPath); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove key: %v", err)
		}
	}

	return worker, cleanup
}
