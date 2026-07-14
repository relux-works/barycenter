package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/store"
)

func captureModerationCommandOutput(t *testing.T, run func() error) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	runErr := run()
	_ = writer.Close()
	os.Stdout = previous
	output, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if runErr != nil {
		t.Fatal(runErr)
	}
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(output)
}

func moderationCommandField(t *testing.T, output, name string) string {
	t.Helper()
	prefix := name + "="
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	t.Fatalf("missing %s in command output", name)
	return ""
}

func TestModerationOperatorCommandProvisionsScopedTokenAndRevokesIt(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "moderation-command.db")
	configPath := filepath.Join(dir, "coordinator.yml")
	configYAML := "listen: \"127.0.0.1:18080\"\n" +
		"db_path: " + dbPath + "\n" +
		"media_dir: " + filepath.Join(dir, "media") + "\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	output := captureModerationCommandOutput(t, func() error {
		return runModerationOperatorCommand(
			configPath, "Ivan Oparin", "", "list,evidence",
		)
	})
	operatorID := moderationCommandField(t, output, "operator_id")
	token := moderationCommandField(t, output, "operator_token")
	if !strings.HasPrefix(token, "mod_") || !strings.Contains(output, "shown_once=true") {
		t.Fatalf("unexpected provision output shape")
	}
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	operator, err := st.ResolveModerationOperator(token)
	if err != nil {
		_ = st.Close()
		t.Fatal(err)
	}
	if operator.ID != operatorID || !operator.Capabilities.List ||
		!operator.Capabilities.Evidence || operator.Capabilities.Decide {
		_ = st.Close()
		t.Fatalf("operator=%+v", operator)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	revokeOutput := captureModerationCommandOutput(t, func() error {
		return runModerationOperatorCommand(configPath, "", operatorID, "")
	})
	if !strings.Contains(revokeOutput, "revoked=true") {
		t.Fatalf("unexpected revoke output=%q", revokeOutput)
	}
	st, err = store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.ResolveModerationOperator(token); !errors.Is(err, store.ErrUnauthorized) {
		t.Fatalf("revoked operator resolve error=%v", err)
	}
}

func TestParseModerationOperatorScopesRejectsExpansionAndDuplicates(t *testing.T) {
	for _, value := range []string{"", "admin", "list,list", "list,"} {
		if _, err := parseModerationOperatorScopes(value); err == nil {
			t.Fatalf("scopes %q accepted", value)
		}
	}
}
