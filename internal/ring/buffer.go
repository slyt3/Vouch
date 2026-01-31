package ring

import (
	"errors"
	"sync"

	"github.com/slyt3/Logryph/internal/assert"
)

var ErrBufferFull = errors.New("ring buffer is full")
var ErrBufferEmpty = errors.New("ring buffer is empty")

// Buffer is a thread-safe, fixed-size ring buffer for async event submission.
// Zero-allocation on Push/Pop. Bounded loop iterations satisfy NASA Rule 2.
// Generic type T typically *models.Event.
type Buffer[T any] struct {
	data     []T
	capacity int
	head     int
	tail     int
	count    int
	mu       sync.Mutex
}

// New creates a new fixed-size ring buffer with the specified capacity.
// Returns an error if capacity <= 0. Use capacity = 1024 for typical event workloads.
func New[T any](capacity int) (*Buffer[T], error) {
	if err := assert.Check(capacity > 0, "capacity must be positive"); err != nil {
		return nil, err
	}
	return &Buffer[T]{
		data:     make([]T, capacity),
		capacity: capacity,
		head:     0,
		tail:     0,
		count:    0,
	}, nil
}

// Push adds an item to the buffer without allocation.
// Returns ErrBufferFull if capacity is reached. Caller must handle backpressure.
func (b *Buffer[T]) Push(item T) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// NASA Rule 5: Check Preconditions
	if b.count == b.capacity {
		return ErrBufferFull
	}
	// Bounds check (explicit, though go does it)
	if err := assert.InRange(b.tail, 0, b.capacity-1, "tail index"); err != nil {
		return err
	}

	b.data[b.tail] = item
	b.tail = (b.tail + 1) % b.capacity
	b.count++
	return nil
}

// Pop removes and returns the oldest item from the buffer.
// Returns ErrBufferEmpty if buffer is empty. Zero-allocation on success.
func (b *Buffer[T]) Pop() (T, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	var zero T
	if b.count == 0 {
		return zero, ErrBufferEmpty
	}

	if err := assert.InRange(b.head, 0, b.capacity-1, "head index"); err != nil {
		return zero, err
	}
	item := b.data[b.head]

	// Optional: Clear reference to help GC if T is a pointer
	// b.data[b.head] = zero

	b.head = (b.head + 1) % b.capacity
	b.count--
	return item, nil
}

// IsFull returns true if buffer is at capacity.
// Thread-safe. Use to implement backpressure handling.
func (b *Buffer[T]) IsFull() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count == b.capacity
}

// IsEmpty returns true if buffer contains no items.
// Thread-safe. Use to determine when workers should idle.
func (b *Buffer[T]) IsEmpty() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count == 0
}

// Len returns the current number of items in the buffer.
// Thread-safe. Use for queue depth metrics.
func (b *Buffer[T]) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.count
}

// Cap returns the fixed capacity of the buffer.
// Thread-safe (immutable after construction).
func (b *Buffer[T]) Cap() int {
	return b.capacity
}
