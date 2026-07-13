//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	clipboardFormatUnicodeText = 13
	globalMemoryMoveable       = 0x0002
)

var (
	user32ClipboardDLL             = windows.NewLazySystemDLL("user32.dll")
	openClipboardProc              = user32ClipboardDLL.NewProc("OpenClipboard")
	closeClipboardProc             = user32ClipboardDLL.NewProc("CloseClipboard")
	emptyClipboardProc             = user32ClipboardDLL.NewProc("EmptyClipboard")
	setClipboardDataProc           = user32ClipboardDLL.NewProc("SetClipboardData")
	getClipboardDataProc           = user32ClipboardDLL.NewProc("GetClipboardData")
	isClipboardFormatAvailableProc = user32ClipboardDLL.NewProc("IsClipboardFormatAvailable")
	registerClipboardFormatProc    = user32ClipboardDLL.NewProc("RegisterClipboardFormatW")
	getClipboardSequenceNumberProc = user32ClipboardDLL.NewProc("GetClipboardSequenceNumber")
	kernel32ClipboardDLL           = windows.NewLazySystemDLL("kernel32.dll")
	globalAllocProc                = kernel32ClipboardDLL.NewProc("GlobalAlloc")
	globalFreeProc                 = kernel32ClipboardDLL.NewProc("GlobalFree")
	globalLockProc                 = kernel32ClipboardDLL.NewProc("GlobalLock")
	globalUnlockProc               = kernel32ClipboardDLL.NewProc("GlobalUnlock")
	globalSizeProc                 = kernel32ClipboardDLL.NewProc("GlobalSize")
	setLastErrorProc               = kernel32ClipboardDLL.NewProc("SetLastError")
	ntdllClipboardDLL              = windows.NewLazySystemDLL("ntdll.dll")
	rtlMoveMemoryProc              = ntdllClipboardDLL.NewProc("RtlMoveMemory")
)

type windowsRecoveryClipboardBackend struct{}

func newPlatformRecoveryClipboardBackend() (recoveryClipboardBackend, error) {
	return windowsRecoveryClipboardBackend{}, nil
}

func (windowsRecoveryClipboardBackend) Publish(owner uintptr, payload string) (clipboardPublication, error) {
	if owner == 0 {
		return clipboardPublication{}, errors.New("missing clipboard owner")
	}
	formatNames := windowsClipboardExclusionFormats
	formats := make([]uint32, len(formatNames))
	for i, name := range formatNames {
		name16, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return clipboardPublication{}, err
		}
		format, _, _ := registerClipboardFormatProc.Call(uintptr(unsafe.Pointer(name16)))
		if format == 0 {
			return clipboardPublication{}, errors.New("register clipboard exclusion format")
		}
		formats[i] = uint32(format)
	}

	zeroDWORD := make([]byte, 4)
	binary.LittleEndian.PutUint32(zeroDWORD, windowsClipboardExclusionDWORD)
	handles := make([]uintptr, 0, len(formats)+1)
	for range formats {
		handle, err := allocateClipboardBytes(zeroDWORD)
		if err != nil {
			return clipboardPublication{}, freeClipboardHandlesAfter(err, handles)
		}
		handles = append(handles, handle)
	}
	textHandle, err := allocateClipboardUTF16(payload)
	if err != nil {
		return clipboardPublication{}, freeClipboardHandlesAfter(err, handles)
	}
	handles = append(handles, textHandle)
	textIndex := len(formats)

	opened, _, _ := openClipboardProc.Call(owner)
	if opened == 0 {
		return clipboardPublication{}, freeClipboardHandlesAfter(errors.New("open clipboard"), handles)
	}
	exposed := false
	closeWith := func(publication clipboardPublication, operationErr error) (clipboardPublication, error) {
		closed, _, _ := closeClipboardProc.Call()
		if closed == 0 && operationErr == nil {
			operationErr = errors.New("close clipboard")
		}
		return publication, operationErr
	}
	sequenceBefore, _, _ := getClipboardSequenceNumberProc.Call()
	if sequenceBefore == 0 {
		return closeWith(clipboardPublication{}, freeClipboardHandlesAfter(errors.New("read clipboard sequence"), handles))
	}
	emptied, _, _ := emptyClipboardProc.Call()
	if emptied == 0 {
		return closeWith(clipboardPublication{}, freeClipboardHandlesAfter(errors.New("empty clipboard"), handles))
	}
	for i, format := range formats {
		set, _, _ := setClipboardDataProc.Call(uintptr(format), handles[i])
		if set == 0 {
			return closeWith(clipboardPublication{Changed: true}, freeClipboardHandlesAfter(errors.New("publish clipboard exclusion format"), handles[i:]))
		}
		handles[i] = 0 // ownership transferred to the clipboard
	}
	set, _, _ := setClipboardDataProc.Call(clipboardFormatUnicodeText, handles[textIndex])
	if set == 0 {
		return closeWith(clipboardPublication{Changed: true}, freeClipboardHandlesAfter(errors.New("publish clipboard text"), handles[textIndex:]))
	}
	handles[textIndex] = 0
	exposed = true
	sequence, _, _ := getClipboardSequenceNumberProc.Call()
	publication := clipboardPublication{Sequence: uint32(sequence), Changed: true, Exposed: exposed}
	if sequence == 0 || sequence == sequenceBefore {
		cleaned, _, _ := emptyClipboardProc.Call()
		if cleaned != 0 {
			publication.Exposed = false
		} else {
			publication.Sequence = 0 // unknown; retry must rebind only after an exact payload check
		}
		return closeWith(publication, errors.New("read clipboard sequence"))
	}
	return closeWith(publication, nil)
}

func (windowsRecoveryClipboardBackend) ClearIfUnchanged(owner uintptr, sequence uint32, payload string) (bool, error) {
	if owner == 0 {
		return false, errors.New("missing clipboard owner")
	}
	opened, _, _ := openClipboardProc.Call(owner)
	if opened == 0 {
		return false, errors.New("open clipboard")
	}
	closeWith := func(cleared bool, operationErr error) (bool, error) {
		closed, _, _ := closeClipboardProc.Call()
		if closed == 0 && operationErr == nil {
			operationErr = errors.New("close clipboard")
		}
		return cleared, operationErr
	}
	current, _, _ := getClipboardSequenceNumberProc.Call()
	if current == 0 {
		return closeWith(false, errors.New("read clipboard sequence"))
	}
	if sequence != 0 && uint32(current) != sequence {
		return closeWith(false, nil)
	}
	available, err := clipboardFormatAvailable(clipboardFormatUnicodeText)
	if err != nil {
		return closeWith(false, err)
	}
	if !available {
		return closeWith(false, nil)
	}
	handle, _, _ := getClipboardDataProc.Call(clipboardFormatUnicodeText)
	if handle == 0 {
		return closeWith(false, errors.New("read clipboard data"))
	}
	matches, err := clipboardUTF16Equals(handle, payload)
	if err != nil {
		return closeWith(false, err)
	}
	if !matches {
		return closeWith(false, nil)
	}
	emptied, _, _ := emptyClipboardProc.Call()
	if emptied == 0 {
		return closeWith(false, errors.New("empty clipboard"))
	}
	return closeWith(true, nil)
}

func allocateClipboardUTF16(value string) (uintptr, error) {
	units, err := windows.UTF16FromString(value)
	if err != nil {
		return 0, err
	}
	bytes := unsafe.Slice((*byte)(unsafe.Pointer(&units[0])), len(units)*2)
	handle, err := allocateClipboardBytes(bytes)
	runtime.KeepAlive(units)
	return handle, err
}

func allocateClipboardBytes(value []byte) (uintptr, error) {
	handle, _, _ := globalAllocProc.Call(globalMemoryMoveable, uintptr(len(value)))
	if handle == 0 {
		return 0, errors.New("allocate clipboard memory")
	}
	pointer, _, _ := globalLockProc.Call(handle)
	if pointer == 0 {
		return 0, freeClipboardHandlesAfter(errors.New("lock clipboard memory"), []uintptr{handle})
	}
	if len(value) != 0 {
		_, _, _ = rtlMoveMemoryProc.Call(pointer, uintptr(unsafe.Pointer(&value[0])), uintptr(len(value)))
	}
	if err := unlockClipboardMemory(handle); err != nil {
		return 0, freeClipboardHandlesAfter(err, []uintptr{handle})
	}
	runtime.KeepAlive(value)
	return handle, nil
}

func clipboardUTF16Equals(handle uintptr, expected string) (bool, error) {
	units, err := windows.UTF16FromString(expected)
	if err != nil {
		return false, err
	}
	size, _, _ := globalSizeProc.Call(handle)
	if size == 0 {
		return false, errors.New("read clipboard data size")
	}
	if size < uintptr(len(units)*2) {
		return false, nil
	}
	pointer, _, _ := globalLockProc.Call(handle)
	if pointer == 0 {
		return false, errors.New("lock clipboard data")
	}
	actual := make([]byte, len(units)*2)
	if len(actual) != 0 {
		_, _, _ = rtlMoveMemoryProc.Call(uintptr(unsafe.Pointer(&actual[0])), pointer, uintptr(len(actual)))
	}
	matches := true
	for i := range units {
		if binary.LittleEndian.Uint16(actual[i*2:]) != units[i] {
			matches = false
			break
		}
	}
	zeroBytes(actual)
	if err := unlockClipboardMemory(handle); err != nil {
		return false, errors.New("unlock clipboard data")
	}
	runtime.KeepAlive(units)
	return matches, nil
}

func unlockClipboardMemory(handle uintptr) error {
	// GlobalUnlock returns zero both when the final lock is released successfully
	// and on failure. Clear thread last-error immediately before the call so the
	// documented NO_ERROR success case cannot inherit an unrelated stale errno.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	_, _, _ = setLastErrorProc.Call(0)
	unlocked, _, unlockErr := globalUnlockProc.Call(handle)
	if unlocked == 0 && !isSuccessErrno(unlockErr) {
		return errors.New("unlock clipboard memory")
	}
	return nil
}

func clipboardFormatAvailable(format uintptr) (bool, error) {
	// Last-error is thread-local. Keep the clear/read pair on one OS thread so a
	// legitimate "format absent" zero cannot inherit a stale error from another
	// syscall or observe the cleared value on a different scheduler thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	_, _, _ = setLastErrorProc.Call(0)
	available, _, availableErr := isClipboardFormatAvailableProc.Call(format)
	if available != 0 {
		return true, nil
	}
	if !isSuccessErrno(availableErr) {
		return false, errors.New("inspect clipboard format")
	}
	return false, nil
}

func freeClipboardHandles(handles []uintptr) error {
	var failed bool
	for _, handle := range handles {
		if handle != 0 {
			remaining, _, _ := globalFreeProc.Call(handle)
			failed = failed || remaining != 0
		}
	}
	if failed {
		return errors.New("free clipboard memory")
	}
	return nil
}

func freeClipboardHandlesAfter(operationErr error, handles []uintptr) error {
	if freeErr := freeClipboardHandles(handles); freeErr != nil {
		return freeErr
	}
	return operationErr
}

func isSuccessErrno(err error) bool {
	return err == nil || errors.Is(err, syscall.Errno(0))
}
