package store

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func createHistoryMedia(t *testing.T, st *Store, owner OnboardingCredentials, createdAt int64) MediaItem {
	t.Helper()
	item, err := st.CreateMediaItem(CreateMediaItemParams{
		OwnerOrbitID: owner.OrbitID,
		ActorID:      owner.ActorID,
		Kind:         MediaKindAudioClip,
		Source:       MediaSourceApp,
		Title:        "history fixture",
		CreatedAt:    createdAt,
		ExpiresAt:    createdAt + int64((90*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func createHistoryTransmission(
	t *testing.T,
	st *Store,
	source OnboardingCredentials,
	media MediaItem,
	acceptedAt int64,
	availability ...TransmissionTargetAvailability,
) TransmissionCreation {
	t.Helper()
	params := resolvedTransmissionParams(source, media, acceptedAt)
	params.IdempotencyKeyHash = hashToken("history-key:" + media.ID)
	params.RequestHash = hashToken("history-request:" + media.ID)
	params.Availability = availability
	created, err := st.CreateResolvedTransmission(params)
	if err != nil {
		t.Fatal(err)
	}
	return created.Creation
}

func TestHistoryMediaProjectionPaginationAndCursorBinding(t *testing.T) {
	st, owner := newMediaIngestTestStore(t)
	stranger, err := st.CreateSelfServiceOrbit("History stranger")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC).UnixMilli()
	oldFailed := createHistoryMedia(t, st, owner, now-int64((45*24*time.Hour)/time.Millisecond))
	if _, err := st.MarkMediaItemFailed(oldFailed.ID, oldFailed.Revision, "decode_failed", oldFailed.CreatedAt+1); err != nil {
		t.Fatal(err)
	}
	second := createHistoryMedia(t, st, owner, now-2000)
	first := createHistoryMedia(t, st, owner, now-1000)

	page1, err := st.QueryAuthorizedHistory(owner.ActorID,
		Identity{Kind: IdentityBearer, Token: owner.ControlToken}, "all", 1, "", now)
	if err != nil || len(page1.Items) != 1 || page1.Items[0].Media.ID != first.ID || page1.NextCursor == "" {
		t.Fatalf("page1=%+v err=%v", page1, err)
	}
	if strings.Contains(page1.NextCursor, first.ID) || strings.Contains(page1.NextCursor, owner.ControlToken) {
		t.Fatalf("cursor contains readable state: %q", page1.NextCursor)
	}
	// A newer insert after page one must not move or duplicate the frozen page.
	_ = createHistoryMedia(t, st, owner, now+1000)
	page2, err := st.QueryAuthorizedHistory(owner.ActorID,
		Identity{Kind: IdentityBearer, Token: owner.ControlToken}, "all", 1, page1.NextCursor, now+1000)
	if err != nil || len(page2.Items) != 1 || page2.Items[0].Media.ID != second.ID || page2.NextCursor == "" {
		t.Fatalf("page2=%+v err=%v", page2, err)
	}
	page3, err := st.QueryAuthorizedHistory(owner.ActorID,
		Identity{Kind: IdentityBearer, Token: owner.ControlToken}, "all", 1, page2.NextCursor, now+1000)
	if err != nil || len(page3.Items) != 1 || page3.Items[0].Media.ID != oldFailed.ID || page3.NextCursor != "" {
		t.Fatalf("page3=%+v err=%v", page3, err)
	}
	if page3.Items[0].Media.Status != MediaStatusFailed {
		t.Fatalf("retained failure missing after 30 days: %+v", page3.Items[0])
	}

	invalidCases := []struct {
		name     string
		actorID  int64
		identity Identity
		view     string
		limit    int
		now      int64
	}{
		{"wrong actor", stranger.ActorID, Identity{Kind: IdentityBearer, Token: stranger.ControlToken}, "all", 1, now + 1000},
		{"changed view", owner.ActorID, Identity{Kind: IdentityBearer, Token: owner.ControlToken}, "sent", 1, now + 1000},
		{"changed limit", owner.ActorID, Identity{Kind: IdentityBearer, Token: owner.ControlToken}, "all", 2, now + 1000},
		{"credential scope", owner.ActorID, Identity{Kind: IdentityBearer, Token: owner.NodeToken}, "all", 1, now + 1000},
		{"expired", owner.ActorID, Identity{Kind: IdentityBearer, Token: owner.ControlToken}, "all", 1, now + int64((24*time.Hour)/time.Millisecond)},
	}
	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := st.QueryAuthorizedHistory(tc.actorID, tc.identity, tc.view, tc.limit, page1.NextCursor, tc.now)
			if !errors.Is(err, ErrHistoryCursorInvalid) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	if _, err := st.QueryAuthorizedHistory(owner.ActorID,
		Identity{Kind: IdentityBearer, Token: owner.ControlToken}, "all", 1, "hc_bad", now); !errors.Is(err, ErrHistoryCursorInvalid) {
		t.Fatalf("malformed cursor error=%v", err)
	}
}

func TestHistoryTransmissionVisibilityRetentionAggregatesAndActions(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	companion := addTransmissionInstallation(t, st, source, "companion")
	telegramLink, err := st.IssueTelegramLink(source.ActorID, source.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	telegram, err := st.ConsumeTelegramLink(88001, "History bot user", "private", telegramLink.Code)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := st.CreateSelfServiceOrbit("History recipient")
	if err != nil {
		t.Fatal(err)
	}
	activateTransmissionApproach(t, st, source, peer)
	stranger, err := st.CreateSelfServiceOrbit("History outsider")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 14, 13, 0, 0, 0, time.UTC).UnixMilli()
	media := readyLifecycleMedia(t, st, source, now-10_000, now+int64((7*24*time.Hour)/time.Millisecond))
	sourceAvailable := fullTransmissionAvailability(source, now-9000)
	peerOffline := fullTransmissionAvailability(peer, now-9000)
	peerOffline.Connected = false
	creation := createHistoryTransmission(t, st, source, media, now-9000, sourceAvailable, peerOffline)

	sourcePage, err := st.QueryAuthorizedHistory(source.ActorID,
		Identity{Kind: IdentityBearer, Token: source.ControlToken}, "sent", 30, "", now)
	if err != nil || len(sourcePage.Items) != 1 {
		t.Fatalf("source page=%+v err=%v", sourcePage, err)
	}
	sourceItem := sourcePage.Items[0]
	if sourceItem.Direction != HistorySentAndReceived || sourceItem.TargetCount != 3 || len(sourceItem.Targets) != 3 ||
		sourceItem.TargetStatusCounts[TransmissionTargetAccepted] != 1 ||
		sourceItem.TargetStatusCounts[TransmissionTargetMissedOffline] != 2 ||
		!sourceItem.CanCancel || !sourceItem.CanDelete || !sourceItem.CanReplay || sourceItem.CanReport {
		t.Fatalf("source projection=%+v", sourceItem)
	}
	nodeSent, err := st.QueryAuthorizedHistory(source.ActorID,
		Identity{Kind: IdentityBearer, Token: source.NodeToken}, "sent", 30, "", now)
	if err != nil || len(nodeSent.Items) != 0 {
		t.Fatalf("node credential gained source history=%+v err=%v", nodeSent, err)
	}
	nodeReceipt, err := st.QueryAuthorizedHistory(source.ActorID,
		Identity{Kind: IdentityBearer, Token: source.NodeToken}, "received", 30, "", now)
	if err != nil || len(nodeReceipt.Items) != 1 || nodeReceipt.Items[0].Direction != HistoryReceived ||
		len(nodeReceipt.Items[0].Targets) != 1 || nodeReceipt.Items[0].CanReport {
		t.Fatalf("node exact receipt projection=%+v err=%v", nodeReceipt, err)
	}
	companionPage, err := st.QueryAuthorizedHistory(companion.ActorID,
		Identity{Kind: IdentityBearer, Token: companion.ControlToken}, "sent", 30, "", now)
	if err != nil || len(companionPage.Items) != 1 || companionPage.Items[0].Direction != HistorySentAndReceived ||
		len(companionPage.Items[0].Targets) != 1 || companionPage.Items[0].TargetCount != 3 {
		t.Fatalf("companion projection=%+v err=%v", companionPage, err)
	}
	telegramPage, err := st.QueryAuthorizedHistory(telegram.ActorID,
		Identity{Kind: IdentityTelegram, TelegramUserID: 88001}, "sent", 30, "", now)
	if err != nil || len(telegramPage.Items) != 1 || telegramPage.Items[0].Direction != HistorySentAndReceived ||
		len(telegramPage.Items[0].Targets) != 2 || telegramPage.Items[0].TargetCount != 3 ||
		!telegramPage.Items[0].CanReport {
		t.Fatalf("telegram projection=%+v err=%v", telegramPage, err)
	}
	if result, err := st.db.Exec(`UPDATE memberships SET left_at = ? WHERE actor_id = ? AND orbit_id = ? AND left_at IS NULL`,
		now, telegram.ActorID, source.OrbitID); err != nil {
		t.Fatal(err)
	} else if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		t.Fatalf("leave fixture changed=%d err=%v", changed, err)
	}
	if _, err := st.QueryAuthorizedHistory(telegram.ActorID,
		Identity{Kind: IdentityTelegram, TelegramUserID: 88001}, "all", 30, "", now); !errors.Is(err, ErrUnauthorized) && !errors.Is(err, ErrInsufficientCapability) {
		t.Fatalf("left Telegram history error=%v", err)
	}
	peerPage, err := st.QueryAuthorizedHistory(peer.ActorID,
		Identity{Kind: IdentityBearer, Token: peer.ControlToken}, "received", 30, "", now)
	if err != nil || len(peerPage.Items) != 1 || peerPage.Items[0].Direction != HistoryReceived ||
		len(peerPage.Items[0].Targets) != 1 || peerPage.Items[0].Targets[0].ActorID != peer.ActorID ||
		!peerPage.Items[0].CanReport || !peerPage.Items[0].CanBlockActor || !peerPage.Items[0].CanBlockOrbit {
		t.Fatalf("peer projection=%+v err=%v", peerPage, err)
	}
	outsiderPage, err := st.QueryAuthorizedHistory(stranger.ActorID,
		Identity{Kind: IdentityBearer, Token: stranger.ControlToken}, "all", 30, "", now)
	if err != nil || len(outsiderPage.Items) != 0 {
		t.Fatalf("outsider page=%+v err=%v", outsiderPage, err)
	}
	if _, err := st.GetAuthorizedHistoryItem(stranger.ActorID,
		Identity{Kind: IdentityBearer, Token: stranger.ControlToken}, sourceItem.HistoryItemID, now); !errors.Is(err, ErrTransmissionNotFound) {
		t.Fatalf("outsider detail error=%v", err)
	}
	// The linked media row is represented only by its transmission.
	for _, item := range sourcePage.Items {
		if item.ItemKind == "media" && item.Media.ID == media.ID {
			t.Fatalf("linked media duplicated in history: %+v", item)
		}
	}

	if _, err := st.CreateTransmissionBlock(CreateTransmissionBlockParams{
		OwnerScope: BlockOwnerActor, OwnerOrbitID: peer.OrbitID, OwnerActorID: peer.ActorID,
		AuthorizedByActorID: peer.ActorID, BlockedKind: BlockedSubjectActor,
		BlockedActorID: source.ActorID, CreatedAt: now + 1,
	}); err != nil {
		t.Fatal(err)
	}
	blockedPage, err := st.QueryAuthorizedHistory(peer.ActorID,
		Identity{Kind: IdentityBearer, Token: peer.ControlToken}, "received", 30, "", now+2)
	if err != nil || len(blockedPage.Items) != 1 || blockedPage.Items[0].CanBlockActor ||
		!blockedPage.Items[0].CanBlockOrbit || !blockedPage.Items[0].CanUnblock || !blockedPage.Items[0].RevealBlockedReason {
		t.Fatalf("block-derived actions=%+v err=%v", blockedPage, err)
	}
	if found, err := st.RevokeSlot(peer.OrbitID, peer.Slot); err != nil || !found {
		t.Fatalf("revoke peer found=%t err=%v", found, err)
	}
	if _, err := st.QueryAuthorizedHistory(peer.ActorID,
		Identity{Kind: IdentityBearer, Token: peer.ControlToken}, "all", 30, "", now+3); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked installation history error=%v", err)
	}

	oldMedia := readyLifecycleMedia(t, st, source,
		now-int64((31*24*time.Hour)/time.Millisecond)-1000,
		now+int64((7*24*time.Hour)/time.Millisecond))
	oldCreation := createHistoryTransmission(t, st, source, oldMedia,
		now-int64((31*24*time.Hour)/time.Millisecond), fullTransmissionAvailability(source, now))
	oldID := historyID("transmission", oldCreation.Transmission.ID)
	if _, err := st.GetAuthorizedHistoryItem(source.ActorID,
		Identity{Kind: IdentityBearer, Token: source.ControlToken}, oldID, now); !errors.Is(err, ErrTransmissionNotFound) {
		t.Fatalf("old transmission detail error=%v", err)
	}
	if creation.Transmission.ID == oldCreation.Transmission.ID {
		t.Fatal("history fixture ids collided")
	}
}
