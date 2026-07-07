// Voice inserts (spec 6.3 play_voice): a size-bounded LRU media cache with
// Bearer-authed downloads (port of VoiceCache.swift), plus a tiny WAV parser
// (16-bit PCM and 32-bit float, no external deps) feeding the engine at
// 44.1 kHz interleaved stereo.
package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// defaultVoiceCacheBytes bounds the on-disk voice cache: 2 GiB, evicting the
// least recently used files first.
const defaultVoiceCacheBytes int64 = 2 << 30

type VoiceCache struct {
	dir      string
	token    string
	capacity int64
	httpc    *http.Client
	log      *slog.Logger

	mu    sync.Mutex
	order []string // media ids, most recently used last
	sizes map[string]int64
}

// NewVoiceCache roots the cache at <cacheDir>/voice and indexes any files
// already there (oldest-modified = least recently used), so evictions stay
// size-correct across restarts.
func NewVoiceCache(cacheDir, token string, capacityBytes int64, log *slog.Logger) (*VoiceCache, error) {
	if capacityBytes <= 0 {
		capacityBytes = defaultVoiceCacheBytes
	}
	c := &VoiceCache{
		dir:      filepath.Join(cacheDir, "voice"),
		token:    token,
		capacity: capacityBytes,
		httpc:    &http.Client{Timeout: 60 * time.Second},
		log:      log,
		sizes:    map[string]int64{},
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return nil, fmt.Errorf("create voice cache dir: %w", err)
	}

	type entry struct {
		id   string
		size int64
		mod  time.Time
	}
	var existing []entry
	items, err := os.ReadDir(c.dir)
	if err != nil {
		return nil, fmt.Errorf("scan voice cache dir: %w", err)
	}
	for _, it := range items {
		if it.IsDir() || !strings.HasSuffix(it.Name(), ".wav") {
			continue
		}
		info, err := it.Info()
		if err != nil {
			continue
		}
		existing = append(existing, entry{
			id:   strings.TrimSuffix(it.Name(), ".wav"),
			size: info.Size(),
			mod:  info.ModTime(),
		})
	}
	sort.Slice(existing, func(i, j int) bool { return existing[i].mod.Before(existing[j].mod) })
	for _, e := range existing {
		c.order = append(c.order, e.id)
		c.sizes[e.id] = e.size
	}
	return c, nil
}

// MediaID extracts the cache key from the coordinator media URL: the last
// path segment without the .wav suffix (mirror of VoiceCache.mediaID).
func MediaID(fileURL string) string {
	parts := strings.Split(fileURL, "/")
	last := fileURL
	if len(parts) > 0 {
		last = parts[len(parts)-1]
	}
	return strings.TrimSuffix(last, ".wav")
}

// Fetch returns the local path of the media file, downloading it with the
// node token when it is not cached yet.
func (c *VoiceCache) Fetch(ctx context.Context, fileURL string) (string, error) {
	id := MediaID(fileURL)
	local := filepath.Join(c.dir, id+".wav")

	c.mu.Lock()
	_, cached := c.sizes[id]
	c.mu.Unlock()
	if cached {
		if _, err := os.Stat(local); err == nil {
			c.touch(id)
			return local, nil
		}
		// Index is stale (file removed externally): fall through to download.
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return "", fmt.Errorf("bad media url %s: %w", fileURL, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		return "", fmt.Errorf("media download HTTP %d", resp.StatusCode)
	}

	tmp := local + ".part"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", err
	}
	size, err := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, local); err != nil {
		os.Remove(tmp)
		return "", err
	}

	c.mu.Lock()
	c.sizes[id] = size
	c.orderTouchLocked(id)
	c.evictLocked()
	c.mu.Unlock()
	return local, nil
}

func (c *VoiceCache) touch(id string) {
	c.mu.Lock()
	c.orderTouchLocked(id)
	c.mu.Unlock()
}

func (c *VoiceCache) orderTouchLocked(id string) {
	for i, v := range c.order {
		if v == id {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
	c.order = append(c.order, id)
}

// evictLocked drops least-recently-used files until the total fits the
// capacity. The newest entry (just fetched) is never evicted.
func (c *VoiceCache) evictLocked() {
	total := int64(0)
	for _, s := range c.sizes {
		total += s
	}
	for total > c.capacity && len(c.order) > 1 {
		victim := c.order[0]
		c.order = c.order[1:]
		total -= c.sizes[victim]
		delete(c.sizes, victim)
		os.Remove(filepath.Join(c.dir, victim+".wav"))
		c.log.Debug("voice cache evicted", "media_id", victim)
	}
}

// CachedIDs is the current LRU order, least recently used first (test hook).
func (c *VoiceCache) CachedIDs() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.order...)
}

// TotalBytes is the indexed cache size (test hook).
func (c *VoiceCache) TotalBytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := int64(0)
	for _, s := range c.sizes {
		total += s
	}
	return total
}

// --- WAV decoding (no deps: RIFF walk, PCM16 / float32, any rate/channels) ---

type wavData struct {
	sampleRate int
	channels   int
	samples    []float32 // interleaved, one float per channel-sample
}

var errNotWAV = errors.New("not a RIFF/WAVE file")

// parseWAV walks the RIFF chunks and decodes the data chunk. Supported
// encodings: PCM 16-bit (format 1) and IEEE float 32-bit (format 3),
// including their WAVE_FORMAT_EXTENSIBLE (0xFFFE) wrappings — the two
// shapes the coordinator's TTS pipeline produces.
func parseWAV(raw []byte) (wavData, error) {
	if len(raw) < 12 || string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		return wavData{}, errNotWAV
	}

	var (
		haveFmt       bool
		audioFormat   uint16
		numChannels   int
		rate          int
		bitsPerSample int
		data          []byte
	)

	off := 12
	for off+8 <= len(raw) {
		id := string(raw[off : off+4])
		size := int(binary.LittleEndian.Uint32(raw[off+4 : off+8]))
		body := off + 8
		if size < 0 || body+size > len(raw) {
			// Tolerate a truncated final chunk (some writers pad sloppily).
			size = len(raw) - body
			if size < 0 {
				break
			}
		}
		switch id {
		case "fmt ":
			if size < 16 {
				return wavData{}, fmt.Errorf("wav: fmt chunk too short (%d bytes)", size)
			}
			audioFormat = binary.LittleEndian.Uint16(raw[body:])
			numChannels = int(binary.LittleEndian.Uint16(raw[body+2:]))
			rate = int(binary.LittleEndian.Uint32(raw[body+4:]))
			bitsPerSample = int(binary.LittleEndian.Uint16(raw[body+14:]))
			if audioFormat == 0xFFFE && size >= 40 {
				// WAVE_FORMAT_EXTENSIBLE: the real format is the first two
				// bytes of the SubFormat GUID at offset 24.
				audioFormat = binary.LittleEndian.Uint16(raw[body+24:])
			}
			haveFmt = true
		case "data":
			data = raw[body : body+size]
		}
		off = body + size
		if size%2 == 1 {
			off++ // RIFF chunks are word-aligned
		}
	}

	if !haveFmt || data == nil {
		return wavData{}, errors.New("wav: missing fmt or data chunk")
	}
	if numChannels < 1 || rate <= 0 {
		return wavData{}, fmt.Errorf("wav: implausible fmt (channels=%d rate=%d)", numChannels, rate)
	}

	var samples []float32
	switch {
	case audioFormat == 1 && bitsPerSample == 16:
		n := len(data) / 2
		samples = make([]float32, n)
		for i := 0; i < n; i++ {
			samples[i] = float32(int16(binary.LittleEndian.Uint16(data[i*2:]))) / 32768
		}
	case audioFormat == 3 && bitsPerSample == 32:
		n := len(data) / 4
		samples = make([]float32, n)
		for i := 0; i < n; i++ {
			samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
		}
	default:
		return wavData{}, fmt.Errorf("wav: unsupported encoding (format=%d bits=%d); need PCM16 or float32", audioFormat, bitsPerSample)
	}

	// Drop a trailing partial frame instead of failing.
	samples = samples[:len(samples)/numChannels*numChannels]
	return wavData{sampleRate: rate, channels: numChannels, samples: samples}, nil
}

// toEngineFormat converts decoded WAV audio to the pipeline format
// (44.1 kHz interleaved stereo): mono duplicates, extra channels are
// dropped, and rate mismatches go through linear interpolation — plenty for
// speech inserts (the macOS node delegated the same job to AVAudioFile).
func toEngineFormat(w wavData) []float32 {
	frames := len(w.samples) / w.channels
	if frames == 0 {
		return nil
	}

	// Channel map first: anything -> stereo.
	stereo := make([]float32, frames*channels)
	for f := 0; f < frames; f++ {
		l := w.samples[f*w.channels]
		r := l
		if w.channels >= 2 {
			r = w.samples[f*w.channels+1]
		}
		stereo[f*channels] = l
		stereo[f*channels+1] = r
	}
	if w.sampleRate == sampleRate {
		return stereo
	}

	// Linear resample to 44.1 kHz.
	outFrames := int(int64(frames) * sampleRate / int64(w.sampleRate))
	if outFrames < 1 {
		outFrames = 1
	}
	out := make([]float32, outFrames*channels)
	step := float64(w.sampleRate) / float64(sampleRate)
	for f := 0; f < outFrames; f++ {
		pos := float64(f) * step
		i := int(pos)
		frac := float32(pos - float64(i))
		j := i + 1
		if j >= frames {
			j = frames - 1
		}
		for ch := 0; ch < channels; ch++ {
			a := stereo[i*channels+ch]
			b := stereo[j*channels+ch]
			out[f*channels+ch] = a + (b-a)*frac
		}
	}
	return out
}

// loadVoiceFile reads and decodes a cached WAV into engine-format samples.
func loadVoiceFile(path string) ([]float32, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	w, err := parseWAV(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return toEngineFormat(w), nil
}
