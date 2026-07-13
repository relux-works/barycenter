// Config and credentials storage for the Windows Pulsar shell.
//
// Everything lives in one directory — %APPDATA%\Pulsar on Windows
// (os.UserConfigDir), the platform config dir on dev machines:
//
//	config.json            — non-secret settings
//	credentials.v1.dpapi  — current-user-DPAPI protected credential bundle
//	credentials.json      — bounded legacy migration source only
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	configFileName      = "config.json"
	credentialsFileName = "credentials.json"

	// defaultPipeName uses the \\.\pipe\LOCAL\ prefix — the only named-pipe
	// syntax allowed inside an AppContainer (research report §B).
	defaultPipeName = `\\.\pipe\LOCAL\pulsar-audio`
)

// Config is the on-disk settings file. Absent fields keep their defaults.
type Config struct {
	APIPort               int    `json:"api_port"`
	PipeName              string `json:"pipe_name"`
	CacheDir              string `json:"cache_dir"`
	DeviceName            string `json:"device_name,omitempty"` // empty = "Pulsar <SLOT>"
	RingBufferMS          int    `json:"ring_buffer_ms"`
	OutputLatencyOffsetMS int    `json:"output_latency_offset_ms"`
}

// Credentials preserves the legacy public node-only view used by startup,
// /pair, CLI, and Win32 onboarding callers.
type Credentials struct {
	OrbitID int64  `json:"orbit_id"`
	Slot    string `json:"slot"`
	Token   string `json:"token"`
	WSURL   string `json:"ws_url"`
}

func (c Credentials) String() string {
	return fmt.Sprintf("Credentials{orbit:%d credentials:<redacted>}", c.OrbitID)
}

func (c Credentials) GoString() string { return c.String() }

// DefaultConfigDir is %APPDATA%\Pulsar on Windows.
func DefaultConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}
	return filepath.Join(base, "Pulsar"), nil
}

// DefaultConfig returns the zero-config settings rooted at dir.
func DefaultConfig(dir string) Config {
	return Config{
		APIPort:               3678,
		PipeName:              defaultPipeName,
		CacheDir:              filepath.Join(dir, "cache"),
		RingBufferMS:          1000,
		OutputLatencyOffsetMS: 0,
	}
}

// LoadConfig reads config.json from dir; a missing file means defaults
// (zero-config mode, mirroring the macOS R1 behavior). Fields absent from
// the file keep their default values.
func LoadConfig(dir string) (Config, error) {
	cfg := DefaultConfig(dir)
	raw, err := os.ReadFile(filepath.Join(dir, configFileName))
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read %s: %w", configFileName, err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("%s is not valid JSON: %w", configFileName, err)
	}
	return cfg, nil
}

// Save writes config.json under dir.
func (c Config) Save(dir string) error {
	return writeJSON(filepath.Join(dir, configFileName), c, 0o644)
}

// LoadCredentials returns nil (not an error) when the node is unpaired.
func LoadCredentials(dir string) (*Credentials, error) {
	repository, err := newDefaultCredentialRepository(dir)
	if err != nil {
		return nil, err
	}
	return LoadCredentialsFromRepository(repository)
}

func LoadCredentialsFromRepository(repository *ProtectedCredentialRepository) (*Credentials, error) {
	bundle, err := repository.LoadBundle()
	if err != nil || bundle == nil || bundle.Node == nil {
		return nil, err
	}
	credentials := credentialsFromNode(*bundle.Node)
	return &credentials, nil
}

// Save updates only the node capability in the protected bundle. Any valid
// control capability and non-secret recovery context remain untouched.
func (c Credentials) Save(dir string) error {
	repository, err := newDefaultCredentialRepository(dir)
	if err != nil {
		return err
	}
	return c.SaveToRepository(repository)
}

func (c Credentials) SaveToRepository(repository *ProtectedCredentialRepository) error {
	if err := ValidateCredentials(c); err != nil {
		return err
	}
	return repository.UpdateNode(nodeFromCredentials(c))
}

// writeJSON writes pretty JSON atomically (temp file + rename).
func writeJSON(path string, v any, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename into %s: %w", path, err)
	}
	return nil
}

// ConfigError collects every problem at once so the user fixes the file in
// one pass — validation reads like advice, not a stack trace (goal DoD-10,
// same contract as the macOS ConfigLoader).
type ConfigError struct {
	Problems []string
}

func (e *ConfigError) Error() string {
	return "config invalid:\n  - " + strings.Join(e.Problems, "\n  - ")
}

// ValidateConfig checks the settings file.
func ValidateConfig(c Config) error {
	var problems []string
	if c.APIPort < 1 || c.APIPort > 65535 {
		problems = append(problems, fmt.Sprintf("api_port %d is not a valid port", c.APIPort))
	}
	if !strings.HasPrefix(c.PipeName, `\\.\pipe\`) {
		problems = append(problems, fmt.Sprintf(`pipe_name %q must start with \\.\pipe\ (use \\.\pipe\LOCAL\ for AppContainer)`, c.PipeName))
	}
	if c.RingBufferMS < 100 || c.RingBufferMS > 5000 {
		problems = append(problems, fmt.Sprintf("ring_buffer_ms is %d, expected 100..5000", c.RingBufferMS))
	}
	if c.CacheDir == "" {
		problems = append(problems, "cache_dir is required (voice insert cache)")
	}
	if len(problems) > 0 {
		return &ConfigError{Problems: problems}
	}
	return nil
}

// ValidateCredentials checks the pairing result (same rules the macOS
// ConfigLoader applies to node_id/coordinator url/token).
func ValidateCredentials(c Credentials) error {
	var problems []string
	if c.OrbitID <= 0 {
		problems = append(problems, "orbit_id must be a positive integer")
	}
	if len(c.Slot) != 1 || c.Slot[0] < 'a' || c.Slot[0] > 'z' {
		problems = append(problems, fmt.Sprintf("slot is %q, must be a single letter a…z", c.Slot))
	}
	if err := validateLegacyWSURL(c.WSURL); err != nil {
		problems = append(problems, "ws_url is invalid (expected a safe ws:// or wss:// coordinator URL)")
	}
	if !isHexToken(c.Token) {
		problems = append(problems, "token must be 64 lowercase hex chars (32 random bytes)")
	}
	if len(problems) > 0 {
		return &ConfigError{Problems: problems}
	}
	return nil
}

func isHexToken(s string) bool {
	return lowerHexTokenPattern.MatchString(s)
}
