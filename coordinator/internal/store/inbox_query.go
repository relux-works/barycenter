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

var (
	ErrInboxCursorExpired   = errors.New("inbox cursor is invalid or expired")
	ErrReceiptCursorExpired = errors.New("receipt cursor is invalid or expired")
)

const inboxCursorTTL = 24 * time.Hour

// AuthorizedTransmissionInboxItem is the transport-neutral safe projection.
// Internal identifiers stay available only inside Item so policy code can
// reauthorize commands; HTTP projections must use HistoryItemID and labels.
type AuthorizedTransmissionInboxItem struct {
	Item            TransmissionInboxItem
	HistoryItemID   string
	MediaTitle      string
	DurationMS      int64
	SourceName      string
	SourceOrbitName string
	CanReplay       bool
	CanDismiss      bool
	CanReport       bool
	CanBlockActor   bool
	CanBlockOrbit   bool
	CanUnblock      bool
}

type AuthorizedTransmissionInboxPage struct {
	Items      []AuthorizedTransmissionInboxItem
	NextCursor string
}

type AuthorizedHistoryReceipt struct {
	Target       TransmissionTarget
	DisplayLabel string
	RevealReason bool
}

type AuthorizedHistoryReceiptPage struct {
	Items      []AuthorizedHistoryReceipt
	NextCursor string
}

type CreateAuthorizedInboxReplayParams struct {
	ExpectedActorID    int64
	Identity           Identity
	InboxID            string
	IdempotencyKeyHash string
	RequestHash        string
	RequestedDelivery  TransmissionDelivery
	AcceptedAt         int64
	Availability       []TransmissionTargetAvailability
	Confirmation       *ConfirmTransmissionFallback
	ChallengeTokenHash string
}

func newOpaquePageCursor(prefix string) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw), nil
}

func inboxAuthorizationHash(ctx ActorContext, identity Identity, pairedAt int64) string {
	return hashToken(strings.Join([]string{
		historyAuthorizationHash(ctx, identity), strconv.FormatInt(pairedAt, 10),
	}, ":"))
}

func authorizeInboxActorTx(
	tx *sql.Tx,
	expectedActorID int64,
	identity Identity,
) (ActorContext, int64, error) {
	ctx, err := resolveActorContext(tx, identity)
	if err != nil || ctx.ActorID != expectedActorID {
		if err == nil {
			err = ErrUnauthorized
		}
		return ActorContext{}, 0, err
	}
	if !ctx.Capabilities.Has(CapabilityControl) || !ctx.Capabilities.Has(CapabilityNode) {
		return ActorContext{}, 0, ErrInsufficientCapability
	}
	pairedAt, err := currentInboxBindingTx(tx, ctx)
	return ctx, pairedAt, err
}

func loadAuthorizedInboxItemTx(
	tx *sql.Tx,
	ctx ActorContext,
	pairedAt int64,
	inboxID string,
	now int64,
) (TransmissionInboxItem, error) {
	if !transmissionInboxIDPattern.MatchString(inboxID) || now <= 0 {
		return TransmissionInboxItem{}, ErrTransmissionInboxNotFound
	}
	item, err := scanTransmissionInboxItem(tx.QueryRow(
		`SELECT `+transmissionInboxColumns+` FROM transmission_inbox_items
WHERE id = ? AND actor_id = ? AND orbit_id = ? AND slot = ?
  AND binding_paired_at = ?`, inboxID, ctx.ActorID, ctx.OrbitID, ctx.Slot, pairedAt,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return TransmissionInboxItem{}, ErrTransmissionInboxNotFound
	}
	if err != nil {
		return TransmissionInboxItem{}, err
	}
	if item.ExpiresAt <= now || item.Availability == TransmissionInboxExpired ||
		item.Availability == TransmissionInboxUnavailable {
		return TransmissionInboxItem{}, ErrTransmissionInboxNotFound
	}
	return item, nil
}

func authorizedInboxProjectionTx(
	tx *sql.Tx,
	ctx ActorContext,
	item TransmissionInboxItem,
	now int64,
) (AuthorizedTransmissionInboxItem, error) {
	view := AuthorizedTransmissionInboxItem{
		Item: item, HistoryItemID: historyID("transmission", item.TransmissionID),
		CanReplay:  item.Availability == TransmissionInboxAvailable && item.ExpiresAt > now,
		CanDismiss: item.Availability == TransmissionInboxAvailable && item.ExpiresAt > now,
	}
	if err := tx.QueryRow(`SELECT mi.title, mi.duration_ms, a.display_name, o.title
FROM transmissions tr
JOIN media_items mi ON mi.id = tr.media_id
JOIN actors a ON a.id = tr.source_actor_id
JOIN orbits o ON o.id = tr.source_orbit_id
WHERE tr.id = ?`, item.TransmissionID).Scan(
		&view.MediaTitle, &view.DurationMS, &view.SourceName, &view.SourceOrbitName,
	); err != nil {
		return AuthorizedTransmissionInboxItem{}, err
	}
	history, visible, err := historyTransmissionItemTx(tx, ctx, item.TransmissionID, now)
	if err != nil {
		return AuthorizedTransmissionInboxItem{}, err
	}
	if !visible {
		return AuthorizedTransmissionInboxItem{}, ErrTransmissionInboxNotFound
	}
	view.CanReport = history.CanReport
	view.CanBlockActor = history.CanBlockActor
	view.CanBlockOrbit = history.CanBlockOrbit
	view.CanUnblock = history.CanUnblock
	return view, nil
}

func loadInboxCursorTx(
	tx *sql.Tx,
	token string,
	ctx ActorContext,
	identity Identity,
	pairedAt int64,
	view string,
	limit int,
	now int64,
) (TransmissionInboxPageKey, TransmissionInboxPageKey, error) {
	if len(token) != 67 || !strings.HasPrefix(token, "ic_") {
		return TransmissionInboxPageKey{}, TransmissionInboxPageKey{}, ErrInboxCursorExpired
	}
	var actorID, storedPairedAt int64
	var authorizationHash, storedView, upperID, lastID string
	var storedLimit int
	var upperAt, lastAt, expiresAt int64
	err := tx.QueryRow(`SELECT actor_id, authorization_hash, binding_paired_at,
       view, page_limit, upper_at, upper_id, last_at, last_id, expires_at
FROM transmission_inbox_cursors WHERE token_hash = ?`, hashToken(token)).Scan(
		&actorID, &authorizationHash, &storedPairedAt, &storedView, &storedLimit,
		&upperAt, &upperID, &lastAt, &lastID, &expiresAt,
	)
	if err != nil || actorID != ctx.ActorID || storedPairedAt != pairedAt ||
		authorizationHash != inboxAuthorizationHash(ctx, identity, pairedAt) ||
		storedView != view || storedLimit != limit || expiresAt <= now {
		return TransmissionInboxPageKey{}, TransmissionInboxPageKey{}, ErrInboxCursorExpired
	}
	return TransmissionInboxPageKey{CreatedAt: upperAt, ID: upperID},
		TransmissionInboxPageKey{CreatedAt: lastAt, ID: lastID}, nil
}

// QueryAuthorizedTransmissionInbox freezes the first-page upper bound and
// binds every subsequent page to the exact credential and installation
// generation. A later membership or replacement binding cannot expand it.
func (s *Store) QueryAuthorizedTransmissionInbox(
	expectedActorID int64,
	identity Identity,
	view string,
	limit int,
	cursor string,
	now int64,
) (AuthorizedTransmissionInboxPage, error) {
	viewClause, ok := inboxViewSQL(view)
	if expectedActorID <= 0 || !ok || limit < 1 || limit > 100 || now <= 0 {
		return AuthorizedTransmissionInboxPage{}, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AuthorizedTransmissionInboxPage{}, err
	}
	defer tx.Rollback()
	ctx, pairedAt, err := authorizeInboxActorTx(tx, expectedActorID, identity)
	if err != nil {
		return AuthorizedTransmissionInboxPage{}, err
	}
	if _, err := tx.Exec(`UPDATE transmission_inbox_items
SET availability = 'expired', revision = revision + 1, updated_at = ?
WHERE actor_id = ? AND orbit_id = ? AND slot = ? AND binding_paired_at = ?
  AND expires_at <= ? AND availability NOT IN ('expired', 'unavailable')`,
		now, ctx.ActorID, ctx.OrbitID, ctx.Slot, pairedAt, now); err != nil {
		return AuthorizedTransmissionInboxPage{}, err
	}
	upper, after := TransmissionInboxPageKey{}, TransmissionInboxPageKey{}
	if cursor != "" {
		upper, after, err = loadInboxCursorTx(
			tx, cursor, ctx, identity, pairedAt, view, limit, now,
		)
		if err != nil {
			return AuthorizedTransmissionInboxPage{}, err
		}
	}
	if upper.CreatedAt == 0 {
		err = tx.QueryRow(`SELECT created_at, id FROM transmission_inbox_items
WHERE actor_id = ? AND orbit_id = ? AND slot = ? AND binding_paired_at = ?`+
			viewClause+` ORDER BY created_at DESC, id DESC LIMIT 1`,
			ctx.ActorID, ctx.OrbitID, ctx.Slot, pairedAt,
		).Scan(&upper.CreatedAt, &upper.ID)
		if errors.Is(err, sql.ErrNoRows) {
			if err := tx.Commit(); err != nil {
				return AuthorizedTransmissionInboxPage{}, err
			}
			return AuthorizedTransmissionInboxPage{Items: []AuthorizedTransmissionInboxItem{}}, nil
		}
		if err != nil {
			return AuthorizedTransmissionInboxPage{}, err
		}
	}
	query := `SELECT ` + transmissionInboxColumns + ` FROM transmission_inbox_items
WHERE actor_id = ? AND orbit_id = ? AND slot = ? AND binding_paired_at = ?
  AND (created_at < ? OR (created_at = ? AND id <= ?))` + viewClause
	args := []any{ctx.ActorID, ctx.OrbitID, ctx.Slot, pairedAt,
		upper.CreatedAt, upper.CreatedAt, upper.ID}
	if after.CreatedAt > 0 {
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, after.CreatedAt, after.CreatedAt, after.ID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := tx.Query(query, args...)
	if err != nil {
		return AuthorizedTransmissionInboxPage{}, err
	}
	var raw []TransmissionInboxItem
	for rows.Next() {
		item, scanErr := scanTransmissionInboxItem(rows)
		if scanErr != nil {
			rows.Close()
			return AuthorizedTransmissionInboxPage{}, scanErr
		}
		raw = append(raw, item)
	}
	if err := rows.Close(); err != nil {
		return AuthorizedTransmissionInboxPage{}, err
	}
	page := AuthorizedTransmissionInboxPage{Items: []AuthorizedTransmissionInboxItem{}}
	hasMore := len(raw) > limit
	if hasMore {
		raw = raw[:limit]
	}
	for _, item := range raw {
		projection, err := authorizedInboxProjectionTx(tx, ctx, item, now)
		if err != nil {
			return AuthorizedTransmissionInboxPage{}, err
		}
		page.Items = append(page.Items, projection)
	}
	if hasMore && len(raw) > 0 {
		last := raw[len(raw)-1]
		token, err := newOpaquePageCursor("ic_")
		if err != nil {
			return AuthorizedTransmissionInboxPage{}, err
		}
		if err := pruneOpaqueCursorsTx(tx, "transmission_inbox_cursors", ctx.ActorID, now); err != nil {
			return AuthorizedTransmissionInboxPage{}, err
		}
		_, err = tx.Exec(`INSERT INTO transmission_inbox_cursors(
  token_hash, actor_id, authorization_hash, binding_paired_at, view, page_limit,
  upper_at, upper_id, last_at, last_id, expires_at, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, hashToken(token), ctx.ActorID,
			inboxAuthorizationHash(ctx, identity, pairedAt), pairedAt, view, limit,
			upper.CreatedAt, upper.ID, last.CreatedAt, last.ID,
			now+inboxCursorTTL.Milliseconds(), now)
		if err != nil {
			return AuthorizedTransmissionInboxPage{}, err
		}
		page.NextCursor = token
	}
	if err := tx.Commit(); err != nil {
		return AuthorizedTransmissionInboxPage{}, err
	}
	return page, nil
}

func pruneOpaqueCursorsTx(tx *sql.Tx, table string, actorID, now int64) error {
	if table != "transmission_inbox_cursors" && table != "transmission_receipt_cursors" {
		return ErrTransmissionInvalid
	}
	if _, err := tx.Exec(`DELETE FROM `+table+` WHERE expires_at <= ?`, now); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM `+table+` WHERE token_hash IN (
  SELECT token_hash FROM `+table+` WHERE actor_id = ?
  ORDER BY created_at DESC, token_hash DESC LIMIT -1 OFFSET 127
)`, actorID)
	return err
}

func (s *Store) GetAuthorizedTransmissionInboxItem(
	expectedActorID int64,
	identity Identity,
	inboxID string,
	now int64,
) (AuthorizedTransmissionInboxItem, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return AuthorizedTransmissionInboxItem{}, err
	}
	defer tx.Rollback()
	ctx, pairedAt, err := authorizeInboxActorTx(tx, expectedActorID, identity)
	if err != nil {
		return AuthorizedTransmissionInboxItem{}, err
	}
	item, err := loadAuthorizedInboxItemTx(tx, ctx, pairedAt, inboxID, now)
	if err != nil {
		return AuthorizedTransmissionInboxItem{}, err
	}
	view, err := authorizedInboxProjectionTx(tx, ctx, item, now)
	if err != nil {
		return AuthorizedTransmissionInboxItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return AuthorizedTransmissionInboxItem{}, err
	}
	return view, nil
}

// DismissAuthorizedTransmissionInboxItem is locally idempotent. It never
// deletes media or mutates any other target or scheduler state.
func (s *Store) DismissAuthorizedTransmissionInboxItem(
	expectedActorID int64,
	identity Identity,
	inboxID string,
	now int64,
) (AuthorizedTransmissionInboxItem, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return AuthorizedTransmissionInboxItem{}, err
	}
	defer tx.Rollback()
	ctx, pairedAt, err := authorizeInboxActorTx(tx, expectedActorID, identity)
	if err != nil {
		return AuthorizedTransmissionInboxItem{}, err
	}
	item, err := loadAuthorizedInboxItemTx(tx, ctx, pairedAt, inboxID, now)
	if err != nil {
		return AuthorizedTransmissionInboxItem{}, err
	}
	if item.Availability != TransmissionInboxAvailable && item.Availability != TransmissionInboxDismissed {
		return AuthorizedTransmissionInboxItem{}, ErrTransmissionInboxNotFound
	}
	if item.Availability == TransmissionInboxAvailable {
		result, err := tx.Exec(`UPDATE transmission_inbox_items
SET availability = 'dismissed', dismissed_at = ?, revision = revision + 1,
    updated_at = ? WHERE id = ? AND revision = ? AND availability = 'available'`,
			now, now, item.ID, item.Revision)
		if err != nil {
			return AuthorizedTransmissionInboxItem{}, err
		}
		if changed, err := result.RowsAffected(); err != nil || changed != 1 {
			if err != nil {
				return AuthorizedTransmissionInboxItem{}, err
			}
			return AuthorizedTransmissionInboxItem{}, ErrTransmissionInboxConflict
		}
		item, err = scanTransmissionInboxItem(tx.QueryRow(
			`SELECT `+transmissionInboxColumns+` FROM transmission_inbox_items WHERE id = ?`, item.ID,
		))
		if err != nil {
			return AuthorizedTransmissionInboxItem{}, err
		}
	}
	view, err := authorizedInboxProjectionTx(tx, ctx, item, now)
	if err != nil {
		return AuthorizedTransmissionInboxItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return AuthorizedTransmissionInboxItem{}, err
	}
	return view, nil
}

// CreateAuthorizedInboxReplay accepts no media or audience identifier from
// the caller. The repository resolves both from the exact current inbox
// binding and atomically consumes the item only after a new transmission and
// replay lineage are durable. Idempotent retries return that same creation.
func (s *Store) CreateAuthorizedInboxReplay(
	params CreateAuthorizedInboxReplayParams,
) (ResolvedTransmissionCreation, error) {
	if params.ExpectedActorID <= 0 ||
		!transmissionInboxIDPattern.MatchString(params.InboxID) ||
		!transmissionDigestPattern.MatchString(params.IdempotencyKeyHash) ||
		!transmissionDigestPattern.MatchString(params.RequestHash) ||
		params.AcceptedAt <= 0 || !validTransmissionDelivery(params.RequestedDelivery) {
		return ResolvedTransmissionCreation{}, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	defer tx.Rollback()
	ctx, pairedAt, err := authorizeInboxActorTx(
		tx, params.ExpectedActorID, params.Identity,
	)
	if err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	reused, err := transmissionIdempotentReplayTx(
		tx, ctx.ActorID, params.IdempotencyKeyHash, params.RequestHash,
	)
	if err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	if reused != nil {
		if err := tx.Commit(); err != nil {
			return ResolvedTransmissionCreation{}, err
		}
		return ResolvedTransmissionCreation{Creation: *reused, Reused: true}, nil
	}
	inbox, err := loadAuthorizedInboxItemTx(
		tx, ctx, pairedAt, params.InboxID, params.AcceptedAt,
	)
	if err != nil || inbox.Availability != TransmissionInboxAvailable {
		return ResolvedTransmissionCreation{}, ErrTransmissionInboxNotFound
	}
	if inbox.ReplayDepth >= 8 {
		return ResolvedTransmissionCreation{}, ErrTransmissionReplayDepthExceeded
	}
	create := CreateResolvedTransmissionParams{
		ExpectedActorID: params.ExpectedActorID, Identity: params.Identity,
		IdempotencyKeyHash: params.IdempotencyKeyHash, RequestHash: params.RequestHash,
		MediaID: inbox.MediaID, AudienceKind: TransmissionAudienceThisPulsar,
		OriginKind: TransmissionOriginFile, IncludeOrigin: true,
		RequestedDelivery: params.RequestedDelivery, AcceptedAt: params.AcceptedAt,
		Availability: params.Availability, Confirmation: params.Confirmation,
		ChallengeTokenHash: params.ChallengeTokenHash, ReplayInboxID: inbox.ID,
	}
	if params.Identity.Kind == IdentityBearer {
		create.Bearer = params.Identity.Token
		create.Identity = Identity{}
	}
	if !validResolvedTransmissionParams(create) {
		return ResolvedTransmissionCreation{}, ErrTransmissionInvalid
	}
	result, err := s.createResolvedTransmissionTx(tx, ctx, create)
	if err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	if err := tx.Commit(); err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	return result, nil
}

type receiptCursorKey struct {
	orbitID, actorID, bindingPairedAt int64
	slot                              string
}

func receiptKey(target TransmissionTarget) receiptCursorKey {
	return receiptCursorKey{target.OrbitID, target.ActorID, target.BindingPairedAt, target.Slot}
}

func receiptKeyAfter(left, right receiptCursorKey) bool {
	if left.orbitID != right.orbitID {
		return left.orbitID > right.orbitID
	}
	if left.actorID != right.actorID {
		return left.actorID > right.actorID
	}
	if left.slot != right.slot {
		return left.slot > right.slot
	}
	return left.bindingPairedAt > right.bindingPairedAt
}

func loadReceiptCursorTx(
	tx *sql.Tx,
	token string,
	ctx ActorContext,
	identity Identity,
	historyItemID string,
	limit int,
	now int64,
) (receiptCursorKey, error) {
	if len(token) != 67 || !strings.HasPrefix(token, "rc_") {
		return receiptCursorKey{}, ErrReceiptCursorExpired
	}
	var actorID int64
	var authorizationHash, storedHistoryID, slot string
	var storedLimit int
	var key receiptCursorKey
	var expiresAt int64
	err := tx.QueryRow(`SELECT actor_id, authorization_hash, history_item_id,
       page_limit, last_orbit_id, last_actor_id, last_slot,
       last_binding_paired_at, expires_at
FROM transmission_receipt_cursors WHERE token_hash = ?`, hashToken(token)).Scan(
		&actorID, &authorizationHash, &storedHistoryID, &storedLimit,
		&key.orbitID, &key.actorID, &slot, &key.bindingPairedAt, &expiresAt,
	)
	key.slot = slot
	if err != nil || actorID != ctx.ActorID || authorizationHash != historyAuthorizationHash(ctx, identity) ||
		storedHistoryID != historyItemID || storedLimit != limit || expiresAt <= now {
		return receiptCursorKey{}, ErrReceiptCursorExpired
	}
	return key, nil
}

func (s *Store) QueryAuthorizedHistoryReceipts(
	expectedActorID int64,
	identity Identity,
	historyItemID string,
	limit int,
	cursor string,
	now int64,
) (AuthorizedHistoryReceiptPage, error) {
	if expectedActorID <= 0 || limit < 1 || limit > 100 || now <= 0 ||
		len(historyItemID) != 29 || !strings.HasPrefix(historyItemID, "hi_") {
		return AuthorizedHistoryReceiptPage{}, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AuthorizedHistoryReceiptPage{}, err
	}
	defer tx.Rollback()
	ctx, err := resolveActorContext(tx, identity)
	if err != nil || ctx.ActorID != expectedActorID {
		if err == nil {
			err = ErrUnauthorized
		}
		return AuthorizedHistoryReceiptPage{}, err
	}
	transmissionID := "tr_" + strings.TrimPrefix(historyItemID, "hi_")
	item, visible, err := historyTransmissionItemTx(tx, ctx, transmissionID, now)
	if err != nil || !visible || item.OccurredAt < now-int64((30*24*time.Hour)/time.Millisecond) {
		if err == nil {
			err = ErrTransmissionNotFound
		}
		return AuthorizedHistoryReceiptPage{}, err
	}
	last := receiptCursorKey{}
	if cursor != "" {
		last, err = loadReceiptCursorTx(tx, cursor, ctx, identity, historyItemID, limit, now)
		if err != nil {
			return AuthorizedHistoryReceiptPage{}, err
		}
	}
	sort.Slice(item.Targets, func(i, j int) bool {
		return receiptKeyAfter(receiptKey(item.Targets[j]), receiptKey(item.Targets[i]))
	})
	visibleTargets := make([]TransmissionTarget, 0, len(item.Targets))
	for _, target := range item.Targets {
		if last.orbitID != 0 && !receiptKeyAfter(receiptKey(target), last) {
			continue
		}
		visibleTargets = append(visibleTargets, target)
	}
	hasMore := len(visibleTargets) > limit
	if hasMore {
		visibleTargets = visibleTargets[:limit]
	}
	page := AuthorizedHistoryReceiptPage{Items: []AuthorizedHistoryReceipt{}}
	for _, target := range visibleTargets {
		var actorName, orbitName string
		if err := tx.QueryRow(`SELECT a.display_name, o.title FROM actors a, orbits o
WHERE a.id = ? AND o.id = ?`, target.ActorID, target.OrbitID).Scan(&actorName, &orbitName); err != nil {
			return AuthorizedHistoryReceiptPage{}, err
		}
		page.Items = append(page.Items, AuthorizedHistoryReceipt{
			Target: target, DisplayLabel: actorName + " · " + orbitName,
			RevealReason: target.ReasonCode != TransmissionReasonReported &&
				(item.RevealBlockedReason || target.Status != TransmissionTargetBlocked),
		})
	}
	if hasMore && len(visibleTargets) > 0 {
		token, err := newOpaquePageCursor("rc_")
		if err != nil {
			return AuthorizedHistoryReceiptPage{}, err
		}
		if err := pruneOpaqueCursorsTx(tx, "transmission_receipt_cursors", ctx.ActorID, now); err != nil {
			return AuthorizedHistoryReceiptPage{}, err
		}
		key := receiptKey(visibleTargets[len(visibleTargets)-1])
		_, err = tx.Exec(`INSERT INTO transmission_receipt_cursors(
  token_hash, actor_id, authorization_hash, history_item_id, page_limit,
  last_orbit_id, last_actor_id, last_slot, last_binding_paired_at,
  expires_at, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, hashToken(token), ctx.ActorID,
			historyAuthorizationHash(ctx, identity), historyItemID, limit,
			key.orbitID, key.actorID, key.slot, key.bindingPairedAt,
			now+inboxCursorTTL.Milliseconds(), now)
		if err != nil {
			return AuthorizedHistoryReceiptPage{}, err
		}
		page.NextCursor = token
	}
	if err := tx.Commit(); err != nil {
		return AuthorizedHistoryReceiptPage{}, err
	}
	return page, nil
}
