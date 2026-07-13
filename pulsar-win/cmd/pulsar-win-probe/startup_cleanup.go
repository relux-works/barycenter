package main

// runStartupCleanup centralizes the rollback order shared by every partial
// initialization stage. The helper must quiesce before Go-owned notification
// events can be closed; if destroy is refused, those events are intentionally
// left to process teardown so no worker can signal a recycled handle.
func runStartupCleanup(destroyHelper func() bool, destroyWindows []func(), closeEvents []func(), closeLog func()) {
	canCloseEvents := true
	if destroyHelper != nil {
		canCloseEvents = destroyHelper()
	}
	for _, destroy := range destroyWindows {
		destroy()
	}
	if canCloseEvents {
		for _, closeEvent := range closeEvents {
			closeEvent()
		}
	}
	if closeLog != nil {
		closeLog()
	}
}
