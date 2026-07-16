package store

import (
	"crypto/sha256"
	"errors"
	"fmt"
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

func TestStreamVariantPersistedTargetAuthorizationAndReporterLocalHide(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	reporter, err := st.CreateSelfServiceOrbit("Stream reporter target")
	if err != nil {
		t.Fatal(err)
	}
	neighbor, err := st.CreateSelfServiceOrbit("Stream neighbor target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media, _ := createStreamTrackFixture(t, st, source, now)
	publication, err := st.StageMediaPublication(media.ID, media.Revision, now+2)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := st.CompleteMediaPublication(publication.ID, publication.Revision, MediaPublication{
		MIME: "audio/mpeg", Codec: "mp3", DurationMS: 12000, SizeBytes: 2 << 20,
		SHA256:       strings.Repeat("e", 64),
		LoudnessJSON: `{"input_i":"-14","input_tp":"-1","output_i":"-14","output_tp":"-1"}`,
	}, now+3)
	if err != nil {
		t.Fatal(err)
	}
	variant, err := st.CreateStagedStreamVariant(streamVariantParams(ready.ID, "auth-range-v1", now+4))
	if err != nil {
		t.Fatal(err)
	}
	variant, err = st.PublishStreamVariant(variant.ID, variant.Revision, now+5)
	if err != nil {
		t.Fatal(err)
	}
	transmissionID := "tr_00000000000000000000000001"
	if _, err := st.db.Exec(`INSERT INTO transmissions(
id, media_id, source_orbit_id, source_actor_id, source_slot,
playback_domain_kind, playback_domain_id, audience_kind, origin_kind,
include_origin, requested_delivery, effective_delivery, accepted_at, expires_at, updated_at
) VALUES(?, ?, ?, ?, ?, 'orbit', ?, 'explicit', 'file', 0, 'overlay', 'overlay', ?, ?, ?)`,
		transmissionID, ready.ID, source.OrbitID, source.ActorID, source.Slot,
		source.OrbitID, now+6, now+int64(time.Hour/time.Millisecond), now+6,
	); err != nil {
		t.Fatal(err)
	}
	for _, target := range []OnboardingCredentials{reporter, neighbor} {
		var pairedAt int64
		if err := st.db.QueryRow(`SELECT slot_paired_at FROM installation_credentials
WHERE actor_id = ? AND slot_orbit_id = ? AND slot_name = ?`,
			target.ActorID, target.OrbitID, target.Slot,
		).Scan(&pairedAt); err != nil {
			t.Fatal(err)
		}
		if _, err := st.db.Exec(`INSERT INTO transmission_targets(
transmission_id, orbit_id, actor_id, slot, binding_paired_at, resolved_at_ms,
online_at_acceptance, media_clip_capable, overlay_capable,
interrupt_capable, interrupt_resume_ready, updated_at
) VALUES(?, ?, ?, ?, ?, ?, 1, 0, 0, 0, 0, ?)`,
			transmissionID, target.OrbitID, target.ActorID, target.Slot,
			pairedAt, now+6, now+6,
		); err != nil {
			t.Fatal(err)
		}
	}
	open := func(credentials OnboardingCredentials) error {
		ctx, err := st.ResolveTokenActorContext(credentials.NodeToken)
		if err != nil {
			return err
		}
		_, err = st.WithAuthorizedPersistedStreamVariantDownload(
			ctx, credentials.NodeToken,
			MediaTargetIdentity{MediaID: ready.ID, OrbitID: ctx.OrbitID, ActorID: ctx.ActorID, Slot: ctx.Slot},
			variant.ID, now+7, func(candidate StreamVariant) error {
				if candidate.ID != variant.ID || candidate.ETag != variant.ETag {
					return errors.New("wrong immutable variant")
				}
				return nil
			},
		)
		return err
	}
	if err := open(reporter); err != nil {
		t.Fatalf("reporter initial open=%v", err)
	}
	if err := open(neighbor); err != nil {
		t.Fatalf("neighbor initial open=%v", err)
	}
	controlContext, err := st.ResolveTokenActorContext(source.ControlToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.WithAuthorizedPersistedStreamVariantDownload(
		controlContext, source.ControlToken,
		MediaTargetIdentity{MediaID: ready.ID, OrbitID: source.OrbitID, ActorID: source.ActorID, Slot: source.Slot},
		variant.ID, now+7, func(StreamVariant) error { return nil },
	); !errors.Is(err, ErrStreamTrackNotFound) {
		t.Fatalf("owner control stream error=%v", err)
	}
	if _, err := st.CreateModerationReport(reporter.ActorID, reporter.ControlToken, CreateModerationReportParams{
		MediaID: ready.ID, Reason: ModerationReasonSpam, CreatedAt: now + 8,
	}); err != nil {
		t.Fatal(err)
	}
	if err := open(reporter); !errors.Is(err, ErrStreamTrackNotFound) {
		t.Fatalf("reporter local hide error=%v", err)
	}
	if err := open(neighbor); err != nil {
		t.Fatalf("plain report affected neighbor=%v", err)
	}
	neighborContext, err := st.ResolveTokenActorContext(neighbor.NodeToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.WithAuthorizedPersistedStreamVariantDownload(
		neighborContext, neighbor.NodeToken,
		MediaTargetIdentity{MediaID: ready.ID, OrbitID: neighbor.OrbitID, ActorID: neighbor.ActorID, Slot: neighbor.Slot},
		"sv_00000000000000000000000000", now+9, func(StreamVariant) error { return nil },
	); !errors.Is(err, ErrStreamTrackNotFound) {
		t.Fatalf("unknown variant error=%v", err)
	}
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

func TestStreamPlaybackQueueReplaceRebufferCompletionAndRestartFence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream-flow.db")
	st, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := st.CreateSelfServiceOrbit("Stream coordinator flow")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	createReady := func(label string, at int64) (MediaItem, StreamVariant) {
		media, _ := createStreamTrackFixture(t, st, credentials, at)
		params := streamVariantParams(media.ID, label, at+1)
		digest := fmt.Sprintf("%x", sha256.Sum256([]byte(label)))
		params.SHA256 = digest
		params.ETag = CreateStrongStreamETag(digest)
		params.StorageKey = "stream/v1/" + digest
		variant, err := st.CreateStagedStreamVariant(params)
		if err != nil {
			t.Fatal(err)
		}
		variant, err = st.PublishStreamVariant(variant.ID, variant.Revision, at+2)
		if err != nil {
			t.Fatal(err)
		}
		return media, variant
	}
	firstMedia, firstVariant := createReady("flow-first", now)
	secondMedia, secondVariant := createReady("flow-second", now+10)
	fourthMedia, fourthVariant := createReady("flow-fourth", now+20)
	replacementMedia, replacementVariant := createReady("flow-replacement", now+30)
	domain, err := st.EnsureStreamPlaybackDomain("air", "air-flow", now+40)
	if err != nil {
		t.Fatal(err)
	}
	domain, err = st.PinStreamMainProgramSource(
		domain.ID, "spotify", "spotify:track:parked", domain.Revision, now+41,
	)
	if err != nil {
		t.Fatal(err)
	}
	var first, second, fourth StreamQueueItem
	for index, input := range []struct {
		media   MediaItem
		variant StreamVariant
		out     *StreamQueueItem
	}{{firstMedia, firstVariant, &first}, {secondMedia, secondVariant, &second}, {fourthMedia, fourthVariant, &fourth}} {
		*input.out, err = st.EnqueueStreamTrack(
			domain.ID, input.media.ID, input.variant.Profile, domain.Revision, now+42+int64(index),
		)
		if err != nil {
			t.Fatal(err)
		}
		domain, err = st.LoadStreamPlaybackDomainByTarget("air", "air-flow")
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.ActivateStreamQueueItem(
		domain.ID, second.ID, domain.Revision, now+49,
	); !errors.Is(err, ErrStreamTrackConflict) {
		t.Fatalf("out-of-order activation error=%v", err)
	}
	domain, err = st.ActivateNextStreamQueueItem(domain.ID, domain.Revision, now+50)
	if err != nil || domain.CurrentQueueItemID != first.ID || domain.PlaybackGeneration != 1 {
		t.Fatalf("first activation=%+v err=%v", domain, err)
	}
	if _, err := st.ActivateStreamQueueItem(
		domain.ID, second.ID, domain.Revision, now+51,
	); !errors.Is(err, ErrStreamTrackConflict) {
		t.Fatalf("parallel activation error=%v", err)
	}
	domain, err = st.RecordStreamAudibleProgress(
		domain.ID, domain.Revision, 1, 0, 1000, "playing", now+52,
	)
	if err != nil {
		t.Fatal(err)
	}
	domain, err = st.RebufferStreamPlayback(
		domain.ID, domain.Revision, 1, 0, 1200, now+53,
	)
	if err != nil || domain.State != "buffering" || domain.AudiblePositionMS != 1200 {
		t.Fatalf("rebuffer=%+v err=%v", domain, err)
	}
	domain, err = st.RestartStreamPlayback(domain.ID, domain.Revision, 1, now+54)
	if err != nil || domain.PlaybackGeneration != 2 || domain.SeekGeneration != 0 ||
		domain.AudiblePositionMS != 1200 {
		t.Fatalf("restart=%+v err=%v", domain, err)
	}
	if _, err := st.CompleteStreamPlayback(
		domain.ID, domain.Revision, 1, 0, "played", now+55,
	); !errors.Is(err, ErrStreamTrackConflict) {
		t.Fatalf("stale completion error=%v", err)
	}
	domain, err = st.SeekStreamPlayback(domain.ID, domain.Revision, 2, 500, now+56)
	if err != nil || domain.SeekGeneration != 1 || domain.AudiblePositionMS != 500 {
		t.Fatalf("post-restart seek=%+v err=%v", domain, err)
	}
	domain, err = st.RecordStreamAudibleProgress(
		domain.ID, domain.Revision, 2, 1, 700, "playing", now+57,
	)
	if err != nil {
		t.Fatal(err)
	}
	domain, err = st.CompleteStreamPlayback(
		domain.ID, domain.Revision, 2, 1, "played", now+58,
	)
	if err != nil || domain.State != "idle" || domain.SourceKind != "none" ||
		domain.CurrentQueueItemID != "" || domain.MainProgramRef != "spotify:track:parked" {
		t.Fatalf("first completion=%+v err=%v", domain, err)
	}
	if _, err := st.ReplaceStreamTrack(
		domain.ID, replacementMedia.ID, replacementVariant.Profile, domain.Revision, now+59,
	); !errors.Is(err, ErrStreamTrackConflict) {
		t.Fatalf("idle replacement error=%v", err)
	}
	domain, err = st.ActivateNextStreamQueueItem(domain.ID, domain.Revision, now+59)
	if err != nil || domain.CurrentQueueItemID != second.ID || domain.PlaybackGeneration != 3 {
		t.Fatalf("second activation=%+v err=%v", domain, err)
	}
	domain, err = st.ReplaceStreamTrack(
		domain.ID, replacementMedia.ID, replacementVariant.Profile, domain.Revision, now+60,
	)
	if err != nil || domain.PlaybackGeneration != 4 || domain.CurrentQueueItemID == second.ID {
		t.Fatalf("replacement=%+v err=%v", domain, err)
	}
	var states = make(map[string]string)
	for _, item := range domain.Queue {
		states[item.ID] = item.State
	}
	if states[first.ID] != "played" || states[second.ID] != "cancelled" ||
		states[fourth.ID] != "queued" || states[domain.CurrentQueueItemID] != "active" {
		t.Fatalf("replacement queue states=%v", states)
	}
	domain, err = st.RecordStreamAudibleProgress(
		domain.ID, domain.Revision, 4, 0, 4000, "paused", now+61,
	)
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
	restored, err := st.LoadStreamPlaybackDomainByTarget("air", "air-flow")
	if err != nil || restored.CurrentQueueItemID != domain.CurrentQueueItemID ||
		restored.PlaybackGeneration != 4 || restored.AudiblePositionMS != 4000 ||
		restored.State != "paused" || restored.MainProgramRef != "spotify:track:parked" {
		t.Fatalf("restored replacement=%+v err=%v", restored, err)
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
