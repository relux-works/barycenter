package main

import "fmt"

type quitState struct {
	timeSeconds      int
	registryEntries  int
	callbackRefs     int
	waiterAlive      bool
	cleanupReadySent bool
	watchdogArmed    bool
	watchdogDefeated bool
	destroyed        bool
}

func (s quitState) cleanupReadyInvariant() bool {
	return !s.cleanupReadySent ||
		(s.registryEntries == 0 && s.callbackRefs == 0 && s.waiterAlive)
}

func rejectedTimeoutPolicy(lateTerminalAt int) quitState {
	s := quitState{
		registryEntries: 1,
		callbackRefs:    1,
		waiterAlive:     true,
		watchdogArmed:   true,
	}

	// Rev 15's stale algorithm proceeds at five seconds, posts CLEANUP_READY,
	// and exits the sole-owner waiter while the operation is still pending.
	s.timeSeconds = 5
	s.cleanupReadySent = true
	s.waiterAlive = false

	// The callback can become terminal later, but no owner remains to query and
	// release its registry entry.
	s.timeSeconds = lateTerminalAt
	s.callbackRefs = 0
	return s
}

func requiredLateTerminalPolicy(lateTerminalAt int) quitState {
	s := quitState{
		registryEntries: 1,
		callbackRefs:    1,
		waiterAlive:     true,
		watchdogArmed:   true,
	}

	// Five seconds only changes UI/logging. The waiter retains ownership.
	s.timeSeconds = 5

	// On late terminal, the sole owner queries and releases, then proves
	// quiescence before posting CLEANUP_READY.
	s.timeSeconds = lateTerminalAt
	s.callbackRefs = 0
	s.registryEntries = 0
	s.cleanupReadySent = true
	s.destroyed = true
	s.watchdogDefeated = true
	return s
}

func main() {
	for _, late := range []int{6, 15, 29} {
		bad := rejectedTimeoutPolicy(late)
		fmt.Printf(
			"REJECTED t=%ds invariant=%v registry=%d waiterAlive=%v cleanupReady=%v\n",
			late, bad.cleanupReadyInvariant(), bad.registryEntries,
			bad.waiterAlive, bad.cleanupReadySent,
		)

		good := requiredLateTerminalPolicy(late)
		fmt.Printf(
			"REQUIRED t=%ds invariant=%v registry=%d waiterAlive=%v cleanupReady=%v watchdogDefeated=%v\n",
			late, good.cleanupReadyInvariant(), good.registryEntries,
			good.waiterAlive, good.cleanupReadySent, good.watchdogDefeated,
		)
	}
}
