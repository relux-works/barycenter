// Voice cache (LRU by size, Bearer downloads) and the WAV decoder.
package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
)

// --- WAV builders ---

func wavHeader(buf *bytes.Buffer, format uint16, chans, rate, bits, dataLen int) {
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, uint32(36+dataLen))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(buf, binary.LittleEndian, uint32(16))
	binary.Write(buf, binary.LittleEndian, format)
	binary.Write(buf, binary.LittleEndian, uint16(chans))
	binary.Write(buf, binary.LittleEndian, uint32(rate))
	binary.Write(buf, binary.LittleEndian, uint32(rate*chans*bits/8))
	binary.Write(buf, binary.LittleEndian, uint16(chans*bits/8))
	binary.Write(buf, binary.LittleEndian, uint16(bits))
	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, uint32(dataLen))
}

func makeWAV16(chans, rate int, samples []int16) []byte {
	var buf bytes.Buffer
	wavHeader(&buf, 1, chans, rate, 16, len(samples)*2)
	binary.Write(&buf, binary.LittleEndian, samples)
	return buf.Bytes()
}

func makeWAVf32(chans, rate int, samples []float32) []byte {
	var buf bytes.Buffer
	wavHeader(&buf, 3, chans, rate, 32, len(samples)*4)
	binary.Write(&buf, binary.LittleEndian, samples)
	return buf.Bytes()
}

func TestParseWAV16BitPCM(t *testing.T) {
	raw := makeWAV16(2, 44100, []int16{0, 16384, -16384, 32767})
	w, err := parseWAV(raw)
	if err != nil {
		t.Fatal(err)
	}
	if w.sampleRate != 44100 || w.channels != 2 || len(w.samples) != 4 {
		t.Fatalf("fmt parsed wrong: %+v", w)
	}
	want := []float32{0, 0.5, -0.5, 32767.0 / 32768}
	for i := range want {
		if math.Abs(float64(w.samples[i]-want[i])) > 1e-4 {
			t.Fatalf("sample %d = %v, want %v", i, w.samples[i], want[i])
		}
	}
}

func TestParseWAVFloat32(t *testing.T) {
	src := []float32{0.25, -0.75, 1, -1}
	w, err := parseWAV(makeWAVf32(1, 22050, src))
	if err != nil {
		t.Fatal(err)
	}
	if w.sampleRate != 22050 || w.channels != 1 {
		t.Fatalf("fmt parsed wrong: %+v", w)
	}
	if !reflect.DeepEqual(w.samples, src) {
		t.Fatalf("samples %v, want %v", w.samples, src)
	}
}

func TestParseWAVExtensible(t *testing.T) {
	// WAVE_FORMAT_EXTENSIBLE wrapping of plain PCM16: the real format tag is
	// the first two bytes of the SubFormat GUID at offset 24 of the chunk.
	var buf bytes.Buffer
	samples := []int16{100, -100}
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(60+len(samples)*2))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(40))
	binary.Write(&buf, binary.LittleEndian, uint16(0xFFFE))
	binary.Write(&buf, binary.LittleEndian, uint16(2))
	binary.Write(&buf, binary.LittleEndian, uint32(48000))
	binary.Write(&buf, binary.LittleEndian, uint32(48000*4))
	binary.Write(&buf, binary.LittleEndian, uint16(4))
	binary.Write(&buf, binary.LittleEndian, uint16(16))
	binary.Write(&buf, binary.LittleEndian, uint16(22)) // cbSize
	binary.Write(&buf, binary.LittleEndian, uint16(16)) // valid bits
	binary.Write(&buf, binary.LittleEndian, uint32(3))  // channel mask
	binary.Write(&buf, binary.LittleEndian, uint16(1))  // SubFormat: PCM
	buf.Write(make([]byte, 14))                         // rest of the GUID
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(len(samples)*2))
	binary.Write(&buf, binary.LittleEndian, samples)

	w, err := parseWAV(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if w.channels != 2 || w.sampleRate != 48000 || len(w.samples) != 2 {
		t.Fatalf("extensible parse wrong: %+v", w)
	}
}

func TestParseWAVRejectsGarbage(t *testing.T) {
	if _, err := parseWAV([]byte("definitely not a wav file")); err == nil {
		t.Fatal("garbage accepted as WAV")
	}
	// Unsupported encoding: 8-bit PCM.
	var buf bytes.Buffer
	wavHeader(&buf, 1, 1, 44100, 8, 2)
	buf.Write([]byte{1, 2})
	if _, err := parseWAV(buf.Bytes()); err == nil {
		t.Fatal("8-bit PCM must be rejected (only 16-bit and float32)")
	}
}

func TestToEngineFormatMonoUpmixResample(t *testing.T) {
	// Mono 22050 Hz constant DC: expect stereo 44100 with ~2x frames, L==R.
	mono := make([]float32, 2205) // 100 ms
	for i := range mono {
		mono[i] = 0.5
	}
	out := toEngineFormat(wavData{sampleRate: 22050, channels: 1, samples: mono})
	frames := len(out) / channels
	if frames != 4410 {
		t.Fatalf("resampled frames %d, want 4410 (100 ms at 44.1k)", frames)
	}
	for f := 0; f < frames; f++ {
		if out[f*2] != 0.5 || out[f*2+1] != 0.5 {
			t.Fatalf("frame %d = %v/%v, want 0.5 on both channels", f, out[f*2], out[f*2+1])
		}
	}

	// Native-rate stereo passes through untouched.
	src := []float32{0.1, 0.2, 0.3, 0.4}
	out = toEngineFormat(wavData{sampleRate: 44100, channels: 2, samples: src})
	if !reflect.DeepEqual(out, src) {
		t.Fatalf("native passthrough %v, want %v", out, src)
	}
}

// --- cache ---

func TestVoiceCacheDownloadsWithBearerAndHits(t *testing.T) {
	var hits atomic.Int64
	wav := makeWAV16(2, 44100, []int16{1, 2, 3, 4})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		hits.Add(1)
		w.Write(wav)
	}))
	defer ts.Close()

	c, err := NewVoiceCache(t.TempDir(), "tok123", 0, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	path1, err := c.Fetch(context.Background(), ts.URL+"/media/m_abc.wav")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path1)
	if !bytes.Equal(raw, wav) {
		t.Fatal("cached file differs from the served body")
	}

	// Second fetch is a cache hit: no new download.
	if _, err := c.Fetch(context.Background(), ts.URL+"/media/m_abc.wav"); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("server hit %d times, want 1 (second fetch must hit the cache)", hits.Load())
	}

	// Wrong token surfaces as an HTTP error.
	cBad, _ := NewVoiceCache(t.TempDir(), "wrong", 0, testLogger())
	if _, err := cBad.Fetch(context.Background(), ts.URL+"/media/m_abc.wav"); err == nil {
		t.Fatal("expected an error for a rejected token")
	}
}

func TestVoiceCacheEvictsLRUBySize(t *testing.T) {
	bodies := map[string][]byte{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bodies[filepath.Base(r.URL.Path)])
	}))
	defer ts.Close()

	c, err := NewVoiceCache(t.TempDir(), "tok", 100, testLogger()) // 100-byte cap
	if err != nil {
		t.Fatal(err)
	}

	fetch := func(id string, size int) {
		t.Helper()
		bodies[id+".wav"] = bytes.Repeat([]byte{7}, size)
		if _, err := c.Fetch(context.Background(), fmt.Sprintf("%s/media/%s.wav", ts.URL, id)); err != nil {
			t.Fatal(err)
		}
	}

	fetch("a", 60)
	fetch("b", 30)
	if got := c.CachedIDs(); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("order %v, want [a b]", got)
	}

	// Touch a so b becomes the LRU victim.
	if _, err := c.Fetch(context.Background(), ts.URL+"/media/a.wav"); err != nil {
		t.Fatal(err)
	}
	fetch("c", 60) // 60+30+60 > 100: evict b (LRU), then keep [a? no: a(60)+c(60)>100 -> evict a too? see below]

	// After evicting b: a(60)+c(60)=120 still > 100 -> a evicted as well.
	if got := c.CachedIDs(); !reflect.DeepEqual(got, []string{"c"}) {
		t.Fatalf("order after eviction %v, want [c]", got)
	}
	if got := c.TotalBytes(); got != 60 {
		t.Fatalf("total %d, want 60", got)
	}
	if _, err := os.Stat(filepath.Join(c.dir, "b.wav")); !os.IsNotExist(err) {
		t.Fatal("evicted file b.wav still on disk")
	}

	// The newest entry survives even when it alone exceeds the cap.
	fetch("huge", 500)
	if got := c.CachedIDs(); !reflect.DeepEqual(got, []string{"huge"}) {
		t.Fatalf("order %v, want [huge] (newest never evicted)", got)
	}
}

func TestVoiceCacheRestoresIndexAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	c1, err := NewVoiceCache(dir, "tok", 1000, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(c1.dir, "old.wav"), make([]byte, 42), 0o644); err != nil {
		t.Fatal(err)
	}

	c2, err := NewVoiceCache(dir, "tok", 1000, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	if got := c2.TotalBytes(); got != 42 {
		t.Fatalf("restarted cache indexed %d bytes, want 42", got)
	}
	if got := c2.CachedIDs(); !reflect.DeepEqual(got, []string{"old"}) {
		t.Fatalf("restarted cache ids %v, want [old]", got)
	}
}

func TestMediaID(t *testing.T) {
	cases := map[string]string{
		"https://host/media/m_123.wav": "m_123",
		"https://host/media/m_123":     "m_123",
		"m_9.wav":                      "m_9",
	}
	for in, want := range cases {
		if got := MediaID(in); got != want {
			t.Fatalf("MediaID(%q) = %q, want %q", in, got, want)
		}
	}
}
