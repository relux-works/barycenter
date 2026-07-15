//go:build !windows

package main

func copyAirInviteToClipboard(string) bool { return false }
func clearAirInviteClipboard()             {}
