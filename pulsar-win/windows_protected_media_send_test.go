package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

type windowsProtectedSendVectors struct {
	Contract         string `json:"contract"`
	Status           string `json:"status"`
	FixtureSuite     string `json:"fixtureSuite"`
	FixtureContainer string `json:"fixtureContainer"`
	SourceSHA256     string `json:"sourceSHA256"`
	ManifestSHA256   string `json:"manifestSHA256"`
	CiphertextSHA256 string `json:"ciphertextSHA256"`
	Chunks           []struct {
		Index  int    `json:"index"`
		Offset int64  `json:"offset"`
		Size   int    `json:"size"`
		Nonce  string `json:"nonce"`
		SHA256 string `json:"sha256"`
	} `json:"chunks"`
	Resume struct {
		InterruptedAtChunk           int    `json:"interruptedAtChunk"`
		ExpectedGeneration           uint64 `json:"expectedGeneration"`
		ExpectedSealCount            int    `json:"expectedSealCount"`
		ExpectedStageCount           int    `json:"expectedStageCount"`
		ExpectedUploadedChunkIndices []int  `json:"expectedUploadedChunkIndices"`
	} `json:"resume"`
}

func loadWindowsProtectedSendVectors(t testing.TB) windowsProtectedSendVectors {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "protocol", "windows-protected-media-send-v1-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors windowsProtectedSendVectors
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	return vectors
}

type windowsProtectedFixtureSealer struct {
	mu                 sync.Mutex
	sealCount          int
	duplicateNonce     bool
	verificationResult bool
	block              chan struct{}
}

func newWindowsProtectedFixtureSealer() *windowsProtectedFixtureSealer {
	return &windowsProtectedFixtureSealer{verificationResult: true}
}

func (*windowsProtectedFixtureSealer) ProductionApproved() bool { return false }

func (s *windowsProtectedFixtureSealer) Seal(ctx context.Context, source []byte, value WindowsProtectedMediaSealContext, identity *WindowsE2EEDeviceIdentityLease, group *WindowsE2EEGroupStateLease) (WindowsProtectedMediaSealedArtifact, error) {
	s.mu.Lock()
	s.sealCount++
	duplicate := s.duplicateNonce
	block := s.block
	s.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return WindowsProtectedMediaSealedArtifact{}, ctx.Err()
		}
	}
	if identity == nil || group == nil || identity.Metadata.DeviceID == "" || group.Metadata.Epoch != value.Epoch {
		return WindowsProtectedMediaSealedArtifact{}, errors.New("missing witnessed key state")
	}
	midpoint := len(source) / 2
	if midpoint < 1 {
		midpoint = 1
	}
	parts := [][]byte{source[:midpoint], source[midpoint:]}
	chunks := make([]WindowsProtectedMediaCiphertextChunk, 0, 2)
	for index, part := range parts {
		if len(part) == 0 {
			continue
		}
		nonce := fmt.Sprintf("fixture-nonce-%d-%d", value.Generation, index)
		if duplicate {
			nonce = "fixture-nonce"
		}
		ciphertext := append([]byte(fmt.Sprintf("fixture-ciphertext-%d:", index)), part...)
		chunks = append(chunks, WindowsProtectedMediaCiphertextChunk{Nonce: nonce, Ciphertext: ciphertext})
	}
	return WindowsProtectedMediaSealedArtifact{
		Contract: "e2ee-media-audit.v1", Capability: "e2ee_media_v1",
		Suite: "AUDIT_FIXTURE_SUITE_NOT_FOR_PRODUCTION", Container: "AUDIT_FIXTURE_CONTAINER_NOT_FOR_PRODUCTION",
		Context: value, EncryptedManifest: []byte(fmt.Sprintf("fixture-encrypted-manifest-%d", value.Generation)),
		OpaqueKeyEnvelopes: []byte("fixture-opaque-envelopes"), AuthenticatedManifest: []byte("fixture-authenticated-manifest"),
		Signature: []byte("fixture-signature"), Chunks: chunks,
	}, nil
}

func (s *windowsProtectedFixtureSealer) Verify(_ context.Context, artifact WindowsProtectedMediaSealedArtifact) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.verificationResult && bytes.Equal(artifact.Signature, []byte("fixture-signature"))
}

func (s *windowsProtectedFixtureSealer) counts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sealCount
}

type windowsProtectedUploadedChunk struct {
	Index      int
	ByteOffset int64
	Digest     string
	Bytes      []byte
}

type windowsProtectedFixtureUploader struct {
	mu            sync.Mutex
	stages        []WindowsProtectedMediaStageRequest
	chunks        []windowsProtectedUploadedChunk
	finalizeCount int
	deleteCount   int
	failChunkOnce *int
	didFail       bool
	stageEntered  chan struct{}
	stageRelease  chan struct{}
	onFinalize    func()
}

func (u *windowsProtectedFixtureUploader) Stage(ctx context.Context, request WindowsProtectedMediaStageRequest) (WindowsProtectedMediaRemoteObject, error) {
	u.mu.Lock()
	if len(u.stages) > 0 {
		if !reflect.DeepEqual(u.stages[0], request) {
			u.mu.Unlock()
			return WindowsProtectedMediaRemoteObject{}, errors.New("idempotency mismatch")
		}
	} else {
		u.stages = append(u.stages, cloneWindowsProtectedStage(request))
	}
	entered, release := u.stageEntered, u.stageRelease
	u.mu.Unlock()
	if entered != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return WindowsProtectedMediaRemoteObject{}, ctx.Err()
		}
	}
	return WindowsProtectedMediaRemoteObject{ObjectID: "em_01K123456789ABCDEFGHJKMNPQ", Revision: 1}, nil
}

func (u *windowsProtectedFixtureUploader) PutChunk(_ context.Context, _ string, _ string, index int, offset int64, digest string, ciphertext []byte) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.failChunkOnce != nil && *u.failChunkOnce == index && !u.didFail {
		u.didFail = true
		return errors.New("injected chunk failure")
	}
	for _, previous := range u.chunks {
		if previous.Index == index {
			if previous.ByteOffset != offset || previous.Digest != digest || !bytes.Equal(previous.Bytes, ciphertext) {
				return errors.New("idempotency mismatch")
			}
			return nil
		}
	}
	u.chunks = append(u.chunks, windowsProtectedUploadedChunk{Index: index, ByteOffset: offset, Digest: digest, Bytes: cloneBytes(ciphertext)})
	return nil
}

func (u *windowsProtectedFixtureUploader) Finalize(_ context.Context, objectID, _ string, revision uint64) (WindowsProtectedMediaRemoteObject, error) {
	u.mu.Lock()
	u.finalizeCount++
	onFinalize := u.onFinalize
	u.mu.Unlock()
	if onFinalize != nil {
		onFinalize()
	}
	return WindowsProtectedMediaRemoteObject{ObjectID: objectID, Revision: revision + 1}, nil
}

func (u *windowsProtectedFixtureUploader) Delete(_ context.Context, _ string, _ string, _ uint64) error {
	u.mu.Lock()
	u.deleteCount++
	u.mu.Unlock()
	return nil
}

func (u *windowsProtectedFixtureUploader) snapshot() ([]WindowsProtectedMediaStageRequest, []windowsProtectedUploadedChunk, int, int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	stages := append([]WindowsProtectedMediaStageRequest(nil), u.stages...)
	chunks := append([]windowsProtectedUploadedChunk(nil), u.chunks...)
	return stages, chunks, u.finalizeCount, u.deleteCount
}

func cloneWindowsProtectedStage(value WindowsProtectedMediaStageRequest) WindowsProtectedMediaStageRequest {
	value.EncryptedManifest = cloneBytes(value.EncryptedManifest)
	value.OpaqueKeyEnvelopes = cloneBytes(value.OpaqueKeyEnvelopes)
	value.AuthenticatedManifest = cloneBytes(value.AuthenticatedManifest)
	value.Signature = cloneBytes(value.Signature)
	return value
}

type windowsProtectedSendFixture struct {
	root           string
	plaintextRoot  string
	ciphertextRoot string
	source         string
	keyState       *WindowsE2EEKeyStateRepository
	identity       WindowsE2EEDeviceIdentityMetadata
	group          WindowsE2EEGroupStateMetadata
	sealer         *windowsProtectedFixtureSealer
	uploader       *windowsProtectedFixtureUploader
}

func newWindowsProtectedSendFixture(t testing.TB) windowsProtectedSendFixture {
	t.Helper()
	root := t.TempDir()
	plaintextRoot := filepath.Join(root, "plaintext")
	if err := os.MkdirAll(plaintextRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(plaintextRoot, "recording.wav")
	if err := os.WriteFile(source, []byte("private audio fixture bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	keyFixture := newWindowsE2EEFixture(t, 0x42)
	group := installWindowsE2EEGroup(t, keyFixture)
	return windowsProtectedSendFixture{
		root: root, plaintextRoot: plaintextRoot, ciphertextRoot: filepath.Join(root, "ciphertext"), source: source,
		keyState: keyFixture.repository, identity: keyFixture.identity, group: group,
		sealer: newWindowsProtectedFixtureSealer(), uploader: &windowsProtectedFixtureUploader{},
	}
}

func (f windowsProtectedSendFixture) request() WindowsProtectedMediaSendRequest {
	return WindowsProtectedMediaSendRequest{
		DraftID: "draft_01K123456789ABCDEFGHJKMNPQ", SourceObjectID: "source_01K123456789ABCDEFGHJKMNPQ",
		SourcePath: f.source, PlaintextPolicy: WindowsProtectedMediaAppPrivateDeleteOnTerminal, Kind: WindowsProtectedMediaClip,
		AuthorDeviceID: f.identity.DeviceID, GroupID: f.group.GroupID, ExpectedGroupRevision: f.group.Revision,
		ExpectedTargetSnapshotDigest: f.group.TargetSnapshotDigest,
		Recipients: []WindowsProtectedMediaRecipient{
			{DeviceID: "dev_01K123456789ABCDEFGHJKMNPQ", Verified: true, CurrentMember: true, SupportsProtectedMedia: true},
			{DeviceID: "dev_01K123456789ABCDEFGHJKMNP2", Verified: true, CurrentMember: true, SupportsProtectedMedia: true},
		},
		DeclaredDurationMS: 1000, RightsConfirmed: true, TargetConfirmed: true, ExpiresAtMS: 10_000,
	}
}

func (f windowsProtectedSendFixture) service(t testing.TB, audit bool) *WindowsProtectedMediaSendService {
	t.Helper()
	options := WindowsProtectedMediaSendOptions{KeyState: f.keyState, Sealer: f.sealer, Uploader: f.uploader, CiphertextRoot: f.ciphertextRoot, PlaintextDraftRoot: f.plaintextRoot}
	var service *WindowsProtectedMediaSendService
	var err error
	if audit {
		service, err = newWindowsProtectedMediaSendServiceForAudit(options)
	} else {
		service, err = NewWindowsProtectedMediaSendService(options)
	}
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestWindowsProtectedMediaProductionProviderGate(t *testing.T) {
	fixture := newWindowsProtectedSendFixture(t)
	_, err := fixture.service(t, false).Send(context.Background(), fixture.request(), 2000, nil)
	if !errors.Is(err, ErrWindowsProtectedMediaProductionDisabled) || fixture.sealer.counts() != 0 {
		t.Fatalf("err=%v sealCount=%d", err, fixture.sealer.counts())
	}
	stages, _, _, _ := fixture.uploader.snapshot()
	if len(stages) != 0 {
		t.Fatal("production-disabled send reached uploader")
	}
}

func TestWindowsProtectedMediaPublishesGoldenCiphertextAndCleansOwnedPlaintext(t *testing.T) {
	fixture := newWindowsProtectedSendFixture(t)
	vectors := loadWindowsProtectedSendVectors(t)
	var phases []WindowsProtectedMediaProgressPhase
	publication, err := fixture.service(t, true).Send(context.Background(), fixture.request(), 2000, func(value WindowsProtectedMediaSendProgress) {
		phases = append(phases, value.Phase)
	})
	if err != nil {
		t.Fatal(err)
	}
	stages, chunks, finalized, _ := fixture.uploader.snapshot()
	if vectors.Contract != "windows-protected-media-send-v1-vectors" || vectors.Status != "audit-fixture-only-production-disabled" || publication.Generation != 1 || len(stages) != 1 || len(chunks) != 2 || finalized != 1 {
		t.Fatalf("publication=%+v stages=%d chunks=%d finalize=%d vectors=%+v", publication, len(stages), len(chunks), finalized, vectors)
	}
	if stages[0].ManifestDigest != vectors.ManifestSHA256 || stages[0].CiphertextDigest != vectors.CiphertextSHA256 || windowsProtectedDigest([]byte("private audio fixture bytes")) != vectors.SourceSHA256 {
		t.Fatalf("stage=%+v", stages[0])
	}
	for index, chunk := range chunks {
		vector := vectors.Chunks[index]
		if chunk.Index != vector.Index || chunk.ByteOffset != vector.Offset || len(chunk.Bytes) != vector.Size || chunk.Digest != vector.SHA256 {
			t.Fatalf("chunk[%d]=%+v vector=%+v", index, chunk, vector)
		}
	}
	if _, err := os.Stat(fixture.source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned plaintext not deleted: %v", err)
	}
	if len(phases) == 0 || phases[0] != WindowsProtectedMediaPreparing || phases[len(phases)-1] != WindowsProtectedMediaPublished {
		t.Fatalf("phases=%v", phases)
	}
	state, err := fixture.keyState.LoadGroupState(fixture.identity.InstallationID, fixture.group.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Destroy()
	if state.Metadata.SendGeneration != 1 {
		t.Fatalf("generation=%d", state.Metadata.SendGeneration)
	}
}

func TestWindowsProtectedMediaInterruptedUploadResumesExactCiphertext(t *testing.T) {
	fixture := newWindowsProtectedSendFixture(t)
	vectors := loadWindowsProtectedSendVectors(t)
	fixture.uploader.failChunkOnce = &vectors.Resume.InterruptedAtChunk
	service := fixture.service(t, true)
	if _, err := service.Send(context.Background(), fixture.request(), 2000, nil); !errors.Is(err, ErrWindowsProtectedMediaTransport) {
		t.Fatalf("first err=%v", err)
	}
	publication, err := service.Send(context.Background(), fixture.request(), 2100, nil)
	if err != nil {
		t.Fatal(err)
	}
	stages, chunks, _, _ := fixture.uploader.snapshot()
	indices := make([]int, 0, len(chunks))
	for _, chunk := range chunks {
		indices = append(indices, chunk.Index)
	}
	if publication.Generation != vectors.Resume.ExpectedGeneration || fixture.sealer.counts() != vectors.Resume.ExpectedSealCount || len(stages) != vectors.Resume.ExpectedStageCount || !reflect.DeepEqual(indices, vectors.Resume.ExpectedUploadedChunkIndices) {
		t.Fatalf("publication=%+v seal=%d stages=%d indices=%v", publication, fixture.sealer.counts(), len(stages), indices)
	}
	state, err := fixture.keyState.LoadGroupState(fixture.identity.InstallationID, fixture.group.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Destroy()
	if state.Metadata.SendGeneration != 1 {
		t.Fatalf("generation reused: %d", state.Metadata.SendGeneration)
	}
}

func TestWindowsProtectedMediaTargetFailuresPrecedeGenerationReservation(t *testing.T) {
	for _, test := range []struct {
		name      string
		configure func(*WindowsProtectedMediaSendRequest)
		want      error
	}{
		{name: "unsupported", want: ErrWindowsProtectedMediaUnsupportedTarget, configure: func(request *WindowsProtectedMediaSendRequest) { request.Recipients[0].SupportsProtectedMedia = false }},
		{name: "removed", want: ErrWindowsProtectedMediaTargetChanged, configure: func(request *WindowsProtectedMediaSendRequest) { request.Recipients[0].CurrentMember = false }},
		{name: "unverified", want: ErrWindowsProtectedMediaTargetChanged, configure: func(request *WindowsProtectedMediaSendRequest) { request.Recipients[0].Verified = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWindowsProtectedSendFixture(t)
			request := fixture.request()
			test.configure(&request)
			_, err := fixture.service(t, true).Send(context.Background(), request, 2000, nil)
			if !errors.Is(err, test.want) {
				t.Fatalf("err=%v want=%v", err, test.want)
			}
			state, loadErr := fixture.keyState.LoadGroupState(fixture.identity.InstallationID, fixture.group.GroupID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			defer state.Destroy()
			if state.Metadata.SendGeneration != 0 || fixture.sealer.counts() != 0 {
				t.Fatalf("generation=%d seal=%d", state.Metadata.SendGeneration, fixture.sealer.counts())
			}
		})
	}
}

func TestWindowsProtectedMediaProviderFailuresConsumeReservationWithoutPersistence(t *testing.T) {
	for _, test := range []struct {
		name      string
		configure func(*windowsProtectedFixtureSealer)
	}{
		{name: "duplicate nonce", configure: func(sealer *windowsProtectedFixtureSealer) { sealer.duplicateNonce = true }},
		{name: "invalid signature", configure: func(sealer *windowsProtectedFixtureSealer) { sealer.verificationResult = false }},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newWindowsProtectedSendFixture(t)
			test.configure(fixture.sealer)
			_, err := fixture.service(t, true).Send(context.Background(), fixture.request(), 2000, nil)
			if !errors.Is(err, ErrWindowsProtectedMediaInvalidArtifact) {
				t.Fatalf("err=%v", err)
			}
			if _, statErr := os.Stat(filepath.Join(fixture.ciphertextRoot, fixture.request().DraftID)); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("ciphertext persisted: %v", statErr)
			}
			state, loadErr := fixture.keyState.LoadGroupState(fixture.identity.InstallationID, fixture.group.GroupID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			defer state.Destroy()
			if state.Metadata.SendGeneration != 1 {
				t.Fatalf("generation=%d", state.Metadata.SendGeneration)
			}
		})
	}
}

func TestWindowsProtectedMediaResumeRejectsSourceCiphertextAuthorAndEpochDrift(t *testing.T) {
	newInterrupted := func(t *testing.T) (windowsProtectedSendFixture, *WindowsProtectedMediaSendService) {
		fixture := newWindowsProtectedSendFixture(t)
		index := 1
		fixture.uploader.failChunkOnce = &index
		service := fixture.service(t, true)
		if _, err := service.Send(context.Background(), fixture.request(), 2000, nil); !errors.Is(err, ErrWindowsProtectedMediaTransport) {
			t.Fatalf("prepare err=%v", err)
		}
		return fixture, service
	}
	t.Run("source", func(t *testing.T) {
		fixture, service := newInterrupted(t)
		if err := os.WriteFile(fixture.source, []byte("modified plaintext"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Send(context.Background(), fixture.request(), 2100, nil); !errors.Is(err, ErrWindowsProtectedMediaInvalidRequest) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("ciphertext", func(t *testing.T) {
		fixture, service := newInterrupted(t)
		path := filepath.Join(fixture.ciphertextRoot, fixture.request().DraftID, "chunk-0000.bin")
		value, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		value[0] ^= 0xff
		if err := os.WriteFile(path, value, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Send(context.Background(), fixture.request(), 2100, nil); !errors.Is(err, ErrWindowsProtectedMediaPersistence) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("author", func(t *testing.T) {
		fixture, service := newInterrupted(t)
		request := fixture.request()
		request.AuthorDeviceID = "dev_01K123456789ABCDEFGHJKMNP2"
		if _, err := service.Send(context.Background(), request, 2100, nil); !errors.Is(err, ErrWindowsProtectedMediaInvalidRequest) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("epoch", func(t *testing.T) {
		fixture, service := newInterrupted(t)
		current, err := fixture.keyState.LoadGroupState(fixture.identity.InstallationID, fixture.group.GroupID)
		if err != nil {
			t.Fatal(err)
		}
		current.Destroy()
		_, err = fixture.keyState.PersistGroupState(fixture.identity.InstallationID, fixture.group.GroupID, fixture.group.Epoch+1, fixture.group.CommitDigest, stringsOf('e', 64), fixture.group.TargetSnapshotDigest, bytes.Repeat([]byte{0x44}, 64), fixture.group.Revision+1, 2050)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Send(context.Background(), fixture.request(), 2100, nil); !errors.Is(err, ErrWindowsProtectedMediaStaleKeyState) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestWindowsProtectedMediaCancelAndExpiryDeleteRemoteAndOwnedPlaintext(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprintf("expired=%t", expired), func(t *testing.T) {
			fixture := newWindowsProtectedSendFixture(t)
			index := 1
			fixture.uploader.failChunkOnce = &index
			service := fixture.service(t, true)
			if _, err := service.Send(context.Background(), fixture.request(), 2000, nil); !errors.Is(err, ErrWindowsProtectedMediaTransport) {
				t.Fatalf("send err=%v", err)
			}
			if expired {
				removed, err := service.RecoverExpiredDrafts(context.Background(), 10_001, 1)
				if err != nil || removed != 1 {
					t.Fatalf("removed=%d err=%v", removed, err)
				}
			} else if err := service.Cancel(context.Background(), fixture.request().DraftID); err != nil {
				t.Fatal(err)
			}
			_, _, _, deleted := fixture.uploader.snapshot()
			if deleted != 1 {
				t.Fatalf("deleteCount=%d", deleted)
			}
			if _, err := os.Stat(fixture.source); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("plaintext remains: %v", err)
			}
		})
	}
}

func TestWindowsProtectedMediaUserOwnedSourceRetainedAndKindsSharePipeline(t *testing.T) {
	for _, kind := range []WindowsProtectedMediaKind{WindowsProtectedMediaClip, WindowsProtectedMediaTrack, WindowsProtectedMediaSavedCue} {
		t.Run(string(kind), func(t *testing.T) {
			fixture := newWindowsProtectedSendFixture(t)
			request := fixture.request()
			request.Kind = kind
			request.PlaintextPolicy = WindowsProtectedMediaUserOwnedRetain
			if _, err := fixture.service(t, true).Send(context.Background(), request, 2000, nil); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(fixture.source); err != nil {
				t.Fatalf("user-owned source deleted: %v", err)
			}
			stages, _, _, _ := fixture.uploader.snapshot()
			if len(stages) != 1 || stages[0].Kind != kind {
				t.Fatalf("stages=%+v", stages)
			}
		})
	}
}

func TestWindowsProtectedMediaConcurrentDuplicateDraftFailsBusyAndRecoverySkipsActive(t *testing.T) {
	fixture := newWindowsProtectedSendFixture(t)
	fixture.uploader.stageEntered = make(chan struct{}, 1)
	fixture.uploader.stageRelease = make(chan struct{})
	service := fixture.service(t, true)
	result := make(chan error, 1)
	go func() {
		_, err := service.Send(context.Background(), fixture.request(), 2000, nil)
		result <- err
	}()
	<-fixture.uploader.stageEntered
	if _, err := service.Send(context.Background(), fixture.request(), 2001, nil); !errors.Is(err, ErrWindowsProtectedMediaBusy) {
		t.Fatalf("duplicate err=%v", err)
	}
	removed, err := service.RecoverExpiredDrafts(context.Background(), 10_001, 1)
	if err != nil || removed != 0 {
		t.Fatalf("active recovery removed=%d err=%v", removed, err)
	}
	close(fixture.uploader.stageRelease)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestWindowsProtectedMediaStoredStateRejectsUnknownFields(t *testing.T) {
	fixture := newWindowsProtectedSendFixture(t)
	index := 1
	fixture.uploader.failChunkOnce = &index
	service := fixture.service(t, true)
	if _, err := service.Send(context.Background(), fixture.request(), 2000, nil); !errors.Is(err, ErrWindowsProtectedMediaTransport) {
		t.Fatalf("send err=%v", err)
	}
	path := filepath.Join(fixture.ciphertextRoot, fixture.request().DraftID, "state.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"plaintext":"forbidden"`), 1)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Send(context.Background(), fixture.request(), 2100, nil); !errors.Is(err, ErrWindowsProtectedMediaPersistence) {
		t.Fatalf("err=%v", err)
	}
}

func TestWindowsProtectedMediaPublishedCheckpointDoesNotRefinalizeAfterCleanupRetry(t *testing.T) {
	fixture := newWindowsProtectedSendFixture(t)
	fixture.uploader.onFinalize = func() {
		if err := os.Remove(fixture.source); err != nil {
			return
		}
		_ = os.Mkdir(fixture.source, 0o700)
	}
	service := fixture.service(t, true)
	if _, err := service.Send(context.Background(), fixture.request(), 2000, nil); !errors.Is(err, ErrWindowsProtectedMediaLocalCleanup) {
		t.Fatalf("first err=%v", err)
	}
	_, _, finalized, _ := fixture.uploader.snapshot()
	if finalized != 1 {
		t.Fatalf("finalizeCount=%d", finalized)
	}
	fixture.uploader.mu.Lock()
	fixture.uploader.onFinalize = nil
	fixture.uploader.mu.Unlock()
	if err := os.RemoveAll(fixture.source); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.source, []byte("private audio fixture bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	publication, err := service.Send(context.Background(), fixture.request(), 2100, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, finalized, _ = fixture.uploader.snapshot()
	if finalized != 1 || publication.Revision != 2 {
		t.Fatalf("publication=%+v finalizeCount=%d", publication, finalized)
	}
}

func stringsOf(value byte, count int) string { return string(bytes.Repeat([]byte{value}, count)) }
