package store

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPhase2ObservabilityUsesCanonicalBoundedCounters(t *testing.T) {
	st, source := newMediaIngestTestStore(t)
	first, err := st.CreateSelfServiceOrbit("Observability first target")
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateSelfServiceOrbit("Observability second target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	track, _ := createStreamTrackFixture(t, st, source, now)
	publication, err := st.StageMediaPublication(track.ID, track.Revision, now+2)
	if err != nil {
		t.Fatal(err)
	}
	track, err = st.CompleteMediaPublication(publication.ID, publication.Revision, MediaPublication{
		MIME: "audio/mpeg", Codec: "mp3", DurationMS: 60_000, SizeBytes: 4 << 20,
		SHA256: strings.Repeat("d", 64), LoudnessJSON: `{"output_i":"-14"}`,
	}, now+3)
	if err != nil {
		t.Fatal(err)
	}
	variant, err := st.CreateStagedStreamVariant(streamVariantParams(track.ID, "observability", now+4))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.PublishStreamVariant(variant.ID, variant.Revision, now+5); err != nil {
		t.Fatal(err)
	}
	domain, err := st.EnsureStreamPlaybackDomain("orbit", "observability-domain", now+6)
	if err != nil {
		t.Fatal(err)
	}
	item, err := st.EnqueueStreamTrack(domain.ID, track.ID, "observability", domain.Revision, now+7)
	if err != nil {
		t.Fatal(err)
	}
	domain, err = st.LoadStreamPlaybackDomainByTarget("orbit", "observability-domain")
	if err != nil {
		t.Fatal(err)
	}
	domain, err = st.ActivateStreamQueueItem(domain.ID, item.ID, domain.Revision, now+8)
	if err != nil {
		t.Fatal(err)
	}
	domain, err = st.RecordStreamAudibleProgress(domain.ID, domain.Revision, 1, 0, 1000, "playing", now+9)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.SeekStreamPlayback(domain.ID, domain.Revision, 1, 10_000, now+10); err != nil {
		t.Fatal(err)
	}

	// The production track creator remains intentionally disabled by the
	// current codec/player no-go. Seed the shared target lifecycle through the
	// already-proven clip creator, then restore the track kind so this test
	// exercises only the cross-cutting observability query.
	if _, err := st.db.Exec(`UPDATE media_items SET kind='audio_clip' WHERE id=?`, track.ID); err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateTransmission(transmissionParams(
		track, source, now+20, transmissionTarget(first, true), transmissionTarget(second, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	for index, target := range created.Targets {
		readyAt := now + 120 + int64(index*20)
		startedAt := now + 220 + int64(index*40)
		if _, err := st.db.Exec(`UPDATE transmission_targets SET status='playing',
ready_at=?, scheduled_at=?, started_at=?, last_receipt_at=?, updated_at=?, revision=revision+1
WHERE transmission_id=? AND actor_id=?`, readyAt, readyAt+20, startedAt, startedAt,
			startedAt, created.Transmission.ID, target.ActorID); err != nil {
			t.Fatal(err)
		}
	}
	offline := transmissionTarget(first, false)
	offline.Status = TransmissionTargetMissedOffline
	offline.ReasonCode = TransmissionReasonOfflineAtAcceptance
	if _, err := st.CreateTransmission(transmissionParams(track, source, now+300, offline)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE media_items SET kind='audio_track' WHERE id=?`, track.ID); err != nil {
		t.Fatal(err)
	}

	view, err := st.Phase2ObservabilitySnapshot(now + 1000)
	if err != nil {
		t.Fatal(err)
	}
	if view.Contract != "p2-observability-quota-view.v1" || view.WindowSeconds != 86400 ||
		view.Features.StreamedTracks.Enabled || view.Features.AirRooms.Enabled ||
		!view.Features.TargetsInbox.Enabled || !view.Readiness.Ready ||
		!view.Accounting.Ready {
		t.Fatalf("feature/readiness view=%+v", view)
	}
	if view.Timing.ReleaseToReady.SampleCount != 2 || view.Timing.ReleaseToReady.P95MS != 120 ||
		view.Timing.TrackStart.SampleCount != 2 || view.Timing.TrackStart.P95MS != 240 ||
		view.Timing.StartSkew.SampleCount != 1 || view.Timing.StartSkew.P95MS != 40 ||
		view.Timing.SeekToAudio.Status != "client_evidence_required" {
		t.Fatalf("timing=%+v", view.Timing)
	}
	if view.Playback.Domains != 1 || view.Playback.BufferingDomains != 1 ||
		view.Playback.SeekGenerations != 1 || view.Playback.ActiveItems != 1 ||
		view.Playback.BufferSampleStatus != "client_evidence_required" {
		t.Fatalf("playback=%+v", view.Playback)
	}
	if view.Delivery.TargetStatuses.Playing != 2 ||
		view.Delivery.TargetStatuses.MissedOffline != 1 ||
		view.Delivery.InboxReasons.OfflineAtAcceptance != 1 ||
		view.Delivery.DuplicateTargetAnomalies != 0 {
		t.Fatalf("delivery=%+v", view.Delivery)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"actor_id", "orbit_id", "media_id", "transmission_id", "original_filename",
		"long-track.flac", source.ControlToken,
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("sanitized view leaked %q: %s", forbidden, raw)
		}
	}
}

func TestAuthorizedPhase2ObservabilityRechecksListCapability(t *testing.T) {
	st, _ := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	reader, err := st.ProvisionModerationOperator(
		"Phase 2 observability reader", ModerationOperatorCapabilities{List: true}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	view, err := st.GetAuthorizedPhase2Observability(reader.Operator.ID, reader.Token, now+1)
	if err != nil || view.SchemaVersion != 1 {
		t.Fatalf("authorized view=%+v err=%v", view, err)
	}
	if revoked, err := st.RevokeModerationOperator(reader.Operator.ID, now+2); err != nil || !revoked {
		t.Fatalf("revoke=%v err=%v", revoked, err)
	}
	if _, err := st.GetAuthorizedPhase2Observability(reader.Operator.ID, reader.Token, now+3); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked observability error=%v", err)
	}
}
