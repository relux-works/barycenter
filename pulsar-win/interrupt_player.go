package main

import (
	"context"
	"errors"
	"time"
)

var errInterruptUnavailable = errors.New("exact interrupt resume unavailable")

// windowsInterruptAnchor binds the audible position to the exact player
// generation and element that was suspended. A later load/stop invalidates
// the token before any old completion can seek or resume the daemon.
type windowsInterruptAnchor struct {
	elementID  string
	loadGen    int64
	positionMS int64
	pauseDone  chan struct{}
}

func (p *Player) InterruptReady() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.daemon != nil && p.ring != nil && p.engine != nil &&
		p.playback == PlaybackPlaying && p.elementID != "" && p.interruptAnchor == nil
}

// SuspendForInterrupt snapshots what is actually audible, not the provider's
// buffered-ahead position. The player state and ring cut are frozen before the
// asynchronous daemon pause so the render completion dispatcher stays timely.
func (p *Player) SuspendForInterrupt() (*windowsInterruptAnchor, error) {
	p.mu.Lock()
	if p.playback != PlaybackPlaying || p.elementID == "" || p.interruptAnchor != nil {
		p.mu.Unlock()
		return nil, errInterruptUnavailable
	}
	anchor := &windowsInterruptAnchor{
		elementID: p.elementID, loadGen: p.loadGen,
		positionMS: p.audiblePositionLocked(), pauseDone: make(chan struct{}),
	}
	p.cancelTimersLocked()
	p.playback = PlaybackPaused
	p.startedPending = false
	p.draining = false
	p.anchorPosMS = anchor.positionMS
	p.anchorAt = time.Now()
	p.extrapolate = false
	p.interruptAnchor = anchor
	p.mu.Unlock()

	p.engine.SetExpectingMusic(false)
	p.ring.Clear()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err := p.daemon.Pause(ctx)
		cancel()
		// Drop both the audible tail included in the anchor and any samples the
		// producer delivered while the pause request was in flight.
		p.ring.Clear()
		if err != nil {
			p.log.Warn("interrupt daemon pause failed", "err", err)
		}
		close(anchor.pauseDone)
	}()
	return anchor, nil
}

// ResumeFromInterrupt seeks to the preserved audible anchor exactly once,
// then resumes behind a 120 ms (wire-controlled) raised-cosine music fade.
func (p *Player) ResumeFromInterrupt(anchor *windowsInterruptAnchor, fadeInMS int64) bool {
	if anchor == nil {
		return false
	}
	select {
	case <-anchor.pauseDone:
	case <-time.After(2200 * time.Millisecond):
		return false
	}

	p.mu.Lock()
	valid := p.interruptAnchor == anchor && p.loadGen == anchor.loadGen &&
		p.elementID == anchor.elementID
	if !valid {
		if p.interruptAnchor == anchor {
			p.interruptAnchor = nil
		}
		p.mu.Unlock()
		return false
	}
	p.mu.Unlock()

	p.ring.Clear()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	err := p.daemon.Seek(ctx, anchor.positionMS)
	cancel()
	if err != nil {
		p.log.Warn("interrupt resume seek failed", "err", err)
		return false
	}

	p.mu.Lock()
	valid = p.interruptAnchor == anchor && p.loadGen == anchor.loadGen &&
		p.elementID == anchor.elementID
	if !valid {
		if p.interruptAnchor == anchor {
			p.interruptAnchor = nil
		}
		p.mu.Unlock()
		return false
	}
	p.playback = PlaybackPlaying
	p.anchorPosMS = anchor.positionMS
	p.anchorAt = time.Now()
	p.extrapolate = true
	p.startedPending = false
	p.interruptAnchor = nil
	p.mu.Unlock()

	p.engine.gain.SetMusicGain(0, 0)
	p.engine.gain.SetMusicGain(1, int(max(fadeInMS, 0)))
	p.engine.SetExpectingMusic(true)
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	err = p.daemon.Resume(ctx)
	cancel()
	if err != nil {
		p.log.Warn("interrupt daemon resume failed", "err", err)
		p.mu.Lock()
		if p.loadGen == anchor.loadGen && p.elementID == anchor.elementID {
			p.playback = PlaybackPaused
			p.extrapolate = false
		}
		p.mu.Unlock()
		p.engine.SetExpectingMusic(false)
		return false
	}
	return true
}
