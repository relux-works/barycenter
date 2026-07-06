// Direct port of the macOS RingBufferTests.swift — same contract, same cases,
// including the SPSC concurrent-integrity test.
package main

import (
	"testing"
	"time"
)

func TestRingWriteReadRoundTrip(t *testing.T) {
	ring := NewRing(16)
	input := []float32{1, 2, 3, 4, 5, 6}
	if n := ring.Write(input); n != 6 {
		t.Fatalf("write %d, want 6", n)
	}
	if ring.Fill() != 6 {
		t.Fatalf("fill %d, want 6", ring.Fill())
	}
	out := make([]float32, 6)
	if n := ring.Read(out); n != 6 {
		t.Fatalf("read %d, want 6", n)
	}
	for i := range input {
		if out[i] != input[i] {
			t.Fatalf("out[%d] = %v, want %v", i, out[i], input[i])
		}
	}
	if ring.Fill() != 0 {
		t.Fatalf("fill %d after drain, want 0", ring.Fill())
	}
}

func TestRingFullRefusesWithoutDropping(t *testing.T) {
	ring := NewRing(8)
	chunk := make([]float32, 8)
	for i := range chunk {
		chunk[i] = 7
	}
	if n := ring.Write(chunk); n != 8 {
		t.Fatalf("first write %d, want 8", n)
	}
	if n := ring.Write([]float32{9, 9}); n != 0 {
		t.Fatalf("full ring must refuse, not overwrite (spec 6.3: drop forbidden); wrote %d", n)
	}
	out := make([]float32, 8)
	ring.Read(out)
	for i, v := range out {
		if v != 7 {
			t.Fatalf("backpressure must preserve the oldest audio; out[%d] = %v", i, v)
		}
	}
}

func TestRingPartialWriteReportsCount(t *testing.T) {
	ring := NewRing(8)
	six := make([]float32, 6)
	for i := range six {
		six[i] = 1
	}
	ring.Write(six)
	more := make([]float32, 6)
	for i := range more {
		more[i] = 2
	}
	if n := ring.Write(more); n != 2 {
		t.Fatalf("only the free space is written; got %d, want 2", n)
	}
	if ring.Fill() != 8 {
		t.Fatalf("fill %d, want 8", ring.Fill())
	}
}

func TestRingEmptyReadReturnsZero(t *testing.T) {
	ring := NewRing(8)
	out := make([]float32, 4)
	if n := ring.Read(out); n != 0 {
		t.Fatalf("underrun: caller zero-fills, ring must not fabricate data; got %d", n)
	}
}

func TestRingWrapAroundKeepsOrder(t *testing.T) {
	ring := NewRing(8)
	ring.Write([]float32{1, 2, 3, 4, 5, 6})
	out4 := make([]float32, 4)
	ring.Read(out4)

	if n := ring.Write([]float32{7, 8, 9, 10}); n != 4 { // crosses the physical end
		t.Fatalf("wrap write %d, want 4", n)
	}
	out6 := make([]float32, 6)
	if n := ring.Read(out6); n != 6 {
		t.Fatalf("read %d, want 6", n)
	}
	want := []float32{5, 6, 7, 8, 9, 10}
	for i := range want {
		if out6[i] != want[i] {
			t.Fatalf("out6 = %v, want %v", out6, want)
		}
	}
}

func TestRingClearEmpties(t *testing.T) {
	ring := NewRing(8)
	ring.Write([]float32{1, 2, 3})
	ring.Clear()
	if ring.Fill() != 0 {
		t.Fatalf("fill %d after clear, want 0", ring.Fill())
	}
}

func TestRingFillMS(t *testing.T) {
	ring := NewRing(sampleRate * channels) // 1 s capacity
	buf := make([]float32, sampleRate*channels/10)
	ring.Write(buf) // 100 ms
	if ms := ring.FillMS(sampleRate, channels); ms != 100 {
		t.Fatalf("FillMS = %d, want 100", ms)
	}
}

// SPSC integrity under real concurrency: a monotonic sequence pushed with
// backpressure retries must come out complete and ordered.
func TestRingConcurrentProducerConsumerIntegrity(t *testing.T) {
	ring := NewRing(1024)
	const total = 200_000

	go func() {
		value := float32(0)
		chunk := make([]float32, 300)
		sent := 0
		for sent < total {
			n := 300
			if total-sent < n {
				n = total - sent
			}
			for i := 0; i < n; i++ {
				chunk[i] = value + float32(i)
			}
			offset := 0
			for offset < n {
				written := ring.Write(chunk[offset:n])
				if written == 0 {
					time.Sleep(200 * time.Microsecond) // backpressure stall, never drop
				}
				offset += written
			}
			value += float32(n)
			sent += n
		}
	}()

	received := 0
	expected := float32(0)
	out := make([]float32, 257) // deliberately co-prime-ish chunk
	corrupt := false
	deadline := time.Now().Add(30 * time.Second)
	for received < total {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %d/%d floats", received, total)
		}
		n := ring.Read(out)
		if n == 0 {
			time.Sleep(100 * time.Microsecond)
			continue
		}
		for i := 0; i < n; i++ {
			if out[i] != expected+float32(i) {
				corrupt = true
			}
		}
		expected += float32(n)
		received += n
	}
	if corrupt {
		t.Fatal("sequence corrupted in SPSC transfer")
	}
	if received != total {
		t.Fatalf("received %d, want %d", received, total)
	}
}
