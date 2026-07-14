//go:build windows

package main

func replaceStateFile(from, to string) error {
	return (windowsSecureFileOps{}).Move(from, to, moveReplaceExisting|moveWriteThrough)
}
