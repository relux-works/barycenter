package main

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
	"relux.works/duet/coordinator/internal/store"
)

func schedulerTestLoop(
	t *testing.T,
) (*loop, *fakeSender, onboardingHarness, store.OnboardingCredentials, store.MediaItem) {
	t.Helper()
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("Scheduler runtime owner")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	mediaItem := readyTransmissionHTTPMedia(t, harness, owner, now, 1200)
	cfg := testConfig(t)
	cfg.PublicURL = "https://coord.example/"
	fake := &fakeSender{snapshots: make(map[hub.NodeKey]hub.NodeSnapshot)}
	l := newLoop(slog.Default(), cfg, fake, harness.store, nil, nil)
	// Runtime tests supply exact synthetic coordinator timestamps explicitly.
	l.transmissionNow = func() int64 { return 0 }
	t.Cleanup(func() {
		if l.transmissionTimer != nil {
			l.transmissionTimer.Stop()
		}
	})
	return l, fake, harness, owner, mediaItem
}

func schedulerCapabilities(t *testing.T) protocol.CapabilitySet {
	t.Helper()
	capabilities, err := protocol.ParseCapabilitySet([]string{
		protocol.CapabilityInterruptResume,
		protocol.CapabilityMediaClip,
		protocol.CapabilityOverlayMix,
	})
	if err != nil {
		t.Fatal(err)
	}
	return capabilities
}

func tokenWitness(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func runtimeTransmission(
	t *testing.T,
	harness onboardingHarness,
	owner store.OnboardingCredentials,
	mediaItem store.MediaItem,
	acceptedAt int64,
	delivery store.TransmissionDelivery,
) store.TransmissionCreation {
	t.Helper()
	params := store.CreateTransmissionParams{
		MediaID: mediaItem.ID, SourceOrbitID: owner.OrbitID,
		SourceActorID: owner.ActorID, SourceSlot: owner.Slot,
		PlaybackDomainKind: store.PlaybackDomainOrbit, PlaybackDomainID: owner.OrbitID,
		AudienceKind: store.TransmissionAudienceThisPulsar,
		OriginKind:   store.TransmissionOriginFile, IncludeOrigin: true,
		RequestedDelivery: delivery, EffectiveDelivery: delivery,
		AcceptedAt: acceptedAt,
		Targets: []store.CreateTransmissionTarget{{
			OrbitID: owner.OrbitID, ActorID: owner.ActorID, Slot: owner.Slot,
			OnlineAtAcceptance: true, MediaClipCapable: true,
			OverlayCapable: true, InterruptCapable: true,
			InterruptResumeReady: true,
		}},
	}
	created, err := harness.store.CreateTransmission(params)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func TestOverlayControllerRuntimeSendsExactPrepareAndRTTSchedule(t *testing.T) {
	l, fake, harness, owner, mediaItem := schedulerTestLoop(t)
	now := time.Now().UnixMilli()
	created := runtimeTransmission(
		t, harness, owner, mediaItem, now+3, store.TransmissionDeliveryOverlay,
	)
	key := hub.NodeKey{Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot)}
	witness := tokenWitness(owner.NodeToken)
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 4,
		Capabilities:        schedulerCapabilities(t),
		CredentialTokenHash: tokenWitness(owner.NodeToken),
		RTTMS:               80, RTTSampledAt: now + 4,
	}
	l.runTransmissionScheduler(now + 4)
	prepareMessages := fake.ofType(protocol.TypePrepareMedia)
	if len(prepareMessages) != 1 {
		t.Fatalf("prepare messages=%+v", prepareMessages)
	}
	prepare := prepareMessages[0].payload.(*protocol.PrepareMediaPayload)
	if prepare.TransmissionID != created.Transmission.ID || prepare.Generation != 1 ||
		prepare.FileURL != "https://coord.example/v1/media/"+mediaItem.ID ||
		prepare.SHA256 != mediaItem.SHA256 || prepare.SizeBytes != mediaItem.SizeBytes ||
		prepare.DurationMS != mediaItem.DurationMS ||
		prepare.PrepareDeadlineCoordMS != now+4+store.TransmissionPrepareBarrierMS {
		t.Fatalf("prepare payload=%+v", prepare)
	}
	l.handleMediaReady(key, witness, &protocol.MediaReadyPayload{
		TransmissionID: created.Transmission.ID, Generation: 1,
		DecodedDurationMS: mediaItem.DurationMS,
	})
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 10,
		Capabilities:        schedulerCapabilities(t),
		CredentialTokenHash: tokenWitness(owner.NodeToken),
		RTTMS:               80, RTTSampledAt: now + 10,
	}
	l.runTransmissionScheduler(now + 10)
	playMessages := fake.ofType(protocol.TypePlayMediaAt)
	if len(playMessages) != 1 {
		t.Fatalf("play messages=%+v", playMessages)
	}
	play := playMessages[0].payload.(*protocol.PlayMediaAtPayload)
	if play.TransmissionID != created.Transmission.ID || play.Generation != 1 ||
		play.TCoordMS != now+510 || play.StartDeadlineCoordMS != now+610 ||
		play.Delivery != string(store.TransmissionDeliveryOverlay) ||
		play.DuckDB == nil || *play.DuckDB != -12 ||
		play.AttackMS == nil || *play.AttackMS != 250 ||
		play.ReleaseMS == nil || *play.ReleaseMS != 600 ||
		play.FadeOutMS != nil || play.FadeInMS != nil {
		t.Fatalf("play payload=%+v", play)
	}
	// A forged/out-of-order completion cannot skip the durable playing edge.
	l.handleMediaEnded(key, witness, &protocol.MediaEndedPayload{
		TransmissionID: created.Transmission.ID, Generation: 1,
		TLastSampleCoordMS: play.TCoordMS + mediaItem.DurationMS,
		Reason:             string(store.TransmissionReasonCompleted),
	})
	beforeStart, err := harness.store.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil || beforeStart.Targets[0].Status != store.TransmissionTargetScheduled {
		t.Fatalf("out-of-order completion=%+v err=%v", beforeStart, err)
	}
	l.handleMediaStarted(key, witness, &protocol.MediaStartedPayload{
		TransmissionID: created.Transmission.ID, Generation: 1,
		TFirstSampleCoordMS: play.TCoordMS,
	})
	l.handleMediaEnded(key, witness, &protocol.MediaEndedPayload{
		TransmissionID: created.Transmission.ID, Generation: 1,
		TLastSampleCoordMS: play.TCoordMS + mediaItem.DurationMS,
		Reason:             string(store.TransmissionReasonCompleted),
	})
	work, err := harness.store.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil {
		t.Fatal(err)
	}
	if work.Transmission.Status != store.TransmissionStatusPlayed ||
		work.Targets[0].Status != store.TransmissionTargetPlayed {
		t.Fatalf("terminal work=%+v", work)
	}
	// Exact replay is ignored after the generation reached a terminal state.
	l.handleMediaEnded(key, witness, &protocol.MediaEndedPayload{
		TransmissionID: created.Transmission.ID, Generation: 1,
		TLastSampleCoordMS: play.TCoordMS + mediaItem.DurationMS,
		Reason:             string(store.TransmissionReasonCompleted),
	})
}

func TestOverlayControllerMultiTargetBarrierUsesFreshMaximumRTTAndOneT(t *testing.T) {
	l, fake, harness, owner, mediaItem := schedulerTestLoop(t)
	companion := installTransmissionCompanion(t, harness, owner)
	now := time.Now().UnixMilli()
	created, err := harness.store.CreateTransmission(store.CreateTransmissionParams{
		MediaID: mediaItem.ID, SourceOrbitID: owner.OrbitID,
		SourceActorID: owner.ActorID, SourceSlot: owner.Slot,
		PlaybackDomainKind: store.PlaybackDomainOrbit, PlaybackDomainID: owner.OrbitID,
		AudienceKind: store.TransmissionAudienceOwnBarycenter,
		OriginKind:   store.TransmissionOriginFile, IncludeOrigin: true,
		RequestedDelivery: store.TransmissionDeliveryOverlay,
		EffectiveDelivery: store.TransmissionDeliveryOverlay,
		AcceptedAt:        now + 3,
		Targets: []store.CreateTransmissionTarget{
			{
				OrbitID: owner.OrbitID, ActorID: owner.ActorID, Slot: owner.Slot,
				OnlineAtAcceptance: true, MediaClipCapable: true, OverlayCapable: true,
			},
			{
				OrbitID: companion.OrbitID, ActorID: companion.ActorID, Slot: companion.Slot,
				OnlineAtAcceptance: true, MediaClipCapable: true, OverlayCapable: true,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	barrierAt := now + 1000
	targets := []struct {
		credentials store.OnboardingCredentials
		rtt         int64
	}{
		{owner, 20},
		{companion, 140},
	}
	for _, target := range targets {
		key := hub.NodeKey{
			Orbit: target.credentials.OrbitID,
			Slot:  protocol.NodeID(target.credentials.Slot),
		}
		fake.snapshots[key] = hub.NodeSnapshot{
			Connected: true, LastSeenAt: barrierAt,
			Capabilities:        schedulerCapabilities(t),
			CredentialTokenHash: tokenWitness(target.credentials.NodeToken),
			RTTMS:               target.rtt, RTTSampledAt: barrierAt,
		}
	}
	l.runTransmissionScheduler(barrierAt)
	if got := len(fake.ofType(protocol.TypePrepareMedia)); got != 2 {
		t.Fatalf("multi-target prepare messages=%+v", fake.sent)
	}
	for _, target := range targets {
		key := hub.NodeKey{
			Orbit: target.credentials.OrbitID,
			Slot:  protocol.NodeID(target.credentials.Slot),
		}
		l.handleMediaReady(key, tokenWitness(target.credentials.NodeToken),
			&protocol.MediaReadyPayload{
				TransmissionID: created.Transmission.ID, Generation: 1,
				DecodedDurationMS: mediaItem.DurationMS,
			})
	}
	decisionAt := barrierAt + 10
	for _, target := range targets {
		key := hub.NodeKey{
			Orbit: target.credentials.OrbitID,
			Slot:  protocol.NodeID(target.credentials.Slot),
		}
		snapshot := fake.snapshots[key]
		snapshot.LastSeenAt = decisionAt
		snapshot.RTTSampledAt = decisionAt
		fake.snapshots[key] = snapshot
	}
	l.runTransmissionScheduler(decisionAt)
	plays := fake.ofType(protocol.TypePlayMediaAt)
	if len(plays) != 2 {
		t.Fatalf("multi-target play messages=%+v", fake.sent)
	}
	wantT := decisionAt + 530 // 2*max(20, 140) + 250 ms.
	seen := make(map[protocol.NodeID]bool, 2)
	for _, message := range plays {
		payload := message.payload.(*protocol.PlayMediaAtPayload)
		if payload.TransmissionID != created.Transmission.ID ||
			payload.TCoordMS != wantT || payload.StartDeadlineCoordMS != wantT+100 {
			t.Fatalf("multi-target schedule node=%s payload=%+v", message.node, payload)
		}
		seen[message.node] = true
	}
	if !seen[protocol.NodeID(owner.Slot)] || !seen[protocol.NodeID(companion.Slot)] {
		t.Fatalf("multi-target schedule recipients=%v", seen)
	}
}

func TestOverlayControllerRejectsEarlyCompletedReceipt(t *testing.T) {
	l, fake, harness, owner, mediaItem := schedulerTestLoop(t)
	now := time.Now().UnixMilli()
	created := runtimeTransmission(
		t, harness, owner, mediaItem, now, store.TransmissionDeliveryOverlay,
	)
	key := hub.NodeKey{Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot)}
	witness := tokenWitness(owner.NodeToken)
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 1,
		Capabilities: schedulerCapabilities(t), CredentialTokenHash: witness,
		RTTMS: 10, RTTSampledAt: now + 1,
	}
	l.runTransmissionScheduler(now + 1)
	l.handleMediaReady(key, witness, &protocol.MediaReadyPayload{
		TransmissionID: created.Transmission.ID, Generation: 1,
		DecodedDurationMS: mediaItem.DurationMS,
	})
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 2,
		Capabilities: schedulerCapabilities(t), CredentialTokenHash: witness,
		RTTMS: 10, RTTSampledAt: now + 2,
	}
	l.runTransmissionScheduler(now + 2)
	work, err := harness.store.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil {
		t.Fatal(err)
	}
	l.handleMediaStarted(key, witness, &protocol.MediaStartedPayload{
		TransmissionID: created.Transmission.ID, Generation: 1,
		TFirstSampleCoordMS: work.Scheduler.TCoordMS,
	})
	l.handleMediaEnded(key, witness, &protocol.MediaEndedPayload{
		TransmissionID: created.Transmission.ID, Generation: 1,
		TLastSampleCoordMS: work.Scheduler.TCoordMS + mediaItem.DurationMS - 1,
		Reason:             string(store.TransmissionReasonCompleted),
	})
	terminal, err := harness.store.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil || terminal.Targets[0].Status != store.TransmissionTargetFailed ||
		terminal.Targets[0].ReasonCode != store.TransmissionReasonInternalError ||
		terminal.Transmission.CompletedAt == 0 {
		t.Fatalf("early completion accepted=%+v err=%v", terminal, err)
	}
	cancels := fake.ofType(protocol.TypeCancelMedia)
	if len(cancels) != 1 {
		t.Fatalf("early completion safety cancels=%+v", fake.sent)
	}
	cancel := cancels[0].payload.(*protocol.CancelMediaPayload)
	if cancel.Action != "fade_stop" || cancel.FadeMS != 120 || cancel.ResumeMain ||
		cancel.Reason != string(store.TransmissionReasonInternalError) {
		t.Fatalf("early completion safety cancel=%+v", cancel)
	}
}

func TestOverlayControllerLegacyBridgeUsesGenericACLAndExactTargets(t *testing.T) {
	l, fake, harness, owner, mediaItem := schedulerTestLoop(t)
	now := time.Now().UnixMilli()
	params := store.CreateTransmissionParams{
		MediaID: mediaItem.ID, SourceOrbitID: owner.OrbitID,
		SourceActorID: owner.ActorID, SourceSlot: owner.Slot,
		PlaybackDomainKind: store.PlaybackDomainOrbit, PlaybackDomainID: owner.OrbitID,
		AudienceKind: store.TransmissionAudienceThisPulsar,
		OriginKind:   store.TransmissionOriginFile, IncludeOrigin: true,
		RequestedDelivery: store.TransmissionDeliveryOverlay,
		EffectiveDelivery: store.TransmissionDeliveryAfterCurrent,
		DowngradeReason:   store.TransmissionDowngradeMissingOverlay,
		AcceptedAt:        now + 3,
		Targets: []store.CreateTransmissionTarget{{
			OrbitID: owner.OrbitID, ActorID: owner.ActorID, Slot: owner.Slot,
			OnlineAtAcceptance: true,
		}},
	}
	created, err := harness.store.CreateTransmission(params)
	if err != nil {
		t.Fatal(err)
	}
	l.orbit(owner.OrbitID).sess.EnsurePeer(protocol.NodeID(owner.Slot))
	l.handleNode(hub.EvOnline{Key: hub.NodeKey{
		Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot),
	}})
	l.runTransmissionScheduler(now + 4)
	legacy := fake.ofType(protocol.TypePlayVoice)
	if len(legacy) != 1 {
		t.Fatalf("legacy bridge messages=%+v", fake.sent)
	}
	payload := legacy[0].payload.(*protocol.PlayVoicePayload)
	if payload.ElementID != created.Transmission.ID ||
		payload.FileURL != "https://coord.example/v1/media/"+mediaItem.ID ||
		strings.Contains(payload.FileURL, "token") {
		t.Fatalf("legacy payload=%+v", payload)
	}
	work, err := harness.store.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil || work.Scheduler.LegacyElementID != created.Transmission.ID ||
		work.Targets[0].Status != store.TransmissionTargetScheduled {
		t.Fatalf("legacy claim=%+v err=%v", work, err)
	}
	key := hub.NodeKey{Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot)}
	witness := tokenWitness(owner.NodeToken)
	l.handleLegacyTransmissionStarted(key, witness, created.Transmission.ID)
	l.handleLegacyTransmissionEnded(key, witness, created.Transmission.ID)
	work, err = harness.store.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil || work.Transmission.Status != store.TransmissionStatusPlayed {
		t.Fatalf("legacy completion=%+v err=%v", work, err)
	}
	// A repeated wake cannot enqueue or send the durable bridge twice.
	l.runTransmissionScheduler(now + 5)
	if got := len(fake.ofType(protocol.TypePlayVoice)); got != 1 {
		t.Fatalf("legacy bridge replayed %d times", got)
	}
}

func TestOverlayControllerWholeDowngradeNeverSplitsTargetProtocols(t *testing.T) {
	l, fake, harness, owner, mediaItem := schedulerTestLoop(t)
	companion := installTransmissionCompanion(t, harness, owner)
	now := time.Now().UnixMilli()
	created, err := harness.store.CreateTransmission(store.CreateTransmissionParams{
		MediaID: mediaItem.ID, SourceOrbitID: owner.OrbitID,
		SourceActorID: owner.ActorID, SourceSlot: owner.Slot,
		PlaybackDomainKind: store.PlaybackDomainOrbit, PlaybackDomainID: owner.OrbitID,
		AudienceKind: store.TransmissionAudienceOwnBarycenter,
		OriginKind:   store.TransmissionOriginFile, IncludeOrigin: true,
		RequestedDelivery: store.TransmissionDeliveryOverlay,
		EffectiveDelivery: store.TransmissionDeliveryAfterCurrent,
		DowngradeReason:   store.TransmissionDowngradeMissingOverlay,
		AcceptedAt:        now + 3,
		Targets: []store.CreateTransmissionTarget{
			{
				OrbitID: owner.OrbitID, ActorID: owner.ActorID, Slot: owner.Slot,
				OnlineAtAcceptance: true, MediaClipCapable: true, OverlayCapable: true,
			},
			{
				OrbitID: companion.OrbitID, ActorID: companion.ActorID, Slot: companion.Slot,
				OnlineAtAcceptance: true,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := l.orbit(owner.OrbitID)
	for _, credentials := range []store.OnboardingCredentials{owner, companion} {
		node := protocol.NodeID(credentials.Slot)
		state.sess.EnsurePeer(node)
		l.handleNode(hub.EvOnline{Key: hub.NodeKey{Orbit: credentials.OrbitID, Slot: node}})
	}
	l.runTransmissionScheduler(now + 4)

	if got := len(fake.ofType(protocol.TypePrepareMedia)); got != 0 {
		t.Fatalf("whole downgrade sent %d prepare_media messages", got)
	}
	if got := len(fake.ofType(protocol.TypePlayMediaAt)); got != 0 {
		t.Fatalf("whole downgrade sent %d play_media_at messages", got)
	}
	legacy := fake.ofType(protocol.TypePlayVoice)
	if len(legacy) != 2 {
		t.Fatalf("whole downgrade legacy messages=%+v", fake.sent)
	}
	wantNodes := map[protocol.NodeID]bool{
		protocol.NodeID(owner.Slot):     true,
		protocol.NodeID(companion.Slot): true,
	}
	for _, message := range legacy {
		payload := message.payload.(*protocol.PlayVoicePayload)
		if !wantNodes[message.node] || payload.ElementID != created.Transmission.ID ||
			payload.FileURL != "https://coord.example/v1/media/"+mediaItem.ID {
			t.Fatalf("split downgrade legacy message=%+v payload=%+v", message, payload)
		}
		delete(wantNodes, message.node)
	}
	if len(wantNodes) != 0 {
		t.Fatalf("whole downgrade missed legacy nodes=%v", wantNodes)
	}
	work, err := harness.store.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil || work.Transmission.EffectiveDelivery != store.TransmissionDeliveryAfterCurrent ||
		work.Scheduler.LegacyElementID != created.Transmission.ID || len(work.Targets) != 2 {
		t.Fatalf("whole downgrade work=%+v err=%v", work, err)
	}
	for _, target := range work.Targets {
		if target.Status != store.TransmissionTargetScheduled {
			t.Fatalf("whole downgrade split target=%+v", target)
		}
	}
}

func TestOverlayControllerCancellationUsesExactDisarmAndActiveInterruptFade(t *testing.T) {
	l, fake, harness, owner, mediaItem := schedulerTestLoop(t)
	now := time.Now().UnixMilli()
	key := hub.NodeKey{Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot)}
	witness := tokenWitness(owner.NodeToken)
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 1,
		Capabilities:        schedulerCapabilities(t),
		CredentialTokenHash: tokenWitness(owner.NodeToken),
		RTTMS:               10, RTTSampledAt: now + 1,
	}

	overlay := runtimeTransmission(
		t, harness, owner, mediaItem, now, store.TransmissionDeliveryOverlay,
	)
	l.runTransmissionScheduler(now + 1)
	l.handleMediaReady(key, witness, &protocol.MediaReadyPayload{
		TransmissionID: overlay.Transmission.ID, Generation: 1,
		DecodedDurationMS: mediaItem.DurationMS,
	})
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 2,
		Capabilities:        schedulerCapabilities(t),
		CredentialTokenHash: tokenWitness(owner.NodeToken),
		RTTMS:               10, RTTSampledAt: now + 2,
	}
	l.runTransmissionScheduler(now + 2)
	results, err := harness.store.CancelTransmissionsForMedia(
		mediaItem.ID, store.TransmissionReasonMediaDeleted, now+3,
	)
	if err != nil || len(results) != 1 {
		t.Fatalf("scheduled cancellation=%+v err=%v", results, err)
	}
	fake.drain()
	l.deliverTransmissionCancellation(results[0])
	cancels := fake.ofType(protocol.TypeCancelMedia)
	if len(cancels) != 1 {
		t.Fatalf("scheduled cancel messages=%+v", fake.sent)
	}
	disarm := cancels[0].payload.(*protocol.CancelMediaPayload)
	if disarm.Action != "disarm" || disarm.ResumeMain || disarm.FadeMS != 0 ||
		disarm.Generation != 1 || disarm.Reason != string(store.TransmissionReasonMediaDeleted) {
		t.Fatalf("scheduled disarm=%+v", disarm)
	}
	l.handleMediaCancelled(key, witness, &protocol.MediaCancelledPayload{
		TransmissionID: overlay.Transmission.ID, Generation: 1,
		Reason: disarm.Reason, Action: disarm.Action,
	})

	interrupt := runtimeTransmission(
		t, harness, owner, mediaItem, now+10, store.TransmissionDeliveryInterrupt,
	)
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 11,
		Capabilities:        schedulerCapabilities(t),
		CredentialTokenHash: tokenWitness(owner.NodeToken),
		RTTMS:               10, RTTSampledAt: now + 11,
	}
	l.runTransmissionScheduler(now + 11)
	l.handleMediaReady(key, witness, &protocol.MediaReadyPayload{
		TransmissionID: interrupt.Transmission.ID, Generation: 1,
		DecodedDurationMS: mediaItem.DurationMS,
	})
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 12,
		Capabilities:        schedulerCapabilities(t),
		CredentialTokenHash: tokenWitness(owner.NodeToken),
		RTTMS:               10, RTTSampledAt: now + 12,
	}
	l.runTransmissionScheduler(now + 12)
	work, err := harness.store.GetTransmissionSchedulerWork(interrupt.Transmission.ID)
	if err != nil {
		t.Fatal(err)
	}
	l.handleMediaStarted(key, witness, &protocol.MediaStartedPayload{
		TransmissionID: interrupt.Transmission.ID, Generation: 1,
		TFirstSampleCoordMS: work.Scheduler.TCoordMS,
	})
	results, err = harness.store.CancelTransmissionsForMedia(
		mediaItem.ID, store.TransmissionReasonMediaExpired, now+13,
	)
	if err != nil || len(results) != 1 {
		t.Fatalf("active cancellation=%+v err=%v", results, err)
	}
	fake.drain()
	l.deliverTransmissionCancellation(results[0])
	cancels = fake.ofType(protocol.TypeCancelMedia)
	if len(cancels) != 1 {
		t.Fatalf("active cancel messages=%+v", fake.sent)
	}
	fade := cancels[0].payload.(*protocol.CancelMediaPayload)
	if fade.Action != "fade_stop" || !fade.ResumeMain || fade.FadeMS != 120 ||
		fade.Reason != string(store.TransmissionReasonMediaExpired) {
		t.Fatalf("active interrupt fade=%+v", fade)
	}
	l.handleMediaCancelled(key, witness, &protocol.MediaCancelledPayload{
		TransmissionID: interrupt.Transmission.ID, Generation: 1,
		Reason: fade.Reason, Action: fade.Action, MainResumed: true,
	})
	work, err = harness.store.GetTransmissionSchedulerWork(interrupt.Transmission.ID)
	if err != nil || work.Targets[0].Status != store.TransmissionTargetCancelled ||
		work.Transmission.Status != store.TransmissionStatusCancelled {
		t.Fatalf("active cancellation receipt=%+v err=%v", work, err)
	}
}

func TestOverlayControllerRevokedSocketCanAckButReplacementCannot(t *testing.T) {
	l, fake, harness, owner, mediaItem := schedulerTestLoop(t)
	now := time.Now().UnixMilli()
	created := runtimeTransmission(
		t, harness, owner, mediaItem, now, store.TransmissionDeliveryOverlay,
	)
	key := hub.NodeKey{Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot)}
	witness := tokenWitness(owner.NodeToken)
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 1,
		Capabilities:        schedulerCapabilities(t),
		CredentialTokenHash: witness,
		RTTMS:               10, RTTSampledAt: now + 1,
	}
	l.runTransmissionScheduler(now + 1)
	identity, err := harness.store.CurrentInstallationTarget(owner.OrbitID, owner.Slot)
	if err != nil || identity == nil {
		t.Fatalf("current cancellation identity=%+v err=%v", identity, err)
	}
	if found, err := harness.store.RevokeSlot(owner.OrbitID, owner.Slot); err != nil || !found {
		t.Fatalf("revoke target found=%v err=%v", found, err)
	}
	results, err := harness.store.CancelTransmissionNode(
		identity.OrbitID, identity.ActorID, identity.Slot,
		store.TransmissionReasonTargetRevoked, now+2,
	)
	if err != nil || len(results) != 1 || len(results[0].DisarmTargets) != 1 {
		t.Fatalf("revoked target cancellation=%+v err=%v", results, err)
	}
	fake.drain()
	l.deliverTransmissionCancellation(results[0])
	cancels := fake.ofType(protocol.TypeCancelMedia)
	if len(cancels) != 1 {
		t.Fatalf("revoked target cancel messages=%+v", fake.sent)
	}
	disarm := cancels[0].payload.(*protocol.CancelMediaPayload)
	forgedWitness := strings.Repeat("f", 64)
	l.handleMediaCancelled(key, forgedWitness, &protocol.MediaCancelledPayload{
		TransmissionID: created.Transmission.ID, Generation: disarm.Generation,
		Reason: disarm.Reason, Action: disarm.Action,
	})
	stillCancelling, err := harness.store.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil || stillCancelling.Targets[0].Status != store.TransmissionTargetCancelling {
		t.Fatalf("replacement forged cancellation=%+v err=%v", stillCancelling, err)
	}
	l.handleMediaCancelled(key, witness, &protocol.MediaCancelledPayload{
		TransmissionID: created.Transmission.ID, Generation: disarm.Generation,
		Reason: disarm.Reason, Action: disarm.Action,
	})
	terminal, err := harness.store.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil || terminal.Targets[0].Status != store.TransmissionTargetCancelled ||
		terminal.Targets[0].ReasonCode != store.TransmissionReasonTargetRevoked ||
		terminal.Transmission.CompletedAt == 0 {
		t.Fatalf("revoked socket acknowledgement=%+v err=%v", terminal, err)
	}
}

func TestOverlayControllerReconnectResendsFutureGenerationWithoutNewAcceptance(t *testing.T) {
	l, fake, harness, owner, mediaItem := schedulerTestLoop(t)
	now := time.Now().UnixMilli()
	created := runtimeTransmission(
		t, harness, owner, mediaItem, now, store.TransmissionDeliveryOverlay,
	)
	key := hub.NodeKey{Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot)}
	witness := tokenWitness(owner.NodeToken)
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 1,
		Capabilities:        schedulerCapabilities(t),
		CredentialTokenHash: tokenWitness(owner.NodeToken),
		RTTMS:               20, RTTSampledAt: now + 1,
	}
	l.runTransmissionScheduler(now + 1)
	l.handleMediaReady(key, witness, &protocol.MediaReadyPayload{
		TransmissionID: created.Transmission.ID, Generation: 1,
		DecodedDurationMS: mediaItem.DurationMS,
	})
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 2,
		Capabilities:        schedulerCapabilities(t),
		CredentialTokenHash: tokenWitness(owner.NodeToken),
		RTTMS:               20, RTTSampledAt: now + 2,
	}
	l.runTransmissionScheduler(now + 2)
	before, err := harness.store.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil {
		t.Fatal(err)
	}
	fake.drain()
	l.reconcileTransmissionNode(key, now+3)
	plays := fake.ofType(protocol.TypePlayMediaAt)
	if len(plays) != 1 {
		t.Fatalf("reconnect plays=%+v", fake.sent)
	}
	play := plays[0].payload.(*protocol.PlayMediaAtPayload)
	after, err := harness.store.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil || play.Generation != before.Targets[0].Generation ||
		play.TCoordMS != before.Scheduler.TCoordMS ||
		play.StartDeadlineCoordMS != before.Scheduler.StartDeadlineCoordMS ||
		after.Transmission.AcceptedAt != before.Transmission.AcceptedAt ||
		after.Targets[0].Revision != before.Targets[0].Revision {
		t.Fatalf("reconnect mutated work before=%+v after=%+v play=%+v err=%v",
			before, after, play, err)
	}
}

func TestOverlayControllerReconnectNeverResendsAfterCapabilityLoss(t *testing.T) {
	l, fake, harness, owner, mediaItem := schedulerTestLoop(t)
	now := time.Now().UnixMilli()
	created := runtimeTransmission(
		t, harness, owner, mediaItem, now, store.TransmissionDeliveryOverlay,
	)
	key := hub.NodeKey{Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot)}
	witness := tokenWitness(owner.NodeToken)
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 1,
		Capabilities: schedulerCapabilities(t), CredentialTokenHash: witness,
		RTTMS: 20, RTTSampledAt: now + 1,
	}
	l.runTransmissionScheduler(now + 1)
	l.handleMediaReady(key, witness, &protocol.MediaReadyPayload{
		TransmissionID: created.Transmission.ID, Generation: 1,
		DecodedDurationMS: mediaItem.DurationMS,
	})
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 2,
		Capabilities: schedulerCapabilities(t), CredentialTokenHash: witness,
		RTTMS: 20, RTTSampledAt: now + 2,
	}
	l.runTransmissionScheduler(now + 2)
	mediaOnly, err := protocol.ParseCapabilitySet([]string{protocol.CapabilityMediaClip})
	if err != nil {
		t.Fatal(err)
	}
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 3,
		Capabilities: mediaOnly, CredentialTokenHash: witness,
	}
	fake.drain()
	l.reconcileTransmissionNode(key, now+3)
	if got := len(fake.ofType(protocol.TypePlayMediaAt)); got != 0 {
		t.Fatalf("reconnect resent play after capability loss: %+v", fake.sent)
	}
	l.runTransmissionScheduler(now + 3)
	work, err := harness.store.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil || work.Targets[0].Status != store.TransmissionTargetFailed ||
		work.Targets[0].ReasonCode != store.TransmissionReasonCapabilityLost ||
		len(fake.ofType(protocol.TypePlayMediaAt)) != 0 {
		t.Fatalf("capability-loss convergence=%+v messages=%+v err=%v", work, fake.sent, err)
	}
}

func TestOverlayControllerMainPauseAndSkipDoNotTerminateScheduler(t *testing.T) {
	l, fake, harness, owner, mediaItem := schedulerTestLoop(t)
	now := time.Now().UnixMilli()
	created := runtimeTransmission(
		t, harness, owner, mediaItem, now, store.TransmissionDeliveryOverlay,
	)
	key := hub.NodeKey{Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot)}
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 1,
		Capabilities:        schedulerCapabilities(t),
		CredentialTokenHash: tokenWitness(owner.NodeToken),
		RTTMS:               10, RTTSampledAt: now + 1,
	}
	l.runTransmissionScheduler(now + 1)
	state := l.orbit(owner.OrbitID)
	state.sess.EnsurePeer(protocol.NodeID(owner.Slot))
	state.sess.State = session.StatePlaying
	state.sess.Current = &session.Element{
		ID: "main_program", Kind: session.KindTrack, URI: "spotify:track:main",
	}
	l.apply(state, state.sess.CmdPause())
	l.apply(state, state.sess.CmdSkip())
	work, err := harness.store.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil || work.Targets[0].Status != store.TransmissionTargetPreparing ||
		work.Scheduler.BarrierOpenedAt == 0 || work.Transmission.CompletedAt != 0 {
		t.Fatalf("main controls changed scheduler=%+v err=%v", work, err)
	}
}

func TestOverlayControllerNeverInventsInterruptFallback(t *testing.T) {
	l, fake, harness, owner, mediaItem := schedulerTestLoop(t)
	now := time.Now().UnixMilli()
	created := runtimeTransmission(
		t, harness, owner, mediaItem, now, store.TransmissionDeliveryInterrupt,
	)
	capabilities, err := protocol.ParseCapabilitySet([]string{
		protocol.CapabilityMediaClip,
		protocol.CapabilityOverlayMix,
	})
	if err != nil {
		t.Fatal(err)
	}
	key := hub.NodeKey{Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot)}
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 1, Capabilities: capabilities,
		CredentialTokenHash: tokenWitness(owner.NodeToken),
		RTTMS:               10, RTTSampledAt: now + 1,
	}
	l.runTransmissionScheduler(now + 1)
	work, err := harness.store.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil || work.Transmission.EffectiveDelivery != store.TransmissionDeliveryInterrupt ||
		work.Targets[0].Status != store.TransmissionTargetFailed ||
		work.Targets[0].ReasonCode != store.TransmissionReasonInterruptCapabilityLost ||
		work.Scheduler.LegacyElementID != "" || len(fake.ofType(protocol.TypePlayVoice)) != 0 ||
		len(fake.ofType(protocol.TypePlayMediaAt)) != 0 {
		t.Fatalf("interrupt fallback invented work=%+v messages=%+v err=%v",
			work, fake.sent, err)
	}
}

func TestOverlayControllerTimerNeverExtendsAnArmedDeadline(t *testing.T) {
	l := &loop{}
	t.Cleanup(l.clearTransmissionTimer)
	now := time.Now().UnixMilli()

	l.armTransmissionTimer(now+5_000, now)
	if l.transmissionTimerC == nil || l.transmissionTimerDue != now+5_000 {
		t.Fatalf("initial timer due=%d channel=%v", l.transmissionTimerDue, l.transmissionTimerC)
	}
	// A wall-clock rollback plus a later candidate must not reset the existing
	// monotonic timer and turn five seconds into a longer persisted barrier.
	l.armTransmissionTimer(now+8_000, now-60_000)
	if l.transmissionTimerDue != now+5_000 {
		t.Fatalf("armed deadline extended to %d", l.transmissionTimerDue)
	}
	// A genuinely earlier persisted deadline still preempts it.
	l.armTransmissionTimer(now+2_000, now)
	if l.transmissionTimerDue != now+2_000 {
		t.Fatalf("earlier deadline did not preempt: %d", l.transmissionTimerDue)
	}
	l.clearTransmissionTimer()
	if l.transmissionTimerC != nil || l.transmissionTimerDue != 0 {
		t.Fatalf("timer not cleared: due=%d channel=%v", l.transmissionTimerDue, l.transmissionTimerC)
	}
}

func TestOverlayControllerTerminalWorkClearsTimerAndStaleWakeIsInert(t *testing.T) {
	l, fake, harness, owner, mediaItem := schedulerTestLoop(t)
	now := time.Now().UnixMilli()
	created := runtimeTransmission(
		t, harness, owner, mediaItem, now, store.TransmissionDeliveryOverlay,
	)
	key := hub.NodeKey{Orbit: owner.OrbitID, Slot: protocol.NodeID(owner.Slot)}
	witness := tokenWitness(owner.NodeToken)
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 1,
		Capabilities: schedulerCapabilities(t), CredentialTokenHash: witness,
		RTTMS: 10, RTTSampledAt: now + 1,
	}
	l.runTransmissionScheduler(now + 1)
	if l.transmissionTimerC == nil || l.transmissionTimerDue == 0 {
		t.Fatalf("open barrier did not arm a persisted deadline: due=%d", l.transmissionTimerDue)
	}
	l.handleMediaReady(key, witness, &protocol.MediaReadyPayload{
		TransmissionID: created.Transmission.ID, Generation: 1,
		DecodedDurationMS: mediaItem.DurationMS,
	})
	fake.snapshots[key] = hub.NodeSnapshot{
		Connected: true, LastSeenAt: now + 2,
		Capabilities: schedulerCapabilities(t), CredentialTokenHash: witness,
		RTTMS: 10, RTTSampledAt: now + 2,
	}
	l.runTransmissionScheduler(now + 2)
	work, err := harness.store.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil || work.Scheduler.TCoordMS == 0 || l.transmissionTimerC == nil {
		t.Fatalf("scheduled work=%+v timer_due=%d err=%v", work, l.transmissionTimerDue, err)
	}
	l.handleMediaStarted(key, witness, &protocol.MediaStartedPayload{
		TransmissionID: created.Transmission.ID, Generation: 1,
		TFirstSampleCoordMS: work.Scheduler.TCoordMS,
	})
	endedAt := work.Scheduler.TCoordMS + mediaItem.DurationMS
	l.handleMediaEnded(key, witness, &protocol.MediaEndedPayload{
		TransmissionID: created.Transmission.ID, Generation: 1,
		TLastSampleCoordMS: endedAt, Reason: string(store.TransmissionReasonCompleted),
	})
	l.runTransmissionScheduler(endedAt)
	if l.transmissionTimerC != nil || l.transmissionTimerDue != 0 {
		t.Fatalf("terminal work retained orphan timer: due=%d channel=%v",
			l.transmissionTimerDue, l.transmissionTimerC)
	}
	messages := len(fake.sent)
	l.runTransmissionScheduler(endedAt + 60_000)
	if len(fake.sent) != messages || l.transmissionTimerC != nil || l.transmissionTimerDue != 0 {
		t.Fatalf("stale wake recreated work: messages=%d/%d due=%d channel=%v",
			messages, len(fake.sent), l.transmissionTimerDue, l.transmissionTimerC)
	}
}
