package store

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/ulid"
)

const (
	SavedCueLifecyclePolicyV1 = "saved_cue_lifecycle_v1"
	SavedCueMaxCount          = 64
	SavedCueMaxTotalBytes     = int64(50 << 20)
	SavedCueMaxItemBytes      = int64(10 << 20)
	SavedCueMaxDurationMS     = int64(60_000)

	BuiltinRecordingCueAssetID  = "pulsar.recording-cue.v1"
	BuiltinRecordingCueSHA256   = "479b1a9d605ac12454e3449e129991b7ce8599251506ca54a93be0b6144730fd"
	BuiltinRecordingCueBytes    = int64(15404)
	BuiltinRecordingCueDuration = int64(160)
)

var (
	ErrSavedCueNotFound      = errors.New("saved cue was not found")
	ErrSavedCueInvalid       = errors.New("saved cue input is invalid")
	ErrSavedCueStateConflict = errors.New("saved cue state changed")
	ErrSavedCueQuotaExceeded = errors.New("saved cue quota exceeded")
	ErrSavedCueDuplicate     = errors.New("saved cue source is already saved")
	savedCueIDPattern        = regexp.MustCompile(`^cq_[0-9A-HJKMNP-TV-Z]{26}$`)
)

type SavedCueSourceKind string

const (
	SavedCueSourceMedia   SavedCueSourceKind = "media"
	SavedCueSourceBuiltin SavedCueSourceKind = "builtin"
)

type SavedCueState string

const (
	SavedCueActive        SavedCueState = "active"
	SavedCueDeleted       SavedCueState = "deleted"
	SavedCueSourceRevoked SavedCueState = "source_revoked"
)

type SavedCue struct {
	ID               string
	OwnerOrbitID     int64
	CreatedByActorID int64
	Title            string
	SourceKind       SavedCueSourceKind
	MediaID          string
	MediaRevision    int64
	BuiltinAssetID   string
	BuiltinSHA256    string
	SourceSHA256     string
	SourceBytes      int64
	SourceDurationMS int64
	State            SavedCueState
	RevokeReason     string
	Revision         int64
	SourceGeneration int64
	CreatedAt        int64
	UpdatedAt        int64
	DeletedAt        int64
}

type SavedCueUsage struct {
	Count int
	Bytes int64
}

type SavedCueMutationParams struct {
	ExpectedActorID int64
	Bearer          string
	Title           string
	MediaID         string
	BuiltinAssetID  string
	BuiltinSHA256   string
	OccurredAt      int64
}

type SavedCueRevocation struct {
	CueID                 string
	InvalidatedGeneration int64
	Reason                string
	PendingAction         string
	ActiveAction          string
	InterruptedMainAction string
	State                 string
	CreatedAt             int64
	UpdatedAt             int64
	CompletedAt           int64
}

const savedCueColumns = `id, owner_orbit_id, created_by_actor_id, title,
source_kind, COALESCE(media_id, ''), media_revision, builtin_asset_id,
builtin_sha256, source_sha256, source_bytes, source_duration_ms, state,
revoke_reason, revision, source_generation, created_at, updated_at, deleted_at`

func scanSavedCue(row interface{ Scan(...any) error }) (SavedCue, error) {
	var cue SavedCue
	err := row.Scan(&cue.ID, &cue.OwnerOrbitID, &cue.CreatedByActorID, &cue.Title,
		&cue.SourceKind, &cue.MediaID, &cue.MediaRevision, &cue.BuiltinAssetID,
		&cue.BuiltinSHA256, &cue.SourceSHA256, &cue.SourceBytes,
		&cue.SourceDurationMS, &cue.State, &cue.RevokeReason, &cue.Revision,
		&cue.SourceGeneration, &cue.CreatedAt, &cue.UpdatedAt, &cue.DeletedAt)
	return cue, err
}

func validSavedCueTitle(title string) (string, bool) {
	title = strings.TrimSpace(title)
	return title, title != "" && len(title) <= 128
}

type resolvedSavedCueSource struct {
	kind       SavedCueSourceKind
	mediaID    string
	mediaRev   int64
	builtinID  string
	builtinSHA string
	sha        string
	bytes      int64
	durationMS int64
}

func resolveSavedCueSourceTx(tx *sql.Tx, orbitID int64, params SavedCueMutationParams) (resolvedSavedCueSource, error) {
	media := params.MediaID != ""
	builtin := params.BuiltinAssetID != "" || params.BuiltinSHA256 != ""
	if media == builtin {
		return resolvedSavedCueSource{}, ErrSavedCueInvalid
	}
	if builtin {
		if params.BuiltinAssetID != BuiltinRecordingCueAssetID ||
			params.BuiltinSHA256 != BuiltinRecordingCueSHA256 {
			return resolvedSavedCueSource{}, ErrSavedCueInvalid
		}
		return resolvedSavedCueSource{
			kind: SavedCueSourceBuiltin, builtinID: BuiltinRecordingCueAssetID,
			builtinSHA: BuiltinRecordingCueSHA256, sha: BuiltinRecordingCueSHA256,
			bytes: BuiltinRecordingCueBytes, durationMS: BuiltinRecordingCueDuration,
		}, nil
	}
	if !mediaItemIDPattern.MatchString(params.MediaID) {
		return resolvedSavedCueSource{}, ErrSavedCueNotFound
	}
	item, err := scanMediaItem(tx.QueryRow(
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ? AND owner_orbit_id = ?`,
		params.MediaID, orbitID))
	if errors.Is(err, sql.ErrNoRows) {
		return resolvedSavedCueSource{}, ErrSavedCueNotFound
	}
	if err != nil {
		return resolvedSavedCueSource{}, err
	}
	if item.Status != MediaStatusReady || item.Kind != MediaKindAudioClip ||
		item.Source != MediaSourceApp || item.StorageKey == "" || item.SHA256 == "" ||
		item.SizeBytes <= 0 || item.SizeBytes > SavedCueMaxItemBytes ||
		item.DurationMS <= 0 || item.DurationMS > SavedCueMaxDurationMS {
		return resolvedSavedCueSource{}, ErrSavedCueInvalid
	}
	if item.ExpiresAt <= params.OccurredAt {
		pinned, err := savedCuePinExistsTx(tx, item.ID)
		if err != nil {
			return resolvedSavedCueSource{}, err
		}
		if !pinned {
			return resolvedSavedCueSource{}, ErrSavedCueInvalid
		}
	}
	var sourceAuthorized int
	if err := tx.QueryRow(`SELECT EXISTS(
  SELECT 1 FROM actors a
  JOIN memberships m ON m.actor_id = a.id
  WHERE a.id = ? AND a.revoked_at IS NULL AND m.orbit_id = ? AND m.left_at IS NULL
)`, item.ActorID, orbitID).Scan(&sourceAuthorized); err != nil {
		return resolvedSavedCueSource{}, err
	}
	if sourceAuthorized == 0 {
		return resolvedSavedCueSource{}, ErrSavedCueInvalid
	}
	return resolvedSavedCueSource{
		kind: SavedCueSourceMedia, mediaID: item.ID, mediaRev: item.Revision,
		sha: item.SHA256, bytes: item.SizeBytes, durationMS: item.DurationMS,
	}, nil
}

func savedCueUsageTx(tx *sql.Tx, orbitID int64, excludingID string) (SavedCueUsage, error) {
	var usage SavedCueUsage
	err := tx.QueryRow(`SELECT COUNT(*), COALESCE(SUM(source_bytes), 0)
FROM saved_cues WHERE owner_orbit_id = ? AND state = 'active' AND id <> ?`,
		orbitID, excludingID).Scan(&usage.Count, &usage.Bytes)
	return usage, err
}

func enforceSavedCueQuota(usage SavedCueUsage, source resolvedSavedCueSource) error {
	if source.bytes > SavedCueMaxItemBytes || source.durationMS > SavedCueMaxDurationMS ||
		usage.Count+1 > SavedCueMaxCount || usage.Bytes+source.bytes > SavedCueMaxTotalBytes {
		return ErrSavedCueQuotaExceeded
	}
	return nil
}

func insertSavedCueAuditTx(tx *sql.Tx, cue SavedCue, actorID int64, event string, now int64) error {
	_, err := tx.Exec(`INSERT INTO saved_cue_audit_events(
  cue_id, owner_orbit_id, actor_id, event_type, source_generation, occurred_at
) VALUES(?, ?, ?, ?, ?, ?)`, cue.ID, cue.OwnerOrbitID, actorID, event,
		cue.SourceGeneration, now)
	return err
}

func existingSavedCueForSourceTx(tx *sql.Tx, orbitID int64, source resolvedSavedCueSource, excludingID string) (*SavedCue, error) {
	query := `SELECT ` + savedCueColumns + ` FROM saved_cues
WHERE owner_orbit_id = ? AND state = 'active' AND id <> ? AND source_kind = ? AND media_id = ?`
	args := []any{orbitID, excludingID, source.kind, source.mediaID}
	if source.kind == SavedCueSourceBuiltin {
		query = `SELECT ` + savedCueColumns + ` FROM saved_cues
WHERE owner_orbit_id = ? AND state = 'active' AND id <> ? AND source_kind = ?
  AND builtin_asset_id = ? AND builtin_sha256 = ?`
		args = []any{orbitID, excludingID, source.kind, source.builtinID, source.builtinSHA}
	}
	cue, err := scanSavedCue(tx.QueryRow(query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &cue, nil
}

func authorizeSavedCueMutationTx(tx *sql.Tx, expectedActorID int64, bearer string) (ActorContext, error) {
	if expectedActorID <= 0 || bearer == "" {
		return ActorContext{}, ErrUnauthorized
	}
	ctx, err := mutationActorContextTx(tx, expectedActorID, hashToken(bearer))
	if err != nil {
		return ActorContext{}, err
	}
	if !ctx.Capabilities.Has(CapabilityControl) || ctx.Role != "primary" {
		return ActorContext{}, ErrInsufficientCapability
	}
	return ctx, nil
}

func (s *Store) CreateSavedCue(params SavedCueMutationParams) (SavedCue, bool, error) {
	title, valid := validSavedCueTitle(params.Title)
	if !valid || params.OccurredAt <= 0 {
		return SavedCue{}, false, ErrSavedCueInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return SavedCue{}, false, err
	}
	defer tx.Rollback()
	ctx, err := authorizeSavedCueMutationTx(tx, params.ExpectedActorID, params.Bearer)
	if err != nil {
		return SavedCue{}, false, err
	}
	if err := requireCurrentContentPolicyTx(tx, ctx); err != nil {
		return SavedCue{}, false, err
	}
	source, err := resolveSavedCueSourceTx(tx, ctx.OrbitID, params)
	if err != nil {
		return SavedCue{}, false, err
	}
	if existing, err := existingSavedCueForSourceTx(tx, ctx.OrbitID, source, ""); err != nil {
		return SavedCue{}, false, err
	} else if existing != nil {
		if err := tx.Commit(); err != nil {
			return SavedCue{}, false, err
		}
		return *existing, true, nil
	}
	usage, err := savedCueUsageTx(tx, ctx.OrbitID, "")
	if err != nil {
		return SavedCue{}, false, err
	}
	if err := enforceSavedCueQuota(usage, source); err != nil {
		return SavedCue{}, false, err
	}
	id := ulid.NewSavedCueID(time.UnixMilli(params.OccurredAt))
	var mediaID any
	if source.mediaID != "" {
		mediaID = source.mediaID
	}
	_, err = tx.Exec(`INSERT INTO saved_cues(
  id, owner_orbit_id, created_by_actor_id, title, source_kind, media_id,
  media_revision, builtin_asset_id, builtin_sha256, source_sha256,
  source_bytes, source_duration_ms, created_at, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, ctx.OrbitID,
		ctx.ActorID, title, source.kind, mediaID, source.mediaRev, source.builtinID,
		source.builtinSHA, source.sha, source.bytes, source.durationMS,
		params.OccurredAt, params.OccurredAt)
	if err != nil {
		return SavedCue{}, false, err
	}
	cue, err := scanSavedCue(tx.QueryRow(`SELECT `+savedCueColumns+` FROM saved_cues WHERE id = ?`, id))
	if err != nil {
		return SavedCue{}, false, err
	}
	if err := insertSavedCueAuditTx(tx, cue, ctx.ActorID, "saved_cue.created", params.OccurredAt); err != nil {
		return SavedCue{}, false, err
	}
	if err := s.checkpoint("saved_cue_create_before_commit"); err != nil {
		return SavedCue{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return SavedCue{}, false, err
	}
	return cue, false, nil
}

func savedCueOwnedTx(tx *sql.Tx, cueID string, orbitID int64) (SavedCue, error) {
	if !savedCueIDPattern.MatchString(cueID) {
		return SavedCue{}, ErrSavedCueNotFound
	}
	cue, err := scanSavedCue(tx.QueryRow(`SELECT `+savedCueColumns+`
FROM saved_cues WHERE id = ? AND owner_orbit_id = ?`, cueID, orbitID))
	if errors.Is(err, sql.ErrNoRows) {
		return SavedCue{}, ErrSavedCueNotFound
	}
	return cue, err
}

func scheduleSavedCueRevocationTx(tx *sql.Tx, cue SavedCue, reason string, now int64) error {
	_, err := tx.Exec(`INSERT INTO saved_cue_revocations(
  cue_id, invalidated_generation, reason, policy_version, pending_action,
  active_action, interrupted_main_action, state, created_at, updated_at
) VALUES(?, ?, ?, ?, 'cancel', 'fade_stop', 'resume_once', 'pending', ?, ?)
ON CONFLICT(cue_id, invalidated_generation) DO NOTHING`, cue.ID,
		cue.SourceGeneration, reason, SavedCueLifecyclePolicyV1, now, now)
	return err
}

func (s *Store) RenameSavedCue(expectedActorID int64, bearer, cueID, title string, expectedRevision, now int64) (SavedCue, error) {
	title, valid := validSavedCueTitle(title)
	if !valid || expectedRevision <= 0 || now <= 0 {
		return SavedCue{}, ErrSavedCueInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return SavedCue{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeSavedCueMutationTx(tx, expectedActorID, bearer)
	if err != nil {
		return SavedCue{}, err
	}
	cue, err := savedCueOwnedTx(tx, cueID, ctx.OrbitID)
	if err != nil {
		return SavedCue{}, err
	}
	if cue.State != SavedCueActive || cue.Revision != expectedRevision {
		return SavedCue{}, ErrSavedCueStateConflict
	}
	if _, err := tx.Exec(`UPDATE saved_cues SET title = ?, revision = revision + 1,
updated_at = ? WHERE id = ? AND revision = ?`, title, now, cue.ID, cue.Revision); err != nil {
		return SavedCue{}, err
	}
	cue, err = scanSavedCue(tx.QueryRow(`SELECT `+savedCueColumns+` FROM saved_cues WHERE id = ?`, cue.ID))
	if err != nil {
		return SavedCue{}, err
	}
	if err := insertSavedCueAuditTx(tx, cue, ctx.ActorID, "saved_cue.renamed", now); err != nil {
		return SavedCue{}, err
	}
	if err := tx.Commit(); err != nil {
		return SavedCue{}, err
	}
	return cue, nil
}

func (s *Store) ReplaceSavedCue(params SavedCueMutationParams, cueID string, expectedRevision int64) (SavedCue, error) {
	if expectedRevision <= 0 || params.OccurredAt <= 0 {
		return SavedCue{}, ErrSavedCueInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return SavedCue{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeSavedCueMutationTx(tx, params.ExpectedActorID, params.Bearer)
	if err != nil {
		return SavedCue{}, err
	}
	if err := requireCurrentContentPolicyTx(tx, ctx); err != nil {
		return SavedCue{}, err
	}
	cue, err := savedCueOwnedTx(tx, cueID, ctx.OrbitID)
	if err != nil {
		return SavedCue{}, err
	}
	if cue.State != SavedCueActive || cue.Revision != expectedRevision {
		return SavedCue{}, ErrSavedCueStateConflict
	}
	source, err := resolveSavedCueSourceTx(tx, ctx.OrbitID, params)
	if err != nil {
		return SavedCue{}, err
	}
	if existing, err := existingSavedCueForSourceTx(tx, ctx.OrbitID, source, cue.ID); err != nil {
		return SavedCue{}, err
	} else if existing != nil {
		return SavedCue{}, ErrSavedCueDuplicate
	}
	if cue.SourceKind == source.kind && cue.MediaID == source.mediaID &&
		cue.BuiltinAssetID == source.builtinID && cue.BuiltinSHA256 == source.builtinSHA {
		return cue, tx.Commit()
	}
	usage, err := savedCueUsageTx(tx, ctx.OrbitID, cue.ID)
	if err != nil {
		return SavedCue{}, err
	}
	if err := enforceSavedCueQuota(usage, source); err != nil {
		return SavedCue{}, err
	}
	if err := scheduleSavedCueRevocationTx(tx, cue, "cue_replaced", params.OccurredAt); err != nil {
		return SavedCue{}, err
	}
	oldMediaID := cue.MediaID
	var mediaID any
	if source.mediaID != "" {
		mediaID = source.mediaID
	}
	_, err = tx.Exec(`UPDATE saved_cues SET source_kind = ?, media_id = ?,
media_revision = ?, builtin_asset_id = ?, builtin_sha256 = ?, source_sha256 = ?,
source_bytes = ?, source_duration_ms = ?, revision = revision + 1,
source_generation = source_generation + 1, updated_at = ?
WHERE id = ? AND revision = ?`, source.kind, mediaID, source.mediaRev,
		source.builtinID, source.builtinSHA, source.sha, source.bytes,
		source.durationMS, params.OccurredAt, cue.ID, cue.Revision)
	if err != nil {
		return SavedCue{}, err
	}
	cue, err = scanSavedCue(tx.QueryRow(`SELECT `+savedCueColumns+` FROM saved_cues WHERE id = ?`, cue.ID))
	if err != nil {
		return SavedCue{}, err
	}
	if err := insertSavedCueAuditTx(tx, cue, ctx.ActorID, "saved_cue.replaced", params.OccurredAt); err != nil {
		return SavedCue{}, err
	}
	if err := expireUnpinnedSavedCueMediaTx(tx, oldMediaID, params.OccurredAt); err != nil {
		return SavedCue{}, err
	}
	if err := s.checkpoint("saved_cue_replace_before_commit"); err != nil {
		return SavedCue{}, err
	}
	if err := tx.Commit(); err != nil {
		return SavedCue{}, err
	}
	return cue, nil
}

func (s *Store) DeleteSavedCue(expectedActorID int64, bearer, cueID string, expectedRevision, now int64) (SavedCue, error) {
	if expectedRevision <= 0 || now <= 0 {
		return SavedCue{}, ErrSavedCueInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return SavedCue{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeSavedCueMutationTx(tx, expectedActorID, bearer)
	if err != nil {
		return SavedCue{}, err
	}
	cue, err := savedCueOwnedTx(tx, cueID, ctx.OrbitID)
	if err != nil {
		return SavedCue{}, err
	}
	if cue.State == SavedCueDeleted {
		return cue, tx.Commit()
	}
	if cue.State != SavedCueActive || cue.Revision != expectedRevision {
		return SavedCue{}, ErrSavedCueStateConflict
	}
	if err := scheduleSavedCueRevocationTx(tx, cue, "cue_deleted", now); err != nil {
		return SavedCue{}, err
	}
	_, err = tx.Exec(`UPDATE saved_cues SET state = 'deleted', revoke_reason = 'cue_deleted',
revision = revision + 1, source_generation = source_generation + 1,
updated_at = ?, deleted_at = ? WHERE id = ? AND revision = ?`, now, now, cue.ID, cue.Revision)
	if err != nil {
		return SavedCue{}, err
	}
	mediaID := cue.MediaID
	cue, err = scanSavedCue(tx.QueryRow(`SELECT `+savedCueColumns+` FROM saved_cues WHERE id = ?`, cue.ID))
	if err != nil {
		return SavedCue{}, err
	}
	if err := insertSavedCueAuditTx(tx, cue, ctx.ActorID, "saved_cue.deleted", now); err != nil {
		return SavedCue{}, err
	}
	if err := expireUnpinnedSavedCueMediaTx(tx, mediaID, now); err != nil {
		return SavedCue{}, err
	}
	if err := s.checkpoint("saved_cue_delete_before_commit"); err != nil {
		return SavedCue{}, err
	}
	if err := tx.Commit(); err != nil {
		return SavedCue{}, err
	}
	return cue, nil
}

func expireUnpinnedSavedCueMediaTx(tx *sql.Tx, mediaID string, now int64) error {
	if mediaID == "" {
		return nil
	}
	pinned, err := savedCuePinExistsTx(tx, mediaID)
	if err != nil || pinned {
		return err
	}
	item, err := scanMediaItem(tx.QueryRow(`SELECT `+mediaItemColumns+`
FROM media_items WHERE id = ? AND status = 'ready' AND expires_at <= ?`, mediaID, now))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = transitionMediaTerminalTx(tx, item.ID, item.Revision, MediaStatusExpired, "", now, true)
	return err
}

func (s *Store) SavedCueUsage(orbitID int64) (SavedCueUsage, error) {
	if orbitID <= 0 {
		return SavedCueUsage{}, ErrSavedCueInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return SavedCueUsage{}, err
	}
	defer tx.Rollback()
	usage, err := savedCueUsageTx(tx, orbitID, "")
	if err != nil {
		return SavedCueUsage{}, err
	}
	return usage, tx.Commit()
}

func (s *Store) ListSavedCues(orbitID int64) ([]SavedCue, error) {
	rows, err := s.db.Query(`SELECT `+savedCueColumns+`
FROM saved_cues WHERE owner_orbit_id = ? AND state = 'active'
ORDER BY created_at, id`, orbitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cues []SavedCue
	for rows.Next() {
		cue, err := scanSavedCue(rows)
		if err != nil {
			return nil, err
		}
		cues = append(cues, cue)
	}
	return cues, rows.Err()
}

func revokeSavedCueTx(tx *sql.Tx, cue SavedCue, reason string, now int64) error {
	if cue.State != SavedCueActive {
		return nil
	}
	if err := scheduleSavedCueRevocationTx(tx, cue, reason, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE saved_cues SET state = 'source_revoked',
revoke_reason = ?, revision = revision + 1, source_generation = source_generation + 1,
updated_at = ?, deleted_at = ? WHERE id = ? AND state = 'active'`,
		reason, now, now, cue.ID); err != nil {
		return err
	}
	cue.State = SavedCueSourceRevoked
	cue.RevokeReason = reason
	cue.Revision++
	cue.SourceGeneration++
	cue.UpdatedAt = now
	cue.DeletedAt = now
	return insertSavedCueAuditTx(tx, cue, cue.CreatedByActorID, "saved_cue.source_revoked", now)
}

func revokeSavedCuesQueryTx(tx *sql.Tx, query, reason string, now int64, args ...any) error {
	rows, err := tx.Query(`SELECT `+savedCueColumns+` FROM saved_cues WHERE state = 'active' AND (`+query+`)`, args...)
	if err != nil {
		return err
	}
	var cues []SavedCue
	for rows.Next() {
		cue, err := scanSavedCue(rows)
		if err != nil {
			rows.Close()
			return err
		}
		cues = append(cues, cue)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, cue := range cues {
		if err := revokeSavedCueTx(tx, cue, reason, now); err != nil {
			return err
		}
	}
	return nil
}

func revokeSavedCuesForMediaTx(tx *sql.Tx, mediaID, reason string, now int64) error {
	return revokeSavedCuesQueryTx(tx, `media_id = ?`, reason, now, mediaID)
}

func revokeSavedCuesForActorTx(tx *sql.Tx, actorID, now int64) error {
	return revokeSavedCuesQueryTx(tx, `created_by_actor_id = ? OR media_id IN (
  SELECT id FROM media_items WHERE actor_id = ?
)`, "source_actor_disabled", now, actorID, actorID)
}

func revokeSavedCuesForOrbitTx(tx *sql.Tx, orbitID, now int64) error {
	return revokeSavedCuesQueryTx(tx, `owner_orbit_id = ?`, "owner_orbit_disabled", now, orbitID)
}

// ReconcileSavedCues derives validity from canonical rows on every startup.
// It repairs no counters: active-row SUM/COUNT is the accounting authority.
func (s *Store) ReconcileSavedCues(now int64) error {
	if now <= 0 {
		return ErrSavedCueInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT ` + savedCueColumns + ` FROM saved_cues WHERE state = 'active' ORDER BY id`)
	if err != nil {
		return err
	}
	var cues []SavedCue
	for rows.Next() {
		cue, err := scanSavedCue(rows)
		if err != nil {
			rows.Close()
			return err
		}
		cues = append(cues, cue)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, cue := range cues {
		reason := ""
		var active int
		if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM orbits WHERE id = ? AND status = 'active')`, cue.OwnerOrbitID).Scan(&active); err != nil {
			return err
		}
		if active == 0 {
			reason = "owner_orbit_disabled"
		} else if err := tx.QueryRow(`SELECT EXISTS(
  SELECT 1 FROM actors a JOIN memberships m ON m.actor_id = a.id
  WHERE a.id = ? AND a.revoked_at IS NULL AND m.orbit_id = ? AND m.left_at IS NULL
)`, cue.CreatedByActorID, cue.OwnerOrbitID).Scan(&active); err != nil {
			return err
		} else if active == 0 {
			reason = "source_actor_disabled"
		} else if cue.SourceKind == SavedCueSourceBuiltin {
			if cue.BuiltinAssetID != BuiltinRecordingCueAssetID || cue.BuiltinSHA256 != BuiltinRecordingCueSHA256 ||
				cue.SourceSHA256 != BuiltinRecordingCueSHA256 || cue.SourceBytes != BuiltinRecordingCueBytes ||
				cue.SourceDurationMS != BuiltinRecordingCueDuration {
				reason = "builtin_version_unsupported"
			}
		} else {
			if err := tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM media_items
WHERE id = ? AND owner_orbit_id = ? AND status = 'ready' AND kind = 'audio_clip'
  AND source = 'app' AND storage_key <> '' AND sha256 = ? AND size_bytes = ?
  AND duration_ms = ?)`, cue.MediaID, cue.OwnerOrbitID, cue.SourceSHA256,
				cue.SourceBytes, cue.SourceDurationMS).Scan(&active); err != nil {
				return err
			}
			if active == 0 {
				reason = "source_media_deleted"
			}
		}
		if reason != "" {
			if err := revokeSavedCueTx(tx, cue, reason, now); err != nil {
				return err
			}
		}
	}
	if err := s.checkpoint("saved_cue_reconcile_before_commit"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) PendingSavedCueRevocations(limit int) ([]SavedCueRevocation, error) {
	if limit <= 0 || limit > 1000 {
		return nil, ErrSavedCueInvalid
	}
	rows, err := s.db.Query(`SELECT cue_id, invalidated_generation, reason,
pending_action, active_action, interrupted_main_action, state,
created_at, updated_at, completed_at FROM saved_cue_revocations
WHERE state = 'pending' ORDER BY created_at, cue_id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []SavedCueRevocation
	for rows.Next() {
		var value SavedCueRevocation
		if err := rows.Scan(&value.CueID, &value.InvalidatedGeneration, &value.Reason,
			&value.PendingAction, &value.ActiveAction, &value.InterruptedMainAction,
			&value.State, &value.CreatedAt, &value.UpdatedAt, &value.CompletedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) CompleteSavedCueRevocation(cueID string, generation, now int64) (SavedCueRevocation, error) {
	if !savedCueIDPattern.MatchString(cueID) || generation <= 0 || now <= 0 {
		return SavedCueRevocation{}, ErrSavedCueInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return SavedCueRevocation{}, err
	}
	defer tx.Rollback()
	var value SavedCueRevocation
	err = tx.QueryRow(`SELECT cue_id, invalidated_generation, reason,
pending_action, active_action, interrupted_main_action, state,
created_at, updated_at, completed_at FROM saved_cue_revocations
WHERE cue_id = ? AND invalidated_generation = ?`, cueID, generation).Scan(
		&value.CueID, &value.InvalidatedGeneration, &value.Reason,
		&value.PendingAction, &value.ActiveAction, &value.InterruptedMainAction, &value.State,
		&value.CreatedAt, &value.UpdatedAt, &value.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SavedCueRevocation{}, fmt.Errorf("%w: revocation", ErrSavedCueNotFound)
	}
	if err != nil {
		return SavedCueRevocation{}, err
	}
	if value.State == "done" {
		return value, tx.Commit()
	}
	result, err := tx.Exec(`UPDATE saved_cue_revocations SET state = 'done',
updated_at = ?, completed_at = ? WHERE cue_id = ? AND invalidated_generation = ?
AND state = 'pending'`, now, now, cueID, generation)
	if err != nil {
		return SavedCueRevocation{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return SavedCueRevocation{}, err
		}
		return SavedCueRevocation{}, ErrSavedCueStateConflict
	}
	value.State = "done"
	value.UpdatedAt = now
	value.CompletedAt = now
	var orbitID, actorID int64
	if err := tx.QueryRow(`SELECT owner_orbit_id, created_by_actor_id
FROM saved_cues WHERE id = ?`, cueID).Scan(&orbitID, &actorID); err != nil {
		return SavedCueRevocation{}, err
	}
	if _, err := tx.Exec(`INSERT INTO saved_cue_audit_events(
  cue_id, owner_orbit_id, actor_id, event_type, source_generation, occurred_at
) VALUES(?, ?, ?, 'saved_cue.revocation_completed', ?, ?)`, cueID, orbitID,
		actorID, generation, now); err != nil {
		return SavedCueRevocation{}, err
	}
	if err := s.checkpoint("saved_cue_revocation_complete_before_commit"); err != nil {
		return SavedCueRevocation{}, err
	}
	if err := tx.Commit(); err != nil {
		return SavedCueRevocation{}, err
	}
	return value, nil
}
