// Command generate-recording-cue produces the canonical Pulsar recording cue.
//
// The waveform is deliberately synthesized from this source without samples,
// instruments, voice, model output, or third-party audio. Re-running it with
// the same Go math implementation produces the reviewed PCM payload.
package main

import (
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

const (
	sampleRate = 48_000
	frames     = 7_680 // 160 ms
	channels   = 1
	bits       = 16
)

func main() {
	output := flag.String("output", "assets/audio/pulsar-recording-cue.wav", "output WAV path")
	flag.Parse()

	payload := make([]byte, 44+frames*channels*(bits/8))
	writeHeader(payload)
	for frame := 0; frame < frames; frame++ {
		t := float64(frame) / sampleRate
		envelope := raisedCosineEnvelope(frame)
		// A quiet two-partial bell: deterministic, short, and recognizable
		// without resembling a sampled or authored third-party sound.
		wave := 0.18*math.Sin(2*math.Pi*880*t) +
			0.07*math.Sin(2*math.Pi*1320*t+math.Pi/7)
		sample := int16(math.Round(32767 * envelope * wave))
		binary.LittleEndian.PutUint16(payload[44+frame*2:], uint16(sample))
	}

	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(*output, payload, 0o644); err != nil {
		panic(err)
	}
	digest := sha256.Sum256(payload)
	fmt.Printf("%x  %s\n", digest, *output)
}

func raisedCosineEnvelope(frame int) float64 {
	const attackFrames = 480  // 10 ms
	const releaseFrames = 960 // 20 ms
	switch {
	case frame < attackFrames:
		x := float64(frame) / float64(attackFrames)
		return 0.5 - 0.5*math.Cos(math.Pi*x)
	case frame >= frames-releaseFrames:
		x := float64(frames-1-frame) / float64(releaseFrames)
		return 0.5 - 0.5*math.Cos(math.Pi*math.Max(0, x))
	default:
		return 1
	}
}

func writeHeader(dst []byte) {
	copy(dst[0:4], "RIFF")
	binary.LittleEndian.PutUint32(dst[4:8], uint32(len(dst)-8))
	copy(dst[8:12], "WAVE")
	copy(dst[12:16], "fmt ")
	binary.LittleEndian.PutUint32(dst[16:20], 16)
	binary.LittleEndian.PutUint16(dst[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(dst[22:24], channels)
	binary.LittleEndian.PutUint32(dst[24:28], sampleRate)
	byteRate := sampleRate * channels * (bits / 8)
	binary.LittleEndian.PutUint32(dst[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(dst[32:34], channels*(bits/8))
	binary.LittleEndian.PutUint16(dst[34:36], bits)
	copy(dst[36:40], "data")
	binary.LittleEndian.PutUint32(dst[40:44], uint32(len(dst)-44))
}
