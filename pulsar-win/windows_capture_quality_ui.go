package main

import protocol "relux.works/duet/pulsar-win/wire"

type WindowsCaptureQualityPresentation struct {
	Mode                      WindowsCaptureQualityMode
	DegradedConsent           bool
	BackendAvailable          bool
	Quality                   string
	Reason                    string
	Lifecycle                 string
	ResolvedMode              string
	AEC                       string
	NS                        string
	AGC                       string
	InputHealth               string
	InputCeilingDBFS          float64
	ReceiverOutputCeilingDBFS float64
	Active                    bool
	CanStop                   bool
	RequiresDegradedConsent   bool
}

func presentWindowsCaptureQuality(snapshot ShellSnapshot) WindowsCaptureQualityPresentation {
	result := WindowsCaptureQualityPresentation{
		Mode: snapshot.CaptureQualityMode, DegradedConsent: snapshot.CaptureQualityDegradedConsent,
		BackendAvailable: snapshot.CaptureQualityBackendAvailable,
		Lifecycle:        protocol.CaptureLifecycleIdle, ResolvedMode: protocol.CaptureRouteUnknown,
		AEC: protocol.CaptureEffectUnavailable, NS: protocol.CaptureEffectUnavailable,
		AGC: protocol.CaptureEffectActive, InputHealth: protocol.CaptureHealthOK,
		InputCeilingDBFS:          protocol.CaptureInputCeilingDBFS,
		ReceiverOutputCeilingDBFS: protocol.CaptureOutputCeilingDBFS,
	}
	if result.Mode != WindowsCaptureQualitySpeaker && result.Mode != WindowsCaptureQualityHeadphone {
		result.Mode = WindowsCaptureQualityAuto
	}
	if state := snapshot.CaptureQualityState; state != nil {
		result.Quality = state.Quality
		result.Reason = state.Reason
		result.Lifecycle = state.Lifecycle
		result.ResolvedMode = state.ResolvedMode
		result.AEC, result.NS, result.AGC = state.AEC, state.NS, state.AGC
		result.InputHealth = state.InputHealth
		result.InputCeilingDBFS = state.InputCeilingDBFS
		if state.InputHealth != protocol.CaptureHealthOK {
			result.Reason = state.InputHealth
		}
	} else if !result.BackendAvailable {
		result.Quality = protocol.CaptureQualityUnsupported
		result.Reason = "mixed_version"
	} else {
		// The current signed helper requests the Communications category but
		// deliberately does not claim verified native AEC/NS. Keep preflight
		// degraded until a capture generation supplies stronger exact state.
		result.Quality = protocol.CaptureQualityDegraded
		result.Reason = "aec_unavailable"
	}
	result.Active = result.Lifecycle == protocol.CaptureLifecyclePreparing ||
		result.Lifecycle == protocol.CaptureLifecycleAwaitingFallback ||
		result.Lifecycle == protocol.CaptureLifecycleCapturing ||
		result.Lifecycle == protocol.CaptureLifecycleReconfiguring ||
		snapshot.Recording == ShellRecordingActive || snapshot.Recording == ShellRecordingProcessing ||
		snapshot.SelfTestPhase == WindowsLocalSelfTestRequestingPermission ||
		snapshot.SelfTestPhase == WindowsLocalSelfTestRecording
	result.CanStop = result.Active && result.Lifecycle != protocol.CaptureLifecycleStopping
	result.RequiresDegradedConsent = (result.Quality == protocol.CaptureQualityDegraded ||
		result.Quality == protocol.CaptureQualityUnsupported) && !result.DegradedConsent
	return result
}

func nextWindowsCaptureQualityMode(mode WindowsCaptureQualityMode) WindowsCaptureQualityMode {
	switch mode {
	case WindowsCaptureQualityAuto:
		return WindowsCaptureQualitySpeaker
	case WindowsCaptureQualitySpeaker:
		return WindowsCaptureQualityHeadphone
	default:
		return WindowsCaptureQualityAuto
	}
}
