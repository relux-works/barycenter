package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
)

type failingLogWriter struct{}

func (failingLogWriter) Write([]byte) (int, error) {
	return 0, errors.New("stderr handle is unavailable")
}

func TestBestEffortWriterPersistsAfterUnavailableGUIStderr(t *testing.T) {
	var file bytes.Buffer
	writer := bestEffortWriter{failingLogWriter{}, &file}

	n, err := writer.Write([]byte("startup failure\n"))
	if err != nil || n != len("startup failure\n") {
		t.Fatalf("Write = (%d, %v), want (%d, nil)", n, err, len("startup failure\n"))
	}
	if got := file.String(); got != "startup failure\n" {
		t.Fatalf("durable sink = %q, want startup record", got)
	}
}

func TestBestEffortWriterReturnsFailureWhenEverySinkFails(t *testing.T) {
	n, err := (bestEffortWriter{failingLogWriter{}, failingLogWriter{}}).Write([]byte("record"))
	if n != 0 || err == nil {
		t.Fatalf("Write = (%d, %v), want (0, error)", n, err)
	}
}

func TestConfigureCrashOutputCreatesDurableSink(t *testing.T) {
	dir := t.TempDir()
	if err := configureCrashOutput(dir); err != nil {
		t.Fatalf("configureCrashOutput: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "pulsar-crash.log"))
	if err != nil {
		t.Fatalf("crash log: %v", err)
	}
	if info.IsDir() {
		t.Fatal("crash log is a directory")
	}
	if err := debug.SetCrashOutput(nil, debug.CrashOptions{}); err != nil {
		t.Fatalf("disable crash output: %v", err)
	}
}
