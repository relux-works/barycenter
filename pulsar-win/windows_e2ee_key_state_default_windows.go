//go:build windows

package main

func newDefaultWindowsE2EEKeyStateRepository(dir string) (*WindowsE2EEKeyStateRepository, error) {
	return NewWindowsE2EEKeyStateRepository(WindowsE2EEKeyStateOptions{
		Directory: dir,
		Protector: dpapiDataProtector{api: windowsDataProtectionAPI{}},
		Files:     windowsSecureFileOps{},
	})
}
