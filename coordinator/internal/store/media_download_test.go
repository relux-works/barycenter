package store

import (
	"errors"
	"testing"
	"time"
)

func mustDownloadContext(t *testing.T, st *Store, token string) ActorContext {
	t.Helper()
	ctx, err := st.ResolveTokenActorContext(token)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}

func TestAuthorizeMediaDownloadSeparatesOwnerControlAndSnapshottedNodes(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	target, err := st.CreateSelfServiceOrbit("Download target")
	if err != nil {
		t.Fatal(err)
	}
	nontarget, err := st.CreateSelfServiceOrbit("Download non-target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	ready := readyLifecycleMedia(
		t, st, owner, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	ownerControl := mustDownloadContext(t, st, owner.ControlToken)
	ownerNode := mustDownloadContext(t, st, owner.NodeToken)
	targetNode := mustDownloadContext(t, st, target.NodeToken)
	targetControl := mustDownloadContext(t, st, target.ControlToken)
	nontargetNode := mustDownloadContext(t, st, nontarget.NodeToken)

	if got, err := st.AuthorizeMediaDownload(
		ownerControl, owner.ControlToken, ready.ID, false, now+3,
	); err != nil || got.ID != ready.ID {
		t.Fatalf("owner control download=%+v err=%v", got, err)
	}
	if _, err := st.AuthorizeMediaDownload(
		ownerNode, owner.NodeToken, ready.ID, false, now+3,
	); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("unsnapshotted owner node error=%v", err)
	}
	if got, err := st.AuthorizeMediaDownload(
		ownerNode, owner.NodeToken, ready.ID, true, now+3,
	); err != nil || got.ID != ready.ID {
		t.Fatalf("snapshotted owner node download=%+v err=%v", got, err)
	}
	if got, err := st.AuthorizeMediaDownload(
		targetNode, target.NodeToken, ready.ID, true, now+3,
	); err != nil || got.ID != ready.ID {
		t.Fatalf("snapshotted target node download=%+v err=%v", got, err)
	}
	if _, err := st.AuthorizeMediaDownload(
		targetControl, target.ControlToken, ready.ID, true, now+3,
	); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("foreign target control error=%v", err)
	}
	if _, err := st.AuthorizeMediaDownload(
		nontargetNode, nontarget.NodeToken, ready.ID, false, now+3,
	); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("non-target node error=%v", err)
	}
	if _, err := st.AuthorizeMediaDownload(
		targetNode, target.NodeToken, "m_00000000000000000000000000", true, now+3,
	); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("guessed media error=%v", err)
	}
	if _, err := st.AuthorizeMediaDownload(
		targetNode, owner.NodeToken, ready.ID, true, now+3,
	); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("mismatched middleware identity error=%v", err)
	}
}

func TestAuthorizeMediaDownloadNeverUsesLiveApproachMembership(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	target, err := st.CreateSelfServiceOrbit("Approach target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	ready := readyLifecycleMedia(
		t, st, owner, now, now+int64((7*24*time.Hour)/time.Millisecond),
	)
	targetNode := mustDownloadContext(t, st, target.NodeToken)
	code, err := st.ProposeLink(owner.OrbitID, owner.ActorID)
	if err != nil {
		t.Fatal(err)
	}
	linkID, _, err := st.AcceptByCode(code, target.OrbitID)
	if err != nil || linkID == 0 {
		t.Fatalf("accept approach link=%d err=%v", linkID, err)
	}
	if err := st.ActivateLink(linkID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AuthorizeMediaDownload(
		targetNode, target.NodeToken, ready.ID, false, now+3,
	); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("active approach expanded generic ACL: %v", err)
	}
	if _, err := st.AuthorizeMediaDownload(
		targetNode, target.NodeToken, ready.ID, true, now+3,
	); err != nil {
		t.Fatalf("accepted snapshot denied during approach: %v", err)
	}
	if err := st.BreakLink(linkID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AuthorizeMediaDownload(
		targetNode, target.NodeToken, ready.ID, true, now+3,
	); err != nil {
		t.Fatalf("approach leave rewrote immutable snapshot: %v", err)
	}
}

func TestAuthorizeMediaDownloadRejectsTerminalExpiredAndRevokedState(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	target, err := st.CreateSelfServiceOrbit("Revocation target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	targetNode := mustDownloadContext(t, st, target.NodeToken)

	expiring := readyLifecycleMedia(t, st, owner, now, now+100)
	if _, err := st.AuthorizeMediaDownload(
		targetNode, target.NodeToken, expiring.ID, true, now+100,
	); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("expired boundary error=%v", err)
	}
	deletable := readyLifecycleMedia(
		t, st, owner, now+200, now+200+int64((7*24*time.Hour)/time.Millisecond),
	)
	if _, err := st.DeleteAuthorizedMedia(
		owner.ActorID, owner.ControlToken, deletable.ID, now+203,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AuthorizeMediaDownload(
		targetNode, target.NodeToken, deletable.ID, true, now+204,
	); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("deleted media error=%v", err)
	}
	if found, err := st.RevokeSlot(target.OrbitID, target.Slot); err != nil || !found {
		t.Fatalf("revoke target slot found=%v err=%v", found, err)
	}
	if _, err := st.AuthorizeMediaDownload(
		targetNode, target.NodeToken, deletable.ID, true, now+205,
	); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked target credential error=%v", err)
	}
	secondTarget, err := st.CreateSelfServiceOrbit("Revoked sender target")
	if err != nil {
		t.Fatal(err)
	}
	secondTargetNode := mustDownloadContext(t, st, secondTarget.NodeToken)
	revokedOwnerMedia := readyLifecycleMedia(
		t, st, owner, now+300, now+300+int64((7*24*time.Hour)/time.Millisecond),
	)
	if _, err := st.db.Exec(`UPDATE actors SET revoked_at = ? WHERE id = ?`, now+303, owner.ActorID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AuthorizeMediaDownload(
		secondTargetNode, secondTarget.NodeToken, revokedOwnerMedia.ID, true, now+304,
	); !errors.Is(err, ErrMediaNotFound) {
		t.Fatalf("revoked sender media error=%v", err)
	}
}

func TestLegacyMediaLookupAcceptsNodeButNotControlCredential(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	if orbitID, slot, ok, err := st.LookupLegacyMediaNodeToken(owner.NodeToken); err != nil ||
		!ok || orbitID != owner.OrbitID || slot != owner.Slot {
		t.Fatalf("legacy node orbit=%d slot=%q ok=%v err=%v", orbitID, slot, ok, err)
	}
	if _, _, ok, err := st.LookupLegacyMediaNodeToken(owner.ControlToken); err != nil || ok {
		t.Fatalf("legacy control credential ok=%v err=%v", ok, err)
	}
}
