package store

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func addConfirmedParityAirMember(
	t *testing.T,
	st *Store,
	airID string,
	member OnboardingCredentials,
	now int64,
) {
	t.Helper()
	pending, err := st.AddPendingAirMember(airID, member.OrbitID, "member", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ConfirmAirMember(pending.ID, pending.Revision, true, "none", now+1); err != nil {
		t.Fatal(err)
	}
}

func TestTargetsInboxB5ExplicitACLAndFrozenAudienceRegression(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	nonTarget, err := st.CreateSelfServiceOrbit("B5 non-target")
	if err != nil {
		t.Fatal(err)
	}
	target, err := st.CreateSelfServiceOrbit("B5 exact target")
	if err != nil {
		t.Fatal(err)
	}
	laterMember, err := st.CreateSelfServiceOrbit("B5 later member")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	if _, err := st.CutoverLinksToAirs(1, now); err != nil {
		t.Fatal(err)
	}
	air, err := st.CreateAir(CreateAirParams{
		Title: "B5 frozen Air", OwnerOrbitID: source.OrbitID, CreatedAt: now + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ActivateAir(source.OrbitID, air.ID, "none", now+2); err != nil {
		t.Fatal(err)
	}
	addConfirmedParityAirMember(t, st, air.ID, nonTarget, now+3)
	addConfirmedParityAirMember(t, st, air.ID, target, now+5)

	media := readyLifecycleMedia(t, st, source, now+10,
		now+10+int64((45*24*time.Hour)/time.Millisecond))
	targetReference := issueTargetReference(t, st, source,
		TransmissionSelectorPulsar, target.OrbitID, target.Slot, now+11)
	params := resolvedTransmissionParams(source, media, now+12)
	params.AudienceKind = TransmissionAudienceExplicit
	params.IncludeOrigin = false
	params.Selectors = []TransmissionAudienceSelector{{Reference: targetReference}}
	params.Availability = []TransmissionTargetAvailability{
		fullTransmissionAvailability(source, params.AcceptedAt),
		fullTransmissionAvailability(nonTarget, params.AcceptedAt),
		fullTransmissionAvailability(target, params.AcceptedAt),
	}
	created, err := st.CreateResolvedTransmission(params)
	if err != nil || len(created.Creation.Targets) != 1 ||
		created.Creation.Targets[0].ActorID != target.ActorID {
		t.Fatalf("explicit B5 create=%+v err=%v", created, err)
	}
	if _, err := st.GetAuthorizedTransmission(nonTarget.ActorID, nonTarget.NodeToken,
		created.Creation.Transmission.ID); !errors.Is(err, ErrTransmissionNotFound) {
		t.Fatalf("non-target learned transmission by known ID: %v", err)
	}
	for _, check := range []struct {
		credentials OnboardingCredentials
		want        bool
	}{
		{target, true},
		{nonTarget, false},
		{source, false},
	} {
		allowed, err := st.AllowsMediaDownload(context.Background(), MediaTargetIdentity{
			MediaID: media.ID, OrbitID: check.credentials.OrbitID,
			ActorID: check.credentials.ActorID, Slot: check.credentials.Slot,
		})
		if err != nil || allowed != check.want {
			t.Fatalf("media ACL actor=%d allowed=%v want=%v err=%v",
				check.credentials.ActorID, allowed, check.want, err)
		}
	}
	nonTargetInbox, err := st.QueryAuthorizedTransmissionInbox(
		nonTarget.ActorID, bearerIdentity(nonTarget), "all", 20, "", now+13,
	)
	if err != nil || len(nonTargetInbox.Items) != 0 {
		t.Fatalf("non-target inbox=%+v err=%v", nonTargetInbox, err)
	}

	transitionTargetToInboxReceipt(t, st, created.Creation,
		TransmissionTargetMissedOffline, TransmissionReasonOfflineBeforeStart, now+14)
	beforeRead, err := st.GetTransmission(created.Creation.Transmission.ID)
	if err != nil || beforeRead == nil {
		t.Fatal(err)
	}
	targetInbox, err := st.QueryAuthorizedTransmissionInbox(
		target.ActorID, bearerIdentity(target), "all", 20, "", now+15,
	)
	if err != nil || len(targetInbox.Items) != 1 {
		t.Fatalf("exact target inbox=%+v err=%v", targetInbox, err)
	}
	afterRead, err := st.GetTransmission(created.Creation.Transmission.ID)
	if err != nil || afterRead == nil || afterRead.Revision != beforeRead.Revision {
		t.Fatalf("inbox read mutated transmission before=%+v after=%+v err=%v",
			beforeRead, afterRead, err)
	}
	expiredPage, err := st.QueryAuthorizedTransmissionInbox(
		target.ActorID, bearerIdentity(target), "all", 20, "",
		now+15+int64((31*24*time.Hour)/time.Millisecond),
	)
	if err != nil || len(expiredPage.Items) != 1 ||
		expiredPage.Items[0].Item.Availability != TransmissionInboxExpired ||
		expiredPage.Items[0].CanReplay {
		t.Fatalf("expired inbox retained replay authority=%+v err=%v", expiredPage, err)
	}

	// Joining after acceptance cannot expand the immutable target snapshot,
	// even when the new member knows both transmission and media IDs.
	addConfirmedParityAirMember(t, st, air.ID, laterMember, now+16)
	if _, err := st.GetAuthorizedTransmission(laterMember.ActorID, laterMember.NodeToken,
		created.Creation.Transmission.ID); !errors.Is(err, ErrTransmissionNotFound) {
		t.Fatalf("later member learned old transmission: %v", err)
	}
	allowed, err := st.AllowsMediaDownload(context.Background(), MediaTargetIdentity{
		MediaID: media.ID, OrbitID: laterMember.OrbitID,
		ActorID: laterMember.ActorID, Slot: laterMember.Slot,
	})
	if err != nil || allowed {
		t.Fatalf("later member inherited old media allowed=%v err=%v", allowed, err)
	}

	// A separate N-recipient create deduplicates an orbit and installation
	// selector for the same target, excludes the source and never falls back to
	// the newly joined member or the whole Air.
	bReference := issueTargetReference(t, st, source,
		TransmissionSelectorBarycenter, nonTarget.OrbitID, "", now+17)
	cOrbitReference := issueTargetReference(t, st, source,
		TransmissionSelectorBarycenter, target.OrbitID, "", now+17)
	cPulsarReference := issueTargetReference(t, st, source,
		TransmissionSelectorPulsar, target.OrbitID, target.Slot, now+17)
	nTargets := params
	nTargets.IdempotencyKeyHash = strings.Repeat("c", 64)
	nTargets.RequestHash = strings.Repeat("d", 64)
	nTargets.AcceptedAt = now + 18
	nTargets.Selectors = []TransmissionAudienceSelector{
		{Reference: cPulsarReference},
		{Reference: bReference},
		{Reference: cOrbitReference},
	}
	nTargets.Availability = []TransmissionTargetAvailability{
		fullTransmissionAvailability(source, nTargets.AcceptedAt),
		fullTransmissionAvailability(nonTarget, nTargets.AcceptedAt),
		fullTransmissionAvailability(target, nTargets.AcceptedAt),
		fullTransmissionAvailability(laterMember, nTargets.AcceptedAt),
	}
	nCreated, err := st.CreateResolvedTransmission(nTargets)
	if err != nil || len(nCreated.Creation.Targets) != 2 {
		t.Fatalf("N-recipient create=%+v err=%v", nCreated, err)
	}
	actors := map[int64]int{}
	for _, snapshot := range nCreated.Creation.Targets {
		actors[snapshot.ActorID]++
	}
	if actors[nonTarget.ActorID] != 1 || actors[target.ActorID] != 1 ||
		actors[source.ActorID] != 0 || actors[laterMember.ActorID] != 0 || len(actors) != 2 {
		t.Fatalf("N-recipient snapshot expanded or duplicated: %v", actors)
	}
}
