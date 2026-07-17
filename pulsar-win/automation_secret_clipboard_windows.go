//go:build windows

package main

// Automation secrets reuse the hardened explicit clipboard publication used
// by Air invites: cloud/history exclusion plus compare-and-clear after 60s.
func copyAutomationSecretToClipboard(secret string) bool { return copyAirInviteToClipboard(secret) }
func clearAutomationSecretClipboard()                    { clearAirInviteClipboard() }
