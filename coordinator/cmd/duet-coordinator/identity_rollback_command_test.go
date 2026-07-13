package main

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/store"
)

func TestIdentityRollbackProjectionCommandRejectsIncompatibleYAMLBeforeOpeningDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "must-not-open.db")
	configPath := filepath.Join(dir, "coordinator.yml")
	configYAML := "listen: \"127.0.0.1:18080\"\n" +
		"db_path: " + dbPath + "\n" +
		"media_dir: " + filepath.Join(dir, "media") + "\n" +
		"self_service_onboarding: true\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	err := projectIdentityForLegacyRollback(configPath)
	if err == nil || !strings.Contains(err.Error(), "predecessor-neutral") {
		t.Fatalf("incompatible rollback config error=%v", err)
	}
	if _, statErr := os.Stat(dbPath); !os.IsNotExist(statErr) {
		t.Fatalf("rollback config guard opened database: %v", statErr)
	}
}

func TestIdentityRollbackProjectionCommandUsesFeatureOffStoreAndLegacyBarriers(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "rollback-command.db")
	current, err := store.OpenWithOptions(dbPath, store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	created, err := current.CreateSelfServiceOrbit("Rollback command")
	if err != nil {
		t.Fatal(err)
	}
	if err := current.AddMember(created.OrbitID, 88002, "companion"); err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}

	inspect, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inspect.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, created.OrbitID); err != nil {
		inspect.Close()
		t.Fatal(err)
	}
	if err := inspect.Close(); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(dir, "coordinator.yml")
	configYAML := "listen: \"127.0.0.1:18080\"\n" +
		"db_path: " + dbPath + "\n" +
		"media_dir: " + filepath.Join(dir, "media") + "\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	// A live deployment enables the current feature through this variable.
	// The one-shot command must nevertheless avoid feature-on reconciliation.
	t.Setenv("DUET_SELF_SERVICE_ONBOARDING", "1")
	t.Setenv("DUET_DB_PATH", "")
	if err := projectIdentityForLegacyRollback(configPath); err != nil {
		t.Fatal(err)
	}
	if err := projectIdentityForLegacyRollback(configPath); err != nil {
		t.Fatalf("projection command is not idempotent: %v", err)
	}

	legacy, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if _, _, ok, err := legacy.LookupToken(created.NodeToken); err != nil || ok {
		t.Fatalf("projected node token lookup ok=%v err=%v", ok, err)
	}
	if _, _, err := legacy.PairSlot(created.OrbitID, 88001); !errors.Is(err, store.ErrLimit) {
		t.Fatalf("legacy PairSlot barrier error=%v", err)
	}
	if err := legacy.AddMember(created.OrbitID, 88003, "companion"); !errors.Is(err, store.ErrLimit) {
		t.Fatalf("legacy AddMember barrier error=%v", err)
	}
}
