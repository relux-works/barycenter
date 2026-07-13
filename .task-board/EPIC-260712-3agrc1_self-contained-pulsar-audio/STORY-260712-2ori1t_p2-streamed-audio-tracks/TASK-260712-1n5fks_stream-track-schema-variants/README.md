# Add track, variant, seek and playback persistence

## Description
Extend the generic media and session substrate with the exact ADR fields required for compressed streaming without changing Phase 1 rows.

## Scope
Add audio_track kind and original metadata plus stream_variants with exact codec, container, profile, bitrate, duration, size, ETag, whole or chunk integrity, VBR seek-map and storage fields from the ADR. Add conditional processing transitions and persisted main-program source, queue, audible progress, seek generation and restart fields without duplicating transmission targets or history. Keep server-generated keys, legacy clips, Spotify elements and old session snapshots readable. Add additive migration, partial-worker and previous-binary rollback fixtures.

## Acceptance Criteria
Fresh and migrated data deterministically selects a pinned variant profile, maps time to ranges, rejects stale processing or seek state and restores queue and audible progress. Phase 1 clip, transmission and Spotify rows remain unchanged, old binaries ignore new rows without deleting them and no user-controlled key or duplicate target model appears.
