package main

import "relux.works/duet/pulsar-win/internal/winprobe"

type queryFailureOutcome struct {
	cancelHR  winprobe.HResult
	releaseHR winprobe.HResult
	released  bool
}

type resultQueryDisposition uint8

const (
	resultQueryPending resultQueryDisposition = iota
	resultQueryReady
	resultQueryFailed
)

// classifyResultQuery deliberately evaluates the call HRESULT before the
// output state. Failed native calls commonly leave state zero-initialized;
// treating state==0 first would silently hide the failure forever.
func classifyResultQuery(callHR winprobe.HResult, state int32) resultQueryDisposition {
	if callHR.Failed() {
		return resultQueryFailed
	}
	if callHR == 1 || state == 0 {
		return resultQueryPending
	}
	return resultQueryReady
}

// failClosedQuery cancels/stops the real operation first, then attempts its
// production release export. A pending release remains owned for retry; a
// terminal or unknown operation is removed without trusting zeroed result
// outputs from the failed query call.
func failClosedQuery(cancel func() winprobe.HResult, release func() winprobe.HResult) queryFailureOutcome {
	outcome := queryFailureOutcome{}
	if cancel != nil {
		outcome.cancelHR = cancel()
	}
	if release != nil {
		outcome.releaseHR = release()
		outcome.released = outcome.releaseHR.Succeeded()
	}
	return outcome
}
