package store

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func createTelegramAliasOrbit(t *testing.T, st *Store, title string, telegramUserID int64) int64 {
	t.Helper()
	orbit, err := st.CreateOrbit(title, telegramUserID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := st.ResolveTelegramActorContext(telegramUserID)
	if err != nil || ctx.OrbitID != orbit.ID || ctx.Role != "primary" {
		t.Fatalf("telegram context=%+v err=%v", ctx, err)
	}
	return orbit.ID
}

func TestAirApproachAliasLifecycleIdempotencyRestartAndCallerLocalApart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "air-approach-alias.db")
	st, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	ownerOrbit := createTelegramAliasOrbit(t, st, "Owner", 101)
	peerOrbit := createTelegramAliasOrbit(t, st, "Peer", 202)
	if _, err := st.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}

	created, err := st.CreateAirApproachAlias(101, 200)
	if err != nil || len(created.Code) != 12 || created.OwnerOrbitID != ownerOrbit {
		t.Fatalf("created=%+v err=%v", created, err)
	}
	replayed, err := st.CreateAirApproachAlias(101, 201)
	if err != nil || !replayed.Replayed || replayed.AirID != created.AirID || replayed.Code != created.Code {
		t.Fatalf("create replay=%+v err=%v", replayed, err)
	}
	var hash string
	if err := st.db.QueryRow(`SELECT code_hash FROM air_invites WHERE public_id = ?`, created.InviteID).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if hash == created.Code || strings.Contains(hash, created.Code) || len(hash) != 64 {
		t.Fatalf("plaintext alias code reached storage: %q", hash)
	}

	claimed, err := st.ConsumeAirApproachAlias(202, created.Code, 210)
	if err != nil || claimed.AirID != created.AirID || claimed.CallerOrbitID != peerOrbit {
		t.Fatalf("claimed=%+v err=%v", claimed, err)
	}
	claimReplay, err := st.ConsumeAirApproachAlias(202, created.Code, 211)
	if err != nil || !claimReplay.Replayed || claimReplay.MembershipID != claimed.MembershipID {
		t.Fatalf("claim replay=%+v err=%v", claimReplay, err)
	}
	if _, err := st.ConsumeAirApproachAlias(101, created.Code, 212); !errors.Is(err, ErrAirInviteUnavailable) {
		t.Fatalf("burned code accepted by owner: %v", err)
	}

	confirmed, err := st.ConfirmAirApproachAlias(202, 220)
	if err != nil || confirmed.Outcome != "joined" {
		t.Fatalf("confirmed=%+v err=%v", confirmed, err)
	}
	confirmReplay, err := st.ConfirmAirApproachAlias(202, 221)
	if err != nil || !confirmReplay.Replayed {
		t.Fatalf("confirm replay=%+v err=%v", confirmReplay, err)
	}
	runtime, err := st.ActiveAirRuntimeByID(created.AirID)
	if err != nil || runtime == nil || len(runtime.OrbitIDs) != 2 {
		t.Fatalf("runtime=%+v err=%v", runtime, err)
	}
	if err := st.SetSetting("session_state_"+created.AirID, `{"state":"paused"}`); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	runtime, err = st.ActiveAirRuntimeByID(created.AirID)
	if err != nil || runtime == nil || len(runtime.OrbitIDs) != 2 {
		t.Fatalf("restart runtime=%+v err=%v", runtime, err)
	}

	left, err := st.LeaveCurrentAirAlias(101, 230)
	if err != nil || left.CallerOrbitID != ownerOrbit || left.OtherOrbitID != peerOrbit {
		t.Fatalf("owner apart=%+v err=%v", left, err)
	}
	if runtime, err := st.ActiveAirRuntimeByID(created.AirID); err != nil || runtime != nil {
		t.Fatalf("pair should park after one caller leaves: runtime=%+v err=%v", runtime, err)
	}
	if _, _, ok, err := st.ActiveAirForOrbit(ownerOrbit); err != nil || ok {
		t.Fatalf("owner pointer survived apart ok=%v err=%v", ok, err)
	}
	peerAir, _, ok, err := st.ActiveAirForOrbit(peerOrbit)
	if err != nil || !ok || peerAir != created.AirID {
		t.Fatalf("remaining peer pointer=%q ok=%v err=%v", peerAir, ok, err)
	}
	var newOwner, oldStatus string
	if err := st.db.QueryRow(`SELECT air_role FROM air_members WHERE air_id = ? AND orbit_id = ? AND status = 'joined'`,
		created.AirID, peerOrbit).Scan(&newOwner); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT status FROM air_members WHERE air_id = ? AND orbit_id = ? ORDER BY created_at DESC LIMIT 1`,
		created.AirID, ownerOrbit).Scan(&oldStatus); err != nil {
		t.Fatal(err)
	}
	if newOwner != "owner" || oldStatus != "left" {
		t.Fatalf("ownership after apart new=%q old=%q", newOwner, oldStatus)
	}
	if _, err := st.ProposeLink(ownerOrbit, 101); !errors.Is(err, ErrAirRevision) {
		t.Fatalf("stale legacy alias resurrected after cutover: %v", err)
	}
}

func TestAirApproachAliasDeclineWithdrawAndSwitchGuard(t *testing.T) {
	st := openIdentityTemp(t)
	ownerOrbit := createTelegramAliasOrbit(t, st, "Owner", 301)
	peerOrbit := createTelegramAliasOrbit(t, st, "Peer", 302)
	otherOrbit := createTelegramAliasOrbit(t, st, "Other", 303)
	if _, err := st.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}

	open, err := st.CreateAirApproachAlias(301, 200)
	if err != nil {
		t.Fatal(err)
	}
	withdrawn, err := st.DeclineAirApproachAlias(301, 201)
	if err != nil || withdrawn.Outcome != "withdrawn" {
		t.Fatalf("withdraw=%+v err=%v", withdrawn, err)
	}
	if _, err := st.ConsumeAirApproachAlias(302, open.Code, 202); !errors.Is(err, ErrAirInviteUnavailable) {
		t.Fatalf("withdrawn code accepted: %v", err)
	}
	if _, _, ok, err := st.ActiveAirForOrbit(ownerOrbit); err != nil || ok {
		t.Fatalf("withdraw left creator pointer ok=%v err=%v", ok, err)
	}

	pending, err := st.CreateAirApproachAlias(301, 210)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ConsumeAirApproachAlias(302, pending.Code, 211); err != nil {
		t.Fatal(err)
	}
	declined, err := st.DeclineAirApproachAlias(302, 212)
	if err != nil || declined.Outcome != "declined" {
		t.Fatalf("decline=%+v err=%v", declined, err)
	}
	if _, err := st.ConfirmAirApproachAlias(302, 213); !errors.Is(err, ErrAirApproachNothingPending) {
		t.Fatalf("declined membership confirmed: %v", err)
	}

	switchInvite, err := st.CreateAirApproachAlias(301, 220)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ConsumeAirApproachAlias(302, switchInvite.Code, 221); err != nil {
		t.Fatal(err)
	}
	otherAir, err := st.CreateAir(CreateAirParams{Title: "Other current", OwnerOrbitID: otherOrbit, CreatedAt: 222})
	if err != nil {
		t.Fatal(err)
	}
	otherMember, err := st.AddPendingAirMember(otherAir.ID, peerOrbit, "member", 223)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ConfirmAirMember(otherMember.ID, 1, true, "none", 224); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ConfirmAirApproachAlias(302, 225); !errors.Is(err, ErrAirApproachSwitchConfirmation) {
		t.Fatalf("silent Air switch was not blocked: %v", err)
	}
	current, _, ok, err := st.ActiveAirForOrbit(peerOrbit)
	if err != nil || !ok || current != otherAir.ID {
		t.Fatalf("switch guard changed pointer=%q ok=%v err=%v", current, ok, err)
	}
}

func TestMigratedApproachAliasRestartAndRollbackCannotResurrectStaleLink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migrated-approach-alias.db")
	st, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	ownerOrbit := createTelegramAliasOrbit(t, st, "Migrated owner", 401)
	peerOrbit := createTelegramAliasOrbit(t, st, "Migrated peer", 402)
	code, err := st.ProposeLink(ownerOrbit, 401)
	if err != nil {
		t.Fatal(err)
	}
	linkID, _, err := st.AcceptByCode(code, peerOrbit)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ActivateLink(linkID); err != nil {
		t.Fatal(err)
	}
	authority, err := st.CutoverLinksToAirs(1, 100)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := st.ActiveAirRuntimeForOrbit(ownerOrbit)
	if err != nil || runtime == nil || len(runtime.OrbitIDs) != 2 {
		t.Fatalf("migrated runtime=%+v err=%v", runtime, err)
	}
	airID := runtime.AirID
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	runtime, err = st.ActiveAirRuntimeForOrbit(peerOrbit)
	if err != nil || runtime == nil || runtime.AirID != airID {
		t.Fatalf("restarted migrated runtime=%+v err=%v", runtime, err)
	}
	left, err := st.LeaveCurrentAirAlias(402, 200)
	if err != nil || left.AirID != airID || left.CallerOrbitID != peerOrbit {
		t.Fatalf("migrated apart=%+v err=%v", left, err)
	}
	if runtime, err := st.ActiveAirRuntimeForOrbit(ownerOrbit); err != nil || runtime != nil {
		t.Fatalf("stale pair runtime survived apart=%+v err=%v", runtime, err)
	}
	if link, err := st.GetLink(linkID); err != nil || link == nil || link.State != "active" {
		t.Fatalf("frozen legacy rollback row changed link=%+v err=%v", link, err)
	}
	if _, err := st.RollbackAirsToLinks(authority.Generation, 210); !errors.Is(err, ErrAirRollbackUnsafe) {
		t.Fatalf("divergent rollback resurrected stale link: %v", err)
	}
	hold, err := st.AirAuthority()
	if err != nil || hold.Mode != "rollback_hold" {
		t.Fatalf("rollback authority=%+v err=%v", hold, err)
	}
}
