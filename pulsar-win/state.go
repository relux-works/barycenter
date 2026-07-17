// Heartbeat state (spec 8.4) and dropout telemetry — the pulsar-win side of
// the macOS statePayload/startStarvationWatch pair.
package main

import (
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

// providerName: v1.1 heartbeats carry the node's active provider; only
// Spotify exists on this node for now (spec-providers §7).
const providerName = "spotify"

// starvedStreakThreshold ~ 3 s of starved-only callbacks while music is
// expected (mac: ~10 ms callbacks, streak > 300). WASAPI event periods are
// coarser, so treat this as a loose mirror, not a calibrated constant.
const starvedStreakThreshold = 300

// SetSpeakerName records the render device shown in heartbeats. The Windows
// render loop calls it with the WASAPI default device's friendly name; the
// "Default output" placeholder stands until then.
func (p *Player) SetSpeakerName(name string) {
	if name == "" {
		return
	}
	p.mu.Lock()
	p.speakerName = name
	p.mu.Unlock()
}

// SetCaptureQualityState is the future processor/UI seam. It stores only the
// validated categorical projection and never enables or advertises capture.
func (p *Player) SetCaptureQualityState(state *protocol.CaptureQualityState) error {
	if err := protocol.ValidateCaptureQualityState(state); err != nil {
		return err
	}
	p.mu.Lock()
	p.captureQuality = protocol.CloneCaptureQualityState(state)
	p.mu.Unlock()
	if state != nil && (state.Quality != protocol.CaptureQualityAccepted || state.InputHealth != protocol.CaptureHealthOK) {
		p.log.Info("capture quality diagnostic",
			"generation", state.Generation,
			"workflow", state.Workflow,
			"quality", state.Quality,
			"reason", state.Reason,
			"input_health", state.InputHealth)
	}
	return nil
}

// CaptureQualityPresentation is the read-only native-shell seam. Passing the
// build's advertised capabilities keeps legacy builds visibly unsupported.
func (p *Player) CaptureQualityPresentation(capabilities protocol.CapabilitySet) protocol.CaptureQualityGuidance {
	p.mu.Lock()
	state := protocol.CloneCaptureQualityState(p.captureQuality)
	p.mu.Unlock()
	return protocol.PresentCaptureQuality(capabilities, state)
}

// StatePayload builds the 5 s heartbeat snapshot.
func (p *Player) StatePayload(rttMS int64) protocol.StatePayload {
	p.mu.Lock()
	playback := string(p.playback)
	uri := p.uri
	volume := p.volume
	speaker := p.speakerName
	captureQuality := protocol.CloneCaptureQualityState(p.captureQuality)
	p.mu.Unlock()

	var uriPtr *string
	if uri != "" {
		uriPtr = &uri
	}
	return protocol.StatePayload{
		Playback:       playback,
		URI:            uriPtr,
		PositionMS:     p.AudiblePositionMS(),
		Volume:         volume,
		Degraded:       false, // placeholder: no degradation source wired yet
		Underruns:      p.underruns.Load(),
		RTTMS:          rttMS,
		CaptureQuality: captureQuality,
		Provider:       providerName,
		// Single WASAPI default endpoint; no Airfoil on Windows (spec 6.5).
		Speakers: []protocol.Speaker{{Name: speaker, Connected: true}},
	}
}

// telemetryWatch mirrors the macOS starvation watch loosely: once per
// second, fed AND starved callbacks within the same window = an audible
// glitch (idle silence is starved-only); a long starved streak while music
// is expected is starvation worth shouting about.
func (p *Player) telemetryWatch() {
	ticker := time.NewTicker(p.telemetryInterval)
	defer ticker.Stop()
	var lastFed, lastStarved, lastOverlayFrames, lastInterruptFrames, lastLimiterHits int64
	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
		}
		s := p.engine.Stats()
		fedDelta := s.Fed - lastFed
		starvedDelta := s.Starved - lastStarved
		overlayDelta := s.OverlayFrames - lastOverlayFrames
		interruptDelta := s.InterruptFrames - lastInterruptFrames
		limiterDelta := s.LimiterHits - lastLimiterHits
		lastFed, lastStarved = s.Fed, s.Starved
		lastOverlayFrames, lastInterruptFrames = s.OverlayFrames, s.InterruptFrames
		lastLimiterHits = s.LimiterHits
		if fedDelta > 0 && starvedDelta > 0 {
			p.log.Warn("audible dropout",
				"starved_cbs", starvedDelta,
				"fed_cbs", fedDelta,
				"ring_fill_ms", p.ring.FillMS(sampleRate, channels))
		}
		if s.StarvedStreak > starvedStreakThreshold {
			p.log.Warn("audio starvation: no samples while expecting music",
				"starved_streak", s.StarvedStreak)
		}
		if overlayDelta > 0 || limiterDelta > 0 {
			p.log.Info("overlay mixer telemetry",
				"overlay_frames", overlayDelta,
				"limiter_hits", limiterDelta,
				"ring_fill_ms", p.ring.FillMS(sampleRate, channels))
		}
		if interruptDelta > 0 {
			p.log.Info("interrupt mixer telemetry",
				"interrupt_frames", interruptDelta,
				"ring_fill_ms", p.ring.FillMS(sampleRate, channels))
		}
	}
}
