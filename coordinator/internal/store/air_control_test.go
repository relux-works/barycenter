package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func airTestAuth(credentials OnboardingCredentials, key, request string, now int64) AirMutationAuth {
	keyDigest := sha256.Sum256([]byte(key))
	requestDigest := sha256.Sum256([]byte(request))
	return AirMutationAuth{
		ExpectedActorID:    credentials.ActorID,
		Bearer:             credentials.ControlToken,
		IdempotencyKeyHash: hex.EncodeToString(keyDigest[:]),
		RequestHash:        hex.EncodeToString(requestDigest[:]),
		Now:                now,
	}
}

func createAirControlOrbit(t *testing.T, st *Store, title string) OnboardingCredentials {
	t.Helper()
	credentials, err := st.CreateSelfServiceOrbit(title)
	if err != nil {
		t.Fatal(err)
	}
	return credentials
}

func TestAuthorizedAirControlLifecycleIdempotencyAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "air-control.db")
	st, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	owner := createAirControlOrbit(t, st, "Owner")
	peer := createAirControlOrbit(t, st, "Peer")
	third := createAirControlOrbit(t, st, "Third")
	if _, err := st.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}

	createAuth := airTestAuth(owner, "create-owner-air-0001", `{"title":"Family"}`, 200)
	created, err := st.CreateAuthorizedAir(createAuth, " Family ")
	if err != nil || created.Title != "Family" || created.AirRole != "owner" || created.Status != "parked" {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	replayed, err := st.CreateAuthorizedAir(createAuth, "Family")
	if err != nil || replayed.AirID != created.AirID {
		t.Fatalf("create replay=%+v err=%v", replayed, err)
	}
	conflict := createAuth
	conflict.RequestHash = airTestAuth(owner, "unused", `{"title":"Other"}`, 201).RequestHash
	if _, err := st.CreateAuthorizedAir(conflict, "Other"); !errors.Is(err, ErrAirIdempotencyConflict) {
		t.Fatalf("create conflict=%v", err)
	}

	issueAuth := airTestAuth(owner, "issue-peer-air-0001", `{"air_role":"member"}`, 210)
	issued, err := st.IssueAuthorizedAirInvite(issueAuth, created.AirID, "member")
	if err != nil || len(issued.Code) != 43 {
		t.Fatalf("issued=%+v err=%v", issued, err)
	}
	issuedReplay, err := st.IssueAuthorizedAirInvite(issueAuth, created.AirID, "member")
	if err != nil || issuedReplay != issued {
		t.Fatalf("issue replay=%+v want=%+v err=%v", issuedReplay, issued, err)
	}
	var storedHash, mutationJSON string
	if err := st.db.QueryRow(`SELECT code_hash FROM air_invites WHERE public_id = ?`, issued.InviteID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT response_json FROM air_mutation_results
WHERE actor_id = ? AND idempotency_key_hash = ?`, owner.ActorID, issueAuth.IdempotencyKeyHash).Scan(&mutationJSON); err != nil {
		t.Fatal(err)
	}
	if storedHash == issued.Code || strings.Contains(mutationJSON, issued.Code) || len(storedHash) != 64 {
		t.Fatalf("invite secret reached storage hash=%q response=%q", storedHash, mutationJSON)
	}

	consumeAuth := airTestAuth(peer, "consume-peer-air-0001", `{"code":"redacted"}`, 220)
	preview, err := st.ConsumeAuthorizedAirInvite(consumeAuth, issued.Code)
	if err != nil || preview.AirID != created.AirID || preview.MembershipRevision != 1 ||
		preview.OwnerDisplayName != "Owner" {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	previewReplay, err := st.ConsumeAuthorizedAirInvite(consumeAuth, issued.Code)
	if err != nil || previewReplay.MembershipID != preview.MembershipID {
		t.Fatalf("consume replay=%+v err=%v", previewReplay, err)
	}
	if _, err := st.ConsumeAuthorizedAirInvite(
		airTestAuth(third, "consume-burned-air-0001", `{"code":"redacted"}`, 221), issued.Code,
	); !errors.Is(err, ErrAirInviteUnavailable) {
		t.Fatalf("burned invite error=%v", err)
	}

	if _, err := st.db.Exec(`UPDATE memberships SET role = 'companion' WHERE actor_id = ?`, peer.ActorID); err != nil {
		t.Fatal(err)
	}
	confirmAuth := airTestAuth(peer, "confirm-peer-air-0001", `{"membership_revision":1,"activate":true,"expected_active_air_id":"none"}`, 230)
	if _, err := st.ConfirmAuthorizedAirJoin(confirmAuth, created.AirID, 1, true, "none"); !errors.Is(err, ErrAirForbidden) {
		t.Fatalf("companion confirmation error=%v", err)
	}
	if _, err := st.db.Exec(`UPDATE memberships SET role = 'primary' WHERE actor_id = ?`, peer.ActorID); err != nil {
		t.Fatal(err)
	}
	confirmed, err := st.ConfirmAuthorizedAirJoin(confirmAuth, created.AirID, 1, true, "none")
	if err != nil || confirmed.Projection == nil || !confirmed.Projection.IsCurrent {
		t.Fatalf("confirmed=%+v err=%v", confirmed, err)
	}
	ownerActive, err := st.ActivateAuthorizedAir(
		airTestAuth(owner, "activate-owner-air-0001", `{"membership_revision":1,"expected_active_air_id":"none"}`, 240),
		created.AirID, 1, "none",
	)
	if err != nil || ownerActive.Status != "active" {
		t.Fatalf("owner activation=%+v err=%v", ownerActive, err)
	}

	list, err := st.AuthorizedAirList(peer.ActorID, peer.ControlToken)
	if err != nil || list.CurrentAirID != created.AirID || len(list.Saved) != 1 || !list.Saved[0].IsCurrent {
		t.Fatalf("peer list=%+v err=%v", list, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	list, err = st.AuthorizedAirList(peer.ActorID, peer.ControlToken)
	if err != nil || list.CurrentAirID != created.AirID || list.Saved[0].Status != "active" {
		t.Fatalf("restart list=%+v err=%v", list, err)
	}
	var audits int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM air_audit_events
WHERE actor_id > 0 AND operation IN ('air.create','air.invite.issue','air.invite.consume','air.join.confirm','air.activate')`).Scan(&audits); err != nil || audits != 5 {
		t.Fatalf("accepted mutation audits=%d err=%v", audits, err)
	}
}

func TestAuthorizedAirConcurrentConsumeAndCapacity(t *testing.T) {
	st := openIdentityTemp(t)
	owner := createAirControlOrbit(t, st, "Capacity owner")
	peers := make([]OnboardingCredentials, 9)
	for i := range peers {
		peers[i] = createAirControlOrbit(t, st, "Capacity peer")
	}
	if _, err := st.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateAuthorizedAir(
		airTestAuth(owner, "capacity-create-0001", `{"title":"Capacity"}`, 200), "Capacity",
	)
	if err != nil {
		t.Fatal(err)
	}

	issue := func(index int, now int64) AirInviteIssueResult {
		t.Helper()
		result, err := st.IssueAuthorizedAirInvite(
			airTestAuth(owner, "capacity-issue-"+string(rune('a'+index))+"-0001", `{"air_role":"member"}`, now),
			created.AirID, "member",
		)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	first := issue(0, 210)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, err := st.ConsumeAuthorizedAirInvite(
				airTestAuth(peers[index], "race-consume-"+string(rune('a'+index))+"-0001", `{"code":"redacted"}`, 220+int64(index)),
				first.Code,
			)
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	successes, unavailable := 0, 0
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrAirInviteUnavailable):
			unavailable++
		default:
			t.Fatalf("concurrent consume error=%v", err)
		}
	}
	if successes != 1 || unavailable != 1 {
		t.Fatalf("concurrent consume successes=%d unavailable=%d", successes, unavailable)
	}

	// Owner plus the winning pending member occupy two of eight slots.
	for index := 2; index < 8; index++ {
		invite := issue(index, 230+int64(index))
		if _, err := st.ConsumeAuthorizedAirInvite(
			airTestAuth(peers[index], "capacity-consume-"+string(rune('a'+index))+"-0001", `{"code":"redacted"}`, 250+int64(index)),
			invite.Code,
		); err != nil {
			t.Fatal(err)
		}
	}
	overflow := issue(8, 300)
	if _, err := st.ConsumeAuthorizedAirInvite(
		airTestAuth(peers[8], "capacity-overflow-0001", `{"code":"redacted"}`, 301), overflow.Code,
	); !errors.Is(err, ErrAirCapacity) {
		t.Fatalf("overflow consume error=%v", err)
	}
}

func TestAuthorizedAirGovernanceTransferLeaveAndDissolve(t *testing.T) {
	st := openIdentityTemp(t)
	owner := createAirControlOrbit(t, st, "Governance owner")
	peer := createAirControlOrbit(t, st, "Governance peer")
	if _, err := st.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateAuthorizedAir(
		airTestAuth(owner, "governance-create-0001", `{"title":"Governance"}`, 200), "Governance",
	)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := st.IssueAuthorizedAirInvite(
		airTestAuth(owner, "governance-issue-0001", `{"air_role":"member"}`, 210), created.AirID, "member",
	)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := st.ConsumeAuthorizedAirInvite(
		airTestAuth(peer, "governance-consume-0001", `{"code":"redacted"}`, 220), issued.Code,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ConfirmAuthorizedAirJoin(
		airTestAuth(peer, "governance-confirm-0001", `{"membership_revision":1,"activate":false}`, 230),
		created.AirID, preview.MembershipRevision, false, "",
	); err != nil {
		t.Fatal(err)
	}
	peerView, err := st.AuthorizedAir(peer.ActorID, peer.ControlToken, created.AirID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ReplaceAuthorizedAirPolicy(
		airTestAuth(peer, "governance-policy-denied-0001", `{"policy_revision":1}`, 240),
		created.AirID, storeDefaultAirPolicyView(1),
	); !errors.Is(err, ErrAirForbidden) {
		t.Fatalf("member policy error=%v", err)
	}
	ownerView, err := st.AuthorizedAir(owner.ActorID, owner.ControlToken, created.AirID)
	if err != nil {
		t.Fatal(err)
	}
	roleResult, err := st.ReplaceAuthorizedAirMemberRole(
		airTestAuth(owner, "governance-role-0001", `{"air_role":"admin"}`, 250),
		created.AirID, peerView.MembershipID, ownerView.Revision, peerView.MembershipRevision, "admin",
	)
	if err != nil || roleResult.Revision != ownerView.Revision+1 {
		t.Fatalf("role result=%+v err=%v", roleResult, err)
	}
	peerView, err = st.AuthorizedAir(peer.ActorID, peer.ControlToken, created.AirID)
	if err != nil {
		t.Fatal(err)
	}
	transferred, err := st.TransferAuthorizedAirOwnership(
		airTestAuth(owner, "governance-transfer-0001", `{"membership_id":"redacted"}`, 260),
		created.AirID, peerView.MembershipID, roleResult.Revision, peerView.MembershipRevision,
	)
	if err != nil || transferred.AirRole != "admin" {
		t.Fatalf("transfer result=%+v err=%v", transferred, err)
	}
	ownerView, err = st.AuthorizedAir(owner.ActorID, owner.ControlToken, created.AirID)
	if err != nil {
		t.Fatal(err)
	}
	left, err := st.LeaveAuthorizedAir(
		airTestAuth(owner, "governance-leave-0001", `{"expected_active_air_id":"none"}`, 270),
		created.AirID, ownerView.MembershipRevision, "none",
	)
	if err != nil || left.Status != "left" {
		t.Fatalf("leave=%+v err=%v", left, err)
	}
	peerView, err = st.AuthorizedAir(peer.ActorID, peer.ControlToken, created.AirID)
	if err != nil || peerView.AirRole != "owner" {
		t.Fatalf("new owner view=%+v err=%v", peerView, err)
	}
	dissolved, err := st.DissolveAuthorizedAir(
		airTestAuth(peer, "governance-dissolve-0001", `{"air_revision":5}`, 280),
		created.AirID, peerView.Revision,
	)
	if err != nil || dissolved.Status != "dissolved" {
		t.Fatalf("dissolved=%+v err=%v", dissolved, err)
	}
	if _, err := st.AuthorizedAir(peer.ActorID, peer.ControlToken, created.AirID); !errors.Is(err, ErrAirNotFound) {
		t.Fatalf("dissolved read error=%v", err)
	}
	if _, err := st.DissolveAuthorizedAir(
		airTestAuth(peer, "governance-dissolve-again-0001", `{"air_revision":6}`, 290),
		created.AirID, peerView.Revision+1,
	); !errors.Is(err, ErrAirDissolved) {
		t.Fatalf("terminal dissolve error=%v", err)
	}
}

func storeDefaultAirPolicyView(revision int64) AirPolicyView {
	return AirPolicyView{
		Revision: revision, Invite: "air_admin_primary", Overlay: "primary_companion",
		Queue: "primary_companion", Replace: "air_admin_primary",
	}
}
