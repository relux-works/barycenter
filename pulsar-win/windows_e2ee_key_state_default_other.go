//go:build !windows

package main

func newDefaultWindowsE2EEKeyStateRepository(string) (*WindowsE2EEKeyStateRepository, error) {
	return nil, ErrWindowsE2EEUnavailable
}
