// Portable pump tests: f32le byte stream -> ring with backpressure.
package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"sync"
	"testing"
	"time"
)

func f32leBytes(values []float32) []byte {
	out := make([]byte, len(values)*4)
	for i, v := range values {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(v))
	}
	return out
}

// chunkReader yields at most n bytes per Read to exercise the partial-sample
// carry (1003 is prime relative to the 4-byte sample size).
type chunkReader struct {
	r io.Reader
	n int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if len(p) > c.n {
		p = p[:c.n]
	}
	return c.r.Read(p)
}

func TestPumpIntegrityThroughSmallRing(t *testing.T) {
	const total = 100_000
	values := make([]float32, total)
	for i := range values {
		values[i] = float32(i)
	}
	src := &chunkReader{r: bytes.NewReader(f32leBytes(values)), n: 1003}
	ring := NewRing(1024) // much smaller than the stream: forces backpressure
	stop := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	var corrupt bool
	var received int
	go func() {
		defer wg.Done()
		out := make([]float32, 257)
		expected := float32(0)
		deadline := time.Now().Add(30 * time.Second)
		for received < total && time.Now().Before(deadline) {
			n := ring.Read(out)
			if n == 0 {
				time.Sleep(50 * time.Microsecond)
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
	}()

	if err := pumpF32LE(src, ring, stop); err != nil {
		t.Fatalf("pump: %v", err)
	}
	wg.Wait()
	if corrupt {
		t.Fatal("sequence corrupted between pipe and ring")
	}
	if received != total {
		t.Fatalf("received %d floats, want %d (drop forbidden)", received, total)
	}
}

func TestPumpHandlesSingleByteReads(t *testing.T) {
	values := []float32{1, -2, 3.5, -4.25, 1e-3}
	src := &chunkReader{r: bytes.NewReader(f32leBytes(values)), n: 1}
	ring := NewRing(64)
	if err := pumpF32LE(src, ring, make(chan struct{})); err != nil {
		t.Fatal(err)
	}
	out := make([]float32, len(values))
	if n := ring.Read(out); n != len(values) {
		t.Fatalf("read %d, want %d", n, len(values))
	}
	for i := range values {
		if out[i] != values[i] {
			t.Fatalf("out[%d] = %v, want %v", i, out[i], values[i])
		}
	}
}

// endlessZeros never returns EOF — the pump must be unblockable via stop even
// with a full ring and no consumer (the daemon-stall scenario inverted).
type endlessZeros struct{}

func (endlessZeros) Read(p []byte) (int, error) { return len(p), nil }

func TestPumpStopUnblocksFullRing(t *testing.T) {
	ring := NewRing(8) // tiny: fills instantly, then the pump stalls
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- pumpF32LE(endlessZeros{}, ring, stop) }()

	time.Sleep(20 * time.Millisecond) // let it hit the backpressure stall
	close(stop)
	select {
	case err := <-done:
		if err != errPumpStopped {
			t.Fatalf("pump returned %v, want errPumpStopped", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pump never released the stall after stop")
	}
}

func TestPumpEOFDrainsCleanly(t *testing.T) {
	values := []float32{7, 8, 9}
	ring := NewRing(64)
	if err := pumpF32LE(bytes.NewReader(f32leBytes(values)), ring, make(chan struct{})); err != nil {
		t.Fatalf("EOF must return nil, got %v", err)
	}
	if ring.Fill() != 3 {
		t.Fatalf("ring fill %d, want 3", ring.Fill())
	}
}
