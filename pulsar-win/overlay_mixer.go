package main

import (
	"sync"

	protocol "relux.works/duet/pulsar-win/wire"
)

type windowsOverlayPrepared struct {
	mu               sync.Mutex
	samples          []float32
	delivery         string
	state            *overlayState
	interruptState   *interruptState
	interruptAnchor  *windowsInterruptAnchor
	interruptDone    chan struct{}
	interruptFinal   bool
	interruptResumed bool
	interruptErr     error
}

type windowsInterruptController interface {
	InterruptReady() bool
	SuspendForInterrupt() (*windowsInterruptAnchor, error)
	ResumeFromInterrupt(*windowsInterruptAnchor, int64) bool
	AbandonInterrupt(*windowsInterruptAnchor)
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
	failed func(error),
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
	if prepared.state != nil || prepared.interruptState != nil ||
		prepared.delivery != plan.Control.Delivery || failed == nil {
		return mediaClipFailure("audio_graph_failed")
	}
	if plan.Control.Delivery == "interrupt" {
		controller := m.interruptController()
		if controller == nil || !controller.InterruptReady() {
			return mediaClipFailure("interrupt_capability_lost")
		}
		var state *interruptState
		prepared.interruptDone = make(chan struct{})
		prepared.interruptFinal = false
		prepared.interruptResumed = false
		prepared.interruptErr = nil
		startedWrapper := func(localMS int64) {
			anchor, err := controller.SuspendForInterrupt()
			if err != nil {
				m.engine.CancelInterrupt(state, 0, func() {
					_, _ = m.finalizeInterrupt(prepared, controller, state, false, 0)
					failed(mediaClipFailure("interrupt_capability_lost"))
				})
				return
			}
			prepared.mu.Lock()
			prepared.interruptAnchor = anchor
			prepared.mu.Unlock()
			started(localMS)
		}
		endedWrapper := func(localMS int64) {
			resumed, err := m.finalizeInterrupt(
				prepared, controller, state, true, plan.Control.FadeInMS,
			)
			if err != nil || !resumed {
				failed(mediaClipFailure("audio_graph_failed"))
				return
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
	interruptFinalizing := prepared.interruptFinal
	prepared.mu.Unlock()
	if interrupt == nil && interruptFinalizing {
		controller := m.interruptController()
		go func() {
			resumed, err := m.finalizeInterrupt(prepared, controller, nil, command.ResumeMain, 0)
			done(resumed, err)
		}()
		return
	}
	if interrupt != nil {
		controller := m.interruptController()
		if !m.engine.CancelInterrupt(interrupt, command.FadeMS, func() {
			if controller == nil {
				m.engine.ReleaseInterrupt(interrupt)
				done(false, mediaClipFailure("audio_graph_failed"))
				return
			}
			resumed, finalizeErr := m.finalizeInterrupt(
				prepared, controller, interrupt, command.ResumeMain,
				interrupt.control.FadeInMS,
			)
			if finalizeErr != nil || (command.ResumeMain && !resumed) {
				done(false, mediaClipFailure("audio_graph_failed"))
				return
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

func (m *WindowsOverlayMediaClipMixer) finalizeInterrupt(
	prepared *windowsOverlayPrepared,
	controller windowsInterruptController,
	state *interruptState,
	resumeMain bool,
	fadeInMS int64,
) (bool, error) {
	prepared.mu.Lock()
	if prepared.interruptFinal {
		done := prepared.interruptDone
		prepared.mu.Unlock()
		<-done
		prepared.mu.Lock()
		resumed, err := prepared.interruptResumed, prepared.interruptErr
		prepared.mu.Unlock()
		return resumed, err
	}
	prepared.interruptFinal = true
	done := prepared.interruptDone
	anchor := prepared.interruptAnchor
	prepared.interruptAnchor = nil
	prepared.interruptState = nil
	prepared.mu.Unlock()

	resumed := false
	var resultErr error
	if anchor == nil {
		resultErr = mediaClipFailure("audio_graph_failed")
	} else if !resumeMain {
		controller.AbandonInterrupt(anchor)
	} else {
		resumed = controller.ResumeFromInterrupt(anchor, fadeInMS)
		if !resumed {
			controller.AbandonInterrupt(anchor)
			resultErr = mediaClipFailure("audio_graph_failed")
		}
	}
	if !m.engine.ReleaseInterrupt(state) && resultErr == nil {
		resultErr = mediaClipFailure("audio_graph_failed")
		resumed = false
	}
	prepared.mu.Lock()
	prepared.interruptResumed = resumed
	prepared.interruptErr = resultErr
	if done != nil {
		close(done)
	}
	prepared.mu.Unlock()
	return resumed, resultErr
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
		prepared.interruptDone = nil
		prepared.interruptFinal = false
		prepared.interruptResumed = false
		prepared.interruptErr = nil
		prepared.mu.Unlock()
	}
	clip.Decoder = nil
}
