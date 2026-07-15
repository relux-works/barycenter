//go:build !windows

package main

func chooseWindowsRecoveryDestination(uintptr, ShellLocale) string { return "" }
