package store

// streamTrackSchema is a candidate-neutral, additive persistence seam. It
// deliberately does not alter Phase 1 media, transmission, inbox, history or
// legacy Spotify/session rows. A previous coordinator can therefore ignore
// every table below and later roll forward without losing streamed-track state.
const streamTrackSchema = `
CREATE TABLE IF NOT EXISTS stream_track_metadata (
  media_id TEXT PRIMARY KEY REFERENCES media_items(id),
  original_filename TEXT NOT NULL CHECK(length(original_filename) BETWEEN 1 AND 512),
  original_mime TEXT NOT NULL CHECK(length(original_mime) BETWEEN 3 AND 128),
  original_container TEXT NOT NULL CHECK(length(original_container) BETWEEN 1 AND 64),
  original_codec TEXT NOT NULL CHECK(length(original_codec) BETWEEN 1 AND 64),
  original_size_bytes INTEGER NOT NULL CHECK(original_size_bytes > 0),
  original_sha256 TEXT NOT NULL CHECK(
    length(original_sha256) = 64 AND original_sha256 NOT GLOB '*[^0-9a-f]*'
  ),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at)
);

CREATE TABLE IF NOT EXISTS stream_variants (
  id TEXT PRIMARY KEY CHECK(length(id) = 29 AND substr(id, 1, 3) = 'sv_'),
  media_id TEXT NOT NULL REFERENCES stream_track_metadata(media_id),
  purpose TEXT NOT NULL CHECK(purpose IN ('original', 'canonical')),
  profile TEXT NOT NULL CHECK(length(profile) BETWEEN 1 AND 64),
  codec TEXT NOT NULL CHECK(codec IN ('mp3', 'aac-lc', 'opus')),
  container TEXT NOT NULL CHECK(container IN ('mp3', 'm4a-faststart', 'adts', 'ogg')),
  mime TEXT NOT NULL CHECK(mime IN ('audio/mpeg', 'audio/mp4', 'audio/aac', 'audio/ogg')),
  rate_mode TEXT NOT NULL CHECK(rate_mode IN ('cbr', 'vbr')),
  bitrate_bps INTEGER NOT NULL CHECK(bitrate_bps > 0),
  sample_rate_hz INTEGER NOT NULL CHECK(sample_rate_hz = 48000),
  channels INTEGER NOT NULL CHECK(channels = 2),
  duration_ms INTEGER NOT NULL CHECK(duration_ms BETWEEN 1 AND 7200000),
  size_bytes INTEGER NOT NULL CHECK(size_bytes BETWEEN 1 AND 524288000),
  sha256 TEXT NOT NULL CHECK(length(sha256) = 64 AND sha256 NOT GLOB '*[^0-9a-f]*'),
  etag TEXT NOT NULL CHECK(length(etag) = 73 AND substr(etag, 1, 8) = '"sha256-' AND substr(etag, 73, 1) = '"'),
  storage_key TEXT NOT NULL UNIQUE CHECK(length(storage_key) = 74 AND substr(storage_key, 1, 10) = 'stream/v1/'),
  chunk_size_bytes INTEGER NOT NULL CHECK(chunk_size_bytes BETWEEN 1 AND 1048576),
  chunk_manifest_json TEXT NOT NULL CHECK(length(chunk_manifest_json) BETWEEN 2 AND 1048576),
  seek_map_json TEXT NOT NULL CHECK(length(seek_map_json) BETWEEN 2 AND 1048576),
  status TEXT NOT NULL DEFAULT 'staged' CHECK(status IN ('staged', 'ready', 'revoked')),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  published_at INTEGER NOT NULL DEFAULT 0 CHECK(published_at >= 0),
  revoked_at INTEGER NOT NULL DEFAULT 0 CHECK(revoked_at >= 0),
  UNIQUE(media_id, profile),
  UNIQUE(media_id, codec, container, rate_mode, bitrate_bps),
  CHECK(etag = '"sha256-' || sha256 || '"'),
  CHECK(storage_key = 'stream/v1/' || sha256),
  CHECK((codec = 'mp3' AND container = 'mp3' AND mime = 'audio/mpeg')
     OR (codec = 'aac-lc' AND container = 'm4a-faststart' AND mime = 'audio/mp4')
     OR (codec = 'aac-lc' AND container = 'adts' AND mime = 'audio/aac')
     OR (codec = 'opus' AND container = 'ogg' AND mime = 'audio/ogg')),
  CHECK((status = 'staged' AND published_at = 0 AND revoked_at = 0)
     OR (status = 'ready' AND published_at > 0 AND revoked_at = 0)
     OR (status = 'revoked' AND revoked_at > 0))
);
CREATE INDEX IF NOT EXISTS stream_variants_media_status
  ON stream_variants(media_id, status, profile);

CREATE TRIGGER IF NOT EXISTS stream_variants_immutable_payload
BEFORE UPDATE ON stream_variants
WHEN NEW.media_id <> OLD.media_id OR NEW.purpose <> OLD.purpose
  OR NEW.profile <> OLD.profile OR NEW.codec <> OLD.codec
  OR NEW.container <> OLD.container OR NEW.mime <> OLD.mime
  OR NEW.rate_mode <> OLD.rate_mode OR NEW.bitrate_bps <> OLD.bitrate_bps
  OR NEW.sample_rate_hz <> OLD.sample_rate_hz OR NEW.channels <> OLD.channels
  OR NEW.duration_ms <> OLD.duration_ms OR NEW.size_bytes <> OLD.size_bytes
  OR NEW.sha256 <> OLD.sha256 OR NEW.etag <> OLD.etag
  OR NEW.storage_key <> OLD.storage_key
  OR NEW.chunk_size_bytes <> OLD.chunk_size_bytes
  OR NEW.chunk_manifest_json <> OLD.chunk_manifest_json
  OR NEW.seek_map_json <> OLD.seek_map_json
BEGIN
  SELECT RAISE(ABORT, 'published stream variant payload is immutable');
END;

CREATE TABLE IF NOT EXISTS stream_variant_policy (
  singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
  contract_version TEXT NOT NULL,
  production_selection_enabled INTEGER NOT NULL CHECK(production_selection_enabled IN (0, 1)),
  selected_profile TEXT NOT NULL DEFAULT '',
  revision INTEGER NOT NULL CHECK(revision > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at > 0),
  CHECK(production_selection_enabled = 0 AND selected_profile = '')
);
INSERT OR IGNORE INTO stream_variant_policy(
  singleton, contract_version, production_selection_enabled, selected_profile, revision, updated_at
) VALUES(1, 'p2-codec-player-adr-handoff.v1', 0, '', 1, 1);

CREATE TABLE IF NOT EXISTS stream_playback_domains (
  id TEXT PRIMARY KEY CHECK(length(id) = 30 AND substr(id, 1, 4) = 'spd_'),
  target_kind TEXT NOT NULL CHECK(target_kind IN ('orbit', 'air')),
  target_ref TEXT NOT NULL CHECK(length(target_ref) BETWEEN 1 AND 64),
  main_program_kind TEXT NOT NULL DEFAULT 'none'
    CHECK(main_program_kind IN ('none', 'legacy_session', 'spotify')),
  main_program_ref TEXT NOT NULL DEFAULT '' CHECK(length(main_program_ref) <= 512),
  source_kind TEXT NOT NULL DEFAULT 'none' CHECK(source_kind IN ('none', 'stream_track')),
  current_queue_item_id TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT 'idle' CHECK(state IN ('idle', 'buffering', 'playing', 'paused')),
  playback_generation INTEGER NOT NULL DEFAULT 0 CHECK(playback_generation >= 0),
  seek_generation INTEGER NOT NULL DEFAULT 0 CHECK(seek_generation >= 0),
  audible_position_ms INTEGER NOT NULL DEFAULT 0 CHECK(audible_position_ms >= 0),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  UNIQUE(target_kind, target_ref),
  CHECK((main_program_kind = 'none' AND main_program_ref = '')
     OR (main_program_kind <> 'none' AND main_program_ref <> ''))
);

CREATE TABLE IF NOT EXISTS stream_queue_items (
  id TEXT PRIMARY KEY CHECK(length(id) = 29 AND substr(id, 1, 3) = 'sq_'),
  domain_id TEXT NOT NULL REFERENCES stream_playback_domains(id),
  media_id TEXT NOT NULL REFERENCES stream_track_metadata(media_id),
  variant_profile TEXT NOT NULL CHECK(length(variant_profile) BETWEEN 1 AND 64),
  sequence INTEGER NOT NULL CHECK(sequence > 0),
  state TEXT NOT NULL DEFAULT 'queued' CHECK(state IN ('queued', 'active', 'played', 'cancelled')),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  UNIQUE(domain_id, sequence)
);
CREATE INDEX IF NOT EXISTS stream_queue_items_domain_sequence
  ON stream_queue_items(domain_id, sequence);
`

func (s *Store) initStreamTrackSchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(streamTrackSchema); err != nil {
		return err
	}
	if err := foreignKeyCheck(tx); err != nil {
		return err
	}
	if err := s.checkpoint("stream_track_ddl_before_commit"); err != nil {
		return err
	}
	return tx.Commit()
}
