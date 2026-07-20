package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type windowsProtectedPlaybackVectors struct {
	Contract          string   `json:"contract"`
	Status            string   `json:"status"`
	FixtureSuite      string   `json:"fixtureSuite"`
	FixtureContainer  string   `json:"fixtureContainer"`
	PlatformProducers []string `json:"platformProducers"`
	CiphertextSHA256  string   `json:"ciphertextSHA256"`
	Chunks            []struct {
		Index                        int    `json:"index"`
		Offset                       int64  `json:"offset"`
		Size                         int    `json:"size"`
		CiphertextSHA256             string `json:"ciphertextSHA256"`
		AuthenticatedPlaintextSHA256 string `json:"authenticatedPlaintextSHA256"`
	} `json:"chunks"`
	FailClosed []map[string]string `json:"failClosed"`
}

func loadWindowsProtectedPlaybackVectors(t testing.TB, name string) windowsProtectedPlaybackVectors {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "protocol", name))
	if err != nil {
		t.Fatal(err)
	}
	var vectors windowsProtectedPlaybackVectors
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	return vectors
}

type windowsProtectedPlaybackTransport struct {
	mu            sync.Mutex
	route         WindowsProtectedMediaPlaybackRoute
	bodies        map[string][]byte
	manifestCalls int
	rangeCalls    int
	corrupt       bool
	failure       error
}

func newWindowsProtectedPlaybackTransport(
	route WindowsProtectedMediaPlaybackRoute, ciphertext [][]byte,
) *windowsProtectedPlaybackTransport {
	bodies := make(map[string][]byte, len(ciphertext))
	for index, body := range ciphertext {
		chunk := route.StreamManifest.Chunks[index]
		bodies[streamRangeKey(chunk.Start, chunk.End)] = append([]byte(nil), body...)
	}
	return &windowsProtectedPlaybackTransport{route: route, bodies: bodies}
}

func (t *windowsProtectedPlaybackTransport) FetchManifest(
	_ context.Context, objectID, recipientDeviceID string, requestedAtMS int64,
) (WindowsProtectedMediaPlaybackRoute, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.manifestCalls++
	if t.failure != nil {
		return WindowsProtectedMediaPlaybackRoute{}, t.failure
	}
	if objectID != t.route.ObjectID || recipientDeviceID != t.route.RecipientDeviceID || requestedAtMS <= 0 {
		return WindowsProtectedMediaPlaybackRoute{}, ErrWindowsProtectedMediaPlaybackInvalidRequest
	}
	return t.route, nil
}

func (t *windowsProtectedPlaybackTransport) FetchRange(
	_ context.Context, request WindowsProtectedMediaPlaybackRangeRequest,
) ([]byte, string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rangeCalls++
	if t.failure != nil {
		return nil, "", t.failure
	}
	if request.ObjectID != t.route.ObjectID || request.RecipientDeviceID != t.route.RecipientDeviceID ||
		request.GroupID != t.route.GroupID || request.Epoch != t.route.Epoch ||
		request.Generation != t.route.Generation || request.TargetSnapshotDigest != t.route.TargetSnapshotDigest ||
		request.ManifestDigest != t.route.ManifestDigest || request.ETag != t.route.StreamManifest.ETag {
		return nil, "", ErrWindowsProtectedMediaPlaybackInvalidRequest
	}
	body := append([]byte(nil), t.bodies[streamRangeKey(request.Start, request.End)]...)
	if t.corrupt {
		body = bytes.Repeat([]byte{0xff}, len(body))
	}
	return body, request.ETag, nil
}

func (t *windowsProtectedPlaybackTransport) setRoute(route WindowsProtectedMediaPlaybackRoute) {
	t.mu.Lock()
	t.route = route
	t.mu.Unlock()
}

func (t *windowsProtectedPlaybackTransport) setCorrupt(value bool) {
	t.mu.Lock()
	t.corrupt = value
	t.mu.Unlock()
}

func (t *windowsProtectedPlaybackTransport) counts() (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.manifestCalls, t.rangeCalls
}

type windowsProtectedPlaybackFixtureOpener struct {
	mu              sync.Mutex
	openCount       int
	decryptCount    int
	sawHistoryGrant bool
	rejectOpen      bool
	rejectChunk     bool
}

func (o *windowsProtectedPlaybackFixtureOpener) ProductionApproved() bool { return false }

func (o *windowsProtectedPlaybackFixtureOpener) Open(
	_ context.Context, route WindowsProtectedMediaPlaybackRoute,
	_ *WindowsE2EEDeviceIdentityLease, _ *WindowsE2EEGroupStateLease,
	historyGrant *WindowsE2EESecretLease,
) (*WindowsProtectedMediaOpenLease, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.openCount++
	o.sawHistoryGrant = historyGrant != nil
	if o.rejectOpen || !bytes.Equal(route.Signature, []byte("fixture-signature")) {
		return nil, ErrWindowsProtectedMediaPlaybackInvalidAuthentication
	}
	return NewWindowsProtectedMediaOpenLease([]byte("fixture-open-key-canary")), nil
}

func (o *windowsProtectedPlaybackFixtureOpener) AuthenticateAndDecrypt(
	_ context.Context, ciphertext []byte, _ WindowsStreamChunk,
	_ WindowsProtectedMediaPlaybackRoute, lease *WindowsProtectedMediaOpenLease,
) ([]byte, error) {
	o.mu.Lock()
	o.decryptCount++
	reject := o.rejectChunk
	o.mu.Unlock()
	if reject || lease.WithOpaqueState(func(value []byte) error {
		if !bytes.Equal(value, []byte("fixture-open-key-canary")) {
			return ErrWindowsProtectedMediaPlaybackInvalidAuthentication
		}
		return nil
	}) != nil {
		return nil, ErrWindowsProtectedMediaPlaybackInvalidAuthentication
	}
	prefix := []byte("fixture-cipher-v1:")
	if !bytes.HasPrefix(ciphertext, prefix) {
		return nil, ErrWindowsProtectedMediaPlaybackInvalidAuthentication
	}
	plaintext := append([]byte(nil), ciphertext[len(prefix):]...)
	for index := range plaintext {
		plaintext[index] ^= 0xa5
	}
	return plaintext, nil
}

func (o *windowsProtectedPlaybackFixtureOpener) counts() (int, int, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.openCount, o.decryptCount, o.sawHistoryGrant
}

type windowsProtectedPlaybackFixture struct {
	root       string
	cacheRoot  string
	keys       windowsE2EEFixture
	group      WindowsE2EEGroupStateMetadata
	plaintext  [][]byte
	ciphertext [][]byte
	opener     *windowsProtectedPlaybackFixtureOpener
	secret     []byte
}

func newWindowsProtectedPlaybackFixture(t testing.TB, epoch uint64) windowsProtectedPlaybackFixture {
	t.Helper()
	root := t.TempDir()
	keys := newWindowsE2EEFixture(t, 0x5a)
	group, err := keys.repository.PersistGroupState(
		keys.identity.InstallationID, keys.vectors.GroupID, epoch, "",
		keys.vectors.CommitDigest, keys.vectors.TargetSnapshotDigest,
		bytes.Repeat([]byte{0xb3}, 128), 0, 1100,
	)
	if err != nil {
		t.Fatal(err)
	}
	plaintext := [][]byte{[]byte("clear-audio-mac"), []byte("clear-audio-windows")}
	ciphertext := make([][]byte, len(plaintext))
	for index, body := range plaintext {
		ciphertext[index] = append([]byte("fixture-cipher-v1:"), body...)
		for offset := len("fixture-cipher-v1:"); offset < len(ciphertext[index]); offset++ {
			ciphertext[index][offset] ^= 0xa5
		}
	}
	return windowsProtectedPlaybackFixture{
		root: root, cacheRoot: filepath.Join(root, "ciphertext-cache"), keys: keys,
		group: group, plaintext: plaintext, ciphertext: ciphertext,
		opener: &windowsProtectedPlaybackFixtureOpener{}, secret: bytes.Repeat([]byte{0x77}, 32),
	}
}

func (f windowsProtectedPlaybackFixture) manifest(objectID string) WindowsStreamManifest {
	chunks := make([]WindowsStreamChunk, len(f.ciphertext))
	var offset int64
	var whole []byte
	for index, body := range f.ciphertext {
		chunks[index] = WindowsStreamChunk{Index: index, Start: offset, End: offset + int64(len(body)) - 1, SHA256: lowerSHA256(body)}
		offset += int64(len(body))
		whole = append(whole, body...)
	}
	digest := lowerSHA256(whole)
	return WindowsStreamManifest{
		Identity: "svm1.protected.shared-mac-windows", VariantURL: "/v1/media/" + objectID + "/variants/protected",
		ETag: `"sha256-` + digest + `"`, SHA256: digest, SizeBytes: int64(len(whole)), DurationMS: 20_000,
		Chunks: chunks, SeekMap: []WindowsStreamSeekPoint{{TimeMS: 0, Offset: 0}, {TimeMS: 10_000, Offset: chunks[1].Start}},
	}
}

func (f windowsProtectedPlaybackFixture) route(epoch uint64, objectID string) WindowsProtectedMediaPlaybackRoute {
	manifest := f.manifest(objectID)
	encrypted := []byte("fixture-encrypted-manifest")
	return WindowsProtectedMediaPlaybackRoute{
		Contract: "e2ee-media-audit.v1", Capability: "e2ee_media_v1",
		Suite: "AUDIT_FIXTURE_SUITE_NOT_FOR_PRODUCTION", Container: "AUDIT_FIXTURE_CONTAINER_NOT_FOR_PRODUCTION",
		ObjectID: objectID, SourceObjectID: "source_01K123456789ABCDEFGHJKMNPQ", Kind: WindowsProtectedMediaTrack,
		AuthorDeviceID: "dev_01K123456789ABCDEFGHJKMNP2", RecipientDeviceID: f.keys.vectors.DeviceID,
		GroupID: f.keys.vectors.GroupID, Epoch: epoch, Generation: 3,
		TargetSnapshotDigest: f.group.TargetSnapshotDigest, ExpiresAtMS: 20_000,
		ManifestDigest: lowerSHA256(encrypted), EncryptedManifest: encrypted,
		OpaqueKeyEnvelope: []byte("opaque-envelope"), AuthenticatedManifest: []byte("authenticated-manifest"),
		Signature: []byte("fixture-signature"), StreamManifest: manifest,
	}
}

func (f windowsProtectedPlaybackFixture) request(epoch uint64, objectID string) WindowsProtectedMediaPlaybackRequest {
	return WindowsProtectedMediaPlaybackRequest{
		ObjectID: objectID, RecipientDeviceID: f.keys.vectors.DeviceID, GroupID: f.keys.vectors.GroupID,
		ExpectedGroupRevision: f.group.Revision, ExpectedEpoch: epoch, ExpectedGeneration: 3,
		ExpectedTargetSnapshotDigest: f.group.TargetSnapshotDigest,
		PolicyAllowed:                true, DNDAllowed: true,
	}
}

func (f windowsProtectedPlaybackFixture) service(
	t testing.TB, transport WindowsProtectedMediaPlaybackTransport, production bool,
) *WindowsProtectedMediaPlaybackService {
	t.Helper()
	options := WindowsProtectedMediaPlaybackOptions{
		KeyState: f.keys.repository, Opener: f.opener, Transport: transport,
		CiphertextCacheRoot: f.cacheRoot, CacheInstallationSecret: f.secret,
	}
	var service *WindowsProtectedMediaPlaybackService
	var err error
	if production {
		service, err = NewWindowsProtectedMediaPlaybackService(options)
	} else {
		service, err = newWindowsProtectedMediaPlaybackServiceForAudit(options, func() int64 { return 2000 })
	}
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func windowsProtectedPlaybackDiskContains(root string, needle []byte) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || found || entry.IsDir() {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr == nil && bytes.Contains(body, needle) {
			found = true
		}
		return nil
	})
	return found
}

func TestWindowsProtectedMediaPlaybackSharedFixtureParity(t *testing.T) {
	fixture := newWindowsProtectedPlaybackFixture(t, 7)
	windowsVectors := loadWindowsProtectedPlaybackVectors(t, "windows-protected-media-playback-v1-vectors.json")
	macVectors := loadWindowsProtectedPlaybackVectors(t, "macos-protected-media-playback-v1-vectors.json")
	manifest := fixture.manifest("em_01K123456789ABCDEFGHJKMNPQ")
	if windowsVectors.Contract != "windows-protected-media-playback-v1-vectors" ||
		windowsVectors.Status != "audit-fixture-only-production-disabled" ||
		windowsVectors.FixtureSuite != macVectors.FixtureSuite ||
		windowsVectors.FixtureContainer != macVectors.FixtureContainer ||
		windowsVectors.CiphertextSHA256 != macVectors.CiphertextSHA256 ||
		windowsVectors.CiphertextSHA256 != manifest.SHA256 || len(windowsVectors.Chunks) != len(manifest.Chunks) {
		t.Fatalf("windows=%+v mac=%+v manifest=%+v", windowsVectors, macVectors, manifest)
	}
	for index, chunk := range windowsVectors.Chunks {
		if chunk.Index != index || chunk.Offset != manifest.Chunks[index].Start ||
			chunk.Size != int(manifest.Chunks[index].End-manifest.Chunks[index].Start+1) ||
			chunk.CiphertextSHA256 != manifest.Chunks[index].SHA256 ||
			chunk.AuthenticatedPlaintextSHA256 != lowerSHA256(fixture.plaintext[index]) ||
			chunk.CiphertextSHA256 != macVectors.Chunks[index].CiphertextSHA256 ||
			chunk.AuthenticatedPlaintextSHA256 != macVectors.Chunks[index].AuthenticatedPlaintextSHA256 {
			t.Fatalf("chunk[%d]=%+v", index, chunk)
		}
	}
}

func TestWindowsProtectedMediaPlaybackProductionAndPolicyRemainDark(t *testing.T) {
	fixture := newWindowsProtectedPlaybackFixture(t, 7)
	route := fixture.route(7, "em_01K123456789ABCDEFGHJKMNPQ")
	transport := newWindowsProtectedPlaybackTransport(route, fixture.ciphertext)
	_, err := fixture.service(t, transport, true).Prepare(context.Background(), fixture.request(7, route.ObjectID), 2000)
	if !errors.Is(err, ErrWindowsProtectedMediaPlaybackProductionDisabled) {
		t.Fatalf("production err=%v", err)
	}
	request := fixture.request(7, route.ObjectID)
	request.PolicyAllowed = false
	_, err = fixture.service(t, transport, false).Prepare(context.Background(), request, 2000)
	if !errors.Is(err, ErrWindowsProtectedMediaPlaybackBlocked) {
		t.Fatalf("policy err=%v", err)
	}
	if manifest, ranges := transport.counts(); manifest != 0 || ranges != 0 {
		t.Fatalf("network calls before policy manifest=%d ranges=%d", manifest, ranges)
	}
}

func TestWindowsProtectedMediaPlaybackIncrementalRestartCiphertextOnly(t *testing.T) {
	fixture := newWindowsProtectedPlaybackFixture(t, 7)
	route := fixture.route(7, "em_01K123456789ABCDEFGHJKMNPQ")
	transport := newWindowsProtectedPlaybackTransport(route, fixture.ciphertext)
	first, err := fixture.service(t, transport, false).Prepare(context.Background(), fixture.request(7, route.ObjectID), 2000)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	first.Route.EncryptedManifest[0] ^= 0xff
	first.Route.StreamManifest.Chunks[0].SHA256 = string(bytes.Repeat([]byte{'0'}, 64))
	got, err := first.Chunks.ReadChunk(context.Background(), 0)
	if err != nil || !bytes.Equal(got, fixture.plaintext[0]) {
		t.Fatalf("first=%q err=%v", got, err)
	}
	zeroBytes(got)
	restarted, err := fixture.service(t, transport, false).Prepare(context.Background(), fixture.request(7, route.ObjectID), 2100)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	got, err = restarted.Chunks.ReadChunk(context.Background(), 0)
	if err != nil || !bytes.Equal(got, fixture.plaintext[0]) {
		t.Fatalf("restart=%q err=%v", got, err)
	}
	zeroBytes(got)
	_, ranges := transport.counts()
	_, decrypts, _ := fixture.opener.counts()
	if ranges != 1 || decrypts != 2 {
		t.Fatalf("ranges=%d decrypts=%d", ranges, decrypts)
	}
	for _, secret := range [][]byte{fixture.plaintext[0], []byte("fixture-open-key-canary"), []byte("opaque-group-key-canary")} {
		if windowsProtectedPlaybackDiskContains(fixture.root, secret) {
			t.Fatalf("protected plaintext/secret persisted: %q", secret)
		}
	}
}

func TestWindowsProtectedMediaPlaybackTamperNeverReachesDecoder(t *testing.T) {
	fixture := newWindowsProtectedPlaybackFixture(t, 7)
	route := fixture.route(7, "em_01K123456789ABCDEFGHJKMNPQ")
	transport := newWindowsProtectedPlaybackTransport(route, fixture.ciphertext)
	transport.setCorrupt(true)
	prepared, err := fixture.service(t, transport, false).Prepare(context.Background(), fixture.request(7, route.ObjectID), 2000)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if _, err := prepared.Chunks.ReadChunk(context.Background(), 0); err == nil {
		t.Fatal("ciphertext tamper accepted")
	}
	_, decrypts, _ := fixture.opener.counts()
	if _, ranges := transport.counts(); ranges != 2 || decrypts != 0 {
		t.Fatalf("ranges=%d decrypts=%d", ranges, decrypts)
	}
	transport.setCorrupt(false)
	fixture.opener.mu.Lock()
	fixture.opener.rejectChunk = true
	fixture.opener.mu.Unlock()
	if _, err := prepared.Chunks.ReadChunk(context.Background(), 0); err == nil {
		t.Fatal("invalid record authentication accepted")
	}
	if stats := prepared.Cache.Stats(); stats.Bytes != 0 {
		t.Fatalf("failed authentication retained cache: %+v", stats)
	}
}

func TestWindowsProtectedMediaPlaybackDowngradeExpiryTargetAndGrantFailClosed(t *testing.T) {
	fixture := newWindowsProtectedPlaybackFixture(t, 8)
	downgradeID := "em_01K123456789ABCDEFGHJKMNPA"
	expiredID := "em_01K123456789ABCDEFGHJKMNPB"
	targetID := "em_01K123456789ABCDEFGHJKMNPC"
	objectID := "em_01K123456789ABCDEFGHJKMNPQ"
	route := fixture.route(8, downgradeID)
	transport := newWindowsProtectedPlaybackTransport(route, fixture.ciphertext)
	service := fixture.service(t, transport, false)
	route.Contract = "legacy-media.v0"
	transport.setRoute(route)
	if _, err := service.Prepare(context.Background(), fixture.request(8, downgradeID), 2000); !errors.Is(err, ErrWindowsProtectedMediaPlaybackDowngradeForbidden) {
		t.Fatalf("downgrade err=%v", err)
	}
	route = fixture.route(8, expiredID)
	route.ExpiresAtMS = 1999
	transport.setRoute(route)
	if _, err := service.Prepare(context.Background(), fixture.request(8, expiredID), 2000); !errors.Is(err, ErrWindowsProtectedMediaPlaybackExpired) {
		t.Fatalf("expiry err=%v", err)
	}
	route = fixture.route(8, targetID)
	route.TargetSnapshotDigest = string(bytes.Repeat([]byte{'d'}, 64))
	transport.setRoute(route)
	if _, err := service.Prepare(context.Background(), fixture.request(8, targetID), 2000); !errors.Is(err, ErrWindowsProtectedMediaPlaybackTargetChanged) {
		t.Fatalf("target err=%v", err)
	}

	historical := fixture.route(7, objectID)
	transport.setRoute(historical)
	request := fixture.request(7, objectID)
	if _, err := service.Prepare(context.Background(), request, 2000); !errors.Is(err, ErrWindowsProtectedMediaPlaybackMissingGrant) {
		t.Fatalf("missing grant err=%v", err)
	}
	grantID := "grant_01K123456789ABCDEFGHJKMNPQ"
	_, err := fixture.keys.repository.StoreGrant(fixture.keys.identity.InstallationID, grantID, fixture.keys.vectors.GroupID, 6, 7, 10_000, []byte("opaque-history-grant"), 0, 1500)
	if err != nil {
		t.Fatal(err)
	}
	request.HistoryGrantID = grantID
	prepared, err := service.Prepare(context.Background(), request, 2000)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if got, err := prepared.Chunks.ReadChunk(context.Background(), 1); err != nil || !bytes.Equal(got, fixture.plaintext[1]) {
		t.Fatalf("historical=%q err=%v", got, err)
	}
	if _, _, sawGrant := fixture.opener.counts(); !sawGrant {
		t.Fatal("provider did not receive history grant")
	}
	if err := fixture.keys.repository.RevokeGrant(fixture.keys.identity.InstallationID, grantID); err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Chunks.ReadChunk(context.Background(), 0); err == nil {
		t.Fatal("revoked history grant accepted")
	}
}

func TestWindowsProtectedMediaPlaybackRevocationMarkerIsMonotonicAcrossActors(t *testing.T) {
	fixture := newWindowsProtectedPlaybackFixture(t, 7)
	objectID := "em_01K123456789ABCDEFGHJKMNPQ"
	route := fixture.route(7, objectID)
	transport := newWindowsProtectedPlaybackTransport(route, fixture.ciphertext)
	request := fixture.request(7, objectID)
	actorA, err := fixture.service(t, transport, false).Prepare(context.Background(), request, 2000)
	if err != nil {
		t.Fatal(err)
	}
	defer actorA.Close()
	actorB, err := fixture.service(t, transport, false).Prepare(context.Background(), request, 2000)
	if err != nil {
		t.Fatal(err)
	}
	defer actorB.Close()
	if _, err := actorA.Chunks.ReadChunk(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if err := actorB.Revoke(); err != nil {
		t.Fatal(err)
	}
	if _, err := actorA.Chunks.ReadChunk(context.Background(), 0); err == nil {
		t.Fatal("parallel actor erased revocation")
	}
	rangesBefore := func() int { _, value := transport.counts(); return value }()
	restarted, err := fixture.service(t, transport, false).Prepare(context.Background(), request, 2100)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if _, err := restarted.Chunks.ReadChunk(context.Background(), 0); err == nil {
		t.Fatal("restart erased revocation")
	}
	if _, rangesAfter := transport.counts(); rangesAfter != rangesBefore {
		t.Fatalf("revoked restart fetched range before=%d after=%d", rangesBefore, rangesAfter)
	}
}

func TestWindowsProtectedMediaPlaybackConcurrentDistinctActorsMergeDurableCache(t *testing.T) {
	fixture := newWindowsProtectedPlaybackFixture(t, 7)
	objectA := "em_01K123456789ABCDEFGHJKMNPA"
	objectB := "em_01K123456789ABCDEFGHJKMNPB"
	routeA := fixture.route(7, objectA)
	routeB := fixture.route(7, objectB)
	transportA := newWindowsProtectedPlaybackTransport(routeA, fixture.ciphertext)
	transportB := newWindowsProtectedPlaybackTransport(routeB, fixture.ciphertext)
	actorA, err := fixture.service(t, transportA, false).Prepare(context.Background(), fixture.request(7, objectA), 2000)
	if err != nil {
		t.Fatal(err)
	}
	defer actorA.Close()
	actorB, err := fixture.service(t, transportB, false).Prepare(context.Background(), fixture.request(7, objectB), 2000)
	if err != nil {
		t.Fatal(err)
	}
	defer actorB.Close()
	start := make(chan struct{})
	errorsSeen := make(chan error, 2)
	for _, actor := range []*WindowsProtectedMediaPreparedPlayback{actorA, actorB} {
		go func(value *WindowsProtectedMediaPreparedPlayback) {
			<-start
			_, readErr := value.Chunks.ReadChunk(context.Background(), 0)
			errorsSeen <- readErr
		}(actor)
	}
	close(start)
	for range 2 {
		if err := <-errorsSeen; err != nil {
			t.Fatal(err)
		}
	}
	restartA, err := fixture.service(t, transportA, false).Prepare(context.Background(), fixture.request(7, objectA), 2100)
	if err != nil {
		t.Fatal(err)
	}
	defer restartA.Close()
	restartB, err := fixture.service(t, transportB, false).Prepare(context.Background(), fixture.request(7, objectB), 2100)
	if err != nil {
		t.Fatal(err)
	}
	defer restartB.Close()
	if _, err := restartA.Chunks.ReadChunk(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := restartB.Chunks.ReadChunk(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	if _, ranges := transportA.counts(); ranges != 1 {
		t.Fatalf("actor A lost durable cache, ranges=%d", ranges)
	}
	if _, ranges := transportB.counts(); ranges != 1 {
		t.Fatalf("actor B lost durable cache, ranges=%d", ranges)
	}
	if err := restartB.Revoke(); err != nil {
		t.Fatal(err)
	}
	if _, err := restartA.Chunks.ReadChunk(context.Background(), 0); err != nil {
		t.Fatalf("distinct actor poisoned by B revocation: %v", err)
	}
	finalB, err := fixture.service(t, transportB, false).Prepare(context.Background(), fixture.request(7, objectB), 2200)
	if err != nil {
		t.Fatal(err)
	}
	defer finalB.Close()
	if _, err := finalB.Chunks.ReadChunk(context.Background(), 0); err == nil {
		t.Fatal("B tombstone lost after concurrent A cache hit")
	}
}

func TestWindowsProtectedMediaPlaybackMembershipRotationPurgesButAllowsBoundedRegrant(t *testing.T) {
	fixture := newWindowsProtectedPlaybackFixture(t, 7)
	objectID := "em_01K123456789ABCDEFGHJKMNPQ"
	route := fixture.route(7, objectID)
	transport := newWindowsProtectedPlaybackTransport(route, fixture.ciphertext)
	prepared, err := fixture.service(t, transport, false).Prepare(
		context.Background(), fixture.request(7, objectID), 2000,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if _, err := prepared.Chunks.ReadChunk(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	rotated, err := fixture.keys.repository.PersistGroupState(
		fixture.keys.identity.InstallationID, fixture.keys.vectors.GroupID, 8,
		fixture.group.CommitDigest, fixture.keys.vectors.NextCommitDigest,
		fixture.keys.vectors.NextTargetSnapshotDigest, bytes.Repeat([]byte{0xc4}, 128),
		fixture.group.Revision, 2100,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Chunks.ReadChunk(context.Background(), 0); err == nil {
		t.Fatal("membership rotation accepted by frozen reader")
	}
	if stats := prepared.Cache.Stats(); stats.Bytes != 0 {
		t.Fatalf("rotation retained ciphertext: %+v", stats)
	}
	grantID := "grant_01K123456789ABCDEFGHJKMNP3"
	if _, err := fixture.keys.repository.StoreGrant(
		fixture.keys.identity.InstallationID, grantID, fixture.keys.vectors.GroupID,
		7, 7, 10_000, []byte("bounded-regrant"), 0, 2200,
	); err != nil {
		t.Fatal(err)
	}
	request := fixture.request(7, objectID)
	request.ExpectedGroupRevision = rotated.Revision
	request.HistoryGrantID = grantID
	regranted, err := fixture.service(t, transport, false).Prepare(context.Background(), request, 2300)
	if err != nil {
		t.Fatal(err)
	}
	defer regranted.Close()
	if got, err := regranted.Chunks.ReadChunk(context.Background(), 0); err != nil || !bytes.Equal(got, fixture.plaintext[0]) {
		t.Fatalf("bounded regrant=%q err=%v", got, err)
	}
}

type windowsProtectedPlaybackDecoder struct {
	mu       sync.Mutex
	received []byte
}

func (d *windowsProtectedPlaybackDecoder) Decode(ctx context.Context, request WindowsStreamDecodeRequest) error {
	for index := range request.Manifest.Chunks {
		body, err := request.Chunks.ReadChunk(ctx, index)
		if err != nil {
			return err
		}
		if index == 0 {
			d.mu.Lock()
			d.received = append([]byte(nil), body...)
			d.mu.Unlock()
		}
		zeroBytes(body)
	}
	return io.EOF
}

func (d *windowsProtectedPlaybackDecoder) value() []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]byte(nil), d.received...)
}

func TestWindowsProtectedMediaPlaybackCandidatePlayerGetsAuthenticatedReader(t *testing.T) {
	fixture := newWindowsProtectedPlaybackFixture(t, 7)
	objectID := "em_01K123456789ABCDEFGHJKMNPQ"
	route := fixture.route(7, objectID)
	transport := newWindowsProtectedPlaybackTransport(route, fixture.ciphertext)
	prepared, err := fixture.service(t, transport, false).Prepare(context.Background(), fixture.request(7, objectID), 2000)
	if err != nil {
		t.Fatal(err)
	}
	decoder := &windowsProtectedPlaybackDecoder{}
	player, err := prepared.MakeCandidatePlayer(decoder, fixedClock{ok: true}, func(string, any) {})
	if err != nil {
		t.Fatal(err)
	}
	defer player.Close()
	load := protocolStreamLoad(route.StreamManifest, 1, 0, nowMS()+3000)
	if err := player.Load(load, route.StreamManifest); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !bytes.Equal(decoder.value(), fixture.plaintext[0]) {
		time.Sleep(time.Millisecond)
	}
	if !bytes.Equal(decoder.value(), fixture.plaintext[0]) || bytes.Equal(decoder.value(), fixture.ciphertext[0]) {
		t.Fatalf("decoder received=%q", decoder.value())
	}
	if snapshot := player.Snapshot(); snapshot.RingBytes > snapshot.RingCeilingBytes || snapshot.RingCeilingBytes != windowsStreamPCMRingBytes {
		t.Fatalf("snapshot=%+v", snapshot)
	}
}

func TestWindowsProtectedMediaOpenLeaseRedactsAndZeros(t *testing.T) {
	secret := []byte("secret-open-lease")
	lease := NewWindowsProtectedMediaOpenLease(secret)
	if lease.String() != "WindowsProtectedMediaOpenLease{<redacted>}" {
		t.Fatalf("unsafe String=%s", lease)
	}
	var digest string
	if err := lease.WithOpaqueState(func(value []byte) error {
		hash := sha256.Sum256(value)
		digest = hex.EncodeToString(hash[:])
		return nil
	}); err != nil || digest != lowerSHA256(secret) {
		t.Fatalf("digest=%s err=%v", digest, err)
	}
	lease.Destroy()
	if err := lease.WithOpaqueState(func([]byte) error { return nil }); !errors.Is(err, ErrWindowsProtectedMediaPlaybackRevoked) {
		t.Fatalf("destroyed lease err=%v", err)
	}
}
