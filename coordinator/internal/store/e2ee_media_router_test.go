package store

import (
	"errors"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/e2eecontract"
	"relux.works/duet/coordinator/internal/protocol"
)

func stageRoutedCiphertextParams(f e2eeRoutingFixture, sourceID string, chunks ...[]byte) StageE2EEProtectedObjectParams {
	var all []byte
	for _, chunk := range chunks {
		all = append(all, chunk...)
	}
	manifest := []byte("opaque-encrypted-manifest:" + sourceID)
	envelopes := []byte("opaque-key-envelopes:" + sourceID)
	return StageE2EEProtectedObjectParams{
		GroupID: f.group.ID, SourceObjectID: sourceID, ObjectKind: "clip",
		AuthorDeviceID: f.ownerDevice, Epoch: f.group.CurrentEpoch, Generation: 1,
		TargetSnapshotDigest: f.group.TargetSnapshotDigest,
		ManifestDigest:       e2eeDigest(manifest), EncryptedManifest: manifest,
		OpaqueKeyEnvelopes: envelopes, CiphertextRef: "ciphertext/v1/" + e2eeDigest(all),
		CiphertextDigest: e2eeDigest(all), CiphertextSize: int64(len(all)),
		ChunkCount: int64(len(chunks)), DeclaredDurationMS: 1200, CreatedAt: f.now + 100,
	}
}

func stageRoutedCiphertextObject(t *testing.T, f e2eeRoutingFixture, sourceID string, chunks ...[]byte) E2EEProtectedObject {
	t.Helper()
	object, err := f.store.StageE2EEProtectedObject(stageRoutedCiphertextParams(f, sourceID, chunks...))
	if err != nil {
		t.Fatal(err)
	}
	return object
}

func putRoutedCiphertextChunks(t *testing.T, f e2eeRoutingFixture, object E2EEProtectedObject, chunks ...[]byte) {
	t.Helper()
	var offset int64
	for index, chunk := range chunks {
		if _, err := f.store.PutE2EEProtectedChunk(PutE2EEProtectedChunkParams{
			ProtectedObjectID: object.ID, AuthorDeviceID: f.ownerDevice,
			CiphertextDigest: e2eeDigest(chunk), ChunkIndex: int64(index),
			ByteOffset: offset, Ciphertext: chunk, CreatedAt: f.now + 101 + int64(index),
		}); err != nil {
			t.Fatal(err)
		}
		offset += int64(len(chunk))
	}
}

func TestE2EEOpaqueObjectChunkRangeRestartAndDelete(t *testing.T) {
	f := newE2EERoutingFixture(t)
	chunks := [][]byte{[]byte("opaque-ciphertext-chunk-alpha"), []byte("opaque-ciphertext-chunk-bravo")}
	invalid := stageRoutedCiphertextParams(f, "opaque_bad_manifest_001", chunks...)
	invalid.ManifestDigest = strings.Repeat("f", 64)
	if _, err := f.store.StageE2EEProtectedObject(invalid); !errors.Is(err, ErrE2EEInvalid) {
		t.Fatalf("tampered manifest digest error=%v", err)
	}
	object := stageRoutedCiphertextObject(t, f, "opaque_route_source_0001", chunks...)
	if _, err := f.store.FinalizeE2EEProtectedObject(object.ID, object.Revision,
		f.now+101); !errors.Is(err, ErrE2EEObjectIncomplete) {
		t.Fatalf("incomplete finalize error=%v", err)
	}
	first, err := f.store.PutE2EEProtectedChunk(PutE2EEProtectedChunkParams{
		ProtectedObjectID: object.ID, AuthorDeviceID: f.ownerDevice,
		CiphertextDigest: e2eeDigest(chunks[0]), ChunkIndex: 0, ByteOffset: 0,
		Ciphertext: chunks[0], CreatedAt: f.now + 102,
	})
	if err != nil {
		t.Fatal(err)
	}
	if replay, err := f.store.PutE2EEProtectedChunk(PutE2EEProtectedChunkParams{
		ProtectedObjectID: object.ID, AuthorDeviceID: f.ownerDevice,
		CiphertextDigest: first.CiphertextDigest, ChunkIndex: 0, ByteOffset: 0,
		Ciphertext: chunks[0], CreatedAt: f.now + 103,
	}); err != nil || replay.CiphertextDigest != first.CiphertextDigest {
		t.Fatalf("idempotent chunk replay=%+v err=%v", replay, err)
	}
	if _, err := f.store.PutE2EEProtectedChunk(PutE2EEProtectedChunkParams{
		ProtectedObjectID: object.ID, AuthorDeviceID: f.ownerDevice,
		CiphertextDigest: e2eeDigest(chunks[1]), ChunkIndex: 1, ByteOffset: 999,
		Ciphertext: chunks[1], CreatedAt: f.now + 104,
	}); !errors.Is(err, ErrE2EEChunkOrder) {
		t.Fatalf("misaligned chunk error=%v", err)
	}
	if _, err := f.store.PutE2EEProtectedChunk(PutE2EEProtectedChunkParams{
		ProtectedObjectID: object.ID, AuthorDeviceID: f.ownerDevice,
		CiphertextDigest: e2eeDigest(chunks[1]), ChunkIndex: 1,
		ByteOffset: int64(len(chunks[0])), Ciphertext: chunks[1], CreatedAt: f.now + 105,
	}); err != nil {
		t.Fatal(err)
	}
	ready, err := f.store.FinalizeE2EEProtectedObject(object.ID, object.Revision, f.now+106)
	if err != nil || ready.Status != "ready" {
		t.Fatalf("ready=%+v err=%v", ready, err)
	}
	manifest, err := f.store.GetAuthorizedE2EEProtectedManifest(ready.ID, f.peerDevice, f.now+106)
	if err != nil || manifest.Object.ManifestDigest != ready.ManifestDigest || len(manifest.OpaqueKeyEnvelopes) == 0 {
		t.Fatalf("manifest=%+v err=%v", manifest, err)
	}
	rangeParams := FetchE2EEProtectedRangeParams{
		ProtectedObjectID: ready.ID, RecipientDeviceID: f.peerDevice,
		TargetSnapshotDigest: ready.TargetSnapshotDigest, ManifestDigest: ready.ManifestDigest,
		IfRangeManifestDigest: ready.ManifestDigest, Epoch: ready.Epoch,
		Generation: ready.Generation, Start: 0, EndExclusive: int64(len(chunks[0])),
		RequestedAt: f.now + 106,
	}
	routed, err := f.store.FetchAuthorizedE2EEProtectedRange(rangeParams)
	if err != nil || len(routed.Chunks) != 1 || string(routed.Chunks[0].Ciphertext) != string(chunks[0]) {
		t.Fatalf("range=%+v err=%v", routed, err)
	}
	rangeParams.Start = 1
	if _, err := f.store.FetchAuthorizedE2EEProtectedRange(rangeParams); !errors.Is(err, ErrE2EEChunkOrder) {
		t.Fatalf("partial authenticated chunk error=%v", err)
	}
	rangeParams.Start = 0
	rangeParams.IfRangeManifestDigest = strings.Repeat("f", 64)
	if _, err := f.store.FetchAuthorizedE2EEProtectedRange(rangeParams); !errors.Is(err, ErrE2EEIfRangeMismatch) {
		t.Fatalf("If-Range mismatch error=%v", err)
	}
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithOptions(f.path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	f.store = reopened
	t.Cleanup(func() { _ = reopened.Close() })
	rangeParams.IfRangeManifestDigest = ready.ManifestDigest
	if routed, err = reopened.FetchAuthorizedE2EEProtectedRange(rangeParams); err != nil || len(routed.Chunks) != 1 {
		t.Fatalf("restart range=%+v err=%v", routed, err)
	}
	var chargedBytes, actualBytes, rangeRequests int64
	if err := reopened.db.QueryRow(`SELECT charged_bytes, actual_bytes, range_requests
FROM e2ee_protected_egress_usage WHERE recipient_device_id = ?`, f.peerDevice).Scan(
		&chargedBytes, &actualBytes, &rangeRequests); err != nil {
		t.Fatal(err)
	}
	manifestBytes := int64(len(manifest.EncryptedManifest) + len(manifest.OpaqueKeyEnvelopes))
	if chargedBytes != 3*StreamRangeRequestChargeBytes ||
		actualBytes != manifestBytes+2*int64(len(chunks[0])) || rangeRequests != 3 {
		t.Fatalf("egress charged=%d actual=%d requests=%d", chargedBytes, actualBytes, rangeRequests)
	}
	if _, err := reopened.DeleteE2EEProtectedObject(ready.ID, f.peerDevice,
		ready.Revision, f.now+107); !errors.Is(err, ErrE2EEInvalid) {
		t.Fatalf("non-author delete error=%v", err)
	}
	deleted, err := reopened.DeleteE2EEProtectedObject(ready.ID, f.ownerDevice, ready.Revision, f.now+107)
	if err != nil || deleted.Status != "deleted" {
		t.Fatalf("deleted=%+v err=%v", deleted, err)
	}
	if _, err := reopened.GetAuthorizedE2EEProtectedManifest(ready.ID, f.peerDevice, f.now+108); !errors.Is(err, ErrE2EERevoked) {
		t.Fatalf("post-delete manifest error=%v", err)
	}
	var chunksLeft, legacyRows int
	if err := reopened.db.QueryRow(`SELECT COUNT(*) FROM e2ee_protected_object_chunks
WHERE protected_object_id = ?`, ready.ID).Scan(&chunksLeft); err != nil {
		t.Fatal(err)
	}
	if err := reopened.db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE id = ?`,
		ready.SourceObjectID).Scan(&legacyRows); err != nil {
		t.Fatal(err)
	}
	if chunksLeft != 0 || legacyRows != 0 {
		t.Fatalf("delete chunks=%d legacy plaintext rows=%d", chunksLeft, legacyRows)
	}
}

func TestE2EEOpaqueObjectRecipientRevocationForkAndQuota(t *testing.T) {
	f := newE2EERoutingFixture(t)
	chunk := []byte("opaque-targeted-ciphertext")
	object := stageRoutedCiphertextObject(t, f, "opaque_route_source_0002", chunk)
	putRoutedCiphertextChunks(t, f, object, chunk)
	ready, err := f.store.FinalizeE2EEProtectedObject(object.ID, object.Revision, f.now+120)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.db.Exec(`UPDATE e2ee_groups SET fork_state = 'forked'
WHERE id = ?`, f.group.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.GetAuthorizedE2EEProtectedManifest(ready.ID, f.peerDevice, f.now+120); !errors.Is(err, ErrE2EEForked) {
		t.Fatalf("persisted fork fetch error=%v", err)
	}
	if _, err := f.store.db.Exec(`UPDATE e2ee_groups SET fork_state = 'clean'
WHERE id = ?`, f.group.ID); err != nil {
		t.Fatal(err)
	}
	peer, err := f.store.GetE2EEPublicDevice(f.peerDevice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.RevokeE2EEPublicDevice(f.peerDevice, peer.Revision, f.now+121); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.GetAuthorizedE2EEProtectedManifest(ready.ID, f.peerDevice, f.now+122); !errors.Is(err, ErrE2EERotationRequired) {
		t.Fatalf("revoked recipient fetch error=%v", err)
	}

	f = newE2EERoutingFixture(t)
	for index := 0; index < e2eeMaxConcurrentObjects; index++ {
		uniqueChunk := append(append([]byte(nil), chunk...), byte(index))
		_ = stageRoutedCiphertextObject(t, f, "quota_source_0000000"+string(rune('a'+index)), uniqueChunk)
	}
	manifest := []byte("quota-overflow-manifest")
	overflow := []byte("quota-overflow")
	if _, err := f.store.StageE2EEProtectedObject(StageE2EEProtectedObjectParams{
		GroupID: f.group.ID, SourceObjectID: "quota_source_overflow_01", ObjectKind: "clip",
		AuthorDeviceID: f.ownerDevice, Epoch: f.group.CurrentEpoch, Generation: 1,
		TargetSnapshotDigest: f.group.TargetSnapshotDigest,
		ManifestDigest:       e2eeDigest(manifest), EncryptedManifest: manifest,
		OpaqueKeyEnvelopes: []byte("opaque-envelope"),
		CiphertextRef:      "ciphertext/v1/" + e2eeDigest(overflow),
		CiphertextDigest:   e2eeDigest(overflow), CiphertextSize: int64(len(overflow)),
		ChunkCount: 1, CreatedAt: f.now + 200,
	}); !errors.Is(err, ErrE2EEQuotaExceeded) {
		t.Fatalf("concurrent quota error=%v", err)
	}
}

func liveSessionParams(f e2eeRoutingFixture, sessionID string, generation, stamp int64) StartE2EEOpaqueLiveParams {
	header := []byte("opaque-live-header:" + sessionID)
	return StartE2EEOpaqueLiveParams{SessionID: sessionID, GroupID: f.group.ID,
		AuthorDeviceID: f.ownerDevice, TargetSnapshotDigest: f.group.TargetSnapshotDigest,
		HeaderDigest: e2eeDigest(header), Epoch: f.group.CurrentEpoch, Generation: generation,
		StartedAt: stamp, ExpiresAt: stamp + protocol.LivePTTMaxDurationMS,
		OpaqueHeader: header}
}

func opaqueLiveFrame(t *testing.T, f e2eeRoutingFixture, sessionID string, generation int64, sequence uint32, flags byte) []byte {
	t.Helper()
	parsed, err := protocol.ParseLivePTTSessionID(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := e2eecontract.EncodeOpaqueLiveFrame(e2eecontract.OpaqueLiveFrame{
		Flags: flags, SessionID: parsed, Epoch: uint64(f.group.CurrentEpoch),
		Generation: uint64(generation), Sequence: sequence,
		CaptureMonotonicUS:   uint64(sequence) * e2eecontract.OpaqueLiveFrameMS * 1000,
		TargetSnapshotDigest: f.group.TargetSnapshotDigest,
		Ciphertext:           []byte{0xde, 0xad, 0xbe, 0xef, byte(sequence)},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestE2EEOpaqueLiveRelayBackpressureRestartAndRotation(t *testing.T) {
	f := newE2EERoutingFixture(t)
	sessionID := "11111111111111111111111111111111"
	session, recipients, err := f.store.StartE2EEOpaqueLiveSession(
		liveSessionParams(f, sessionID, 1, f.now+300))
	if err != nil || session.State != "active" || len(recipients) != 1 || recipients[0] != f.peerDevice {
		t.Fatalf("start=%+v recipients=%v err=%v", session, recipients, err)
	}
	if err := f.store.RecordE2EEOpaqueLiveReceipt(sessionID, f.peerDevice,
		1, "accepted", f.now+300); err != nil {
		t.Fatal(err)
	}
	if err := f.store.RecordE2EEOpaqueLiveReceipt(sessionID, f.peerDevice,
		1, "accepted", f.now+300); err != nil {
		t.Fatalf("idempotent receipt error=%v", err)
	}
	if err := f.store.RecordE2EEOpaqueLiveReceipt(sessionID, f.peerDevice,
		1, "failed", f.now+300); !errors.Is(err, ErrE2EEConflict) {
		t.Fatalf("conflicting receipt error=%v", err)
	}
	raw := opaqueLiveFrame(t, f, sessionID, 1, 1, e2eecontract.OpaqueLiveFlagStart)
	relay, err := f.store.RelayE2EEOpaqueLiveFrame(sessionID, f.ownerDevice, raw, f.now+301)
	if err != nil || len(relay.RecipientDeviceIDs) != 1 ||
		string(relay.OpaqueFrame) != string(raw) {
		t.Fatalf("relay=%+v err=%v", relay, err)
	}
	duplicate, err := f.store.RelayE2EEOpaqueLiveFrame(sessionID, f.ownerDevice, raw, f.now+302)
	if err != nil || len(duplicate.RecipientDeviceIDs) != 0 || len(duplicate.OpaqueFrame) != 0 {
		t.Fatalf("duplicate=%+v err=%v", duplicate, err)
	}
	if err := f.store.MarkE2EEOpaqueLiveRecipientUnavailable(sessionID,
		f.peerDevice, "backpressure", f.now+303); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.RelayE2EEOpaqueLiveFrame(sessionID, f.ownerDevice,
		opaqueLiveFrame(t, f, sessionID, 1, 2, 0), f.now+304); !errors.Is(err, ErrE2EERevoked) {
		t.Fatalf("post-backpressure relay error=%v", err)
	}

	sessionID = "22222222222222222222222222222222"
	if _, _, err := f.store.StartE2EEOpaqueLiveSession(
		liveSessionParams(f, sessionID, 2, f.now+310)); err != nil {
		t.Fatal(err)
	}
	if err := f.store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithOptions(f.path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	f.store = reopened
	t.Cleanup(func() { _ = reopened.Close() })
	if _, err := reopened.RelayE2EEOpaqueLiveFrame(sessionID, f.ownerDevice,
		opaqueLiveFrame(t, f, sessionID, 2, 1, e2eecontract.OpaqueLiveFlagStart), f.now+311); !errors.Is(err, ErrE2EERevoked) {
		t.Fatalf("restart relay error=%v", err)
	}
	if _, _, err := reopened.StartE2EEOpaqueLiveSession(
		liveSessionParams(f, "33333333333333333333333333333333", 2, f.now+312)); !errors.Is(err, ErrE2EEReplay) {
		t.Fatalf("generation reset error=%v", err)
	}
	activeID := "44444444444444444444444444444444"
	if _, _, err := reopened.StartE2EEOpaqueLiveSession(
		liveSessionParams(f, activeID, 3, f.now+313)); err != nil {
		t.Fatal(err)
	}
	if err := reopened.LeaveAirMember(f.peerMemberID, 2, f.now+314); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.RelayE2EEOpaqueLiveFrame(activeID, f.ownerDevice,
		opaqueLiveFrame(t, f, activeID, 3, 1, e2eecontract.OpaqueLiveFlagStart), f.now+315); !errors.Is(err, ErrE2EERotationRequired) {
		t.Fatalf("membership-change relay error=%v", err)
	}
}

func TestE2EEOpaqueLiveFrameBindingAndRateAreFailClosed(t *testing.T) {
	f := newE2EERoutingFixture(t)
	sessionID := "55555555555555555555555555555555"
	if _, _, err := f.store.StartE2EEOpaqueLiveSession(
		liveSessionParams(f, sessionID, 1, f.now+400)); err != nil {
		t.Fatal(err)
	}
	foreignTarget := opaqueLiveFrame(t, f, sessionID, 1, 1, e2eecontract.OpaqueLiveFlagStart)
	foreignTarget[48] ^= 0xff
	if _, err := f.store.RelayE2EEOpaqueLiveFrame(sessionID, f.ownerDevice,
		foreignTarget, f.now+401); !errors.Is(err, ErrE2EEStaleEpoch) {
		t.Fatalf("foreign target frame error=%v", err)
	}
	legacy := opaqueLiveFrame(t, f, sessionID, 1, 1, e2eecontract.OpaqueLiveFlagStart)
	copy(legacy[:2], []byte("BP"))
	if _, err := f.store.RelayE2EEOpaqueLiveFrame(sessionID, f.ownerDevice,
		legacy, f.now+401); !errors.Is(err, ErrE2EEInvalid) {
		t.Fatalf("legacy frame downgrade error=%v", err)
	}
	for sequence := uint32(1); sequence <= uint32(e2eeOpaqueLiveBurstFrames); sequence++ {
		flags := byte(0)
		if sequence == 1 {
			flags = e2eecontract.OpaqueLiveFlagStart
		}
		if _, err := f.store.RelayE2EEOpaqueLiveFrame(sessionID, f.ownerDevice,
			opaqueLiveFrame(t, f, sessionID, 1, sequence, flags), f.now+401); err != nil {
			t.Fatalf("burst frame %d error=%v", sequence, err)
		}
	}
	if _, err := f.store.RelayE2EEOpaqueLiveFrame(sessionID, f.ownerDevice,
		opaqueLiveFrame(t, f, sessionID, 1, 9, 0), f.now+401); !errors.Is(err, ErrE2EELiveRateExceeded) {
		t.Fatalf("rate limit error=%v", err)
	}
}
