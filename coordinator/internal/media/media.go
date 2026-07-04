// Package media processes Telegram voice messages into broadcast-ready WAVs
// (spec ch. 10): highpass + compressor + single-pass loudnorm to I=-14 LUFS,
// output s16le 44100 stereo. ffmpeg/ffprobe are external binaries.
package media

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type Preset string

const (
	PresetDefault Preset = "default"
	PresetRadio   Preset = "radio"
)

type Result struct {
	WAVPath      string
	DurationMS   int64
	LoudnormJSON string
}

func filterChain(p Preset) string {
	base := "highpass=f=90, acompressor=threshold=-20dB:ratio=3:attack=10:release=180:makeup=4"
	if p == PresetRadio {
		// Spec 10.2: presence EQ + light de-esser, off by default (phase 2 taste).
		base += ", equalizer=f=4500:t=q:w=1:g=2, deesser"
	}
	return base + ", loudnorm=I=-14:TP=-1.5:LRA=11:print_format=json"
}

// Process converts inPath (ogg/opus from Telegram) to a normalized WAV.
func Process(inPath, outPath string, preset Preset) (Result, error) {
	args := []string{
		"-y", "-i", inPath,
		"-af", filterChain(preset),
		"-ar", "44100", "-ac", "2", "-c:a", "pcm_s16le",
		outPath,
	}
	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return Result{}, fmt.Errorf("ffmpeg failed: %w\n%s", err, tail(stderr.String(), 1200))
	}

	loudnorm := extractLastJSON(stderr.String())
	if loudnorm == "" {
		return Result{}, fmt.Errorf("ffmpeg succeeded but no loudnorm json found in output")
	}

	durMS, err := probeDurationMS(outPath)
	if err != nil {
		return Result{}, err
	}
	return Result{WAVPath: outPath, DurationMS: durMS, LoudnormJSON: loudnorm}, nil
}

// extractLastJSON pulls the trailing { ... } block loudnorm prints to stderr.
func extractLastJSON(s string) string {
	end := strings.LastIndex(s, "}")
	if end < 0 {
		return ""
	}
	depth := 0
	for i := end; i >= 0; i-- {
		switch s[i] {
		case '}':
			depth++
		case '{':
			depth--
			if depth == 0 {
				return s[i : end+1]
			}
		}
	}
	return ""
}

func probeDurationMS(path string) (int64, error) {
	out, err := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed on %s: %w", path, err)
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("ffprobe duration unparseable: %q", out)
	}
	return int64(seconds * 1000), nil
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
