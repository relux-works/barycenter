//go:build !windows

package main

func copyAutomationSecretToClipboard(string) bool { return false }
func clearAutomationSecretClipboard()             {}
