//go:build !windows

package main

import "log/slog"

type unavailableWindowsAudioOutput struct{}

func (unavailableWindowsAudioOutput) Snapshot() ([]WindowsAudioOutput, int) { return nil, 0 }
func (unavailableWindowsAudioOutput) SelectNext()                           {}
func (unavailableWindowsAudioOutput) Close()                                {}
func newWindowsAudioOutputController(*Engine, *Player, *slog.Logger) WindowsAudioOutputControl {
	return unavailableWindowsAudioOutput{}
}
