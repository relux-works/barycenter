//go:build linux

package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestLinuxWorkerStartsBehindKernelLimitBarrier(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := runLimitedCommand(ctx, commandSpec{
		Tool:        "/bin/sh",
		Args:        []string{"-c", "ulimit -t; ulimit -n; ulimit -f"},
		StdoutLimit: 1024, StderrLimit: 1024,
		CPUSeconds: 3, MemoryBytes: 256 << 20, OpenFiles: 32, FileBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Fields(string(result.Stdout))
	if len(lines) != 3 || lines[0] != "3" || lines[1] != "32" {
		t.Fatalf("worker soft limits=%q", result.Stdout)
	}
	fileBlocks, err := strconv.ParseUint(lines[2], 10, 64)
	if err != nil || fileBlocks == 0 || fileBlocks > 8 {
		t.Fatalf("worker file-size blocks=%q err=%v", lines[2], err)
	}
}

func TestLinuxWorkerKernelFileCapStopsOversizedOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worker-output.bin")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := runLimitedCommand(ctx, commandSpec{
		Tool:        "/bin/sh",
		Args:        []string{"-c", "dd if=/dev/zero of=\"$1\" bs=1024 count=64 status=none", "worker", path},
		StdoutLimit: 1024, StderrLimit: 1024,
		CPUSeconds: 3, MemoryBytes: 256 << 20, OpenFiles: 32, FileBytes: 4096,
	})
	if err == nil {
		t.Fatal("kernel file-size cap allowed oversized worker output")
	}
	info, statErr := os.Stat(path)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal(statErr)
	}
	if statErr == nil && info.Size() > 4096 {
		t.Fatalf("worker output size=%d exceeds kernel cap", info.Size())
	}
}
