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

func TestMigratedPairKeepsFrozenAirPolicyDefaultsAndSchedulerDomain(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	peer, err := st.CreateSelfServiceOrbit("Migrated policy peer")
	if err != nil {
		t.Fatal(err)
	}
	linkID := activateTransmissionApproach(t, st, source, peer)
	now := time.Now().UnixMilli()
	if _, err := st.CutoverLinksToAirs(1, now); err != nil {
		t.Fatal(err)
	}
	var airID string
	if err := st.db.QueryRow(`SELECT air_id FROM air_legacy_link_mappings WHERE link_id = ?`, linkID).
		Scan(&airID); err != nil {
		t.Fatal(err)
	}
	policy, err := st.AirPolicy(airID)
	if err != nil || policy.Revision != 1 || policy.Invite != "air_admin_primary" ||
		policy.Overlay != "primary_companion" || policy.Queue != "primary_companion" ||
		policy.Replace != "air_admin_primary" {
		t.Fatalf("migrated policy=%+v err=%v", policy, err)
	}
	media := readyLifecycleMedia(t, st, source, now+1,
		now+int64((7*24*time.Hour)/time.Millisecond))
	params := resolvedTransmissionParams(source, media, now+5)
	params.Availability = []TransmissionTargetAvailability{
		fullTransmissionAvailability(source, params.AcceptedAt),
		fullTransmissionAvailability(peer, params.AcceptedAt),
	}
	created, err := st.CreateResolvedTransmission(params)
	if err != nil {
		t.Fatal(err)
	}
	transmission := created.Creation.Transmission
	if transmission.AirID != airID || transmission.AirPolicyRevision != 1 ||
		transmission.PlaybackDomainKind != PlaybackDomainApproach ||
		transmission.PlaybackDomainID != linkID || len(created.Creation.Targets) != 2 {
		t.Fatalf("migrated transmission=%+v targets=%+v", transmission, created.Creation.Targets)
	}
}

func TestAirPolicyAuthorizationSnapshotsAndNeverExpandsAcceptedWork(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	peer, err := st.CreateSelfServiceOrbit("Air policy peer")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	if _, err := st.CutoverLinksToAirs(1, now); err != nil {
		t.Fatal(err)
	}
	air, err := st.CreateAir(CreateAirParams{
		Title: "Policy Air", OwnerOrbitID: owner.OrbitID, CreatedAt: now + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ActivateAir(owner.OrbitID, air.ID, "none", now+2); err != nil {
		t.Fatal(err)
	}
	pending, err := st.AddPendingAirMember(air.ID, peer.OrbitID, "member", now+3)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ConfirmAirMember(pending.ID, pending.Revision, true, "none", now+4); err != nil {
		t.Fatal(err)
	}
	companion := addTransmissionInstallation(t, st, owner, "companion")
	telegramIdentity := Identity{Kind: IdentityBearer, Token: companion.ControlToken}
	queueAuthorization, err := st.AuthorizeAirActionForIdentity(telegramIdentity, AirPolicyQueue)
	if err != nil || queueAuthorization.AirID != air.ID || queueAuthorization.PolicyRevision != 1 {
		t.Fatalf("queue authorization=%+v err=%v", queueAuthorization, err)
	}
	if _, err := st.AuthorizeAirActionForIdentity(telegramIdentity, AirPolicyReplace); !errors.Is(err, ErrAirPolicyDenied) {
		t.Fatalf("companion replace authorization=%v", err)
	}
	if _, err := st.AuthorizeInstallationAirAction(
		owner.OrbitID, owner.Slot, AirPolicyReplace,
	); err != nil {
		t.Fatalf("owner installation replace authorization=%v", err)
	}
	telegramLink, err := st.IssueTelegramLink(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	const telegramUserID = int64(99125862)
	if _, err := st.ConsumeTelegramLink(
		telegramUserID, "Air policy Telegram", "private", telegramLink.Code,
	); err != nil {
		t.Fatal(err)
	}
	telegramProof := Identity{Kind: IdentityTelegram, TelegramUserID: telegramUserID}
	if authorization, err := st.AuthorizeAirActionForIdentity(telegramProof, AirPolicyQueue); err != nil ||
		authorization.AirID != air.ID {
		t.Fatalf("Telegram queue authorization=%+v err=%v", authorization, err)
	}
	if _, err := st.AuthorizeAirActionForIdentity(telegramProof, AirPolicyReplace); !errors.Is(err, ErrAirPolicyDenied) {
		t.Fatalf("Telegram replace authorization=%v", err)
	}
	media := readyLifecycleMedia(t, st, companion, now+10,
		now+int64((7*24*time.Hour)/time.Millisecond))
	params := resolvedTransmissionParams(companion, media, now+20)
	params.Availability = []TransmissionTargetAvailability{
		fullTransmissionAvailability(owner, params.AcceptedAt),
		fullTransmissionAvailability(companion, params.AcceptedAt),
		fullTransmissionAvailability(peer, params.AcceptedAt),
	}
	accepted, err := st.CreateResolvedTransmission(params)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := accepted.Creation.Transmission
	if snapshot.AirID != air.ID || snapshot.AirPolicyRevision != 1 ||
		snapshot.AirPolicyOperation != AirPolicyOverlay || snapshot.AirPolicyResult != "allowed" ||
		snapshot.PlaybackDomainKind != PlaybackDomainApproach || len(accepted.Creation.Targets) != 3 {
		t.Fatalf("policy snapshot=%+v targets=%+v", snapshot, accepted.Creation.Targets)
	}
	queuedParams := params
	queuedParams.IdempotencyKeyHash = strings.Repeat("c", 64)
	queuedParams.RequestHash = strings.Repeat("d", 64)
	queuedParams.RequestedDelivery = TransmissionDeliveryAfterCurrent
	queuedParams.AcceptedAt++
	for i := range queuedParams.Availability {
		queuedParams.Availability[i].LastSeenAt = queuedParams.AcceptedAt
	}
	queued, err := st.CreateResolvedTransmission(queuedParams)
	if err != nil || queued.Creation.Transmission.AirPolicyOperation != AirPolicyQueue ||
		queued.Creation.Transmission.AirPolicyRevision != 1 {
		t.Fatalf("queue snapshot=%+v err=%v", queued, err)
	}
	if _, err := st.AuthorizedSetDND(AuthorizedDNDMutationParams{
		ExpectedActorID: peer.ActorID, Bearer: peer.ControlToken, Layer: "local",
		Mode: DNDMutedUntil, MutedUntil: now + 60_000, ExpectedRevision: 0,
		IdempotencyKeyHash: strings.Repeat("e", 64), RequestHash: strings.Repeat("f", 64),
		UpdatedAt: now + 22,
	}); err != nil {
		t.Fatal(err)
	}
	dndParams := params
	dndParams.IdempotencyKeyHash = strings.Repeat("3", 64)
	dndParams.RequestHash = strings.Repeat("4", 64)
	dndParams.AcceptedAt = now + 23
	for i := range dndParams.Availability {
		dndParams.Availability[i].LastSeenAt = dndParams.AcceptedAt
	}
	dndAccepted, err := st.CreateResolvedTransmission(dndParams)
	if err != nil {
		t.Fatal(err)
	}
	var peerStatus TransmissionTargetStatus
	for _, target := range dndAccepted.Creation.Targets {
		if target.ActorID == peer.ActorID {
			peerStatus = target.Status
		}
	}
	if peerStatus != TransmissionTargetMissedDND {
		t.Fatalf("local DND did not override allowed Air policy: targets=%+v", dndAccepted.Creation.Targets)
	}

	policy, err := st.AirPolicy(air.ID)
	if err != nil {
		t.Fatal(err)
	}
	policy.Overlay = "disabled"
	if err := st.ReplaceAirPolicy(*policy, policy.Revision, now+24); err != nil {
		t.Fatal(err)
	}
	var oldPolicyAudit, newPolicyAudit string
	if err := st.db.QueryRow(`SELECT old_value, new_value FROM air_audit_events
WHERE air_id = ? AND operation = 'air.policy.replace' ORDER BY id DESC LIMIT 1`, air.ID).
		Scan(&oldPolicyAudit, &newPolicyAudit); err != nil ||
		!strings.Contains(oldPolicyAudit, `"overlay":"primary_companion"`) ||
		!strings.Contains(newPolicyAudit, `"overlay":"disabled"`) {
		t.Fatalf("policy audit old=%q new=%q err=%v", oldPolicyAudit, newPolicyAudit, err)
	}
	denied := params
	denied.IdempotencyKeyHash = strings.Repeat("a", 64)
	denied.RequestHash = strings.Repeat("b", 64)
	denied.AcceptedAt++
	for i := range denied.Availability {
		denied.Availability[i].LastSeenAt = denied.AcceptedAt
	}
	if _, err := st.CreateResolvedTransmission(denied); !errors.Is(err, ErrAirPolicyDenied) {
		t.Fatalf("restricted companion create error=%v", err)
	}

	// An exact retry remains the immutable accepted result even after policy
	// restriction; no member or target is removed, added or reauthorized.
	replay := params
	replay.AcceptedAt += 1000
	replayed, err := st.CreateResolvedTransmission(replay)
	if err != nil || !replayed.Reused || replayed.Creation.Transmission.ID != snapshot.ID ||
		replayed.Creation.Transmission.AirPolicyRevision != 1 ||
		len(replayed.Creation.Targets) != len(accepted.Creation.Targets) {
		t.Fatalf("accepted replay=%+v err=%v", replayed, err)
	}
	if _, err := st.db.Exec(`UPDATE transmissions SET air_policy_revision = 2 WHERE id = ?`, snapshot.ID); err == nil ||
		!strings.Contains(err.Error(), "acceptance snapshot is immutable") {
		t.Fatalf("policy snapshot mutation error=%v", err)
	}
	var audits int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM air_audit_events
WHERE air_id = ? AND actor_id = ? AND operation = 'air.policy.authorize.overlay'
  AND new_value = ? AND result_code = 'ok'`, air.ID, companion.ActorID, snapshot.ID).
		Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("authorization audits=%d err=%v", audits, err)
	}
	var databaseSequence int
	var databaseName, databasePath string
	if err := st.db.QueryRow(`PRAGMA database_list`).Scan(
		&databaseSequence, &databaseName, &databasePath,
	); err != nil || databasePath == "" {
		t.Fatalf("database path=%q err=%v", databasePath, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := OpenWithOptions(databasePath, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restarted.Close() })
	restored, err := restarted.GetTransmission(snapshot.ID)
	if err != nil || restored == nil || restored.AirID != air.ID ||
		restored.AirPolicyRevision != 1 || restored.AirPolicyOperation != AirPolicyOverlay {
		t.Fatalf("restart snapshot=%+v err=%v", restored, err)
	}
	if authorization, err := restarted.AuthorizeAirActionForIdentity(
		Identity{Kind: IdentityBearer, Token: companion.ControlToken}, AirPolicyQueue,
	); err != nil || authorization.PolicyRevision != 2 {
		t.Fatalf("restart policy authorization=%+v err=%v", authorization, err)
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
