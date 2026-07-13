package winprobe

// CaptureDiagnosticEvents converts the additive private native evidence
// channel into the same JSONL events used by packaged scenarios. Timestamp
// errors are accepted evidence; cleanup errors are secondary failures and do
// not replace the terminal reason/HRESULT returned by CaptureGetResult.
func CaptureDiagnosticEvents(previous, current CaptureDiagnostics) []LogEvent {
	var events []LogEvent
	if current.TimestampErrorCount > previous.TimestampErrorCount {
		events = append(events, LogEvent{
			Scenario:        ScenarioCapture,
			Result:          ResultAttempt,
			Action:          "wasapi_timestamp_error",
			SelectedAPIPath: "AUDCLNT_BUFFERFLAGS_TIMESTAMP_ERROR+PulsarProbeCaptureGetDiagnosticsV1",
			Fields: map[string]any{
				"observedCount": current.TimestampErrorCount,
				"accepted":      true,
			},
		})
	}
	if (current.CleanupReleaseBufferError.Failed() && current.CleanupReleaseBufferError != previous.CleanupReleaseBufferError) ||
		(current.CleanupStopError.Failed() && current.CleanupStopError != previous.CleanupStopError) {
		events = append(events, LogEvent{
			Scenario:        ScenarioCapture,
			Result:          ResultFail,
			Action:          "capture_secondary_cleanup_failure",
			SelectedAPIPath: "PulsarProbeCaptureGetDiagnosticsV1(private-extension-v1)",
			FailureCause:    "secondary cleanup failed; primary terminal cause remains authoritative",
			Fields: map[string]any{
				"releaseBufferHResult": current.CleanupReleaseBufferError.Hex(),
				"stopHResult":          current.CleanupStopError.Hex(),
			},
		})
	}
	return events
}
