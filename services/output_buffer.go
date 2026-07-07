package services

import (
	"bytes"
	"sync"
	"time"
)

// outputBufferMaxBytes is the default cap on the buffered output. When
// exceeded, the oldest data is dropped to make room for new data. This
// bounds memory use when the frontend isn't draining the buffer quickly
// enough (e.g. a long `npm install` running in a backgrounded terminal).
//
// 1 MiB is large enough to keep recent scrollback visible after a Read,
// but small enough that a runaway process can't exhaust the heap.
const outputBufferMaxBytes = 1 << 20

type outputBuffer struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	notify   chan struct{}
	maxBytes int
}

func newOutputBuffer() *outputBuffer {
	return &outputBuffer{
		notify:   make(chan struct{}, 1),
		maxBytes: outputBufferMaxBytes,
	}
}

// Append writes data to the buffer. If the buffer would exceed maxBytes,
// the oldest bytes are dropped first so that the most recent output is
// always retained (N-66). Dropping oldest matches terminal semantics:
// users care about what just happened, not what scrolled off the top.
func (o *outputBuffer) Append(data []byte) {
	o.mu.Lock()
	o.buf.Write(data)
	// N-66: enforce the cap. Trimming after write keeps the code simple
	// and avoids underflow when the incoming chunk itself exceeds the cap
	// (in which case we keep only the tail of the new data).
	if max := o.maxBytes; max > 0 && o.buf.Len() > max {
		excess := o.buf.Len() - max
		// Next(n) reads and discards the first n bytes from the buffer.
		_ = o.buf.Next(excess)
	}
	o.mu.Unlock()
	select {
	case o.notify <- struct{}{}:
	default:
	}
}

func (o *outputBuffer) Read(timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for {
		o.mu.Lock()
		hasData := o.buf.Len() > 0
		o.mu.Unlock()
		if hasData || !time.Now().Before(deadline) {
			break
		}
		select {
		case <-o.notify:
		case <-time.After(time.Until(deadline)):
		}
	}
	end := time.Now().Add(50 * time.Millisecond)
	if end.After(deadline) {
		end = deadline
	}
	for time.Now().Before(end) {
		select {
		case <-o.notify:
		case <-time.After(time.Until(end)):
		}
	}
	o.mu.Lock()
	s := o.buf.String()
	o.buf.Reset()
	o.mu.Unlock()
	return s
}
