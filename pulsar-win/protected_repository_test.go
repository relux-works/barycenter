package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type testProtector struct {
	mu             sync.Mutex
	protectCalls   int
	unprotectCalls int
	failProtect    bool
	failUnprotect  bool
}

func (p *testProtector) Protect(value []byte) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.protectCalls++
	if p.failProtect {
		return nil, errors.New("canary protect error")
	}
	result := make([]byte, len(value)+4)
	copy(result, "ENC1")
	for i := range value {
		result[i+4] = value[i] ^ 0xa5
	}
	return result, nil
}

func (p *testProtector) Unprotect(value []byte) ([]byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.unprotectCalls++
	if p.failUnprotect || len(value) < 4 || string(value[:4]) != "ENC1" {
		return nil, errors.New("canary unprotect error")
	}
	result := make([]byte, len(value)-4)
	for i := range result {
		result[i] = value[i+4] ^ 0xa5
	}
	return result, nil
}

type testFileCall struct {
	Operation string
	Path      string
	Spec      secureOpenSpec
	Flags     uint32
	Handle    secureFileHandle
	Count     int
}

type testOpenFile struct {
	path   string
	pos    int
	lock   bool
	closed bool
}

type testSecureFileOps struct {
	mu           sync.Mutex
	data         map[string][]byte
	handles      map[secureFileHandle]*testOpenFile
	locks        map[string]bool
	nextHandle   secureFileHandle
	calls        []testFileCall
	failAt       map[string]int
	counts       map[string]int
	writeChunk   int
	readChunk    int
	zeroWrite    bool
	zeroRead     bool
	reportedSize map[string]int64
	afterMove    func(*testSecureFileOps, string)
}

func newTestSecureFileOps() *testSecureFileOps {
	return &testSecureFileOps{data: map[string][]byte{}, handles: map[secureFileHandle]*testOpenFile{}, locks: map[string]bool{}, failAt: map[string]int{}, counts: map[string]int{}, reportedSize: map[string]int64{}}
}

func (f *testSecureFileOps) fail(operation string) bool {
	f.counts[operation]++
	return f.failAt[operation] > 0 && f.counts[operation] == f.failAt[operation]
}

func (f *testSecureFileOps) EnsureDir(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, testFileCall{Operation: "ensure", Path: path})
	if f.fail("ensure") {
		return errors.New("canary ensure")
	}
	return nil
}

func (f *testSecureFileOps) Exists(path string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, testFileCall{Operation: "exists", Path: path})
	if f.fail("exists") {
		return false, errors.New("canary exists")
	}
	_, ok := f.data[path]
	return ok, nil
}

func (f *testSecureFileOps) Open(path string, spec secureOpenSpec) (secureFileHandle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, testFileCall{Operation: "open", Path: path, Spec: spec})
	if f.fail("open") {
		return 0, errors.New("canary open")
	}
	_, exists := f.data[path]
	switch spec.Disposition {
	case fileCreateNew:
		if exists {
			return 0, errors.New("exists")
		}
		f.data[path] = nil
	case fileOpenExisting:
		if !exists {
			return 0, errors.New("missing")
		}
	case fileOpenAlways:
		if !exists {
			f.data[path] = nil
		}
	default:
		return 0, errors.New("bad disposition")
	}
	f.nextHandle++
	handle := f.nextHandle
	f.handles[handle] = &testOpenFile{path: path}
	return handle, nil
}

func (f *testSecureFileOps) Write(handle secureFileHandle, value []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, testFileCall{Operation: "write", Handle: handle, Count: len(value)})
	if f.fail("write") {
		return 0, errors.New("canary write")
	}
	if f.zeroWrite {
		return 0, nil
	}
	file := f.handles[handle]
	if file == nil || file.closed {
		return 0, errors.New("bad handle")
	}
	n := len(value)
	if f.writeChunk > 0 && n > f.writeChunk {
		n = f.writeChunk
	}
	current := f.data[file.path]
	if file.pos+n > len(current) {
		grown := make([]byte, file.pos+n)
		copy(grown, current)
		current = grown
	}
	copy(current[file.pos:], value[:n])
	file.pos += n
	f.data[file.path] = current
	return n, nil
}

func (f *testSecureFileOps) Read(handle secureFileHandle, value []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, testFileCall{Operation: "read", Handle: handle, Count: len(value)})
	if f.fail("read") {
		return 0, errors.New("canary read")
	}
	if f.zeroRead {
		return 0, nil
	}
	file := f.handles[handle]
	if file == nil || file.closed {
		return 0, errors.New("bad handle")
	}
	remaining := f.data[file.path][file.pos:]
	n := len(value)
	if n > len(remaining) {
		n = len(remaining)
	}
	if f.readChunk > 0 && n > f.readChunk {
		n = f.readChunk
	}
	copy(value, remaining[:n])
	file.pos += n
	return n, nil
}

func (f *testSecureFileOps) Size(handle secureFileHandle) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, testFileCall{Operation: "size", Handle: handle})
	if f.fail("size") {
		return 0, errors.New("canary size")
	}
	file := f.handles[handle]
	if value, ok := f.reportedSize[file.path]; ok {
		return value, nil
	}
	return int64(len(f.data[file.path])), nil
}

func (f *testSecureFileOps) Flush(handle secureFileHandle) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, testFileCall{Operation: "flush", Handle: handle})
	if f.fail("flush") {
		return errors.New("canary flush")
	}
	return nil
}

func (f *testSecureFileOps) Close(handle secureFileHandle) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, testFileCall{Operation: "close", Handle: handle})
	file := f.handles[handle]
	if file == nil || file.closed {
		return errors.New("double close")
	}
	file.closed = true
	if file.lock {
		delete(f.locks, file.path)
	}
	if f.fail("close") {
		return errors.New("canary close")
	}
	return nil
}

func (f *testSecureFileOps) Move(from, to string, flags uint32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, testFileCall{Operation: "move", Path: from + "->" + to, Flags: flags})
	if f.fail("move") {
		return errors.New("canary move")
	}
	value, ok := f.data[from]
	if !ok {
		return errors.New("missing")
	}
	f.data[to] = append([]byte(nil), value...)
	delete(f.data, from)
	if f.afterMove != nil {
		f.afterMove(f, to)
	}
	return nil
}

func (f *testSecureFileOps) Delete(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, testFileCall{Operation: "delete", Path: path})
	if f.fail("delete") {
		return errors.New("canary delete")
	}
	delete(f.data, path)
	return nil
}

func (f *testSecureFileOps) List(dir string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, testFileCall{Operation: "list", Path: dir})
	if f.fail("list") {
		return nil, errors.New("canary list")
	}
	var names []string
	for path := range f.data {
		if filepath.Dir(path) == dir {
			names = append(names, filepath.Base(path))
		}
	}
	return names, nil
}

func (f *testSecureFileOps) AcquireLock(path string) (secureFileHandle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, testFileCall{Operation: "lock", Path: path})
	if f.fail("lock") || f.locks[path] {
		return 0, errors.New("busy")
	}
	f.locks[path] = true
	if _, ok := f.data[path]; !ok {
		f.data[path] = nil
	}
	f.nextHandle++
	handle := f.nextHandle
	f.handles[handle] = &testOpenFile{path: path, lock: true}
	return handle, nil
}

type fixedTestClock struct{ value time.Time }

func (c fixedTestClock) Now() time.Time { return c.value }

func newTestCredentialRepository(t testing.TB) (*ProtectedCredentialRepository, *testSecureFileOps) {
	t.Helper()
	files := newTestSecureFileOps()
	repository, err := NewProtectedCredentialRepository(CredentialRepositoryOptions{Directory: filepath.Join("C:", "Users", "test", "Pulsar"), Protector: &testProtector{}, Files: files, Random: bytes.NewReader(bytes.Repeat([]byte{0x42}, 1024)), Clock: fixedTestClock{time.Unix(1, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	return repository, files
}

func sampleCredentials() Credentials {
	return Credentials{OrbitID: 7, Slot: "b", Token: strings.Repeat("ab", 32), WSURL: "wss://coord.example/ws"}
}

func sampleBundle() CredentialBundle {
	node := nodeFromCredentials(sampleCredentials())
	control := ControlCredential{ActorID: 9, OrbitID: 7, Role: "primary", ControlToken: strings.Repeat("cd", 32), Context: ControlContextActive}
	return CredentialBundle{Version: credentialBundleVersion, Node: &node, Control: &control, RecoveryID: "rec_0123456789abcdef0123456789abcdef", CoordinatorOrigin: "https://coord.example"}
}

func TestCredentialBundleSplitCompatibilityAndRedaction(t *testing.T) {
	repository, _ := newTestCredentialRepository(t)
	bundle := sampleBundle()
	if err := repository.SaveBundle(bundle); err != nil {
		t.Fatal(err)
	}
	paired, err := LoadCredentialsFromRepository(repository)
	if err != nil || paired == nil || *paired != sampleCredentials() {
		t.Fatalf("node compatibility: %#v %v", paired, err)
	}
	controlOnly := bundle
	controlOnly.Node = nil
	if err := repository.SaveBundle(controlOnly); err != nil {
		t.Fatal(err)
	}
	paired, err = LoadCredentialsFromRepository(repository)
	if err != nil || paired != nil {
		t.Fatalf("control-only must not masquerade as paired: %#v %v", paired, err)
	}
	for _, printable := range []string{fmt.Sprint(bundle), fmt.Sprintf("%#v", bundle), fmt.Sprint(*bundle.Node), fmt.Sprintf("%#v", *bundle.Control), fmt.Sprint(sampleCredentials())} {
		for _, secret := range []string{bundle.Node.NodeToken, bundle.Control.ControlToken, bundle.Node.WSURL, bundle.RecoveryID} {
			if strings.Contains(printable, secret) {
				t.Fatalf("secret leaked through formatting: %q", printable)
			}
		}
	}
	canary := "ADVERSARIAL_SECRET_CANARY"
	secretCode := newOneTimeCode("ABCDEFGHJKMNPQRSTVWXYZ23456")
	values := []any{
		Credentials{Slot: canary, Token: canary, WSURL: canary},
		NodeCredential{Slot: canary, NodeToken: canary, WSURL: canary},
		ControlCredential{Role: canary, Context: ControlContextState(canary), ControlToken: canary},
		CreateOrbitResult{Role: canary, Title: canary, Bundle: bundle, Recovery: testRecoveryMaterial(t)},
		JoinOrbitResult{Role: canary, Title: canary, Bundle: bundle},
		DeviceInvite{Code: secretCode, IntendedRole: canary},
		TelegramLink{Code: secretCode, DesiredRole: canary, BotUsername: canary},
		RecoveryResult{State: RecoveryState(canary), Bundle: &bundle},
		&OnboardingClientError{Kind: ClientErrorKind(canary), Code: canary},
	}
	for _, value := range values {
		for _, printable := range []string{fmt.Sprint(value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value)} {
			if strings.Contains(printable, canary) || strings.Contains(printable, "ABCDEFGHJKMNPQRSTVWXYZ23456") {
				t.Fatalf("adversarial formatter leak from %T: %q", value, printable)
			}
		}
	}
}

func TestSensitiveServiceContainerFormattingIsRedacted(t *testing.T) {
	pathCanary := `C:\Users\PATH-CANARY\Pulsar`
	origin, err := CanonicalCoordinatorOrigin("https://url-canary.example")
	if err != nil {
		t.Fatal(err)
	}
	repository := &ProtectedCredentialRepository{dir: pathCanary}
	client := &OnboardingClient{origin: origin}
	recovery := &RecoveryService{repository: repository, client: client}
	service := &WindowsOnboardingService{client: client, repository: repository, recovery: recovery}
	for name, value := range map[string]any{
		"repository": repository,
		"client":     client,
		"recovery":   recovery,
		"service":    service,
	} {
		for _, rendered := range []string{fmt.Sprint(value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value)} {
			for _, canary := range []string{pathCanary, "url-canary.example"} {
				if strings.Contains(rendered, canary) {
					t.Fatalf("%s formatting leaked %q: %s", name, canary, rendered)
				}
			}
		}
	}
}

func TestLegacyNodeDerivesOriginBoundCapabilityWithoutChangingWSBytes(t *testing.T) {
	for _, test := range []struct {
		wsURL, origin string
	}{
		{"wss://coord.example:443/ws", "https://coord.example"},
		{"wss://coord.example:8443/ws", "https://coord.example:8443"},
		{"ws://127.0.0.1:8080/ws", "http://127.0.0.1:8080"},
		{"ws://[0:0:0:0:0:0:0:1]:8080/ws", "http://[::1]:8080"},
	} {
		t.Run(test.wsURL, func(t *testing.T) {
			repository, _ := newTestCredentialRepository(t)
			credentials := sampleCredentials()
			credentials.WSURL = test.wsURL
			if err := credentials.SaveToRepository(repository); err != nil {
				t.Fatal(err)
			}
			bundle, err := repository.LoadBundle()
			if err != nil || bundle.Node.WSURL != test.wsURL || bundle.CoordinatorOrigin != test.origin {
				t.Fatalf("bundle=%#v err=%v", bundle, err)
			}
			capability, ok := bundle.NodeCapability()
			if !ok || capability.origin.String() != test.origin || capability.value.WSURL != test.wsURL {
				t.Fatalf("node capability=%#v ok=%t", capability, ok)
			}
		})
	}
}

func TestLegacyMigrationPreservesNodeAndRemovesPlaintext(t *testing.T) {
	repository, files := newTestCredentialRepository(t)
	legacyPath := filepath.Join(repository.dir, credentialsFileName)
	raw, _ := json.Marshal(sampleCredentials())
	files.data[legacyPath] = raw
	bundle, err := repository.LoadBundle()
	if err != nil {
		t.Fatal(err)
	}
	if bundle == nil || bundle.Node == nil || credentialsFromNode(*bundle.Node) != sampleCredentials() {
		t.Fatalf("migration changed node: %#v", bundle)
	}
	if _, ok := files.data[legacyPath]; ok {
		t.Fatal("legacy plaintext survived verified migration")
	}
	ciphertext := files.data[repository.activePath()]
	if bytes.Contains(ciphertext, []byte(sampleCredentials().Token)) || bytes.Contains(ciphertext, []byte(sampleCredentials().WSURL)) {
		t.Fatal("protected file contains plaintext node data")
	}
	second, err := repository.LoadBundle()
	if err != nil || !reflect.DeepEqual(second, bundle) {
		t.Fatalf("restart did not converge: %#v %v", second, err)
	}
}

func TestLegacyMigrationDeleteFailureConvergesAndControlOnlyMerges(t *testing.T) {
	repository, files := newTestCredentialRepository(t)
	controlOnly := sampleBundle()
	controlOnly.Node = nil
	if err := repository.SaveBundle(controlOnly); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(repository.dir, credentialsFileName)
	raw, _ := json.Marshal(sampleCredentials())
	files.data[legacyPath] = raw
	files.counts = map[string]int{}
	files.failAt["delete"] = 1
	if _, err := repository.LoadBundle(); err == nil {
		t.Fatal("legacy delete failure claimed migration success")
	}
	if _, ok := files.data[legacyPath]; !ok {
		t.Fatal("delete failure lost legacy copy")
	}
	protected, err := repository.readBundle(repository.activePath())
	if err != nil || protected.Node == nil || protected.Control == nil {
		t.Fatalf("verified merged protected bundle missing: %#v %v", protected, err)
	}
	files.failAt["delete"] = 0
	converged, err := repository.LoadBundle()
	if err != nil || converged.Node == nil || converged.Control == nil {
		t.Fatalf("restart failed to converge: %#v %v", converged, err)
	}
	if _, ok := files.data[legacyPath]; ok {
		t.Fatal("equivalent surviving plaintext was not retried for deletion")
	}
}

func TestRepairUpdatesNodeWithoutErasingControl(t *testing.T) {
	repository, _ := newTestCredentialRepository(t)
	initial := sampleBundle()
	if err := repository.SaveBundle(initial); err != nil {
		t.Fatal(err)
	}
	repaired := sampleCredentials()
	repaired.Slot = "c"
	repaired.Token = strings.Repeat("12", 32)
	if err := repaired.SaveToRepository(repository); err != nil {
		t.Fatal(err)
	}
	bundle, err := repository.LoadBundle()
	if err != nil {
		t.Fatal(err)
	}
	if credentialsFromNode(*bundle.Node) != repaired {
		t.Fatal("re-pair node bytes changed")
	}
	if *bundle.Control != *initial.Control || bundle.RecoveryID != initial.RecoveryID {
		t.Fatal("re-pair erased control/recovery state")
	}
}

func TestCrossOriginRepairFailsClosedWithoutChangingProtectedBundle(t *testing.T) {
	repository, files := newTestCredentialRepository(t)
	initial := sampleBundle()
	if err := repository.SaveBundle(initial); err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), files.data[repository.activePath()]...)
	conflicting := sampleCredentials()
	conflicting.Token = strings.Repeat("12", 32)
	conflicting.WSURL = "wss://other.example/ws"
	if err := conflicting.SaveToRepository(repository); !errors.Is(err, errCredentialStorageConflict) {
		t.Fatalf("cross-origin repair error=%v", err)
	}
	if !bytes.Equal(before, files.data[repository.activePath()]) {
		t.Fatal("cross-origin repair rewrote protected state")
	}
	loaded, err := repository.LoadBundle()
	if err != nil || !reflect.DeepEqual(*loaded, initial) {
		t.Fatalf("protected state changed: %#v err=%v", loaded, err)
	}

	wrongPath := sampleCredentials()
	wrongPath.WSURL = "wss://coord.example/other"
	if err := ValidateCredentials(wrongPath); err == nil {
		t.Fatal("legacy node accepted a non-/ws endpoint")
	}
}

func TestRecoveryBackupAcknowledgementIsGenerationBound(t *testing.T) {
	repository, _ := newTestCredentialRepository(t)
	bundle := sampleBundle()
	bundle.RecoveryBackupAcknowledged = true
	if err := repository.SaveBundle(bundle); err != nil {
		t.Fatal(err)
	}
	origin, _ := CanonicalCoordinatorOrigin(bundle.CoordinatorOrigin)
	capability, _ := bundle.ControlCapability()
	newID := "rec_abcdefabcdefabcdefabcdefabcdefab"
	if err := repository.UpdateRecoveryMetadata(capability, newID); err != nil {
		t.Fatal(err)
	}
	rotated, err := repository.LoadBundle()
	if err != nil || rotated.RecoveryID != newID || rotated.RecoveryBackupAcknowledged {
		t.Fatalf("rotation retained stale acknowledgement: %#v err=%v", rotated, err)
	}
	if err := repository.AcknowledgeRecoveryBackup(origin, bundle.Control.ActorID, bundle.RecoveryID); !errors.Is(err, errCredentialStorageConflict) {
		t.Fatalf("stale generation acknowledged: %v", err)
	}
	if err := repository.AcknowledgeRecoveryBackup(origin, bundle.Control.ActorID, newID); err != nil {
		t.Fatal(err)
	}
	acknowledged, err := repository.LoadBundle()
	if err != nil || !acknowledged.RecoveryBackupAcknowledged || acknowledged.RecoveryID != newID {
		t.Fatalf("exact generation not acknowledged: %#v err=%v", acknowledged, err)
	}
}

func TestConcurrentRecoveryRotationCannotCarryStaleAcknowledgement(t *testing.T) {
	for iteration := 0; iteration < 20; iteration++ {
		repository, _ := newTestCredentialRepository(t)
		bundle := sampleBundle()
		if err := repository.SaveBundle(bundle); err != nil {
			t.Fatal(err)
		}
		origin, _ := CanonicalCoordinatorOrigin(bundle.CoordinatorOrigin)
		capability, _ := bundle.ControlCapability()
		newID := "rec_abcdefabcdefabcdefabcdefabcdefab"
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() {
			<-start
			results <- repository.AcknowledgeRecoveryBackup(origin, bundle.Control.ActorID, bundle.RecoveryID)
		}()
		go func() {
			<-start
			results <- repository.UpdateRecoveryMetadata(capability, newID)
		}()
		close(start)
		<-results
		<-results
		loaded, err := repository.LoadBundle()
		if err != nil || loaded.RecoveryID != newID || loaded.RecoveryBackupAcknowledged {
			t.Fatalf("iteration %d stale concurrent acknowledgement: %#v err=%v", iteration, loaded, err)
		}
	}
}

func TestLegacyMigrationConflictAndCorruptDestinationFailClosed(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*ProtectedCredentialRepository, *testSecureFileOps)
	}{
		{"conflict", func(repository *ProtectedCredentialRepository, files *testSecureFileOps) {
			bundle := sampleBundle()
			bundle.Node.NodeToken = strings.Repeat("ef", 32)
			if err := repository.SaveBundle(bundle); err != nil {
				t.Fatal(err)
			}
		}},
		{"corrupt", func(repository *ProtectedCredentialRepository, files *testSecureFileOps) {
			files.data[repository.activePath()] = []byte("not ciphertext")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, files := newTestCredentialRepository(t)
			test.setup(repository, files)
			legacyPath := filepath.Join(repository.dir, credentialsFileName)
			raw, _ := json.Marshal(sampleCredentials())
			files.data[legacyPath] = raw
			if _, err := repository.LoadBundle(); err == nil {
				t.Fatal("expected fail-closed migration")
			}
			if _, ok := files.data[legacyPath]; !ok {
				t.Fatal("legacy source was deleted on conflict/corruption")
			}
			if _, ok := files.data[repository.activePath()]; !ok {
				t.Fatal("protected destination was deleted on conflict/corruption")
			}
		})
	}
}

func TestLegacyMigrationCrashMatrixKeepsReadableCopy(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		at        int
	}{
		{"temp open", "open", 2},
		{"write", "write", 1},
		{"flush", "flush", 1},
		{"temp close", "close", 2},
		{"move", "move", 1},
		{"readback size", "size", 2},
		{"readback read", "read", 2},
		{"readback close", "close", 3},
		{"legacy delete", "delete", 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository, files := newTestCredentialRepository(t)
			legacyPath := filepath.Join(repository.dir, credentialsFileName)
			raw, _ := json.Marshal(sampleCredentials())
			files.data[legacyPath] = raw
			files.failAt[test.operation] = test.at
			if _, err := repository.LoadBundle(); err == nil {
				t.Fatal("fault claimed migration success")
			}
			if _, ok := files.data[legacyPath]; !ok {
				t.Fatal("legacy source lost before verified deletion")
			}
			if protected, ok := files.data[repository.activePath()]; ok {
				if _, err := (&testProtector{}).Unprotect(protected); err != nil {
					t.Fatal("surviving protected copy is unreadable")
				}
			}
		})
	}
}

func TestDurableWriteExactOrderAndParameters(t *testing.T) {
	repository, files := newTestCredentialRepository(t)
	files.writeChunk = 7
	files.readChunk = 5
	if err := repository.SaveBundle(sampleBundle()); err != nil {
		t.Fatal(err)
	}
	var operations []string
	for _, call := range files.calls {
		if call.Operation == "open" && strings.Contains(call.Path, ".tmp.") {
			want := secureOpenSpec{Access: fileGenericWrite, Share: 0, Disposition: fileCreateNew, Flags: fileAttributeNormal | fileFlagWriteThrough}
			if call.Spec != want {
				t.Fatalf("temp open %#v, want %#v", call.Spec, want)
			}
			operations = append(operations, "open-temp")
		} else if call.Operation == "open" && call.Path == repository.activePath() {
			want := secureOpenSpec{Access: fileGenericRead, Share: 0, Disposition: fileOpenExisting, Flags: fileAttributeNormal}
			if call.Spec != want {
				t.Fatalf("readback open %#v, want %#v", call.Spec, want)
			}
			operations = append(operations, "open-readback")
		} else if call.Operation == "flush" || call.Operation == "close" || call.Operation == "move" || call.Operation == "size" || call.Operation == "read" || call.Operation == "write" {
			operations = append(operations, call.Operation)
			if call.Operation == "move" && call.Flags != moveReplaceExisting|moveWriteThrough {
				t.Fatalf("move flags %#x", call.Flags)
			}
		}
	}
	joined := strings.Join(operations, ",")
	for _, sequence := range []string{"open-temp,write", "flush,close,move,open-readback,size,read"} {
		if !strings.Contains(joined, sequence) {
			t.Fatalf("missing durable sequence %q in %s", sequence, joined)
		}
	}
}

func TestDurableWriteFaultsDoNotClaimSuccess(t *testing.T) {
	for _, operation := range []string{"open", "write", "flush", "close", "move", "size", "read"} {
		t.Run(operation, func(t *testing.T) {
			repository, files := newTestCredentialRepository(t)
			files.failAt[operation] = 1
			if err := repository.SaveBundle(sampleBundle()); err == nil {
				t.Fatalf("%s fault claimed success", operation)
			}
			if operation == "flush" || operation == "close" {
				for _, call := range files.calls {
					if call.Operation == "move" {
						t.Fatalf("move followed %s failure", operation)
					}
				}
			}
		})
	}
	t.Run("zero progress write", func(t *testing.T) {
		repository, files := newTestCredentialRepository(t)
		files.zeroWrite = true
		if err := repository.SaveBundle(sampleBundle()); err == nil {
			t.Fatal("zero-progress write claimed success")
		}
	})
	t.Run("zero progress read", func(t *testing.T) {
		repository, files := newTestCredentialRepository(t)
		files.zeroRead = true
		if err := repository.SaveBundle(sampleBundle()); err == nil {
			t.Fatal("zero-progress readback claimed success")
		}
	})
	t.Run("post-move corruption", func(t *testing.T) {
		repository, files := newTestCredentialRepository(t)
		files.afterMove = func(files *testSecureFileOps, destination string) {
			files.data[destination] = []byte("corrupt after move")
		}
		if err := repository.SaveBundle(sampleBundle()); err == nil {
			t.Fatal("corrupt read-back claimed success")
		}
		if _, ok := files.data[repository.activePath()]; !ok {
			t.Fatal("corrupt destination was deleted")
		}
	})
}

func TestProtectedReadBoundsAndShortRead(t *testing.T) {
	for _, test := range []struct {
		name   string
		size   int
		report int64
	}{
		{"zero", 0, 0},
		{"oversize", maximumCiphertextBytes + 1, maximumCiphertextBytes + 1},
		{"short", 8, 16},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, files := newTestCredentialRepository(t)
			files.data[repository.activePath()] = make([]byte, test.size)
			if test.report != int64(test.size) {
				files.reportedSize[repository.activePath()] = test.report
			}
			if _, err := repository.LoadBundle(); err == nil {
				t.Fatal("bounded read fault accepted")
			}
			closeCount := 0
			for _, call := range files.calls {
				if call.Operation == "close" {
					closeCount++
				}
			}
			if closeCount != 1 {
				t.Fatalf("read handle close attempts=%d", closeCount)
			}
		})
	}
}

func TestProtectedEnvelopeStrictness(t *testing.T) {
	payload := []byte(`{"version":1}`)
	valid, err := encodeProtectedEnvelope(payload)
	if err != nil {
		t.Fatal(err)
	}
	cases := [][]byte{
		valid[:8],
		append([]byte("NOPE"), valid[4:]...),
		append(append([]byte(nil), valid...), 0),
	}
	unknown := append([]byte(nil), valid...)
	unknown[4] = 99
	cases = append(cases, unknown)
	oversized := append([]byte(nil), valid...)
	oversized[5], oversized[6] = 0xff, 0xff
	cases = append(cases, oversized)
	for i, value := range cases {
		if _, err := decodeProtectedEnvelope(value); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
	if _, err := strictDecodeBundleForTest([]byte(`{"version":1,"version":1}`)); err == nil {
		t.Fatal("duplicate JSON key accepted")
	}
	if _, err := strictDecodeBundleForTest([]byte(`{"version":1,"unknown":true}`)); err == nil {
		t.Fatal("unknown JSON key accepted")
	}
	pendingMissingSent := []byte(`{"canonical_coordinator_origin":"https://coord.example","actor_id":9,"recovery_id":"rec_0123456789abcdef0123456789abcdef","pending_control_token":"` + strings.Repeat("ef", 32) + `"}`)
	if _, err := decodePendingRecovery(pendingMissingSent); err == nil {
		t.Fatal("pending record without explicit ever_sent accepted")
	}
	for _, scalar := range []string{"null", `"false"`, "0"} {
		pendingInvalidSent := append(append([]byte(nil), pendingMissingSent[:len(pendingMissingSent)-1]...), []byte(`,"ever_sent":`+scalar+`}`)...)
		if _, err := decodePendingRecovery(pendingInvalidSent); err == nil {
			t.Fatalf("pending record with %s ever_sent accepted", scalar)
		}
	}
	for _, field := range []string{"recovery_consumed", "recovery_backup_acknowledged"} {
		for _, scalar := range []string{"null", `"true"`, "1"} {
			bundle := sampleBundle()
			bundle.RecoveryConsumed = true
			bundle.RecoveryBackupAcknowledged = true
			raw, _ := json.Marshal(bundle)
			raw = bytes.Replace(raw, []byte(`"`+field+`":true`), []byte(`"`+field+`":`+scalar), 1)
			if _, err := decodeCredentialBundle(raw); err == nil {
				t.Fatalf("bundle accepted %s %s", scalar, field)
			}
		}
	}
	incoherent := sampleBundle()
	incoherent.Control = nil
	incoherent.RecoveryID = ""
	incoherent.RecoveryBackupAcknowledged = true
	if incoherent.validate() == nil {
		t.Fatal("acknowledgement without recovery/control metadata accepted")
	}
}

func TestCredentialBundleRejectsExplicitNullOptionalFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"node", func(object map[string]any) { object["node"] = nil }},
		{"control", func(object map[string]any) {
			object["control"] = nil
			delete(object, "recovery_id")
		}},
		{"recovery id", func(object map[string]any) { object["recovery_id"] = nil }},
		{"active last known orbit", func(object map[string]any) {
			object["control"].(map[string]any)["last_known_orbit_id"] = nil
		}},
		{"active last known role", func(object map[string]any) {
			object["control"].(map[string]any)["last_known_role"] = nil
		}},
		{"limited orbit", func(object map[string]any) {
			control := object["control"].(map[string]any)
			control["context"] = "limited"
			control["orbit_id"] = nil
			delete(control, "role")
		}},
		{"limited role", func(object map[string]any) {
			control := object["control"].(map[string]any)
			control["context"] = "limited"
			delete(control, "orbit_id")
			control["role"] = nil
		}},
		{"limited empty last known pair", func(object map[string]any) {
			control := object["control"].(map[string]any)
			control["context"] = "limited"
			delete(control, "orbit_id")
			delete(control, "role")
			control["last_known_orbit_id"] = nil
			control["last_known_role"] = nil
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := json.Marshal(sampleBundle())
			if err != nil {
				t.Fatal(err)
			}
			var object map[string]any
			if err := json.Unmarshal(raw, &object); err != nil {
				t.Fatal(err)
			}
			test.mutate(object)
			raw, err = json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeCredentialBundle(raw); err == nil {
				t.Fatalf("explicit null accepted: %s", raw)
			}
		})
	}
}

func strictDecodeBundleForTest(payload []byte) (CredentialBundle, error) {
	value, err := decodeCredentialBundle(payload)
	if err != nil {
		return CredentialBundle{}, err
	}
	return value.(CredentialBundle), nil
}

type testAllocation struct {
	value []byte
	freed int
	err   error
}

func (a *testAllocation) Bytes() []byte { return a.value }
func (a *testAllocation) Free() error   { a.freed++; return a.err }

type testNativeDPAPI struct {
	protectFlags, unprotectFlags uint32
	protectAllocation            *testAllocation
	unprotectAllocation          *testAllocation
	protectErr, unprotectErr     error
}

func (a *testNativeDPAPI) Protect([]byte, uint32) (protectedAllocation, error) {
	panic("use configured wrapper")
}

type recordingNativeDPAPI struct{ *testNativeDPAPI }

func (a recordingNativeDPAPI) Protect(_ []byte, flags uint32) (protectedAllocation, error) {
	a.protectFlags = flags
	return a.protectAllocation, a.protectErr
}
func (a recordingNativeDPAPI) Unprotect(_ []byte, flags uint32) (protectedAllocation, error) {
	a.unprotectFlags = flags
	return a.unprotectAllocation, a.unprotectErr
}

func TestDPAPIFlagsAndPartialAllocationCleanup(t *testing.T) {
	native := &testNativeDPAPI{protectAllocation: &testAllocation{value: []byte("cipher")}, unprotectAllocation: &testAllocation{value: []byte("plain")}}
	protector := dpapiDataProtector{api: recordingNativeDPAPI{native}}
	if _, err := protector.Protect([]byte("plain")); err != nil {
		t.Fatal(err)
	}
	if _, err := protector.Unprotect([]byte("cipher")); err != nil {
		t.Fatal(err)
	}
	if native.protectFlags != cryptprotectUIForbidden || native.unprotectFlags != cryptprotectUIForbidden || native.protectFlags&cryptprotectLocalMachine != 0 {
		t.Fatalf("wrong flags protect=%#x unprotect=%#x", native.protectFlags, native.unprotectFlags)
	}
	if native.protectAllocation.freed != 1 || native.unprotectAllocation.freed != 1 {
		t.Fatalf("allocations not freed exactly once: %d %d", native.protectAllocation.freed, native.unprotectAllocation.freed)
	}
	if !bytes.Equal(native.protectAllocation.value, make([]byte, len(native.protectAllocation.value))) || !bytes.Equal(native.unprotectAllocation.value, make([]byte, len(native.unprotectAllocation.value))) {
		t.Fatal("native allocation bytes were not zeroed before LocalFree")
	}
	partial := &testAllocation{value: []byte("partial")}
	native.protectAllocation, native.protectErr = partial, errors.New("canary native")
	if _, err := protector.Protect([]byte("plain")); err == nil || partial.freed != 1 {
		t.Fatalf("partial failure cleanup err=%v freed=%d", err, partial.freed)
	}
	if !bytes.Equal(partial.value, make([]byte, len(partial.value))) {
		t.Fatal("partial encrypt allocation was not zeroed")
	}
	decryptPartial := &testAllocation{value: []byte("partial")}
	native.unprotectAllocation, native.unprotectErr = decryptPartial, errors.New("canary native")
	if _, err := protector.Unprotect([]byte("cipher")); err == nil || decryptPartial.freed != 1 {
		t.Fatalf("partial decrypt cleanup err=%v freed=%d", err, decryptPartial.freed)
	}
	if !bytes.Equal(decryptPartial.value, make([]byte, len(decryptPartial.value))) {
		t.Fatal("partial decrypt allocation was not zeroed")
	}
}

type partialErrorProtector struct{ plaintext []byte }

func (p *partialErrorProtector) Protect([]byte) ([]byte, error) { return nil, errors.New("unused") }
func (p *partialErrorProtector) Unprotect([]byte) ([]byte, error) {
	return p.plaintext, errors.New("partial decrypt")
}

func TestProtectedReadZerosPartialPlaintextOnDecryptError(t *testing.T) {
	files := newTestSecureFileOps()
	protector := &partialErrorProtector{plaintext: []byte("plaintext canary")}
	repository, err := NewProtectedCredentialRepository(CredentialRepositoryOptions{
		Directory: filepath.Join("C:", "Users", "test", "Pulsar"), Protector: protector, Files: files,
	})
	if err != nil {
		t.Fatal(err)
	}
	files.data[repository.activePath()] = []byte("ciphertext")
	if _, err := repository.LoadBundle(); err == nil {
		t.Fatal("partial decrypt failure accepted")
	}
	if !bytes.Equal(protector.plaintext, make([]byte, len(protector.plaintext))) {
		t.Fatal("partial plaintext was not zeroed")
	}
}

func TestTemporaryCloseFailureDeletesTempAndNeverMoves(t *testing.T) {
	repository, files := newTestCredentialRepository(t)
	files.failAt["close"] = 1
	if err := repository.SaveBundle(sampleBundle()); err == nil {
		t.Fatal("temporary close failure claimed success")
	}
	deleteCount, moveCount := 0, 0
	for _, call := range files.calls {
		if call.Operation == "delete" && strings.Contains(call.Path, ".tmp.") {
			deleteCount++
		}
		if call.Operation == "move" {
			moveCount++
		}
	}
	if deleteCount != 1 || moveCount != 0 {
		t.Fatalf("close failure delete=%d move=%d", deleteCount, moveCount)
	}
}

func TestStaleTempCleanupIsScoped(t *testing.T) {
	repository, files := newTestCredentialRepository(t)
	owned := repository.activePath() + ".tmp.0011223344556677"
	unrelated := repository.activePath() + ".tmp.not-owned"
	other := filepath.Join(repository.dir, "other.tmp.0011223344556677")
	files.data[owned], files.data[unrelated], files.data[other] = []byte("x"), []byte("y"), []byte("z")
	if err := repository.SaveBundle(sampleBundle()); err != nil {
		t.Fatal(err)
	}
	if _, ok := files.data[owned]; ok {
		t.Fatal("owned stale temp survived")
	}
	if _, ok := files.data[unrelated]; !ok {
		t.Fatal("unrelated similar file was deleted")
	}
	if _, ok := files.data[other]; !ok {
		t.Fatal("other file was deleted")
	}
}

func TestRecoverySerializationIsScopedByOriginAndActor(t *testing.T) {
	repository, _ := newTestCredentialRepository(t)
	origin, _ := CanonicalCoordinatorOrigin("https://coord.example")
	releaseA, err := repository.AcquireRecoveryScope(origin, 9)
	if err != nil {
		t.Fatal(err)
	}
	releaseB, err := repository.AcquireRecoveryScope(origin, 10)
	if err != nil {
		_ = releaseA()
		t.Fatalf("other actor scope was globally blocked: %v", err)
	}
	if err := releaseB(); err != nil {
		t.Fatal(err)
	}
	if err := releaseA(); err != nil {
		t.Fatal(err)
	}
	pendingPath, err := repository.pendingPath(origin, 9)
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range []string{origin.String(), "coord.example", sampleBundle().RecoveryID, sampleBundle().Control.ControlToken} {
		if strings.Contains(pendingPath, canary) {
			t.Fatalf("pending filename leaked canary %q: %s", canary, filepath.Base(pendingPath))
		}
	}
}

func TestConcurrentPendingWritesSerializeOnlySharedEntropyReader(t *testing.T) {
	files := newTestSecureFileOps()
	repository, err := NewProtectedCredentialRepository(CredentialRepositoryOptions{
		Directory: filepath.Join("C:", "Users", "test", "Pulsar"),
		Protector: &testProtector{}, Files: files,
		Random: bytes.NewReader(bytes.Repeat([]byte{0x31}, 128)),
	})
	if err != nil {
		t.Fatal(err)
	}
	origin, _ := CanonicalCoordinatorOrigin("https://coord.example")
	start := make(chan struct{})
	results := make(chan error, 2)
	for actorID := int64(9); actorID <= 10; actorID++ {
		actorID := actorID
		go func() {
			<-start
			results <- repository.CreatePendingUnsent(PendingRecoveryRecord{
				CanonicalCoordinatorOrigin: origin.String(), ActorID: actorID,
				RecoveryID:          "rec_0123456789abcdef0123456789abcdef",
				PendingControlToken: strings.Repeat(fmt.Sprintf("%x", actorID)[0:1], 64),
			})
		}()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	for actorID := int64(9); actorID <= 10; actorID++ {
		pending, err := repository.LoadPending(origin, actorID)
		if err != nil || pending == nil || pending.ActorID != actorID {
			t.Fatalf("actor %d pending=%#v err=%v", actorID, pending, err)
		}
	}
}

var _ io.Reader = bytes.NewReader(nil)
