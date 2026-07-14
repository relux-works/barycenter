package store

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func transmissionParams(
	media MediaItem,
	source OnboardingCredentials,
	acceptedAt int64,
	targets ...CreateTransmissionTarget,
) CreateTransmissionParams {
	return CreateTransmissionParams{
		MediaID:            media.ID,
		SourceOrbitID:      source.OrbitID,
		SourceActorID:      source.ActorID,
		SourceSlot:         source.Slot,
		PlaybackDomainKind: PlaybackDomainOrbit,
		PlaybackDomainID:   source.OrbitID,
		AudienceKind:       TransmissionAudienceExplicit,
		OriginKind:         TransmissionOriginFile,
		IncludeOrigin:      true,
		RequestedDelivery:  TransmissionDeliveryOverlay,
		EffectiveDelivery:  TransmissionDeliveryOverlay,
		AcceptedAt:         acceptedAt,
		Targets:            targets,
	}
}

func transmissionTarget(credentials OnboardingCredentials, online bool) CreateTransmissionTarget {
	return CreateTransmissionTarget{
		OrbitID:              credentials.OrbitID,
		ActorID:              credentials.ActorID,
		Slot:                 credentials.Slot,
		OnlineAtAcceptance:   online,
		MediaClipCapable:     true,
		OverlayCapable:       true,
		InterruptCapable:     true,
		InterruptResumeReady: true,
	}
}

func TestCreateTransmissionPersistsImmutableOnlineAndOfflineSnapshots(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	online, err := st.CreateSelfServiceOrbit("Online transmission target")
	if err != nil {
		t.Fatal(err)
	}
	offline, err := st.CreateSelfServiceOrbit("Offline transmission target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	offlineTarget := transmissionTarget(offline, false)
	offlineTarget.Status = TransmissionTargetMissedOffline
	offlineTarget.ReasonCode = TransmissionReasonOfflineAtAcceptance
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+3, transmissionTarget(online, true), offlineTarget,
	))
	if err != nil {
		t.Fatal(err)
	}
	if !transmissionIDPattern.MatchString(created.Transmission.ID) ||
		created.Transmission.MediaID != media.ID ||
		created.Transmission.Status != TransmissionStatusAccepted ||
		created.Transmission.AcceptedAt != now+3 ||
		created.Transmission.ExpiresAt != now+3+int64((5*time.Minute)/time.Millisecond) {
		t.Fatalf("created transmission=%+v", created.Transmission)
	}
	if len(created.Targets) != 2 || !created.Targets[0].OnlineAtAcceptance ||
		created.Targets[1].OnlineAtAcceptance ||
		created.Targets[1].Status != TransmissionTargetMissedOffline ||
		created.Targets[1].ReasonCode != TransmissionReasonOfflineAtAcceptance ||
		created.Targets[1].EndedAt != now+3 ||
		created.Targets[1].LastReceiptAt != now+3 {
		t.Fatalf("created targets=%+v", created.Targets)
	}
	for _, target := range created.Targets {
		var pairedAt int64
		if err := st.db.QueryRow(`SELECT slot_paired_at FROM installation_credentials
WHERE actor_id = ? AND slot_orbit_id = ? AND slot_name = ?`,
			target.ActorID, target.OrbitID, target.Slot,
		).Scan(&pairedAt); err != nil {
			t.Fatal(err)
		}
		if target.BindingPairedAt != pairedAt {
			t.Fatalf("target paired_at=%d live=%d", target.BindingPairedAt, pairedAt)
		}
	}
	if _, err := st.db.Exec(`UPDATE transmission_targets
SET online_at_acceptance = 0 WHERE transmission_id = ? AND actor_id = ?`,
		created.Transmission.ID, online.ActorID,
	); err == nil || !strings.Contains(err.Error(), "snapshot is immutable") {
		t.Fatalf("mutable target snapshot error=%v", err)
	}
	if _, err := st.db.Exec(`UPDATE transmissions
SET accepted_at = accepted_at + 1 WHERE id = ?`, created.Transmission.ID,
	); err == nil || !strings.Contains(err.Error(), "snapshot is immutable") {
		t.Fatalf("mutable transmission snapshot error=%v", err)
	}

	second, err := st.CreateTransmission(transmissionParams(
		media, source, now+4, transmissionTarget(online, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	third, err := st.CreateTransmission(transmissionParams(
		media, source, now+4, transmissionTarget(online, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := st.ListTransmissionDomainFIFO(PlaybackDomainOrbit, source.OrbitID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ordered) != 3 || ordered[0].ID != created.Transmission.ID {
		t.Fatalf("FIFO order=%+v", ordered)
	}
	sameTimestamp := []string{second.Transmission.ID, third.Transmission.ID}
	sort.Strings(sameTimestamp)
	if ordered[1].ID != sameTimestamp[0] || ordered[2].ID != sameTimestamp[1] {
		t.Fatalf("ULID tie order=%s,%s want=%v", ordered[1].ID, ordered[2].ID, sameTimestamp)
	}
	if err := foreignKeyCheck(st.db); err != nil {
		t.Fatal(err)
	}
}

func TestTransmissionTargetTransitionsAreGenerationSafeAndAggregateDeterministically(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	targetCredentials, err := st.CreateSelfServiceOrbit("Receipt target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+3, transmissionTarget(targetCredentials, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	target := created.Targets[0]
	advanced, err := st.AdvanceTransmissionTargetGeneration(
		target.TransmissionID, target.OrbitID, target.ActorID, target.Slot,
		target.Revision, target.Generation, now+4,
	)
	if err != nil || advanced.Generation != target.Generation+1 ||
		advanced.Revision != target.Revision+1 {
		t.Fatalf("advanced generation=%+v err=%v", advanced, err)
	}
	if _, err := st.TransitionTransmissionTarget(TransitionTransmissionTargetParams{
		TransmissionID: target.TransmissionID, OrbitID: target.OrbitID,
		ActorID: target.ActorID, Slot: target.Slot,
		ExpectedRevision: advanced.Revision, Generation: target.Generation,
		Status: TransmissionTargetPreparing, OccurredAt: now + 5,
	}); !errors.Is(err, ErrTransmissionStateConflict) {
		t.Fatalf("invalidated receipt generation error=%v", err)
	}
	target = advanced
	steps := []struct {
		status TransmissionTargetStatus
		reason TransmissionReason
		want   TransmissionStatus
	}{
		{TransmissionTargetPreparing, "", TransmissionStatusPreparing},
		{TransmissionTargetReady, "", TransmissionStatusPreparing},
		{TransmissionTargetScheduled, "", TransmissionStatusScheduled},
		{TransmissionTargetPlaying, "", TransmissionStatusPlaying},
		{TransmissionTargetPlayed, TransmissionReasonCompleted, TransmissionStatusPlayed},
	}
	var last TransmissionTargetTransition
	for index, step := range steps {
		last, err = st.TransitionTransmissionTarget(TransitionTransmissionTargetParams{
			TransmissionID:   target.TransmissionID,
			OrbitID:          target.OrbitID,
			ActorID:          target.ActorID,
			Slot:             target.Slot,
			ExpectedRevision: target.Revision,
			Generation:       target.Generation,
			Status:           step.status,
			ReasonCode:       step.reason,
			OccurredAt:       now + 5 + int64(index),
		})
		if err != nil {
			t.Fatalf("transition %s: %v", step.status, err)
		}
		if !last.Changed || last.Target.Status != step.status ||
			last.Transmission.Status != step.want {
			t.Fatalf("transition %s=%+v", step.status, last)
		}
		target = last.Target
	}
	if last.Target.ReadyAt != now+6 || last.Target.ScheduledAt != now+7 ||
		last.Target.StartedAt != now+8 || last.Target.EndedAt != now+9 ||
		last.Target.LastReceiptAt != now+9 ||
		last.Transmission.ReasonCode != TransmissionReasonCompleted ||
		last.Transmission.CompletedAt != now+9 {
		t.Fatalf("terminal receipt=%+v aggregate=%+v", last.Target, last.Transmission)
	}
	idempotent, err := st.TransitionTransmissionTarget(TransitionTransmissionTargetParams{
		TransmissionID:   target.TransmissionID,
		OrbitID:          target.OrbitID,
		ActorID:          target.ActorID,
		Slot:             target.Slot,
		ExpectedRevision: 1,
		Generation:       target.Generation,
		Status:           TransmissionTargetPlayed,
		ReasonCode:       TransmissionReasonCompleted,
		OccurredAt:       now + 50,
	})
	if err != nil || idempotent.Changed {
		t.Fatalf("idempotent receipt=%+v err=%v", idempotent, err)
	}
	if _, err := st.TransitionTransmissionTarget(TransitionTransmissionTargetParams{
		TransmissionID: target.TransmissionID, OrbitID: target.OrbitID,
		ActorID: target.ActorID, Slot: target.Slot, ExpectedRevision: target.Revision,
		Generation: target.Generation + 1, Status: TransmissionTargetPlayed,
		ReasonCode: TransmissionReasonCompleted, OccurredAt: now + 51,
	}); !errors.Is(err, ErrTransmissionStateConflict) {
		t.Fatalf("stale generation error=%v", err)
	}
	if _, err := st.TransitionTransmissionTarget(TransitionTransmissionTargetParams{
		TransmissionID: target.TransmissionID, OrbitID: target.OrbitID,
		ActorID: target.ActorID, Slot: target.Slot, ExpectedRevision: target.Revision,
		Generation: target.Generation, Status: TransmissionTargetFailed,
		ReasonCode: TransmissionReasonCompleted, OccurredAt: now + 52,
	}); !errors.Is(err, ErrTransmissionInvalid) {
		t.Fatalf("invalid reason error=%v", err)
	}
	if _, err := st.db.Exec(`UPDATE transmission_targets SET actor_id = actor_id + 1
WHERE transmission_id = ?`, target.TransmissionID); err == nil ||
		!strings.Contains(err.Error(), "snapshot is immutable") {
		t.Fatalf("mutable receipt identity error=%v", err)
	}
}

func TestTransmissionLevelCauseDrivesTerminalAggregate(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	targetCredentials, err := st.CreateSelfServiceOrbit("Cancellation target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+3, transmissionTarget(targetCredentials, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	withCause, err := st.CommitTransmissionCause(CommitTransmissionCauseParams{
		TransmissionID: created.Transmission.ID, ExpectedRevision: created.Transmission.Revision,
		Cause: TransmissionReasonSenderCancelled, OccurredAt: now + 4,
	})
	if err != nil || withCause.CancellationCause != TransmissionReasonSenderCancelled ||
		withCause.Status != TransmissionStatusAccepted {
		t.Fatalf("committed cause=%+v err=%v", withCause, err)
	}
	target := created.Targets[0]
	transition, err := st.TransitionTransmissionTarget(TransitionTransmissionTargetParams{
		TransmissionID: target.TransmissionID, OrbitID: target.OrbitID,
		ActorID: target.ActorID, Slot: target.Slot,
		ExpectedRevision: target.Revision, Generation: target.Generation,
		Status:     TransmissionTargetCancelled,
		ReasonCode: TransmissionReasonSenderCancelled, OccurredAt: now + 5,
	})
	if err != nil || transition.Transmission.Status != TransmissionStatusCancelled ||
		transition.Transmission.ReasonCode != TransmissionReasonSenderCancelled ||
		transition.Transmission.CompletedAt != now+5 {
		t.Fatalf("cancelled aggregate=%+v err=%v", transition, err)
	}
	replayed, err := st.CommitTransmissionCause(CommitTransmissionCauseParams{
		TransmissionID: created.Transmission.ID, ExpectedRevision: 1,
		Cause: TransmissionReasonSenderCancelled, OccurredAt: now + 50,
	})
	if err != nil || replayed.Revision != transition.Transmission.Revision {
		t.Fatalf("cause replay=%+v err=%v", replayed, err)
	}
}

func TestTransmissionTargetConcurrentTerminalReceiptsCommitExactlyOneOutcome(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	targetCredentials, err := st.CreateSelfServiceOrbit("Concurrent receipt target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+3, transmissionTarget(targetCredentials, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	target := created.Targets[0]
	candidates := []struct {
		status TransmissionTargetStatus
		reason TransmissionReason
	}{
		{TransmissionTargetPlayed, TransmissionReasonCompleted},
		{TransmissionTargetFailed, TransmissionReasonInternalError},
	}
	start := make(chan struct{})
	errorsByWorker := make([]error, len(candidates))
	results := make([]TransmissionTargetTransition, len(candidates))
	var wait sync.WaitGroup
	for index, candidate := range candidates {
		wait.Add(1)
		go func(index int, candidate struct {
			status TransmissionTargetStatus
			reason TransmissionReason
		}) {
			defer wait.Done()
			<-start
			results[index], errorsByWorker[index] = st.TransitionTransmissionTarget(
				TransitionTransmissionTargetParams{
					TransmissionID: target.TransmissionID, OrbitID: target.OrbitID,
					ActorID: target.ActorID, Slot: target.Slot,
					ExpectedRevision: target.Revision, Generation: target.Generation,
					Status: candidate.status, ReasonCode: candidate.reason,
					OccurredAt: now + 4,
				},
			)
		}(index, candidate)
	}
	close(start)
	wait.Wait()
	successes, conflicts := 0, 0
	for index, err := range errorsByWorker {
		switch {
		case err == nil:
			successes++
			if !results[index].Changed {
				t.Fatalf("winner did not commit: %+v", results[index])
			}
		case errors.Is(err, ErrTransmissionStateConflict):
			conflicts++
		default:
			t.Fatalf("worker %d error=%v", index, err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("receipt race successes=%d conflicts=%d results=%+v errors=%+v",
			successes, conflicts, results, errorsByWorker)
	}
	targets, err := st.TransmissionTargets(created.Transmission.ID)
	if err != nil || len(targets) != 1 || targets[0].Revision != 2 ||
		!terminalTransmissionTargetStatus(targets[0].Status) {
		t.Fatalf("receipt race target=%+v err=%v", targets, err)
	}
}

func TestTransmissionTargetSnapshotIsTheOnlyGenericMediaACL(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	targetCredentials, err := st.CreateSelfServiceOrbit("ACL target")
	if err != nil {
		t.Fatal(err)
	}
	nontarget, err := st.CreateSelfServiceOrbit("ACL nontarget")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	code, err := st.ProposeLink(source.OrbitID, source.ActorID)
	if err != nil {
		t.Fatal(err)
	}
	linkID, _, err := st.AcceptByCode(code, targetCredentials.OrbitID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ActivateLink(linkID); err != nil {
		t.Fatal(err)
	}
	params := transmissionParams(
		media, source, now+3, transmissionTarget(targetCredentials, true),
	)
	params.PlaybackDomainKind = PlaybackDomainApproach
	params.PlaybackDomainID = linkID
	created, err := st.CreateTransmission(params)
	if err != nil {
		t.Fatal(err)
	}
	identity := MediaTargetIdentity{
		MediaID: media.ID, OrbitID: targetCredentials.OrbitID,
		ActorID: targetCredentials.ActorID, Slot: targetCredentials.Slot,
	}
	allowed, err := st.AllowsMediaDownload(context.Background(), identity)
	if err != nil || !allowed {
		t.Fatalf("accepted ACL allowed=%v err=%v", allowed, err)
	}
	for name, denied := range map[string]MediaTargetIdentity{
		"source member without snapshot": {
			MediaID: media.ID, OrbitID: source.OrbitID,
			ActorID: source.ActorID, Slot: source.Slot,
		},
		"foreign installation": {
			MediaID: media.ID, OrbitID: nontarget.OrbitID,
			ActorID: nontarget.ActorID, Slot: nontarget.Slot,
		},
		"guessed media": {
			MediaID: "m_00000000000000000000000000",
			OrbitID: targetCredentials.OrbitID, ActorID: targetCredentials.ActorID,
			Slot: targetCredentials.Slot,
		},
	} {
		allowed, err := st.AllowsMediaDownload(context.Background(), denied)
		if err != nil || allowed {
			t.Fatalf("%s allowed=%v err=%v", name, allowed, err)
		}
	}
	if err := st.BreakLink(linkID); err != nil {
		t.Fatal(err)
	}
	if allowed, err := st.AllowsMediaDownload(context.Background(), identity); err != nil || !allowed {
		t.Fatalf("approach split rewrote ACL allowed=%v err=%v", allowed, err)
	}
	block, err := st.CreateTransmissionBlock(CreateTransmissionBlockParams{
		OwnerScope: BlockOwnerActor, OwnerOrbitID: targetCredentials.OrbitID,
		OwnerActorID: targetCredentials.ActorID, BlockedKind: BlockedSubjectActor,
		BlockedActorID: source.ActorID, AuthorizedByActorID: targetCredentials.ActorID,
		CreatedAt: now + 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if allowed, err := st.AllowsMediaDownload(context.Background(), identity); err != nil || allowed {
		t.Fatalf("active block ACL allowed=%v err=%v", allowed, err)
	}
	if _, err := st.RevokeTransmissionBlock(
		block.Block.ID, targetCredentials.ActorID, block.Block.Revision, now+5,
	); err != nil {
		t.Fatal(err)
	}
	if allowed, err := st.AllowsMediaDownload(context.Background(), identity); err != nil || !allowed {
		t.Fatalf("removed block ACL allowed=%v err=%v", allowed, err)
	}
	if found, err := st.RevokeSlot(targetCredentials.OrbitID, targetCredentials.Slot); err != nil || !found {
		t.Fatalf("revoke target found=%v err=%v", found, err)
	}
	if allowed, err := st.AllowsMediaDownload(context.Background(), identity); err != nil || allowed {
		t.Fatalf("revoked binding ACL allowed=%v err=%v", allowed, err)
	}
	_, replacementToken, err := st.PairSlot(targetCredentials.OrbitID, 0)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := st.ResolveTokenActorContext(replacementToken)
	if err != nil {
		t.Fatal(err)
	}
	if allowed, err := st.AllowsMediaDownload(context.Background(), MediaTargetIdentity{
		MediaID: media.ID, OrbitID: replacement.OrbitID,
		ActorID: replacement.ActorID, Slot: replacement.Slot,
	}); err != nil || allowed {
		t.Fatalf("replacement inherited ACL allowed=%v err=%v", allowed, err)
	}
	targets, err := st.TransmissionTargets(created.Transmission.ID)
	if err != nil || len(targets) != 1 || targets[0].ActorID != targetCredentials.ActorID {
		t.Fatalf("immutable historical target=%+v err=%v", targets, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := st.AllowsMediaDownload(cancelled, identity); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ACL context error=%v", err)
	}
}

func TestTransmissionBlocksAndLayeredDNDPersistOwnershipAndRevision(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	target, err := st.CreateSelfServiceOrbit("Policy target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	personal, err := st.CreateTransmissionBlock(CreateTransmissionBlockParams{
		OwnerScope: BlockOwnerActor, OwnerOrbitID: target.OrbitID,
		OwnerActorID: target.ActorID, BlockedKind: BlockedSubjectActor,
		BlockedActorID: source.ActorID, AuthorizedByActorID: target.ActorID,
		CreatedAt: now,
	})
	if err != nil || personal.Reused {
		t.Fatalf("personal block=%+v err=%v", personal, err)
	}
	replay, err := st.CreateTransmissionBlock(CreateTransmissionBlockParams{
		OwnerScope: BlockOwnerActor, OwnerOrbitID: target.OrbitID,
		OwnerActorID: target.ActorID, BlockedKind: BlockedSubjectActor,
		BlockedActorID: source.ActorID, AuthorizedByActorID: target.ActorID,
		CreatedAt: now + 1,
	})
	if err != nil || !replay.Reused || replay.Block.ID != personal.Block.ID {
		t.Fatalf("block replay=%+v err=%v", replay, err)
	}
	decision, err := st.TransmissionBlockDecision(
		context.Background(), target.OrbitID, target.ActorID,
		source.OrbitID, source.ActorID,
	)
	if err != nil || !decision.Blocked || decision.Reason != TransmissionReasonActorBlocked {
		t.Fatalf("personal decision=%+v err=%v", decision, err)
	}
	if _, err := st.RevokeTransmissionBlock(
		personal.Block.ID, target.ActorID, personal.Block.Revision, now+2,
	); err != nil {
		t.Fatal(err)
	}
	orbitBlock, err := st.CreateTransmissionBlock(CreateTransmissionBlockParams{
		OwnerScope: BlockOwnerOrbit, OwnerOrbitID: target.OrbitID,
		BlockedKind: BlockedSubjectOrbit, BlockedOrbitID: source.OrbitID,
		AuthorizedByActorID: target.ActorID, CreatedAt: now + 3,
	})
	if err != nil || orbitBlock.Block.OwnerActorID != 0 {
		t.Fatalf("orbit block=%+v err=%v", orbitBlock, err)
	}
	decision, err = st.TransmissionBlockDecision(
		context.Background(), target.OrbitID, target.ActorID,
		source.OrbitID, source.ActorID,
	)
	if err != nil || decision.Reason != TransmissionReasonOrbitBlocked {
		t.Fatalf("orbit decision=%+v err=%v", decision, err)
	}

	node, err := st.SetNodeDND(SetNodeDNDParams{
		OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
		Mode: DNDMessagesOnly, Revision: 1, UpdatedAt: now + 4,
	})
	if err != nil || !node.Changed || node.Setting.Mode != DNDMessagesOnly {
		t.Fatalf("node DND=%+v err=%v", node, err)
	}
	nodeReplay, err := st.SetNodeDND(SetNodeDNDParams{
		OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
		Mode: DNDMessagesOnly, Revision: 1, UpdatedAt: now + 5,
	})
	if err != nil || nodeReplay.Changed {
		t.Fatalf("node DND replay=%+v err=%v", nodeReplay, err)
	}
	if _, err := st.SetNodeDND(SetNodeDNDParams{
		OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
		Mode: DNDAllowAll, Revision: 1, UpdatedAt: now + 5,
	}); !errors.Is(err, ErrDNDRevisionConflict) {
		t.Fatalf("node same-revision conflict=%v", err)
	}
	localUntil := now + int64((10*time.Minute)/time.Millisecond)
	node, err = st.SetNodeDND(SetNodeDNDParams{
		OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
		Mode: DNDMutedUntil, MutedUntil: localUntil,
		Revision: 2, UpdatedAt: now + 6,
	})
	if err != nil || !node.Changed {
		t.Fatalf("node muted DND=%+v err=%v", node, err)
	}
	orbitUntil := now + int64((20*time.Minute)/time.Millisecond)
	orbit, err := st.SetOrbitDND(SetOrbitDNDParams{
		OrbitID: target.OrbitID, AuthorizedByActorID: target.ActorID,
		Mode: DNDMutedUntil, MutedUntil: orbitUntil,
		Revision: 1, UpdatedAt: now + 7,
	})
	if err != nil || !orbit.Changed {
		t.Fatalf("orbit DND=%+v err=%v", orbit, err)
	}
	effective, err := st.EffectiveDND(context.Background(), MediaTargetIdentity{
		OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
	}, now+8)
	if err != nil || effective.Mode != DNDMutedUntil ||
		effective.MutedUntil != orbitUntil || effective.Reason != TransmissionReasonOrbitDND ||
		effective.NodeRevision != 2 || effective.OrbitRevision != 1 {
		t.Fatalf("effective DND=%+v err=%v", effective, err)
	}
	effective, err = st.EffectiveDND(context.Background(), MediaTargetIdentity{
		OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
	}, orbitUntil)
	if err != nil || effective.Mode != DNDAllowAll || effective.Reason != "" {
		t.Fatalf("expired effective DND=%+v err=%v", effective, err)
	}

	if err := st.AddMember(target.OrbitID, 98001, "companion"); err != nil {
		t.Fatal(err)
	}
	_, companionToken, err := st.PairSlot(target.OrbitID, 98001)
	if err != nil {
		t.Fatal(err)
	}
	companion, err := st.ResolveTokenActorContext(companionToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetOrbitDND(SetOrbitDNDParams{
		OrbitID: target.OrbitID, AuthorizedByActorID: companion.ActorID,
		Mode: DNDAllowAll, Revision: 2, UpdatedAt: now + 9,
	}); !errors.Is(err, ErrTransmissionPolicyForbidden) {
		t.Fatalf("companion orbit DND error=%v", err)
	}
	if _, err := st.CreateTransmissionBlock(CreateTransmissionBlockParams{
		OwnerScope: BlockOwnerOrbit, OwnerOrbitID: target.OrbitID,
		BlockedKind: BlockedSubjectOrbit, BlockedOrbitID: source.OrbitID,
		AuthorizedByActorID: companion.ActorID, CreatedAt: now + 9,
	}); !errors.Is(err, ErrTransmissionPolicyForbidden) {
		t.Fatalf("companion orbit block error=%v", err)
	}
}

func TestTransmissionSchemaInstallIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "transmission-schema-atomic.db")
	st := openMigrationStore(t, path)
	if _, err := st.db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(orbitSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(identitySchema); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(mediaIngestSchema); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("transmission DDL interrupted")
	st.testCheckpoint = func(name string) error {
		if name == "transmission_ddl_before_commit" {
			return injected
		}
		return nil
	}
	if err := st.initTransmissionSchema(); !errors.Is(err, injected) {
		t.Fatalf("injected schema error=%v", err)
	}
	for _, table := range []string{
		"transmissions", "transmission_targets", "blocks",
		"node_dnd_settings", "orbit_dnd_settings",
	} {
		exists, err := tableExists(st.db, table)
		if err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Fatalf("partially committed table %s", table)
		}
	}
	st.testCheckpoint = nil
	if err := st.initTransmissionSchema(); err != nil {
		t.Fatal(err)
	}
	if err := foreignKeyCheck(st.db); err != nil {
		t.Fatal(err)
	}
}
