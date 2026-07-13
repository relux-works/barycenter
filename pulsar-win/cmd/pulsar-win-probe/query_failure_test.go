package main

import (
	"reflect"
	"testing"

	"relux.works/duet/pulsar-win/internal/winprobe"
)

func TestFailClosedQueryUsesCancelThenProductionReleasePolicy(t *testing.T) {
	t.Parallel()
	var calls []string
	outcome := failClosedQuery(
		func() winprobe.HResult { calls = append(calls, "cancel"); return 0 },
		func() winprobe.HResult { calls = append(calls, "release"); return 0 },
	)
	if !reflect.DeepEqual(calls, []string{"cancel", "release"}) || !outcome.released {
		t.Fatalf("calls/outcome = %v/%#v", calls, outcome)
	}

	pending := failClosedQuery(
		func() winprobe.HResult { return 0 },
		func() winprobe.HResult { return winprobe.HResult(int32(-2147483634)) }, // E_ILLEGAL_METHOD_CALL
	)
	if pending.released {
		t.Fatal("pending operation was treated as released")
	}
}

func TestClassifyResultQueryDoesNotHideFailedCallBehindZeroState(t *testing.T) {
	t.Parallel()
	failed := winprobe.HResult(int32(-2147024891))
	if got := classifyResultQuery(failed, 0); got != resultQueryFailed {
		t.Fatalf("failed call with zeroed state = %v, want failed", got)
	}
	if got := classifyResultQuery(1, 0); got != resultQueryPending {
		t.Fatalf("S_FALSE pending = %v", got)
	}
	if got := classifyResultQuery(0, 0); got != resultQueryPending {
		t.Fatalf("S_OK zero state = %v", got)
	}
	if got := classifyResultQuery(0, 1); got != resultQueryReady {
		t.Fatalf("S_OK terminal state = %v", got)
	}
}
