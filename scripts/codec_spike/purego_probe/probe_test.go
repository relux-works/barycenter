package puregoprobe

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hajimehoshi/go-mp3"
	"github.com/pion/opus/pkg/oggreader"
)

func fixtureDirectory(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "..", "acceptance", "codec-spike", "fixtures", "smoke-v1"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBoundedPCMQueue(t *testing.T) {
	queue := NewPCMQueue(8)
	if got := queue.Push([]byte("abcdefghijkl")); got != 8 {
		t.Fatalf("push=%d", got)
	}
	if queue.Used() != 8 {
		t.Fatalf("used=%d", queue.Used())
	}
	out := make([]byte, 5)
	if got := queue.Drain(out); got != 5 || string(out) != "abcde" {
		t.Fatalf("drain=%d %q", got, out)
	}
	if got := queue.Push([]byte("XYZ")); got != 3 {
		t.Fatalf("wrap push=%d", got)
	}
	out = make([]byte, 6)
	if got := queue.Drain(out); got != 6 || string(out) != "fghXYZ" {
		t.Fatalf("wrap drain=%d %q", got, out)
	}
}

func TestFrozenResearchResultIsConcreteReject(t *testing.T) {
	evidence := Run(fixtureDirectory(t))
	if evidence.Passed || evidence.ShippingDecision != "rejected-license-seek-and-manual-evidence-gates" {
		t.Fatalf("unexpected decision: %+v", evidence)
	}
	if evidence.DecoderOwnsNetwork || evidence.RenderThreadIO {
		t.Fatal("unsafe build posture")
	}
	if len(evidence.Fixtures) != 6 {
		t.Fatalf("fixtures=%d", len(evidence.Fixtures))
	}
	for _, item := range evidence.Fixtures {
		if item.SourceBytes == 0 {
			t.Fatalf("%s missing source", item.ID)
		}
		if item.Reads.MaximumRead > MaximumReadBytes {
			t.Fatalf("%s read=%d", item.ID, item.Reads.MaximumRead)
		}
		switch item.Codec {
		case "mp3":
			if item.Outcome != "decode-rejected-seek-full-scan" || !item.IncrementalFirstPCM || !item.SeekRequiresFullScan {
				t.Fatalf("mp3 evidence: %+v", item)
			}
		case "opus":
			if item.Outcome != "decode-rejected-no-random-seek" || !item.IncrementalFirstPCM || item.SeekSupported {
				t.Fatalf("opus evidence: %+v", item)
			}
		case "aac-lc":
			if item.Outcome != "reject-forbidden-module" {
				t.Fatalf("aac evidence: %+v", item)
			}
		}
	}
}

func TestHostileInputsReturnWithoutPanic(t *testing.T) {
	directory := fixtureDirectory(t)
	mp3Data, err := os.ReadFile(filepath.Join(directory, "mp3_cbr_12s.mp3"))
	if err != nil {
		t.Fatal(err)
	}
	oggData, err := os.ReadFile(filepath.Join(directory, "opus_ogg_vbr_12s.ogg"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		run  func() error
	}{
		{"mp3-truncated", func() error { _, err := mp3.NewDecoder(bytes.NewReader(mp3Data[:8])); return err }},
		{"mp3-bitflip", func() error {
			mutated := append([]byte(nil), mp3Data...)
			mutated[len(mutated)/2] ^= 0x5a
			decoder, err := mp3.NewDecoder(bytes.NewReader(mutated))
			if err != nil {
				return err
			}
			_, err = io.Copy(io.Discard, decoder)
			return err
		}},
		{"ogg-truncated", func() error { _, _, err := oggreader.NewWith(bytes.NewReader(oggData[:31])); return err }},
		{"ogg-bad-crc", func() error {
			mutated := append([]byte(nil), oggData...)
			mutated[22] ^= 0xff
			_, _, err := oggreader.NewWith(bytes.NewReader(mutated))
			return err
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("panic: %v", recovered)
				}
			}()
			_ = test.run()
		})
	}
}

func TestConcurrentDecodeRaceSurface(t *testing.T) {
	directory := fixtureDirectory(t)
	var wait sync.WaitGroup
	errorsFound := make(chan error, 8)
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			item := decodeOpus(t.Context(), "opus", filepath.Join(directory, "opus_ogg_cbr_12s.ogg"))
			if item.Outcome != "decode-rejected-no-random-seek" {
				errorsFound <- errors.New(item.Outcome)
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatal(err)
	}
}

func TestForbiddenAACModuleIsAbsent(t *testing.T) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "go-aac") || strings.Contains(string(data), "cgo") {
		t.Fatal("forbidden dependency")
	}
}
