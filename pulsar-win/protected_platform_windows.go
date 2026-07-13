//go:build windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	crypt32DLL             = windows.NewLazySystemDLL("crypt32.dll")
	cryptProtectDataProc   = crypt32DLL.NewProc("CryptProtectData")
	cryptUnprotectDataProc = crypt32DLL.NewProc("CryptUnprotectData")
	kernel32CredentialDLL  = windows.NewLazySystemDLL("kernel32.dll")
	getFileSizeExProc      = kernel32CredentialDLL.NewProc("GetFileSizeEx")
)

type windowsDataBlob struct {
	Size uint32
	Data *byte
}

type windowsProtectedAllocation struct {
	mu    sync.Mutex
	data  *byte
	size  uint32
	freed bool
}

func (a *windowsProtectedAllocation) Bytes() []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.freed || a.data == nil || a.size == 0 {
		return nil
	}
	return unsafe.Slice(a.data, int(a.size))
}

func (a *windowsProtectedAllocation) Free() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.freed {
		return errors.New("allocation already freed")
	}
	a.freed = true
	if a.data == nil {
		return nil
	}
	_, err := windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(a.data))))
	a.data, a.size = nil, 0
	return err
}

type windowsDataProtectionAPI struct{}

func (windowsDataProtectionAPI) Protect(input []byte, flags uint32) (protectedAllocation, error) {
	in := windowsDataBlob{Size: uint32(len(input))}
	if len(input) != 0 {
		in.Data = &input[0]
	}
	var out windowsDataBlob
	result, _, callErr := cryptProtectDataProc.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		0,
		0,
		0,
		uintptr(flags),
		uintptr(unsafe.Pointer(&out)),
	)
	runtime.KeepAlive(input)
	allocation := allocationFromWindowsBlob(out)
	if result == 0 {
		return allocation, normalizeWindowsCallError(callErr)
	}
	return allocation, nil
}

func (windowsDataProtectionAPI) Unprotect(input []byte, flags uint32) (protectedAllocation, error) {
	in := windowsDataBlob{Size: uint32(len(input))}
	if len(input) != 0 {
		in.Data = &input[0]
	}
	var out windowsDataBlob
	result, _, callErr := cryptUnprotectDataProc.Call(
		uintptr(unsafe.Pointer(&in)),
		0,
		0,
		0,
		0,
		uintptr(flags),
		uintptr(unsafe.Pointer(&out)),
	)
	runtime.KeepAlive(input)
	allocation := allocationFromWindowsBlob(out)
	if result == 0 {
		return allocation, normalizeWindowsCallError(callErr)
	}
	return allocation, nil
}

func allocationFromWindowsBlob(blob windowsDataBlob) protectedAllocation {
	if blob.Data == nil {
		return nil
	}
	return &windowsProtectedAllocation{data: blob.Data, size: blob.Size}
}

func normalizeWindowsCallError(err error) error {
	if err == nil || errors.Is(err, syscall.Errno(0)) {
		return errors.New("windows operation failed")
	}
	return err
}

type windowsSecureFileOps struct{}

func (windowsSecureFileOps) EnsureDir(path string) error { return os.MkdirAll(path, 0o700) }

func (windowsSecureFileOps) Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (windowsSecureFileOps) Open(path string, spec secureOpenSpec) (secureFileHandle, error) {
	path16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	handle, err := windows.CreateFile(path16, spec.Access, spec.Share, nil, spec.Disposition, spec.Flags, 0)
	return secureFileHandle(handle), err
}

func (windowsSecureFileOps) Write(handle secureFileHandle, value []byte) (int, error) {
	var written uint32
	err := windows.WriteFile(windows.Handle(handle), value, &written, nil)
	return int(written), err
}

func (windowsSecureFileOps) Read(handle secureFileHandle, value []byte) (int, error) {
	var read uint32
	err := windows.ReadFile(windows.Handle(handle), value, &read, nil)
	return int(read), err
}

func (windowsSecureFileOps) Size(handle secureFileHandle) (int64, error) {
	var size int64
	result, _, callErr := getFileSizeExProc.Call(uintptr(handle), uintptr(unsafe.Pointer(&size)))
	if result == 0 {
		return 0, normalizeWindowsCallError(callErr)
	}
	return size, nil
}

func (windowsSecureFileOps) Flush(handle secureFileHandle) error {
	return windows.FlushFileBuffers(windows.Handle(handle))
}

func (windowsSecureFileOps) Close(handle secureFileHandle) error {
	return windows.CloseHandle(windows.Handle(handle))
}

func (windowsSecureFileOps) Move(from, to string, flags uint32) error {
	from16, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	to16, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from16, to16, flags)
}

func (windowsSecureFileOps) Delete(path string) error {
	path16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	err = windows.DeleteFile(path16)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		return nil
	}
	return err
}

func (windowsSecureFileOps) List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names, nil
}

func (ops windowsSecureFileOps) AcquireLock(path string) (secureFileHandle, error) {
	if err := ops.EnsureDir(filepath.Dir(path)); err != nil {
		return 0, err
	}
	return ops.Open(path, secureOpenSpec{Access: fileGenericRead | fileGenericWrite, Share: fileShareNone, Disposition: fileOpenAlways, Flags: fileAttributeNormal})
}

func newDefaultCredentialRepository(dir string) (*ProtectedCredentialRepository, error) {
	return NewProtectedCredentialRepository(CredentialRepositoryOptions{
		Directory: dir,
		Protector: dpapiDataProtector{api: windowsDataProtectionAPI{}},
		Files:     windowsSecureFileOps{},
	})
}
