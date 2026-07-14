package store

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func schedulerRuntime(
	credentials OnboardingCredentials,
	now int64,
	rttMS int64,
) TransmissionRuntimeTarget {
	return TransmissionRuntimeTarget{
		OrbitID: credentials.OrbitID, Slot: credentials.Slot,
		Connected: true, LastSeenAt: now,
		CredentialTokenHash: hashToken(credentials.NodeToken),
		MediaClipCapable:    true, OverlayCapable: true,
		InterruptCapable: true, InterruptResumeReady: true,
		RTTMS: rttMS, RTTSampledAt: now,
	}
}

func schedulerTransition(
	t *testing.T,
	st *Store,
	target TransmissionTarget,
	status TransmissionTargetStatus,
	reason TransmissionReason,
	now int64,
) TransmissionTargetTransition {
	t.Helper()
	transition, err := st.TransitionTransmissionTarget(
		TransitionTransmissionTargetParams{
			TransmissionID: target.TransmissionID,
			OrbitID:        target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
			ExpectedRevision: target.Revision, Generation: target.Generation,
			Status: status, ReasonCode: reason,
			OccurredAt: maxInt64(now, target.UpdatedAt),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return transition
}

func TestTransmissionSchedulerEnforcesDomainFIFOAndExactRTTBarrier(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	target, err := st.CreateSelfServiceOrbit("Scheduler FIFO target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	first, err := st.CreateTransmission(transmissionParams(
		media, source, now+10, transmissionTarget(target, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateTransmission(transmissionParams(
		media, source, now+11, transmissionTarget(target, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	runtimeAt := now + 20
	runtime := []TransmissionRuntimeTarget{schedulerRuntime(target, runtimeAt, 140)}
	if _, err := st.OpenTransmissionBarrier(
		second.Transmission.ID, runtimeAt, runtime,
	); !errors.Is(err, ErrTransmissionNotFIFOHead) {
		t.Fatalf("second FIFO row opened before first: %v", err)
	}
	opened, err := st.OpenTransmissionBarrier(first.Transmission.ID, runtimeAt, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if !opened.Opened || len(opened.PrepareTargets) != 1 ||
		opened.Work.Scheduler.BarrierOpenedAt != runtimeAt ||
		opened.Work.Scheduler.PrepareDeadlineAt != runtimeAt+TransmissionPrepareBarrierMS ||
		opened.PrepareTargets[0].Status != TransmissionTargetPreparing {
		t.Fatalf("opened barrier=%+v", opened)
	}
	ready := schedulerTransition(
		t, st, opened.PrepareTargets[0], TransmissionTargetReady, "", runtimeAt+5,
	).Target
	decisionAt := runtimeAt + 6
	runtime[0].LastSeenAt = decisionAt
	runtime[0].RTTSampledAt = decisionAt
	decision, err := st.DecideTransmissionBarrier(first.Transmission.ID, decisionAt, runtime)
	if err != nil {
		t.Fatal(err)
	}
	wantLead := int64(530) // 2*140 + 250, larger than the 500 ms floor.
	if !decision.Decided || !decision.Changed || len(decision.ScheduledTargets) != 1 ||
		decision.ScheduledTargets[0].Generation != ready.Generation ||
		decision.Work.Scheduler.TCoordMS != decisionAt+wantLead ||
		decision.Work.Scheduler.StartDeadlineCoordMS != decisionAt+wantLead+100 {
		t.Fatalf("decision=%+v", decision)
	}
	playing := schedulerTransition(
		t, st, decision.ScheduledTargets[0], TransmissionTargetPlaying, "",
		decision.Work.Scheduler.TCoordMS,
	).Target
	played := schedulerTransition(
		t, st, playing, TransmissionTargetPlayed, TransmissionReasonCompleted,
		decision.Work.Scheduler.TCoordMS+media.DurationMS,
	)
	if played.Transmission.Status != TransmissionStatusPlayed {
		t.Fatalf("first aggregate=%+v", played.Transmission)
	}
	runtime[0].LastSeenAt = decision.Work.Scheduler.TCoordMS + media.DurationMS + 1
	runtime[0].RTTSampledAt = runtime[0].LastSeenAt
	if openedSecond, err := st.OpenTransmissionBarrier(
		second.Transmission.ID, runtime[0].LastSeenAt, runtime,
	); err != nil || !openedSecond.Opened {
		t.Fatalf("second did not become FIFO head: opened=%+v err=%v", openedSecond, err)
	}
}

func TestTransmissionSchedulerBreaksEqualAcceptanceTiesByULIDAcrossDeliveries(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	target, err := st.CreateSelfServiceOrbit("Scheduler ULID tie target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	acceptedAt := now + 10
	overlayParams := transmissionParams(
		media, source, acceptedAt, transmissionTarget(target, true),
	)
	overlay, err := st.CreateTransmission(overlayParams)
	if err != nil {
		t.Fatal(err)
	}
	interruptParams := transmissionParams(
		media, source, acceptedAt, transmissionTarget(target, true),
	)
	interruptParams.RequestedDelivery = TransmissionDeliveryInterrupt
	interruptParams.EffectiveDelivery = TransmissionDeliveryInterrupt
	interrupt, err := st.CreateTransmission(interruptParams)
	if err != nil {
		t.Fatal(err)
	}
	head, tail := overlay, interrupt
	if tail.Transmission.ID < head.Transmission.ID {
		head, tail = tail, head
	}
	runtimeAt := now + 20
	runtime := []TransmissionRuntimeTarget{schedulerRuntime(target, runtimeAt, 10)}
	if _, err := st.OpenTransmissionBarrier(
		tail.Transmission.ID, runtimeAt, runtime,
	); !errors.Is(err, ErrTransmissionNotFIFOHead) {
		t.Fatalf("larger equal-time ULID bypassed mixed-delivery FIFO: %v", err)
	}
	opened, err := st.OpenTransmissionBarrier(head.Transmission.ID, runtimeAt, runtime)
	if err != nil || !opened.Opened {
		t.Fatalf("smaller equal-time ULID did not win: opened=%+v err=%v", opened, err)
	}
}

func TestTransmissionSchedulerPartialReadinessAndNoLateAutoplay(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	left, err := st.CreateSelfServiceOrbit("Scheduler ready target")
	if err != nil {
		t.Fatal(err)
	}
	right, err := st.CreateSelfServiceOrbit("Scheduler timeout target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+10,
		transmissionTarget(left, true), transmissionTarget(right, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	barrierAt := now + 20
	runtime := []TransmissionRuntimeTarget{
		schedulerRuntime(left, barrierAt, 20),
		schedulerRuntime(right, barrierAt, 60),
	}
	opened, err := st.OpenTransmissionBarrier(created.Transmission.ID, barrierAt, runtime)
	if err != nil || len(opened.PrepareTargets) != 2 {
		t.Fatalf("open=%+v err=%v", opened, err)
	}
	var readyTarget TransmissionTarget
	for _, candidate := range opened.PrepareTargets {
		if candidate.ActorID == left.ActorID {
			readyTarget = candidate
		}
	}
	readyTarget = schedulerTransition(
		t, st, readyTarget, TransmissionTargetReady, "", barrierAt+10,
	).Target
	if early, err := st.DecideTransmissionBarrier(
		created.Transmission.ID, barrierAt+11, runtime,
	); err != nil || early.Decided {
		t.Fatalf("barrier decided before pending deadline: %+v err=%v", early, err)
	}
	deadline := opened.Work.Scheduler.PrepareDeadlineAt
	for index := range runtime {
		runtime[index].LastSeenAt = deadline
		runtime[index].RTTSampledAt = deadline
	}
	decision, err := st.DecideTransmissionBarrier(created.Transmission.ID, deadline, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.ScheduledTargets) != 1 ||
		decision.ScheduledTargets[0].ActorID != readyTarget.ActorID {
		t.Fatalf("partial schedule=%+v", decision)
	}
	var missed TransmissionTarget
	for _, candidate := range decision.Work.Targets {
		if candidate.ActorID == right.ActorID {
			missed = candidate
		}
	}
	if missed.Status != TransmissionTargetMissedNotReady ||
		missed.ReasonCode != TransmissionReasonPrepareDeadline ||
		missed.EndedAt != deadline {
		t.Fatalf("deadline miss=%+v", missed)
	}
	expired, err := st.ExpireTransmissionRuntime(
		created.Transmission.ID,
		decision.Work.Scheduler.StartDeadlineCoordMS+1,
	)
	if err != nil || !expired.Changed || len(expired.DisarmTargets) != 1 {
		t.Fatalf("stale scheduled target not closed: result=%+v err=%v", expired, err)
	}
	after, err := st.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range after.Targets {
		if candidate.ActorID == left.ActorID &&
			(candidate.Status != TransmissionTargetFailed ||
				candidate.ReasonCode != TransmissionReasonStalePlay) {
			t.Fatalf("late target=%+v", candidate)
		}
	}
}

func TestTransmissionSchedulerRechecksBlockDNDOfflineAndClockEvidence(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	blocked, err := st.CreateSelfServiceOrbit("Scheduler blocked target")
	if err != nil {
		t.Fatal(err)
	}
	dnd, err := st.CreateSelfServiceOrbit("Scheduler DND target")
	if err != nil {
		t.Fatal(err)
	}
	offline, err := st.CreateSelfServiceOrbit("Scheduler offline target")
	if err != nil {
		t.Fatal(err)
	}
	clockless, err := st.CreateSelfServiceOrbit("Scheduler clock target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	if _, err := st.CreateTransmissionBlock(CreateTransmissionBlockParams{
		OwnerScope: BlockOwnerOrbit, OwnerOrbitID: blocked.OrbitID,
		BlockedKind: BlockedSubjectActor, BlockedActorID: source.ActorID,
		AuthorizedByActorID: blocked.ActorID, CreatedAt: now + 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetNodeDND(SetNodeDNDParams{
		OrbitID: dnd.OrbitID, ActorID: dnd.ActorID, Slot: dnd.Slot,
		Mode: DNDMutedUntil, MutedUntil: now + 60_000,
		Revision: 1, UpdatedAt: now + 2,
	}); err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+10,
		transmissionTarget(blocked, true), transmissionTarget(dnd, true),
		transmissionTarget(offline, true), transmissionTarget(clockless, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	barrierAt := now + 20
	runtime := []TransmissionRuntimeTarget{
		schedulerRuntime(blocked, barrierAt, 20),
		schedulerRuntime(dnd, barrierAt, 20),
		schedulerRuntime(offline, barrierAt-TransmissionRTTFreshMS-1, 20),
		schedulerRuntime(clockless, barrierAt, 20),
	}
	runtime[2].Connected = false
	runtime[3].RTTSampledAt = 0
	opened, err := st.OpenTransmissionBarrier(created.Transmission.ID, barrierAt, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if len(opened.PrepareTargets) != 1 || opened.PrepareTargets[0].ActorID != clockless.ActorID {
		t.Fatalf("policy precedence targets=%+v", opened.Work.Targets)
	}
	ready := schedulerTransition(
		t, st, opened.PrepareTargets[0], TransmissionTargetReady, "", barrierAt+1,
	).Target
	decision, err := st.DecideTransmissionBarrier(
		created.Transmission.ID, barrierAt+2, runtime,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.ScheduledTargets) != 0 {
		t.Fatalf("clockless target scheduled=%+v", decision.ScheduledTargets)
	}
	statuses := make(map[int64]TransmissionTarget)
	for _, target := range decision.Work.Targets {
		statuses[target.ActorID] = target
	}
	if statuses[blocked.ActorID].Status != TransmissionTargetBlocked ||
		statuses[dnd.ActorID].Status != TransmissionTargetMissedDND ||
		statuses[offline.ActorID].Status != TransmissionTargetMissedOffline ||
		statuses[ready.ActorID].ReasonCode != TransmissionReasonClockUnsynchronized ||
		decision.Work.Transmission.Status != TransmissionStatusFailed {
		t.Fatalf("policy/clock outcomes=%+v aggregate=%+v", statuses, decision.Work.Transmission)
	}
}

func TestTransmissionSchedulerLegacyClaimIsDurableAndIdempotent(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	target, err := st.CreateSelfServiceOrbit("Scheduler legacy target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	params := transmissionParams(media, source, now+10, transmissionTarget(target, true))
	params.EffectiveDelivery = TransmissionDeliveryAfterCurrent
	params.DowngradeReason = TransmissionDowngradeMissingOverlay
	created, err := st.CreateTransmission(params)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := st.ClaimLegacyTransmission(
		created.Transmission.ID, created.Transmission.ID, now+20,
	)
	if err != nil || !claimed.Changed || len(claimed.Targets) != 1 ||
		claimed.Targets[0].Status != TransmissionTargetScheduled {
		t.Fatalf("claim=%+v err=%v", claimed, err)
	}
	replay, err := st.ClaimLegacyTransmission(
		created.Transmission.ID, created.Transmission.ID, now+21,
	)
	if err != nil || replay.Changed || replay.Work.Scheduler.LegacyElementID != created.Transmission.ID {
		t.Fatalf("claim replay=%+v err=%v", replay, err)
	}
}

func TestTransmissionSchedulerSerializesOppositeApproachOrigins(t *testing.T) {
	st, left := newMediaIngestTestStore(t)
	right, err := st.CreateSelfServiceOrbit("Scheduler opposite origin")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	leftMedia := readyLifecycleMedia(
		t, st, left, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	rightMedia := readyLifecycleMedia(
		t, st, right, now+3, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	const approachID = int64(991)
	leftParams := transmissionParams(
		leftMedia, left, now+10, transmissionTarget(left, true),
	)
	leftParams.PlaybackDomainKind = PlaybackDomainApproach
	leftParams.PlaybackDomainID = approachID
	rightParams := transmissionParams(
		rightMedia, right, now+11, transmissionTarget(right, true),
	)
	rightParams.PlaybackDomainKind = PlaybackDomainApproach
	rightParams.PlaybackDomainID = approachID
	first, err := st.CreateTransmission(leftParams)
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateTransmission(rightParams)
	if err != nil {
		t.Fatal(err)
	}
	barrierAt := now + 20
	runtime := []TransmissionRuntimeTarget{
		schedulerRuntime(left, barrierAt, 10),
		schedulerRuntime(right, barrierAt, 10),
	}
	if _, err := st.OpenTransmissionBarrier(
		second.Transmission.ID, barrierAt, runtime,
	); !errors.Is(err, ErrTransmissionNotFIFOHead) {
		t.Fatalf("opposite origin bypassed shared approach FIFO: %v", err)
	}
	opened, err := st.OpenTransmissionBarrier(first.Transmission.ID, barrierAt, runtime)
	if err != nil {
		t.Fatal(err)
	}
	ready := schedulerTransition(
		t, st, opened.PrepareTargets[0], TransmissionTargetReady, "", barrierAt+1,
	).Target
	for i := range runtime {
		runtime[i].LastSeenAt = barrierAt + 2
		runtime[i].RTTSampledAt = barrierAt + 2
	}
	decision, err := st.DecideTransmissionBarrier(
		first.Transmission.ID, barrierAt+2, runtime,
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ready
	// The decision advanced the target revision; finish through the returned
	// snapshot and prove the opposite origin becomes head only then.
	started := schedulerTransition(
		t, st, decision.ScheduledTargets[0], TransmissionTargetPlaying, "",
		decision.Work.Scheduler.TCoordMS,
	).Target
	schedulerTransition(
		t, st, started, TransmissionTargetPlayed, TransmissionReasonCompleted,
		decision.Work.Scheduler.TCoordMS+leftMedia.DurationMS,
	)
	openAt := decision.Work.Scheduler.TCoordMS + leftMedia.DurationMS + 1
	for i := range runtime {
		runtime[i].LastSeenAt = openAt
		runtime[i].RTTSampledAt = openAt
	}
	if opened, err := st.OpenTransmissionBarrier(
		second.Transmission.ID, openAt, runtime,
	); err != nil || !opened.Opened {
		t.Fatalf("opposite origin did not inherit FIFO head: opened=%+v err=%v", opened, err)
	}
}

func TestTransmissionSchedulerApproachSplitDisarmsOnlyNonStartedTargets(t *testing.T) {
	st, left := newMediaIngestTestStore(t)
	right, err := st.CreateSelfServiceOrbit("Scheduler split target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, left, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	const approachID = int64(992)
	params := transmissionParams(
		media, left, now+10,
		transmissionTarget(left, true), transmissionTarget(right, true),
	)
	params.PlaybackDomainKind = PlaybackDomainApproach
	params.PlaybackDomainID = approachID
	created, err := st.CreateTransmission(params)
	if err != nil {
		t.Fatal(err)
	}
	barrierAt := now + 20
	runtime := []TransmissionRuntimeTarget{
		schedulerRuntime(left, barrierAt, 10),
		schedulerRuntime(right, barrierAt, 10),
	}
	opened, err := st.OpenTransmissionBarrier(created.Transmission.ID, barrierAt, runtime)
	if err != nil || len(opened.PrepareTargets) != 2 {
		t.Fatalf("open split fixture=%+v err=%v", opened, err)
	}
	for _, target := range opened.PrepareTargets {
		schedulerTransition(
			t, st, target, TransmissionTargetReady, "", barrierAt+1,
		)
	}
	for i := range runtime {
		runtime[i].LastSeenAt = barrierAt + 2
		runtime[i].RTTSampledAt = barrierAt + 2
	}
	decision, err := st.DecideTransmissionBarrier(
		created.Transmission.ID, barrierAt+2, runtime,
	)
	if err != nil || len(decision.ScheduledTargets) != 2 {
		t.Fatalf("schedule split fixture=%+v err=%v", decision, err)
	}
	var leftScheduled TransmissionTarget
	for _, target := range decision.ScheduledTargets {
		if target.ActorID == left.ActorID {
			leftScheduled = target
		}
	}
	playing := schedulerTransition(
		t, st, leftScheduled, TransmissionTargetPlaying, "",
		decision.Work.Scheduler.TCoordMS,
	).Target

	results, err := st.CancelTransmissionPlaybackDomain(
		PlaybackDomainApproach, approachID, TransmissionReasonApproachApart,
		decision.Work.Scheduler.TCoordMS+1,
	)
	if err != nil || len(results) != 1 || len(results[0].DisarmTargets) != 1 {
		t.Fatalf("split cancellation=%+v err=%v", results, err)
	}
	nonStarted := results[0].DisarmTargets[0]
	if nonStarted.ActorID != right.ActorID ||
		nonStarted.Status != TransmissionTargetCancelling ||
		nonStarted.ReasonCode != TransmissionReasonApproachApart ||
		results[0].Transmission.Status != TransmissionStatusPlaying ||
		results[0].Transmission.CancellationCause != TransmissionReasonApproachApart {
		t.Fatalf("split states=%+v", results[0])
	}
	schedulerTransition(
		t, st, nonStarted, TransmissionTargetCancelled,
		TransmissionReasonApproachApart, decision.Work.Scheduler.TCoordMS+2,
	)
	watchdog, err := st.ExpireTransmissionRuntime(
		created.Transmission.ID, decision.Work.Scheduler.TCoordMS+3,
	)
	if err != nil || watchdog.Changed || watchdog.NextDue == 0 {
		t.Fatalf("playing split target lost watchdog=%+v err=%v", watchdog, err)
	}
	terminal := schedulerTransition(
		t, st, playing, TransmissionTargetPlayed, TransmissionReasonCompleted,
		decision.Work.Scheduler.TCoordMS+media.DurationMS,
	)
	if terminal.Transmission.Status != TransmissionStatusPartial ||
		terminal.Transmission.CompletedAt == 0 {
		t.Fatalf("split terminal=%+v", terminal.Transmission)
	}
	closed, err := st.ExpireTransmissionRuntime(
		created.Transmission.ID, terminal.Transmission.CompletedAt+1,
	)
	if err != nil || closed.Changed || closed.NextDue != 0 {
		t.Fatalf("split left orphan timer=%+v err=%v", closed, err)
	}
}

func TestTransmissionSchedulerRechecksDNDWithoutSuppressingUserMessagesOnly(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+10, transmissionTarget(source, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetNodeDND(SetNodeDNDParams{
		OrbitID: source.OrbitID, ActorID: source.ActorID, Slot: source.Slot,
		Mode: DNDMessagesOnly, Revision: 1, UpdatedAt: now + 11,
	}); err != nil {
		t.Fatal(err)
	}
	barrierAt := now + 20
	runtime := []TransmissionRuntimeTarget{schedulerRuntime(source, barrierAt, 25)}
	opened, err := st.OpenTransmissionBarrier(created.Transmission.ID, barrierAt, runtime)
	if err != nil || len(opened.PrepareTargets) != 1 {
		t.Fatalf("messages_only suppressed a user clip: opened=%+v err=%v", opened, err)
	}
	ready := schedulerTransition(
		t, st, opened.PrepareTargets[0], TransmissionTargetReady, "", barrierAt+1,
	).Target
	runtime[0].LastSeenAt = barrierAt + 2
	runtime[0].RTTSampledAt = barrierAt + 2
	decision, err := st.DecideTransmissionBarrier(created.Transmission.ID, barrierAt+2, runtime)
	if err != nil || len(decision.ScheduledTargets) != 1 {
		t.Fatalf("user clip schedule=%+v err=%v ready=%+v", decision, err, ready)
	}
	if _, err := st.SetNodeDND(SetNodeDNDParams{
		OrbitID: source.OrbitID, ActorID: source.ActorID, Slot: source.Slot,
		Mode: DNDMutedUntil, MutedUntil: now + 60_000,
		Revision: 2, UpdatedAt: barrierAt + 3,
	}); err != nil {
		t.Fatal(err)
	}
	runtime[0].LastSeenAt = barrierAt + 4
	rechecked, err := st.RecheckTransmissionRuntime(
		created.Transmission.ID, barrierAt+4, runtime,
	)
	if err != nil || !rechecked.Changed || len(rechecked.DisarmTargets) != 1 ||
		rechecked.DisarmTargets[0].Status != TransmissionTargetMissedDND ||
		rechecked.DisarmTargets[0].ReasonCode != TransmissionReasonLocalDND ||
		rechecked.Work.Transmission.Status != TransmissionStatusFailed {
		t.Fatalf("DND schedule recheck=%+v err=%v", rechecked, err)
	}
}

func TestTransmissionSchedulerActiveDNDUsesFadeStopCancellation(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+10, transmissionTarget(source, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	barrierAt := now + 20
	runtime := []TransmissionRuntimeTarget{schedulerRuntime(source, barrierAt, 10)}
	opened, err := st.OpenTransmissionBarrier(created.Transmission.ID, barrierAt, runtime)
	if err != nil {
		t.Fatal(err)
	}
	schedulerTransition(
		t, st, opened.PrepareTargets[0], TransmissionTargetReady, "", barrierAt+1,
	)
	runtime[0].LastSeenAt = barrierAt + 2
	runtime[0].RTTSampledAt = barrierAt + 2
	decision, err := st.DecideTransmissionBarrier(created.Transmission.ID, barrierAt+2, runtime)
	if err != nil {
		t.Fatal(err)
	}
	playing := schedulerTransition(
		t, st, decision.ScheduledTargets[0], TransmissionTargetPlaying, "",
		decision.Work.Scheduler.TCoordMS,
	).Target
	dndAt := decision.Work.Scheduler.TCoordMS + 1
	if _, err := st.SetNodeDND(SetNodeDNDParams{
		OrbitID: source.OrbitID, ActorID: source.ActorID, Slot: source.Slot,
		Mode: DNDMutedUntil, MutedUntil: dndAt + 60_000,
		Revision: 1, UpdatedAt: dndAt,
	}); err != nil {
		t.Fatal(err)
	}
	runtime[0].LastSeenAt = dndAt + 1
	rechecked, err := st.RecheckTransmissionRuntime(
		created.Transmission.ID, dndAt+1, runtime,
	)
	if err != nil || !rechecked.Changed || len(rechecked.DisarmTargets) != 1 {
		t.Fatalf("active DND recheck=%+v err=%v", rechecked, err)
	}
	cancelling := rechecked.DisarmTargets[0]
	if cancelling.Status != TransmissionTargetCancelling ||
		cancelling.ReasonCode != TransmissionReasonDNDEnabled ||
		cancelling.StartedAt != playing.StartedAt {
		t.Fatalf("active DND target=%+v", cancelling)
	}
	terminal := schedulerTransition(
		t, st, cancelling, TransmissionTargetCancelled,
		TransmissionReasonDNDEnabled, dndAt+2,
	)
	if terminal.Transmission.Status != TransmissionStatusFailed {
		t.Fatalf("target-local DND aggregate=%+v", terminal.Transmission)
	}
}

func TestTransmissionSchedulerDeleteCancellationAndAckTimeoutConverge(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+10, transmissionTarget(source, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	barrierAt := now + 20
	runtime := []TransmissionRuntimeTarget{schedulerRuntime(source, barrierAt, 10)}
	opened, err := st.OpenTransmissionBarrier(created.Transmission.ID, barrierAt, runtime)
	if err != nil || len(opened.PrepareTargets) != 1 {
		t.Fatalf("open=%+v err=%v", opened, err)
	}
	cancelAt := barrierAt + 1
	results, err := st.CancelTransmissionsForMedia(
		media.ID, TransmissionReasonMediaDeleted, cancelAt,
	)
	if err != nil || len(results) != 1 || len(results[0].DisarmTargets) != 1 ||
		results[0].DisarmTargets[0].Status != TransmissionTargetCancelling {
		t.Fatalf("delete cancellation=%+v err=%v", results, err)
	}
	replayed, err := st.CancelTransmissionsForMedia(
		media.ID, TransmissionReasonMediaDeleted, cancelAt+1,
	)
	if err != nil || len(replayed) != 1 || replayed[0].Changed ||
		len(replayed[0].DisarmTargets) != 1 {
		t.Fatalf("delete cancellation replay=%+v err=%v", replayed, err)
	}
	expired, err := st.ExpireTransmissionRuntime(
		created.Transmission.ID, cancelAt+TransmissionCancelAckMS+1,
	)
	if err != nil || !expired.Changed ||
		expired.Work.Targets[0].Status != TransmissionTargetFailed ||
		expired.Work.Targets[0].ReasonCode != TransmissionReasonCancelUnacknowledged ||
		expired.Work.Transmission.Status != TransmissionStatusCancelled ||
		expired.Work.Transmission.ReasonCode != TransmissionReasonMediaDeleted {
		t.Fatalf("cancel acknowledgement timeout=%+v err=%v", expired, err)
	}
}

func TestTransmissionSchedulerDeliveryExpiryDisarmsPreparedWork(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+10, transmissionTarget(source, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	barrierAt := now + 20
	runtime := []TransmissionRuntimeTarget{schedulerRuntime(source, barrierAt, 10)}
	if _, err := st.OpenTransmissionBarrier(created.Transmission.ID, barrierAt, runtime); err != nil {
		t.Fatal(err)
	}
	expired, err := st.ExpireTransmissionDelivery(
		created.Transmission.ID, created.Transmission.ExpiresAt,
	)
	if err != nil || !expired.Changed || len(expired.DisarmTargets) != 1 ||
		expired.DisarmTargets[0].Status != TransmissionTargetExpired ||
		expired.Transmission.Status != TransmissionStatusExpired ||
		expired.Transmission.ReasonCode != TransmissionReasonDeliveryExpired {
		t.Fatalf("delivery expiry=%+v err=%v", expired, err)
	}
	replay, err := st.ExpireTransmissionDelivery(
		created.Transmission.ID, created.Transmission.ExpiresAt+1,
	)
	if err != nil || replay.Changed || len(replay.DisarmTargets) != 0 {
		t.Fatalf("delivery expiry replay=%+v err=%v", replay, err)
	}
}

func TestTransmissionSchedulerNeverSchedulesAtOrPastDeliveryExpiry(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+10, transmissionTarget(source, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	// decisionAt is barrierAt+2 and the minimum lead is 500 ms, so a
	// non-exclusive implementation would place T exactly on expires_at.
	barrierAt := created.Transmission.ExpiresAt - 502
	runtime := []TransmissionRuntimeTarget{schedulerRuntime(source, barrierAt, 0)}
	opened, err := st.OpenTransmissionBarrier(created.Transmission.ID, barrierAt, runtime)
	if err != nil {
		t.Fatal(err)
	}
	schedulerTransition(
		t, st, opened.PrepareTargets[0], TransmissionTargetReady, "", barrierAt+1,
	)
	decisionAt := barrierAt + 2
	runtime[0].LastSeenAt = decisionAt
	runtime[0].RTTSampledAt = decisionAt
	decision, err := st.DecideTransmissionBarrier(
		created.Transmission.ID, decisionAt, runtime,
	)
	if err != nil || len(decision.ScheduledTargets) != 0 ||
		len(decision.DisarmTargets) != 1 || decision.Work.Scheduler.TCoordMS != 0 ||
		decision.DisarmTargets[0].Status != TransmissionTargetExpired ||
		decision.Work.Transmission.Status != TransmissionStatusExpired ||
		decision.Work.Transmission.CancellationCause != TransmissionReasonDeliveryExpired {
		t.Fatalf("past-expiry schedule=%+v err=%v", decision, err)
	}
}

func TestTransmissionSchedulerRestartCancelsPreparedKeepsDeadlineAndStalesPast(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	pastTarget, err := st.CreateSelfServiceOrbit("Scheduler restart past")
	if err != nil {
		t.Fatal(err)
	}
	futureTarget, err := st.CreateSelfServiceOrbit("Scheduler restart future")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	makeWork := func(
		target OnboardingCredentials, acceptedAt, barrierAt, decisionAt int64, decide bool,
	) TransmissionSchedulerWork {
		t.Helper()
		params := transmissionParams(media, source, acceptedAt, transmissionTarget(target, true))
		params.PlaybackDomainID = target.OrbitID
		created, err := st.CreateTransmission(params)
		if err != nil {
			t.Fatal(err)
		}
		runtime := []TransmissionRuntimeTarget{schedulerRuntime(target, barrierAt, 0)}
		opened, err := st.OpenTransmissionBarrier(created.Transmission.ID, barrierAt, runtime)
		if err != nil {
			t.Fatal(err)
		}
		if !decide {
			return opened.Work
		}
		schedulerTransition(
			t, st, opened.PrepareTargets[0], TransmissionTargetReady, "", barrierAt+1,
		)
		runtime[0].LastSeenAt = decisionAt
		runtime[0].RTTSampledAt = decisionAt
		decision, err := st.DecideTransmissionBarrier(created.Transmission.ID, decisionAt, runtime)
		if err != nil {
			t.Fatal(err)
		}
		return decision.Work
	}
	prepared := makeWork(source, now+10, now+20, 0, false)
	past := makeWork(pastTarget, now+11, now+30, now+40, true)
	future := makeWork(futureTarget, now+12, now+800, now+900, true)
	restartAt := future.Scheduler.StartDeadlineCoordMS
	if past.Scheduler.StartDeadlineCoordMS >= restartAt ||
		future.Scheduler.StartDeadlineCoordMS != restartAt {
		t.Fatalf("invalid restart fixture past=%+v future=%+v", past.Scheduler, future.Scheduler)
	}
	results, err := st.ReconcileTransmissionSchedulerRestart(restartAt)
	if err != nil || len(results) != 2 {
		t.Fatalf("restart results=%+v err=%v", results, err)
	}
	preparedAfter, err := st.GetTransmissionSchedulerWork(prepared.Transmission.ID)
	if err != nil || preparedAfter.Transmission.CancellationCause !=
		TransmissionReasonCoordinatorRestarted ||
		preparedAfter.Targets[0].Status != TransmissionTargetCancelling {
		t.Fatalf("prepared restart=%+v err=%v", preparedAfter, err)
	}
	pastAfter, err := st.GetTransmissionSchedulerWork(past.Transmission.ID)
	if err != nil || pastAfter.Targets[0].Status != TransmissionTargetFailed ||
		pastAfter.Targets[0].ReasonCode != TransmissionReasonStalePlay {
		t.Fatalf("past restart=%+v err=%v", pastAfter, err)
	}
	futureAfter, err := st.GetTransmissionSchedulerWork(future.Transmission.ID)
	if err != nil || futureAfter.Targets[0].Status != TransmissionTargetScheduled ||
		futureAfter.Targets[0].Generation != future.Targets[0].Generation ||
		futureAfter.Scheduler.TCoordMS != future.Scheduler.TCoordMS {
		t.Fatalf("future restart=%+v err=%v", futureAfter, err)
	}
}

func TestTransmissionSchedulerSchemaBackfillsPredecessorAcceptanceOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scheduler-roll-forward.db")
	st, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	source, err := st.CreateSelfServiceOrbit("Scheduler predecessor row")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+10, transmissionTarget(source, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	// Model a row written by the immediate predecessor, which knew the
	// transmission snapshot but not the additive scheduler companion.
	if _, err := st.db.Exec(
		`DELETE FROM transmission_scheduler_state WHERE transmission_id = ?`,
		created.Transmission.ID,
	); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	work, err := reopened.GetTransmissionSchedulerWork(created.Transmission.ID)
	if err != nil || work.Scheduler.UpdatedAt != created.Transmission.AcceptedAt ||
		work.Scheduler.BarrierOpenedAt != 0 || work.Scheduler.PrepareDeadlineAt != 0 ||
		work.Scheduler.DecisionAt != 0 || work.Scheduler.TCoordMS != 0 ||
		work.Scheduler.LegacyElementID != "" {
		t.Fatalf("scheduler backfill invented runtime state=%+v err=%v", work.Scheduler, err)
	}
}

func TestTransmissionSchedulerReceiptLookupUsesExactAuthenticatedBinding(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	target, err := st.CreateSelfServiceOrbit("Scheduler receipt binding")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+10, transmissionTarget(target, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	witness := hashToken(target.NodeToken)
	current, err := st.TransmissionTargetForReceipt(
		created.Transmission.ID, target.OrbitID, target.Slot, witness,
	)
	if err != nil || current == nil {
		t.Fatalf("current receipt binding=%+v err=%v", current, err)
	}
	if found, err := st.RevokeSlot(target.OrbitID, target.Slot); err != nil || !found {
		t.Fatalf("revoke target found=%v err=%v", found, err)
	}
	revoked, err := st.TransmissionTargetForReceipt(
		created.Transmission.ID, target.OrbitID, target.Slot, witness,
	)
	if err != nil || revoked == nil {
		t.Fatalf("exact revoked socket lost cancellation receipt target=%+v err=%v", revoked, err)
	}
	forged, err := st.TransmissionTargetForReceipt(
		created.Transmission.ID, target.OrbitID, target.Slot,
		strings.Repeat("f", 64),
	)
	if err != nil || forged != nil {
		t.Fatalf("replacement socket forged predecessor receipt target=%+v err=%v", forged, err)
	}
}

func TestTransmissionSchedulerDNDLookupRequiresCurrentSocketBinding(t *testing.T) {
	st, _ := newMediaIngestTestStore(t)
	target, err := st.CreateSelfServiceOrbit("Scheduler DND socket binding")
	if err != nil {
		t.Fatal(err)
	}
	witness := hashToken(target.NodeToken)
	current, err := st.CurrentInstallationTargetForSocket(
		target.OrbitID, target.Slot, witness,
	)
	if err != nil || current == nil || current.ActorID != target.ActorID {
		t.Fatalf("current DND socket=%+v err=%v", current, err)
	}
	forged, err := st.CurrentInstallationTargetForSocket(
		target.OrbitID, target.Slot, strings.Repeat("f", 64),
	)
	if err != nil || forged != nil {
		t.Fatalf("stale socket resolved replacement DND=%+v err=%v", forged, err)
	}
	if found, err := st.RevokeSlot(target.OrbitID, target.Slot); err != nil || !found {
		t.Fatalf("revoke target found=%v err=%v", found, err)
	}
	revoked, err := st.CurrentInstallationTargetForSocket(
		target.OrbitID, target.Slot, witness,
	)
	if err != nil || revoked != nil {
		t.Fatalf("revoked socket retained DND authority=%+v err=%v", revoked, err)
	}
}
