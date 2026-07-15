package main

import "errors"

type airControlCall struct {
	done chan error
}

// airControlChanged waits until the serialized runtime has accepted the
// committed authority generation. An activation response is the client-side
// barrier, so this cannot be a best-effort wake.
func (l *loop) airControlChanged() error {
	call := airControlCall{done: make(chan error, 1)}
	select {
	case l.airControlCh <- call:
	case <-l.stopped:
		return errors.New("Air runtime stopped")
	}
	select {
	case err := <-call.done:
		return err
	case <-l.stopped:
		return errors.New("Air runtime stopped")
	}
}

// reconcileAirControlRuntime runs only on the serialized coordinator loop.
// It resolves the complete authoritative set, parks stale controllers, and
// lets the existing Air resolver apply join/switch generations and catch-up.
func (l *loop) reconcileAirControlRuntime() error {
	runtimes, err := l.st.ActiveAirRuntimes()
	if err != nil {
		l.log.Error("Air lifecycle runtime reconciliation failed", "err", err)
		return err
	}
	active := make(map[string]bool, len(runtimes))
	for _, runtime := range runtimes {
		active[runtime.AirID] = true
	}
	for airID := range l.airs {
		if !active[airID] {
			l.parkAirState(airID)
		}
	}
	for orbitID := range l.airOf {
		delete(l.airOf, orbitID)
	}
	for _, runtime := range runtimes {
		l.airState(runtime)
	}
	// Membership/pointer changes can cancel pending targets and unblock newly
	// eligible work. The scheduler re-resolves durable target snapshots.
	l.runTransmissionScheduler(l.transmissionNow())
	return nil
}
