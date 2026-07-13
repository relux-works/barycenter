//go:build !windows

package main

import (
	"errors"
	"testing"
)

func TestNonWindowsDefaultsNeverCreatePlaintextFallbacks(t *testing.T) {
	if _, err := newDefaultCredentialRepository(t.TempDir()); !errors.Is(err, errCredentialStorageUnavailable) {
		t.Fatalf("credential default error=%v", err)
	}
	if _, err := NewRecoveryClipboard(1, &directTestDispatcher{}); !errors.Is(err, errClipboardUnsupported) {
		t.Fatalf("clipboard default error=%v", err)
	}
}
