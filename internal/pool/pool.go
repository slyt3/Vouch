package pool

import (
	"bytes"
	"sync"
	"sync/atomic"

	"github.com/slyt3/Logryph/internal/assert"
	"github.com/slyt3/Logryph/internal/models"
)

// Metrics tracks pool performance with hit/miss counters for events and buffers.
// EventHits/EventMisses track event pool reuse, BufferHits/BufferMisses track buffer pool reuse.
// Higher hit rates indicate better memory efficiency.
type Metrics struct {
	EventHits    uint64
	EventMisses  uint64
	BufferHits   uint64
	BufferMisses uint64
}

var globalMetrics Metrics

const maxEventFields = 256

// GetMetrics returns a snapshot of current pool metrics.
// Safe for concurrent access. Use for monitoring zero-allocation performance.
func GetMetrics() Metrics {
	return Metrics{
		EventHits:    atomic.LoadUint64(&globalMetrics.EventHits),
		EventMisses:  atomic.LoadUint64(&globalMetrics.EventMisses),
		BufferHits:   atomic.LoadUint64(&globalMetrics.BufferHits),
		BufferMisses: atomic.LoadUint64(&globalMetrics.BufferMisses),
	}
}

var eventPool = sync.Pool{
	New: func() interface{} {
		atomic.AddUint64(&globalMetrics.EventMisses, 1)
		return &models.Event{
			Params: make(map[string]interface{}, 8),
		}
	},
}

// GetEvent acquires an event from the pool for zero-allocation hot path.
// Returns a clean Event with pre-allocated Params map. Always defer PutEvent() to avoid leaks.
// Increments EventHits metric.
func GetEvent() *models.Event {
	if err := assert.Check(eventPool.New != nil, "eventPool.New must be defined"); err != nil {
		return &models.Event{}
	}
	e := eventPool.Get().(*models.Event)
	atomic.AddUint64(&globalMetrics.EventHits, 1)
	return e
}

// PutEvent returns an event to the pool after clearing all fields to prevent data leakage.
// Safe to call with nil (no-op). Must be called after GetEvent() to avoid memory leaks.
// Does NOT return events with oversized maps (maxEventFields check).
func PutEvent(e *models.Event) {
	if e == nil {
		return
	}
	// Reset all fields to prevent data leakage between requests
	e.ID = ""
	e.RunID = ""
	e.SeqIndex = 0
	e.Timestamp = e.Timestamp.Truncate(0)
	e.Actor = ""
	e.EventType = ""
	e.Method = ""
	e.PrevHash = ""
	e.CurrentHash = ""
	e.Signature = ""
	e.TaskID = ""
	e.TaskState = ""
	e.ParentID = ""
	e.PolicyID = ""
	e.RiskLevel = ""
	e.WasBlocked = false

	// Clear maps but keep allocated capacity
	if err := assert.Check(len(e.Params) <= maxEventFields, "params map too large: %d", len(e.Params)); err != nil {
		return
	}
	if err := assert.Check(len(e.Response) <= maxEventFields, "response map too large: %d", len(e.Response)); err != nil {
		return
	}
	for i := 0; i < maxEventFields; i++ {
		key := ""
		found := false
		for k := range e.Params {
			key = k
			found = true
			break
		}
		if !found {
			break
		}
		delete(e.Params, key)
	}
	for i := 0; i < maxEventFields; i++ {
		key := ""
		found := false
		for k := range e.Response {
			key = k
			found = true
			break
		}
		if !found {
			break
		}
		delete(e.Response, key)
	}

	eventPool.Put(e)
}

var bufferPool = sync.Pool{
	New: func() interface{} {
		atomic.AddUint64(&globalMetrics.BufferMisses, 1)
		return bytes.NewBuffer(make([]byte, 0, 4096))
	},
}

const maxBufferSize = 1024 * 1024 // 1MB limit for pooling

// GetBuffer acquires a bytes.Buffer from the pool for zero-allocation I/O.
// Pre-allocated with 4KB capacity. Always defer PutBuffer() to avoid leaks.
// Increments BufferHits metric.
func GetBuffer() *bytes.Buffer {
	if err := assert.Check(bufferPool.New != nil, "bufferPool.New must be defined"); err != nil {
		return bytes.NewBuffer(nil)
	}
	atomic.AddUint64(&globalMetrics.BufferHits, 1)
	return bufferPool.Get().(*bytes.Buffer)
}

// PutBuffer returns a buffer to the pool after resetting it.
// Safe to call with nil (no-op). Drops buffers exceeding maxBufferSize (1MB) to prevent bloat.
// Must be called after GetBuffer() to avoid memory leaks.
func PutBuffer(b *bytes.Buffer) {
	if b == nil {
		return
	}
	// If the buffer grew too large, don't return it to the pool to avoid memory bloat
	if b.Cap() > maxBufferSize {
		return
	}
	if err := assert.Check(b.Cap() <= maxBufferSize*2, "buffer grew dangerously large: cap=%d", b.Cap()); err != nil {
		return
	}
	b.Reset()
	bufferPool.Put(b)
}
