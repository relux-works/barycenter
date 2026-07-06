//go:build !windows

// Non-Windows stub so the package builds and the portable unit tests run on
// darwin/linux CI. The real implementation lives in audio_windows.go.
package main

import (
	"errors"
	"log/slog"
)

func startAudio(pipeName string, ring *Ring, player *Player, log *slog.Logger, stop <-chan struct{}) error {
	return errors.New("audio pipeline is Windows-only in this build (stub)")
}
