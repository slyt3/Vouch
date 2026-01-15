package pool

import (
	"bytes"
	"sync"
	"sync/atomic"

	"github.com/slyt3/Vouch/internal/assert"
	"github.com/slyt3/Vouch/internal/proxy"
)

// Metrics tracks pool performance
type Metrics struct {
	EventHits    uint64
	EventMisses  uint64
	BufferHits   uint64
	BufferMisses uint64
}

var globalMetrics Metrics

// GetMetrics returns a copy of the current pool metrics
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
		return &proxy.Event{
			Params: make(map[string]interface{}, 8),
		}
	},
}

// GetEvent acquires an event from the pool
func GetEvent() *proxy.Event {
	if err := assert.Check(eventPool.New != nil, "eventPool.New must be defined"); err != nil {
		return &proxy.Event{}
	}
	e := eventPool.Get().(*proxy.Event)
	// If it wasn't a new creation (miss), it's a hit
	// This is a bit tricky with sync.Pool, but we can approximate miss count in New()
	// and calculate hits as TotalRequest - Misses if we track requests.
	// For simplicity, we'll increment hit here if we didn't just increment miss in New.
	// However, New() is only called on miss. So we can just track misses in New().
	atomic.AddUint64(&globalMetrics.EventHits, 1)
	return e
}

// PutEvent returns an event to the pool after resetting it
func PutEvent(e *proxy.Event) {
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
	for k := range e.Params {
		delete(e.Params, k)
	}
	for k := range e.Response {
		delete(e.Response, k)
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

// GetBuffer acquires a buffer from the pool
func GetBuffer() *bytes.Buffer {
	if err := assert.Check(bufferPool.New != nil, "bufferPool.New must be defined"); err != nil {
		return bytes.NewBuffer(nil)
	}
	atomic.AddUint64(&globalMetrics.BufferHits, 1)
	return bufferPool.Get().(*bytes.Buffer)
}

// PutBuffer returns a buffer to the pool after resetting it
func PutBuffer(b *bytes.Buffer) {
	if b == nil {
		return
	}
	// If the buffer grew too large, don't return it to the pool to avoid memory bloat
	if b.Cap() > maxBufferSize {
		return
	}
	if err := assert.Check(b.Cap() <= maxBufferSize*2, "buffer grew dangerously large", "cap", b.Cap()); err != nil {
		return
	}
	b.Reset()
	bufferPool.Put(b)
}
