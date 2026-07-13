package winprobe

// CaptureObservation tracks the positive evidence needed before the shell may
// create an artifact or report that capture started. A valid format in a
// terminal result is not proof that IAudioClient::Start succeeded.
type CaptureObservation struct {
	ObservedCapturing bool
	ArtifactAttempted bool
}

// Observe returns true exactly once, and only for an observed CAPTURING state
// carrying a valid format. Terminal-first observations must still be drained,
// but they cannot create or promote an artifact.
func (o *CaptureObservation) Observe(state CaptureState, format CaptureFormat) bool {
	if state != CaptureStateCapturing {
		return false
	}
	o.ObservedCapturing = true
	if format.Valid != 1 || o.ArtifactAttempted {
		return false
	}
	o.ArtifactAttempted = true
	return true
}

// FrameLimiter clips evidence at an exact whole-frame boundary. Once the
// limit is reached it requests stop once, while later buffered frames remain
// drainable but are discarded instead of extending the artifact.
type FrameLimiter struct {
	LimitFrames   uint64
	WrittenFrames uint64
	StopRequested bool
}

func (l *FrameLimiter) Accept(readFrames uint32) (writeFrames uint32, requestStop bool) {
	if l.LimitFrames == 0 {
		return readFrames, false
	}
	remaining := uint64(0)
	if l.WrittenFrames < l.LimitFrames {
		remaining = l.LimitFrames - l.WrittenFrames
	}
	write := uint64(readFrames)
	if write > remaining {
		write = remaining
	}
	l.WrittenFrames += write
	if l.WrittenFrames >= l.LimitFrames && !l.StopRequested {
		l.StopRequested = true
		requestStop = true
	}
	return uint32(write), requestStop
}

func AccountCaptureRead(observedCapturing, writerAvailable bool, readFrames uint32) (writeFrames, discardFrames uint32) {
	if !observedCapturing || !writerAvailable {
		return 0, readFrames
	}
	return readFrames, 0
}
