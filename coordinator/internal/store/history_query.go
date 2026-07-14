package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrHistoryCursorInvalid = errors.New("history cursor is invalid")

type HistoryDirection string

const (
	HistorySent            HistoryDirection = "sent"
	HistoryReceived        HistoryDirection = "received"
	HistorySentAndReceived HistoryDirection = "sent_and_received"
)

type HistoryQueryItem struct {
	HistoryItemID       string
	ItemKind            string
	OccurredAt          int64
	Direction           HistoryDirection
	Media               MediaItem
	Transmission        *Transmission
	Targets             []TransmissionTarget
	TargetCount         int
	TargetStatusCounts  map[TransmissionTargetStatus]int
	CanCancel           bool
	CanDelete           bool
	CanReplay           bool
	CanReport           bool
	CanBlockActor       bool
	CanBlockOrbit       bool
	CanUnblock          bool
	RevealBlockedReason bool
	SourceActorID       int64
	SourceActorName     string
	SourceOrbitID       int64
	SourceOrbitName     string
}

type HistoryPage struct {
	Items      []HistoryQueryItem
	NextCursor string
}

type historyKey struct {
	at              int64
	id, kind, rawID string
}

func historyID(kind, rawID string) string {
	if kind == "transmission" {
		return "hi_" + strings.TrimPrefix(rawID, "tr_")
	}
	return "hi_" + strings.TrimPrefix(rawID, "m_")
}

func historyDirection(sent, received bool) HistoryDirection {
	if sent && received {
		return HistorySentAndReceived
	}
	if sent {
		return HistorySent
	}
	return HistoryReceived
}

func historyViewMatches(view string, direction HistoryDirection) bool {
	switch view {
	case "sent":
		return direction == HistorySent || direction == HistorySentAndReceived
	case "received":
		return direction == HistoryReceived || direction == HistorySentAndReceived
	default:
		return true
	}
}

func historyAuthorizationHash(ctx ActorContext, identity Identity) string {
	credential := ""
	if identity.Kind == IdentityBearer {
		credential = hashToken(identity.Token)
	} else {
		credential = hashToken("telegram:" + strconv.FormatInt(identity.TelegramUserID, 10))
	}
	return hashToken(strings.Join([]string{
		credential,
		strconv.FormatInt(ctx.ActorID, 10),
		strconv.FormatInt(ctx.OrbitID, 10),
		ctx.Role,
		ctx.Slot,
		strconv.Itoa(int(ctx.Capabilities)),
	}, ":"))
}

func newHistoryCursor() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "hc_" + hex.EncodeToString(raw), nil
}

func loadHistoryCursorTx(tx *sql.Tx, token string, ctx ActorContext, identity Identity, view string, limit int, now int64) (historyKey, historyKey, error) {
	if len(token) != 67 || !strings.HasPrefix(token, "hc_") {
		return historyKey{}, historyKey{}, ErrHistoryCursorInvalid
	}
	var actorID int64
	var authorizationHash, storedView, upperID, lastID string
	var storedLimit int
	var upperAt, lastAt, expiresAt int64
	err := tx.QueryRow(`SELECT actor_id, authorization_hash, view, page_limit,
       upper_at, upper_id, last_at, last_id, expires_at
FROM transmission_history_cursors WHERE token_hash = ?`, hashToken(token)).Scan(
		&actorID, &authorizationHash, &storedView, &storedLimit, &upperAt, &upperID,
		&lastAt, &lastID, &expiresAt)
	if err != nil || actorID != ctx.ActorID || authorizationHash != historyAuthorizationHash(ctx, identity) ||
		storedView != view || storedLimit != limit || expiresAt <= now {
		return historyKey{}, historyKey{}, ErrHistoryCursorInvalid
	}
	return historyKey{at: upperAt, id: upperID}, historyKey{at: lastAt, id: lastID}, nil
}

func historyBlockStateTx(tx *sql.Tx, ctx ActorContext, sourceActorID, sourceOrbitID int64) (actorBlocked, orbitBlocked bool, err error) {
	var actorCount, orbitCount int
	err = tx.QueryRow(`SELECT
  (SELECT COUNT(*) FROM blocks WHERE revoked_at = 0
    AND owner_scope = 'actor' AND owner_orbit_id = ? AND owner_actor_id = ?
    AND blocked_kind = 'actor' AND blocked_actor_id = ?),
  (SELECT COUNT(*) FROM blocks WHERE revoked_at = 0
    AND owner_scope = 'orbit' AND owner_orbit_id = ?
    AND blocked_kind = 'orbit' AND blocked_orbit_id = ?)`,
		ctx.OrbitID, ctx.ActorID, sourceActorID, ctx.OrbitID, sourceOrbitID).Scan(&actorCount, &orbitCount)
	if err != nil {
		return false, false, err
	}
	return actorCount > 0, ctx.Role == "primary" && orbitCount > 0, nil
}

func currentHistoryTargetTx(tx *sql.Tx, ctx ActorContext, target TransmissionTarget) (bool, error) {
	if target.ActorID != ctx.ActorID {
		return false, nil
	}
	return targetMatchesCurrentBindingTx(tx, target)
}

func historyTransmissionItemTx(tx *sql.Tx, ctx ActorContext, id string, now int64) (HistoryQueryItem, bool, error) {
	creation, err := loadTransmissionCreationTx(tx, id)
	if err != nil {
		return HistoryQueryItem{}, false, err
	}
	t := creation.Transmission
	canUseHistoryControls := ctx.Capabilities.Has(CapabilityControl) || ctx.Capabilities.Has(CapabilityTelegram)
	sent := ctx.OrbitID == t.SourceOrbitID && canUseHistoryControls
	received := false
	visible := make([]TransmissionTarget, 0, len(creation.Targets))
	showAll := sent && (ctx.ActorID == t.SourceActorID || ctx.Role == "primary")
	for _, target := range creation.Targets {
		current, err := currentHistoryTargetTx(tx, ctx, target)
		if err != nil {
			return HistoryQueryItem{}, false, err
		}
		if current {
			received = true
		}
		if showAll || current {
			visible = append(visible, target)
		}
	}
	if !sent && !received {
		return HistoryQueryItem{}, false, nil
	}
	media, err := scanMediaItem(tx.QueryRow(`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, t.MediaID))
	if err != nil {
		return HistoryQueryItem{}, false, err
	}
	item := HistoryQueryItem{HistoryItemID: historyID("transmission", t.ID), ItemKind: "transmission",
		OccurredAt: t.AcceptedAt, Direction: historyDirection(sent, received), Media: media,
		Transmission: &t, Targets: visible, TargetCount: len(creation.Targets),
		TargetStatusCounts: map[TransmissionTargetStatus]int{}, SourceActorID: t.SourceActorID,
		SourceOrbitID: t.SourceOrbitID, CanCancel: showAll && ctx.Capabilities.Has(CapabilityControl) && senderCancellationAllowed(t, creation.Targets),
		CanDelete: canUseHistoryControls && media.DeletedAt == 0 && media.ExpiresAt > now &&
			media.Status != MediaStatusDeleted && media.Status != MediaStatusExpired &&
			(ctx.ActorID == media.ActorID || (ctx.Role == "primary" && ctx.OrbitID == media.OwnerOrbitID)),
		CanReport: canUseHistoryControls && received && media.ActorID != ctx.ActorID,
	}
	item.CanReplay = item.CanDelete && media.Status == MediaStatusReady && media.DeletedAt == 0 && media.ExpiresAt > now
	if received && t.SourceActorID != ctx.ActorID {
		actorBlocked, orbitBlocked, err := historyBlockStateTx(tx, ctx, t.SourceActorID, t.SourceOrbitID)
		if err != nil {
			return HistoryQueryItem{}, false, err
		}
		item.CanBlockActor = canUseHistoryControls && !actorBlocked
		item.CanBlockOrbit = canUseHistoryControls && ctx.Role == "primary" &&
			t.SourceOrbitID != ctx.OrbitID && !orbitBlocked
		item.CanUnblock = actorBlocked || orbitBlocked
		item.RevealBlockedReason = item.CanUnblock
	}
	for _, target := range creation.Targets {
		item.TargetStatusCounts[target.Status]++
	}
	if err := tx.QueryRow(`SELECT display_name FROM actors WHERE id = ?`, t.SourceActorID).Scan(&item.SourceActorName); err != nil {
		return HistoryQueryItem{}, false, err
	}
	if err := tx.QueryRow(`SELECT title FROM orbits WHERE id = ?`, t.SourceOrbitID).Scan(&item.SourceOrbitName); err != nil {
		return HistoryQueryItem{}, false, err
	}
	return item, true, nil
}

func historyMediaItemTx(tx *sql.Tx, ctx ActorContext, id string, now int64) (HistoryQueryItem, bool, error) {
	media, err := scanMediaItem(tx.QueryRow(`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return HistoryQueryItem{}, false, nil
	}
	if err != nil {
		return HistoryQueryItem{}, false, err
	}
	if media.Status != MediaStatusProcessing && media.Status != MediaStatusReady && media.Status != MediaStatusFailed {
		return HistoryQueryItem{}, false, nil
	}
	canUseHistoryControls := ctx.Capabilities.Has(CapabilityControl) || ctx.Capabilities.Has(CapabilityTelegram)
	owner := canUseHistoryControls && (ctx.ActorID == media.ActorID ||
		(ctx.Role == "primary" && ctx.OrbitID == media.OwnerOrbitID))
	if !owner {
		return HistoryQueryItem{}, false, nil
	}
	canContent := media.Status == MediaStatusReady && media.DeletedAt == 0 && media.ExpiresAt > now
	return HistoryQueryItem{HistoryItemID: historyID("media", media.ID), ItemKind: "media",
		OccurredAt: media.CreatedAt, Direction: HistorySent, Media: media,
		CanDelete: canUseHistoryControls && media.DeletedAt == 0 && media.ExpiresAt > now &&
			media.Status != MediaStatusDeleted && media.Status != MediaStatusExpired,
		CanReplay: canUseHistoryControls && canContent}, true, nil
}

func (s *Store) QueryAuthorizedHistory(expectedActorID int64, identity Identity, view string, limit int, cursor string, now int64) (HistoryPage, error) {
	if expectedActorID <= 0 || now <= 0 || limit < 1 || limit > 100 || (view != "all" && view != "sent" && view != "received") {
		return HistoryPage{}, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return HistoryPage{}, err
	}
	defer tx.Rollback()
	ctx, err := resolveActorContext(tx, identity)
	if err != nil || ctx.ActorID != expectedActorID {
		if err == nil {
			err = ErrUnauthorized
		}
		return HistoryPage{}, err
	}
	cutoff := now - int64((30*24*time.Hour)/time.Millisecond)
	rows, err := tx.Query(`SELECT 'transmission', id, accepted_at FROM transmissions
WHERE accepted_at >= ? AND (source_orbit_id = ? OR EXISTS(
  SELECT 1 FROM transmission_targets tt WHERE tt.transmission_id = transmissions.id AND tt.actor_id = ?))
UNION ALL
SELECT 'media', m.id, m.created_at FROM media_items m
WHERE (m.actor_id = ? OR (? = 'primary' AND m.owner_orbit_id = ?))
  AND NOT EXISTS(SELECT 1 FROM transmissions t WHERE t.media_id = m.id)`,
		cutoff, ctx.OrbitID, ctx.ActorID, ctx.ActorID, ctx.Role, ctx.OrbitID)
	if err != nil {
		return HistoryPage{}, err
	}
	var keys []historyKey
	for rows.Next() {
		var k historyKey
		if err := rows.Scan(&k.kind, &k.rawID, &k.at); err != nil {
			rows.Close()
			return HistoryPage{}, err
		}
		k.id = historyID(k.kind, k.rawID)
		keys = append(keys, k)
	}
	if err := rows.Close(); err != nil {
		return HistoryPage{}, err
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].at == keys[j].at {
			return keys[i].id > keys[j].id
		}
		return keys[i].at > keys[j].at
	})
	upper := historyKey{at: now, id: "hi_ZZZZZZZZZZZZZZZZZZZZZZZZZZ"}
	before := historyKey{}
	if cursor != "" {
		upper, before, err = loadHistoryCursorTx(tx, cursor, ctx, identity, view, limit, now)
		if err != nil {
			return HistoryPage{}, err
		}
	}
	page := HistoryPage{Items: []HistoryQueryItem{}}
	var pageKeys []historyKey
	for _, key := range keys {
		if key.at > upper.at || (key.at == upper.at && key.id > upper.id) {
			continue
		}
		if before.at > 0 && (key.at > before.at || (key.at == before.at && key.id >= before.id)) {
			continue
		}
		var item HistoryQueryItem
		var ok bool
		if key.kind == "transmission" {
			item, ok, err = historyTransmissionItemTx(tx, ctx, key.rawID, now)
		} else {
			item, ok, err = historyMediaItemTx(tx, ctx, key.rawID, now)
		}
		if err != nil {
			return HistoryPage{}, err
		}
		if !ok || !historyViewMatches(view, item.Direction) {
			continue
		}
		if len(page.Items) == limit {
			pageKeys = append(pageKeys, key)
			break
		}
		page.Items = append(page.Items, item)
		pageKeys = append(pageKeys, key)
	}
	if len(pageKeys) > len(page.Items) && len(page.Items) > 0 {
		if cursor == "" {
			upper = pageKeys[0]
		}
		last := pageKeys[len(page.Items)-1]
		token, err := newHistoryCursor()
		if err != nil {
			return HistoryPage{}, err
		}
		// Keep the stateful capability set bounded per actor. A cursor is still
		// reusable for deterministic reads until it expires or falls outside the
		// most recent 128 pages issued for that actor.
		if _, err := tx.Exec(`DELETE FROM transmission_history_cursors WHERE expires_at <= ?`, now); err != nil {
			return HistoryPage{}, err
		}
		if _, err := tx.Exec(`DELETE FROM transmission_history_cursors WHERE token_hash IN (
  SELECT token_hash FROM transmission_history_cursors WHERE actor_id = ?
  ORDER BY created_at DESC, token_hash DESC LIMIT -1 OFFSET 127
)`, ctx.ActorID); err != nil {
			return HistoryPage{}, err
		}
		_, err = tx.Exec(`INSERT INTO transmission_history_cursors(
  token_hash, actor_id, authorization_hash, view, page_limit, upper_at, upper_id,
  last_at, last_id, expires_at, created_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			hashToken(token), ctx.ActorID, historyAuthorizationHash(ctx, identity), view, limit,
			upper.at, upper.id, last.at, last.id, now+int64((24*time.Hour)/time.Millisecond), now)
		if err != nil {
			return HistoryPage{}, err
		}
		page.NextCursor = token
	}
	if err := tx.Commit(); err != nil {
		return HistoryPage{}, err
	}
	return page, nil
}

func (s *Store) GetAuthorizedHistoryItem(expectedActorID int64, identity Identity, historyItemID string, now int64) (HistoryQueryItem, error) {
	if len(historyItemID) != 29 || !strings.HasPrefix(historyItemID, "hi_") ||
		!transmissionIDPattern.MatchString("tr_"+strings.TrimPrefix(historyItemID, "hi_")) {
		return HistoryQueryItem{}, ErrTransmissionNotFound
	}
	tx, err := s.db.Begin()
	if err != nil {
		return HistoryQueryItem{}, err
	}
	defer tx.Rollback()
	ctx, err := resolveActorContext(tx, identity)
	if err != nil || ctx.ActorID != expectedActorID {
		if err == nil {
			err = ErrUnauthorized
		}
		return HistoryQueryItem{}, err
	}
	suffix := strings.TrimPrefix(historyItemID, "hi_")
	var matches int
	if err := tx.QueryRow(`SELECT (SELECT COUNT(*) FROM transmissions WHERE id = ?) +
  (SELECT COUNT(*) FROM media_items m WHERE m.id = ? AND NOT EXISTS(SELECT 1 FROM transmissions t WHERE t.media_id = m.id))`, "tr_"+suffix, "m_"+suffix).Scan(&matches); err != nil {
		return HistoryQueryItem{}, err
	}
	if matches != 1 {
		return HistoryQueryItem{}, ErrTransmissionNotFound
	}
	var item HistoryQueryItem
	var ok bool
	t, visible, transmissionErr := historyTransmissionItemTx(tx, ctx, "tr_"+suffix, now)
	if transmissionErr == nil && visible {
		if t.OccurredAt < now-int64((30*24*time.Hour)/time.Millisecond) {
			return HistoryQueryItem{}, ErrTransmissionNotFound
		}
		item, ok = t, true
	} else if transmissionErr != nil && !errors.Is(transmissionErr, ErrTransmissionNotFound) {
		return HistoryQueryItem{}, transmissionErr
	}
	if !ok {
		item, ok, err = historyMediaItemTx(tx, ctx, "m_"+suffix, now)
		if err != nil {
			return HistoryQueryItem{}, err
		}
	}
	if !ok {
		return HistoryQueryItem{}, ErrTransmissionNotFound
	}
	if err := tx.Commit(); err != nil {
		return HistoryQueryItem{}, err
	}
	return item, nil
}
