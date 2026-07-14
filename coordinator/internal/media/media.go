// Package media validates and canonicalizes untrusted phase-one audio clips.
// ffprobe and ffmpeg run with fixed demuxers/arguments, network protocols
// disabled, bounded output, deadlines, and production Linux rlimits.
package media

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	MaxClipBytes      = int64(50 << 20)
	MaxClipDurationMS = int64(180_000)
	MaxCanonicalBytes = int64(34 << 20)
)

type Preset string

const (
	PresetDefault Preset = "default"
	PresetRadio   Preset = "radio"
)

type Format string

const (
	FormatWAV  Format = "wav"
	FormatMP3  Format = "mp3"
	FormatM4A  Format = "m4a"
	FormatAAC  Format = "aac"
	FormatOGG  Format = "ogg"
	FormatFLAC Format = "flac"
)

type Limits struct {
	MaxInputBytes      int64
	MaxDurationMS      int64
	MaxOutputBytes     int64
	ProbeTimeout       time.Duration
	TranscodeTimeout   time.Duration
	WorkerCPUSeconds   uint64
	WorkerMemoryBytes  uint64
	WorkerOpenFiles    uint64
	WorkerOutputBytes  uint64
	WorkerConcurrency  int
	WorkerQueueTimeout time.Duration
	ProbeOutputBytes   int64
	WorkerLogBytes     int64
}

func DefaultLimits() Limits {
	return Limits{
		MaxInputBytes: MaxClipBytes, MaxDurationMS: MaxClipDurationMS,
		MaxOutputBytes: MaxCanonicalBytes,
		ProbeTimeout:   10 * time.Second, TranscodeTimeout: 60 * time.Second,
		WorkerCPUSeconds: 45, WorkerMemoryBytes: 512 << 20,
		WorkerOpenFiles: 64, WorkerOutputBytes: uint64(MaxCanonicalBytes),
		WorkerConcurrency: 4, WorkerQueueTimeout: 5 * time.Second,
		ProbeOutputBytes: 128 << 10, WorkerLogBytes: 256 << 10,
	}
}

type Result struct {
	WAVPath      string
	DurationMS   int64
	LoudnormJSON string
	SizeBytes    int64
	SHA256       string
	InputSHA256  string
	InputFormat  Format
}

// ProcessingError contains only a stable sanitized failure code. Underlying
// command and filesystem errors are intentionally not exposed through Error.
type ProcessingError struct {
	Code  string
	cause error
}

func (e *ProcessingError) Error() string { return e.Code }
func (e *ProcessingError) Unwrap() error { return e.cause }

func processingError(code string, cause error) error {
	return &ProcessingError{Code: code, cause: cause}
}

func FailureCode(err error) (string, bool) {
	var processing *ProcessingError
	if !errors.As(err, &processing) {
		return "", false
	}
	return processing.Code, true
}

type Processor struct {
	ffprobe string
	ffmpeg  string
	runner  commandRunner
	limits  Limits
}

func NewProcessor() (*Processor, error) {
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		return nil, errors.New("ffprobe is unavailable")
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("ffmpeg is unavailable")
	}
	return &Processor{
		ffprobe: ffprobe, ffmpeg: ffmpeg, runner: osCommandRunner{},
		limits: DefaultLimits(),
	}, nil
}

func newProcessorForTest(runner commandRunner, limits Limits) *Processor {
	return &Processor{ffprobe: "/tools/ffprobe", ffmpeg: "/tools/ffmpeg", runner: runner, limits: limits}
}

func filterChain(p Preset) string {
	base := "highpass=f=90,acompressor=threshold=-20dB:ratio=3:attack=10:release=180:makeup=4"
	if p == PresetRadio {
		base += ",equalizer=f=4500:t=q:w=1:g=2,deesser"
	}
	return base + ",loudnorm=I=-14:TP=-1.5:LRA=11:print_format=json"
}

const disabledProtocols = "bluray,cache,concat,concatf,crypto,data,fd,ftp,gopher,gophers,hls,http,httpproxy,https,icecast,ipfs,ipns,md5,mmsh,mmst,pipe,prompeg,rist,rtmp,rtmpe,rtmps,rtmpt,rtmpte,rtmpts,rtp,sctp,sftp,srt,srtp,subfile,tee,tcp,tls,udp,udplite,unix"

func (p *Processor) Process(ctx context.Context, inputPath, outputPath string, preset Preset) (Result, error) {
	if ctx == nil || (preset != PresetDefault && preset != PresetRadio) {
		return Result{}, processingError("media_request_invalid", nil)
	}
	info, err := os.Stat(inputPath)
	if err != nil || !info.Mode().IsRegular() {
		return Result{}, processingError("media_input_unavailable", err)
	}
	if info.Size() <= 0 || info.Size() > p.limits.MaxInputBytes {
		return Result{}, processingError("media_input_oversized", nil)
	}
	format, err := detectFormat(inputPath, info.Size())
	if err != nil {
		return Result{}, err
	}
	inputHash, _, err := hashFile(inputPath, p.limits.MaxInputBytes)
	if err != nil {
		return Result{}, processingError("media_input_unreadable", err)
	}
	probe, err := p.probe(ctx, inputPath, format, info.Size())
	if err != nil {
		return Result{}, err
	}
	if err := validateInputProbe(probe, format, p.limits.MaxDurationMS); err != nil {
		return Result{}, err
	}

	_ = os.Remove(outputPath)
	staging, err := os.OpenFile(outputPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return Result{}, processingError("canonical_output_unavailable", err)
	}
	if err := staging.Close(); err != nil {
		_ = os.Remove(outputPath)
		return Result{}, processingError("canonical_output_unavailable", err)
	}
	transcodeCtx, cancel := context.WithTimeout(ctx, p.limits.TranscodeTimeout)
	defer cancel()
	command := commandSpec{
		Tool:        p.ffmpeg,
		Args:        transcodeArguments(format, inputPath, outputPath, preset, p.limits),
		StdoutLimit: 1024, StderrLimit: p.limits.WorkerLogBytes,
		CPUSeconds:  p.limits.WorkerCPUSeconds,
		MemoryBytes: p.limits.WorkerMemoryBytes,
		OpenFiles:   p.limits.WorkerOpenFiles,
		FileBytes:   p.limits.WorkerOutputBytes,
	}
	result, runErr := p.runner.Run(transcodeCtx, command)
	if runErr != nil {
		_ = os.Remove(outputPath)
		if errors.Is(transcodeCtx.Err(), context.DeadlineExceeded) {
			return Result{}, processingError("ffmpeg_timeout", transcodeCtx.Err())
		}
		return Result{}, processingError("ffmpeg_failed", runErr)
	}
	if err := os.Chmod(outputPath, 0o600); err != nil {
		_ = os.Remove(outputPath)
		return Result{}, processingError("canonical_output_unavailable", err)
	}
	loudness := extractLastJSON(string(result.Stderr))
	if !validLoudnessJSON(loudness) {
		_ = os.Remove(outputPath)
		return Result{}, processingError("loudness_invalid", nil)
	}
	outputInfo, err := os.Stat(outputPath)
	if err != nil || !outputInfo.Mode().IsRegular() {
		_ = os.Remove(outputPath)
		return Result{}, processingError("canonical_output_missing", err)
	}
	if outputInfo.Size() <= 44 || outputInfo.Size() > p.limits.MaxOutputBytes {
		_ = os.Remove(outputPath)
		return Result{}, processingError("canonical_output_oversized", nil)
	}
	canonicalFormat, err := detectFormat(outputPath, outputInfo.Size())
	if err != nil || canonicalFormat != FormatWAV {
		_ = os.Remove(outputPath)
		return Result{}, processingError("canonical_output_invalid", err)
	}
	canonicalProbe, err := p.probe(ctx, outputPath, FormatWAV, outputInfo.Size())
	if err != nil {
		_ = os.Remove(outputPath)
		return Result{}, processingError("canonical_output_invalid", err)
	}
	if err := validateCanonicalProbe(canonicalProbe, p.limits.MaxDurationMS); err != nil {
		_ = os.Remove(outputPath)
		return Result{}, err
	}
	canonicalHash, canonicalSize, err := hashFile(outputPath, p.limits.MaxOutputBytes)
	if err != nil {
		_ = os.Remove(outputPath)
		return Result{}, processingError("canonical_output_unreadable", err)
	}
	return Result{
		WAVPath: outputPath, DurationMS: canonicalProbe.DurationMS,
		LoudnormJSON: loudness, SizeBytes: canonicalSize, SHA256: canonicalHash,
		InputSHA256: inputHash, InputFormat: format,
	}, nil
}

// Process preserves the legacy Telegram-facing function while routing its
// bytes through the same constrained validator/transcoder used by SubmitMedia.
func Process(inPath, outPath string, preset Preset) (Result, error) {
	processor, err := NewProcessor()
	if err != nil {
		return Result{}, processingError("media_worker_unavailable", err)
	}
	return processor.Process(context.Background(), inPath, outPath, preset)
}

type probeResult struct {
	FormatName string
	CodecName  string
	CodecType  string
	DurationMS int64
	SizeBytes  int64
	SampleRate int
	Channels   int
	Streams    int
}

func (p *Processor) probe(ctx context.Context, path string, format Format, expectedSize int64) (probeResult, error) {
	probeCtx, cancel := context.WithTimeout(ctx, p.limits.ProbeTimeout)
	defer cancel()
	spec := commandSpec{
		Tool: p.ffprobe, Args: probeArguments(format, path),
		StdoutLimit: p.limits.ProbeOutputBytes, StderrLimit: p.limits.WorkerLogBytes,
		CPUSeconds: p.limits.WorkerCPUSeconds, MemoryBytes: p.limits.WorkerMemoryBytes,
		OpenFiles: p.limits.WorkerOpenFiles,
	}
	raw, err := p.runner.Run(probeCtx, spec)
	if err != nil {
		if errors.Is(probeCtx.Err(), context.DeadlineExceeded) {
			return probeResult{}, processingError("ffprobe_timeout", probeCtx.Err())
		}
		return probeResult{}, processingError("ffprobe_failed", err)
	}
	var payload struct {
		Format struct {
			FormatName string `json:"format_name"`
			Duration   string `json:"duration"`
			Size       string `json:"size"`
		} `json:"format"`
		Streams []struct {
			CodecName  string `json:"codec_name"`
			CodecType  string `json:"codec_type"`
			SampleRate string `json:"sample_rate"`
			Channels   int    `json:"channels"`
		} `json:"streams"`
	}
	if len(raw.Stdout) == 0 || len(raw.Stdout) > int(p.limits.ProbeOutputBytes) ||
		json.Unmarshal(raw.Stdout, &payload) != nil {
		return probeResult{}, processingError("ffprobe_output_invalid", nil)
	}
	duration, err := strconv.ParseFloat(payload.Format.Duration, 64)
	if err != nil || math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 {
		return probeResult{}, processingError("media_duration_invalid", err)
	}
	size, err := strconv.ParseInt(payload.Format.Size, 10, 64)
	if err != nil || size != expectedSize {
		return probeResult{}, processingError("media_size_mismatch", err)
	}
	result := probeResult{
		FormatName: payload.Format.FormatName, DurationMS: int64(math.Ceil(duration * 1000)),
		SizeBytes: size, Streams: len(payload.Streams),
	}
	if len(payload.Streams) == 1 {
		result.CodecName = payload.Streams[0].CodecName
		result.CodecType = payload.Streams[0].CodecType
		result.Channels = payload.Streams[0].Channels
		result.SampleRate, _ = strconv.Atoi(payload.Streams[0].SampleRate)
	}
	return result, nil
}

func probeArguments(format Format, path string) []string {
	return []string{
		"-v", "error", "-hide_banner", "-protocol_whitelist", "file",
		"-protocol_blacklist", disabledProtocols,
		"-probesize", "8388608", "-analyzeduration", "10000000",
		"-max_alloc", "134217728", "-threads", "1",
		"-f", demuxer(format), "-i", path,
		"-show_entries", "format=format_name,duration,size:stream=codec_name,codec_type,sample_rate,channels",
		"-of", "json",
	}
}

func transcodeArguments(format Format, inputPath, outputPath string, preset Preset, limits Limits) []string {
	return []string{
		"-v", "info", "-hide_banner", "-nostdin", "-nostats", "-xerror",
		"-err_detect", "explode", "-protocol_whitelist", "file",
		"-protocol_blacklist", disabledProtocols,
		"-max_alloc", "134217728", "-threads", "1",
		"-filter_threads", "1", "-filter_complex_threads", "1",
		"-f", demuxer(format), "-i", inputPath,
		"-map", "0:a:0", "-map_metadata", "-1", "-vn", "-sn", "-dn",
		"-af", filterChain(preset), "-t", "180.001",
		"-ar", "44100", "-ac", "2", "-c:a", "pcm_s16le",
		"-fflags", "+bitexact", "-flags:a", "+bitexact",
		"-max_muxing_queue_size", "64", "-f", "wav",
		"-fs", strconv.FormatInt(limits.MaxOutputBytes, 10), "-y", outputPath,
	}
}

func demuxer(format Format) string {
	switch format {
	case FormatM4A:
		return "mov"
	default:
		return string(format)
	}
}

func validateInputProbe(probe probeResult, format Format, maxDurationMS int64) error {
	if probe.Streams != 1 || probe.CodecType != "audio" {
		return processingError("media_stream_layout_unsupported", nil)
	}
	if probe.DurationMS <= 0 || probe.DurationMS > maxDurationMS {
		return processingError("media_duration_exceeded", nil)
	}
	if !formatNameMatches(probe.FormatName, format) || !codecAllowed(format, probe.CodecName) {
		return processingError("media_codec_unsupported", nil)
	}
	if probe.Channels <= 0 || probe.Channels > 8 || probe.SampleRate <= 0 || probe.SampleRate > 384000 {
		return processingError("media_stream_parameters_invalid", nil)
	}
	return nil
}

func validateCanonicalProbe(probe probeResult, maxDurationMS int64) error {
	if probe.Streams != 1 || probe.CodecType != "audio" || probe.CodecName != "pcm_s16le" ||
		probe.SampleRate != 44100 || probe.Channels != 2 ||
		probe.DurationMS <= 0 || probe.DurationMS > maxDurationMS+1 {
		return processingError("canonical_output_invalid", nil)
	}
	return nil
}

func formatNameMatches(name string, format Format) bool {
	for _, part := range strings.Split(name, ",") {
		if part == string(format) || (format == FormatM4A && (part == "mov" || part == "mp4" || part == "m4a")) {
			return true
		}
	}
	return false
}

func codecAllowed(format Format, codec string) bool {
	allowed := map[Format]map[string]bool{
		FormatWAV: {
			"pcm_u8": true, "pcm_s8": true, "pcm_s16le": true, "pcm_s24le": true,
			"pcm_s32le": true, "pcm_f32le": true, "pcm_f64le": true,
		},
		FormatMP3:  {"mp3": true},
		FormatM4A:  {"aac": true, "alac": true},
		FormatAAC:  {"aac": true},
		FormatOGG:  {"opus": true, "vorbis": true},
		FormatFLAC: {"flac": true},
	}
	return allowed[format][codec]
}

func detectFormat(path string, size int64) (Format, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", processingError("media_input_unreadable", err)
	}
	defer file.Close()
	header := make([]byte, 64)
	n, err := io.ReadFull(file, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", processingError("media_input_unreadable", err)
	}
	header = header[:n]
	switch {
	case len(header) >= 12 && string(header[:4]) == "RIFF" && string(header[8:12]) == "WAVE":
		declared := int64(binary.LittleEndian.Uint32(header[4:8])) + 8
		if declared != size {
			return "", processingError("media_polyglot_or_truncated", nil)
		}
		return FormatWAV, nil
	case len(header) >= 3 && string(header[:3]) == "ID3":
		if err := validateMP3(file, size); err != nil {
			return "", err
		}
		return FormatMP3, nil
	case len(header) >= 2 && header[0] == 0xff && header[1]&0xe0 == 0xe0 && header[1]&0x06 != 0:
		if err := validateMP3(file, size); err != nil {
			return "", err
		}
		return FormatMP3, nil
	case len(header) >= 12 && string(header[4:8]) == "ftyp":
		if err := validateBMFF(file, size); err != nil {
			return "", err
		}
		return FormatM4A, nil
	case len(header) >= 2 && header[0] == 0xff && header[1]&0xf6 == 0xf0:
		if err := validateADTS(file, size); err != nil {
			return "", err
		}
		return FormatAAC, nil
	case len(header) >= 4 && string(header[:4]) == "OggS":
		if err := validateOgg(file, size); err != nil {
			return "", err
		}
		return FormatOGG, nil
	case len(header) >= 4 && string(header[:4]) == "fLaC":
		if err := validateFLAC(file, size); err != nil {
			return "", err
		}
		return FormatFLAC, nil
	default:
		return "", processingError("media_signature_unsupported", nil)
	}
}

func validateMP3(file *os.File, size int64) error {
	var offset int64
	header := make([]byte, 10)
	if size >= 10 {
		if _, err := file.ReadAt(header, 0); err != nil {
			return processingError("media_input_unreadable", err)
		}
		if string(header[:3]) == "ID3" {
			if header[3] < 2 || header[3] > 4 ||
				header[6]&0x80 != 0 || header[7]&0x80 != 0 || header[8]&0x80 != 0 || header[9]&0x80 != 0 {
				return processingError("media_polyglot_or_truncated", nil)
			}
			tagSize := int64(header[6])<<21 | int64(header[7])<<14 | int64(header[8])<<7 | int64(header[9])
			offset = 10 + tagSize
			if header[3] == 4 && header[5]&0x10 != 0 {
				offset += 10
			}
			if offset >= size {
				return processingError("media_polyglot_or_truncated", nil)
			}
		}
	}
	frames := 0
	frameHeader := make([]byte, 4)
	for offset < size {
		remaining := size - offset
		if remaining == 128 {
			var tag [3]byte
			if _, err := file.ReadAt(tag[:], offset); err != nil {
				return processingError("media_input_unreadable", err)
			}
			if string(tag[:]) == "TAG" && frames > 0 {
				return nil
			}
		}
		if remaining < 4 {
			return processingError("media_polyglot_or_truncated", nil)
		}
		if _, err := file.ReadAt(frameHeader, offset); err != nil {
			return processingError("media_input_unreadable", err)
		}
		frameSize, ok := mp3FrameSize(binary.BigEndian.Uint32(frameHeader))
		if !ok || int64(frameSize) > remaining {
			return processingError("media_polyglot_or_truncated", nil)
		}
		offset += int64(frameSize)
		frames++
	}
	if offset != size || frames == 0 {
		return processingError("media_polyglot_or_truncated", nil)
	}
	return nil
}

func mp3FrameSize(header uint32) (int, bool) {
	if header&0xffe00000 != 0xffe00000 {
		return 0, false
	}
	version := (header >> 19) & 0x3
	layer := (header >> 17) & 0x3
	bitrateIndex := int((header >> 12) & 0xf)
	sampleIndex := int((header >> 10) & 0x3)
	padding := int((header >> 9) & 0x1)
	if version == 1 || layer == 0 || bitrateIndex == 0 || bitrateIndex == 15 || sampleIndex == 3 {
		return 0, false
	}
	mpeg1Layer1 := [...]int{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448}
	mpeg1Layer2 := [...]int{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384}
	mpeg1Layer3 := [...]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
	mpeg2Layer1 := [...]int{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256}
	mpeg2Layer23 := [...]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
	var bitrate int
	if version == 3 {
		switch layer {
		case 3:
			bitrate = mpeg1Layer1[bitrateIndex]
		case 2:
			bitrate = mpeg1Layer2[bitrateIndex]
		case 1:
			bitrate = mpeg1Layer3[bitrateIndex]
		}
	} else if layer == 3 {
		bitrate = mpeg2Layer1[bitrateIndex]
	} else {
		bitrate = mpeg2Layer23[bitrateIndex]
	}
	sampleRate := [...]int{44100, 48000, 32000}[sampleIndex]
	if version == 2 {
		sampleRate /= 2
	} else if version == 0 {
		sampleRate /= 4
	}
	bitrate *= 1000
	switch layer {
	case 3:
		return (12*bitrate/sampleRate + padding) * 4, true
	case 2:
		return 144*bitrate/sampleRate + padding, true
	case 1:
		coefficient := 144
		if version != 3 {
			coefficient = 72
		}
		return coefficient*bitrate/sampleRate + padding, true
	default:
		return 0, false
	}
}

func validateFLAC(file *os.File, size int64) error {
	if size < 4+4+34+6 {
		return processingError("media_polyglot_or_truncated", nil)
	}
	offset := int64(4)
	blocks := 0
	last := false
	header := make([]byte, 4)
	for !last {
		if size-offset < 4 {
			return processingError("media_polyglot_or_truncated", nil)
		}
		if _, err := file.ReadAt(header, offset); err != nil {
			return processingError("media_input_unreadable", err)
		}
		last = header[0]&0x80 != 0
		blockType := header[0] & 0x7f
		length := int64(header[1])<<16 | int64(header[2])<<8 | int64(header[3])
		if blockType == 0x7f || length > size-offset-4 ||
			(blocks == 0 && (blockType != 0 || length != 34)) ||
			(blocks > 0 && blockType == 0) {
			return processingError("media_polyglot_or_truncated", nil)
		}
		offset += 4 + length
		blocks++
	}
	var frame [2]byte
	if size-offset < 6 {
		return processingError("media_polyglot_or_truncated", nil)
	}
	if _, err := file.ReadAt(frame[:], offset); err != nil {
		return processingError("media_input_unreadable", err)
	}
	if frame[0] != 0xff || frame[1]&0xfe != 0xf8 {
		return processingError("media_polyglot_or_truncated", nil)
	}
	return nil
}

func validateBMFF(file *os.File, size int64) error {
	var offset int64
	foundFtyp, foundMedia := false, false
	header := make([]byte, 16)
	for offset < size {
		if size-offset < 8 {
			return processingError("media_polyglot_or_truncated", nil)
		}
		if _, err := file.ReadAt(header[:8], offset); err != nil {
			return processingError("media_input_unreadable", err)
		}
		atomSize := int64(binary.BigEndian.Uint32(header[:4]))
		headerSize := int64(8)
		if atomSize == 1 {
			if size-offset < 16 {
				return processingError("media_polyglot_or_truncated", nil)
			}
			if _, err := file.ReadAt(header, offset); err != nil {
				return processingError("media_input_unreadable", err)
			}
			wide := binary.BigEndian.Uint64(header[8:16])
			if wide > math.MaxInt64 {
				return processingError("media_polyglot_or_truncated", nil)
			}
			atomSize, headerSize = int64(wide), 16
		} else if atomSize == 0 {
			atomSize = size - offset
		}
		if atomSize < headerSize || atomSize > size-offset {
			return processingError("media_polyglot_or_truncated", nil)
		}
		kind := string(header[4:8])
		foundFtyp = foundFtyp || kind == "ftyp"
		foundMedia = foundMedia || kind == "moov" || kind == "mdat"
		offset += atomSize
	}
	if offset != size || !foundFtyp || !foundMedia {
		return processingError("media_polyglot_or_truncated", nil)
	}
	return nil
}

func validateADTS(file *os.File, size int64) error {
	var offset int64
	header := make([]byte, 7)
	frames := 0
	for offset < size {
		if size-offset < 7 {
			return processingError("media_polyglot_or_truncated", nil)
		}
		if _, err := file.ReadAt(header, offset); err != nil {
			return processingError("media_input_unreadable", err)
		}
		if header[0] != 0xff || header[1]&0xf6 != 0xf0 {
			return processingError("media_polyglot_or_truncated", nil)
		}
		frameLength := int64(header[3]&0x03)<<11 | int64(header[4])<<3 | int64(header[5]>>5)
		if frameLength < 7 || frameLength > size-offset {
			return processingError("media_polyglot_or_truncated", nil)
		}
		offset += frameLength
		frames++
	}
	if offset != size || frames == 0 {
		return processingError("media_polyglot_or_truncated", nil)
	}
	return nil
}

func validateOgg(file *os.File, size int64) error {
	var offset int64
	header := make([]byte, 27)
	pages := 0
	for offset < size {
		if size-offset < 27 {
			return processingError("media_polyglot_or_truncated", nil)
		}
		if _, err := file.ReadAt(header, offset); err != nil {
			return processingError("media_input_unreadable", err)
		}
		if string(header[:4]) != "OggS" || header[4] != 0 {
			return processingError("media_polyglot_or_truncated", nil)
		}
		segments := int(header[26])
		segmentTable := make([]byte, segments)
		if _, err := file.ReadAt(segmentTable, offset+27); err != nil {
			return processingError("media_polyglot_or_truncated", err)
		}
		pageSize := int64(27 + segments)
		for _, segment := range segmentTable {
			pageSize += int64(segment)
		}
		if pageSize > size-offset {
			return processingError("media_polyglot_or_truncated", nil)
		}
		offset += pageSize
		pages++
	}
	if offset != size || pages == 0 {
		return processingError("media_polyglot_or_truncated", nil)
	}
	return nil
}

func hashFile(path string, limit int64) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, limit+1))
	if err != nil {
		return "", 0, err
	}
	if written > limit {
		return "", written, errors.New("file exceeds hash limit")
	}
	return hex.EncodeToString(hash.Sum(nil)), written, nil
}

func validLoudnessJSON(value string) bool {
	if len(value) == 0 || len(value) > 16<<10 {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(value), &fields) != nil || fields == nil {
		return false
	}
	// Keep both source and canonical loudness/true-peak measurements. The
	// output values are the publication contract; the input values make the
	// normalization decision auditable without retaining source bytes.
	for _, key := range []string{"input_i", "input_tp", "output_i", "output_tp"} {
		raw, ok := fields[key]
		if !ok {
			return false
		}
		var text string
		if json.Unmarshal(raw, &text) != nil {
			text = strings.TrimSpace(string(raw))
		}
		number, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return false
		}
	}
	return true
}

// extractLastJSON pulls the trailing balanced object that loudnorm prints.
func extractLastJSON(value string) string {
	end := strings.LastIndex(value, "}")
	if end < 0 {
		return ""
	}
	depth := 0
	for index := end; index >= 0; index-- {
		switch value[index] {
		case '}':
			depth++
		case '{':
			depth--
			if depth == 0 {
				return value[index : end+1]
			}
		}
	}
	return ""
}
