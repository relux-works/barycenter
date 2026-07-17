package pmcprobe

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func vectorFixture(t *testing.T) (Config, []byte, []byte, []byte, []byte) {
	t.Helper()
	var config Config
	config.Kind = MediaTrack
	config.ChunkSize = 64
	config.DurationMS = 7_200_000
	config.Epoch = 42
	for index := range config.ContainerID {
		config.ContainerID[index] = byte(0x10 + index)
	}
	for index := range config.TargetSnapshotDigest {
		config.TargetSnapshotDigest[index] = byte(0x40 + index)
		config.Salt[index] = byte(0x80 + index)
	}
	copy(config.NoncePrefix[:], []byte{0x01, 0x23, 0x45, 0x67})
	master := make([]byte, 32)
	for index := range master {
		master[index] = byte(index)
	}
	privateManifest := []byte(`{"codec":"probe-bytes","title":"synthetic"}`)
	plaintext := make([]byte, 150)
	for index := range plaintext {
		plaintext[index] = byte((index*29 + 7) % 251)
	}
	container, err := Seal(config, master, privateManifest, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	return config, master, privateManifest, plaintext, container
}

func TestProbeContainerDeterministicVectorAndRoundTrip(t *testing.T) {
	config, master, privateManifest, plaintext, container := vectorFixture(t)
	digest := sha256.Sum256(container)
	const expected = "1ed44e2c5e5739c97840d2d82ccb6582e16647686159a85578b0516eb74398b8"
	if got := hex.EncodeToString(digest[:]); got != expected {
		t.Fatalf("container sha256=%s", got)
	}
	header, err := DecodeHeader(container)
	if err != nil {
		t.Fatal(err)
	}
	if header.ChunkCount != 3 || header.ChunkSize != config.ChunkSize || header.PlaintextSize != uint64(len(plaintext)) || header.DurationMS != config.DurationMS || header.Epoch != config.Epoch {
		t.Fatalf("header=%+v", header)
	}
	openedPrivate, err := OpenPrivateManifest(container, master)
	if err != nil || !bytes.Equal(openedPrivate, privateManifest) {
		t.Fatalf("private=%q err=%v", openedPrivate, err)
	}
	opened, err := OpenAll(container, master)
	if err != nil || !bytes.Equal(opened, plaintext) {
		t.Fatalf("roundtrip bytes=%d err=%v", len(opened), err)
	}
}

func TestProbeContainerIndependentRangeAndResumeBoundaries(t *testing.T) {
	_, master, _, plaintext, container := vectorFixture(t)
	header, err := DecodeHeader(container)
	if err != nil {
		t.Fatal(err)
	}
	middleRange, err := ChunkRange(header, 1)
	if err != nil {
		t.Fatal(err)
	}
	middle, err := OpenChunk(container, master, 1)
	if err != nil || !bytes.Equal(middle, plaintext[64:128]) {
		t.Fatalf("middle=%x err=%v", middle, err)
	}
	if boundary, err := ResumeBoundary(container, middleRange.Offset+middleRange.Length-1); err != nil || boundary != middleRange.Offset {
		t.Fatalf("partial boundary=%d err=%v", boundary, err)
	}
	if boundary, err := ResumeBoundary(container, middleRange.Offset+middleRange.Length); err != nil || boundary != middleRange.Offset+middleRange.Length {
		t.Fatalf("complete boundary=%d err=%v", boundary, err)
	}
	if _, err := ChunkRange(header, header.ChunkCount); !errors.Is(err, ErrMalformedContainer) {
		t.Fatalf("out-of-range error=%v", err)
	}
}

func TestProbeContainerTamperTruncationReorderAndSubstitutionFailClosed(t *testing.T) {
	config, master, _, _, container := vectorFixture(t)
	header, err := DecodeHeader(container)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := ChunkRange(header, 0)
	second, _ := ChunkRange(header, 1)

	tests := map[string]func([]byte){
		"header-target": func(raw []byte) { raw[64] ^= 0x01 },
		"manifest-tag":  func(raw []byte) { raw[HeaderSize] ^= 0x01 },
		"private":       func(raw []byte) { raw[HeaderSize+ManifestTagSize] ^= 0x01 },
		"chunk":         func(raw []byte) { raw[first.Offset] ^= 0x01 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := append([]byte(nil), container...)
			mutate(changed)
			if _, err := OpenAll(changed, master); err == nil {
				t.Fatal("tampered container opened")
			}
		})
	}

	t.Run("truncate", func(t *testing.T) {
		if _, err := OpenAll(container[:len(container)-1], master); !errors.Is(err, ErrMalformedContainer) {
			t.Fatalf("truncate error=%v", err)
		}
	})

	t.Run("reorder", func(t *testing.T) {
		changed := append([]byte(nil), container...)
		firstBytes := append([]byte(nil), changed[first.Offset:first.Offset+first.Length]...)
		secondBytes := append([]byte(nil), changed[second.Offset:second.Offset+second.Length]...)
		copy(changed[first.Offset:first.Offset+first.Length], secondBytes)
		copy(changed[second.Offset:second.Offset+second.Length], firstBytes)
		if _, err := OpenAll(changed, master); err == nil {
			t.Fatal("reordered chunks opened")
		}
	})

	t.Run("substitute-other-container", func(t *testing.T) {
		otherConfig := config
		otherConfig.ContainerID[0] ^= 0xff
		other, err := Seal(otherConfig, master, []byte(`{"codec":"probe-bytes","title":"synthetic"}`), make([]byte, 150))
		if err != nil {
			t.Fatal(err)
		}
		otherHeader, _ := DecodeHeader(other)
		otherFirst, _ := ChunkRange(otherHeader, 0)
		changed := append([]byte(nil), container...)
		copy(changed[first.Offset:first.Offset+first.Length], other[otherFirst.Offset:otherFirst.Offset+otherFirst.Length])
		if _, err := OpenChunk(changed, master, 0); err == nil {
			t.Fatal("foreign chunk opened")
		}
	})

	t.Run("replay-stale-epoch", func(t *testing.T) {
		staleConfig := config
		staleConfig.Epoch--
		stale, err := Seal(staleConfig, master, []byte(`{"codec":"probe-bytes","title":"synthetic"}`), make([]byte, 150))
		if err != nil {
			t.Fatal(err)
		}
		staleHeader, _ := DecodeHeader(stale)
		staleFirst, _ := ChunkRange(staleHeader, 0)
		changed := append([]byte(nil), container...)
		copy(changed[first.Offset:first.Offset+first.Length], stale[staleFirst.Offset:staleFirst.Offset+staleFirst.Length])
		if _, err := OpenChunk(changed, master, 0); err == nil {
			t.Fatal("stale-epoch chunk opened")
		}
	})
}

func TestProbeContainerRejectsUnsafeBoundsAndWrongKey(t *testing.T) {
	config, master, privateManifest, plaintext, container := vectorFixture(t)
	config.ChunkSize = MaximumChunkBytes + 1
	if _, err := Seal(config, master, privateManifest, plaintext); !errors.Is(err, ErrUnsupportedContract) {
		t.Fatalf("oversize chunk error=%v", err)
	}
	config.ChunkSize = 64
	if _, err := Seal(config, master[:31], privateManifest, plaintext); !errors.Is(err, ErrUnsupportedContract) {
		t.Fatalf("short key error=%v", err)
	}
	wrong := append([]byte(nil), master...)
	wrong[0] ^= 0xff
	if _, err := OpenAll(container, wrong); !errors.Is(err, ErrAuthentication) {
		t.Fatalf("wrong key error=%v", err)
	}
}

func TestProbeContainerDurationMetadataAndBoundedOverhead(t *testing.T) {
	config, master, privateManifest, _, _ := vectorFixture(t)
	config.ChunkSize = MaximumChunkBytes
	plaintext := make([]byte, 4*MaximumChunkBytes)
	oneHour := config
	oneHour.DurationMS = 3_600_000
	twoHours := config
	twoHours.DurationMS = 7_200_000
	one, err := Seal(oneHour, master, privateManifest, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	two, err := Seal(twoHours, master, privateManifest, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if len(one) != len(two) {
		t.Fatalf("duration changed container size: one=%d two=%d", len(one), len(two))
	}
	overhead := len(two) - len(plaintext)
	if overhead > 512 {
		t.Fatalf("four-chunk overhead=%d", overhead)
	}
	if bytes.Equal(one, two) {
		t.Fatal("authenticated duration did not change ciphertext")
	}
}
