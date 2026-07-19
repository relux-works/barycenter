package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type windowsE2EEVectors struct {
	Contract                 string `json:"contract"`
	Status                   string `json:"status"`
	InstallationRandomHex    string `json:"installation_random_hex"`
	DeviceID                 string `json:"device_id"`
	KeyFormat                string `json:"key_format"`
	GroupID                  string `json:"group_id"`
	InitialEpoch             uint64 `json:"initial_epoch"`
	NextEpoch                uint64 `json:"next_epoch"`
	CommitDigest             string `json:"commit_digest"`
	NextCommitDigest         string `json:"next_commit_digest"`
	TargetSnapshotDigest     string `json:"target_snapshot_digest"`
	NextTargetSnapshotDigest string `json:"next_target_snapshot_digest"`
	Transitions              []struct {
		Name      string `json:"name"`
		Operation string `json:"operation"`
		Expected  string `json:"expected"`
	} `json:"transitions"`
	CrashVectors []struct {
		Name     string `json:"name"`
		Expected string `json:"expected"`
	} `json:"crash_vectors"`
	TargetVectors []struct {
		Name       string `json:"name"`
		Active     bool   `json:"active"`
		Registered int    `json:"registered"`
		Verified   int    `json:"verified"`
		Supported  int    `json:"supported"`
		Expected   string `json:"expected"`
	} `json:"target_vectors"`
}

type windowsE2EEFixture struct {
	repository *WindowsE2EEKeyStateRepository
	files      *testSecureFileOps
	identity   WindowsE2EEDeviceIdentityMetadata
	vectors    windowsE2EEVectors
}

func loadWindowsE2EEVectors(t testing.TB) windowsE2EEVectors {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "protocol", "e2ee-key-state-v1-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vectors windowsE2EEVectors
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	return vectors
}

func newWindowsE2EEFixture(t testing.TB, randomByte byte) windowsE2EEFixture {
	t.Helper()
	vectors := loadWindowsE2EEVectors(t)
	files := newTestSecureFileOps()
	repository, err := NewWindowsE2EEKeyStateRepository(WindowsE2EEKeyStateOptions{
		Directory: filepath.Join("C:", "Users", "test", fmt.Sprintf("Pulsar-%02x", randomByte)),
		Protector: &testProtector{}, Files: files,
		Random: bytes.NewReader(bytes.Repeat([]byte{randomByte}, 1<<20)),
	})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := repository.InstallDeviceIdentity(
		vectors.DeviceID, vectors.KeyFormat, bytes.Repeat([]byte{0x91}, 32),
		bytes.Repeat([]byte{0xA2}, 32), 1000,
	)
	if err != nil {
		t.Fatal(err)
	}
	return windowsE2EEFixture{repository: repository, files: files, identity: identity, vectors: vectors}
}

func installWindowsE2EEGroup(t testing.TB, fixture windowsE2EEFixture) WindowsE2EEGroupStateMetadata {
	t.Helper()
	metadata, err := fixture.repository.PersistGroupState(
		fixture.identity.InstallationID, fixture.vectors.GroupID, fixture.vectors.InitialEpoch,
		"", fixture.vectors.CommitDigest, fixture.vectors.TargetSnapshotDigest,
		bytes.Repeat([]byte{0xB3}, 128), 0, 1100,
	)
	if err != nil {
		t.Fatal(err)
	}
	return metadata
}

func TestWindowsE2EEDeviceIdentityUsesDistinctDPAPISlotsAndRedactedLeases(t *testing.T) {
	fixture := newWindowsE2EEFixture(t, 0x42)
	if fixture.vectors.Contract != "e2ee-key-state.v1" || fixture.vectors.Status != "production-disabled" {
		t.Fatalf("unexpected vectors: %+v", fixture.vectors)
	}
	if fixture.identity.InstallationID != fixture.vectors.InstallationRandomHex || fixture.identity.Revision != 1 {
		t.Fatalf("identity=%+v", fixture.identity)
	}
	lease, err := fixture.repository.LoadDeviceIdentity(fixture.vectors.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	var signing, agreement []byte
	if err := lease.WithSigningPrivateKey(func(value []byte) error { signing = append([]byte(nil), value...); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := lease.WithKeyAgreementPrivateKey(func(value []byte) error { agreement = append([]byte(nil), value...); return nil }); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(signing, bytes.Repeat([]byte{0x91}, 32)) || !bytes.Equal(agreement, bytes.Repeat([]byte{0xA2}, 32)) {
		t.Fatal("private key mismatch")
	}
	if strings.Contains(lease.String(), fixture.vectors.DeviceID) || !strings.Contains(lease.String(), "<redacted>") {
		t.Fatalf("unsafe description: %s", lease)
	}
	lease.Destroy()
	if err := lease.WithSigningPrivateKey(func([]byte) error { return nil }); !errors.Is(err, ErrWindowsE2EEUnavailable) {
		t.Fatalf("destroyed lease err=%v", err)
	}

	fixture.files.mu.Lock()
	defer fixture.files.mu.Unlock()
	var protectedPaths []string
	for path, value := range fixture.files.data {
		if strings.HasSuffix(path, ".dpapi") {
			protectedPaths = append(protectedPaths, path)
			if bytes.Contains(value, bytes.Repeat([]byte{0x91}, 8)) || bytes.Contains(value, bytes.Repeat([]byte{0xA2}, 8)) {
				t.Fatalf("plaintext secret in %s", path)
			}
		}
	}
	if len(protectedPaths) != 6 {
		t.Fatalf("protected path count=%d paths=%v", len(protectedPaths), protectedPaths)
	}
	for _, kind := range []string{"device_metadata", "device_signing", "device_agreement"} {
		var state, witness bool
		for _, path := range protectedPaths {
			state = state || strings.Contains(filepath.Base(path), "state-"+kind+"-")
			witness = witness || strings.Contains(filepath.Base(path), "witness-"+kind+"-")
		}
		if !state || !witness {
			t.Fatalf("missing state/witness for %s", kind)
		}
	}
}

func TestWindowsE2EEPartialDeviceInstallFailsClosed(t *testing.T) {
	vectors := loadWindowsE2EEVectors(t)
	files := newTestSecureFileOps()
	repository, err := NewWindowsE2EEKeyStateRepository(WindowsE2EEKeyStateOptions{
		Directory: filepath.Join("C:", "Users", "test", "partial"), Protector: &testProtector{},
		Files: files, Random: bytes.NewReader(bytes.Repeat([]byte{0x42}, 4096)),
	})
	if err != nil {
		t.Fatal(err)
	}
	files.failAt["move"] = 5 // metadata state+witness, signing state+witness, then agreement state
	_, err = repository.InstallDeviceIdentity(vectors.DeviceID, vectors.KeyFormat, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32), 1000)
	if !errors.Is(err, ErrWindowsE2EEUnavailable) {
		t.Fatalf("partial install err=%v", err)
	}
	files.failAt["move"] = 0
	_, err = repository.InstallDeviceIdentity(vectors.DeviceID, vectors.KeyFormat, bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32), 1000)
	if !errors.Is(err, ErrWindowsE2EERollbackOrClone) {
		t.Fatalf("retry err=%v", err)
	}
}

func TestWindowsE2EEEpochGenerationReplayForkAndDeleteVectors(t *testing.T) {
	fixture := newWindowsE2EEFixture(t, 0x42)
	initial := installWindowsE2EEGroup(t, fixture)
	reservation, err := fixture.repository.ReserveSendGeneration(fixture.identity.InstallationID, fixture.vectors.GroupID, "media", initial.Revision, 1200)
	if err != nil || reservation.Generation != 1 || reservation.Revision != 2 || reservation.Epoch != 7 {
		t.Fatalf("reservation=%+v err=%v", reservation, err)
	}
	if _, err := fixture.repository.ReserveSendGeneration(fixture.identity.InstallationID, fixture.vectors.GroupID, "media", initial.Revision, 1201); !errors.Is(err, ErrWindowsE2EEConflict) {
		t.Fatalf("stale reservation err=%v", err)
	}
	advanced, err := fixture.repository.PersistGroupState(
		fixture.identity.InstallationID, fixture.vectors.GroupID, fixture.vectors.NextEpoch,
		fixture.vectors.CommitDigest, fixture.vectors.NextCommitDigest,
		fixture.vectors.NextTargetSnapshotDigest, bytes.Repeat([]byte{0xC4}, 128), 2, 1300,
	)
	if err != nil || advanced.Epoch != 8 || advanced.SendGeneration != 0 || advanced.Revision != 3 {
		t.Fatalf("advanced=%+v err=%v", advanced, err)
	}
	_, err = fixture.repository.PersistGroupState(fixture.identity.InstallationID, fixture.vectors.GroupID, 8, fixture.vectors.NextCommitDigest, fixture.vectors.NextCommitDigest, fixture.vectors.NextTargetSnapshotDigest, []byte{1}, 3, 1400)
	if !errors.Is(err, ErrWindowsE2EEStaleEpoch) {
		t.Fatalf("stale epoch err=%v", err)
	}
	_, err = fixture.repository.PersistGroupState(fixture.identity.InstallationID, fixture.vectors.GroupID, 10, fixture.vectors.NextCommitDigest, strings.Repeat("e", 64), fixture.vectors.NextTargetSnapshotDigest, []byte{1}, 3, 1400)
	if !errors.Is(err, ErrWindowsE2EERollbackOrClone) {
		t.Fatalf("epoch gap err=%v", err)
	}
	_, err = fixture.repository.PersistGroupState(fixture.identity.InstallationID, fixture.vectors.GroupID, 9, fixture.vectors.CommitDigest, strings.Repeat("e", 64), fixture.vectors.NextTargetSnapshotDigest, []byte{1}, 3, 1400)
	if !errors.Is(err, ErrWindowsE2EERollbackOrClone) {
		t.Fatalf("wrong predecessor err=%v", err)
	}
	if len(fixture.vectors.Transitions) != 5 {
		t.Fatalf("transition count=%d", len(fixture.vectors.Transitions))
	}
	if err := fixture.repository.DeleteGroupState(fixture.identity.InstallationID, fixture.vectors.GroupID); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.LoadGroupState(fixture.identity.InstallationID, fixture.vectors.GroupID); !errors.Is(err, ErrWindowsE2EENotFound) {
		t.Fatalf("deleted group err=%v", err)
	}
}

func TestWindowsE2EECrashAfterRecordBeforeWitnessFailsClosed(t *testing.T) {
	fixture := newWindowsE2EEFixture(t, 0x42)
	initial := installWindowsE2EEGroup(t, fixture)
	fixture.files.mu.Lock()
	fixture.files.failAt["move"] = fixture.files.counts["move"] + 2
	fixture.files.mu.Unlock()
	_, err := fixture.repository.ReserveSendGeneration(fixture.identity.InstallationID, fixture.vectors.GroupID, "live", initial.Revision, 1200)
	if !errors.Is(err, ErrWindowsE2EEUnavailable) {
		t.Fatalf("reserve err=%v", err)
	}
	fixture.files.mu.Lock()
	fixture.files.failAt["move"] = 0
	fixture.files.mu.Unlock()
	if _, err := fixture.repository.LoadGroupState(fixture.identity.InstallationID, fixture.vectors.GroupID); !errors.Is(err, ErrWindowsE2EERollbackOrClone) {
		t.Fatalf("load torn state err=%v", err)
	}
	if fixture.vectors.CrashVectors[0].Expected != "rollback_or_clone" {
		t.Fatalf("crash vector=%+v", fixture.vectors.CrashVectors[0])
	}
}

func TestWindowsE2EELostReadbackAfterBothWritesConsumesGeneration(t *testing.T) {
	fixture := newWindowsE2EEFixture(t, 0x42)
	initial := installWindowsE2EEGroup(t, fixture)
	armed := true
	fixture.files.afterMove = func(files *testSecureFileOps, destination string) {
		if armed && strings.Contains(filepath.Base(destination), "witness-group-") {
			armed = false
			files.failAt["open"] = files.counts["open"] + 1
		}
	}
	_, err := fixture.repository.ReserveSendGeneration(fixture.identity.InstallationID, fixture.vectors.GroupID, "live", initial.Revision, 1200)
	if !errors.Is(err, ErrWindowsE2EEUnavailable) {
		t.Fatalf("lost readback err=%v", err)
	}
	fixture.files.mu.Lock()
	fixture.files.failAt["open"] = 0
	fixture.files.afterMove = nil
	fixture.files.mu.Unlock()
	recovered, err := fixture.repository.LoadGroupState(fixture.identity.InstallationID, fixture.vectors.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Metadata.SendGeneration != 1 || recovered.Metadata.Revision != 2 {
		t.Fatalf("recovered=%+v", recovered.Metadata)
	}
	recovered.Destroy()
	if _, err := fixture.repository.ReserveSendGeneration(fixture.identity.InstallationID, fixture.vectors.GroupID, "live", initial.Revision, 1201); !errors.Is(err, ErrWindowsE2EEConflict) {
		t.Fatalf("old revision err=%v", err)
	}
	if fixture.vectors.CrashVectors[1].Expected != "generation-consumed-no-reuse" {
		t.Fatalf("crash vector=%+v", fixture.vectors.CrashVectors[1])
	}
}

func TestWindowsE2EECopiedGroupStateCannotCrossInstallation(t *testing.T) {
	source := newWindowsE2EEFixture(t, 0x42)
	installWindowsE2EEGroup(t, source)
	source.files.mu.Lock()
	copied := map[string][]byte{}
	for path, value := range source.files.data {
		base := filepath.Base(path)
		if strings.Contains(base, "state-group-") || strings.Contains(base, "witness-group-") {
			copied[base] = append([]byte(nil), value...)
		}
	}
	source.files.mu.Unlock()
	destination := newWindowsE2EEFixture(t, 0x43)
	destination.files.mu.Lock()
	for name, value := range copied {
		destination.files.data[filepath.Join(destination.repository.dir, name)] = value
	}
	destination.files.mu.Unlock()
	if source.identity.InstallationID == destination.identity.InstallationID {
		t.Fatal("installation ids match")
	}
	if _, err := destination.repository.LoadGroupState(destination.identity.InstallationID, source.vectors.GroupID); !errors.Is(err, ErrWindowsE2EERollbackOrClone) {
		t.Fatalf("copied group err=%v", err)
	}
}

func TestWindowsE2EEGrantsAreMonotonicExpiringAndRevocable(t *testing.T) {
	fixture := newWindowsE2EEFixture(t, 0x42)
	grant, err := fixture.repository.StoreGrant(fixture.identity.InstallationID, "grant-0001", fixture.vectors.GroupID, 7, 8, 5000, bytes.Repeat([]byte{0xD5}, 64), 0, 1500)
	if err != nil || grant.Revision != 1 {
		t.Fatalf("grant=%+v err=%v", grant, err)
	}
	loaded, lease, err := fixture.repository.LoadGrant(fixture.identity.InstallationID, grant.GrantID, 2000)
	if err != nil || loaded != grant {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
	if err := lease.WithBytes(func(value []byte) error {
		if !bytes.Equal(value, bytes.Repeat([]byte{0xD5}, 64)) {
			return errors.New("mismatch")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	lease.Destroy()
	_, err = fixture.repository.StoreGrant(fixture.identity.InstallationID, grant.GrantID, fixture.vectors.GroupID, 7, 7, 5000, []byte{1}, 1, 1600)
	if !errors.Is(err, ErrWindowsE2EEReplay) {
		t.Fatalf("grant replay err=%v", err)
	}
	if _, _, err := fixture.repository.LoadGrant(fixture.identity.InstallationID, grant.GrantID, 5000); !errors.Is(err, ErrWindowsE2EEExpired) {
		t.Fatalf("expired err=%v", err)
	}
	if err := fixture.repository.RevokeGrant(fixture.identity.InstallationID, grant.GrantID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fixture.repository.LoadGrant(fixture.identity.InstallationID, grant.GrantID, 2000); !errors.Is(err, ErrWindowsE2EENotFound) {
		t.Fatalf("revoked err=%v", err)
	}
}

func TestWindowsE2EEContentCacheIsBoundedExpiringAndClearable(t *testing.T) {
	fixture := newWindowsE2EEFixture(t, 0x42)
	var revision uint64
	for index := 0; index < 35; index++ {
		var err error
		revision, err = fixture.repository.CacheContentKey(
			fixture.identity.InstallationID, fmt.Sprintf("object-%03d", index), fixture.vectors.GroupID,
			7, int64(10_000+index), bytes.Repeat([]byte{byte(index)}, 32), revision, int64(2000+index),
		)
		if err != nil {
			t.Fatalf("cache %d: %v", index, err)
		}
	}
	if revision != 35 {
		t.Fatalf("revision=%d", revision)
	}
	if _, _, err := fixture.repository.LoadContentKey(fixture.identity.InstallationID, "object-000", 3000); !errors.Is(err, ErrWindowsE2EENotFound) {
		t.Fatalf("evicted err=%v", err)
	}
	metadata, lease, err := fixture.repository.LoadContentKey(fixture.identity.InstallationID, "object-034", 3000)
	if err != nil || metadata.Epoch != 7 {
		t.Fatalf("metadata=%+v err=%v", metadata, err)
	}
	lease.Destroy()
	if _, _, err := fixture.repository.LoadContentKey(fixture.identity.InstallationID, "object-034", 10_034); !errors.Is(err, ErrWindowsE2EEExpired) {
		t.Fatalf("expired err=%v", err)
	}
	if err := fixture.repository.ClearContentKeyCache(fixture.identity.InstallationID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := fixture.repository.LoadContentKey(fixture.identity.InstallationID, "object-034", 3000); !errors.Is(err, ErrWindowsE2EENotFound) {
		t.Fatalf("cleared err=%v", err)
	}
}

func TestWindowsE2EETargetVectorsAndCrossProcessLock(t *testing.T) {
	fixture := newWindowsE2EEFixture(t, 0x42)
	for _, vector := range fixture.vectors.TargetVectors {
		actual := DecideWindowsE2EETargetDevice(vector.Active, vector.Registered, vector.Verified, vector.Supported)
		if string(actual) != vector.Expected {
			t.Fatalf("target %s=%s want %s", vector.Name, actual, vector.Expected)
		}
	}
	lockPath := filepath.Join(fixture.repository.dir, "repository.lock")
	fixture.files.mu.Lock()
	fixture.files.locks[lockPath] = true
	fixture.files.mu.Unlock()
	if _, err := fixture.repository.LoadDeviceIdentity(fixture.vectors.DeviceID); !errors.Is(err, ErrWindowsE2EEBusy) {
		t.Fatalf("busy lock err=%v", err)
	}
	fixture.files.mu.Lock()
	delete(fixture.files.locks, lockPath)
	fixture.files.mu.Unlock()
}

func TestWindowsE2EESourceExcludesConfigLogsAndRuntimeActivation(t *testing.T) {
	source, err := os.ReadFile("windows_e2ee_key_state.go")
	if err != nil {
		t.Fatal(err)
	}
	windowsDefault, err := os.ReadFile("windows_e2ee_key_state_default_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(string(windowsDefault), "dpapiDataProtector{api: windowsDataProtectionAPI{}}") ||
		!strings.Contains(text, "AcquireLock(lockPath)") || !strings.Contains(text, "moveReplaceExisting|moveWriteThrough") {
		t.Fatal("DPAPI or durable lock boundary missing")
	}
	for _, forbidden := range []string{"config.json", "log.Printf", "slog.", "fmt.Printf", "e2ee_media_v1"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("forbidden source token %q", forbidden)
		}
	}
	mainSource, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mainSource), "newDefaultWindowsE2EEKeyStateRepository") || strings.Contains(string(mainSource), "WindowsE2EEKeyStateRepository") {
		t.Fatal("production-dark repository wired into main")
	}
}
