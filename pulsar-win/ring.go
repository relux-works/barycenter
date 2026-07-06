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
// (approximate between operations).
func (r *Ring) Fill() int {
	return int(r.head.Load() - r.tail.Load())
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

// Clear drops all buffered audio (load of a new element, spec 6.3).
// Caller must guarantee the producer is parked while clearing.
func (r *Ring) Clear() {
	r.tail.Store(r.head.Load())
}
