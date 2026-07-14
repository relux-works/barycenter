package main

import (
	protocol "relux.works/duet/pulsar-win/wire"
)

type windowsOverlayPrepared struct {
	samples []float32
	state   *overlayState
}

// WindowsOverlayMediaClipMixer binds the protocol lifecycle to the portable
// render engine. Decode/allocation stays in Prepare; Arm only publishes the
// already-built PCM snapshot.
type WindowsOverlayMediaClipMixer struct {
	engine *Engine
}

func NewWindowsOverlayMediaClipMixer(engine *Engine) *WindowsOverlayMediaClipMixer {
	return &WindowsOverlayMediaClipMixer{engine: engine}
}

func (*WindowsOverlayMediaClipMixer) DeliveryCapabilities() []string {
	return []string{protocol.CapabilityOverlayMix}
}

func (*WindowsOverlayMediaClipMixer) Prepare(localPath, delivery string) (*PreparedMediaClip, error) {
	clip, err := (PreparedOnlyWindowsMediaClipMixer{}).Prepare(localPath, delivery)
	if err != nil {
		return nil, err
	}
	samples, ok := clip.Decoder.([]float32)
	if !ok || len(samples) < channels {
		return nil, mediaClipFailure("decode_failed")
	}
	clip.Decoder = &windowsOverlayPrepared{samples: samples}
	return clip, nil
}

func (m *WindowsOverlayMediaClipMixer) Arm(
	clip *PreparedMediaClip,
	plan MediaClipPlayPlan,
	started func(int64),
	ended func(int64),
) error {
	if m == nil || m.engine == nil || clip == nil || plan.Control.Delivery != "overlay" {
		return mediaClipFailure("capability_lost")
	}
	prepared, ok := clip.Decoder.(*windowsOverlayPrepared)
	if !ok || prepared.state != nil {
		return mediaClipFailure("audio_graph_failed")
	}
	state, err := m.engine.ArmOverlay(prepared.samples, plan, started, ended)
	if err != nil {
		return err
	}
	prepared.state = state
	return nil
}

func (m *WindowsOverlayMediaClipMixer) Cancel(
	clip *PreparedMediaClip,
	command protocol.CancelMediaPayload,
	done func(bool, error),
) {
	prepared, ok := clip.Decoder.(*windowsOverlayPrepared)
	if m == nil || m.engine == nil || !ok || prepared.state == nil {
		done(false, nil)
		return
	}
	if !m.engine.CancelOverlay(prepared.state, command.FadeMS, func() { done(false, nil) }) {
		done(false, nil)
	}
}

func (*WindowsOverlayMediaClipMixer) Dispose(clip *PreparedMediaClip) {
	if clip == nil {
		return
	}
	if prepared, ok := clip.Decoder.(*windowsOverlayPrepared); ok {
		prepared.samples = nil
		prepared.state = nil
	}
	clip.Decoder = nil
}
