//go:build windows

package main

import (
	"errors"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/windows"
)

const (
	modNoRepeat                                = 0x4000
	errorHotKeyAlreadyRegistered syscall.Errno = 1409
)

var (
	pRegisterHotKey   = user32.NewProc("RegisterHotKey")
	pUnregisterHotKey = user32.NewProc("UnregisterHotKey")
)

type win32RecordingShortcutRegistrar struct {
	hwnd windows.Handle
}

var win32ShortcutRegistrationID atomic.Uint32

func (r *win32RecordingShortcutRegistrar) Register(shortcut WindowsRecordingShortcut) (WindowsShortcutRegistration, error) {
	if r == nil || r.hwnd == 0 || !shortcut.Valid() {
		return 0, ErrWindowsShortcutUnavailable
	}
	next := WindowsShortcutRegistration(win32ShortcutRegistrationID.Add(1))
	if next == 0 {
		next = WindowsShortcutRegistration(win32ShortcutRegistrationID.Add(1))
	}
	result, _, callErr := pRegisterHotKey.Call(
		uintptr(r.hwnd), uintptr(next), uintptr(shortcut.Modifiers|modNoRepeat), uintptr(shortcut.VirtualKey))
	if result == 0 {
		if errors.Is(callErr, errorHotKeyAlreadyRegistered) {
			return 0, ErrWindowsShortcutConflict
		}
		return 0, errors.Join(ErrWindowsShortcutUnavailable, callErr)
	}
	return next, nil
}

func (r *win32RecordingShortcutRegistrar) Unregister(registration WindowsShortcutRegistration) error {
	if r == nil || r.hwnd == 0 || registration == 0 {
		return ErrWindowsShortcutUnavailable
	}
	result, _, callErr := pUnregisterHotKey.Call(uintptr(r.hwnd), uintptr(registration))
	if result == 0 {
		return errors.Join(ErrWindowsShortcutUnavailable, callErr)
	}
	return nil
}
