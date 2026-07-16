package presentation

// StreamTrackDraftPhaseLabel presents only the frozen finite draft phases.
// Unknown server values fail closed to failed and are never echoed to users.
func StreamTrackDraftPhaseLabel(phase string) Label {
	switch phase {
	case "retained", "uploading", "uploaded", "processing", "ready", "failed":
		return label("stream_track.draft." + phase)
	default:
		return label("stream_track.draft.failed")
	}
}

// StreamTrackPlaybackPhaseLabel presents only the frozen player phases.
func StreamTrackPlaybackPhaseLabel(phase string) Label {
	switch phase {
	case "idle", "queued", "loading", "ready", "playing", "paused", "seeking", "rebuffering", "ended", "failed":
		return label("stream_track.playback." + phase)
	default:
		return label("stream_track.playback.failed")
	}
}

// StreamTrackActionLabel returns no generic fallback that could be mistaken
// for a capability. Callers must still require the exact server action.
func StreamTrackActionLabel(action string) Label {
	switch action {
	case "accept_policy", "upload", "retry", "delete", "queue", "replace", "pause", "seek", "resume", "report":
		return label("stream_track.action." + action)
	default:
		return Label{}
	}
}

// StreamTrackFailureLabel bounds failure copy to protocol-owned codes.
func StreamTrackFailureLabel(code string) Label {
	switch code {
	case "offline", "quota_exceeded", "unsupported_targets", "policy_required", "processing_failed", "variant_unavailable", "stale_generation", "service_unavailable":
		return label("stream_track.failure." + code)
	default:
		return label("stream_track.failure.service_unavailable")
	}
}
