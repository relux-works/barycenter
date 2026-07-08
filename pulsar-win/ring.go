// SPSC lock-free ring buffer for interleaved float32 PCM — a port of the
// macOS node's RingBuffer.swift (spec 6.3), same contract:
//   - Exactly one producer (pipe reader) and one consumer (WASAPI render loop).
//   - Write never drops: it returns how many floats fit; the producer loop
//     holds the remainder and retries — that stall is the backpressure that
//     ultimately blocks go-librespot on the kernel pipe buffer.
//   - Read never blocks: it returns what is available; the caller zero-fills
//     the shortfall (underrun = silence).
//   - Indices are monotonically increasing int64 totals; head is written only
//     by the producer, tail only by the consumer. sync/atomic load/store pairs
//     give the acquire/release ordering the Swift CAtomics shim provided.
package main

import "sync/atomic"

type Ring struct {
	capacity int64
	buf      []float32
	head     atomic.Int64 // total floats written (producer-owned)
	tail     atomic.Int64 // total floats read (consumer-owned)
	// clearThrough is a head snapshot the consumer discards up to on its next
	// Read (M7). Clear() is called from ws/timer goroutines while both the
	// pipe pump and the render loop keep running — a third-party tail.Store
	// raced Read's own tail.Store and could rewind tail below a concurrent
	// reader's position, replaying a chunk of the PREVIOUS track after a
	// load/seek. Only the consumer may move tail; Clear just posts the cut.
	clearThrough atomic.Int64
}

func NewRing(capacityFloats int) *Ring {
	if capacityFloats <= 0 {
		panic("ring capacity must be positive")
	}
	return &Ring{
		capacity: int64(capacityFloats),
		buf:      make([]float32, capacityFloats),
	}
}

// Capacity in floats.
func (r *Ring) Capacity() int { return int(r.capacity) }

// Fill is the number of floats currently readable. Safe from any goroutine
// (approximate between operations). A pending Clear counts as already
// applied — the next Read will discard that span anyway.
func (r *Ring) Fill() int {
	h := r.head.Load()
	t := r.tail.Load()
	if ct := r.clearThrough.Load(); ct > t {
		t = ct
	}
	if h < t {
		return 0
	}
	return int(h - t)
}

// FillMS converts the current fill to milliseconds of audio.
func (r *Ring) FillMS(sampleRate, channels int) int64 {
	return int64(r.Fill()) * 1000 / int64(sampleRate*channels)
}

// Write copies as much of p as fits and returns the count copied.
// Producer side only.
func (r *Ring) Write(p []float32) int {
	h := r.head.Load()
	t := r.tail.Load()
	free := r.capacity - (h - t)
	n := int64(len(p))
	if n > free {
		n = free
	}
	if n <= 0 {
		return 0
	}
	idx := h % r.capacity
	first := r.capacity - idx
	if first > n {
		first = n
	}
	copy(r.buf[idx:idx+first], p[:first])
	if first < n {
		copy(r.buf[:n-first], p[first:n])
	}
	r.head.Store(h + n) // release: buffer writes above happen-before this store
	return int(n)
}

// Read copies up to len(out) floats and returns the count copied.
// Consumer side only (render callback: no locks, no allocation).
func (r *Ring) Read(out []float32) int {
	h := r.head.Load()
	t := r.tail.Load()
	// Apply a posted Clear (M7): jump past everything buffered before the
	// cut. ct may exceed our stale h snapshot (a Clear landed between the two
	// loads) — available then goes non-positive and we simply return 0.
	if ct := r.clearThrough.Load(); ct > t {
		t = ct
		r.tail.Store(t)
	}
	available := h - t
	n := int64(len(out))
	if n > available {
		n = available
	}
	if n <= 0 {
		return 0
	}
	idx := t % r.capacity
	first := r.capacity - idx
	if first > n {
		first = n
	}
	copy(out[:first], r.buf[idx:idx+first])
	if first < n {
		copy(out[first:n], r.buf[:n-first])
	}
	r.tail.Store(t + n)
	return int(n)
}

// Clear schedules everything buffered SO FAR to be dropped (load of a new
// element, spec 6.3). Safe from any goroutine (M7): the consumer applies the
// cut on its next Read, samples written after this call are kept. The CAS
// loop only ever raises the watermark — two racing Clears keep the later cut.
func (r *Ring) Clear() {
	h := r.head.Load()
	for {
		cur := r.clearThrough.Load()
		if cur >= h || r.clearThrough.CompareAndSwap(cur, h) {
			return
		}
	}
}
