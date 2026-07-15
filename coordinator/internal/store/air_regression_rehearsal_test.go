package store

import (
	"testing"
	"time"
)

type airSchedulerFixture struct {
	store        *Store
	source, peer OnboardingCredentials
	airID        string
	media        MediaItem
	now          int64
}

func newAirSchedulerFixture(t *testing.T) airSchedulerFixture {
	t.Helper()
	st, source := newMediaIngestTestStore(t)
	peer, err := st.CreateSelfServiceOrbit("Air scheduler peer")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	if _, err := st.CutoverLinksToAirs(1, now); err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateAuthorizedAir(
		airTestAuth(source, "air-scheduler-create-0001", `{"title":"Scheduler Air"}`, now+1),
		"Scheduler Air",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ActivateAuthorizedAir(
		airTestAuth(source, "air-scheduler-activate-0001", `{"membership_revision":1}`, now+2),
		created.AirID, created.MembershipRevision, "none",
	); err != nil {
		t.Fatal(err)
	}
	invite, err := st.IssueAuthorizedAirInvite(
		airTestAuth(source, "air-scheduler-invite-0001", `{"air_role":"member"}`, now+3),
		created.AirID, "member",
	)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := st.ConsumeAuthorizedAirInvite(
		airTestAuth(peer, "air-scheduler-consume-0001", `{"code":"redacted"}`, now+4),
		invite.Code,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ConfirmAuthorizedAirJoin(
		airTestAuth(peer, "air-scheduler-confirm-0001", `{"membership_revision":1}`, now+5),
		created.AirID, preview.MembershipRevision, true, "none",
	); err != nil {
		t.Fatal(err)
	}
	media := readyLifecycleMedia(t, st, source, now+6,
		now+int64((7*24*time.Hour)/time.Millisecond))
	return airSchedulerFixture{store: st, source: source, peer: peer, airID: created.AirID, media: media, now: now}
}

func (f airSchedulerFixture) createOverlay(t *testing.T, suffix string) ResolvedTransmissionCreation {
	t.Helper()
	params := resolvedTransmissionParams(f.source, f.media, f.now+10)
	params.IdempotencyKeyHash = telegramRoutingDigest("air-rehearsal", suffix, "key")
	params.RequestHash = telegramRoutingDigest("air-rehearsal", suffix, "request")
	params.Availability = []TransmissionTargetAvailability{
		fullTransmissionAvailability(f.source, params.AcceptedAt),
		fullTransmissionAvailability(f.peer, params.AcceptedAt),
	}
	created, err := f.store.CreateResolvedTransmission(params)
	if err != nil {
		t.Fatal(err)
	}
	if created.Creation.Transmission.AirID != f.airID || len(created.Creation.Targets) != 2 {
		t.Fatalf("Air overlay=%+v", created)
	}
	return created
}

func (f airSchedulerFixture) leavePeer(t *testing.T, suffix string, now int64) {
	t.Helper()
	projection, err := f.store.AuthorizedAir(f.peer.ActorID, f.peer.ControlToken, f.airID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.LeaveAuthorizedAir(
		airTestAuth(f.peer, "air-scheduler-leave-"+suffix, `{"leave":true}`, now),
		f.airID, projection.MembershipRevision, f.airID,
	); err != nil {
		t.Fatal(err)
	}
}

func TestAirOverlayPrepareAndPlaybackCancelOnlyLeavingBarycenter(t *testing.T) {
	t.Run("prepare", func(t *testing.T) {
		fixture := newAirSchedulerFixture(t)
		created := fixture.createOverlay(t, "prepare")
		at := fixture.now + 20
		runtime := []TransmissionRuntimeTarget{
			schedulerRuntime(fixture.source, at, 10), schedulerRuntime(fixture.peer, at, 10),
		}
		opened, err := fixture.store.OpenTransmissionBarrier(created.Creation.Transmission.ID, at, runtime)
		if err != nil || len(opened.PrepareTargets) != 2 {
			t.Fatalf("open=%+v err=%v", opened, err)
		}
		fixture.leavePeer(t, "prepare-0001", at+1)
		rechecked, err := fixture.store.RecheckTransmissionRuntime(
			created.Creation.Transmission.ID, at+2, runtime,
		)
		if err != nil || !rechecked.Changed || len(rechecked.DisarmTargets) != 1 {
			t.Fatalf("recheck=%+v err=%v", rechecked, err)
		}
		left := rechecked.DisarmTargets[0]
		if left.OrbitID != fixture.peer.OrbitID || left.Status != TransmissionTargetCancelled ||
			left.ReasonCode != TransmissionReasonApproachLeft {
			t.Fatalf("leaving target=%+v", left)
		}
		for _, target := range rechecked.Work.Targets {
			if target.OrbitID == fixture.source.OrbitID && target.Status != TransmissionTargetPreparing {
				t.Fatalf("remaining target=%+v", target)
			}
		}
	})

	t.Run("playback", func(t *testing.T) {
		fixture := newAirSchedulerFixture(t)
		created := fixture.createOverlay(t, "playback")
		at := fixture.now + 20
		runtime := []TransmissionRuntimeTarget{
			schedulerRuntime(fixture.source, at, 10), schedulerRuntime(fixture.peer, at, 10),
		}
		opened, err := fixture.store.OpenTransmissionBarrier(created.Creation.Transmission.ID, at, runtime)
		if err != nil {
			t.Fatal(err)
		}
		for _, target := range opened.PrepareTargets {
			schedulerTransition(t, fixture.store, target, TransmissionTargetReady, "", at+1)
		}
		runtime[0].LastSeenAt, runtime[0].RTTSampledAt = at+2, at+2
		runtime[1].LastSeenAt, runtime[1].RTTSampledAt = at+2, at+2
		decision, err := fixture.store.DecideTransmissionBarrier(created.Creation.Transmission.ID, at+2, runtime)
		if err != nil || len(decision.ScheduledTargets) != 2 {
			t.Fatalf("decision=%+v err=%v", decision, err)
		}
		for _, target := range decision.ScheduledTargets {
			schedulerTransition(t, fixture.store, target, TransmissionTargetPlaying, "", decision.Work.Scheduler.TCoordMS)
		}
		fixture.leavePeer(t, "playback-0001", decision.Work.Scheduler.TCoordMS+1)
		rechecked, err := fixture.store.RecheckTransmissionRuntime(
			created.Creation.Transmission.ID, decision.Work.Scheduler.TCoordMS+2, runtime,
		)
		if err != nil || len(rechecked.DisarmTargets) != 1 {
			t.Fatalf("recheck=%+v err=%v", rechecked, err)
		}
		left := rechecked.DisarmTargets[0]
		if left.OrbitID != fixture.peer.OrbitID || left.Status != TransmissionTargetCancelling ||
			left.ReasonCode != TransmissionReasonApproachLeft {
			t.Fatalf("playing leaver=%+v", left)
		}
		for _, target := range rechecked.Work.Targets {
			if target.OrbitID == fixture.source.OrbitID && target.Status != TransmissionTargetPlaying {
				t.Fatalf("remaining playback=%+v", target)
			}
		}
	})
}
