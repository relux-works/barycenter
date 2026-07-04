package media

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// Live pipeline test on a synthetic opus voice file — runs wherever ffmpeg
// exists (dev Mac, VPS); skips otherwise.
func TestProcessLive(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	dir := t.TempDir()
	ogg := filepath.Join(dir, "in.oga")
	wav := filepath.Join(dir, "out.wav")

	// 3 s of tone as a stand-in for a voice message (quiet, forces makeup gain).
	gen := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "sine=frequency=220:duration=3",
		"-af", "volume=0.05",
		"-c:a", "libopus", ogg)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("generate test opus: %v\n%s", err, out)
	}

	res, err := Process(ogg, wav, PresetDefault)
	if err != nil {
		t.Fatal(err)
	}

	if res.DurationMS < 2800 || res.DurationMS > 3400 {
		t.Fatalf("duration %d ms, want ~3000", res.DurationMS)
	}

	var report map[string]any
	if err := json.Unmarshal([]byte(res.LoudnormJSON), &report); err != nil {
		t.Fatalf("loudnorm json unparseable: %v\n%s", err, res.LoudnormJSON)
	}
	if _, ok := report["input_i"]; !ok {
		t.Fatalf("loudnorm report lacks input_i: %s", res.LoudnormJSON)
	}

	// Output format contract: s16le 44100 stereo (spec 10.2).
	probe := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "stream=codec_name,sample_rate,channels",
		"-of", "default=noprint_wrappers=1", wav)
	out, err := probe.Output()
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	for _, want := range []string{"codec_name=pcm_s16le", "sample_rate=44100", "channels=2"} {
		if !contains(got, want) {
			t.Fatalf("wav format wrong, want %s in:\n%s", want, got)
		}
	}
}

// DoD-6: real speech through the default preset must land at I = -14 LUFS
// +-2 LU (spec 10.2). Speech synthesized by macOS `say`, packed as opus the
// way Telegram delivers voice, then measured on the pipeline output.
func TestLoudnessTargetOnSpeech(t *testing.T) {
	for _, tool := range []string{"ffmpeg", "say"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
	dir := t.TempDir()
	aiff := filepath.Join(dir, "speech.aiff")
	ogg := filepath.Join(dir, "in.oga")
	wav := filepath.Join(dir, "out.wav")

	speech := exec.Command("say", "-o", aiff,
		"Testing the duet voice pipeline. This message should come out at broadcast loudness, close to the music around it.")
	if out, err := speech.CombinedOutput(); err != nil {
		t.Skipf("say failed (headless session?): %v %s", err, out)
	}

	// Telegram voice = mono opus in .oga; -volume 0.3 mimics a quietish mic.
	pack := exec.Command("ffmpeg", "-y", "-i", aiff,
		"-af", "volume=0.3", "-ac", "1", "-ar", "48000", "-c:a", "libopus", ogg)
	if out, err := pack.CombinedOutput(); err != nil {
		t.Fatalf("pack opus: %v\n%s", err, out)
	}

	res, err := Process(ogg, wav, PresetDefault)
	if err != nil {
		t.Fatal(err)
	}

	// Measure the OUTPUT's integrated loudness with an independent pass.
	measure := exec.Command("ffmpeg", "-hide_banner", "-i", wav,
		"-af", "loudnorm=print_format=json", "-f", "null", "-")
	var stderr bytes.Buffer
	measure.Stderr = &stderr
	if err := measure.Run(); err != nil {
		t.Fatalf("measure: %v\n%s", err, stderr.String())
	}
	var report struct {
		InputI string `json:"input_i"`
	}
	if err := json.Unmarshal([]byte(extractLastJSON(stderr.String())), &report); err != nil {
		t.Fatalf("measure json: %v\n%s", err, stderr.String())
	}
	i, err := strconv.ParseFloat(report.InputI, 64)
	if err != nil {
		t.Fatalf("input_i unparseable: %q", report.InputI)
	}
	t.Logf("pipeline output integrated loudness: %.2f LUFS (target -14 +-2)", i)
	if i < -16 || i > -12 {
		t.Fatalf("output loudness %.2f LUFS outside -14 +-2 LU (spec 10.2, goal DoD-6); pipeline loudnorm report: %s", i, res.LoudnormJSON)
	}
}

func TestExtractLastJSON(t *testing.T) {
	stderr := "noise\n[Parsed_loudnorm_0 @ 0x123]\n{\n\t\"input_i\" : \"-33.06\",\n\t\"target_offset\" : \"0.5\"\n}\n"
	got := extractLastJSON(stderr)
	var m map[string]string
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("bad extraction %q: %v", got, err)
	}
	if m["input_i"] != "-33.06" {
		t.Fatalf("m = %v", m)
	}
	if extractLastJSON("no json here") != "" {
		t.Fatal("must return empty on no-json input")
	}
}

func TestFilterChainPresets(t *testing.T) {
	def := filterChain(PresetDefault)
	if contains(def, "equalizer") || contains(def, "deesser") {
		t.Fatalf("default preset must not include radio filters: %s", def)
	}
	radio := filterChain(PresetRadio)
	if !contains(radio, "equalizer=f=4500") || !contains(radio, "deesser") {
		t.Fatalf("radio preset incomplete: %s", radio)
	}
	for _, chain := range []string{def, radio} {
		if !contains(chain, "loudnorm=I=-14:TP=-1.5:LRA=11") {
			t.Fatalf("loudnorm target wrong: %s", chain)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}

var _ = strconv.Itoa // keep import if asserts change
