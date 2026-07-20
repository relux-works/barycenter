package main

// Reviewer repro for TASK-260712-28zhpl review. Not part of the reviewed
// commit; deleted after the review run.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Simulates the post-crash state where the app-owned plaintext draft was
// already removed (crash between os.Remove(plaintext) and os.RemoveAll(dir),
// or RemoveAll failure after Remove succeeded). Expected per AC: terminal
// cleanup converges. Observed: cleanup fails forever and wedges recovery for
// all later-sorted drafts.
func TestReviewReproStuckCleanupWhenOwnedPlaintextAlreadyGone(t *testing.T) {
	fixture := newWindowsProtectedSendFixture(t)
	sourceB := filepath.Join(fixture.plaintextRoot, "second.wav")
	if err := os.WriteFile(sourceB, []byte("second private fixture bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	index := 1
	fixture.uploader.failChunkOnce = &index
	service := fixture.service(t, true)

	requestA := fixture.request()
	requestA.DraftID = "draft_aK123456789ABCDEFGHJKMNPQ"
	if _, err := service.Send(context.Background(), requestA, 2000, nil); !errors.Is(err, ErrWindowsProtectedMediaTransport) {
		t.Fatalf("draft A interrupt err=%v", err)
	}

	current, err := fixture.keyState.LoadGroupState(fixture.identity.InstallationID, fixture.group.GroupID)
	if err != nil {
		t.Fatal(err)
	}
	currentRevision := current.Metadata.Revision
	current.Destroy()

	requestB := fixture.request()
	requestB.DraftID = "draft_bK123456789ABCDEFGHJKMNPQ"
	requestB.SourcePath = sourceB
	requestB.SourceObjectID = "source_2K123456789ABCDEFGHJKMNP"
	requestB.ExpectedGroupRevision = currentRevision
	// Fixture uploader admits one stage shape; draft B fails at Stage after
	// its prepared state (state.json + chunks) is durably persisted.
	if _, err := service.Send(context.Background(), requestB, 2000, nil); !errors.Is(err, ErrWindowsProtectedMediaTransport) {
		t.Fatalf("draft B interrupt err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.ciphertextRoot, requestB.DraftID, "state.json")); err != nil {
		t.Fatalf("draft B state missing: %v", err)
	}

	// The crash-window state: owned plaintext for draft A is already gone.
	if err := os.Remove(fixture.source); err != nil {
		t.Fatal(err)
	}

	// Explicit cancel can never converge.
	if err := service.Cancel(context.Background(), requestA.DraftID); !errors.Is(err, ErrWindowsProtectedMediaLocalCleanup) {
		t.Fatalf("cancel err=%v (want LocalCleanup)", err)
	}
	if _, err := os.Stat(filepath.Join(fixture.ciphertextRoot, requestA.DraftID)); err != nil {
		t.Fatalf("draft A directory unexpectedly gone: %v", err)
	}

	// Expiry recovery hits draft A first (sorted), fails, and never reaches
	// healthy expired draft B.
	removed, err := service.RecoverExpiredDrafts(context.Background(), 10_001, 10)
	if !errors.Is(err, ErrWindowsProtectedMediaLocalCleanup) || removed != 0 {
		t.Fatalf("recovery removed=%d err=%v", removed, err)
	}
	if _, statErr := os.Stat(filepath.Join(fixture.ciphertextRoot, requestB.DraftID)); statErr != nil {
		t.Fatalf("draft B directory state: %v", statErr)
	}
	t.Logf("confirmed: cancel and recovery both return LocalCleanup forever; healthy expired draft B never recovered")
}

// Simulates a crash during persistPrepared after os.Mkdir but before
// state.json exists. The orphan directory blocks the draft ID forever and is
// never cleaned by recovery (which skips directories without state.json).
func TestReviewReproOrphanDraftDirectoryBlocksDraftIDAndIsNeverRecovered(t *testing.T) {
	fixture := newWindowsProtectedSendFixture(t)
	service := fixture.service(t, true)
	request := fixture.request()
	orphan := filepath.Join(fixture.ciphertextRoot, request.DraftID)
	if err := os.MkdirAll(orphan, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(orphan, "chunk-0000.bin"), []byte{1, 2, 3}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Send(context.Background(), request, 2000, nil); !errors.Is(err, ErrWindowsProtectedMediaPersistence) {
		t.Fatalf("send err=%v", err)
	}
	removed, err := service.RecoverExpiredDrafts(context.Background(), 10_001, 10)
	if err != nil || removed != 0 {
		t.Fatalf("recovery removed=%d err=%v", removed, err)
	}
	if _, statErr := os.Stat(orphan); statErr != nil {
		t.Fatalf("orphan state: %v", statErr)
	}
	t.Logf("confirmed: orphan ciphertext directory persists and permanently blocks the draft ID")
}
