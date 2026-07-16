package automation

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBuiltinRecordingCueWAVMatchesReviewedAsset(t *testing.T) {
	generated, err := BuiltinRecordingCueWAV()
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := os.ReadFile(filepath.Join("..", "..", "..", "assets", "audio", "pulsar-recording-cue.wav"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated, reviewed) {
		t.Fatalf("generated builtin differs: generated=%d reviewed=%d", len(generated), len(reviewed))
	}
}
