package media

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeCommandRunner struct {
	mu             sync.Mutex
	commands       []commandSpec
	probeDuration  string
	probeStreams   int
	probeCodec     string
	probeFormat    string
	probeBlock     bool
	probeError     error
	transcodeBlock bool
	transcodeError error
	loudness       string
	output         []byte
}

func newFakeCommandRunner() *fakeCommandRunner {
	return &fakeCommandRunner{
		probeDuration: "1.000", probeStreams: 1,
		loudness: `{"input_i":"-22.1","input_tp":"-3.2","output_i":"-14.0","output_tp":"-1.5"}`,
		output:   testWAVBytes(4410),
	}
}

func (runner *fakeCommandRunner) Run(ctx context.Context, spec commandSpec) (commandResult, error) {
	runner.mu.Lock()
	runner.commands = append(runner.commands, spec)
	runner.mu.Unlock()
	if strings.Contains(spec.Tool, "ffprobe") {
		if runner.probeBlock {
			<-ctx.Done()
			return commandResult{}, ctx.Err()
		}
		if runner.probeError != nil {
			return commandResult{}, runner.probeError
		}
		path := argumentAfter(spec.Args, "-i")
		info, err := os.Stat(path)
		if err != nil {
			return commandResult{}, err
		}
		format := argumentAfter(spec.Args, "-f")
		if runner.probeFormat != "" {
			format = runner.probeFormat
		} else if format == "mov" {
			format = "mov,mp4,m4a"
		}
		codec := runner.probeCodec
		if codec == "" {
			switch argumentAfter(spec.Args, "-f") {
			case "mp3":
				codec = "mp3"
			case "mov", "aac":
				codec = "aac"
			case "ogg":
				codec = "opus"
			case "flac":
				codec = "flac"
			default:
				codec = "pcm_s16le"
			}
		}
		streams := make([]map[string]any, runner.probeStreams)
		for index := range streams {
			streams[index] = map[string]any{
				"codec_name": codec, "codec_type": "audio",
				"sample_rate": "44100", "channels": 2,
			}
		}
		payload, err := json.Marshal(map[string]any{
			"format": map[string]string{
				"format_name": format, "duration": runner.probeDuration,
				"size": strconv.FormatInt(info.Size(), 10),
			},
			"streams": streams,
		})
		return commandResult{Stdout: payload}, err
	}
	if runner.transcodeBlock {
		<-ctx.Done()
		return commandResult{}, ctx.Err()
	}
	if runner.transcodeError != nil {
		return commandResult{}, runner.transcodeError
	}
	outputPath := spec.Args[len(spec.Args)-1]
	if err := os.WriteFile(outputPath, runner.output, 0o600); err != nil {
		return commandResult{}, err
	}
	return commandResult{Stderr: []byte("worker log\n" + runner.loudness + "\n")}, nil
}

func (runner *fakeCommandRunner) commandSnapshot() []commandSpec {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]commandSpec(nil), runner.commands...)
}

func argumentAfter(arguments []string, name string) string {
	for index := 0; index+1 < len(arguments); index++ {
		if arguments[index] == name {
			return arguments[index+1]
		}
	}
	return ""
}

func lastArgumentAfter(arguments []string, name string) string {
	for index := len(arguments) - 2; index >= 0; index-- {
		if arguments[index] == name {
			return arguments[index+1]
		}
	}
	return ""
}

func testWAVBytes(frames int) []byte {
	dataSize := frames * 4
	raw := make([]byte, 44+dataSize)
	copy(raw[:4], "RIFF")
	binary.LittleEndian.PutUint32(raw[4:8], uint32(36+dataSize))
	copy(raw[8:12], "WAVE")
	copy(raw[12:16], "fmt ")
	binary.LittleEndian.PutUint32(raw[16:20], 16)
	binary.LittleEndian.PutUint16(raw[20:22], 1)
	binary.LittleEndian.PutUint16(raw[22:24], 2)
	binary.LittleEndian.PutUint32(raw[24:28], 44100)
	binary.LittleEndian.PutUint32(raw[28:32], 44100*4)
	binary.LittleEndian.PutUint16(raw[32:34], 4)
	binary.LittleEndian.PutUint16(raw[34:36], 16)
	copy(raw[36:40], "data")
	binary.LittleEndian.PutUint32(raw[40:44], uint32(dataSize))
	return raw
}

func testMP3Bytes(frames int) []byte {
	const frameSize = 417 // MPEG-1 Layer III, 128 kbps, 44.1 kHz, no padding.
	raw := make([]byte, frames*frameSize)
	for offset := 0; offset < len(raw); offset += frameSize {
		copy(raw[offset:offset+4], []byte{0xff, 0xfb, 0x90, 0x00})
	}
	return raw
}

func testFLACBytes() []byte {
	raw := make([]byte, 4+4+34+6)
	copy(raw[:4], "fLaC")
	raw[4], raw[7] = 0x80, 34 // final STREAMINFO block, exactly 34 bytes.
	copy(raw[42:], []byte{0xff, 0xf8, 0x69, 0x08, 0x00, 0x00})
	return raw
}

func testM4ABytes() []byte {
	raw := make([]byte, 20)
	binary.BigEndian.PutUint32(raw[:4], 12)
	copy(raw[4:8], "ftyp")
	copy(raw[8:12], "M4A ")
	binary.BigEndian.PutUint32(raw[12:16], 8)
	copy(raw[16:20], "mdat")
	return raw
}

func testADTSBytes() []byte {
	return []byte{0xff, 0xf1, 0x50, 0x80, 0x00, 0xe0, 0xfc}
}

func testOggBytes() []byte {
	raw := make([]byte, 32)
	copy(raw[:4], "OggS")
	raw[4], raw[26], raw[27] = 0, 1, 4
	copy(raw[28:], "data")
	return raw
}

func writeTestInput(t *testing.T, raw []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.bin")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertProcessingCode(t *testing.T, err error, code string) {
	t.Helper()
	got, ok := FailureCode(err)
	if !ok || got != code {
		t.Fatalf("processing error=%v code=%q want=%q", err, got, code)
	}
}

func TestProcessorCanonicalizesWithFixedNetworkDisabledResourceCappedCommands(t *testing.T) {
	runner := newFakeCommandRunner()
	limits := DefaultLimits()
	processor := newProcessorForTest(runner, limits)
	input := writeTestInput(t, testWAVBytes(100))
	output := filepath.Join(t.TempDir(), "canonical.wav")
	result, err := processor.Process(context.Background(), input, output, PresetDefault)
	if err != nil {
		t.Fatal(err)
	}
	if result.DurationMS != 1000 || result.SizeBytes != int64(len(runner.output)) ||
		len(result.SHA256) != 64 || len(result.InputSHA256) != 64 || result.InputFormat != FormatWAV ||
		!validLoudnessJSON(result.LoudnormJSON) {
		t.Fatalf("canonical result=%+v", result)
	}
	commands := runner.commandSnapshot()
	if len(commands) != 3 {
		t.Fatalf("commands=%d want input probe, transcode, output probe", len(commands))
	}
	for _, command := range commands {
		if argumentAfter(command.Args, "-protocol_whitelist") != "file" ||
			argumentAfter(command.Args, "-protocol_blacklist") != disabledProtocols ||
			argumentAfter(command.Args, "-threads") != "1" ||
			command.CPUSeconds != limits.WorkerCPUSeconds ||
			command.MemoryBytes != limits.WorkerMemoryBytes || command.OpenFiles != limits.WorkerOpenFiles {
			t.Fatalf("unbounded or network-enabled command=%+v", command)
		}
	}
	transcode := commands[1]
	if transcode.Tool != "/tools/ffmpeg" || transcode.FileBytes != uint64(limits.MaxOutputBytes) ||
		argumentAfter(transcode.Args, "-af") != filterChain(PresetDefault) ||
		argumentAfter(transcode.Args, "-f") != "wav" || lastArgumentAfter(transcode.Args, "-f") != "wav" ||
		argumentAfter(transcode.Args, "-fs") != strconv.FormatInt(limits.MaxOutputBytes, 10) {
		t.Fatalf("transcode contract=%+v", transcode)
	}
	joined := strings.Join(transcode.Args, " ")
	for _, required := range []string{"-xerror", "explode", "pcm_s16le", "44100", "-map_metadata -1", "-map_chapters -1", "-fflags +bitexact"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("transcode missing %q: %s", required, joined)
		}
	}
}

func TestTrackProbeUsesOneNetworkDisabledResourceCappedCommandAndNoSpeechChain(t *testing.T) {
	runner := newFakeCommandRunner()
	runner.probeDuration = "3600.000"
	limits := DefaultLimits()
	processor := newProcessorForTest(runner, limits)
	input := writeTestInput(t, testWAVBytes(100))
	result, err := processor.ProbeTrack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.DurationMS != 3_600_000 || result.SizeBytes <= 0 || len(result.SHA256) != 64 ||
		result.Container != "wav" || result.Codec != "pcm_s16le" || result.MIME != "audio/wav" {
		t.Fatalf("track probe=%+v", result)
	}
	commands := runner.commandSnapshot()
	if len(commands) != 1 {
		t.Fatalf("track commands=%d want one probe", len(commands))
	}
	command := commands[0]
	joined := strings.Join(command.Args, " ")
	if command.Tool != "/tools/ffprobe" ||
		argumentAfter(command.Args, "-protocol_whitelist") != "file" ||
		argumentAfter(command.Args, "-protocol_blacklist") != disabledProtocols ||
		argumentAfter(command.Args, "-threads") != "1" ||
		command.CPUSeconds != limits.WorkerCPUSeconds ||
		command.MemoryBytes != limits.WorkerMemoryBytes || command.OpenFiles != limits.WorkerOpenFiles ||
		strings.Contains(joined, "loudnorm") || strings.Contains(joined, "acompressor") ||
		strings.Contains(joined, "highpass") || strings.Contains(joined, "pcm_s16le") {
		t.Fatalf("unsafe track probe command=%+v", command)
	}
}

func TestSignatureProbeRejectsUnsupportedTruncatedAndPolyglotBeforeWorker(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		code string
	}{
		{"unsupported", []byte("not audio at all"), "media_signature_unsupported"},
		{"truncated_wav", func() []byte {
			raw := testWAVBytes(10)
			return raw[:len(raw)-3]
		}(), "media_polyglot_or_truncated"},
		{"wav_zip_polyglot", append(testWAVBytes(10), []byte("PK\x03\x04payload")...), "media_polyglot_or_truncated"},
		{"truncated_mp3", testMP3Bytes(1)[:400], "media_polyglot_or_truncated"},
		{"mp3_zip_polyglot", append(testMP3Bytes(1), []byte("PK\x03\x04payload")...), "media_polyglot_or_truncated"},
		{"truncated_m4a", testM4ABytes()[:19], "media_polyglot_or_truncated"},
		{"m4a_zip_polyglot", append(testM4ABytes(), []byte("PK\x03\x04payload")...), "media_polyglot_or_truncated"},
		{"truncated_aac", testADTSBytes()[:6], "media_polyglot_or_truncated"},
		{"aac_zip_polyglot", append(testADTSBytes(), []byte("PK\x03\x04payload")...), "media_polyglot_or_truncated"},
		{"truncated_ogg", testOggBytes()[:31], "media_polyglot_or_truncated"},
		{"ogg_zip_polyglot", append(testOggBytes(), []byte("PK\x03\x04payload")...), "media_polyglot_or_truncated"},
		{"truncated_flac", testFLACBytes()[:47], "media_polyglot_or_truncated"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := newFakeCommandRunner()
			processor := newProcessorForTest(runner, DefaultLimits())
			_, err := processor.Process(
				context.Background(), writeTestInput(t, test.raw), filepath.Join(t.TempDir(), "out.wav"), PresetDefault,
			)
			assertProcessingCode(t, err, test.code)
			if len(runner.commandSnapshot()) != 0 {
				t.Fatal("signature rejection invoked a worker")
			}
		})
	}
}

func TestProcessorRejectsProbeAndWorkerBoundaryFailures(t *testing.T) {
	t.Run("duration", func(t *testing.T) {
		runner := newFakeCommandRunner()
		runner.probeDuration = "180.001"
		processor := newProcessorForTest(runner, DefaultLimits())
		_, err := processor.Process(context.Background(), writeTestInput(t, testWAVBytes(10)), filepath.Join(t.TempDir(), "out.wav"), PresetDefault)
		assertProcessingCode(t, err, "media_duration_exceeded")
	})
	t.Run("multiple_streams", func(t *testing.T) {
		runner := newFakeCommandRunner()
		runner.probeStreams = 2
		processor := newProcessorForTest(runner, DefaultLimits())
		_, err := processor.Process(context.Background(), writeTestInput(t, testWAVBytes(10)), filepath.Join(t.TempDir(), "out.wav"), PresetDefault)
		assertProcessingCode(t, err, "media_stream_layout_unsupported")
	})
	t.Run("probe_timeout", func(t *testing.T) {
		runner := newFakeCommandRunner()
		runner.probeBlock = true
		limits := DefaultLimits()
		limits.ProbeTimeout = 5 * time.Millisecond
		processor := newProcessorForTest(runner, limits)
		_, err := processor.Process(context.Background(), writeTestInput(t, testWAVBytes(10)), filepath.Join(t.TempDir(), "out.wav"), PresetDefault)
		assertProcessingCode(t, err, "ffprobe_timeout")
	})
	t.Run("worker_crash", func(t *testing.T) {
		runner := newFakeCommandRunner()
		runner.transcodeError = errors.New("worker crashed with private diagnostics")
		processor := newProcessorForTest(runner, DefaultLimits())
		output := filepath.Join(t.TempDir(), "out.wav")
		_, err := processor.Process(context.Background(), writeTestInput(t, testWAVBytes(10)), output, PresetDefault)
		assertProcessingCode(t, err, "ffmpeg_failed")
		if _, statErr := os.Stat(output); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("partial worker output stat=%v", statErr)
		}
	})
	t.Run("invalid_loudness", func(t *testing.T) {
		runner := newFakeCommandRunner()
		runner.loudness = `{"input_i":"nan"}`
		processor := newProcessorForTest(runner, DefaultLimits())
		_, err := processor.Process(context.Background(), writeTestInput(t, testWAVBytes(10)), filepath.Join(t.TempDir(), "out.wav"), PresetDefault)
		assertProcessingCode(t, err, "loudness_invalid")
	})
	t.Run("output_cap", func(t *testing.T) {
		runner := newFakeCommandRunner()
		runner.output = testWAVBytes(100)
		limits := DefaultLimits()
		limits.MaxOutputBytes = 100
		limits.WorkerOutputBytes = 100
		processor := newProcessorForTest(runner, limits)
		_, err := processor.Process(context.Background(), writeTestInput(t, testWAVBytes(10)), filepath.Join(t.TempDir(), "out.wav"), PresetDefault)
		assertProcessingCode(t, err, "canonical_output_oversized")
	})
}

func TestSignatureProbeRecognizesSupportedContainersWithExactFraming(t *testing.T) {
	tests := []struct {
		name   string
		raw    []byte
		format Format
	}{
		{"wav", testWAVBytes(1), FormatWAV},
		{"mp3", testMP3Bytes(1), FormatMP3},
		{"mp3_id3", append([]byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0}, testMP3Bytes(1)...), FormatMP3},
		{"m4a", testM4ABytes(), FormatM4A},
		{"aac_adts", testADTSBytes(), FormatAAC},
		{"ogg", testOggBytes(), FormatOGG},
		{"flac", testFLACBytes(), FormatFLAC},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := writeTestInput(t, test.raw)
			format, err := detectFormat(path, int64(len(test.raw)))
			if err != nil || format != test.format {
				t.Fatalf("format=%q want=%q err=%v", format, test.format, err)
			}
		})
	}
}

func TestCaptureBuffersBoundMemoryAndPreserveTail(t *testing.T) {
	stdout := &cappedCapture{limit: 4}
	if _, err := stdout.Write([]byte("123456")); err != nil || string(stdout.data) != "1234" || !stdout.exceeded {
		t.Fatalf("stdout capture=%q exceeded=%v err=%v", stdout.data, stdout.exceeded, err)
	}
	stderr := &tailCapture{limit: 5}
	_, _ = stderr.Write([]byte("123"))
	_, _ = stderr.Write([]byte("4567"))
	if string(stderr.data) != "34567" {
		t.Fatalf("stderr tail=%q", stderr.data)
	}
}
