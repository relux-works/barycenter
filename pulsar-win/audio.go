// Portable audio plumbing: the f32le byte stream -> ring pump.
// The Windows-only legs (named pipe listener, WASAPI render) live in
// audio_windows.go; audio_stub.go keeps non-Windows builds compiling.
package main

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"time"
)

// The pipeline is fixed at 44100 Hz interleaved stereo f32le (spec 6.3).
const (
	sampleRate = 44100
	channels   = 2
)

var errPumpStopped = errors.New("pump stopped")

// pumpF32LE reads little-endian float32 samples from r into the ring with
// backpressure: when the ring is full the pump stalls (never drops), which
// backs the daemon up on the kernel pipe buffer — the same tempo mechanism
// as the macOS FIFO reader (spike rule: block on full ring, drop forbidden).
//
// Returns nil on EOF (writer closed the pipe), errPumpStopped when stop
// closed mid-write, or the read error.
func pumpF32LE(r io.Reader, ring *Ring, stop <-chan struct{}) error {
	buf := make([]byte, 16384)
	scratch := make([]float32, len(buf)/4)
	leftover := 0

	for {
		select {
		case <-stop:
			return errPumpStopped
		default:
		}

		n, err := r.Read(buf[leftover:])
		total := leftover + n
		usable := total - total%4

		floats := scratch[:usable/4]
		for i := range floats {
			floats[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
		}
		if werr := writeAll(ring, floats, stop); werr != nil {
			return werr
		}

		leftover = total - usable
		if leftover > 0 {
			copy(buf, buf[usable:total])
		}

		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// writeAll pushes every float into the ring, stalling while it is full.
func writeAll(ring *Ring, p []float32, stop <-chan struct{}) error {
	for len(p) > 0 {
		n := ring.Write(p)
		p = p[n:]
		if n == 0 {
			select {
			case <-stop:
				return errPumpStopped
			default:
				time.Sleep(200 * time.Microsecond) // backpressure stall, never drop
			}
		}
	}
	return nil
}
