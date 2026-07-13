package winprobe

import "testing"

func TestCaptureObservationRequiresPositiveCapturingState(t *testing.T) {
	t.Parallel()
	format := NewCaptureFormat()
	format.Valid = 1

	var failBeforeStart CaptureObservation
	if failBeforeStart.Observe(CaptureStateFailed, format) {
		t.Fatal("failed terminal with a valid format was treated as capture-start evidence")
	}
	if failBeforeStart.ObservedCapturing {
		t.Fatal("failed terminal marked CAPTURING as observed")
	}

	var terminalFirstWithFrames CaptureObservation
	if terminalFirstWithFrames.Observe(CaptureStateStopped, format) {
		t.Fatal("terminal-first observation was treated as capture-start evidence")
	}
	if !terminalFirstWithFrames.Observe(CaptureStateCapturing, format) {
		t.Fatal("an actual CAPTURING observation was rejected")
	}
	write, discard := AccountCaptureRead(false, false, 512)
	if write != 0 || discard != 512 {
		t.Fatalf("terminal-first buffered frames = write %d discard %d, want 0/512", write, discard)
	}
}

func TestFrameLimiterClipsCrossingPacketAndDiscardsPostStopPackets(t *testing.T) {
	t.Parallel()
	limiter := FrameLimiter{LimitFrames: 10, WrittenFrames: 8}
	write, stop := limiter.Accept(4)
	if write != 2 || !stop || limiter.WrittenFrames != 10 {
		t.Fatalf("crossing packet = write %d stop %v total %d, want 2/true/10", write, stop, limiter.WrittenFrames)
	}
	write, stop = limiter.Accept(4096)
	if write != 0 || stop || limiter.WrittenFrames != 10 {
		t.Fatalf("post-stop packet = write %d stop %v total %d, want 0/false/10", write, stop, limiter.WrittenFrames)
	}
}
