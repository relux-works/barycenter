//go:build previoushead

package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/session"
)

const transmissionStorePreviousRevision = "2aa97c2d08cb93b110200ae159fd43265410ff5a"

type exactTransmissionPreviousResult struct {
	LegacyStatus      string `json:"legacy_status"`
	SessionPositionMS int64  `json:"session_position_ms"`
	InsertedLegacyID  string `json:"inserted_legacy_id"`
	DissolvedOrbitID  int64  `json:"dissolved_orbit_id"`
}

func TestTransmissionStoreExactPreviousHeadRollback(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate current transmission rollback test")
	}
	storeDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(storeDir, "..", "..", ".."))
	assertTransmissionStorePreviousRevision(t, repoRoot)

	path := filepath.Join(t.TempDir(), "transmission-previous-head.db")
	current, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	source, err := current.CreateSelfServiceOrbit("Transmission rollback source")
	if err != nil {
		t.Fatal(err)
	}
	target, err := current.CreateSelfServiceOrbit("Transmission rollback target")
	if err != nil {
		t.Fatal(err)
	}
	dissolve, err := current.CreateSelfServiceOrbit("Transmission rollback dissolve")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	legacy := MediaRecord{
		ID: "m_before_transmission_store", TGFileID: "tg-before-transmission",
		DurationMS: 1000, PathWAV: "/srv/before-transmission.wav",
		LoudnormJSON: "{}", CreatedAt: now,
		ExpiresAt: now + int64((30*24*time.Hour)/time.Millisecond),
		Status:    "ready", OrbitID: source.OrbitID,
	}
	if err := current.InsertMedia(legacy); err != nil {
		t.Fatal(err)
	}
	if err := current.SaveSession(source.OrbitID, SessionSnapshot{
		Mode: session.ModeSolo, State: session.StatePlaying, SavedPositionMS: 101,
	}); err != nil {
		t.Fatal(err)
	}
	media := readyLifecycleMedia(
		t, current, source, now+1,
		now+1+int64((7*24*time.Hour)/time.Millisecond),
	)
	created, err := current.CreateTransmission(transmissionParams(
		media, source, now+4, transmissionTarget(target, true),
	))
	if err != nil {
		t.Fatal(err)
	}
	wantTransmission := created.Transmission
	wantTargets := append([]TransmissionTarget(nil), created.Targets...)
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}

	previousDir := prepareTransmissionStorePreviousTree(t, repoRoot, storeDir)
	resultPath := filepath.Join(t.TempDir(), "transmission-previous-result.json")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(
		ctx, "go", "test", "-count=1", "./internal/store",
		"-run", "^TestMediaPreviousHeadAuthority$",
	)
	command.Dir = previousDir
	command.Env = append(os.Environ(),
		"BARYCENTER_MEDIA_PREVIOUS_DB="+path,
		"BARYCENTER_MEDIA_PREVIOUS_RESULT="+resultPath,
		"BARYCENTER_MEDIA_KEEP_ORBIT="+strconv.FormatInt(source.OrbitID, 10),
		"BARYCENTER_MEDIA_DISSOLVE_ORBIT="+strconv.FormatInt(dissolve.OrbitID, 10),
		"BARYCENTER_MEDIA_NODE_TOKEN="+source.NodeToken,
		"BARYCENTER_MEDIA_LEGACY_ID="+legacy.ID,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("exact transmission predecessor Store API test: %v\n%s", err, output)
	}
	encoded, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result exactTransmissionPreviousResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	if result.LegacyStatus != "processing" || result.SessionPositionMS != 909 ||
		result.InsertedLegacyID == "" || result.DissolvedOrbitID != dissolve.OrbitID {
		t.Fatalf("incomplete predecessor result=%+v", result)
	}

	current, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	gotTransmission, err := current.GetTransmission(wantTransmission.ID)
	if err != nil || gotTransmission == nil || !reflect.DeepEqual(*gotTransmission, wantTransmission) {
		t.Fatalf("transmission after rollback=%+v want=%+v err=%v",
			gotTransmission, wantTransmission, err)
	}
	gotTargets, err := current.TransmissionTargets(wantTransmission.ID)
	if err != nil || !reflect.DeepEqual(gotTargets, wantTargets) {
		t.Fatalf("targets after rollback=%+v want=%+v err=%v", gotTargets, wantTargets, err)
	}
	allowed, err := current.AllowsMediaDownload(context.Background(), MediaTargetIdentity{
		MediaID: media.ID, OrbitID: target.OrbitID,
		ActorID: target.ActorID, Slot: target.Slot,
	})
	if err != nil || !allowed {
		t.Fatalf("target ACL after rollback allowed=%v err=%v", allowed, err)
	}
	legacyAfter, err := current.GetMedia(legacy.ID)
	if err != nil || legacyAfter == nil || legacyAfter.Status != "processing" ||
		legacyAfter.DurationMS != 1777 {
		t.Fatalf("legacy media after rollback=%+v err=%v", legacyAfter, err)
	}
	snapshot, err := current.LoadSession(source.OrbitID)
	if err != nil || snapshot == nil || snapshot.SavedPositionMS != 909 {
		t.Fatalf("session after rollback=%+v err=%v", snapshot, err)
	}
	if err := foreignKeyCheck(current.db); err != nil {
		t.Fatal(err)
	}
}

func assertTransmissionStorePreviousRevision(t *testing.T, repoRoot string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(
		ctx, "git", "rev-parse", transmissionStorePreviousRevision+"^{commit}",
	)
	command.Dir = repoRoot
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve transmission predecessor revision: %v: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != transmissionStorePreviousRevision {
		t.Fatalf("resolved transmission predecessor=%q, want %q",
			strings.TrimSpace(string(output)), transmissionStorePreviousRevision)
	}
}

func prepareTransmissionStorePreviousTree(t *testing.T, repoRoot, storeDir string) string {
	t.Helper()
	extractRoot := filepath.Join(t.TempDir(), "transmission-previous-head")
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	archive := exec.CommandContext(
		ctx, "git", "archive", "--format=tar.gz",
		transmissionStorePreviousRevision, "coordinator",
	)
	archive.Dir = repoRoot
	compressed, err := archive.Output()
	if err != nil {
		t.Fatalf("archive transmission predecessor: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := extractTar(tar.NewReader(reader), extractRoot); err != nil {
		t.Fatal(err)
	}
	driver, err := os.ReadFile(filepath.Join(
		storeDir, "testdata", "media_previous_head_authority_test.go",
	))
	if err != nil {
		t.Fatal(err)
	}
	previousStoreDir := filepath.Join(extractRoot, "coordinator", "internal", "store")
	if err := os.WriteFile(
		filepath.Join(previousStoreDir, "media_previous_head_authority_test.go"),
		driver, 0o600,
	); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(extractRoot, "coordinator")
}
