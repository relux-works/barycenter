//go:build windows

package main

import (
	"sync"
	"time"
)

var airInviteClipboard = struct {
	sync.Mutex
	backend  recoveryClipboardBackend
	owner    uintptr
	sequence uint32
	payload  string
	timer    *time.Timer
	retry    int
	lease    uint64
	next     uint64
}{}

// copyAirInviteToClipboard is explicit-only. The existing hardened clipboard
// backend marks the value as excluded from history/cloud processing and the
// compare-and-clear lease never deletes newer user clipboard content.
func copyAirInviteToClipboard(code string) bool {
	if !validPhaseOneDisplayText(code, 512, false) || currentMainWindowOwner() == 0 {
		return false
	}
	backend, err := newPlatformRecoveryClipboardBackend()
	if err != nil {
		return false
	}
	owner := currentMainWindowOwner()
	airInviteClipboard.Lock()
	defer airInviteClipboard.Unlock()
	if airInviteClipboard.timer != nil {
		airInviteClipboard.timer.Stop()
	}
	if airInviteClipboard.backend != nil && airInviteClipboard.payload != "" {
		_, _ = airInviteClipboard.backend.ClearIfUnchanged(
			airInviteClipboard.owner, airInviteClipboard.sequence, airInviteClipboard.payload)
	}
	resetAirInviteClipboardLocked()
	publication, err := backend.Publish(owner, code)
	if err != nil || !publication.Exposed {
		return false
	}
	airInviteClipboard.backend, airInviteClipboard.owner = backend, owner
	airInviteClipboard.sequence, airInviteClipboard.payload = publication.Sequence, code
	airInviteClipboard.next++
	airInviteClipboard.lease = airInviteClipboard.next
	lease := airInviteClipboard.lease
	airInviteClipboard.timer = time.AfterFunc(60*time.Second, func() {
		clearAirInviteClipboardLease(lease)
	})
	return true
}

func clearAirInviteClipboard() {
	airInviteClipboard.Lock()
	defer airInviteClipboard.Unlock()
	clearAirInviteClipboardLocked()
}

func clearAirInviteClipboardLease(lease uint64) {
	airInviteClipboard.Lock()
	defer airInviteClipboard.Unlock()
	if airInviteClipboard.lease != lease {
		return
	}
	clearAirInviteClipboardLocked()
}

func clearAirInviteClipboardLocked() {
	if airInviteClipboard.timer != nil {
		airInviteClipboard.timer.Stop()
		airInviteClipboard.timer = nil
	}
	if airInviteClipboard.backend != nil && airInviteClipboard.payload != "" {
		_, err := airInviteClipboard.backend.ClearIfUnchanged(
			airInviteClipboard.owner, airInviteClipboard.sequence, airInviteClipboard.payload)
		if err != nil && airInviteClipboard.retry < len(recoveryClipboardRetryDelays) {
			delay := recoveryClipboardRetryDelays[airInviteClipboard.retry]
			airInviteClipboard.retry++
			lease := airInviteClipboard.lease
			airInviteClipboard.timer = time.AfterFunc(delay, func() { clearAirInviteClipboardLease(lease) })
			return
		}
	}
	resetAirInviteClipboardLocked()
}

func resetAirInviteClipboardLocked() {
	airInviteClipboard.backend, airInviteClipboard.owner = nil, 0
	airInviteClipboard.sequence, airInviteClipboard.payload, airInviteClipboard.timer, airInviteClipboard.retry, airInviteClipboard.lease = 0, "", nil, 0, 0
}
