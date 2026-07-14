package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// This is synthetic worker evidence, not manual application or hardware
// acceptance. CI installs the same ffmpeg package class as the production
// image; developer machines without the tools skip it.
func TestProcessorLiveSupportedFormatMatrix(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
	tests := []struct {
		name      string
		extension string
		format    Format
		codecArgs []string
	}{
		{name: "wav", extension: ".wav", format: FormatWAV, codecArgs: []string{"-c:a", "pcm_s16le", "-f", "wav"}},
		{name: "mp3", extension: ".mp3", format: FormatMP3, codecArgs: []string{"-c:a", "libmp3lame", "-f", "mp3"}},
		{name: "m4a_aac", extension: ".m4a", format: FormatM4A, codecArgs: []string{"-c:a", "aac", "-f", "ipod"}},
		{name: "aac_adts", extension: ".aac", format: FormatAAC, codecArgs: []string{"-c:a", "aac", "-f", "adts"}},
		{name: "ogg_opus", extension: ".ogg", format: FormatOGG, codecArgs: []string{"-c:a", "libopus", "-f", "ogg"}},
		{name: "flac", extension: ".flac", format: FormatFLAC, codecArgs: []string{"-c:a", "flac", "-f", "flac"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			input := filepath.Join(directory, "source"+test.extension)
			arguments := []string{
				"-v", "error", "-nostdin", "-y", "-f", "lavfi",
				"-i", "sine=frequency=440:sample_rate=48000:duration=1",
				"-af", "volume=0.2", "-ac", "1", "-ar", "48000",
			}
			arguments = append(arguments, test.codecArgs...)
			arguments = append(arguments, input)
			if output, err := exec.Command("ffmpeg", arguments...).CombinedOutput(); err != nil {
				t.Fatalf("generate %s fixture: %v\n%s", test.name, err, output)
			}
			processor, err := NewProcessor()
			if err != nil {
				t.Fatal(err)
			}
			canonical := filepath.Join(directory, "canonical.wav")
			result, err := processor.Process(context.Background(), input, canonical, PresetDefault)
			if err != nil {
				t.Fatalf("process %s: %v", test.name, err)
			}
			if result.InputFormat != test.format || result.DurationMS < 900 || result.DurationMS > 1100 ||
				result.SizeBytes <= 44 || len(result.SHA256) != 64 || !validLoudnessJSON(result.LoudnormJSON) {
				t.Fatalf("%s canonical result=%+v", test.name, result)
			}
			info, err := os.Stat(canonical)
			if err != nil || info.Size() != result.SizeBytes || info.Mode().Perm()&0o077 != 0 {
				t.Fatalf("%s canonical info=%+v err=%v", test.name, info, err)
			}
		})
	}
}
