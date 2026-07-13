package main

import "fmt"

type state uint8

const (
	preparing state = iota
	prepared
	activating
	capturing
	stopping
	sealed
	terminal
)

type ownedHandle struct {
	created int
	signals int
	closes  int
}

func (h *ownedHandle) create() { h.created++ }
func (h *ownedHandle) signalAndClose() {
	h.signals++
	h.closes++
}
func (h *ownedHandle) closeOnly() { h.closes++ }
func (h ownedHandle) valid() bool {
	return h.created == h.closes && h.signals <= h.closes
}

type session struct {
	state          state
	mtaReady       bool
	started        bool
	captureNotify  ownedHandle
	callbackNotify ownedHandle
}

type launchLifetime struct {
	creatorRef  bool
	registryRef bool
	threadRaw   bool
	terminal    bool
	destroyed   bool
}

func beginLaunch() launchLifetime {
	return launchLifetime{creatorRef: true, threadRaw: true}
}

func (l *launchLifetime) threadCompletesEarly() {
	l.terminal = true
	l.threadRaw = false
	l.maybeDestroy()
}

func (l *launchLifetime) publish() {
	l.registryRef = true
	l.creatorRef = false
	l.maybeDestroy()
}

func (l *launchLifetime) release() {
	l.registryRef = false
	l.maybeDestroy()
}

func (l *launchLifetime) maybeDestroy() {
	l.destroyed = !l.creatorRef && !l.registryRef && !l.threadRaw
}

func prepare() session {
	s := session{state: preparing}
	s.captureNotify.create()
	return s
}

func (s *session) mtaInit() {
	if s.state != preparing {
		panic("MTA init from wrong state")
	}
	s.state = prepared
	s.mtaReady = true
}

func (s *session) createCallbackDuplicate() { s.callbackNotify.create() }

func (s *session) callbackHandoff() {
	if s.state != activating {
		panic("callback handoff from wrong state")
	}
	// The callback transfers the interface but never claims capture started.
	s.callbackNotify.closeOnly()
}

func (s *session) startSucceeded() bool {
	s.started = true
	if s.state != activating {
		return false
	}
	s.state = capturing
	return true
}

func (s *session) activateCAS() bool {
	if s.state != prepared {
		s.callbackNotify.closeOnly()
		return false
	}
	s.state = activating
	return true
}

func (s *session) stop() {
	if s.state < stopping {
		s.state = stopping
	}
}

func require(name string, ok bool) {
	if !ok {
		panic("FAIL: " + name)
	}
	fmt.Println("PASS:", name)
}

func main() {
	require("private enum is contiguous PREPARING=0 through TERMINAL=6",
		preparing == 0 && prepared == 1 && activating == 2 && capturing == 3 &&
			stopping == 4 && sealed == 5 && terminal == 6)

	// _beginthreadex(initflag=0) may run before the launcher publishes the
	// registry entry. The creator-held shared ownership is the lifetime fence;
	// the capture thread itself intentionally owns only a raw pointer.
	early := beginLaunch()
	early.threadCompletesEarly()
	require("creator hold survives terminal-before-publication", early.terminal && !early.destroyed)
	early.publish()
	require("registry takes ownership after early terminal", early.registryRef && !early.creatorRef && !early.destroyed)
	early.release()
	require("terminal early-start state destroys only after release", early.destroyed)

	// Duplicate succeeds, but stop wins before PREPARED->ACTIVATING CAS.
	lost := prepare()
	lost.mtaInit()
	lost.createCallbackDuplicate()
	lost.stop()
	require("lost activation CAS preserves stop", !lost.activateCAS() && lost.state == stopping)
	require("lost activation CAS closes callback duplicate once",
		lost.callbackNotify.valid() && lost.callbackNotify.signals == 0)
	lost.captureNotify.signalAndClose()
	require("lost-CAS capture publisher owns one signal/close", lost.captureNotify.valid())
	require("mtaReady is monotonic after PREPARED", lost.mtaReady)

	// Normal callback hands off and closes its unused duplicate; thread publishes.
	normal := prepare()
	normal.mtaInit()
	normal.createCallbackDuplicate()
	require("normal activation CAS wins", normal.activateCAS())
	normal.callbackHandoff()
	require("callback handoff leaves state ACTIVATING", normal.state == activating)
	require("capture thread moves to CAPTURING only after Start", normal.startSucceeded() && normal.started && normal.state == capturing)
	normal.stop()
	normal.state = sealed
	normal.state = terminal
	normal.captureNotify.signalAndClose()
	require("normal callback closes without signal", normal.callbackNotify.valid() && normal.callbackNotify.signals == 0)
	require("normal capture thread signals/closes once", normal.captureNotify.valid() && normal.captureNotify.signals == 1)

	// Stop can win while Start is executing. A successful Start then requires
	// cleanup/Stop but must not expose CAPTURING.
	duringStart := prepare()
	duringStart.mtaInit()
	duringStart.createCallbackDuplicate()
	require("during-Start activation CAS wins", duringStart.activateCAS())
	duringStart.callbackHandoff()
	duringStart.stop()
	require("stop-winning Start CAS does not expose CAPTURING",
		!duringStart.startSucceeded() && duringStart.started && duringStart.state == stopping)
	duringStart.captureNotify.signalAndClose()
	require("stop-winning Start path still owns capture notification", duringStart.captureNotify.valid())

	// A synchronous launch failure has no callback: activation caller closes
	// its duplicate silently and the capture thread publishes.
	syncFail := prepare()
	syncFail.mtaInit()
	syncFail.createCallbackDuplicate()
	require("sync-failure activation CAS wins", syncFail.activateCAS())
	syncFail.callbackNotify.closeOnly()
	syncFail.stop()
	syncFail.captureNotify.signalAndClose()
	require("sync-failure duplicate ownership is exact",
		syncFail.callbackNotify.valid() && syncFail.callbackNotify.signals == 0 && syncFail.captureNotify.valid())

	// Diagram A: thread exits first and closes silently; callback publishes.
	a := prepare()
	a.mtaInit()
	a.createCallbackDuplicate()
	require("diagram A activation CAS wins", a.activateCAS())
	a.stop()
	a.captureNotify.closeOnly()
	a.callbackNotify.signalAndClose()
	a.state = terminal
	require("diagram A exact handle ownership", a.captureNotify.valid() && a.captureNotify.signals == 0 && a.callbackNotify.valid() && a.callbackNotify.signals == 1)

	// Diagram B: callback stores cause/closes silently; thread publishes.
	b := prepare()
	b.mtaInit()
	b.createCallbackDuplicate()
	require("diagram B activation CAS wins", b.activateCAS())
	b.stop()
	b.callbackNotify.closeOnly()
	b.captureNotify.signalAndClose()
	b.state = terminal
	require("diagram B exact handle ownership", b.callbackNotify.valid() && b.callbackNotify.signals == 0 && b.captureNotify.valid() && b.captureNotify.signals == 1)
}
