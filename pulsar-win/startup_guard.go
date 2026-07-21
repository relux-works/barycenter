package main

import (
	"log/slog"
	"runtime/debug"
)

// guardUnpairedShellStartup turns an otherwise invisible GUI-subsystem panic
// into a durable log record and a native user-facing failure surface. The
// shell remains supported: returning supported=true prevents the CLI fallback
// from calling os.Exit after the diagnostic has already been presented.
func guardUnpairedShellStartup(log *slog.Logger, showFatal func(), start func() (bool, bool)) (paired, supported bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Error("unpaired shell startup panic", "panic", recovered, "stack", string(debug.Stack()))
			if showFatal != nil {
				showFatal()
			}
			paired, supported = false, true
		}
	}()
	return start()
}
