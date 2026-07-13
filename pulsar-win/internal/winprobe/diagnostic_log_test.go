package winprobe

import (
	"bytes"
	"strings"
	"testing"
)

func TestCaptureDiagnosticEventsAreRetrievableJSONLWithoutReplacingPrimaryCause(t *testing.T) {
	t.Parallel()
	current := CaptureDiagnostics{
		TimestampErrorCount:       3,
		CleanupReleaseBufferError: HResult(int32(-2147024891)),
		CleanupStopError:          HResult(int32(-2147467259)),
	}
	events := CaptureDiagnosticEvents(CaptureDiagnostics{}, current)
	if len(events) != 2 {
		t.Fatalf("events = %d, want timestamp and cleanup evidence", len(events))
	}
	var output bytes.Buffer
	logger := NewJSONLogger(&output)
	for _, event := range events {
		if err := logger.Log(event); err != nil {
			t.Fatal(err)
		}
	}
	jsonl := output.String()
	for _, required := range []string{"wasapi_timestamp_error", "capture_secondary_cleanup_failure", "0x80070005", "0x80004005", "primary terminal cause remains authoritative"} {
		if !strings.Contains(jsonl, required) {
			t.Fatalf("JSONL missing %q: %s", required, jsonl)
		}
	}
	if repeated := CaptureDiagnosticEvents(current, current); len(repeated) != 0 {
		t.Fatalf("unchanged diagnostics emitted %d duplicate event(s)", len(repeated))
	}
}
