package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"relux.works/duet/coordinator/internal/ulid"
)

var (
	ErrStreamTrackInvalid              = errors.New("stream track input is invalid")
	ErrStreamTrackNotFound             = errors.New("stream track was not found")
	ErrStreamTrackConflict             = errors.New("stream track state changed")
	ErrStreamProductionSelectionLocked = errors.New("production stream variant selection is disabled by codec ADR")
)

var streamSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type StreamTrackMetadata struct {
	MediaID, OriginalFilename, OriginalMIME, OriginalContainer, OriginalCodec string
	OriginalSizeBytes, Revision, CreatedAt, UpdatedAt                         int64
	OriginalSHA256                                                            string
}

type CreateStreamTrackMetadataParams struct {
	MediaID, OriginalFilename, OriginalMIME, OriginalContainer, OriginalCodec string
	OriginalSizeBytes, CreatedAt                                              int64
	OriginalSHA256                                                            string
}

type StreamChunk struct {
	Index  int    `json:"index"`
	Start  int64  `json:"start"`
	End    int64  `json:"end"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type StreamSeekPoint struct {
	TimeMS int64 `json:"timeMS"`
	Offset int64 `json:"offset"`
}

type StreamVariant struct {
	ID, MediaID, Purpose, Profile, Codec, Container, MIME, RateMode string
	BitrateBPS, SampleRateHz, Channels, DurationMS, SizeBytes       int64
	SHA256, ETag, StorageKey                                        string
	ChunkSizeBytes                                                  int64
	Chunks                                                          []StreamChunk
	SeekMap                                                         []StreamSeekPoint
	Status                                                          string
	Revision, CreatedAt, UpdatedAt, PublishedAt, RevokedAt          int64
}

type CreateStreamVariantParams struct {
	MediaID, Purpose, Profile, Codec, Container, MIME, RateMode string
	BitrateBPS, SampleRateHz, Channels, DurationMS, SizeBytes   int64
	SHA256, ETag, StorageKey                                    string
	ChunkSizeBytes, CreatedAt                                   int64
	Chunks                                                      []StreamChunk
	SeekMap                                                     []StreamSeekPoint
}

type StreamByteRange struct {
	Start, End, SeekTimeMS int64
}

type StreamPlaybackDomain struct {
	ID, TargetKind, TargetRef, MainProgramKind, MainProgramRef string
	SourceKind, CurrentQueueItemID, State                      string
	PlaybackGeneration, SeekGeneration, AudiblePositionMS      int64
	Revision, CreatedAt, UpdatedAt                             int64
	Queue                                                      []StreamQueueItem
}

type StreamQueueItem struct {
	ID, DomainID, MediaID, VariantProfile, State string
	Sequence, CreatedAt, UpdatedAt               int64
}

const streamVariantColumns = `id, media_id, purpose, profile, codec, container,
mime, rate_mode, bitrate_bps, sample_rate_hz, channels, duration_ms, size_bytes,
sha256, etag, storage_key, chunk_size_bytes, chunk_manifest_json, seek_map_json,
status, revision, created_at, updated_at, published_at, revoked_at`

func CreateStrongStreamETag(sha256 string) string { return `"sha256-` + sha256 + `"` }

func validateStreamTrackMetadata(params CreateStreamTrackMetadataParams) error {
	if params.MediaID == "" || params.OriginalFilename == "" || len(params.OriginalFilename) > 512 ||
		params.OriginalMIME == "" || len(params.OriginalMIME) > 128 ||
		params.OriginalContainer == "" || len(params.OriginalContainer) > 64 ||
		params.OriginalCodec == "" || len(params.OriginalCodec) > 64 ||
		params.OriginalSizeBytes <= 0 || params.CreatedAt <= 0 ||
		!streamSHA256Pattern.MatchString(params.OriginalSHA256) {
		return ErrStreamTrackInvalid
	}
	return nil
}

func (s *Store) CreateStreamTrackMetadata(params CreateStreamTrackMetadataParams) (StreamTrackMetadata, error) {
	if err := validateStreamTrackMetadata(params); err != nil {
		return StreamTrackMetadata{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return StreamTrackMetadata{}, err
	}
	defer tx.Rollback()
	var kind MediaKind
	var status MediaItemStatus
	if err := tx.QueryRow(`SELECT kind, status FROM media_items WHERE id = ?`, params.MediaID).Scan(&kind, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return StreamTrackMetadata{}, ErrStreamTrackNotFound
		}
		return StreamTrackMetadata{}, err
	}
	if kind != MediaKindAudioTrack || status == MediaStatusDeleted || status == MediaStatusExpired || status == MediaStatusFailed {
		return StreamTrackMetadata{}, ErrStreamTrackInvalid
	}
	_, err = tx.Exec(`INSERT INTO stream_track_metadata(
media_id, original_filename, original_mime, original_container, original_codec,
original_size_bytes, original_sha256, revision, created_at, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`, params.MediaID, params.OriginalFilename,
		params.OriginalMIME, params.OriginalContainer, params.OriginalCodec,
		params.OriginalSizeBytes, params.OriginalSHA256, params.CreatedAt, params.CreatedAt)
	if err != nil {
		return StreamTrackMetadata{}, err
	}
	if err := tx.Commit(); err != nil {
		return StreamTrackMetadata{}, err
	}
	return s.GetStreamTrackMetadata(params.MediaID)
}

func (s *Store) GetStreamTrackMetadata(mediaID string) (StreamTrackMetadata, error) {
	var out StreamTrackMetadata
	err := s.db.QueryRow(`SELECT media_id, original_filename, original_mime, original_container,
original_codec, original_size_bytes, original_sha256, revision, created_at, updated_at
FROM stream_track_metadata WHERE media_id = ?`, mediaID).Scan(&out.MediaID, &out.OriginalFilename,
		&out.OriginalMIME, &out.OriginalContainer, &out.OriginalCodec, &out.OriginalSizeBytes,
		&out.OriginalSHA256, &out.Revision, &out.CreatedAt, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return StreamTrackMetadata{}, ErrStreamTrackNotFound
	}
	return out, err
}

func validateStreamVariant(params CreateStreamVariantParams) error {
	if params.MediaID == "" || params.Profile == "" || len(params.Profile) > 64 ||
		(params.Purpose != "original" && params.Purpose != "canonical") ||
		(params.RateMode != "cbr" && params.RateMode != "vbr") || params.BitrateBPS <= 0 ||
		params.SampleRateHz != 48000 || params.Channels != 2 || params.DurationMS <= 0 ||
		params.DurationMS > 7200000 || params.SizeBytes <= 0 || params.SizeBytes > 500<<20 ||
		params.ChunkSizeBytes <= 0 || params.ChunkSizeBytes > 1<<20 || params.CreatedAt <= 0 ||
		!streamSHA256Pattern.MatchString(params.SHA256) || params.ETag != CreateStrongStreamETag(params.SHA256) ||
		params.StorageKey != "stream/v1/"+params.SHA256 {
		return ErrStreamTrackInvalid
	}
	pairs := map[string]string{"mp3/mp3": "audio/mpeg", "aac-lc/m4a-faststart": "audio/mp4", "aac-lc/adts": "audio/aac", "opus/ogg": "audio/ogg"}
	if pairs[params.Codec+"/"+params.Container] != params.MIME || len(params.Chunks) == 0 || len(params.SeekMap) == 0 {
		return ErrStreamTrackInvalid
	}
	var cursor int64
	for i, chunk := range params.Chunks {
		if chunk.Index != i || chunk.Start != cursor || chunk.Bytes <= 0 || chunk.Bytes > params.ChunkSizeBytes ||
			chunk.End != cursor+chunk.Bytes-1 || !streamSHA256Pattern.MatchString(chunk.SHA256) {
			return ErrStreamTrackInvalid
		}
		cursor += chunk.Bytes
	}
	if cursor != params.SizeBytes || params.SeekMap[0] != (StreamSeekPoint{}) {
		return ErrStreamTrackInvalid
	}
	previousTime, previousOffset := int64(-1), int64(-1)
	for _, point := range params.SeekMap {
		if point.TimeMS <= previousTime || point.Offset < previousOffset || point.Offset < 0 ||
			point.Offset >= params.SizeBytes || point.Offset%params.ChunkSizeBytes != 0 ||
			(previousTime >= 0 && point.TimeMS-previousTime > 10000) {
			return ErrStreamTrackInvalid
		}
		previousTime, previousOffset = point.TimeMS, point.Offset
	}
	if params.DurationMS-previousTime > 10000 {
		return ErrStreamTrackInvalid
	}
	return nil
}

func (s *Store) CreateStagedStreamVariant(params CreateStreamVariantParams) (StreamVariant, error) {
	if err := validateStreamVariant(params); err != nil {
		return StreamVariant{}, err
	}
	chunks, _ := json.Marshal(params.Chunks)
	seekMap, _ := json.Marshal(params.SeekMap)
	id := "sv_" + ulid.New(time.UnixMilli(params.CreatedAt))
	_, err := s.db.Exec(`INSERT INTO stream_variants(
id, media_id, purpose, profile, codec, container, mime, rate_mode, bitrate_bps,
sample_rate_hz, channels, duration_ms, size_bytes, sha256, etag, storage_key,
chunk_size_bytes, chunk_manifest_json, seek_map_json, status, revision, created_at, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'staged', 1, ?, ?)`,
		id, params.MediaID, params.Purpose, params.Profile, params.Codec, params.Container,
		params.MIME, params.RateMode, params.BitrateBPS, params.SampleRateHz, params.Channels,
		params.DurationMS, params.SizeBytes, params.SHA256, params.ETag, params.StorageKey,
		params.ChunkSizeBytes, string(chunks), string(seekMap), params.CreatedAt, params.CreatedAt)
	if err != nil {
		return StreamVariant{}, err
	}
	return s.GetStreamVariant(id)
}

func scanStreamVariant(row sqlScanner) (StreamVariant, error) {
	var out StreamVariant
	var chunks, seekMap string
	err := row.Scan(&out.ID, &out.MediaID, &out.Purpose, &out.Profile, &out.Codec, &out.Container,
		&out.MIME, &out.RateMode, &out.BitrateBPS, &out.SampleRateHz, &out.Channels,
		&out.DurationMS, &out.SizeBytes, &out.SHA256, &out.ETag, &out.StorageKey,
		&out.ChunkSizeBytes, &chunks, &seekMap, &out.Status, &out.Revision, &out.CreatedAt,
		&out.UpdatedAt, &out.PublishedAt, &out.RevokedAt)
	if err != nil {
		return StreamVariant{}, err
	}
	if json.Unmarshal([]byte(chunks), &out.Chunks) != nil || json.Unmarshal([]byte(seekMap), &out.SeekMap) != nil {
		return StreamVariant{}, fmt.Errorf("%w: corrupt variant manifest", ErrStreamTrackInvalid)
	}
	return out, nil
}

func (s *Store) GetStreamVariant(id string) (StreamVariant, error) {
	out, err := scanStreamVariant(s.db.QueryRow(`SELECT `+streamVariantColumns+` FROM stream_variants WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return StreamVariant{}, ErrStreamTrackNotFound
	}
	return out, err
}

func (s *Store) ListStreamVariants(mediaID string) ([]StreamVariant, error) {
	rows, err := s.db.Query(`SELECT `+streamVariantColumns+` FROM stream_variants
WHERE media_id = ? ORDER BY profile, id`, mediaID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StreamVariant
	for rows.Next() {
		variant, err := scanStreamVariant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, variant)
	}
	return out, rows.Err()
}

func (s *Store) transitionStreamVariant(id, from, to string, expectedRevision, now int64) (StreamVariant, error) {
	if expectedRevision <= 0 || now <= 0 {
		return StreamVariant{}, ErrStreamTrackInvalid
	}
	publishedExpr, revokedExpr := "published_at", "revoked_at"
	if to == "ready" {
		publishedExpr = "?"
	}
	if to == "revoked" {
		revokedExpr = "?"
	}
	query := `UPDATE stream_variants SET status = ?, revision = revision + 1, updated_at = ?, published_at = ` + publishedExpr + `, revoked_at = ` + revokedExpr + ` WHERE id = ? AND status = ? AND revision = ?`
	args := []any{to, now}
	if to == "ready" {
		args = append(args, now)
	}
	if to == "revoked" {
		args = append(args, now)
	}
	args = append(args, id, from, expectedRevision)
	result, err := s.db.Exec(query, args...)
	if err != nil {
		return StreamVariant{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return StreamVariant{}, ErrStreamTrackConflict
	}
	return s.GetStreamVariant(id)
}

func (s *Store) PublishStreamVariant(id string, expectedRevision, now int64) (StreamVariant, error) {
	return s.transitionStreamVariant(id, "staged", "ready", expectedRevision, now)
}

func (s *Store) RevokeStreamVariant(id string, expectedRevision, now int64) (StreamVariant, error) {
	variant, err := s.GetStreamVariant(id)
	if err != nil {
		return StreamVariant{}, err
	}
	if variant.Status != "staged" && variant.Status != "ready" {
		return StreamVariant{}, ErrStreamTrackConflict
	}
	return s.transitionStreamVariant(id, variant.Status, "revoked", expectedRevision, now)
}

func (s *Store) GetReadyStreamVariantForProfile(mediaID, profile string) (StreamVariant, error) {
	out, err := scanStreamVariant(s.db.QueryRow(`SELECT `+streamVariantColumns+` FROM stream_variants WHERE media_id = ? AND profile = ? AND purpose = 'canonical' AND status = 'ready'`, mediaID, profile))
	if errors.Is(err, sql.ErrNoRows) {
		return StreamVariant{}, ErrStreamTrackNotFound
	}
	return out, err
}

// SelectProductionStreamVariant intentionally has no bypass parameter. A
// reviewed replacement ADR must change the persisted policy contract and this
// repository before any production decoder can observe candidate variants.
func (s *Store) SelectProductionStreamVariant(mediaID string) (StreamVariant, error) {
	var enabled int
	var profile string
	if err := s.db.QueryRow(`SELECT production_selection_enabled, selected_profile FROM stream_variant_policy WHERE singleton = 1`).Scan(&enabled, &profile); err != nil {
		return StreamVariant{}, err
	}
	if enabled != 1 || profile == "" {
		return StreamVariant{}, ErrStreamProductionSelectionLocked
	}
	return s.GetReadyStreamVariantForProfile(mediaID, profile)
}

func (variant StreamVariant) ByteRangeForTime(requestedMS int64) (StreamByteRange, error) {
	if requestedMS < 0 || requestedMS > variant.DurationMS || len(variant.SeekMap) == 0 || len(variant.Chunks) == 0 {
		return StreamByteRange{}, ErrStreamTrackInvalid
	}
	point := variant.SeekMap[0]
	for _, candidate := range variant.SeekMap[1:] {
		if candidate.TimeMS > requestedMS {
			break
		}
		point = candidate
	}
	index := point.Offset / variant.ChunkSizeBytes
	if index < 0 || index >= int64(len(variant.Chunks)) {
		return StreamByteRange{}, ErrStreamTrackInvalid
	}
	chunk := variant.Chunks[index]
	return StreamByteRange{Start: chunk.Start, End: chunk.End, SeekTimeMS: point.TimeMS}, nil
}

func scanStreamPlaybackDomain(row sqlScanner) (StreamPlaybackDomain, error) {
	var out StreamPlaybackDomain
	err := row.Scan(&out.ID, &out.TargetKind, &out.TargetRef, &out.MainProgramKind,
		&out.MainProgramRef, &out.SourceKind,
		&out.CurrentQueueItemID, &out.State, &out.PlaybackGeneration, &out.SeekGeneration,
		&out.AudiblePositionMS, &out.Revision, &out.CreatedAt, &out.UpdatedAt)
	return out, err
}

const streamPlaybackDomainColumns = `id, target_kind, target_ref,
main_program_kind, main_program_ref, source_kind,
current_queue_item_id, state, playback_generation, seek_generation,
audible_position_ms, revision, created_at, updated_at`

func (s *Store) EnsureStreamPlaybackDomain(targetKind, targetRef string, now int64) (StreamPlaybackDomain, error) {
	if (targetKind != "orbit" && targetKind != "air") || targetRef == "" || len(targetRef) > 64 || now <= 0 {
		return StreamPlaybackDomain{}, ErrStreamTrackInvalid
	}
	id := "spd_" + ulid.New(time.UnixMilli(now))
	_, err := s.db.Exec(`INSERT OR IGNORE INTO stream_playback_domains(id, target_kind, target_ref, created_at, updated_at) VALUES(?, ?, ?, ?, ?)`, id, targetKind, targetRef, now, now)
	if err != nil {
		return StreamPlaybackDomain{}, err
	}
	return s.LoadStreamPlaybackDomainByTarget(targetKind, targetRef)
}

// PinStreamMainProgramSource persists only the resume pointer needed by the
// streamed-track coordinator. Legacy session/Spotify rows remain authoritative
// and are never rewritten or backfilled by this additive model.
func (s *Store) PinStreamMainProgramSource(domainID, kind, ref string, expectedRevision, now int64) (StreamPlaybackDomain, error) {
	if (kind != "legacy_session" && kind != "spotify") || ref == "" || len(ref) > 512 ||
		expectedRevision <= 0 || now <= 0 {
		return StreamPlaybackDomain{}, ErrStreamTrackInvalid
	}
	result, err := s.db.Exec(`UPDATE stream_playback_domains
SET main_program_kind = ?, main_program_ref = ?, revision = revision + 1, updated_at = ?
WHERE id = ? AND revision = ?`, kind, ref, now, domainID, expectedRevision)
	if err != nil {
		return StreamPlaybackDomain{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return StreamPlaybackDomain{}, ErrStreamTrackConflict
	}
	return s.loadStreamPlaybackDomainByID(domainID)
}

func (s *Store) LoadStreamPlaybackDomainByTarget(targetKind, targetRef string) (StreamPlaybackDomain, error) {
	out, err := scanStreamPlaybackDomain(s.db.QueryRow(`SELECT `+streamPlaybackDomainColumns+` FROM stream_playback_domains WHERE target_kind = ? AND target_ref = ?`, targetKind, targetRef))
	if errors.Is(err, sql.ErrNoRows) {
		return StreamPlaybackDomain{}, ErrStreamTrackNotFound
	}
	if err != nil {
		return StreamPlaybackDomain{}, err
	}
	rows, err := s.db.Query(`SELECT id, domain_id, media_id, variant_profile, sequence, state, created_at, updated_at FROM stream_queue_items WHERE domain_id = ? ORDER BY sequence`, out.ID)
	if err != nil {
		return StreamPlaybackDomain{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item StreamQueueItem
		if err := rows.Scan(&item.ID, &item.DomainID, &item.MediaID, &item.VariantProfile, &item.Sequence, &item.State, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return StreamPlaybackDomain{}, err
		}
		out.Queue = append(out.Queue, item)
	}
	return out, rows.Err()
}

func (s *Store) EnqueueStreamTrack(domainID, mediaID, profile string, expectedRevision, now int64) (StreamQueueItem, error) {
	if domainID == "" || mediaID == "" || profile == "" || expectedRevision <= 0 || now <= 0 {
		return StreamQueueItem{}, ErrStreamTrackInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return StreamQueueItem{}, err
	}
	defer tx.Rollback()
	var ready int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM stream_variants WHERE media_id = ? AND profile = ? AND status = 'ready'`, mediaID, profile).Scan(&ready); err != nil {
		return StreamQueueItem{}, err
	}
	if ready != 1 {
		return StreamQueueItem{}, ErrStreamTrackNotFound
	}
	var sequence int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(sequence), 0) + 1 FROM stream_queue_items WHERE domain_id = ?`, domainID).Scan(&sequence); err != nil {
		return StreamQueueItem{}, err
	}
	id := "sq_" + ulid.New(time.UnixMilli(now))
	result, err := tx.Exec(`UPDATE stream_playback_domains SET revision = revision + 1, updated_at = ? WHERE id = ? AND revision = ?`, now, domainID, expectedRevision)
	if err != nil {
		return StreamQueueItem{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return StreamQueueItem{}, ErrStreamTrackConflict
	}
	if _, err := tx.Exec(`INSERT INTO stream_queue_items(id, domain_id, media_id, variant_profile, sequence, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?)`, id, domainID, mediaID, profile, sequence, now, now); err != nil {
		return StreamQueueItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return StreamQueueItem{}, err
	}
	return StreamQueueItem{ID: id, DomainID: domainID, MediaID: mediaID, VariantProfile: profile, Sequence: sequence, State: "queued", CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) ActivateStreamQueueItem(domainID, itemID string, expectedRevision, now int64) (StreamPlaybackDomain, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return StreamPlaybackDomain{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE stream_queue_items SET state = 'active', updated_at = ? WHERE id = ? AND domain_id = ? AND state = 'queued'`, now, itemID, domainID)
	if err != nil {
		return StreamPlaybackDomain{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return StreamPlaybackDomain{}, ErrStreamTrackConflict
	}
	result, err = tx.Exec(`UPDATE stream_playback_domains SET source_kind = 'stream_track', current_queue_item_id = ?, state = 'buffering', playback_generation = playback_generation + 1, seek_generation = 0, audible_position_ms = 0, revision = revision + 1, updated_at = ? WHERE id = ? AND revision = ?`, itemID, now, domainID, expectedRevision)
	if err != nil {
		return StreamPlaybackDomain{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return StreamPlaybackDomain{}, ErrStreamTrackConflict
	}
	if err := tx.Commit(); err != nil {
		return StreamPlaybackDomain{}, err
	}
	return s.loadStreamPlaybackDomainByID(domainID)
}

func (s *Store) RecordStreamAudibleProgress(domainID string, expectedRevision, playbackGeneration, seekGeneration, positionMS int64, state string, now int64) (StreamPlaybackDomain, error) {
	if (state != "playing" && state != "paused") || expectedRevision <= 0 ||
		playbackGeneration <= 0 || seekGeneration < 0 || positionMS < 0 || now <= 0 {
		return StreamPlaybackDomain{}, ErrStreamTrackInvalid
	}
	result, err := s.db.Exec(`UPDATE stream_playback_domains SET state = ?, audible_position_ms = ?, revision = revision + 1, updated_at = ? WHERE id = ? AND revision = ? AND playback_generation = ? AND seek_generation = ? AND source_kind = 'stream_track' AND ? >= audible_position_ms`, state, positionMS, now, domainID, expectedRevision, playbackGeneration, seekGeneration, positionMS)
	if err != nil {
		return StreamPlaybackDomain{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return StreamPlaybackDomain{}, ErrStreamTrackConflict
	}
	return s.loadStreamPlaybackDomainByID(domainID)
}

func (s *Store) SeekStreamPlayback(domainID string, expectedRevision, playbackGeneration, positionMS, now int64) (StreamPlaybackDomain, error) {
	if expectedRevision <= 0 || playbackGeneration <= 0 || positionMS < 0 || now <= 0 {
		return StreamPlaybackDomain{}, ErrStreamTrackInvalid
	}
	result, err := s.db.Exec(`UPDATE stream_playback_domains SET state = 'buffering', seek_generation = seek_generation + 1, audible_position_ms = ?, revision = revision + 1, updated_at = ? WHERE id = ? AND revision = ? AND playback_generation = ? AND source_kind = 'stream_track'`, positionMS, now, domainID, expectedRevision, playbackGeneration)
	if err != nil {
		return StreamPlaybackDomain{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return StreamPlaybackDomain{}, ErrStreamTrackConflict
	}
	return s.loadStreamPlaybackDomainByID(domainID)
}

func (s *Store) loadStreamPlaybackDomainByID(id string) (StreamPlaybackDomain, error) {
	var kind, ref string
	if err := s.db.QueryRow(`SELECT target_kind, target_ref FROM stream_playback_domains WHERE id = ?`, id).Scan(&kind, &ref); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return StreamPlaybackDomain{}, ErrStreamTrackNotFound
		}
		return StreamPlaybackDomain{}, err
	}
	return s.LoadStreamPlaybackDomainByTarget(kind, ref)
}
