package ring

import (
	"testing"

	"github.com/slyt3/Vouch/internal/assert"
)

// TestNew_EdgeCases tests buffer creation with various edge case inputs
func TestNew_EdgeCases(t *testing.T) {
	// Disable strict mode and logs to test error returns cleanly
	oldStrictMode := assert.StrictMode
	oldSuppressLogs := assert.SuppressLogs
	assert.StrictMode = false
	assert.SuppressLogs = true
	defer func() {
		assert.StrictMode = oldStrictMode
		assert.SuppressLogs = oldSuppressLogs
	}()

	tests := []struct {
		name      string
		capacity  int
		wantError bool
	}{
		{"zero capacity", 0, true},
		{"negative capacity", -1, true},
		{"valid small capacity", 1, false},
		{"valid large capacity", 10000, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, err := New[int](tt.capacity)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error for capacity %d, got nil", tt.capacity)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for capacity %d: %v", tt.capacity, err)
				}
				if buf == nil {
					t.Errorf("expected non-nil buffer for capacity %d", tt.capacity)
				}
			}
		})
	}
}

// TestPushPop_EdgeCases tests push/pop with boundary conditions
func TestPushPop_EdgeCases(t *testing.T) {
	const capacity = 3
	buf, err := New[string](capacity)
	if err != nil {
		t.Fatalf("failed to create buffer: %v", err)
	}

	// Test pop from empty buffer
	_, err = buf.Pop()
	if err != ErrBufferEmpty {
		t.Errorf("expected ErrBufferEmpty, got %v", err)
	}

	// Fill buffer to capacity
	for i := 0; i < capacity; i++ {
		if err := buf.Push("item"); err != nil {
			t.Fatalf("failed to push item %d: %v", i, err)
		}
	}

	// Test push to full buffer
	err = buf.Push("overflow")
	if err != ErrBufferFull {
		t.Errorf("expected ErrBufferFull, got %v", err)
	}

	// Verify buffer is full
	if !buf.IsFull() {
		t.Error("buffer should be full")
	}
	if buf.IsEmpty() {
		t.Error("buffer should not be empty")
	}

	// Drain buffer
	for i := 0; i < capacity; i++ {
		_, err := buf.Pop()
		if err != nil {
			t.Fatalf("failed to pop item %d: %v", i, err)
		}
	}

	// Verify buffer is empty
	if buf.IsFull() {
		t.Error("buffer should not be full")
	}
	if !buf.IsEmpty() {
		t.Error("buffer should be empty")
	}

	// Test pop from empty again
	_, err = buf.Pop()
	if err != ErrBufferEmpty {
		t.Errorf("expected ErrBufferEmpty after drain, got %v", err)
	}
}

// TestPushPop_Wraparound tests ring buffer wraparound behavior
func TestPushPop_Wraparound(t *testing.T) {
	const capacity = 4
	buf, err := New[int](capacity)
	if err != nil {
		t.Fatalf("failed to create buffer: %v", err)
	}

	// Fill and drain multiple times to test wraparound
	for cycle := 0; cycle < 3; cycle++ {
		// Fill buffer
		for i := 0; i < capacity; i++ {
			value := cycle*capacity + i
			if err := buf.Push(value); err != nil {
				t.Fatalf("cycle %d: failed to push %d: %v", cycle, value, err)
			}
		}

		// Partial drain (2 items)
		for i := 0; i < 2; i++ {
			expected := cycle*capacity + i
			got, err := buf.Pop()
			if err != nil {
				t.Fatalf("cycle %d: failed to pop: %v", cycle, err)
			}
			if got != expected {
				t.Errorf("cycle %d: expected %d, got %d", cycle, expected, got)
			}
		}

		// Push 2 more items (should wrap around)
		for i := 0; i < 2; i++ {
			value := (cycle+1)*capacity + i
			if err := buf.Push(value); err != nil {
				t.Fatalf("cycle %d: failed to push wraparound %d: %v", cycle, value, err)
			}
		}

		// Drain remaining 4 items
		for i := 0; i < 4; i++ {
			_, err := buf.Pop()
			if err != nil {
				t.Fatalf("cycle %d: failed to pop remaining: %v", cycle, err)
			}
		}
	}
}

// TestLen_Consistency tests length tracking across operations
func TestLen_Consistency(t *testing.T) {
	const capacity = 5
	buf, err := New[int](capacity)
	if err != nil {
		t.Fatalf("failed to create buffer: %v", err)
	}

	if buf.Len() != 0 {
		t.Errorf("new buffer should have length 0, got %d", buf.Len())
	}

	// Push items and check length
	for i := 1; i <= capacity; i++ {
		if err := buf.Push(i * 10); err != nil {
			t.Fatalf("failed to push: %v", err)
		}
		if buf.Len() != i {
			t.Errorf("expected length %d after %d pushes, got %d", i, i, buf.Len())
		}
	}

	// Pop items and check length
	for i := capacity - 1; i >= 0; i-- {
		_, err := buf.Pop()
		if err != nil {
			t.Fatalf("failed to pop: %v", err)
		}
		if buf.Len() != i {
			t.Errorf("expected length %d after pop, got %d", i, buf.Len())
		}
	}
}

// TestCap_Immutable tests that capacity remains constant
func TestCap_Immutable(t *testing.T) {
	const capacity = 7
	buf, err := New[int](capacity)
	if err != nil {
		t.Fatalf("failed to create buffer: %v", err)
	}

	if buf.Cap() != capacity {
		t.Errorf("expected capacity %d, got %d", capacity, buf.Cap())
	}

	// Push and pop, capacity should remain constant
	for i := 0; i < capacity*2; i++ {
		_ = buf.Push(i)
		if buf.Cap() != capacity {
			t.Errorf("capacity changed to %d after push", buf.Cap())
		}

		if !buf.IsEmpty() {
			_, _ = buf.Pop()
			if buf.Cap() != capacity {
				t.Errorf("capacity changed to %d after pop", buf.Cap())
			}
		}
	}
}

// TestBuffer_NilItems tests buffer behavior with nil pointer types
func TestBuffer_NilItems(t *testing.T) {
	buf, err := New[*int](3)
	if err != nil {
		t.Fatalf("failed to create buffer: %v", err)
	}

	// Push nil values
	if err := buf.Push(nil); err != nil {
		t.Errorf("failed to push nil: %v", err)
	}

	// Pop nil value
	val, err := buf.Pop()
	if err != nil {
		t.Errorf("failed to pop: %v", err)
	}
	if val != nil {
		t.Errorf("expected nil, got %v", val)
	}
}

// Benchmark tests for performance validation

// BenchmarkPush_SingleThread measures push performance on a single goroutine
func BenchmarkPush_SingleThread(b *testing.B) {
	buf, _ := New[int](10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := buf.Push(i); err != nil {
			_, _ = buf.Pop() // Make space if full
			_ = buf.Push(i)
		}
	}
}

// BenchmarkPop_SingleThread measures pop performance on a single goroutine
func BenchmarkPop_SingleThread(b *testing.B) {
	buf, _ := New[int](10000)
	// Pre-fill buffer
	for i := 0; i < 10000; i++ {
		_ = buf.Push(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := buf.Pop(); err != nil {
			_ = buf.Push(i) // Refill if empty
			_, _ = buf.Pop()
		}
	}
}

// BenchmarkPushPop_SingleThread measures combined push/pop cycles
func BenchmarkPushPop_SingleThread(b *testing.B) {
	buf, _ := New[int](1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buf.Push(i)
		_, _ = buf.Pop()
	}
}

// BenchmarkBuffer_ZeroAllocation verifies zero-allocation design
func BenchmarkBuffer_ZeroAllocation(b *testing.B) {
	buf, _ := New[int](1024)
	// Pre-fill to avoid allocation during test
	for i := 0; i < 512; i++ {
		_ = buf.Push(i)
	}
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = buf.Push(i)
		_, _ = buf.Pop()
	}
}
