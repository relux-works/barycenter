package main

import (
	"sync"

	protocol "relux.works/duet/pulsar-win/wire"
)

type windowsOverlayPrepared struct {
	mu              sync.Mutex
	samples         []float32
	delivery        string
	state           *overlayState
	interruptState  *interruptState
	interruptAnchor *windowsInterruptAnchor
}

type windowsInterruptController interface {
	InterruptReady() bool
	SuspendForInterrupt() (*windowsInterruptAnchor, error)
	ResumeFromInterrupt(*windowsInterruptAnchor, int64) bool
}

// WindowsOverlayMediaClipMixer binds the protocol lifecycle to the portable
// render engine. Decode/allocation stays in Prepare; Arm only publishes the
// already-built PCM snapshot.
type WindowsOverlayMediaClipMixer struct {
	engine       *Engine
	controllerMu sync.RWMutex
	controller   windowsInterruptController
}

func NewWindowsOverlayMediaClipMixer(engine *Engine) *WindowsOverlayMediaClipMixer {
	return &WindowsOverlayMediaClipMixer{engine: engine}
}

func (*WindowsOverlayMediaClipMixer) DeliveryCapabilities() []string {
	return []string{protocol.CapabilityOverlayMix, protocol.CapabilityInterruptResume}
}

func (m *WindowsOverlayMediaClipMixer) BindInterruptController(controller windowsInterruptController) {
	m.controllerMu.Lock()
	m.controller = controller
	m.controllerMu.Unlock()
}

func (m *WindowsOverlayMediaClipMixer) interruptController() windowsInterruptController {
	m.controllerMu.RLock()
	defer m.controllerMu.RUnlock()
	return m.controller
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
	clip.Decoder = &windowsOverlayPrepared{samples: samples, delivery: delivery}
	return clip, nil
}

func (m *WindowsOverlayMediaClipMixer) Arm(
	clip *PreparedMediaClip,
	plan MediaClipPlayPlan,
	started func(int64),
	ended func(int64),
) error {
	if m == nil || m.engine == nil || clip == nil {
		return mediaClipFailure("capability_lost")
	}
	prepared, ok := clip.Decoder.(*windowsOverlayPrepared)
	if !ok {
		return mediaClipFailure("audio_graph_failed")
	}
	prepared.mu.Lock()
	defer prepared.mu.Unlock()
	if prepared.state != nil || prepared.interruptState != nil || prepared.delivery != plan.Control.Delivery {
		return mediaClipFailure("audio_graph_failed")
	}
	if plan.Control.Delivery == "interrupt" {
		controller := m.interruptController()
		if controller == nil || !controller.InterruptReady() {
			return mediaClipFailure("interrupt_capability_lost")
		}
		var state *interruptState
		startedWrapper := func(localMS int64) {
			anchor, err := controller.SuspendForInterrupt()
			if err != nil {
				m.engine.CancelInterrupt(state, 0, func() { m.engine.ReleaseInterrupt(state) })
				return
			}
			prepared.mu.Lock()
			prepared.interruptAnchor = anchor
			prepared.mu.Unlock()
			started(localMS)
		}
		endedWrapper := func(localMS int64) {
			resumed := m.resumeInterrupt(prepared, controller, state, plan.Control.FadeInMS)
			if !resumed {
				m.engine.ReleaseInterrupt(state)
			}
			ended(localMS)
		}
		var err error
		state, err = m.engine.ArmInterrupt(prepared.samples, plan, startedWrapper, endedWrapper)
		if err != nil {
			return err
		}
		prepared.interruptState = state
		return nil
	}
	if plan.Control.Delivery != "overlay" {
		return mediaClipFailure("capability_lost")
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
	if m == nil || m.engine == nil || !ok {
		done(false, nil)
		return
	}
	prepared.mu.Lock()
	overlay := prepared.state
	interrupt := prepared.interruptState
	prepared.mu.Unlock()
	if interrupt != nil {
		controller := m.interruptController()
		if !m.engine.CancelInterrupt(interrupt, command.FadeMS, func() {
			resumed := false
			if controller != nil {
				resumed = m.resumeInterrupt(prepared, controller, interrupt, interrupt.control.FadeInMS)
			}
			if !resumed {
				m.engine.ReleaseInterrupt(interrupt)
			}
			done(resumed, nil)
		}) {
			done(false, nil)
		}
		return
	}
	if overlay == nil || !m.engine.CancelOverlay(overlay, command.FadeMS, func() { done(false, nil) }) {
		done(false, nil)
	}
}

func (m *WindowsOverlayMediaClipMixer) resumeInterrupt(
	prepared *windowsOverlayPrepared,
	controller windowsInterruptController,
	state *interruptState,
	fadeInMS int64,
) bool {
	prepared.mu.Lock()
	anchor := prepared.interruptAnchor
	prepared.interruptAnchor = nil
	prepared.interruptState = nil
	prepared.mu.Unlock()
	if anchor == nil || !controller.ResumeFromInterrupt(anchor, fadeInMS) {
		return false
	}
	return m.engine.ReleaseInterrupt(state)
}

func (*WindowsOverlayMediaClipMixer) Dispose(clip *PreparedMediaClip) {
	if clip == nil {
		return
	}
	if prepared, ok := clip.Decoder.(*windowsOverlayPrepared); ok {
		prepared.mu.Lock()
		prepared.samples = nil
		prepared.state = nil
		prepared.interruptState = nil
		prepared.interruptAnchor = nil
		prepared.mu.Unlock()
	}
	clip.Decoder = nil
}
