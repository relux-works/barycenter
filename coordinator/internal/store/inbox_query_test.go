package store

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func createMissedInboxItem(
	t *testing.T,
	st *Store,
	media MediaItem,
	source OnboardingCredentials,
	target OnboardingCredentials,
	acceptedAt int64,
) TransmissionInboxItem {
	t.Helper()
	created, err := st.CreateTransmission(transmissionParams(
		media, source, acceptedAt, transmissionTarget(target, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	transitionTargetToInboxReceipt(t, st, created,
		TransmissionTargetMissedOffline, TransmissionReasonOfflineBeforeStart,
		acceptedAt+1)
	item, err := scanTransmissionInboxItem(st.db.QueryRow(
		`SELECT `+transmissionInboxColumns+` FROM transmission_inbox_items
WHERE transmission_id = ?`, created.Transmission.ID,
	))
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func bearerIdentity(credentials OnboardingCredentials) Identity {
	return Identity{Kind: IdentityBearer, Token: credentials.ControlToken}
}

func TestAuthorizedInboxPaginationIsolationDismissAndReplay(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	target, err := st.CreateSelfServiceOrbit("Inbox API target")
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateSelfServiceOrbit("Inbox API other")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(t, st, source, now,
		now+int64((45*24*time.Hour)/time.Millisecond))
	oldest := createMissedInboxItem(t, st, media, source, target, now+10)
	middle := createMissedInboxItem(t, st, media, source, target, now+20)
	newest := createMissedInboxItem(t, st, media, source, target, now+30)

	first, err := st.QueryAuthorizedTransmissionInbox(
		target.ActorID, bearerIdentity(target), "all", 2, "", now+40,
	)
	if err != nil || len(first.Items) != 2 || first.NextCursor == "" ||
		first.Items[0].Item.ID != newest.ID || first.Items[1].Item.ID != middle.ID {
		t.Fatalf("first page=%+v err=%v", first, err)
	}
	late := createMissedInboxItem(t, st, media, source, target, now+50)
	second, err := st.QueryAuthorizedTransmissionInbox(
		target.ActorID, bearerIdentity(target), "all", 2, first.NextCursor, now+60,
	)
	if err != nil || len(second.Items) != 1 || second.Items[0].Item.ID != oldest.ID {
		t.Fatalf("second page=%+v err=%v", second, err)
	}
	for _, item := range second.Items {
		if item.Item.ID == late.ID {
			t.Fatalf("concurrent insert expanded frozen page: %+v", item)
		}
	}
	if _, err := st.QueryAuthorizedTransmissionInbox(
		other.ActorID, bearerIdentity(other), "all", 20, first.NextCursor, now+60,
	); !errors.Is(err, ErrInboxCursorExpired) {
		t.Fatalf("cross-actor cursor error=%v", err)
	}
	empty, err := st.QueryAuthorizedTransmissionInbox(
		other.ActorID, bearerIdentity(other), "all", 20, "", now+60,
	)
	if err != nil || len(empty.Items) != 0 {
		t.Fatalf("non-target inbox=%+v err=%v", empty, err)
	}
	if _, err := st.GetAuthorizedTransmissionInboxItem(
		other.ActorID, bearerIdentity(other), newest.ID, now+60,
	); !errors.Is(err, ErrTransmissionInboxNotFound) {
		t.Fatalf("non-target detail error=%v", err)
	}

	dismissed, err := st.DismissAuthorizedTransmissionInboxItem(
		target.ActorID, bearerIdentity(target), middle.ID, now+61,
	)
	if err != nil || dismissed.Item.Availability != TransmissionInboxDismissed || dismissed.CanDismiss {
		t.Fatalf("dismissed=%+v err=%v", dismissed, err)
	}
	retryDismiss, err := st.DismissAuthorizedTransmissionInboxItem(
		target.ActorID, bearerIdentity(target), middle.ID, now+62,
	)
	if err != nil || retryDismiss.Item.Revision != dismissed.Item.Revision {
		t.Fatalf("dismiss retry=%+v err=%v", retryDismiss, err)
	}

	acceptCurrentContentPolicy(t, st, target, now+63)
	replayParams := CreateAuthorizedInboxReplayParams{
		ExpectedActorID: target.ActorID, Identity: bearerIdentity(target), InboxID: newest.ID,
		IdempotencyKeyHash: strings.Repeat("a", 64), RequestHash: strings.Repeat("b", 64),
		RequestedDelivery: TransmissionDeliveryAfterCurrent, AcceptedAt: now + 64,
	}
	replay, err := st.CreateAuthorizedInboxReplay(replayParams)
	if err != nil || replay.Reused || len(replay.Creation.Targets) != 1 ||
		replay.Creation.Targets[0].ActorID != target.ActorID ||
		replay.Creation.Targets[0].BindingPairedAt != newest.BindingPairedAt {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	reused, err := st.CreateAuthorizedInboxReplay(replayParams)
	if err != nil || !reused.Reused ||
		reused.Creation.Transmission.ID != replay.Creation.Transmission.ID {
		t.Fatalf("reused replay=%+v err=%v", reused, err)
	}
	consumed, err := st.GetTransmissionInboxItem(newest.ID)
	if err != nil || consumed == nil || consumed.Availability != TransmissionInboxReplayed {
		t.Fatalf("consumed=%+v err=%v", consumed, err)
	}
	var lineageInboxID, lineageOriginalID string
	if err := st.db.QueryRow(`SELECT replay_of_inbox_id, replay_of_transmission_id
FROM transmission_replay_lineage WHERE transmission_id = ?`,
		replay.Creation.Transmission.ID).Scan(&lineageInboxID, &lineageOriginalID); err != nil ||
		lineageInboxID != newest.ID || lineageOriginalID != newest.TransmissionID {
		t.Fatalf("lineage inbox=%q original=%q err=%v", lineageInboxID, lineageOriginalID, err)
	}
	if _, err := st.CreateAuthorizedInboxReplay(CreateAuthorizedInboxReplayParams{
		ExpectedActorID: target.ActorID, Identity: bearerIdentity(target), InboxID: newest.ID,
		IdempotencyKeyHash: strings.Repeat("c", 64), RequestHash: strings.Repeat("d", 64),
		RequestedDelivery: TransmissionDeliveryAfterCurrent, AcceptedAt: now + 65,
	}); !errors.Is(err, ErrTransmissionInboxNotFound) {
		t.Fatalf("second replay authority error=%v", err)
	}
}

func TestAuthorizedHistoryReceiptPaginationIsAudienceSafe(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	first, err := st.CreateSelfServiceOrbit("Receipt first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateSelfServiceOrbit("Receipt second")
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := st.CreateSelfServiceOrbit("Receipt outsider")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyLifecycleMedia(t, st, source, now,
		now+int64((45*24*time.Hour)/time.Millisecond))
	created, err := st.CreateTransmission(transmissionParams(
		media, source, now+3, transmissionTarget(first, true), transmissionTarget(second, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	historyItemID := historyID("transmission", created.Transmission.ID)
	page, err := st.QueryAuthorizedHistoryReceipts(
		source.ActorID, bearerIdentity(source), historyItemID, 1, "", now+4,
	)
	if err != nil || len(page.Items) != 1 || page.NextCursor == "" ||
		page.Items[0].DisplayLabel == "" {
		t.Fatalf("receipt page=%+v err=%v", page, err)
	}
	next, err := st.QueryAuthorizedHistoryReceipts(
		source.ActorID, bearerIdentity(source), historyItemID, 1, page.NextCursor, now+5,
	)
	if err != nil || len(next.Items) != 1 || next.NextCursor != "" ||
		next.Items[0].Target.ActorID == page.Items[0].Target.ActorID {
		t.Fatalf("receipt next=%+v err=%v", next, err)
	}
	if _, err := st.QueryAuthorizedHistoryReceipts(
		outsider.ActorID, bearerIdentity(outsider), historyItemID, 1, "", now+5,
	); !errors.Is(err, ErrTransmissionNotFound) {
		t.Fatalf("outsider receipt error=%v", err)
	}
	targetPage, err := st.QueryAuthorizedHistoryReceipts(
		first.ActorID, bearerIdentity(first), historyItemID, 20, "", now+5,
	)
	if err != nil || len(targetPage.Items) != 1 || targetPage.Items[0].Target.ActorID != first.ActorID {
		t.Fatalf("target receipt page=%+v err=%v", targetPage, err)
	}
	if _, err := st.QueryAuthorizedHistoryReceipts(
		second.ActorID, bearerIdentity(second), historyItemID, 1, page.NextCursor, now+5,
	); !errors.Is(err, ErrReceiptCursorExpired) {
		t.Fatalf("cross-target receipt cursor error=%v", err)
	}
}
