package logs

import "sync"

// DefaultBufferSize is the number of application log entries retained in
// memory. At ~200 bytes per entry this costs roughly 400 KB.
const DefaultBufferSize = 2000

// RingBuffer is a fixed-capacity, thread-safe circular buffer of log entries.
// Once full, each write overwrites the oldest entry.
type RingBuffer struct {
	mu      sync.RWMutex
	entries []Entry
	next    int  // index of the next write
	filled  bool // true once the buffer has wrapped
}

// NewRingBuffer creates a buffer holding up to size entries.
// A size of zero or less falls back to DefaultBufferSize.
func NewRingBuffer(size int) *RingBuffer {
	if size <= 0 {
		size = DefaultBufferSize
	}
	return &RingBuffer{entries: make([]Entry, size)}
}

// Append stores an entry, evicting the oldest when the buffer is full.
func (b *RingBuffer) Append(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries[b.next] = e
	b.next = (b.next + 1) % len(b.entries)
	if b.next == 0 {
		b.filled = true
	}
}

// Snapshot returns the buffered entries in chronological order.
func (b *RingBuffer) Snapshot() []Entry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if !b.filled {
		out := make([]Entry, b.next)
		copy(out, b.entries[:b.next])
		return out
	}

	out := make([]Entry, 0, len(b.entries))
	out = append(out, b.entries[b.next:]...)
	out = append(out, b.entries[:b.next]...)
	return out
}

// Len returns the number of entries currently held.
func (b *RingBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.filled {
		return len(b.entries)
	}
	return b.next
}
