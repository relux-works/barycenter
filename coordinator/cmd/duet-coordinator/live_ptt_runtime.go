package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
	"relux.works/duet/coordinator/internal/store"
)

type livePTTBinarySender interface {
	TrySendBinary(key hub.NodeKey, raw []byte) bool
}

type livePTTHealthView struct {
	Enabled             bool   `json:"enabled"`
	ActiveSessions      int    `json:"active_sessions"`
	FramesRelayedTotal  uint64 `json:"frames_relayed_total"`
	TargetsDroppedTotal uint64 `json:"targets_dropped_total"`
	RetainedAudioBytes  int    `json:"retained_audio_bytes"`
}

func addLivePTTHealth(body map[string]any, runtime *session.LivePTTRuntime, enabled bool) {
	if runtime == nil {
		body["live_ptt"] = map[string]any{"enabled": false, "status": "unavailable"}
		return
	}
	metrics := runtime.Metrics()
	body["live_ptt"] = livePTTHealthView{Enabled: enabled, ActiveSessions: metrics.ActiveSessions,
		FramesRelayedTotal:  metrics.FramesRelayedTotal,
		TargetsDroppedTotal: metrics.TargetBackpressureTotal + metrics.TargetPolicyDropsTotal,
		RetainedAudioBytes:  metrics.RetainedAudioBytes}
}

func livePTTNode(key hub.NodeKey) session.LivePTTNode {
	return session.LivePTTNode{OrbitID: key.Orbit, Slot: key.Slot}
}

func livePTTHubKey(node session.LivePTTNode) hub.NodeKey {
	return hub.NodeKey{Orbit: node.OrbitID, Slot: node.Slot}
}

func (l *loop) handleLivePTTStart(key hub.NodeKey, credentialHash string, payload protocol.LivePTTStartPayload, now int64) {
	if !l.cfg.LivePTT {
		l.rejectLivePTTStart(key, payload, "unsupported", now)
		return
	}
	snapshots := l.hub.NodeSnapshots()
	availability := make([]store.LivePTTAvailability, 0, len(snapshots))
	for node, snapshot := range snapshots {
		availability = append(availability, store.LivePTTAvailability{
			OrbitID: node.Orbit, Slot: string(node.Slot), Connected: snapshot.Connected,
			LastSeenAt: snapshot.LastSeenAt, CredentialTokenHash: snapshot.CredentialTokenHash,
			SupportsLivePTT: snapshot.Capabilities.Supports(protocol.LivePTTCapability),
		})
	}
	resolution, err := l.st.ResolveLivePTTTargets(key.Orbit, string(key.Slot), credentialHash, availability, now)
	if err != nil {
		l.log.Warn("live PTT target resolution rejected", "reason", "policy")
		l.rejectLivePTTStart(key, payload, "policy", now)
		return
	}
	busy, err := l.st.HasActiveTransmissionRuntime(livePTTTransmissionDomainKind(resolution.DomainKind),
		resolution.DomainID)
	if err != nil {
		l.rejectLivePTTStart(key, payload, "policy", now)
		return
	}
	if busy {
		l.rejectLivePTTStart(key, payload, "busy", now)
		return
	}
	if payload.MixedVersionPolicy == protocol.LivePTTMixedVersionRequireAll && len(resolution.Excluded) != 0 {
		l.rejectLivePTTStart(key, payload, "unsupported", now)
		l.sendLivePTTExcludedReceipts(key, payload, resolution.Excluded, now)
		return
	}
	if len(resolution.Targets) == 0 {
		l.rejectLivePTTStart(key, payload, "policy", now)
		l.sendLivePTTExcludedReceipts(key, payload, resolution.Excluded, now)
		return
	}
	targets := make([]session.LivePTTTarget, 0, len(resolution.Targets))
	hashInput := ""
	for _, target := range resolution.Targets {
		targets = append(targets, session.LivePTTTarget{Node: session.LivePTTNode{
			OrbitID: target.OrbitID, Slot: protocol.NodeID(target.Slot)}, ActorID: target.ActorID})
		hashInput += fmt.Sprintf("%d/%s/%d\n", target.OrbitID, target.Slot, target.ActorID)
	}
	digest := sha256.Sum256([]byte(hashInput))
	randomSession := make([]byte, 16)
	if _, err := rand.Read(randomSession); err != nil {
		l.rejectLivePTTStart(key, payload, "policy", now)
		return
	}
	payload.SessionID = hex.EncodeToString(randomSession)
	nextGeneration := now * 1000
	if nextGeneration <= l.livePTTGeneration {
		nextGeneration = l.livePTTGeneration + 1
	}
	l.livePTTGeneration = nextGeneration
	payload.Generation = nextGeneration
	payload.SenderActorID = resolution.SourceActorID
	payload.SenderOrbitID = key.Orbit
	payload.SenderNodeID = string(key.Slot)
	payload.PlaybackDomain = resolution.DomainKind
	payload.PlaybackDomainID = resolution.DomainID
	payload.TargetCount = len(targets)
	payload.TargetSHA256 = hex.EncodeToString(digest[:])
	payload.TargetSnapshot = "lts1." + hex.EncodeToString(digest[:8])
	payload.StartedAtCoordMS = now
	payload.AcceptDeadlineCoordMS = now + protocol.LivePTTAcceptTimeoutMS
	payload.MaxDurationMS = protocol.LivePTTMaxDurationMS
	payload.CodecProfile = protocol.LivePTTCodecProfile
	payload.FrameMS = protocol.LivePTTFrameMS
	payload.MaxPayloadBytes = protocol.LivePTTMaxPayloadBytes
	payload.JitterBufferMS = protocol.LivePTTJitterBufferMS
	payload.LateJoinPolicy = protocol.LivePTTLateJoinPolicy
	payload.CaptureAuthority = protocol.LivePTTCaptureAuthority
	effects, err := l.livePTT.Start(session.LivePTTStart{Sender: livePTTNode(key),
		SenderActorID: resolution.SourceActorID, DomainKind: resolution.DomainKind,
		DomainID: resolution.DomainID, Payload: payload, Targets: targets, NowMS: now})
	if err != nil {
		code := "busy"
		if err != session.ErrLivePTTBusy {
			code = "unauthorized"
		}
		l.rejectLivePTTStart(key, payload, code, now)
		return
	}
	l.applyLivePTTEffects(effects)
	l.sendLivePTTExcludedReceipts(key, payload, resolution.Excluded, now)
}

func (l *loop) sendLivePTTExcludedReceipts(key hub.NodeKey, payload protocol.LivePTTStartPayload, excluded []store.LivePTTResolvedTarget, now int64) {
	for index, target := range excluded {
		state := "rejected"
		if target.Reason == "unsupported" {
			state = "unsupported"
		}
		receipt := protocol.LivePTTReceiptPayload{SessionID: payload.SessionID,
			Generation: payload.Generation, EventSequence: int64(index + 2), State: state,
			ObservedAtCoordMS: now}
		l.hub.Send(key, protocol.TypeLivePTTReceipt, &receipt)
		l.log.Info("live PTT target excluded", "reason", target.Reason)
	}
}

func (l *loop) rejectLivePTTStart(key hub.NodeKey, payload protocol.LivePTTStartPayload, code string, now int64) {
	reject := protocol.LivePTTRejectPayload{SessionID: payload.SessionID, Generation: payload.Generation,
		EventSequence: 1, Code: code, RejectedAtCoordMS: now}
	l.hub.Send(key, protocol.TypeLivePTTReject, &reject)
}

func (l *loop) handleLivePTTBinary(event hub.EvBinary) {
	if !l.cfg.LivePTT {
		return
	}
	now := event.ReceivedAtMS
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	effects, err := l.livePTT.RelayFrame(livePTTNode(event.Key), event.Frame, event.Raw, now)
	if err != nil {
		return
	}
	l.applyLivePTTEffects(effects)
}

func (l *loop) livePTTAvailability() []store.LivePTTAvailability {
	snapshots := l.hub.NodeSnapshots()
	result := make([]store.LivePTTAvailability, 0, len(snapshots))
	for node, snapshot := range snapshots {
		result = append(result, store.LivePTTAvailability{OrbitID: node.Orbit,
			Slot: string(node.Slot), Connected: snapshot.Connected, LastSeenAt: snapshot.LastSeenAt,
			CredentialTokenHash: snapshot.CredentialTokenHash,
			SupportsLivePTT:     snapshot.Capabilities.Supports(protocol.LivePTTCapability)})
	}
	return result
}

func (l *loop) auditLivePTTPolicy(now int64) {
	if !l.cfg.LivePTT {
		return
	}
	availability := l.livePTTAvailability()
	snapshots := l.hub.NodeSnapshots()
	for _, binding := range l.livePTT.ActiveBindings() {
		senderKey := livePTTHubKey(binding.Sender)
		sender := snapshots[senderKey]
		if !sender.Connected || sender.CredentialTokenHash == "" {
			l.applyLivePTTEffects(l.livePTT.Disconnect(binding.Sender, now))
			continue
		}
		resolution, err := l.st.ResolveLivePTTTargets(senderKey.Orbit, string(senderKey.Slot),
			sender.CredentialTokenHash, availability, now)
		if err != nil || resolution.DomainKind != binding.DomainKind || resolution.DomainID != binding.DomainID {
			l.applyLivePTTEffects(l.livePTT.Disconnect(binding.Sender, now))
			continue
		}
		allowed := map[session.LivePTTNode]bool{}
		for _, target := range resolution.Targets {
			allowed[session.LivePTTNode{OrbitID: target.OrbitID, Slot: protocol.NodeID(target.Slot)}] = true
		}
		for _, target := range binding.Targets {
			if !allowed[target] {
				l.applyLivePTTEffects(l.livePTT.TargetUnavailable(target,
					binding.SessionID, "policy_changed", now))
			}
		}
	}
}

func (l *loop) applyLivePTTEffects(effects []session.LivePTTEffect) {
	for len(effects) != 0 {
		effect := effects[0]
		effects = effects[1:]
		switch effect.Kind {
		case session.LivePTTSendSignal:
			if !l.hub.Send(livePTTHubKey(effect.To), effect.Type, effect.Payload) {
				effects = append(effects, l.livePTT.DeliveryUnavailable(effect.To,
					effect.SessionID, time.Now().UnixMilli())...)
			}
		case session.LivePTTSendBinary:
			sender, ok := l.hub.(livePTTBinarySender)
			if !ok || !sender.TrySendBinary(livePTTHubKey(effect.To), effect.Binary) {
				effects = append(effects, l.livePTT.TargetUnavailable(effect.To,
					effect.SessionID, "backpressure", time.Now().UnixMilli())...)
			}
		case session.LivePTTDuckStart:
			l.log.Info("live PTT duck boundary", "state", "start")
		case session.LivePTTDuckEnd:
			l.log.Info("live PTT duck boundary", "state", "release")
			l.signalTransmission(transmissionSignal{})
		}
	}
}

func livePTTTransmissionDomainKind(kind string) store.PlaybackDomainKind {
	if kind == "air" {
		return store.PlaybackDomainApproach
	}
	return store.PlaybackDomainOrbit
}

func (l *loop) livePTTBlocksTransmission(transmission store.Transmission) bool {
	kind := "barycenter"
	if transmission.PlaybackDomainKind == store.PlaybackDomainApproach {
		kind = "air"
	}
	return l.livePTT.DomainActive(kind, transmission.PlaybackDomainID)
}
