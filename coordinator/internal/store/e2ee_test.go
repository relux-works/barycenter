package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type e2eeStoreFixture struct {
	store     *Store
	path      string
	owner     OnboardingCredentials
	recipient OnboardingCredentials
	airID     string
	group     E2EEGroup
	now       int64
}

func e2eeDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func newE2EEStoreFixture(t *testing.T) e2eeStoreFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "e2ee-foundation.db")
	st, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	owner, err := st.CreateSelfServiceOrbit("E2EE owner")
	if err != nil {
		t.Fatal(err)
	}
	recipient, err := st.CreateSelfServiceOrbit("E2EE recipient")
	if err != nil {
		t.Fatal(err)
	}
	authority, err := st.AirAuthority()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CutoverLinksToAirs(authority.Generation, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli() + 100
	air, err := st.CreateAir(CreateAirParams{
		Title: "Protected room", OwnerOrbitID: owner.OrbitID, CreatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	register := func(credentials OnboardingCredentials, deviceID string, stamp int64) {
		payload := []byte("public-package:" + deviceID)
		_, err := st.RegisterE2EEPublicDevice(RegisterE2EEPublicDeviceParams{
			DeviceID: deviceID, ProtocolActorID: "actor_" + deviceID,
			ActorID:       credentials.ActorID,
			PublicPackage: payload, PublicPackageDigest: e2eeDigest(payload),
			VerificationState: "verified", VerificationDigest: strings.Repeat("d", 64),
			CreatedAt: stamp,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	register(owner, "device_owner_verified_0001", now+1)
	register(recipient, "device_recipient_verified_1", now+2)
	group, err := st.CreateE2EEGroup(CreateE2EEGroupParams{
		AirID: air.ID, AuthorDeviceID: "device_owner_verified_0001",
		TargetSnapshotDigest: strings.Repeat("a", 64),
		CommitDigest:         strings.Repeat("b", 64),
		Epoch:                7,
		CreatedAt:            now + 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	return e2eeStoreFixture{
		store: st, path: path, owner: owner, recipient: recipient,
		airID: air.ID, group: group, now: now,
	}
}

func e2eeObjectParams(fixture e2eeStoreFixture, sourceID string, stamp int64) StageE2EEProtectedObjectParams {
	digest := strings.Repeat("c", 64)
	manifest := []byte{0x91, 0x07, 0xa3, 0x55}
	return StageE2EEProtectedObjectParams{
		GroupID: fixture.group.ID, SourceObjectID: sourceID, ObjectKind: "clip",
		AuthorDeviceID: "device_owner_verified_0001", Epoch: fixture.group.CurrentEpoch,
		Generation: 1, TargetSnapshotDigest: fixture.group.TargetSnapshotDigest,
		ManifestDigest:     e2eeDigest(manifest),
		EncryptedManifest:  manifest,
		OpaqueKeyEnvelopes: []byte{0xf1, 0x02, 0x7c, 0x99},
		CiphertextRef:      "ciphertext/v1/" + digest, CiphertextDigest: digest,
		CiphertextSize: 8192, ChunkCount: 2, DeclaredDurationMS: 1200,
		CreatedAt: stamp,
	}
}

func TestE2EESchemaIsDormantAndProtectedObjectLifecycleIsConditional(t *testing.T) {
	fixture := newE2EEStoreFixture(t)
	var enabled int
	var suite, container string
	if err := fixture.store.db.QueryRow(`SELECT enabled, selected_suite, selected_container
FROM e2ee_feature_state WHERE singleton = 1`).Scan(&enabled, &suite, &container); err != nil {
		t.Fatal(err)
	}
	if enabled != 0 || suite != "" || container != "" {
		t.Fatalf("production-dark state enabled=%d suite=%q container=%q", enabled, suite, container)
	}
	if _, err := fixture.store.db.Exec(`UPDATE e2ee_feature_state SET enabled = 1 WHERE singleton = 1`); err == nil {
		t.Fatal("schema unexpectedly allowed E2EE activation")
	}

	object, err := fixture.store.StageE2EEProtectedObject(e2eeObjectParams(
		fixture, "source_clip_00000000000001", fixture.now+10,
	))
	if err != nil {
		t.Fatal(err)
	}
	if object.Status != "staged" || object.Revision != 1 || object.FinalizedAt != 0 {
		t.Fatalf("staged object=%+v", object)
	}
	ready, err := fixture.store.FinalizeE2EEProtectedObject(object.ID, object.Revision, fixture.now+11)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != "ready" || ready.Revision != 2 || ready.FinalizedAt == 0 {
		t.Fatalf("ready object=%+v", ready)
	}
	if _, err := fixture.store.FinalizeE2EEProtectedObject(object.ID, object.Revision, fixture.now+12); !errors.Is(err, ErrE2EEDuplicateFinalize) {
		t.Fatalf("duplicate finalize error=%v", err)
	}
	if _, err := fixture.store.db.Exec(`UPDATE e2ee_protected_objects
SET encrypted_manifest = X'00' WHERE id = ?`, object.ID); err == nil {
		t.Fatal("immutable encrypted payload update unexpectedly succeeded")
	}
	revoked, err := fixture.store.RevokeE2EEProtectedObject(ready.ID, ready.Revision, fixture.now+13)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Status != "revoked" || revoked.Revision != 3 || revoked.RevokedAt == 0 {
		t.Fatalf("revoked object=%+v", revoked)
	}
	if _, err := fixture.store.RevokeE2EEProtectedObject(ready.ID, ready.Revision, fixture.now+14); !errors.Is(err, ErrE2EERevoked) {
		t.Fatalf("duplicate revoke error=%v", err)
	}
	if _, err := fixture.store.db.Exec(`DELETE FROM e2ee_audit_events`); err == nil {
		t.Fatal("immutable E2EE audit delete unexpectedly succeeded")
	}
	var legacyProtectedLinks int
	if err := fixture.store.db.QueryRow(`SELECT COUNT(*) FROM media_items m
JOIN e2ee_protected_objects p ON p.source_object_id = m.id`).Scan(&legacyProtectedLinks); err != nil {
		t.Fatal(err)
	}
	if legacyProtectedLinks != 0 {
		t.Fatalf("protected object leaked into plaintext media rows: %d", legacyProtectedLinks)
	}
}

func TestE2EECommitSingleWinnerStaleAndForkState(t *testing.T) {
	fixture := newE2EEStoreFixture(t)
	commit := func(eventID, commitDigest string, stamp int64) ApplyVerifiedE2EECommitParams {
		payload := []byte("public-commit:" + eventID)
		return ApplyVerifiedE2EECommitParams{
			EventID: eventID, GroupID: fixture.group.ID,
			AuthorDeviceID:       "device_owner_verified_0001",
			PreviousCommitDigest: fixture.group.CommitDigest,
			CommitDigest:         commitDigest, TargetSnapshotDigest: strings.Repeat("f", 64),
			EventDigest: e2eeDigest(payload), PreviousEpoch: fixture.group.CurrentEpoch,
			Epoch: fixture.group.CurrentEpoch + 1, CreatedAt: stamp, PublicPayload: payload,
		}
	}
	params := []ApplyVerifiedE2EECommitParams{
		commit("commit_event_concurrent_0001", strings.Repeat("1", 64), fixture.now+20),
		commit("commit_event_concurrent_0002", strings.Repeat("2", 64), fixture.now+21),
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, candidate := range params {
		candidate := candidate
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := fixture.store.ApplyVerifiedE2EECommit(candidate)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	var success, stale int
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrE2EEStaleEpoch):
			stale++
		default:
			t.Fatalf("concurrent commit error=%v", err)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatalf("commit outcomes success=%d stale=%d", success, stale)
	}
	current, err := fixture.store.GetE2EEGroup(fixture.group.ID)
	if err != nil || current.CurrentEpoch != fixture.group.CurrentEpoch+1 || current.ForkState != "clean" {
		t.Fatalf("current group=%+v err=%v", current, err)
	}

	forkPayload := []byte("public-commit:fork-current-predecessor")
	_, err = fixture.store.ApplyVerifiedE2EECommit(ApplyVerifiedE2EECommitParams{
		EventID: "commit_event_forked_000001", GroupID: current.ID,
		AuthorDeviceID:       "device_owner_verified_0001",
		PreviousCommitDigest: strings.Repeat("9", 64),
		CommitDigest:         strings.Repeat("8", 64), TargetSnapshotDigest: strings.Repeat("7", 64),
		EventDigest: e2eeDigest(forkPayload), PreviousEpoch: current.CurrentEpoch,
		Epoch: current.CurrentEpoch + 1, CreatedAt: fixture.now + 22, PublicPayload: forkPayload,
	})
	if !errors.Is(err, ErrE2EEForked) {
		t.Fatalf("fork error=%v", err)
	}
	forked, err := fixture.store.GetE2EEGroup(current.ID)
	if err != nil || forked.ForkState != "forked" {
		t.Fatalf("forked group=%+v err=%v", forked, err)
	}
}

func TestE2EEReplaySequenceGenerationAndNonceStateSurviveRestart(t *testing.T) {
	fixture := newE2EEStoreFixture(t)
	accept := func(eventID, nonce string, generation, sequence, stamp int64) error {
		return fixture.store.AcceptE2EEReplayState(AcceptE2EEReplayStateParams{
			GroupID: fixture.group.ID, EventID: eventID,
			AuthorDeviceID: "device_owner_verified_0001",
			SourceObjectID: "source_live_ptt_000000001", NonceDigest: nonce,
			Epoch: fixture.group.CurrentEpoch, Generation: generation,
			Sequence: sequence, AcceptedAt: stamp,
		})
	}
	if err := accept("event_replay_state_00000001", strings.Repeat("1", 64), 3, 1, fixture.now+30); err != nil {
		t.Fatal(err)
	}
	if err := accept("event_replay_state_00000002", strings.Repeat("2", 64), 3, 1, fixture.now+31); !errors.Is(err, ErrE2EEReplay) {
		t.Fatalf("sequence regression error=%v", err)
	}
	if err := accept("event_replay_state_00000003", strings.Repeat("1", 64), 3, 2, fixture.now+32); !errors.Is(err, ErrE2EEReplay) {
		t.Fatalf("nonce reuse error=%v", err)
	}
	if err := accept("event_replay_state_00000004", strings.Repeat("4", 64), 3, 2, fixture.now+33); err != nil {
		t.Fatal(err)
	}
	if err := accept("event_replay_state_00000005", strings.Repeat("5", 64), 4, 2, fixture.now+34); !errors.Is(err, ErrE2EEReplay) {
		t.Fatalf("generation reset error=%v", err)
	}
	if err := accept("event_replay_state_00000006", strings.Repeat("6", 64), 4, 1, fixture.now+35); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithOptions(fixture.path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	fixture.store = reopened
	t.Cleanup(func() { _ = reopened.Close() })
	if err := accept("event_replay_state_00000007", strings.Repeat("7", 64), 4, 1, fixture.now+36); !errors.Is(err, ErrE2EEReplay) {
		t.Fatalf("restart sequence reset error=%v", err)
	}
}

func TestE2EEGrantTransferReportAndRevokeRace(t *testing.T) {
	fixture := newE2EEStoreFixture(t)
	grantPayload := []byte{0x88, 0x21, 0x45, 0x7f}
	grant, err := fixture.store.CreateE2EEHistoryGrant(CreateE2EEHistoryGrantParams{
		GroupID: fixture.group.ID, IssuedByDeviceID: "device_owner_verified_0001",
		RecipientDeviceID: "device_recipient_verified_1",
		SourceObjectID:    "source_track_0000000000001", FirstEpoch: 3,
		LastEpoch: fixture.group.CurrentEpoch, TargetSnapshotDigest: fixture.group.TargetSnapshotDigest,
		EncryptedGrant: grantPayload, GrantDigest: e2eeDigest(grantPayload),
		IssuedAt: fixture.now + 40, ExpiresAt: fixture.now + 4000,
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(offset int64) {
			defer wg.Done()
			_, err := fixture.store.RevokeE2EEHistoryGrant(grant.ID, grant.Revision, fixture.now+41+offset)
			results <- err
		}(int64(i))
	}
	wg.Wait()
	close(results)
	var revoked, lost int
	for err := range results {
		if err == nil {
			revoked++
		} else if errors.Is(err, ErrE2EERevoked) || errors.Is(err, ErrE2EEConflict) {
			lost++
		} else {
			t.Fatalf("grant race error=%v", err)
		}
	}
	if revoked != 1 || lost != 1 {
		t.Fatalf("grant race revoked=%d lost=%d", revoked, lost)
	}

	packagePayload := []byte{0xa1, 0x08, 0x77, 0xc4}
	if _, err := fixture.store.CreateE2EETransferPackage(CreateE2EETransferPackageParams{
		GroupID: fixture.group.ID, PackageKind: "device_transfer",
		IssuerDeviceID:    "device_owner_verified_0001",
		RecipientDeviceID: "device_recipient_verified_1", Epoch: fixture.group.CurrentEpoch,
		EncryptedPackage: packagePayload, PackageDigest: e2eeDigest(packagePayload),
		CreatedAt: fixture.now + 50, ExpiresAt: fixture.now + 5000,
	}); err != nil {
		t.Fatal(err)
	}
	object, err := fixture.store.StageE2EEProtectedObject(e2eeObjectParams(
		fixture, "source_clip_report_00000001", fixture.now+51,
	))
	if err != nil {
		t.Fatal(err)
	}
	object, err = fixture.store.FinalizeE2EEProtectedObject(object.ID, object.Revision, fixture.now+52)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.CreateE2EEReportEvidenceMetadata(CreateE2EEReportEvidenceParams{
		ReportID: "report_e2ee_0000000000001", ProtectedObjectID: object.ID,
		ReporterActorID: fixture.recipient.ActorID, ConsentVersion: "report-boundary-v1",
		ConsentDigest: strings.Repeat("6", 64), AuthenticatedEvidenceDigest: strings.Repeat("7", 64),
		EncryptedEvidenceRef: "evidence/v1/" + strings.Repeat("8", 64),
		RetentionExpiresAt:   fixture.now + 5000, CreatedAt: fixture.now + 53,
	}); !errors.Is(err, ErrModerationInvalid) {
		t.Fatalf("unbound legacy report evidence error=%v", err)
	}
}

func TestE2EEMigrationRollbackAndBackupContainOnlyAllowedState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "generation-skip.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO media(
id, tg_file_id, duration_ms, path_wav, loudnorm_json, created_at, expires_at, status, orbit_id
) VALUES('legacy_plain_media', 'telegram-file', 1000, '/legacy/plain.wav', '{}', 10, 10000, 'ready', 0)`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	failed, err := openWithOptionsAndCheckpoint(path, Options{}, func(name string) error {
		if name == "e2ee_ddl_before_commit" {
			return errors.New("injected E2EE migration crash")
		}
		return nil
	})
	if err == nil || failed != nil {
		t.Fatalf("migration fault store=%v err=%v", failed, err)
	}
	inspect, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var e2eeTables int
	if err := inspect.QueryRow(`SELECT COUNT(*) FROM sqlite_master
WHERE type = 'table' AND name LIKE 'e2ee_%'`).Scan(&e2eeTables); err != nil {
		t.Fatal(err)
	}
	if e2eeTables != 0 {
		t.Fatalf("faulted additive migration left %d E2EE tables", e2eeTables)
	}
	if err := inspect.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var legacyPath string
	if err := st.db.QueryRow(`SELECT path_wav FROM media WHERE id = 'legacy_plain_media'`).Scan(&legacyPath); err != nil || legacyPath != "/legacy/plain.wav" {
		t.Fatalf("legacy media path=%q err=%v", legacyPath, err)
	}
	if err := foreignKeyCheck(st.db); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	// A rollback coordinator only knows legacy tables. Its normal writes and
	// reads remain valid while the additive E2EE tables are ignored.
	rollback, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rollback.Exec(`INSERT INTO media(
id, tg_file_id, duration_ms, path_wav, loudnorm_json, created_at, expires_at, status, orbit_id
) VALUES('rollback_plain_media', 'telegram-file-2', 1000, '/rollback/plain.wav', '{}', 11, 10001, 'ready', 0)`); err != nil {
		t.Fatal(err)
	}
	if err := rollback.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	var count int
	if err := reopened.db.QueryRow(`SELECT COUNT(*) FROM media
WHERE id IN ('legacy_plain_media', 'rollback_plain_media')`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("legacy rows after roll-forward=%d err=%v", count, err)
	}

	forbiddenColumns := map[string]bool{
		"private_key": true, "key_package_private_key": true, "epoch_secret": true,
		"sender_key": true, "content_key": true, "recovery_secret": true,
		"history_grant_secret": true, "plaintext": true, "decrypted_evidence": true,
	}
	rows, err := reopened.db.Query(`SELECT name FROM sqlite_master
WHERE type = 'table' AND name LIKE 'e2ee_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		columns, err := reopened.db.Query(`PRAGMA table_info(` + table + `)`)
		if err != nil {
			t.Fatal(err)
		}
		for columns.Next() {
			var cid, notNull, pk int
			var name, columnType string
			var defaultValue any
			if err := columns.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
				t.Fatal(err)
			}
			if forbiddenColumns[name] {
				t.Fatalf("forbidden E2EE storage column %s.%s", table, name)
			}
		}
		if err := columns.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := reopened.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	sentinels := []string{
		"PRIVATE-DEVICE-KEY-SENTINEL", "EPOCH-SECRET-SENTINEL",
		"CONTENT-KEY-SENTINEL", "PROTECTED-PLAINTEXT-SENTINEL",
		"UNCONSENTED-DECRYPTED-EVIDENCE-SENTINEL",
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		raw, err := os.ReadFile(path + suffix)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, sentinel := range sentinels {
			if strings.Contains(string(raw), sentinel) {
				t.Fatalf("backup file %s contains %s", suffix, sentinel)
			}
		}
	}
}
