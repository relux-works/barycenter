// Command phase1-acceptance-fixture creates a fresh, nonproduction two-Pulsar
// coordinator database. Plaintext onboarding material is written only to a
// new mode-0600 private file and is never printed.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"relux.works/duet/coordinator/internal/store"
)

type credentialFile struct {
	SchemaVersion int                         `json:"schemaVersion"`
	FixtureID     string                      `json:"fixtureId"`
	PulsarA       store.OnboardingCredentials `json:"pulsarA"`
	PulsarB       store.OnboardingCredentials `json:"pulsarB"`
}

type publicResult struct {
	SchemaVersion int     `json:"schemaVersion"`
	FixtureID     string  `json:"fixtureId"`
	OrbitID       int64   `json:"orbitId"`
	ActorIDs      []int64 `json:"actorIds"`
	Secrets       string  `json:"secrets"`
}

func main() {
	var dbPath, credentialPath string
	var confirmed bool
	flag.StringVar(&dbPath, "db", "", "new nonproduction SQLite path under .temp/acceptance")
	flag.StringVar(&credentialPath, "credentials", "", "new private credential JSON path under .temp/acceptance")
	flag.BoolVar(&confirmed, "confirm-nonproduction", false, "confirm no production data is in scope")
	flag.Parse()
	result, err := prepareFixture(dbPath, credentialPath, confirmed)
	if err != nil {
		fmt.Fprintln(os.Stderr, "phase1 acceptance fixture:", err)
		os.Exit(1)
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, "phase1 acceptance fixture: encode public result:", err)
		os.Exit(1)
	}
}

func prepareFixture(dbPath, credentialPath string, confirmed bool) (publicResult, error) {
	if !confirmed {
		return publicResult{}, errors.New("-confirm-nonproduction is required")
	}
	dbPath, err := validatedNewAcceptancePath(dbPath)
	if err != nil {
		return publicResult{}, fmt.Errorf("database path: %w", err)
	}
	credentialPath, err = validatedNewAcceptancePath(credentialPath)
	if err != nil {
		return publicResult{}, fmt.Errorf("credential path: %w", err)
	}
	if dbPath == credentialPath {
		return publicResult{}, errors.New("database and credential paths must differ")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return publicResult{}, fmt.Errorf("create private directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(dbPath), 0o700); err != nil {
		return publicResult{}, fmt.Errorf("secure private directory: %w", err)
	}
	if filepath.Dir(credentialPath) != filepath.Dir(dbPath) {
		if err := os.MkdirAll(filepath.Dir(credentialPath), 0o700); err != nil {
			return publicResult{}, fmt.Errorf("create credential directory: %w", err)
		}
		if err := os.Chmod(filepath.Dir(credentialPath), 0o700); err != nil {
			return publicResult{}, fmt.Errorf("secure credential directory: %w", err)
		}
	}

	st, err := store.OpenWithOptions(dbPath, store.Options{SelfServiceOnboarding: true})
	if err != nil {
		removeSQLiteFamily(dbPath)
		return publicResult{}, fmt.Errorf("create store: %w", err)
	}
	failed := true
	defer func() {
		st.Close()
		if failed {
			removeSQLiteFamily(dbPath)
			os.Remove(credentialPath)
		}
	}()

	owner, err := st.CreateSelfServiceOrbit("Acceptance Orbit")
	if err != nil {
		return publicResult{}, fmt.Errorf("create owner: %w", err)
	}
	invite, err := st.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		return publicResult{}, fmt.Errorf("issue companion invite: %w", err)
	}
	companion, err := st.ConsumeDeviceInvite(invite.Code)
	if err != nil {
		return publicResult{}, fmt.Errorf("consume companion invite: %w", err)
	}
	if owner.OrbitID != companion.OrbitID || owner.ActorID == companion.ActorID {
		return publicResult{}, errors.New("generated topology does not contain two distinct actors in one orbit")
	}
	if owner.OrbitID != 1 || owner.ActorID != 1 || companion.ActorID != 2 {
		return publicResult{}, fmt.Errorf(
			"fresh topology IDs changed: orbit=%d actors=%d,%d", owner.OrbitID, owner.ActorID, companion.ActorID,
		)
	}

	file, err := os.OpenFile(credentialPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return publicResult{}, fmt.Errorf("create credential file: %w", err)
	}
	encodeErr := json.NewEncoder(file).Encode(credentialFile{
		SchemaVersion: 1, FixtureID: "phase1-two-pulsar-v1", PulsarA: owner, PulsarB: companion,
	})
	closeErr := file.Close()
	if encodeErr != nil {
		return publicResult{}, fmt.Errorf("write credential file: %w", encodeErr)
	}
	if closeErr != nil {
		return publicResult{}, fmt.Errorf("close credential file: %w", closeErr)
	}
	failed = false
	return publicResult{
		SchemaVersion: 1,
		FixtureID:     "phase1-two-pulsar-v1",
		OrbitID:       owner.OrbitID,
		ActorIDs:      []int64{owner.ActorID, companion.ActorID},
		Secrets:       "written-to-private-file-not-printed",
	}, nil
}

func validatedNewAcceptancePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	marker := string(filepath.Separator) + ".temp" + string(filepath.Separator) + "acceptance" + string(filepath.Separator)
	markerIndex := strings.Index(abs, marker)
	if markerIndex < 0 {
		return "", errors.New("path must be below .temp/acceptance")
	}
	acceptanceRoot := abs[:markerIndex+len(marker)-1]
	stop := filepath.Dir(filepath.Dir(acceptanceRoot))
	for current := filepath.Dir(abs); current != stop; current = filepath.Dir(current) {
		info, statErr := os.Lstat(current)
		if statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("symlinks below .temp are not allowed")
		}
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	if _, err := os.Lstat(abs); err == nil {
		return "", errors.New("path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return abs, nil
}

func removeSQLiteFamily(path string) {
	for _, suffix := range []string{"", "-shm", "-wal"} {
		_ = os.Remove(path + suffix)
	}
}
