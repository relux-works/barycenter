//go:build windows

package main

import (
	"testing"
	"unsafe"
)

// Paired zero-length assertions fail compilation if the x64 SDK layout drifts
// in either direction; the runtime test below supplies readable diagnostics.
var (
	_ [976 - unsafe.Sizeof(notifyIconData{})]byte
	_ [unsafe.Sizeof(notifyIconData{}) - 976]byte
	_ [40 - unsafe.Offsetof(notifyIconData{}.szTip)]byte
	_ [unsafe.Offsetof(notifyIconData{}.szTip) - 40]byte
	_ [968 - unsafe.Offsetof(notifyIconData{}.hBalloonIcon)]byte
	_ [unsafe.Offsetof(notifyIconData{}.hBalloonIcon) - 968]byte
)

func TestNotifyIconDataWLayoutX64(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("probe package is x64-only")
	}
	var value notifyIconData
	if got, want := unsafe.Sizeof(value), uintptr(976); got != want {
		t.Fatalf("NOTIFYICONDATAW size = %d, want %d", got, want)
	}
	if got, want := unsafe.Offsetof(value.szTip), uintptr(40); got != want {
		t.Fatalf("szTip offset = %d, want %d", got, want)
	}
	if got, want := unsafe.Offsetof(value.hBalloonIcon), uintptr(968); got != want {
		t.Fatalf("hBalloonIcon offset = %d, want %d", got, want)
	}
}
