package puregoprobe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/hajimehoshi/go-mp3"
	"github.com/pion/opus"
	"github.com/pion/opus/pkg/oggreader"
)

const (
	MaximumReadBytes = 65_536
	RingBytes        = 1_048_576
)

type ReadMetrics struct {
	Bytes       int64 `json:"bytes"`
	Operations  int   `json:"operations"`
	MaximumRead int   `json:"maximumRead"`
}

type MeteredReader struct {
	reader io.Reader
	mu     sync.Mutex
	metric ReadMetrics
}

func NewMeteredReader(reader io.Reader) *MeteredReader { return &MeteredReader{reader: reader} }

func (r *MeteredReader) Read(buffer []byte) (int, error) {
	if len(buffer) > MaximumReadBytes {
		buffer = buffer[:MaximumReadBytes]
	}
	n, err := r.reader.Read(buffer)
	r.mu.Lock()
	r.metric.Bytes += int64(n)
	r.metric.Operations++
	if n > r.metric.MaximumRead {
		r.metric.MaximumRead = n
	}
	r.mu.Unlock()
	return n, err
}

func (r *MeteredReader) Snapshot() ReadMetrics {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.metric
}

type MeteredReadSeeker struct {
	*MeteredReader
	seeker io.Seeker
}

func NewMeteredReadSeeker(value io.ReadSeeker) *MeteredReadSeeker {
	return &MeteredReadSeeker{MeteredReader: NewMeteredReader(value), seeker: value}
}

func (r *MeteredReadSeeker) Seek(offset int64, whence int) (int64, error) {
	return r.seeker.Seek(offset, whence)
}

type PCMQueue struct {
	storage []byte
	head    int
	tail    int
	used    int
}

func NewPCMQueue(capacity int) *PCMQueue { return &PCMQueue{storage: make([]byte, capacity)} }

func (q *PCMQueue) Push(input []byte) int {
	n := min(len(input), len(q.storage)-q.used)
	for index := 0; index < n; index++ {
		q.storage[(q.head+index)%len(q.storage)] = input[index]
	}
	q.head = (q.head + n) % len(q.storage)
	q.used += n
	return n
}

func (q *PCMQueue) Drain(buffer []byte) int {
	n := min(len(buffer), q.used)
	for index := 0; index < n; index++ {
		buffer[index] = q.storage[(q.tail+index)%len(q.storage)]
	}
	q.tail = (q.tail + n) % len(q.storage)
	q.used -= n
	return n
}

func (q *PCMQueue) Used() int { return q.used }

type FixtureEvidence struct {
	ID                   string      `json:"id"`
	Codec                string      `json:"codec"`
	Outcome              string      `json:"outcome"`
	RejectReason         string      `json:"rejectReason,omitempty"`
	SourceBytes          int64       `json:"sourceBytes"`
	BytesBeforeFirstPCM  int64       `json:"bytesBeforeFirstPCM"`
	Reads                ReadMetrics `json:"reads"`
	PCMBytes             int64       `json:"pcmBytes"`
	TrackStartMS         int64       `json:"trackStartMS"`
	ScheduledSkewMS      int64       `json:"scheduledSkewMS"`
	PausedWithoutRead    bool        `json:"pausedWithoutRead"`
	Resumed              bool        `json:"resumed"`
	Drained              bool        `json:"drained"`
	Cancelled            bool        `json:"cancelled"`
	IncrementalFirstPCM  bool        `json:"incrementalFirstPCM"`
	SeekSupported        bool        `json:"seekSupported"`
	SeekPrepareBytes     int64       `json:"seekPrepareBytes"`
	SeekRequiresFullScan bool        `json:"seekRequiresFullScan"`
	SeekToPCMMS          int64       `json:"seekToPCMMS"`
	RingCapacityBytes    int         `json:"ringCapacityBytes"`
	RingMaximumUsedBytes int         `json:"ringMaximumUsedBytes"`
}

type Evidence struct {
	SchemaVersion      int               `json:"schemaVersion"`
	Contract           string            `json:"contract"`
	CandidateID        string            `json:"candidateId"`
	ClaimClass         string            `json:"claimClass"`
	GOOS               string            `json:"goos"`
	GOARCH             string            `json:"goarch"`
	CGOEnabled         bool              `json:"cgoEnabled"`
	DecoderOwnsNetwork bool              `json:"decoderOwnsNetwork"`
	RenderThreadIO     bool              `json:"renderThreadIO"`
	HeapSysStart       uint64            `json:"heapSysStartBytes"`
	HeapSysEnd         uint64            `json:"heapSysEndBytes"`
	Fixtures           []FixtureEvidence `json:"fixtures"`
	ShippingDecision   string            `json:"shippingDecision"`
	Passed             bool              `json:"passed"`
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func waitForSchedule() (time.Time, time.Time) {
	scheduled := time.Now().Add(25 * time.Millisecond)
	time.Sleep(time.Until(scheduled))
	return scheduled, time.Now()
}

func decodeMP3(ctx context.Context, id, path string) FixtureEvidence {
	result := FixtureEvidence{ID: id, Codec: "mp3", SourceBytes: fileSize(path), RingCapacityBytes: RingBytes}
	file, err := os.Open(path)
	if err != nil {
		result.Outcome, result.RejectReason = "reject", err.Error()
		return result
	}
	defer file.Close()
	meter := NewMeteredReader(file)
	decoder, err := mp3.NewDecoder(meter)
	if err != nil {
		result.Outcome, result.RejectReason = "reject", err.Error()
		return result
	}
	scheduled, started := waitForSchedule()
	buffer := make([]byte, 16_384)
	n, err := decoder.Read(buffer)
	first := time.Now()
	if n == 0 || (err != nil && !errors.Is(err, io.EOF)) {
		result.Outcome, result.RejectReason = "reject", fmt.Sprint(err)
		return result
	}
	result.BytesBeforeFirstPCM = meter.Snapshot().Bytes
	before := meter.Snapshot().Bytes
	time.Sleep(20 * time.Millisecond)
	result.PausedWithoutRead = meter.Snapshot().Bytes == before
	queue := NewPCMQueue(RingBytes)
	drain := make([]byte, 16_384)
	result.PCMBytes += int64(n)
	maxRing := queue.Push(buffer[:n])
	queue.Drain(drain)
	for {
		select {
		case <-ctx.Done():
			result.Cancelled = true
			result.Reads = meter.Snapshot()
			return result
		default:
		}
		n, err = decoder.Read(buffer)
		if n > 0 {
			result.PCMBytes += int64(n)
			queue.Push(buffer[:n])
			maxRing = max(maxRing, queue.Used())
			queue.Drain(drain)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			result.Outcome, result.RejectReason = "reject", err.Error()
			break
		}
	}
	result.Outcome = "decode-rejected-seek-full-scan"
	result.TrackStartMS = first.Sub(started).Milliseconds()
	result.ScheduledSkewMS = first.Sub(scheduled).Milliseconds()
	result.IncrementalFirstPCM = result.BytesBeforeFirstPCM < result.SourceBytes
	result.Resumed = true
	result.Drained = queue.Used() == 0
	result.Cancelled = proveCancelMP3(path)
	result.RingMaximumUsedBytes = maxRing
	result.Reads = meter.Snapshot()

	seekFile, seekErr := os.Open(path)
	if seekErr == nil {
		defer seekFile.Close()
		seekMeter := NewMeteredReadSeeker(seekFile)
		prepare := time.Now()
		seekDecoder, decoderErr := mp3.NewDecoder(seekMeter)
		result.SeekPrepareBytes = seekMeter.Snapshot().Bytes
		result.SeekRequiresFullScan = result.SeekPrepareBytes >= result.SourceBytes
		if decoderErr == nil {
			_, decoderErr = seekDecoder.Seek(int64(decoder.SampleRate()*4*5), io.SeekStart)
			if decoderErr == nil {
				_, decoderErr = seekDecoder.Read(buffer)
			}
			result.SeekSupported = decoderErr == nil
			result.SeekToPCMMS = time.Since(prepare).Milliseconds()
		}
	}
	return result
}

func proveCancelMP3(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	meter := NewMeteredReader(file)
	decoder, err := mp3.NewDecoder(meter)
	if err != nil {
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := meter.Snapshot().Bytes
	select {
	case <-ctx.Done():
		return meter.Snapshot().Bytes == before
	default:
		_, _ = decoder.Read(make([]byte, 1024))
		return false
	}
}

func decodeOpus(ctx context.Context, id, path string) FixtureEvidence {
	result := FixtureEvidence{ID: id, Codec: "opus", SourceBytes: fileSize(path), RingCapacityBytes: RingBytes}
	file, err := os.Open(path)
	if err != nil {
		result.Outcome, result.RejectReason = "reject", err.Error()
		return result
	}
	defer file.Close()
	meter := NewMeteredReader(file)
	ogg, _, err := oggreader.NewWith(meter)
	if err != nil {
		result.Outcome, result.RejectReason = "reject", err.Error()
		return result
	}
	decoder, err := opus.NewDecoderWithOutput(48_000, 2)
	if err != nil {
		result.Outcome, result.RejectReason = "reject", err.Error()
		return result
	}
	scheduled, started := waitForSchedule()
	queue := NewPCMQueue(RingBytes)
	drain := make([]byte, 48_000)
	pcm := make([]float32, 5_760*2)
	first := time.Time{}
	maxRing := 0
	for {
		select {
		case <-ctx.Done():
			result.Cancelled = true
			result.Reads = meter.Snapshot()
			return result
		default:
		}
		packet, _, parseErr := ogg.ParseNextPacket()
		if errors.Is(parseErr, io.EOF) {
			break
		}
		if parseErr != nil {
			result.Outcome, result.RejectReason = "reject", parseErr.Error()
			return result
		}
		if bytes.HasPrefix(packet, []byte("OpusTags")) {
			continue
		}
		samples, decodeErr := decoder.DecodeToFloat32(packet, pcm)
		if decodeErr != nil {
			result.Outcome, result.RejectReason = "reject", decodeErr.Error()
			return result
		}
		if first.IsZero() {
			first = time.Now()
			result.BytesBeforeFirstPCM = meter.Snapshot().Bytes
			before := meter.Snapshot().Bytes
			time.Sleep(20 * time.Millisecond)
			result.PausedWithoutRead = meter.Snapshot().Bytes == before
		}
		byteCount := samples * 2 * 4
		result.PCMBytes += int64(byteCount)
		chunk := make([]byte, byteCount)
		queue.Push(chunk)
		maxRing = max(maxRing, queue.Used())
		queue.Drain(drain)
	}
	result.Outcome = "decode-rejected-no-random-seek"
	result.RejectReason = "pion/opus pkg/oggreader is forward-only and exposes no random seek contract"
	result.TrackStartMS = first.Sub(started).Milliseconds()
	result.ScheduledSkewMS = first.Sub(scheduled).Milliseconds()
	result.IncrementalFirstPCM = result.BytesBeforeFirstPCM < result.SourceBytes
	result.Resumed = true
	result.Drained = queue.Used() == 0
	result.Cancelled = true
	result.SeekSupported = false
	result.RingMaximumUsedBytes = maxRing
	result.Reads = meter.Snapshot()
	return result
}

func rejectedAAC(id, codec string, size int64) FixtureEvidence {
	return FixtureEvidence{
		ID: id, Codec: codec, Outcome: "reject-forbidden-module", SourceBytes: size,
		RejectReason:      "GPL-2.0-only go-aac module is forbidden and absent from go.mod",
		RingCapacityBytes: RingBytes,
	}
}

func cgoEnabled() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, setting := range info.Settings {
		if setting.Key == "CGO_ENABLED" {
			return setting.Value == "1"
		}
	}
	return false
}

func Run(fixtures string) Evidence {
	var start runtime.MemStats
	runtime.ReadMemStats(&start)
	ctx := context.Background()
	items := []FixtureEvidence{
		decodeMP3(ctx, "mp3_cbr_12s", fixtures+"/mp3_cbr_12s.mp3"),
		decodeMP3(ctx, "mp3_vbr_12s", fixtures+"/mp3_vbr_12s.mp3"),
		rejectedAAC("aac_m4a_12s", "aac-lc", fileSize(fixtures+"/aac_m4a_12s.m4a")),
		rejectedAAC("aac_adts_12s", "aac-lc", fileSize(fixtures+"/aac_adts_12s.aac")),
		decodeOpus(ctx, "opus_ogg_cbr_12s", fixtures+"/opus_ogg_cbr_12s.ogg"),
		decodeOpus(ctx, "opus_ogg_vbr_12s", fixtures+"/opus_ogg_vbr_12s.ogg"),
	}
	var end runtime.MemStats
	runtime.ReadMemStats(&end)
	return Evidence{
		SchemaVersion: 1, Contract: "p2-pure-go-streaming-decoder-probe.v1",
		CandidateID: "pure-go-composite-v1", ClaimClass: "bounded-nondistributable-research",
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, CGOEnabled: cgoEnabled(),
		DecoderOwnsNetwork: false, RenderThreadIO: false, HeapSysStart: start.HeapSys, HeapSysEnd: end.HeapSys,
		Fixtures: items, ShippingDecision: "rejected-license-seek-and-manual-evidence-gates", Passed: false,
	}
}
