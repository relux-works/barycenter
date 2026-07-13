package winprobe

import "testing"

func TestCaptureVisibilityEvidenceRequiresTemporalFrameOverlap(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  func(*CaptureVisibilityEvidence)
		want bool
	}{
		{
			name: "hide before start then restore before start",
			run: func(e *CaptureVisibilityEvidence) {
				e.SetHidden(true)
				e.SetHidden(false)
				e.ObserveState(CaptureStateCapturing)
				e.ObserveFrames(256)
			},
		},
		{
			name: "preparing is not capture",
			run: func(e *CaptureVisibilityEvidence) {
				e.SetHidden(true)
				e.ObserveState(CaptureStatePreparing)
				e.ObserveFrames(256)
			},
		},
		{
			name: "hidden through start with captured frame",
			run: func(e *CaptureVisibilityEvidence) {
				e.SetHidden(true)
				e.ObserveState(CaptureStateCapturing)
				e.ObserveFrames(1)
			},
			want: true,
		},
		{
			name: "hide during capture with later frame",
			run: func(e *CaptureVisibilityEvidence) {
				e.ObserveState(CaptureStateCapturing)
				e.SetHidden(true)
				e.ObserveFrames(1)
			},
			want: true,
		},
		{
			name: "picker restore hide with later frame",
			run: func(e *CaptureVisibilityEvidence) {
				e.ObserveState(CaptureStateCapturing)
				e.SetHidden(false)
				e.ObserveFrames(32)
				e.SetHidden(true)
				e.ObserveFrames(32)
			},
			want: true,
		},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var evidence CaptureVisibilityEvidence
			tc.run(&evidence)
			if got := evidence.FrameOverlap(); got != tc.want {
				t.Fatalf("FrameOverlap() = %v, want %v", got, tc.want)
			}
		})
	}
}
