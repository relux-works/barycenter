package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func acceptanceTempPath(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, ".temp", "acceptance", "run", "private")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPrepareFixtureCreatesFreshTwoPulsarTopologyWithoutPrintingSecrets(t *testing.T) {
	private := acceptanceTempPath(t)
	dbPath := filepath.Join(private, "coordinator.db")
	credentialsPath := filepath.Join(private, "credentials.json")
	result, err := prepareFixture(dbPath, credentialsPath, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.OrbitID <= 0 || len(result.ActorIDs) != 2 || result.ActorIDs[0] == result.ActorIDs[1] {
		t.Fatalf("invalid public result: %+v", result)
	}
	public, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(credentialsPath)
	if err != nil {
		t.Fatal(err)
	}
	var credentials credentialFile
	if err := json.Unmarshal(raw, &credentials); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		credentials.PulsarA.NodeToken, credentials.PulsarA.ControlToken,
		credentials.PulsarA.RecoverySecret, credentials.PulsarB.NodeToken,
		credentials.PulsarB.ControlToken,
	} {
		if secret == "" || strings.Contains(string(public), secret) {
			t.Fatal("secret missing from private file or exposed in public output")
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(credentialsPath)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("credential mode=%o, want 600", info.Mode().Perm())
		}
	}
}

func TestPrepareFixtureRejectsProductionExistingAndUnconfirmedPaths(t *testing.T) {
	if _, err := prepareFixture("/tmp/production.db", "/tmp/credentials.json", true); err == nil {
		t.Fatal("production-like paths accepted")
	}
	private := acceptanceTempPath(t)
	dbPath := filepath.Join(private, "coordinator.db")
	credentialsPath := filepath.Join(private, "credentials.json")
	if _, err := prepareFixture(dbPath, credentialsPath, false); err == nil {
		t.Fatal("unconfirmed fixture accepted")
	}
	if err := os.WriteFile(dbPath, []byte("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareFixture(dbPath, credentialsPath, true); err == nil {
		t.Fatal("existing database accepted")
	}
}

func TestPrepareFixtureRejectsSymlinkedAcceptanceDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires optional Windows privilege")
	}
	root := t.TempDir()
	realDirectory := filepath.Join(root, "real")
	if err := os.MkdirAll(realDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	tempRoot := filepath.Join(root, ".temp")
	if err := os.MkdirAll(tempRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDirectory, filepath.Join(tempRoot, "acceptance")); err != nil {
		t.Fatal(err)
	}
	private := filepath.Join(tempRoot, "acceptance", "run", "private")
	if _, err := prepareFixture(
		filepath.Join(private, "coordinator.db"),
		filepath.Join(private, "credentials.json"),
		true,
	); err == nil {
		t.Fatal("symlinked acceptance directory accepted")
	}
}
