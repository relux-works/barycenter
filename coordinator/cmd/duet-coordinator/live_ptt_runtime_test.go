package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
	"relux.works/duet/coordinator/internal/store"
)

func livePTTCapabilitySet(t *testing.T) protocol.CapabilitySet {
	t.Helper()
	set, err := protocol.ParseCapabilitySet([]string{protocol.LivePTTCapability})
	if err != nil {
		t.Fatal(err)
	}
	return set
}
func livePTTTokenHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func TestLoopLivePTTSealsTargetsRelaysBinaryAndRecoversDuck(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("Live home")
	if err != nil {
		t.Fatal(err)
	}
	invite, err := harness.store.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	companion, err := harness.store.ConsumeDeviceInvite(invite.Code)
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t)
	cfg.LivePTT = true
	fake := &fakeSender{}
	l := newLoop(slog.Default(), cfg, fake, harness.store, nil, nil)
	l.warmup()
	l.handleNode(hub.EvOnline{Key: hub.NodeKey{Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot)}})
	l.handleNode(hub.EvOnline{Key: hub.NodeKey{Orbit: companion.OrbitID, Slot: protocol.NodeID(companion.Slot)}})
	now := time.Now().UnixMilli()
	caps := livePTTCapabilitySet(t)
	fake.snapshots = map[hub.NodeKey]hub.NodeSnapshot{
		{Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot)}:         {Connected: true, LastSeenAt: now, Capabilities: caps, CredentialTokenHash: livePTTTokenHash(owner.NodeToken)},
		{Orbit: companion.OrbitID, Slot: protocol.NodeID(companion.Slot)}: {Connected: true, LastSeenAt: now, Capabilities: caps, CredentialTokenHash: livePTTTokenHash(companion.NodeToken)},
	}
	payload := protocol.LivePTTStartPayload{SessionID: "00112233445566778899aabbccddeeff", Generation: 1, SenderActorID: 1, SenderOrbitID: 1, SenderNodeID: "a", TargetSnapshot: "lts1.request", TargetSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", TargetCount: 1, PlaybackDomain: "barycenter", PlaybackDomainID: 1, CodecProfile: protocol.LivePTTCodecProfile, FrameMS: 20, MaxPayloadBytes: 400, JitterBufferMS: 60, StartedAtCoordMS: now, AcceptDeadlineCoordMS: now + 1500, MaxDurationMS: 300000, MixedVersionPolicy: protocol.LivePTTMixedVersionReceipts, LateJoinPolicy: protocol.LivePTTLateJoinPolicy, CaptureAuthority: protocol.LivePTTCaptureAuthority}
	source := hub.NodeKey{Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot)}
	l.handleNodeMessage(hub.EvMessage{Key: source, CredentialTokenHash: fake.snapshots[source].CredentialTokenHash, Payload: &payload})
	starts := fake.ofType(protocol.TypeLivePTTStart)
	if len(starts) != 1 || starts[0].key.Slot != protocol.NodeID(companion.Slot) {
		var reject any
		if values := fake.ofType(protocol.TypeLivePTTReject); len(values) > 0 {
			reject = *values[0].payload.(*protocol.LivePTTRejectPayload)
		}
		t.Fatalf("sealed starts=%+v all=%+v reject=%+v", starts, fake.sent, reject)
	}
	sealed := starts[0].payload.(*protocol.LivePTTStartPayload)
	if sealed.TargetSnapshot == "lts1.request" || sealed.TargetCount != 1 || sealed.SenderActorID <= 0 {
		t.Fatalf("unsealed payload=%+v", sealed)
	}
	if sealed.SessionID == payload.SessionID || sealed.Generation == payload.Generation {
		t.Fatalf("coordinator reused caller identity: session=%s generation=%d", sealed.SessionID, sealed.Generation)
	}
	accept := protocol.LivePTTAcceptPayload{SessionID: sealed.SessionID, Generation: sealed.Generation, EventSequence: 1, AcceptedAtCoordMS: now + 1, LiveEdgeSequence: 1, BufferFrames: 3}
	targetKey := hub.NodeKey{Orbit: companion.OrbitID, Slot: protocol.NodeID(companion.Slot)}
	l.handleNodeMessage(hub.EvMessage{Key: targetKey, Payload: &accept})
	id, _ := protocol.ParseLivePTTSessionID(sealed.SessionID)
	frame := protocol.LivePTTBinaryFrame{Flags: protocol.LivePTTFlagStart | protocol.LivePTTFlagFEC, SessionID: id, Sequence: 1, CaptureMonotonicUS: 1000000, Payload: []byte{0xf8, 0xff, 0xfe}}
	raw, _ := protocol.EncodeLivePTTBinaryFrame(frame)
	l.handleLivePTTBinary(hub.EvBinary{Key: source, Frame: frame, Raw: raw})
	if got := fake.binary[targetKey]; len(got) != 1 || string(got[0]) != string(raw) {
		t.Fatalf("binary fanout=%x", got)
	}
	end := protocol.LivePTTEndPayload{SessionID: sealed.SessionID, Generation: sealed.Generation, CommandSequence: 2, LastSequence: 1, EndedAtCoordMS: now + 20, DrainDeadlineCoordMS: now + 620, Reason: "release"}
	l.handleNodeMessage(hub.EvMessage{Key: source, Payload: &end})
	if l.livePTT.Metrics().ActiveSessions != 0 || l.livePTT.Metrics().PersistedAudioBytes != 0 {
		t.Fatalf("live metrics=%+v", l.livePTT.Metrics())
	}
}

func TestLoopLivePTTPolicyAuditRemovesButNeverAddsTargets(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("Live policy home")
	if err != nil {
		t.Fatal(err)
	}
	invite, err := harness.store.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	companion, err := harness.store.ConsumeDeviceInvite(invite.Code)
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(t)
	cfg.LivePTT = true
	fake := &fakeSender{}
	l := newLoop(slog.Default(), cfg, fake, harness.store, nil, nil)
	l.warmup()
	now := time.Now().UnixMilli()
	caps := livePTTCapabilitySet(t)
	source := hub.NodeKey{Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot)}
	target := hub.NodeKey{Orbit: companion.OrbitID, Slot: protocol.NodeID(companion.Slot)}
	fake.snapshots = map[hub.NodeKey]hub.NodeSnapshot{
		source: {Connected: true, LastSeenAt: now, Capabilities: caps, CredentialTokenHash: livePTTTokenHash(owner.NodeToken)},
		target: {Connected: true, LastSeenAt: now, Capabilities: caps, CredentialTokenHash: livePTTTokenHash(companion.NodeToken)},
	}
	payload := protocol.LivePTTStartPayload{SessionID: "00112233445566778899aabbccddeeff", Generation: 1,
		MixedVersionPolicy: protocol.LivePTTMixedVersionReceipts}
	l.handleLivePTTStart(source, fake.snapshots[source].CredentialTokenHash, payload, now)
	starts := fake.ofType(protocol.TypeLivePTTStart)
	if len(starts) != 1 {
		t.Fatalf("starts=%+v", starts)
	}
	sealed := starts[0].payload.(*protocol.LivePTTStartPayload)
	accept := protocol.LivePTTAcceptPayload{SessionID: sealed.SessionID, Generation: sealed.Generation,
		EventSequence: 1, AcceptedAtCoordMS: now + 1, LiveEdgeSequence: 1, BufferFrames: 3}
	l.handleNodeMessage(hub.EvMessage{Key: target, Payload: &accept})

	targetSnapshot := fake.snapshots[target]
	targetSnapshot.Capabilities = protocol.CapabilitySet{}
	fake.snapshots[target] = targetSnapshot
	l.auditLivePTTPolicy(now + 2)
	if got := l.livePTT.Metrics(); got.ActiveSessions != 0 || got.TargetPolicyDropsTotal != 1 {
		t.Fatalf("policy audit metrics=%+v", got)
	}
	failed := fake.ofType(protocol.TypeLivePTTFailed)
	if len(failed) != 1 || failed[0].key != source || failed[0].payload.(*protocol.LivePTTFailedPayload).Code != "policy_changed" {
		t.Fatalf("policy failures=%+v", failed)
	}

	// Restoring support after the audit cannot repopulate the frozen session.
	targetSnapshot.Capabilities = caps
	fake.snapshots[target] = targetSnapshot
	l.auditLivePTTPolicy(now + 3)
	if len(fake.ofType(protocol.TypeLivePTTStart)) != 1 || l.livePTT.Metrics().ActiveSessions != 0 {
		t.Fatalf("policy audit resurrected a frozen session: sent=%+v", fake.sent)
	}
}

func TestLoopLivePTTFeatureOffIsExplicitAndDoesNotRelay(t *testing.T) {
	l, fake := newTestLoop(t)
	payload := protocol.LivePTTStartPayload{SessionID: "00112233445566778899aabbccddeeff", Generation: 1}
	l.handleLivePTTStart(hub.NodeKey{Orbit: 1, Slot: "a"}, livePTTTokenHash(l.cfg.Nodes["a"].Token), payload, time.Now().UnixMilli())
	rejects := fake.ofType(protocol.TypeLivePTTReject)
	if len(rejects) != 1 || rejects[0].payload.(*protocol.LivePTTRejectPayload).Code != "unsupported" {
		t.Fatalf("feature-off rejects=%+v", rejects)
	}
}

func TestLoopLivePTTSerializesWithDurableOverlayRuntime(t *testing.T) {
	l, fake, harness, owner, mediaItem := schedulerTestLoop(t)
	l.cfg.LivePTT = true
	invite, err := harness.store.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	companion, err := harness.store.ConsumeDeviceInvite(invite.Code)
	if err != nil {
		t.Fatal(err)
	}
	capabilities, err := protocol.ParseCapabilitySet([]string{
		protocol.CapabilityInterruptResume, protocol.LivePTTCapability,
		protocol.CapabilityMediaClip, protocol.CapabilityOverlayMix,
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	source := hub.NodeKey{Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot)}
	target := hub.NodeKey{Orbit: companion.OrbitID, Slot: protocol.NodeID(companion.Slot)}
	fake.snapshots[source] = hub.NodeSnapshot{Connected: true, LastSeenAt: now,
		Capabilities: capabilities, CredentialTokenHash: livePTTTokenHash(owner.NodeToken), RTTMS: 20, RTTSampledAt: now}
	fake.snapshots[target] = hub.NodeSnapshot{Connected: true, LastSeenAt: now,
		Capabilities: capabilities, CredentialTokenHash: livePTTTokenHash(companion.NodeToken), RTTMS: 20, RTTSampledAt: now}
	payload := protocol.LivePTTStartPayload{SessionID: "00112233445566778899aabbccddeeff", Generation: 1,
		MixedVersionPolicy: protocol.LivePTTMixedVersionReceipts}
	l.handleLivePTTStart(source, fake.snapshots[source].CredentialTokenHash, payload, now)
	starts := fake.ofType(protocol.TypeLivePTTStart)
	if len(starts) != 1 {
		t.Fatalf("starts=%+v", starts)
	}
	sealed := starts[0].payload.(*protocol.LivePTTStartPayload)
	accept := protocol.LivePTTAcceptPayload{SessionID: sealed.SessionID, Generation: sealed.Generation,
		EventSequence: 1, AcceptedAtCoordMS: now + 1, LiveEdgeSequence: 1, BufferFrames: 3}
	l.handleNodeMessage(hub.EvMessage{Key: target, Payload: &accept})
	created := runtimeTransmission(t, harness, owner, mediaItem, now+2, store.TransmissionDeliveryOverlay)
	l.runTransmissionScheduler(now + 3)
	if prepares := fake.ofType(protocol.TypePrepareMedia); len(prepares) != 0 {
		t.Fatalf("overlay crossed active live boundary: %+v", prepares)
	}
	end := protocol.LivePTTEndPayload{SessionID: sealed.SessionID, Generation: sealed.Generation,
		CommandSequence: 2, LastSequence: 1, EndedAtCoordMS: now + 4,
		DrainDeadlineCoordMS: now + 4 + protocol.LivePTTDrainTimeoutMS, Reason: "release"}
	l.handleNodeMessage(hub.EvMessage{Key: source, Payload: &end})

	l.handleLivePTTStart(source, fake.snapshots[source].CredentialTokenHash, payload, now+5)
	rejects := fake.ofType(protocol.TypeLivePTTReject)
	if len(rejects) == 0 || rejects[len(rejects)-1].payload.(*protocol.LivePTTRejectPayload).Code != "busy" {
		t.Fatalf("durable overlay did not win serialization: %+v", rejects)
	}
	l.runTransmissionScheduler(now + 6)
	prepares := fake.ofType(protocol.TypePrepareMedia)
	if len(prepares) != 1 || prepares[0].payload.(*protocol.PrepareMediaPayload).TransmissionID != created.Transmission.ID {
		t.Fatalf("overlay did not resume after live release: %+v", prepares)
	}
}

func TestLivePTTHealthContainsOnlyBoundedMetadata(t *testing.T) {
	body := map[string]any{"status": "ok"}
	addLivePTTHealth(body, session.NewLivePTTRuntime(), false)
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, forbidden := range []string{"payload", "chunk", "codec_bytes"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("health leaked %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, `"enabled":false`) || !strings.Contains(text, `"retained_audio_bytes":0`) {
		t.Fatalf("health=%s", text)
	}
}
