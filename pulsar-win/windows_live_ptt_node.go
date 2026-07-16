package main

import (
	"context"
	"sync"

	protocol "relux.works/duet/pulsar-win/wire"
)

type windowsLiveCaptureSending interface {
	SetEventHandler(func(WindowsLiveCaptureEvent))
	Snapshot() WindowsLiveCaptureSnapshot
	LocalHoldBegan(WindowsLiveHoldSource, bool, string) (uint64, bool)
	LocalHoldHeartbeat(uint64)
	AcceptStart(context.Context, protocol.LivePTTStartPayload, uint64, bool) error
	LocalHoldEnded(uint64)
	LocalStop()
	HandleSessionLock()
	HandleSuspend()
	HandlePermissionRevoke()
	HandleDeviceLoss()
	HandleDisconnect()
	CoordinatorCancelled()
	Shutdown()
}

type windowsLiveJitterReceiving interface {
	Start(protocol.LivePTTStartPayload, bool) bool
	Receive(protocol.LivePTTBinaryFrame) protocol.LivePTTFrameDecision
	End(protocol.LivePTTEndPayload)
	Cancel(protocol.LivePTTCancelPayload)
	Revoke()
	Snapshot() WindowsLiveJitterSnapshot
}

type WindowsLivePTTDirection string

const (
	WindowsLivePTTIdle      WindowsLivePTTDirection = "idle"
	WindowsLivePTTSending   WindowsLivePTTDirection = "sending"
	WindowsLivePTTReceiving WindowsLivePTTDirection = "receiving"
)

type WindowsLivePTTPhase string

const (
	WindowsLivePTTPhaseIdle             WindowsLivePTTPhase = "idle"
	WindowsLivePTTPhaseFallback         WindowsLivePTTPhase = "fallback"
	WindowsLivePTTPhaseAwaitingSession  WindowsLivePTTPhase = "awaiting_session"
	WindowsLivePTTPhaseAwaitingReceiver WindowsLivePTTPhase = "awaiting_receiver"
	WindowsLivePTTPhaseCapturing        WindowsLivePTTPhase = "capturing"
	WindowsLivePTTPhaseBuffering        WindowsLivePTTPhase = "buffering"
	WindowsLivePTTPhasePlaying          WindowsLivePTTPhase = "playing"
	WindowsLivePTTPhaseStopping         WindowsLivePTTPhase = "stopping"
	WindowsLivePTTPhaseRejected         WindowsLivePTTPhase = "rejected"
	WindowsLivePTTPhaseFailed           WindowsLivePTTPhase = "failed"
)

type WindowsLivePTTStatus struct {
	Direction         WindowsLivePTTDirection
	Phase             WindowsLivePTTPhase
	SessionID         string
	Generation        int64
	AcceptedReceivers int
	RejectedReceivers int
	LastError         string
	FallbackToClip    bool
}

type WindowsLivePTTIncomingDecision struct {
	Allow bool
	Code  string
}

type windowsLivePTTOutgoing struct {
	localGeneration  uint64
	payload          protocol.LivePTTStartPayload
	captureRequested bool
	accepted         int
	rejected         int
}

// WindowsLivePTTNode is the production-dark composition boundary for the
// reviewed sender, receiver and websocket seams. Target snapshots and incoming
// DND/policy decisions remain injected, so the node cannot broaden an audience
// or authorize capture/playback locally.
type WindowsLivePTTNode struct {
	mu                sync.Mutex
	sender            windowsLiveCaptureSending
	receiver          windowsLiveJitterReceiving
	featureEnabled    func() bool
	prepareStart      func(uint64, WindowsLiveHoldSource) (protocol.LivePTTStartPayload, bool)
	authorizeIncoming func(protocol.LivePTTStartPayload) WindowsLivePTTIncomingDecision
	coordinatorNowMS  func() int64
	send              func(string, any)
	outgoing          *windowsLivePTTOutgoing
	holdClaim         bool
	receiveClaim      bool
	status            WindowsLivePTTStatus
}

func NewWindowsLivePTTNode(
	sender windowsLiveCaptureSending,
	receiver windowsLiveJitterReceiving,
	featureEnabled func() bool,
	prepareStart func(uint64, WindowsLiveHoldSource) (protocol.LivePTTStartPayload, bool),
	authorizeIncoming func(protocol.LivePTTStartPayload) WindowsLivePTTIncomingDecision,
	coordinatorNowMS func() int64,
	send func(string, any),
) *WindowsLivePTTNode {
	if sender == nil || receiver == nil || featureEnabled == nil || prepareStart == nil ||
		authorizeIncoming == nil || coordinatorNowMS == nil || send == nil {
		return nil
	}
	node := &WindowsLivePTTNode{
		sender: sender, receiver: receiver, featureEnabled: featureEnabled,
		prepareStart: prepareStart, authorizeIncoming: authorizeIncoming,
		coordinatorNowMS: coordinatorNowMS, send: send,
		status: WindowsLivePTTStatus{Direction: WindowsLivePTTIdle, Phase: WindowsLivePTTPhaseIdle},
	}
	sender.SetEventHandler(node.consumeSender)
	return node
}

func (n *WindowsLivePTTNode) Snapshot() WindowsLivePTTStatus {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.status
}

func (n *WindowsLivePTTNode) HoldBegan(source WindowsLiveHoldSource, holdAvailable bool, deviceID string) (uint64, bool) {
	if n == nil {
		return 0, false
	}
	n.mu.Lock()
	busy := n.holdClaim || n.receiveClaim || n.outgoing != nil ||
		n.sender.Snapshot().Phase != WindowsLiveCaptureIdle ||
		n.receiver.Snapshot().Phase != WindowsLiveIdle
	if busy {
		if n.status.Direction == WindowsLivePTTIdle {
			n.status.Phase = WindowsLivePTTPhaseRejected
		}
		n.status.LastError = "busy"
		n.mu.Unlock()
		return 0, false
	}
	enabled := n.featureEnabled() && holdAvailable
	n.holdClaim = enabled
	if enabled {
		n.status = WindowsLivePTTStatus{Direction: WindowsLivePTTSending, Phase: WindowsLivePTTPhaseAwaitingSession}
	}
	n.mu.Unlock()
	generation, accepted := n.sender.LocalHoldBegan(source, enabled, deviceID)
	n.mu.Lock()
	claimSurvived := n.holdClaim
	if (!accepted || !claimSurvived) && enabled {
		n.holdClaim = false
		if n.status.Direction == WindowsLivePTTSending {
			n.status = WindowsLivePTTStatus{Direction: WindowsLivePTTIdle, Phase: WindowsLivePTTPhaseRejected, LastError: "busy"}
		}
	}
	n.mu.Unlock()
	if accepted && !claimSurvived {
		n.sender.LocalStop()
		return 0, false
	}
	return generation, accepted
}

func (n *WindowsLivePTTNode) HoldHeartbeat(generation uint64) {
	n.sender.LocalHoldHeartbeat(generation)
}

func (n *WindowsLivePTTNode) HoldEnded(generation uint64) { n.sender.LocalHoldEnded(generation) }
func (n *WindowsLivePTTNode) LocalStop()                  { n.sender.LocalStop() }

func (n *WindowsLivePTTNode) Handle(payload any) {
	if n == nil {
		return
	}
	switch value := payload.(type) {
	case *protocol.LivePTTStartPayload:
		n.handleIncomingStart(*value)
	case *protocol.LivePTTAcceptPayload:
		n.handleAccept(*value)
	case *protocol.LivePTTRejectPayload:
		n.handleReject(*value)
	case *protocol.LivePTTEndPayload:
		n.receiver.End(*value)
		n.updateReceiverStatus("")
	case *protocol.LivePTTCancelPayload:
		if n.matchesOutgoing(value.SessionID, value.Generation) {
			n.sender.CoordinatorCancelled()
		}
		n.receiver.Cancel(*value)
		n.updateReceiverStatus("")
	case *protocol.LivePTTFailedPayload:
		n.handleFailed(*value)
	case *protocol.LivePTTReceiptPayload:
		n.handleReceipt(*value)
	case *protocol.LivePTTStatePayload:
		n.handleState(*value)
	}
}

func (n *WindowsLivePTTNode) HandleFrame(frame protocol.LivePTTBinaryFrame) {
	if !n.featureEnabled() || n.receiver.Receive(frame) != protocol.LivePTTFrameApply {
		return
	}
	n.updateReceiverStatus("")
}

func (n *WindowsLivePTTNode) HandleSessionLock() {
	n.sender.HandleSessionLock()
	n.receiver.Revoke()
	n.reset("session_locked")
}

func (n *WindowsLivePTTNode) HandleSuspend() {
	n.sender.HandleSuspend()
	n.receiver.Revoke()
	n.reset("system_suspend")
}

func (n *WindowsLivePTTNode) HandlePermissionRevoke() {
	n.sender.HandlePermissionRevoke()
	n.receiver.Revoke()
	n.reset("permission_revoked")
}

func (n *WindowsLivePTTNode) HandleDeviceLoss() {
	n.sender.HandleDeviceLoss()
	n.receiver.Revoke()
	n.reset("device_lost")
}

func (n *WindowsLivePTTNode) HandleDisconnect() {
	n.sender.HandleDisconnect()
	n.receiver.Revoke()
	n.reset("disconnect")
}

func (n *WindowsLivePTTNode) RollbackFeature() {
	n.sender.CoordinatorCancelled()
	n.receiver.Revoke()
	n.reset("feature_rollback")
}

func (n *WindowsLivePTTNode) Shutdown() {
	n.sender.Shutdown()
	n.receiver.Revoke()
	n.reset("")
}

func (n *WindowsLivePTTNode) consumeSender(event WindowsLiveCaptureEvent) {
	switch event.Kind {
	case WindowsLiveCaptureRequestEvent:
		n.requestStart(event.Generation, event.Source)
	case WindowsLiveCapturePhaseEvent:
		n.mu.Lock()
		if event.Phase != WindowsLiveCaptureIdle || n.outgoing != nil {
			n.status.Direction = WindowsLivePTTSending
			switch event.Phase {
			case WindowsLiveCaptureAwaiting:
				n.status.Phase = WindowsLivePTTPhaseAwaitingSession
			case WindowsLiveCapturePermission:
				n.status.Phase = WindowsLivePTTPhaseAwaitingReceiver
			case WindowsLiveCaptureActive:
				n.status.Phase = WindowsLivePTTPhaseCapturing
			case WindowsLiveCaptureStopping:
				n.status.Phase = WindowsLivePTTPhaseStopping
			}
		}
		n.mu.Unlock()
	case WindowsLiveCaptureFallbackEvent:
		n.mu.Lock()
		n.holdClaim = false
		n.status = WindowsLivePTTStatus{Direction: WindowsLivePTTIdle, Phase: WindowsLivePTTPhaseFallback, LastError: "hold_unavailable", FallbackToClip: true}
		n.mu.Unlock()
	case WindowsLiveCaptureTerminalEvent:
		n.mu.Lock()
		n.outgoing = nil
		n.holdClaim = false
		if n.status.Phase == WindowsLivePTTPhaseFailed &&
			(event.Reason == WindowsLiveCaptureLocalStop || event.Reason == WindowsLiveCaptureCoordinator) {
			n.mu.Unlock()
			return
		}
		errorCode := ""
		if event.Reason != WindowsLiveCaptureReleased {
			errorCode = string(event.Reason)
		}
		n.status = WindowsLivePTTStatus{Direction: WindowsLivePTTIdle, Phase: WindowsLivePTTPhaseIdle, LastError: errorCode}
		n.mu.Unlock()
	}
}

func (n *WindowsLivePTTNode) requestStart(localGeneration uint64, source WindowsLiveHoldSource) {
	n.mu.Lock()
	if !n.featureEnabled() || !n.holdClaim || n.receiveClaim || n.outgoing != nil ||
		n.receiver.Snapshot().Phase != WindowsLiveIdle {
		n.mu.Unlock()
		n.sender.LocalStop()
		return
	}
	payload, ok := n.prepareStart(localGeneration, source)
	if !ok || protocol.ValidateLivePTTStartPayload(payload) != nil {
		n.status = WindowsLivePTTStatus{Direction: WindowsLivePTTIdle, Phase: WindowsLivePTTPhaseFailed, LastError: "target_or_policy_unavailable"}
		n.mu.Unlock()
		n.sender.LocalStop()
		return
	}
	n.outgoing = &windowsLivePTTOutgoing{localGeneration: localGeneration, payload: payload}
	n.status = WindowsLivePTTStatus{Direction: WindowsLivePTTSending, Phase: WindowsLivePTTPhaseAwaitingReceiver, SessionID: payload.SessionID, Generation: payload.Generation}
	n.mu.Unlock()
	n.send(protocol.TypeLivePTTStart, payload)
}

func (n *WindowsLivePTTNode) handleIncomingStart(payload protocol.LivePTTStartPayload) {
	if !n.featureEnabled() {
		n.reject(payload, "unsupported")
		return
	}
	n.mu.Lock()
	busy := n.holdClaim || n.receiveClaim || n.outgoing != nil ||
		n.sender.Snapshot().Phase != WindowsLiveCaptureIdle ||
		n.receiver.Snapshot().Phase != WindowsLiveIdle
	if !busy {
		n.receiveClaim = true
	}
	if busy {
		n.mu.Unlock()
		n.reject(payload, "busy")
		return
	}
	decision := n.authorizeIncoming(payload)
	if !decision.Allow {
		n.receiveClaim = false
		n.mu.Unlock()
		n.reject(payload, allowedWindowsLiveReject(decision.Code))
		return
	}
	if n.receiver.Start(payload, true) {
		n.status = WindowsLivePTTStatus{Direction: WindowsLivePTTReceiving, Phase: WindowsLivePTTPhaseBuffering, SessionID: payload.SessionID, Generation: payload.Generation}
		n.mu.Unlock()
		return
	}
	n.receiveClaim = false
	n.mu.Unlock()
}

func (n *WindowsLivePTTNode) handleAccept(payload protocol.LivePTTAcceptPayload) {
	if protocol.ValidateLivePTTAcceptPayload(payload) != nil {
		return
	}
	n.mu.Lock()
	active := n.outgoing
	if active == nil || active.payload.SessionID != payload.SessionID || active.payload.Generation != payload.Generation {
		n.mu.Unlock()
		return
	}
	startCapture := !active.captureRequested
	active.captureRequested = true
	active.accepted++
	n.status.AcceptedReceivers = active.accepted
	localGeneration, start := active.localGeneration, active.payload
	n.mu.Unlock()
	if !startCapture {
		return
	}
	go func() {
		if err := n.sender.AcceptStart(context.Background(), start, localGeneration, true); err != nil {
			n.mu.Lock()
			matches := n.outgoing != nil && n.outgoing.payload.SessionID == start.SessionID && n.outgoing.payload.Generation == start.Generation
			if matches {
				n.status.Phase = WindowsLivePTTPhaseFailed
				n.status.LastError = "capture_start_failed"
			}
			n.mu.Unlock()
			if matches {
				n.sender.CoordinatorCancelled()
			}
		}
	}()
}

func (n *WindowsLivePTTNode) handleReject(payload protocol.LivePTTRejectPayload) {
	if protocol.ValidateLivePTTRejectPayload(payload) != nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.outgoing == nil || n.outgoing.payload.SessionID != payload.SessionID || n.outgoing.payload.Generation != payload.Generation {
		return
	}
	n.outgoing.rejected++
	n.status.RejectedReceivers = n.outgoing.rejected
	n.status.LastError = payload.Code
}

func (n *WindowsLivePTTNode) handleFailed(payload protocol.LivePTTFailedPayload) {
	if protocol.ValidateLivePTTFailedPayload(payload) != nil {
		return
	}
	if n.matchesOutgoing(payload.SessionID, payload.Generation) {
		n.sender.CoordinatorCancelled()
		n.mu.Lock()
		n.status.Phase, n.status.LastError = WindowsLivePTTPhaseFailed, payload.Code
		n.mu.Unlock()
		return
	}
	if receiver := n.receiver.Snapshot(); receiver.SessionID == payload.SessionID && receiver.Generation == payload.Generation {
		n.receiver.Revoke()
		n.reset(payload.Code)
	}
}

func (n *WindowsLivePTTNode) handleReceipt(payload protocol.LivePTTReceiptPayload) {
	if protocol.ValidateLivePTTReceiptPayload(payload) != nil || !n.matchesOutgoing(payload.SessionID, payload.Generation) {
		return
	}
	if payload.State == "failed" || payload.State == "cancelled" {
		n.mu.Lock()
		n.status.LastError = payload.State
		n.mu.Unlock()
	}
}

func (n *WindowsLivePTTNode) handleState(payload protocol.LivePTTStatePayload) {
	if protocol.ValidateLivePTTStatePayload(payload) != nil {
		return
	}
	if payload.Phase != "idle" && payload.Phase != "cancelled" && payload.Phase != "terminal" {
		return
	}
	n.mu.Lock()
	active := n.outgoing
	shouldCancel := active != nil && (payload.ActiveSessionID == "" || payload.ActiveSessionID == active.payload.SessionID)
	n.mu.Unlock()
	if shouldCancel {
		n.sender.CoordinatorCancelled()
	}
}

func (n *WindowsLivePTTNode) reject(payload protocol.LivePTTStartPayload, code string) {
	rejection := protocol.LivePTTRejectPayload{
		SessionID: payload.SessionID, Generation: payload.Generation,
		EventSequence: 1, Code: code, RejectedAtCoordMS: max(int64(1), n.coordinatorNowMS()),
	}
	if protocol.ValidateLivePTTRejectPayload(rejection) == nil {
		n.send(protocol.TypeLivePTTReject, rejection)
	}
}

func allowedWindowsLiveReject(code string) string {
	switch code {
	case "blocked", "busy", "dnd", "expired", "policy", "unauthorized", "unsupported":
		return code
	default:
		return "policy"
	}
}

func (n *WindowsLivePTTNode) matchesOutgoing(sessionID string, generation int64) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.outgoing != nil && n.outgoing.payload.SessionID == sessionID && n.outgoing.payload.Generation == generation
}

func (n *WindowsLivePTTNode) updateReceiverStatus(errorCode string) {
	receiver := n.receiver.Snapshot()
	n.mu.Lock()
	defer n.mu.Unlock()
	if receiver.Phase == WindowsLiveIdle {
		n.receiveClaim = false
		phase := WindowsLivePTTPhaseIdle
		if errorCode != "" {
			phase = WindowsLivePTTPhaseFailed
		}
		n.status = WindowsLivePTTStatus{Direction: WindowsLivePTTIdle, Phase: phase, LastError: errorCode}
		return
	}
	phase := WindowsLivePTTPhaseBuffering
	if receiver.Phase == WindowsLivePlaying {
		phase = WindowsLivePTTPhasePlaying
	} else if receiver.Phase == WindowsLiveDraining {
		phase = WindowsLivePTTPhaseStopping
	}
	n.status = WindowsLivePTTStatus{Direction: WindowsLivePTTReceiving, Phase: phase, SessionID: receiver.SessionID, Generation: receiver.Generation, LastError: errorCode}
}

func (n *WindowsLivePTTNode) reset(errorCode string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.outgoing = nil
	n.holdClaim = false
	n.receiveClaim = false
	phase := WindowsLivePTTPhaseIdle
	if errorCode != "" {
		phase = WindowsLivePTTPhaseFailed
	}
	n.status = WindowsLivePTTStatus{Direction: WindowsLivePTTIdle, Phase: phase, LastError: errorCode}
}
