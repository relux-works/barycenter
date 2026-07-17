package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	pmcprobe "relux.works/duet/e2ee-container-probe"
)

type report struct {
	Contract       string `json:"contract"`
	Result         string `json:"result"`
	GOOS           string `json:"goos"`
	GOARCH         string `json:"goarch"`
	GoVersion      string `json:"goVersion"`
	DurationMS     uint64 `json:"durationMs"`
	InputBytes     int    `json:"inputBytes"`
	ContainerBytes int    `json:"containerBytes"`
	OverheadBytes  int    `json:"overheadBytes"`
	ChunkCount     uint32 `json:"chunkCount"`
	SealMicros     int64  `json:"sealMicros"`
	OpenMicros     int64  `json:"openMicros"`
	HeapDeltaBytes int64  `json:"heapDeltaBytes"`
	ManualEvidence string `json:"manualEvidence"`
}

func fillRandom(values ...[]byte) error {
	for _, value := range values {
		if _, err := rand.Read(value); err != nil {
			return err
		}
	}
	return nil
}

func run() (report, error) {
	const inputBytes = 2 * pmcprobe.MaximumChunkBytes
	plaintext := make([]byte, inputBytes)
	for index := range plaintext {
		plaintext[index] = byte((index*17 + 11) % 251)
	}
	master := make([]byte, 32)
	config := pmcprobe.Config{
		Kind:       pmcprobe.MediaTrack,
		ChunkSize:  pmcprobe.MaximumChunkBytes,
		DurationMS: 7_200_000,
		Epoch:      1,
	}
	if err := fillRandom(master, config.ContainerID[:], config.TargetSnapshotDigest[:], config.Salt[:], config.NoncePrefix[:]); err != nil {
		return report{}, err
	}
	privateManifest := []byte(`{"codec":"unselected","fixture":"synthetic"}`)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	sealStart := time.Now()
	container, err := pmcprobe.Seal(config, master, privateManifest, plaintext)
	sealDuration := time.Since(sealStart)
	if err != nil {
		return report{}, err
	}
	openStart := time.Now()
	opened, err := pmcprobe.OpenAll(container, master)
	openDuration := time.Since(openStart)
	if err != nil || !bytes.Equal(opened, plaintext) {
		return report{}, fmt.Errorf("roundtrip failed: %w", err)
	}
	header, err := pmcprobe.DecodeHeader(container)
	if err != nil {
		return report{}, err
	}
	runtime.ReadMemStats(&after)
	result := report{
		Contract:       "pmc-probe-v1",
		Result:         "repository-experiment-pass-production-no-go",
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		GoVersion:      runtime.Version(),
		DurationMS:     config.DurationMS,
		InputBytes:     len(plaintext),
		ContainerBytes: len(container),
		OverheadBytes:  len(container) - len(plaintext),
		ChunkCount:     header.ChunkCount,
		SealMicros:     sealDuration.Microseconds(),
		OpenMicros:     openDuration.Microseconds(),
		HeapDeltaBytes: int64(after.Alloc) - int64(before.Alloc),
		ManualEvidence: "not-run",
	}
	clear(master)
	clear(plaintext)
	clear(opened)
	runtime.KeepAlive(container)
	return result, nil
}

func main() {
	result, err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
