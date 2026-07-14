package store

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func resolvedTransmissionParams(
	source OnboardingCredentials,
	media MediaItem,
	acceptedAt int64,
) CreateResolvedTransmissionParams {
	return CreateResolvedTransmissionParams{
		ExpectedActorID:    source.ActorID,
		Bearer:             source.ControlToken,
		IdempotencyKeyHash: strings.Repeat("1", 64),
		RequestHash:        strings.Repeat("2", 64),
		MediaID:            media.ID,
		AudienceKind:       TransmissionAudienceCurrentAir,
		OriginKind:         TransmissionOriginFile,
		IncludeOrigin:      true,
		RequestedDelivery:  TransmissionDeliveryOverlay,
		AcceptedAt:         acceptedAt,
	}
}

func fullTransmissionAvailability(
	credentials OnboardingCredentials,
	now int64,
) TransmissionTargetAvailability {
	return TransmissionTargetAvailability{
		OrbitID: credentials.OrbitID, Slot: credentials.Slot,
		Connected: true, LastSeenAt: now,
		CredentialTokenHash: hashToken(credentials.NodeToken),
		MediaClipCapable:    true, OverlayCapable: true,
		InterruptCapable: true, MainActive: true,
		InterruptResumeReady: true,
	}
}

func addTransmissionInstallation(
	t *testing.T,
	st *Store,
	owner OnboardingCredentials,
	role string,
) OnboardingCredentials {
	t.Helper()
	invite, err := st.IssueDeviceInvite(owner.ActorID, owner.ControlToken, role)
	if err != nil {
		t.Fatal(err)
	}
	joined, err := st.ConsumeDeviceInvite(invite.Code)
	if err != nil {
		t.Fatal(err)
	}
	return joined
}

func activateTransmissionApproach(
	t *testing.T,
	st *Store,
	left, right OnboardingCredentials,
) int64 {
	t.Helper()
	code, err := st.ProposeLink(left.OrbitID, left.ActorID)
	if err != nil {
		t.Fatal(err)
	}
	linkID, _, err := st.AcceptByCode(code, right.OrbitID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ActivateLink(linkID); err != nil {
		t.Fatal(err)
	}
	return linkID
}

func TestResolvedTransmissionSealsMixedAudienceIdempotencyVisibilityAndCancel(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	companion := addTransmissionInstallation(t, st, source, "companion")
	peer, err := st.CreateSelfServiceOrbit("Transmission peer")
	if err != nil {
		t.Fatal(err)
	}
	linkID := activateTransmissionApproach(t, st, source, peer)
	stranger, err := st.CreateSelfServiceOrbit("Transmission stranger")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	params := resolvedTransmissionParams(source, media, now+3)
	sourceAvailability := fullTransmissionAvailability(source, params.AcceptedAt)
	companionAvailability := fullTransmissionAvailability(companion, params.AcceptedAt)
	companionAvailability.OverlayCapable = false
	peerAvailability := fullTransmissionAvailability(peer, params.AcceptedAt)
	peerAvailability.Connected = false
	params.Availability = []TransmissionTargetAvailability{
		sourceAvailability, companionAvailability, peerAvailability,
	}
	created, err := st.CreateResolvedTransmission(params)
	if err != nil {
		t.Fatal(err)
	}
	if created.Reused || created.Challenge != nil ||
		created.Creation.Transmission.PlaybackDomainKind != PlaybackDomainApproach ||
		created.Creation.Transmission.PlaybackDomainID != linkID ||
		created.Creation.Transmission.EffectiveDelivery != TransmissionDeliveryAfterCurrent ||
		created.Creation.Transmission.DowngradeReason != TransmissionDowngradeMissingOverlay ||
		len(created.Creation.Targets) != 3 {
		t.Fatalf("created=%+v", created)
	}
	statuses := map[int64]TransmissionTargetStatus{}
	for _, target := range created.Creation.Targets {
		statuses[target.ActorID] = target.Status
	}
	if statuses[source.ActorID] != TransmissionTargetAccepted ||
		statuses[companion.ActorID] != TransmissionTargetAccepted ||
		statuses[peer.ActorID] != TransmissionTargetMissedOffline {
		t.Fatalf("target statuses=%v", statuses)
	}

	retryParams := params
	retryParams.AcceptedAt += 1000
	retry, err := st.CreateResolvedTransmission(retryParams)
	if err != nil || !retry.Reused ||
		retry.Creation.Transmission.ID != created.Creation.Transmission.ID ||
		retry.Creation.Transmission.AcceptedAt != params.AcceptedAt {
		t.Fatalf("retry=%+v err=%v", retry, err)
	}
	conflict := params
	conflict.RequestHash = strings.Repeat("3", 64)
	if _, err := st.CreateResolvedTransmission(conflict); !errors.Is(err, ErrTransmissionIdempotencyConflict) {
		t.Fatalf("idempotency conflict=%v", err)
	}

	creatorView, err := st.GetAuthorizedTransmission(
		source.ActorID, source.ControlToken, created.Creation.Transmission.ID,
	)
	if err != nil || len(creatorView.Targets) != 3 || !creatorView.CanCancel {
		t.Fatalf("creator view=%+v err=%v", creatorView, err)
	}
	companionView, err := st.GetAuthorizedTransmission(
		companion.ActorID, companion.NodeToken, created.Creation.Transmission.ID,
	)
	if err != nil || len(companionView.Targets) != 1 ||
		companionView.Targets[0].ActorID != companion.ActorID || companionView.CanCancel ||
		companionView.TargetCount != 3 ||
		companionView.TargetStatusCounts[TransmissionTargetMissedOffline] != 1 {
		t.Fatalf("companion view=%+v err=%v", companionView, err)
	}
	peerView, err := st.GetAuthorizedTransmission(
		peer.ActorID, peer.NodeToken, created.Creation.Transmission.ID,
	)
	if err != nil || len(peerView.Targets) != 1 ||
		peerView.Targets[0].Status != TransmissionTargetMissedOffline {
		t.Fatalf("peer view=%+v err=%v", peerView, err)
	}
	if _, err := st.GetAuthorizedTransmission(
		stranger.ActorID, stranger.NodeToken, created.Creation.Transmission.ID,
	); !errors.Is(err, ErrTransmissionNotFound) {
		t.Fatalf("stranger visibility error=%v", err)
	}
	if _, err := st.CancelAuthorizedTransmission(
		companion.ActorID, companion.ControlToken,
		created.Creation.Transmission.ID, params.AcceptedAt+1,
	); !errors.Is(err, ErrTransmissionNotFound) {
		t.Fatalf("companion cancel error=%v", err)
	}
	cancelled, err := st.CancelAuthorizedTransmission(
		source.ActorID, source.ControlToken,
		created.Creation.Transmission.ID, params.AcceptedAt+2,
	)
	if err != nil || !cancelled.Changed ||
		cancelled.Transmission.Status != TransmissionStatusCancelled ||
		cancelled.Transmission.ReasonCode != TransmissionReasonSenderCancelled ||
		len(cancelled.DisarmTargets) != 0 {
		t.Fatalf("cancelled=%+v err=%v", cancelled, err)
	}
	repeated, err := st.CancelAuthorizedTransmission(
		source.ActorID, source.ControlToken,
		created.Creation.Transmission.ID, params.AcceptedAt+3,
	)
	if err != nil || repeated.Changed || repeated.Transmission.Status != TransmissionStatusCancelled {
		t.Fatalf("repeated cancel=%+v err=%v", repeated, err)
	}

	var requestRows int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmission_requests
WHERE actor_id = ? AND idempotency_key_hash = ? AND request_hash = ?`,
		source.ActorID, params.IdempotencyKeyHash, params.RequestHash,
	).Scan(&requestRows); err != nil || requestRows != 1 {
		t.Fatalf("request rows=%d err=%v", requestRows, err)
	}
}

func TestResolvedTransmissionRejectsCapabilitiesFromStaleBinding(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	params := resolvedTransmissionParams(source, media, now+3)
	params.AudienceKind = TransmissionAudienceOwnBarycenter
	stale := fullTransmissionAvailability(source, params.AcceptedAt)
	stale.CredentialTokenHash = strings.Repeat("0", 64)
	params.Availability = []TransmissionTargetAvailability{stale}
	created, err := st.CreateResolvedTransmission(params)
	if err != nil || len(created.Creation.Targets) != 1 {
		t.Fatalf("stale-binding create=%+v err=%v", created, err)
	}
	target := created.Creation.Targets[0]
	if target.Status != TransmissionTargetMissedOffline ||
		target.OnlineAtAcceptance || target.MediaClipCapable ||
		target.OverlayCapable || target.InterruptCapable ||
		target.InterruptResumeReady {
		t.Fatalf("stale-binding target=%+v", target)
	}

	fresh := params
	fresh.IdempotencyKeyHash = strings.Repeat("8", 64)
	fresh.RequestHash = strings.Repeat("9", 64)
	fresh.AcceptedAt++
	freshAvailability := fullTransmissionAvailability(source, fresh.AcceptedAt)
	// The playback hub accepts either exact current node or control
	// credentials, so both hashes must witness the same binding generation.
	freshAvailability.CredentialTokenHash = hashToken(source.ControlToken)
	fresh.Availability = []TransmissionTargetAvailability{freshAvailability}
	accepted, err := st.CreateResolvedTransmission(fresh)
	if err != nil || len(accepted.Creation.Targets) != 1 ||
		accepted.Creation.Targets[0].Status != TransmissionTargetAccepted ||
		!accepted.Creation.Targets[0].OnlineAtAcceptance ||
		!accepted.Creation.Targets[0].MediaClipCapable {
		t.Fatalf("fresh-binding create=%+v err=%v", accepted, err)
	}
}

func TestResolvedTransmissionInterruptConfirmationIsExplicitBoundAndSingleUse(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	params := resolvedTransmissionParams(source, media, now+3)
	params.AudienceKind = TransmissionAudienceOwnBarycenter
	params.RequestedDelivery = TransmissionDeliveryInterrupt
	params.IdempotencyKeyHash = strings.Repeat("4", 64)
	params.RequestHash = strings.Repeat("5", 64)
	params.ChallengeTokenHash = strings.Repeat("6", 64)
	availability := fullTransmissionAvailability(source, params.AcceptedAt)
	availability.InterruptCapable = false
	availability.InterruptResumeReady = false
	params.Availability = []TransmissionTargetAvailability{availability}

	challenged, err := st.CreateResolvedTransmission(params)
	if err != nil || challenged.Challenge == nil ||
		challenged.Challenge.Alternatives[0].Delivery != TransmissionDeliveryOverlay ||
		!challenged.Challenge.Alternatives[0].Available ||
		challenged.Challenge.ExpiresAt != params.AcceptedAt+transmissionConfirmationTTL.Milliseconds() {
		t.Fatalf("challenge=%+v err=%v", challenged, err)
	}
	var transmissions, requests int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmissions`).Scan(&transmissions); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmission_requests`).Scan(&requests); err != nil {
		t.Fatal(err)
	}
	if transmissions != 0 || requests != 0 {
		t.Fatalf("challenge reserved transmission=%d request=%d", transmissions, requests)
	}

	confirmed := params
	confirmed.AcceptedAt += 100
	confirmed.ChallengeTokenHash = strings.Repeat("7", 64)
	confirmed.Confirmation = &ConfirmTransmissionFallback{
		TokenHash: params.ChallengeTokenHash,
		Delivery:  TransmissionDeliveryOverlay,
	}
	created, err := st.CreateResolvedTransmission(confirmed)
	if err != nil || created.Challenge != nil ||
		created.Creation.Transmission.RequestedDelivery != TransmissionDeliveryInterrupt ||
		created.Creation.Transmission.EffectiveDelivery != TransmissionDeliveryOverlay ||
		created.Creation.Transmission.DowngradeReason != TransmissionDowngradeConfirmedOverlay ||
		created.Creation.Transmission.AcceptedAt != confirmed.AcceptedAt {
		t.Fatalf("confirmed=%+v err=%v", created, err)
	}
	replayed := params
	replayed.AcceptedAt += 200
	replayed.ChallengeTokenHash = strings.Repeat("8", 64)
	retry, err := st.CreateResolvedTransmission(replayed)
	if err != nil || !retry.Reused ||
		retry.Creation.Transmission.ID != created.Creation.Transmission.ID {
		t.Fatalf("post-confirm replay=%+v err=%v", retry, err)
	}

	wrongKey := confirmed
	wrongKey.IdempotencyKeyHash = strings.Repeat("9", 64)
	wrongKey.RequestHash = strings.Repeat("a", 64)
	if _, err := st.CreateResolvedTransmission(wrongKey); !errors.Is(err, ErrTransmissionConfirmationInvalid) {
		t.Fatalf("replayed confirmation error=%v", err)
	}
	var consumedAt int64
	if err := st.db.QueryRow(`SELECT consumed_at
FROM transmission_fallback_confirmations WHERE token_hash = ?`,
		params.ChallengeTokenHash,
	).Scan(&consumedAt); err != nil || consumedAt != confirmed.AcceptedAt {
		t.Fatalf("consumed_at=%d err=%v", consumedAt, err)
	}

	worsened := params
	worsened.AcceptedAt += 300
	worsened.IdempotencyKeyHash = strings.Repeat("b", 64)
	worsened.RequestHash = strings.Repeat("c", 64)
	worsened.ChallengeTokenHash = strings.Repeat("d", 64)
	worsened.Confirmation = nil
	secondChallenge, err := st.CreateResolvedTransmission(worsened)
	if err != nil || secondChallenge.Challenge == nil ||
		!secondChallenge.Challenge.Alternatives[0].Available {
		t.Fatalf("second challenge=%+v err=%v", secondChallenge, err)
	}
	worsenedConfirmation := worsened
	worsenedConfirmation.AcceptedAt++
	worsenedConfirmation.ChallengeTokenHash = strings.Repeat("e", 64)
	worsenedConfirmation.Confirmation = &ConfirmTransmissionFallback{
		TokenHash: worsened.ChallengeTokenHash,
		Delivery:  TransmissionDeliveryOverlay,
	}
	worsenedAvailability := availability
	worsenedAvailability.OverlayCapable = false
	worsenedConfirmation.Availability = []TransmissionTargetAvailability{worsenedAvailability}
	renewed, err := st.CreateResolvedTransmission(worsenedConfirmation)
	if err != nil || renewed.Challenge == nil ||
		renewed.Challenge.Alternatives[0].Available {
		t.Fatalf("renewed challenge=%+v err=%v", renewed, err)
	}
	var worsenedRequestRows int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmission_requests
WHERE actor_id = ? AND idempotency_key_hash = ?`,
		source.ActorID, worsened.IdempotencyKeyHash,
	).Scan(&worsenedRequestRows); err != nil || worsenedRequestRows != 0 {
		t.Fatalf("worsened request rows=%d err=%v", worsenedRequestRows, err)
	}
}

func TestResolvedTransmissionConcurrentIdempotencyHasOneAcceptance(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	params := resolvedTransmissionParams(source, media, now+3)
	params.AudienceKind = TransmissionAudienceOwnBarycenter
	params.Availability = []TransmissionTargetAvailability{
		fullTransmissionAvailability(source, params.AcceptedAt),
	}
	start := make(chan struct{})
	results := make(chan ResolvedTransmissionCreation, 2)
	errorsOut := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			result, err := st.CreateResolvedTransmission(params)
			results <- result
			errorsOut <- err
		}()
	}
	ready.Wait()
	close(start)
	first, second := <-results, <-results
	firstErr, secondErr := <-errorsOut, <-errorsOut
	if firstErr != nil || secondErr != nil ||
		first.Creation.Transmission.ID != second.Creation.Transmission.ID ||
		first.Reused == second.Reused {
		t.Fatalf("concurrent results=%+v / %+v errors=%v / %v", first, second, firstErr, secondErr)
	}
	var transmissionRows, requestRows int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmissions`).Scan(&transmissionRows); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmission_requests`).Scan(&requestRows); err != nil {
		t.Fatal(err)
	}
	if transmissionRows != 1 || requestRows != 1 {
		t.Fatalf("concurrent rows transmissions=%d requests=%d", transmissionRows, requestRows)
	}
}

func TestCancelAuthorizedTransmissionReturnsGenerationBoundDisarm(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	params := resolvedTransmissionParams(source, media, now+3)
	params.AudienceKind = TransmissionAudienceOwnBarycenter
	params.Availability = []TransmissionTargetAvailability{
		fullTransmissionAvailability(source, params.AcceptedAt),
	}
	created, err := st.CreateResolvedTransmission(params)
	if err != nil {
		t.Fatal(err)
	}
	target := created.Creation.Targets[0]
	preparing, err := st.TransitionTransmissionTarget(TransitionTransmissionTargetParams{
		TransmissionID: target.TransmissionID, OrbitID: target.OrbitID,
		ActorID: target.ActorID, Slot: target.Slot,
		ExpectedRevision: target.Revision, Generation: target.Generation,
		Status: TransmissionTargetPreparing, OccurredAt: params.AcceptedAt + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := st.CancelAuthorizedTransmission(
		source.ActorID, source.ControlToken,
		created.Creation.Transmission.ID, params.AcceptedAt+2,
	)
	if err != nil || !cancelled.Changed ||
		cancelled.Transmission.Status != TransmissionStatusCancelling ||
		len(cancelled.DisarmTargets) != 1 ||
		cancelled.DisarmTargets[0].Generation != preparing.Target.Generation ||
		cancelled.DisarmTargets[0].Revision != preparing.Target.Revision+1 ||
		cancelled.DisarmTargets[0].ReasonCode != TransmissionReasonSenderCancelled {
		t.Fatalf("cancel result=%+v err=%v", cancelled, err)
	}
	disarm := cancelled.DisarmTargets[0]
	acknowledged, err := st.TransitionTransmissionTarget(TransitionTransmissionTargetParams{
		TransmissionID: disarm.TransmissionID, OrbitID: disarm.OrbitID,
		ActorID: disarm.ActorID, Slot: disarm.Slot,
		ExpectedRevision: disarm.Revision, Generation: disarm.Generation,
		Status:     TransmissionTargetCancelled,
		ReasonCode: TransmissionReasonSenderCancelled,
		OccurredAt: params.AcceptedAt + 3,
	})
	if err != nil || acknowledged.Transmission.Status != TransmissionStatusCancelled ||
		acknowledged.Transmission.ReasonCode != TransmissionReasonSenderCancelled {
		t.Fatalf("cancel acknowledgement=%+v err=%v", acknowledged, err)
	}
}

func TestResolvedTransmissionExplicitSelectorsDeduplicateThenFilterOrigin(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	companion := addTransmissionInstallation(t, st, source, "companion")
	foreign, err := st.CreateSelfServiceOrbit("Outside explicit domain")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	params := resolvedTransmissionParams(source, media, now+3)
	params.AudienceKind = TransmissionAudienceExplicit
	params.OriginKind = TransmissionOriginMicrophone
	params.IncludeOrigin = false
	params.Selectors = []TransmissionAudienceSelector{
		{Kind: TransmissionSelectorBarycenter, OrbitID: source.OrbitID},
		{Kind: TransmissionSelectorPulsar, OrbitID: source.OrbitID, Slot: companion.Slot},
	}
	params.Availability = []TransmissionTargetAvailability{
		fullTransmissionAvailability(source, params.AcceptedAt),
		fullTransmissionAvailability(companion, params.AcceptedAt),
	}
	created, err := st.CreateResolvedTransmission(params)
	if err != nil || len(created.Creation.Targets) != 1 ||
		created.Creation.Targets[0].ActorID != companion.ActorID {
		t.Fatalf("deduplicated/filter result=%+v err=%v", created, err)
	}
	outside := params
	outside.IdempotencyKeyHash = strings.Repeat("b", 64)
	outside.RequestHash = strings.Repeat("c", 64)
	outside.Selectors = []TransmissionAudienceSelector{
		{Kind: TransmissionSelectorBarycenter, OrbitID: foreign.OrbitID},
	}
	if _, err := st.CreateResolvedTransmission(outside); !errors.Is(err, ErrTransmissionAudienceNotFound) {
		t.Fatalf("outside selector error=%v", err)
	}

	emptyPeer, err := st.CreateSelfServiceOrbit("Empty explicit peer")
	if err != nil {
		t.Fatal(err)
	}
	activateTransmissionApproach(t, st, source, emptyPeer)
	if _, err := st.db.Exec(`UPDATE slots SET revoked_at = ? WHERE orbit_id = ?`,
		params.AcceptedAt, emptyPeer.OrbitID); err != nil {
		t.Fatal(err)
	}
	emptySelector := params
	emptySelector.IdempotencyKeyHash = strings.Repeat("d", 64)
	emptySelector.RequestHash = strings.Repeat("e", 64)
	emptySelector.Selectors = []TransmissionAudienceSelector{
		{Kind: TransmissionSelectorBarycenter, OrbitID: source.OrbitID},
		{Kind: TransmissionSelectorBarycenter, OrbitID: emptyPeer.OrbitID},
	}
	if _, err := st.CreateResolvedTransmission(emptySelector); !errors.Is(err, ErrTransmissionAudienceNotFound) {
		t.Fatalf("empty explicit selector error=%v", err)
	}
}

func TestResolvedTransmissionFailsClosedOnMultipleActiveLinks(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	peerA, err := st.CreateSelfServiceOrbit("Corrupt transmission peer A")
	if err != nil {
		t.Fatal(err)
	}
	peerB, err := st.CreateSelfServiceOrbit("Corrupt transmission peer B")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	for index, peer := range []OnboardingCredentials{peerA, peerB} {
		if _, err := st.db.Exec(`INSERT INTO links(
orbit_a, orbit_b, state, proposed_by, pending_orbit, code, created_at
) VALUES(?, ?, 'active', ?, 0, '', ?)`,
			source.OrbitID, peer.OrbitID, source.ActorID, now+int64(index)); err != nil {
			t.Fatal(err)
		}
	}
	media := readyLifecycleMedia(
		t, st, source, now+2, now+2+int64((7*24*time.Hour)/time.Millisecond),
	)
	params := resolvedTransmissionParams(source, media, now+5)
	if _, err := st.CreateResolvedTransmission(params); err == nil ||
		!strings.Contains(err.Error(), "multiple active links") {
		t.Fatalf("multiple-active-link error=%v", err)
	}
}

func TestResolvedTransmissionAppliesDNDAndBypassesItOnlyForLocalThisPulsar(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	companion := addTransmissionInstallation(t, st, source, "companion")
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(
		t, st, source, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	for index, target := range []OnboardingCredentials{source, companion} {
		if _, err := st.SetNodeDND(SetNodeDNDParams{
			OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
			Mode: DNDMutedUntil, MutedUntil: now + int64(time.Hour/time.Millisecond),
			Revision: 1, UpdatedAt: now + 3 + int64(index),
		}); err != nil {
			t.Fatal(err)
		}
	}
	params := resolvedTransmissionParams(source, media, now+5)
	params.AudienceKind = TransmissionAudienceOwnBarycenter
	params.Availability = []TransmissionTargetAvailability{
		fullTransmissionAvailability(source, params.AcceptedAt),
		fullTransmissionAvailability(companion, params.AcceptedAt),
	}
	muted, err := st.CreateResolvedTransmission(params)
	if err != nil {
		t.Fatal(err)
	}
	if len(muted.Creation.Targets) != 2 {
		t.Fatalf("muted targets=%+v", muted.Creation.Targets)
	}
	for _, target := range muted.Creation.Targets {
		if target.Status != TransmissionTargetMissedDND ||
			target.ReasonCode != TransmissionReasonLocalDND {
			t.Fatalf("remote DND target=%+v", target)
		}
	}
	local := params
	local.IdempotencyKeyHash = strings.Repeat("d", 64)
	local.RequestHash = strings.Repeat("e", 64)
	local.AudienceKind = TransmissionAudienceThisPulsar
	local.AcceptedAt++
	local.Availability = []TransmissionTargetAvailability{
		fullTransmissionAvailability(source, local.AcceptedAt),
	}
	localResult, err := st.CreateResolvedTransmission(local)
	if err != nil || len(localResult.Creation.Targets) != 1 ||
		localResult.Creation.Targets[0].Status != TransmissionTargetAccepted {
		t.Fatalf("local DND bypass=%+v err=%v", localResult, err)
	}
}
