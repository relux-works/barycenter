package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestAuthMigrationCredentialPlaintextAbsentFromSQLiteArtifacts complements
// column-level hash assertions with a physical artifact check. It drives every
// onboarding credential lifecycle through the production Store API, then
// scans the SQLite database and any live WAL/SHM/journal sidecars for the exact
// plaintext values returned to callers.
func TestAuthMigrationCredentialPlaintextAbsentFromSQLiteArtifacts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credential-artifacts.db")
	s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	created, err := s.CreateSelfServiceOrbit("Artifact acceptance")
	if err != nil {
		t.Fatal(err)
	}
	invite, err := s.IssueDeviceInvite(created.ActorID, created.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	joined, err := s.ConsumeDeviceInvite(invite.Code)
	if err != nil {
		t.Fatal(err)
	}
	link, err := s.IssueTelegramLink(created.ActorID, created.ControlToken, "satellite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeTelegramLink(99001, "Artifact Link", "private", link.Code); err != nil {
		t.Fatal(err)
	}
	rotated, err := s.RotateRecovery(created.ActorID, created.ControlToken)
	if err != nil {
		t.Fatal(err)
	}
	replacementControl := runtimeTestToken(t)
	if _, err := s.ConsumeRecovery(rotated.RecoveryID, rotated.RecoverySecret, replacementControl); err != nil {
		t.Fatal(err)
	}

	plaintext := map[string]string{
		"created node token":        created.NodeToken,
		"created control token":     created.ControlToken,
		"created recovery secret":   created.RecoverySecret,
		"device invite code":        invite.Code,
		"joined node token":         joined.NodeToken,
		"joined control token":      joined.ControlToken,
		"telegram link code":        link.Code,
		"rotated recovery secret":   rotated.RecoverySecret,
		"replacement control token": replacementControl,
	}

	// Flush committed WAL frames without discarding the sidecar before the
	// first scan. The second scan after Close covers the final durable shape.
	if _, err := s.db.Exec(`PRAGMA wal_checkpoint(FULL)`); err != nil {
		t.Fatal(err)
	}
	assertCredentialPlaintextAbsentFromSQLiteArtifacts(t, path, plaintext)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	assertCredentialPlaintextAbsentFromSQLiteArtifacts(t, path, plaintext)
}

func assertCredentialPlaintextAbsentFromSQLiteArtifacts(t *testing.T, path string, plaintext map[string]string) {
	t.Helper()
	seen := 0
	for _, artifact := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		raw, err := os.ReadFile(artifact)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		seen++
		for name, secret := range plaintext {
			if secret == "" {
				t.Fatalf("%s was unexpectedly empty", name)
			}
			if bytes.Contains(raw, []byte(secret)) {
				t.Fatalf("%s appears in SQLite artifact %s", name, filepath.Base(artifact))
			}
		}
	}
	if seen == 0 {
		t.Fatal("no SQLite artifacts were available for inspection")
	}
}
