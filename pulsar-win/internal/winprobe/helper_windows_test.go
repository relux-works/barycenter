//go:build windows

package winprobe

import (
	"errors"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestLoadHelperModuleUsesPackagedHandle(t *testing.T) {
	originalPackaged := loadPackagedLibraryFn
	originalFallback := loadLibraryExFn
	t.Cleanup(func() { loadPackagedLibraryFn, loadLibraryExFn = originalPackaged, originalFallback })
	loadPackagedLibraryFn = func(*uint16, uint32) (windows.Handle, uint32) { return windows.Handle(123), 0 }
	loadLibraryExFn = func(string, windows.Handle, uintptr) (windows.Handle, error) {
		t.Fatal("fallback was called")
		return 0, nil
	}
	dll, choice, err := loadHelperModule()
	if err != nil {
		t.Fatal(err)
	}
	if choice != LoaderPackaged || dll.Handle != windows.Handle(123) || dll.Name != HelperDLLName {
		t.Fatalf("module = %#v via %q", dll, choice)
	}
}

func TestLoadHelperModuleFallsBackOnlyForNoPackage(t *testing.T) {
	originalPackaged := loadPackagedLibraryFn
	originalFallback := loadLibraryExFn
	originalExecutable := executablePathFn
	t.Cleanup(func() {
		loadPackagedLibraryFn, loadLibraryExFn, executablePathFn = originalPackaged, originalFallback, originalExecutable
	})
	loadPackagedLibraryFn = func(*uint16, uint32) (windows.Handle, uint32) { return 0, AppModelErrorNoPackage }
	executablePathFn = func() (string, error) { return `C:\Probe\pulsar-win-probe-amd64.exe`, nil }
	var gotPath string
	var gotFlags uintptr
	loadLibraryExFn = func(path string, zero windows.Handle, flags uintptr) (windows.Handle, error) {
		gotPath, gotFlags = path, flags
		if zero != 0 {
			t.Fatalf("zero = %v", zero)
		}
		return windows.Handle(456), nil
	}
	dll, choice, err := loadHelperModule()
	if err != nil {
		t.Fatal(err)
	}
	if choice != LoaderExecutableDir || dll.Handle != windows.Handle(456) {
		t.Fatalf("module = %#v via %q", dll, choice)
	}
	if !filepath.IsAbs(gotPath) || filepath.Base(gotPath) != HelperDLLName {
		t.Fatalf("fallback path = %q", gotPath)
	}
	wantFlags := uintptr(windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR | windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if gotFlags != wantFlags {
		t.Fatalf("flags = %#x, want %#x", gotFlags, wantFlags)
	}
}

func TestLoadHelperModuleRejectsOtherErrors(t *testing.T) {
	originalPackaged := loadPackagedLibraryFn
	originalFallback := loadLibraryExFn
	t.Cleanup(func() { loadPackagedLibraryFn, loadLibraryExFn = originalPackaged, originalFallback })
	loadPackagedLibraryFn = func(*uint16, uint32) (windows.Handle, uint32) { return 0, uint32(windows.ERROR_MOD_NOT_FOUND) }
	loadLibraryExFn = func(string, windows.Handle, uintptr) (windows.Handle, error) {
		return 0, errors.New("must not run")
	}
	if _, _, err := loadHelperModule(); err == nil {
		t.Fatal("loadHelperModule() accepted a packaged module-not-found error")
	}
}
