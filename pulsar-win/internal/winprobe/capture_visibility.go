package winprobe

// CaptureVisibilityEvidence records only a proven frame overlap between a
// positively observed CAPTURING state and a hidden main window. The shell
// establishes the hidden epoch on its sole waiter after draining frames that
// may predate the hide transition.
type CaptureVisibilityEvidence struct {
	hidden       bool
	capturing    bool
	frameOverlap bool
}

func (e *CaptureVisibilityEvidence) SetHidden(hidden bool) {
	e.hidden = hidden
}

func (e *CaptureVisibilityEvidence) ObserveState(state CaptureState) {
	if state == CaptureStateCapturing {
		e.capturing = true
	} else if state == CaptureStatePreparing || state == CaptureStateActivating {
		e.capturing = false
	}
}

func (e *CaptureVisibilityEvidence) ObserveFrames(frames uint32) {
	if frames != 0 && e.hidden && e.capturing {
		e.frameOverlap = true
	}
}

func (e *CaptureVisibilityEvidence) FrameOverlap() bool {
	return e.frameOverlap
}

func (e *CaptureVisibilityEvidence) Reset() {
	*e = CaptureVisibilityEvidence{}
}
