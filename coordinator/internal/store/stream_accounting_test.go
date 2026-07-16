package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func streamPolicyForScope(t *testing.T, st *Store, scopeKind string, scopeID, now int64) StreamQuotaPolicy {
	t.Helper()
	usage, err := st.GetStreamAccountingUsage(scopeKind, scopeID, now)
	if err != nil {
		t.Fatal(err)
	}
	policy := usage.Policy
	policy.ScopeKind = scopeKind
	policy.ScopeID = scopeID
	policy.Revision = 0
	policy.UpdatedAt = 0
	return policy
}

func streamAccountingDecider(t *testing.T, st *Store, now int64) ModerationOperatorCredential {
	t.Helper()
	operator, err := st.ProvisionModerationOperator(
		"Stream accounting test operator",
		ModerationOperatorCapabilities{List: true, Decide: true}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	return operator
}

func TestStreamAccountingApprovedEngineeringDefaults(t *testing.T) {
	st, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	actor, err := st.GetStreamAccountingUsage("actor", credentials.ActorID, now)
	if err != nil {
		t.Fatal(err)
	}
	orbit, err := st.GetStreamAccountingUsage("orbit", credentials.OrbitID, now)
	if err != nil {
		t.Fatal(err)
	}
	if actor.Policy.MaxUploadStarts24h != 100 || actor.Policy.MaxInputBytes24h != 5<<30 ||
		actor.Policy.MaxCanonicalBytes != 10<<30 || actor.Policy.MaxTempProcessingBytes != 2<<30 ||
		actor.Policy.MaxConcurrentJobs != 2 || actor.Policy.MaxRetainedBytes != 20<<30 ||
		actor.Policy.MaxEgressBytes24h != 100<<30 || actor.Policy.EgressAmplificationMilli != 2000 {
		t.Fatalf("actor defaults=%+v", actor.Policy)
	}
	if orbit.Policy.MaxUploadStarts24h != 500 || orbit.Policy.MaxInputBytes24h != 25<<30 ||
		orbit.Policy.MaxCanonicalBytes != 50<<30 || orbit.Policy.MaxTempProcessingBytes != 8<<30 ||
		orbit.Policy.MaxConcurrentJobs != 8 || orbit.Policy.MaxRetainedBytes != 100<<30 ||
		orbit.Policy.MaxEgressBytes24h != 500<<30 || orbit.Policy.EgressAmplificationMilli != 2000 {
		t.Fatalf("orbit defaults=%+v", orbit.Policy)
	}
}

func TestStreamAccountingSchemaMigrationIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stream-accounting-atomic.db")
	_, err := openWithOptionsAndCheckpoint(path, Options{}, func(name string) error {
		if name == "stream_accounting_ddl_before_commit" {
			return errors.New("injected stream accounting DDL failure")
		}
		return nil
	})
	if err == nil {
		t.Fatal("injected stream accounting DDL failure unexpectedly opened store")
	}
	inspect, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	var tables int
	if err := inspect.QueryRow(`SELECT COUNT(*) FROM sqlite_master
WHERE type = 'table' AND name IN (
  'stream_accounting_policies', 'stream_processing_jobs',
  'stream_egress_sessions', 'stream_egress_events'
)`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatalf("partial accounting schema tables=%d", tables)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	snapshot, err := reopened.StreamAccountingSnapshot(time.Now().UnixMilli())
	if err != nil || !snapshot.Ready {
		t.Fatalf("reopened accounting snapshot=%+v err=%v", snapshot, err)
	}
}

func TestStreamAccountingReconcilesProcessingStorageEgressAndDelete(t *testing.T) {
	st, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media, _ := createStreamTrackFixture(t, st, credentials, now)

	usage, err := st.GetStreamAccountingUsage("actor", credentials.ActorID, now+2)
	if err != nil || usage.RetainedStorageBytes != 4<<20 || usage.CanonicalBytes != 0 {
		t.Fatalf("initial usage=%+v err=%v", usage, err)
	}
	job, err := st.BeginStreamProcessing(BeginStreamProcessingParams{
		MediaID: media.ID, IdempotencyKey: "processing-attempt-0001",
		TempReservedBytes: 8 << 20, CreatedAt: now + 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := st.BeginStreamProcessing(BeginStreamProcessingParams{
		MediaID: media.ID, IdempotencyKey: "processing-attempt-0001",
		TempReservedBytes: 8 << 20, CreatedAt: now + 3,
	})
	if err != nil || !replay.Reused || replay.ID != job.ID {
		t.Fatalf("processing replay=%+v err=%v", replay, err)
	}
	job, err = st.RecordStreamProcessingTemp(job.ID, job.Revision, 3<<20, now+4)
	if err != nil || job.TempCurrentBytes != 3<<20 || job.TempHighWaterBytes != 3<<20 {
		t.Fatalf("processing temp=%+v err=%v", job, err)
	}
	variant, err := st.CreateStagedStreamVariant(streamVariantParams(media.ID, "accounting", now+5))
	if err != nil {
		t.Fatal(err)
	}
	variant, err = st.PublishStreamVariant(variant.ID, variant.Revision, now+6)
	if err != nil {
		t.Fatal(err)
	}
	job, err = st.CompleteStreamProcessing(job.ID, job.Revision, "published", now+7)
	if err != nil || job.State != "succeeded" || job.TempCurrentBytes != 0 {
		t.Fatalf("completed processing=%+v err=%v", job, err)
	}

	session, err := st.BeginStreamEgress(BeginStreamEgressParams{
		VariantID: variant.ID, IdempotencyKey: "playback-session-0001",
		PlaybackGeneration: 1, CreatedAt: now + 8,
	})
	if err != nil || session.ReservedBytes != 4<<20 {
		t.Fatalf("egress session=%+v err=%v", session, err)
	}
	first := RecordStreamEgressParams{
		SessionID: session.ID, RequestKey: "range-request-000001",
		ExpectedRevision: session.Revision, RangeStart: 0, RangeEnd: (1 << 20) - 1,
		ActualBytes: 1 << 20, Outcome: "served", CreatedAt: now + 9,
	}
	session, event, err := st.RecordStreamEgress(first)
	if err != nil || event.ActualBytes != 1<<20 || session.RangeRequests != 1 {
		t.Fatalf("first egress session=%+v event=%+v err=%v", session, event, err)
	}
	replayedSession, replayedEvent, err := st.RecordStreamEgress(first)
	if err != nil || !replayedEvent.Reused || replayedSession.ID != session.ID ||
		replayedSession.ActualBytes != session.ActualBytes {
		t.Fatalf("egress replay session=%+v event=%+v err=%v", replayedSession, replayedEvent, err)
	}
	session, _, err = st.RecordStreamEgress(RecordStreamEgressParams{
		SessionID: session.ID, RequestKey: "range-request-000002",
		ExpectedRevision: session.Revision, RangeStart: 1 << 20, RangeEnd: (2 << 20) - 1,
		ActualBytes: 1 << 20, Outcome: "cache_refill", CreatedAt: now + 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err = st.CompleteStreamEgress(session.ID, session.Revision, "completed", now+11)
	if err != nil || session.State != "completed" {
		t.Fatalf("completed egress=%+v err=%v", session, err)
	}
	usage, err = st.GetStreamAccountingUsage("actor", credentials.ActorID, now+12)
	if err != nil || usage.CanonicalBytes != 2<<20 || usage.RetainedStorageBytes != 6<<20 ||
		usage.TempProcessingBytes != 0 || usage.ConcurrentJobs != 0 ||
		usage.RangeRequests24h != 2 || usage.ActualEgressBytes24h != 2<<20 ||
		usage.ActiveEgressReservedBytes != 0 {
		t.Fatalf("reconciled usage=%+v err=%v", usage, err)
	}

	if _, err := st.DeleteAuthorizedMedia(
		credentials.ActorID, credentials.ControlToken, media.ID, now+13,
	); err != nil {
		t.Fatal(err)
	}
	usage, err = st.GetStreamAccountingUsage("actor", credentials.ActorID, now+14)
	if err != nil || usage.RetainedStorageBytes != 0 || usage.CanonicalBytes != 0 ||
		usage.ActualEgressBytes24h != 2<<20 {
		t.Fatalf("post-delete usage=%+v err=%v", usage, err)
	}
	for _, table := range []string{
		"stream_processing_jobs", "stream_egress_sessions", "stream_egress_events",
		"stream_accounting_policies", "stream_accounting_policy_audit",
	} {
		var ddl string
		if err := st.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&ddl); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(ddl), "filename") || strings.Contains(ddl, "long-track.flac") {
			t.Fatalf("private filename reached accounting table %s: %s", table, ddl)
		}
	}
}

func TestStreamAccountingQuotasAreScopedAuditedAndDoNotInterruptActivePlayback(t *testing.T) {
	st, first := newMediaIngestTestStore(t)
	second, err := st.CreateSelfServiceOrbit("Accounting neighbor")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	operator := streamAccountingDecider(t, st, now-1)
	firstMedia, _ := createStreamTrackFixture(t, st, first, now)
	secondMedia, _ := createStreamTrackFixture(t, st, second, now+1)
	policy := streamPolicyForScope(t, st, "actor", first.ActorID, now+2)
	policy.MaxTempProcessingBytes = 1 << 20
	policy.MaxConcurrentJobs = 1
	policy.MaxCanonicalBytes = 1 << 20
	updated, err := st.SetStreamQuotaPolicy(operator.Operator.ID, operator.Token, policy, 0, "tight_actor_test_quota", now+2)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.BeginStreamProcessing(BeginStreamProcessingParams{
		MediaID: firstMedia.ID, IdempotencyKey: "over-temp-attempt-0001",
		TempReservedBytes: 2 << 20, CreatedAt: now + 3,
	})
	var quotaError *StreamQuotaError
	if !errors.As(err, &quotaError) || quotaError.Dimension != StreamQuotaTempBytes ||
		strings.Contains(err.Error(), fmt.Sprint(first.ActorID)) || strings.Contains(err.Error(), fmt.Sprint(policy.MaxTempProcessingBytes)) {
		t.Fatalf("non-private temp quota error=%v", err)
	}
	neighborJob, err := st.BeginStreamProcessing(BeginStreamProcessingParams{
		MediaID: secondMedia.ID, IdempotencyKey: "neighbor-processing-0001",
		TempReservedBytes: 2 << 20, CreatedAt: now + 4,
	})
	if err != nil || neighborJob.ActorID != second.ActorID {
		t.Fatalf("neighbor processing=%+v err=%v", neighborJob, err)
	}
	if _, err := st.CreateStagedStreamVariant(streamVariantParams(firstMedia.ID, "too-large", now+5)); !errors.Is(err, ErrStreamQuotaExceeded) {
		t.Fatalf("canonical quota error=%v", err)
	}
	updated.MaxTempProcessingBytes = 8 << 20
	updated.MaxCanonicalBytes = 8 << 20
	updated.MaxEgressBytes24h = 8 << 20
	updated, err = st.SetStreamQuotaPolicy(
		operator.Operator.ID, operator.Token, updated, updated.Revision, "permit_bounded_test_work", now+6,
	)
	if err != nil {
		t.Fatal(err)
	}
	orbitPolicy := streamPolicyForScope(t, st, "orbit", first.OrbitID, now+6)
	orbitPolicy.MaxTempProcessingBytes = 512 << 10
	orbitPolicy, err = st.SetStreamQuotaPolicy(
		operator.Operator.ID, operator.Token, orbitPolicy, 0, "tight_orbit_temp_quota", now+7,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.BeginStreamProcessing(BeginStreamProcessingParams{
		MediaID: firstMedia.ID, IdempotencyKey: "orbit-temp-attempt-0001",
		TempReservedBytes: 1 << 20, CreatedAt: now + 8,
	})
	if !errors.As(err, &quotaError) || quotaError.Dimension != StreamQuotaTempBytes {
		t.Fatalf("orbit temp quota error=%v", err)
	}
	orbitPolicy.MaxTempProcessingBytes = 8 << 30
	if _, err := st.SetStreamQuotaPolicy(
		operator.Operator.ID, operator.Token, orbitPolicy, orbitPolicy.Revision,
		"restore_orbit_temp_quota", now+9,
	); err != nil {
		t.Fatal(err)
	}
	firstJob, err := st.BeginStreamProcessing(BeginStreamProcessingParams{
		MediaID: firstMedia.ID, IdempotencyKey: "bounded-processing-0001",
		TempReservedBytes: 1 << 20, CreatedAt: now + 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.BeginStreamProcessing(BeginStreamProcessingParams{
		MediaID: firstMedia.ID, IdempotencyKey: "bounded-processing-0002",
		TempReservedBytes: 1 << 20, CreatedAt: now + 11,
	})
	if !errors.As(err, &quotaError) || quotaError.Dimension != StreamQuotaConcurrentJobs {
		t.Fatalf("concurrent processing quota error=%v", err)
	}
	if _, err := st.CompleteStreamProcessing(
		firstJob.ID, firstJob.Revision, "cancelled", now+12,
	); err != nil {
		t.Fatal(err)
	}
	variant, err := st.CreateStagedStreamVariant(streamVariantParams(firstMedia.ID, "bounded", now+13))
	if err != nil {
		t.Fatal(err)
	}
	variant, err = st.PublishStreamVariant(variant.ID, variant.Revision, now+14)
	if err != nil {
		t.Fatal(err)
	}
	session, err := st.BeginStreamEgress(BeginStreamEgressParams{
		VariantID: variant.ID, IdempotencyKey: "active-playback-0001",
		PlaybackGeneration: 7, CreatedAt: now + 15,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated.MaxEgressBytes24h = 1
	updated, err = st.SetStreamQuotaPolicy(
		operator.Operator.ID, operator.Token, updated, updated.Revision,
		"lower_quota_without_terminating_playback", now+16,
	)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		session, _, err = st.RecordStreamEgress(RecordStreamEgressParams{
			SessionID: session.ID, RequestKey: fmt.Sprintf("whole-range-request-%04d", index),
			ExpectedRevision: session.Revision, RangeStart: 0, RangeEnd: (2 << 20) - 1,
			ActualBytes: 2 << 20, Outcome: "served", CreatedAt: now + 17 + int64(index),
		})
		if err != nil {
			t.Fatalf("admitted playback range %d interrupted: %v", index, err)
		}
	}
	_, _, err = st.RecordStreamEgress(RecordStreamEgressParams{
		SessionID: session.ID, RequestKey: "amplified-range-0001",
		ExpectedRevision: session.Revision, RangeStart: 0, RangeEnd: (1 << 20) - 1,
		ActualBytes: 1 << 20, Outcome: "cache_refill", CreatedAt: now + 20,
	})
	if !errors.Is(err, ErrStreamRangeAmplification) {
		t.Fatalf("range amplification error=%v", err)
	}
	audit, err := st.ListStreamQuotaPolicyAudit("actor", first.ActorID, 10)
	if err != nil || len(audit) != 3 || audit[0].OperatorID != operator.Operator.ID {
		t.Fatalf("policy audit=%+v err=%v", audit, err)
	}
	snapshot, err := st.StreamAccountingSnapshot(now + 21)
	if err != nil || !snapshot.Ready || !snapshot.Saturated || snapshot.QuotaRejections24h < 2 {
		t.Fatalf("saturation snapshot=%+v err=%v", snapshot, err)
	}
}

func TestStreamAccountingRetainedQuotaRejectsNewTrackWithoutChargingNeighbor(t *testing.T) {
	st, first := newMediaIngestTestStore(t)
	second, err := st.CreateSelfServiceOrbit("Retained quota neighbor")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	operator := streamAccountingDecider(t, st, now-1)
	policy := streamPolicyForScope(t, st, "actor", first.ActorID, now)
	policy.MaxRetainedBytes = 1024
	if _, err := st.SetStreamQuotaPolicy(
		operator.Operator.ID, operator.Token, policy, 0, "retained_storage_boundary", now,
	); err != nil {
		t.Fatal(err)
	}
	firstMedia, err := st.CreateMediaItem(CreateMediaItemParams{
		OwnerOrbitID: first.OrbitID, ActorID: first.ActorID,
		Kind: MediaKindAudioTrack, Source: MediaSourceApp,
		CreatedAt: now + 1, ExpiresAt: now + int64((7*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata := CreateStreamTrackMetadataParams{
		MediaID: firstMedia.ID, OriginalFilename: "private-name.flac",
		OriginalMIME: "audio/flac", OriginalContainer: "flac", OriginalCodec: "flac",
		OriginalSizeBytes: 2048, OriginalSHA256: strings.Repeat("e", 64), CreatedAt: now + 2,
	}
	if _, err := st.CreateStreamTrackMetadata(metadata); !errors.Is(err, ErrStreamQuotaExceeded) {
		t.Fatalf("retained quota error=%v", err)
	}
	secondMedia, err := st.CreateMediaItem(CreateMediaItemParams{
		OwnerOrbitID: second.OrbitID, ActorID: second.ActorID,
		Kind: MediaKindAudioTrack, Source: MediaSourceApp,
		CreatedAt: now + 3, ExpiresAt: now + int64((7*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata.MediaID = secondMedia.ID
	metadata.CreatedAt = now + 4
	if _, err := st.CreateStreamTrackMetadata(metadata); err != nil {
		t.Fatalf("neighbor retained storage was charged: %v", err)
	}
	firstUsage, err := st.GetStreamAccountingUsage("actor", first.ActorID, now+5)
	if err != nil || firstUsage.RetainedStorageBytes != 0 {
		t.Fatalf("rejected retained usage=%+v err=%v", firstUsage, err)
	}
}

func TestStreamAccountingRetentionRemovesOriginalAndVariantFromCurrentStorage(t *testing.T) {
	st, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media, err := st.CreateMediaItem(CreateMediaItemParams{
		OwnerOrbitID: credentials.OrbitID, ActorID: credentials.ActorID,
		Kind: MediaKindAudioTrack, Source: MediaSourceApp,
		CreatedAt: now, ExpiresAt: now + 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateStreamTrackMetadata(CreateStreamTrackMetadataParams{
		MediaID: media.ID, OriginalFilename: "retention.flac",
		OriginalMIME: "audio/flac", OriginalContainer: "flac", OriginalCodec: "flac",
		OriginalSizeBytes: 4 << 20, OriginalSHA256: strings.Repeat("f", 64), CreatedAt: now + 1,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateStagedStreamVariant(streamVariantParams(media.ID, "retention", now+1)); err != nil {
		t.Fatal(err)
	}
	before, err := st.GetStreamAccountingUsage("actor", credentials.ActorID, now+1)
	if err != nil || before.RetainedStorageBytes != 6<<20 {
		t.Fatalf("pre-retention usage=%+v err=%v", before, err)
	}
	if _, err := st.ExpireMediaItem(media.ID, media.Revision, now+3); err != nil {
		t.Fatal(err)
	}
	after, err := st.GetStreamAccountingUsage("actor", credentials.ActorID, now+4)
	if err != nil || after.RetainedStorageBytes != 0 || after.CanonicalBytes != 0 {
		t.Fatalf("post-retention usage=%+v err=%v", after, err)
	}
}

func TestStreamAccountingOperatorReadsAndMutationsRecheckLiveCapability(t *testing.T) {
	st, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	reader, err := st.ProvisionModerationOperator(
		"Accounting reader", ModerationOperatorCapabilities{List: true}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	decider, err := st.ProvisionModerationOperator(
		"Accounting decider", ModerationOperatorCapabilities{Decide: true}, now+1,
	)
	if err != nil {
		t.Fatal(err)
	}
	policy := streamPolicyForScope(t, st, "actor", credentials.ActorID, now+2)
	if _, err := st.SetStreamQuotaPolicy(
		reader.Operator.ID, reader.Token, policy, 0, "reader_cannot_mutate", now+3,
	); !errors.Is(err, ErrModerationForbidden) {
		t.Fatalf("list-only mutation error=%v", err)
	}
	updated, err := st.SetStreamQuotaPolicy(
		decider.Operator.ID, decider.Token, policy, 0, "decider_mutation", now+4,
	)
	if err != nil || updated.Revision != 1 {
		t.Fatalf("decider mutation=%+v err=%v", updated, err)
	}
	view, err := st.GetAuthorizedStreamAccounting(
		reader.Operator.ID, reader.Token, "actor", credentials.ActorID, now+5,
	)
	if err != nil || view.Usage == nil || view.Usage.ScopeID != credentials.ActorID {
		t.Fatalf("authorized accounting view=%+v err=%v", view, err)
	}
	if revoked, err := st.RevokeModerationOperator(reader.Operator.ID, now+6); err != nil || !revoked {
		t.Fatalf("revoke reader=%v err=%v", revoked, err)
	}
	if _, err := st.GetAuthorizedStreamAccounting(
		reader.Operator.ID, reader.Token, "", 0, now+7,
	); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked reader view error=%v", err)
	}
	if revoked, err := st.RevokeModerationOperator(decider.Operator.ID, now+8); err != nil || !revoked {
		t.Fatalf("revoke decider=%v err=%v", revoked, err)
	}
	if _, err := st.SetStreamQuotaPolicy(
		decider.Operator.ID, decider.Token, updated, updated.Revision,
		"revoked_decider_mutation", now+9,
	); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked decider mutation error=%v", err)
	}
}

func TestStreamAccountingCrashReleasesReservationsAndAllowsRetry(t *testing.T) {
	st, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	media, _ := createStreamTrackFixture(t, st, credentials, now)
	variant, err := st.CreateStagedStreamVariant(streamVariantParams(media.ID, "crash", now+1))
	if err != nil {
		t.Fatal(err)
	}
	variant, err = st.PublishStreamVariant(variant.ID, variant.Revision, now+2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.BeginStreamProcessing(BeginStreamProcessingParams{
		MediaID: media.ID, IdempotencyKey: "crashed-processing-0001",
		TempReservedBytes: 4 << 20, CreatedAt: now + 3,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.BeginStreamEgress(BeginStreamEgressParams{
		VariantID: variant.ID, IdempotencyKey: "crashed-playback-0001",
		PlaybackGeneration: 1, CreatedAt: now + 3,
	}); err != nil {
		t.Fatal(err)
	}
	before, err := st.StreamAccountingSnapshot(now + 4000)
	if err != nil {
		t.Fatal(err)
	}
	st.testCheckpoint = func(name string) error {
		if name == "stream_accounting_reconcile_before_commit" {
			return errors.New("injected accounting reconcile crash")
		}
		return nil
	}
	if _, err := st.ReconcileStreamAccounting(now+5000, time.Second); err == nil {
		t.Fatal("injected reconciliation crash unexpectedly committed")
	}
	st.testCheckpoint = nil
	afterFailed, err := st.StreamAccountingSnapshot(now + 5000)
	if err != nil || afterFailed.LastReconciledAt != before.LastReconciledAt ||
		afterFailed.ConcurrentJobs != 1 || afterFailed.ActiveEgressReservedBytes == 0 {
		t.Fatalf("partial failed reconciliation=%+v before=%+v err=%v", afterFailed, before, err)
	}
	reconciled, err := st.ReconcileStreamAccounting(now+5000, time.Second)
	if err != nil || reconciled.ProcessingCrashReleases != 1 || reconciled.EgressCrashReleases != 1 {
		t.Fatalf("reconcile=%+v err=%v", reconciled, err)
	}
	usage, err := st.GetStreamAccountingUsage("actor", credentials.ActorID, now+5001)
	if err != nil || usage.ConcurrentJobs != 0 || usage.TempProcessingReserved != 0 ||
		usage.ActiveEgressReservedBytes != 0 {
		t.Fatalf("released usage=%+v err=%v", usage, err)
	}
	retry, err := st.BeginStreamProcessing(BeginStreamProcessingParams{
		MediaID: media.ID, IdempotencyKey: "processing-retry-0001",
		TempReservedBytes: 4 << 20, CreatedAt: now + 5002,
	})
	if err != nil || retry.State != "active" {
		t.Fatalf("processing retry=%+v err=%v", retry, err)
	}
}

func TestStreamTrackUploadQuotasUseAuthoritativeSessionReservations(t *testing.T) {
	st, credentials := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	operator := streamAccountingDecider(t, st, now-1)
	policy := streamPolicyForScope(t, st, "actor", credentials.ActorID, now)
	policy.MaxUploadStarts24h = 1
	policy.MaxInputBytes24h = 150
	updated, err := st.SetStreamQuotaPolicy(operator.Operator.ID, operator.Token, policy, 0, "upload_reservation_test", now)
	if err != nil {
		t.Fatal(err)
	}
	params := authorizedUploadParams(credentials, now+1, "stream-upload-quota-0001", 100)
	params.Media.Kind = MediaKindAudioTrack
	created, err := st.CreateAuthorizedMediaUpload(
		credentials.ActorID, credentials.ControlToken, params, permissiveMediaUploadQuota(),
	)
	if err != nil {
		t.Fatal(err)
	}
	replay := params
	replay.Media.CreatedAt++
	replay.Media.ExpiresAt++
	replay.SessionExpiresAt++
	if reused, err := st.CreateAuthorizedMediaUpload(
		credentials.ActorID, credentials.ControlToken, replay, permissiveMediaUploadQuota(),
	); err != nil || !reused.Reused || reused.Session.ID != created.Session.ID {
		t.Fatalf("quota replay=%+v err=%v", reused, err)
	}
	second := authorizedUploadParams(credentials, now+3, "stream-upload-quota-0002", 1)
	second.Media.Kind = MediaKindAudioTrack
	if _, err := st.CreateAuthorizedMediaUpload(
		credentials.ActorID, credentials.ControlToken, second, permissiveMediaUploadQuota(),
	); !errors.Is(err, ErrStreamQuotaExceeded) {
		t.Fatalf("upload starts quota error=%v", err)
	}
	updated.MaxUploadStarts24h = 10
	if _, err := st.SetStreamQuotaPolicy(
		operator.Operator.ID, operator.Token, updated, updated.Revision, "test_input_byte_reservation", now+4,
	); err != nil {
		t.Fatal(err)
	}
	second.Media.CreatedAt = now + 5
	second.Media.ExpiresAt = now + int64((7*24*time.Hour)/time.Millisecond)
	second.SessionExpiresAt = now + int64(time.Hour/time.Millisecond)
	second.DeclaredSizeBytes = 100
	if _, err := st.CreateAuthorizedMediaUpload(
		credentials.ActorID, credentials.ControlToken, second, permissiveMediaUploadQuota(),
	); !errors.Is(err, ErrStreamQuotaExceeded) {
		t.Fatalf("upload input quota error=%v", err)
	}
	usage, err := st.GetStreamAccountingUsage("actor", credentials.ActorID, now+6)
	if err != nil || usage.UploadStarts24h != 1 || usage.InputBytes24h != 100 {
		t.Fatalf("upload usage=%+v err=%v", usage, err)
	}
}
