package store

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func createStreamTrackFixture(t *testing.T, st *Store, credentials OnboardingCredentials, now int64) (MediaItem, StreamTrackMetadata) {
	t.Helper()
	media, err := st.CreateMediaItem(CreateMediaItemParams{
		OwnerOrbitID: credentials.OrbitID, ActorID: credentials.ActorID,
		Kind: MediaKindAudioTrack, Source: MediaSourceApp, Title: "long-track.flac",
		CreatedAt: now, ExpiresAt: now + int64((30*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := st.CreateStreamTrackMetadata(CreateStreamTrackMetadataParams{
		MediaID: media.ID, OriginalFilename: "long-track.flac", OriginalMIME: "audio/flac",
		OriginalContainer: "flac", OriginalCodec: "flac", OriginalSizeBytes: 4 << 20,
		OriginalSHA256: strings.Repeat("a", 64), CreatedAt: now + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return media, metadata
}

func streamVariantParams(mediaID, profile string, now int64) CreateStreamVariantParams {
	digest := strings.Repeat("b", 64)
	return CreateStreamVariantParams{
		MediaID: mediaID, Purpose: "canonical", Profile: profile,
		Codec: "mp3", Container: "mp3", MIME: "audio/mpeg", RateMode: "cbr",
		BitrateBPS: 128000, SampleRateHz: 48000, Channels: 2,
		DurationMS: 12000, SizeBytes: 2 << 20, SHA256: digest,
		ETag: CreateStrongStreamETag(digest), StorageKey: "stream/v1/" + digest,
		ChunkSizeBytes: 1 << 20, CreatedAt: now,
		Chunks: []StreamChunk{
			{Index: 0, Start: 0, End: (1 << 20) - 1, Bytes: 1 << 20, SHA256: strings.Repeat("c", 64)},
			{Index: 1, Start: 1 << 20, End: (2 << 20) - 1, Bytes: 1 << 20, SHA256: strings.Repeat("d", 64)},
		},
		SeekMap: []StreamSeekPoint{{TimeMS: 0, Offset: 0}, {TimeMS: 10000, Offset: 1 << 20}},
	}
}

func TestStreamVariantPinnedProfileRangeLifecycleAndNoGo(t *testing.T) {
	st, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media, metadata := createStreamTrackFixture(t, st, credentials, now)
	if metadata.MediaID != media.ID || metadata.OriginalCodec != "flac" || metadata.Revision != 1 {
		t.Fatalf("metadata=%+v", metadata)
	}

	variant, err := st.CreateStagedStreamVariant(streamVariantParams(media.ID, "mp3-cbr-128-v1", now+2))
	if err != nil {
		t.Fatal(err)
	}
	ready, err := st.PublishStreamVariant(variant.ID, variant.Revision, now+3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PublishStreamVariant(variant.ID, variant.Revision, now+4); !errors.Is(err, ErrStreamTrackConflict) {
		t.Fatalf("stale publish error=%v", err)
	}
	pinned, err := st.GetReadyStreamVariantForProfile(media.ID, "mp3-cbr-128-v1")
	if err != nil || pinned.ID != ready.ID {
		t.Fatalf("pinned=%+v err=%v", pinned, err)
	}
	listed, err := st.ListStreamVariants(media.ID)
	if err != nil || len(listed) != 1 || listed[0].ID != ready.ID {
		t.Fatalf("listed=%+v err=%v", listed, err)
	}
	byteRange, err := pinned.ByteRangeForTime(11500)
	if err != nil || byteRange.Start != 1<<20 || byteRange.End != (2<<20)-1 || byteRange.SeekTimeMS != 10000 {
		t.Fatalf("range=%+v err=%v", byteRange, err)
	}
	if _, err := st.SelectProductionStreamVariant(media.ID); !errors.Is(err, ErrStreamProductionSelectionLocked) {
		t.Fatalf("production selector error=%v", err)
	}
	if _, err := st.db.Exec(`UPDATE stream_variants SET bitrate_bps = 192000 WHERE id = ?`, ready.ID); err == nil {
		t.Fatal("immutable variant payload update unexpectedly succeeded")
	}
	revoked, err := st.RevokeStreamVariant(ready.ID, ready.Revision, now+5)
	if err != nil || revoked.Status != "revoked" {
		t.Fatalf("revoked=%+v err=%v", revoked, err)
	}
	if _, err := st.RevokeStreamVariant(revoked.ID, revoked.Revision, now+6); !errors.Is(err, ErrStreamTrackConflict) {
		t.Fatalf("terminal revoke error=%v", err)
	}
}

func TestStreamVariantValidationAndConditionalWorkerRace(t *testing.T) {
	st, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media, _ := createStreamTrackFixture(t, st, credentials, now)
	invalid := streamVariantParams(media.ID, "bad", now+2)
	invalid.SeekMap[1].Offset = 7
	if _, err := st.CreateStagedStreamVariant(invalid); !errors.Is(err, ErrStreamTrackInvalid) {
		t.Fatalf("unaligned seek error=%v", err)
	}
	variant, err := st.CreateStagedStreamVariant(streamVariantParams(media.ID, "race", now+3))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := st.PublishStreamVariant(variant.ID, variant.Revision, now+4)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	var success, conflict int
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrStreamTrackConflict) {
			conflict++
		} else {
			t.Fatalf("unexpected worker error=%v", err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("worker outcomes success=%d conflict=%d", success, conflict)
	}
}

func TestStreamPlaybackQueueSeekGenerationAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream-restart.db")
	st, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := st.CreateSelfServiceOrbit("Stream restart")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media, _ := createStreamTrackFixture(t, st, credentials, now)
	variant, err := st.CreateStagedStreamVariant(streamVariantParams(media.ID, "restart", now+2))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = st.PublishStreamVariant(variant.ID, variant.Revision, now+3); err != nil {
		t.Fatal(err)
	}
	domain, err := st.EnsureStreamPlaybackDomain("orbit", "orbit-"+strings.Repeat("x", 8), now+4)
	if err != nil {
		t.Fatal(err)
	}
	domain, err = st.PinStreamMainProgramSource(domain.ID, "spotify", "spotify:track:resume", domain.Revision, now+5)
	if err != nil || domain.MainProgramKind != "spotify" || domain.MainProgramRef != "spotify:track:resume" {
		t.Fatalf("main program=%+v err=%v", domain, err)
	}
	item, err := st.EnqueueStreamTrack(domain.ID, media.ID, "restart", domain.Revision, now+6)
	if err != nil {
		t.Fatal(err)
	}
	domain, err = st.LoadStreamPlaybackDomainByTarget(domain.TargetKind, domain.TargetRef)
	if err != nil {
		t.Fatal(err)
	}
	domain, err = st.ActivateStreamQueueItem(domain.ID, item.ID, domain.Revision, now+7)
	if err != nil || domain.PlaybackGeneration != 1 || domain.SeekGeneration != 0 {
		t.Fatalf("active=%+v err=%v", domain, err)
	}
	domain, err = st.RecordStreamAudibleProgress(domain.ID, domain.Revision, 1, 0, 4000, "playing", now+8)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.RecordStreamAudibleProgress(domain.ID, domain.Revision, 0, 0, 4100, "playing", now+9); !errors.Is(err, ErrStreamTrackInvalid) {
		t.Fatalf("invalid playback generation error=%v", err)
	}
	domain, err = st.SeekStreamPlayback(domain.ID, domain.Revision, 1, 9000, now+10)
	if err != nil || domain.SeekGeneration != 1 || domain.AudiblePositionMS != 9000 {
		t.Fatalf("seek=%+v err=%v", domain, err)
	}
	if _, err := st.RecordStreamAudibleProgress(domain.ID, domain.Revision, 1, 0, 9500, "playing", now+11); !errors.Is(err, ErrStreamTrackConflict) {
		t.Fatalf("late pre-seek output error=%v", err)
	}
	domain, err = st.RecordStreamAudibleProgress(domain.ID, domain.Revision, 1, 1, 9100, "playing", now+12)
	if err != nil {
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
	restored, err := st.LoadStreamPlaybackDomainByTarget(domain.TargetKind, domain.TargetRef)
	if err != nil || restored.CurrentQueueItemID != item.ID || restored.AudiblePositionMS != 9100 ||
		restored.PlaybackGeneration != 1 || restored.SeekGeneration != 1 || len(restored.Queue) != 1 ||
		restored.MainProgramKind != "spotify" || restored.MainProgramRef != "spotify:track:resume" {
		t.Fatalf("restored=%+v err=%v", restored, err)
	}
}

func TestStreamTrackTablesDoNotDuplicatePhaseOneAuthorities(t *testing.T) {
	st, credentials := newMediaIngestTestStore(t)
	tables := []string{"elements", "settings", "transmissions", "transmission_inbox_items", "media"}
	before := make(map[string]int)
	for _, table := range tables {
		var count int
		if err := st.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		before[table] = count
	}
	now := time.Now().UnixMilli()
	media, _ := createStreamTrackFixture(t, st, credentials, now)
	variant, err := st.CreateStagedStreamVariant(streamVariantParams(media.ID, "authority", now+2))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PublishStreamVariant(variant.ID, variant.Revision, now+3); err != nil {
		t.Fatal(err)
	}
	domain, err := st.EnsureStreamPlaybackDomain("orbit", "authority", now+4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.EnqueueStreamTrack(domain.ID, media.ID, "authority", domain.Revision, now+5); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		var after int
		if err := st.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if after != before[table] {
			t.Fatalf("legacy/Phase 1 authority %s changed from %d to %d", table, before[table], after)
		}
	}
}
