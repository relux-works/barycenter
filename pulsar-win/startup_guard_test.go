package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestGuardUnpairedShellStartupPassesThroughSuccess(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	shown := false

	paired, supported := guardUnpairedShellStartup(logger, func() { shown = true }, func() (bool, bool) {
		return true, true
	})

	if !paired || !supported {
		t.Fatalf("guard result = (%v, %v), want (true, true)", paired, supported)
	}
	if shown || output.Len() != 0 {
		t.Fatalf("successful startup produced fatal output: shown=%v log=%q", shown, output.String())
	}
}

func TestGuardUnpairedShellStartupPersistsAndSurfacesPanic(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	shown := 0

	paired, supported := guardUnpairedShellStartup(logger, func() { shown++ }, func() (bool, bool) {
		panic("render exploded")
	})

	if paired || !supported {
		t.Fatalf("guard result = (%v, %v), want (false, true)", paired, supported)
	}
	if shown != 1 {
		t.Fatalf("fatal surface count = %d, want 1", shown)
	}
	logText := output.String()
	for _, required := range []string{"unpaired shell startup panic", "render exploded", "goroutine"} {
		if !strings.Contains(logText, required) {
			t.Fatalf("panic log %q does not contain %q", logText, required)
		}
	}
}
