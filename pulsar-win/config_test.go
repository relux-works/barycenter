package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultsAndZeroConfigLoad(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(dir) // no file: zero-config mode
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIPort != 3678 || cfg.RingBufferMS != 1000 {
		t.Fatalf("defaults wrong: %+v", cfg)
	}
	if cfg.PipeName != `\\.\pipe\LOCAL\pulsar-audio` {
		t.Fatalf("pipe name %q", cfg.PipeName)
	}
	if cfg.CacheDir != filepath.Join(dir, "cache") {
		t.Fatalf("cache dir %q", cfg.CacheDir)
	}
	if err := ValidateConfig(cfg); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
}

func TestConfigPartialFileKeepsDefaults(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, configFileName), []byte(`{"api_port": 4444}`), 0o644)
	cfg, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIPort != 4444 {
		t.Fatalf("api_port %d, want 4444", cfg.APIPort)
	}
	if cfg.RingBufferMS != 1000 || cfg.PipeName == "" {
		t.Fatalf("absent fields must keep defaults: %+v", cfg)
	}
}

func TestConfigSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig(dir)
	cfg.DeviceName = "Pulsar Test"
	if err := cfg.Save(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != cfg {
		t.Fatalf("round trip mismatch:\nsaved  %+v\nloaded %+v", cfg, loaded)
	}
}

func TestCredentialsRoundTripAndPermissions(t *testing.T) {
	repository, files := newTestCredentialRepository(t)
	if creds, err := LoadCredentialsFromRepository(repository); err != nil || creds != nil {
		t.Fatalf("unpaired state must be (nil, nil), got (%v, %v)", creds, err)
	}
	creds := Credentials{
		OrbitID: 7, Slot: "b",
		Token: strings.Repeat("ab", 32),
		WSURL: "wss://barycenter.relux.works/ws",
	}
	if err := creds.SaveToRepository(repository); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCredentialsFromRepository(repository)
	if err != nil {
		t.Fatal(err)
	}
	if *loaded != creds {
		t.Fatalf("round trip mismatch: %+v", loaded)
	}
	if _, ok := files.data[repository.activePath()]; !ok {
		t.Fatal("protected destination was not written")
	}
	if _, ok := files.data[filepath.Join(repository.dir, credentialsFileName)]; ok {
		t.Fatal("plaintext credentials must not be created")
	}
}

func TestValidateConfigCollectsAllProblems(t *testing.T) {
	err := ValidateConfig(Config{APIPort: 0, PipeName: "bad", RingBufferMS: 5, CacheDir: ""})
	if err == nil {
		t.Fatal("expected problems")
	}
	msg := err.Error()
	for _, want := range []string{"api_port", "pipe_name", "ring_buffer_ms", "cache_dir"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error must name %s:\n%s", want, msg)
		}
	}
}

func TestValidateCredentials(t *testing.T) {
	good := Credentials{OrbitID: 1, Slot: "a", Token: strings.Repeat("0f", 32), WSURL: "wss://x/ws"}
	if err := ValidateCredentials(good); err != nil {
		t.Fatalf("good creds rejected: %v", err)
	}
	bad := Credentials{Slot: "AB", Token: "short", WSURL: "https://x"}
	err := ValidateCredentials(bad)
	if err == nil {
		t.Fatal("bad creds accepted")
	}
	for _, want := range []string{"slot", "token", "ws_url"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error must name %s:\n%s", want, err)
		}
	}
	plaintextRemote := good
	plaintextRemote.WSURL = "ws://127.0.0.2/ws"
	if err := ValidateCredentials(plaintextRemote); err == nil {
		t.Fatal("non-loopback plaintext websocket accepted")
	}
}
