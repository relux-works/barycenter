package store

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/e2eecontract"
)

const e2eeRoutingSuite = "AUDIT_FIXTURE_SUITE_NOT_FOR_PRODUCTION"

type e2eeRoutingFixture struct {
	store                   *Store
	path                    string
	owner, peer             OnboardingCredentials
	ownerDevice, peerDevice string
	ownerActor, peerActor   string
	airID, peerMemberID     string
	group                   E2EEGroup
	now                     int64
}

func e2eeRoutingConfig() e2eecontract.Config {
	return e2eecontract.Config{
		AllowedSuites: map[string]struct{}{e2eeRoutingSuite: {}},
		Verifier: e2eecontract.VerifierFunc(func(digest, signature string) bool {
			return signature == "fixture-signature:"+digest
		}),
	}
}

func registerE2EERoutingDevice(t *testing.T, st *Store, credentials OnboardingCredentials,
	deviceID, protocolActorID string, now int64,
) {
	t.Helper()
	payload := []byte("public-routing-package:" + deviceID)
	if _, err := st.RegisterE2EEPublicDevice(RegisterE2EEPublicDeviceParams{
		DeviceID: deviceID, ProtocolActorID: protocolActorID,
		ActorID: credentials.ActorID, PublicPackage: payload,
		PublicPackageDigest: e2eeDigest(payload), VerificationState: "verified",
		VerificationDigest: strings.Repeat("d", 64), CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
}

func newE2EERoutingFixture(t *testing.T) e2eeRoutingFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "e2ee-routing.db")
	st, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	owner, err := st.CreateSelfServiceOrbit("E2EE routing owner")
	if err != nil {
		t.Fatal(err)
	}
	peer, err := st.CreateSelfServiceOrbit("E2EE routing peer")
	if err != nil {
		t.Fatal(err)
	}
	authority, err := st.AirAuthority()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CutoverLinksToAirs(authority.Generation, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli() + 1000
	air, err := st.CreateAir(CreateAirParams{Title: "E2EE routed Air",
		OwnerOrbitID: owner.OrbitID, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	peerMember, err := st.AddPendingAirMember(air.ID, peer.OrbitID, "member", now+1)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ConfirmAirMember(peerMember.ID, peerMember.Revision, false, "none", now+2); err != nil {
		t.Fatal(err)
	}
	ownerDevice, peerDevice := "routing_owner_device_0001", "routing_peer_device_00001"
	ownerActor, peerActor := "actor_routing_owner_0001", "actor_routing_peer_00001"
	registerE2EERoutingDevice(t, st, owner, ownerDevice, ownerActor, now+3)
	registerE2EERoutingDevice(t, st, peer, peerDevice, peerActor, now+4)
	snapshot, err := st.E2EEAirSnapshot(air.ID)
	if err != nil || len(snapshot.UnsupportedActorIDs) != 0 || len(snapshot.Members) != 2 {
		t.Fatalf("initial snapshot=%+v err=%v", snapshot, err)
	}
	group, err := st.CreateE2EEGroup(CreateE2EEGroupParams{
		AirID: air.ID, AuthorDeviceID: ownerDevice,
		TargetSnapshotDigest: snapshot.Digest, CommitDigest: strings.Repeat("b", 64),
		Epoch: 7, CreatedAt: now + 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.InitializeE2EEGroupRouting(group.ID, ownerDevice, now+6); err != nil {
		t.Fatal(err)
	}
	return e2eeRoutingFixture{store: st, path: path, owner: owner, peer: peer,
		ownerDevice: ownerDevice, peerDevice: peerDevice, ownerActor: ownerActor,
		peerActor: peerActor, airID: air.ID, peerMemberID: peerMember.ID,
		group: group, now: now}
}

func e2eeProposalRaw(t *testing.T, f e2eeRoutingFixture, eventID,
	targetDigest string,
) []byte {
	t.Helper()
	group, err := f.store.GetE2EEGroup(f.group.ID)
	if err != nil {
		t.Fatal(err)
	}
	authDigest := e2eeDigest([]byte("proposal-auth:" + eventID))
	value := e2eecontract.Proposal{
		Contract: e2eecontract.Contract, Capability: e2eecontract.Capability,
		Suite: e2eeRoutingSuite, EventID: eventID, GroupID: group.ID,
		ActorID: f.ownerActor, DeviceID: f.ownerDevice, AirID: f.airID,
		PreviousEpoch: uint64(group.CurrentEpoch), Epoch: uint64(group.CurrentEpoch + 1),
		TargetSnapshotDigest:    targetDigest,
		ProposalDigest:          e2eeDigest([]byte("proposal:" + eventID)),
		AuthenticatedDataDigest: authDigest, Signature: "fixture-signature:" + authDigest,
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func e2eeCommitRaw(t *testing.T, f e2eeRoutingFixture, eventID,
	targetDigest, commitDigest string,
) []byte {
	t.Helper()
	group, err := f.store.GetE2EEGroup(f.group.ID)
	if err != nil {
		t.Fatal(err)
	}
	authDigest := e2eeDigest([]byte("commit-auth:" + eventID))
	value := e2eecontract.Commit{
		Contract: e2eecontract.Contract, Capability: e2eecontract.Capability,
		Suite: e2eeRoutingSuite, EventID: eventID, GroupID: group.ID,
		ActorID: f.ownerActor, DeviceID: f.ownerDevice, AirID: f.airID,
		PreviousEpoch: uint64(group.CurrentEpoch), Epoch: uint64(group.CurrentEpoch + 1),
		PreviousCommitDigest: group.CommitDigest, CommitDigest: commitDigest,
		TargetSnapshotDigest: targetDigest, AuthenticatedDataDigest: authDigest,
		Signature: "fixture-signature:" + authDigest,
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestE2EERoutedProposalDeliveryAckSurvivesRestart(t *testing.T) {
	f := newE2EERoutingFixture(t)
	raw := e2eeProposalRaw(t, f, "proposal_delivery_restart_0001",
		f.group.TargetSnapshotDigest)
	if _, err := f.store.RouteE2EEProposal(e2eecontract.ProductionConfig(), raw,
		f.now+9); e2eecontract.Code(err) != e2eecontract.ErrUnknownSuite {
		t.Fatalf("production-dark proposal error=%v", err)
	}
	var enabled int
	if err := f.store.db.QueryRow(`SELECT enabled FROM e2ee_feature_state
WHERE singleton = 1`).Scan(&enabled); err != nil || enabled != 0 {
		t.Fatalf("feature state enabled=%d err=%v", enabled, err)
	}
	routed, err := f.store.RouteE2EEProposal(e2eeRoutingConfig(), raw, f.now+10)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(routed.Recipients)
	if len(routed.Recipients) != 2 || routed.Recipients[0] != f.ownerDevice ||
		routed.Recipients[1] != f.peerDevice {
		t.Fatalf("proposal recipients=%v", routed.Recipients)
	}
	pending, err := f.store.PendingE2EEGroupEvents(f.peerDevice, E2EEGroupEventCursor{}, 10)
	if err != nil || len(pending) != 1 || string(pending[0].PublicPayload) != string(raw) {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithOptions(f.path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	f.store = reopened
	t.Cleanup(func() { _ = reopened.Close() })
	pending, err = reopened.PendingE2EEGroupEvents(f.peerDevice, E2EEGroupEventCursor{}, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("restart pending=%+v err=%v", pending, err)
	}
	ack, err := reopened.AcknowledgeE2EEGroupEvent(f.peerDevice, pending[0].EventID,
		pending[0].EventDigest, pending[0].Revision, f.now+11)
	if err != nil || ack.State != "acknowledged" {
		t.Fatalf("ack=%+v err=%v", ack, err)
	}
	if pending, err = reopened.PendingE2EEGroupEvents(f.peerDevice, E2EEGroupEventCursor{}, 10); err != nil || len(pending) != 0 {
		t.Fatalf("post-ack pending=%+v err=%v", pending, err)
	}
	if _, err := reopened.AcknowledgeE2EEGroupEvent(f.peerDevice, ack.EventID,
		ack.EventDigest, ack.Revision-1, f.now+12); !errors.Is(err, ErrE2EEDeliveryNotFound) {
		t.Fatalf("duplicate acknowledgement error=%v", err)
	}
	if _, err := reopened.RouteE2EEProposal(e2eeRoutingConfig(), raw, f.now+13); !errors.Is(err, ErrE2EEReplay) {
		t.Fatalf("proposal replay error=%v", err)
	}
	var rejected int
	if err := reopened.db.QueryRow(`SELECT COUNT(*) FROM e2ee_audit_events
WHERE group_id = ? AND operation = 'proposal.route' AND outcome = 'rejected'
  AND reason_code = 'replay'`, f.group.ID).Scan(&rejected); err != nil || rejected != 1 {
		t.Fatalf("replay audit=%d err=%v", rejected, err)
	}
}

func TestE2EEProtocolActorBindingIsStableAndOneToOne(t *testing.T) {
	f := newE2EERoutingFixture(t)
	if _, err := f.store.db.Exec(`UPDATE e2ee_protocol_actor_bindings
SET protocol_actor_id = ? WHERE device_id = ?`, "actor_rebound_forbidden_01",
		f.ownerDevice); err == nil {
		t.Fatal("protocol actor binding update unexpectedly succeeded")
	}
	payload := []byte("public-routing-package:conflicting-owner-device")
	if _, err := f.store.RegisterE2EEPublicDevice(RegisterE2EEPublicDeviceParams{
		DeviceID: "routing_owner_device_conflict", ProtocolActorID: "actor_conflicting_owner_01",
		ActorID: f.owner.ActorID, PublicPackage: payload,
		PublicPackageDigest: e2eeDigest(payload), VerificationState: "verified",
		VerificationDigest: strings.Repeat("d", 64), CreatedAt: f.now + 14,
	}); !errors.Is(err, ErrE2EEConflict) {
		t.Fatalf("same actor conflicting protocol ID error=%v", err)
	}
	payload = []byte("public-routing-package:conflicting-peer-device")
	if _, err := f.store.RegisterE2EEPublicDevice(RegisterE2EEPublicDeviceParams{
		DeviceID: "routing_peer_device_conflict1", ProtocolActorID: f.ownerActor,
		ActorID: f.peer.ActorID, PublicPackage: payload,
		PublicPackageDigest: e2eeDigest(payload), VerificationState: "verified",
		VerificationDigest: strings.Repeat("d", 64), CreatedAt: f.now + 15,
	}); !errors.Is(err, ErrE2EEConflict) {
		t.Fatalf("shared protocol actor ID error=%v", err)
	}
}

func TestE2EEDeliveryCursorDoesNotSkipSameTimestamp(t *testing.T) {
	f := newE2EERoutingFixture(t)
	for _, eventID := range []string{
		"proposal_same_stamp_alpha_01", "proposal_same_stamp_bravo_01",
	} {
		if _, err := f.store.RouteE2EEProposal(e2eeRoutingConfig(),
			e2eeProposalRaw(t, f, eventID, f.group.TargetSnapshotDigest),
			f.now+18); err != nil {
			t.Fatal(err)
		}
	}
	first, err := f.store.PendingE2EEGroupEvents(f.peerDevice,
		E2EEGroupEventCursor{}, 1)
	if err != nil || len(first) != 1 {
		t.Fatalf("first cursor page=%+v err=%v", first, err)
	}
	second, err := f.store.PendingE2EEGroupEvents(f.peerDevice,
		E2EEGroupEventCursor{CreatedAt: first[0].CreatedAt, EventID: first[0].EventID}, 1)
	if err != nil || len(second) != 1 || second[0].EventID == first[0].EventID {
		t.Fatalf("second cursor page=%+v err=%v", second, err)
	}
}

func TestE2EERotationOnJoinLeaveDeviceRevokeAndActorDisable(t *testing.T) {
	t.Run("join", func(t *testing.T) {
		f := newE2EERoutingFixture(t)
		joined, err := f.store.CreateSelfServiceOrbit("E2EE joined")
		if err != nil {
			t.Fatal(err)
		}
		registerE2EERoutingDevice(t, f.store, joined, "routing_joined_device_001",
			"actor_routing_joined_001", f.now+20)
		member, err := f.store.AddPendingAirMember(f.airID, joined.OrbitID, "member", f.now+21)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.store.ConfirmAirMember(member.ID, member.Revision, false, "none", f.now+22); err != nil {
			t.Fatal(err)
		}
		requirement, err := f.store.ReconcileE2EERotation(f.group.ID, f.now+23)
		if err != nil || requirement == nil || requirement.State != "required" ||
			requirement.ReasonCode != "air_join" {
			t.Fatalf("join requirement=%+v err=%v", requirement, err)
		}
		objectParams := e2eeObjectParams(e2eeStoreFixture{group: f.group},
			"routing_blocked_join_001", f.now+24)
		objectParams.AuthorDeviceID = f.ownerDevice
		if _, err := f.store.StageE2EEProtectedObject(objectParams); !errors.Is(err, ErrE2EERotationRequired) {
			t.Fatalf("unrotated stage error=%v", err)
		}
		snapshot, err := f.store.E2EEAirSnapshot(f.airID)
		if err != nil {
			t.Fatal(err)
		}
		routed, err := f.store.RouteE2EECommit(e2eeRoutingConfig(), e2eeCommitRaw(t, f,
			"commit_after_join_00000001", snapshot.Digest, strings.Repeat("1", 64)), f.now+25)
		if err != nil || len(routed.Recipients) != 3 {
			t.Fatalf("join commit=%+v err=%v", routed, err)
		}
		requirement, err = f.store.GetE2EERotationRequirement(f.group.ID)
		if err != nil || requirement.State != "satisfied" || requirement.SatisfiedEpoch != 8 {
			t.Fatalf("satisfied join=%+v err=%v", requirement, err)
		}
		var satisfactionAudit int
		if err := f.store.db.QueryRow(`SELECT COUNT(*) FROM e2ee_audit_events
WHERE group_id = ? AND operation = 'rotation.satisfy' AND outcome = 'accepted'`,
			f.group.ID).Scan(&satisfactionAudit); err != nil || satisfactionAudit != 1 {
			t.Fatalf("rotation satisfaction audit=%d err=%v", satisfactionAudit, err)
		}
		objectParams = e2eeObjectParams(e2eeStoreFixture{group: routed.Group},
			"routing_after_join_00001", f.now+26)
		objectParams.AuthorDeviceID = f.ownerDevice
		if _, err := f.store.StageE2EEProtectedObject(objectParams); err != nil {
			t.Fatalf("post-rotation stage error=%v", err)
		}
	})

	t.Run("leave", func(t *testing.T) {
		f := newE2EERoutingFixture(t)
		if err := f.store.LeaveAirMember(f.peerMemberID, 2, f.now+30); err != nil {
			t.Fatal(err)
		}
		requirement, err := f.store.ReconcileE2EERotation(f.group.ID, f.now+31)
		if err != nil || requirement.ReasonCode != "air_leave" {
			t.Fatalf("leave requirement=%+v err=%v", requirement, err)
		}
		snapshot, _ := f.store.E2EEAirSnapshot(f.airID)
		removedRaw := e2eeCommitRaw(t, f, "commit_removed_peer_0000001",
			snapshot.Digest, strings.Repeat("e", 64))
		var removedCommit e2eecontract.Commit
		if err := json.Unmarshal(removedRaw, &removedCommit); err != nil {
			t.Fatal(err)
		}
		removedCommit.ActorID, removedCommit.DeviceID = f.peerActor, f.peerDevice
		removedCommit.AuthenticatedDataDigest = e2eeDigest([]byte("removed-peer"))
		removedCommit.Signature = "fixture-signature:" + removedCommit.AuthenticatedDataDigest
		removedRaw, _ = json.Marshal(removedCommit)
		if _, err := f.store.RouteE2EECommit(e2eeRoutingConfig(), removedRaw,
			f.now+32); !errors.Is(err, ErrE2EEInvalid) {
			t.Fatalf("removed peer commit error=%v", err)
		}
		routed, err := f.store.RouteE2EECommit(e2eeRoutingConfig(), e2eeCommitRaw(t, f,
			"commit_after_leave_0000001", snapshot.Digest, strings.Repeat("2", 64)), f.now+33)
		if err != nil || len(routed.Recipients) != 1 || routed.Recipients[0] != f.ownerDevice {
			t.Fatalf("leave commit=%+v err=%v", routed, err)
		}
		if pending, err := f.store.PendingE2EEGroupEvents(f.peerDevice, E2EEGroupEventCursor{}, 10); err != nil || len(pending) != 0 {
			t.Fatalf("removed peer pending=%+v err=%v", pending, err)
		}
	})

	t.Run("device-revoke", func(t *testing.T) {
		f := newE2EERoutingFixture(t)
		device, err := f.store.GetE2EEPublicDevice(f.peerDevice)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.RevokeE2EEPublicDevice(f.peerDevice, device.Revision, f.now+40); err != nil {
			t.Fatal(err)
		}
		requirement, err := f.store.ReconcileE2EERotation(f.group.ID, f.now+41)
		if err != nil || requirement.ReasonCode != "device_revoke" {
			t.Fatalf("revoke requirement=%+v err=%v", requirement, err)
		}
		snapshot, _ := f.store.E2EEAirSnapshot(f.airID)
		routed, err := f.store.RouteE2EECommit(e2eeRoutingConfig(), e2eeCommitRaw(t, f,
			"commit_after_revoke_000001", snapshot.Digest, strings.Repeat("3", 64)), f.now+42)
		if err != nil || len(routed.Recipients) != 1 {
			t.Fatalf("revoke commit=%+v err=%v", routed, err)
		}
	})

	t.Run("actor-disable", func(t *testing.T) {
		f := newE2EERoutingFixture(t)
		if _, err := f.store.DisableActorForModeration(f.peer.ActorID, f.now+50); err != nil {
			t.Fatal(err)
		}
		requirement, err := f.store.ReconcileE2EERotation(f.group.ID, f.now+51)
		if err != nil || requirement.ReasonCode != "actor_disable" {
			t.Fatalf("disable requirement=%+v err=%v", requirement, err)
		}
		snapshot, _ := f.store.E2EEAirSnapshot(f.airID)
		routed, err := f.store.RouteE2EECommit(e2eeRoutingConfig(), e2eeCommitRaw(t, f,
			"commit_after_disable_00001", snapshot.Digest, strings.Repeat("4", 64)), f.now+52)
		if err != nil || len(routed.Recipients) != 1 {
			t.Fatalf("disable commit=%+v err=%v", routed, err)
		}
	})

	t.Run("same-device-leave-rejoin-lineage", func(t *testing.T) {
		f := newE2EERoutingFixture(t)
		if err := f.store.LeaveAirMember(f.peerMemberID, 2, f.now+55); err != nil {
			t.Fatal(err)
		}
		rejoined, err := f.store.AddPendingAirMember(f.airID, f.peer.OrbitID,
			"admin", f.now+56)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.store.ConfirmAirMember(rejoined.ID, rejoined.Revision, false,
			"none", f.now+57); err != nil {
			t.Fatal(err)
		}
		requirement, err := f.store.ReconcileE2EERotation(f.group.ID, f.now+58)
		if err != nil || requirement.ReasonCode != "membership_change" ||
			requirement.RequiredSnapshotDigest == f.group.TargetSnapshotDigest {
			t.Fatalf("rejoin requirement=%+v err=%v", requirement, err)
		}
		snapshot, err := f.store.E2EEAirSnapshot(f.airID)
		if err != nil {
			t.Fatal(err)
		}
		proposal, err := f.store.RouteE2EEProposal(e2eeRoutingConfig(),
			e2eeProposalRaw(t, f, "proposal_after_rejoin_00001", snapshot.Digest),
			f.now+59)
		if err != nil || len(proposal.Recipients) != 1 ||
			proposal.Recipients[0] != f.ownerDevice {
			t.Fatalf("rejoin proposal=%+v err=%v", proposal, err)
		}
		routed, err := f.store.RouteE2EECommit(e2eeRoutingConfig(), e2eeCommitRaw(t, f,
			"commit_after_rejoin_000001", snapshot.Digest, strings.Repeat("f", 64)), f.now+60)
		if err != nil || len(routed.Recipients) != 2 {
			t.Fatalf("rejoin commit=%+v err=%v", routed, err)
		}
	})
}

func TestE2EERoutingMixedVersionReplayForkAndConcurrentCommit(t *testing.T) {
	t.Run("mixed-version", func(t *testing.T) {
		f := newE2EERoutingFixture(t)
		unsupported, err := f.store.CreateSelfServiceOrbit("Unsupported E2EE target")
		if err != nil {
			t.Fatal(err)
		}
		member, err := f.store.AddPendingAirMember(f.airID, unsupported.OrbitID, "member", f.now+60)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.store.ConfirmAirMember(member.ID, member.Revision, false, "none", f.now+61); err != nil {
			t.Fatal(err)
		}
		requirement, err := f.store.ReconcileE2EERotation(f.group.ID, f.now+62)
		if err != nil || requirement.ReasonCode != "unsupported_client" {
			t.Fatalf("unsupported requirement=%+v err=%v", requirement, err)
		}
		snapshot, _ := f.store.E2EEAirSnapshot(f.airID)
		if _, err := f.store.RouteE2EECommit(e2eeRoutingConfig(), e2eeCommitRaw(t, f,
			"commit_mixed_version_00001", snapshot.Digest, strings.Repeat("5", 64)), f.now+63); !errors.Is(err, ErrE2EEUnsupportedTarget) {
			t.Fatalf("mixed-version commit error=%v", err)
		}
	})

	t.Run("concurrent-single-winner", func(t *testing.T) {
		f := newE2EERoutingFixture(t)
		rawA := e2eeCommitRaw(t, f, "commit_concurrent_alpha_001",
			f.group.TargetSnapshotDigest, strings.Repeat("6", 64))
		rawB := e2eeCommitRaw(t, f, "commit_concurrent_bravo_001",
			f.group.TargetSnapshotDigest, strings.Repeat("7", 64))
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for index, raw := range [][]byte{rawA, rawB} {
			wg.Add(1)
			go func(offset int, payload []byte) {
				defer wg.Done()
				_, err := f.store.RouteE2EECommit(e2eeRoutingConfig(), payload,
					f.now+70+int64(offset))
				errs <- err
			}(index, raw)
		}
		wg.Wait()
		close(errs)
		accepted, rejected := 0, 0
		for err := range errs {
			if err == nil {
				accepted++
			} else if e2eecontract.Code(err) == e2eecontract.ErrStaleEpoch ||
				e2eecontract.Code(err) == e2eecontract.ErrForkedEpoch ||
				errors.Is(err, ErrE2EEConflict) {
				rejected++
			} else {
				t.Fatalf("unexpected concurrent error=%v", err)
			}
		}
		if accepted != 1 || rejected != 1 {
			t.Fatalf("concurrent accepted=%d rejected=%d", accepted, rejected)
		}
		group, err := f.store.GetE2EEGroup(f.group.ID)
		if err != nil || group.CurrentEpoch != 8 || group.ForkState != "clean" {
			t.Fatalf("concurrent group=%+v err=%v", group, err)
		}
	})

	t.Run("malicious-fork-freezes", func(t *testing.T) {
		f := newE2EERoutingFixture(t)
		group, _ := f.store.GetE2EEGroup(f.group.ID)
		authDigest := strings.Repeat("8", 64)
		fork := e2eecontract.Commit{Contract: e2eecontract.Contract,
			Capability: e2eecontract.Capability, Suite: e2eeRoutingSuite,
			EventID: "commit_malicious_fork_0001", GroupID: group.ID,
			ActorID: f.ownerActor, DeviceID: f.ownerDevice, AirID: f.airID,
			PreviousEpoch: uint64(group.CurrentEpoch), Epoch: uint64(group.CurrentEpoch + 1),
			PreviousCommitDigest: strings.Repeat("9", 64), CommitDigest: strings.Repeat("a", 64),
			TargetSnapshotDigest:    group.TargetSnapshotDigest,
			AuthenticatedDataDigest: authDigest, Signature: "fixture-signature:" + authDigest}
		raw, _ := json.Marshal(fork)
		if _, err := f.store.RouteE2EECommit(e2eeRoutingConfig(), raw, f.now+80); e2eecontract.Code(err) != e2eecontract.ErrForkedEpoch {
			t.Fatalf("fork error=%v", err)
		}
		group, _ = f.store.GetE2EEGroup(group.ID)
		if group.ForkState != "forked" {
			t.Fatalf("fork state=%+v", group)
		}
		if _, err := f.store.RouteE2EECommit(e2eeRoutingConfig(), e2eeCommitRaw(t, f,
			"commit_after_fork_0000001", group.TargetSnapshotDigest,
			strings.Repeat("c", 64)), f.now+81); err == nil {
			t.Fatal("forked group accepted another commit")
		}
	})

	t.Run("malformed-predecessor-does-not-poison", func(t *testing.T) {
		f := newE2EERoutingFixture(t)
		group, _ := f.store.GetE2EEGroup(f.group.ID)
		authDigest := strings.Repeat("8", 64)
		malformed := e2eecontract.Commit{Contract: e2eecontract.Contract,
			Capability: e2eecontract.Capability, Suite: e2eeRoutingSuite,
			EventID: "commit_malformed_predecessor1", GroupID: group.ID,
			ActorID: f.ownerActor, DeviceID: f.ownerDevice, AirID: f.airID,
			PreviousEpoch: uint64(group.CurrentEpoch), Epoch: uint64(group.CurrentEpoch + 1),
			PreviousCommitDigest: "not-a-digest", CommitDigest: strings.Repeat("a", 64),
			TargetSnapshotDigest:    group.TargetSnapshotDigest,
			AuthenticatedDataDigest: authDigest, Signature: "fixture-signature:" + authDigest}
		raw, _ := json.Marshal(malformed)
		if _, err := f.store.RouteE2EECommit(e2eeRoutingConfig(), raw, f.now+90); err == nil {
			t.Fatal("malformed predecessor accepted")
		}
		group, _ = f.store.GetE2EEGroup(group.ID)
		if group.ForkState != "clean" || group.CurrentEpoch != 7 {
			t.Fatalf("malformed predecessor poisoned group=%+v", group)
		}
	})
}
