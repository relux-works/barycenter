package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var (
	windowsEncryptedMediaTestNow    = time.Unix(1_800_000_000, 0)
	windowsEncryptedMediaTestFuture = windowsEncryptedMediaTestNow.Add(20 * 24 * time.Hour)
)

const (
	windowsEncryptedMediaThisDevice = "device_this_01"
	windowsEncryptedMediaNewDevice  = "device_new_02"
	windowsEncryptedMediaLostDevice = "device_lost_03"
	windowsEncryptedMediaObject     = "object_protected_01"
	windowsEncryptedMediaGrant      = "grant_history_01"
)

func windowsEncryptedMediaTestLabel() TargetsInboxLocalizedLabel {
	return TargetsInboxLocalizedLabel{Key: "fixture", EN: "Fixture device", RU: "Тестовое устройство"}
}

func windowsEncryptedMediaTestActions() []TargetsInboxActionCapability {
	actions := []WindowsEncryptedMediaCommandKind{
		WindowsEncryptedMediaRefresh, WindowsEncryptedMediaSelectPath,
		WindowsEncryptedMediaVerifyDevice, WindowsEncryptedMediaRevokeDevice,
		WindowsEncryptedMediaBeginDeviceTransfer, WindowsEncryptedMediaBeginUserHeldRecovery,
		WindowsEncryptedMediaConfirmUnsupportedExclusion, WindowsEncryptedMediaCreateHistoryGrant,
		WindowsEncryptedMediaRevokeHistoryGrant, WindowsEncryptedMediaReportMetadata,
		WindowsEncryptedMediaExportDecryptedEvidence,
	}
	result := make([]TargetsInboxActionCapability, 0, len(actions))
	for _, action := range actions {
		result = append(result, TargetsInboxActionCapability{Action: string(action), Label: windowsEncryptedMediaTestLabel()})
	}
	return result
}

func windowsEncryptedMediaTestDevice(id string, verification WindowsEncryptedMediaVerification, thisDevice, canRevoke bool) WindowsEncryptedMediaDevice {
	return WindowsEncryptedMediaDevice{ID: id, Label: windowsEncryptedMediaTestLabel(), Verification: verification,
		CurrentMember: true, ThisDevice: thisDevice, CanRevoke: canRevoke, Revision: 1}
}

func windowsEncryptedMediaReadySnapshot() WindowsEncryptedMediaSnapshot {
	draft := &WindowsEncryptedMediaHistoryGrantDraft{ObjectID: windowsEncryptedMediaObject, Title: "Selected voice",
		RecipientDeviceID: windowsEncryptedMediaNewDevice, FirstEpoch: 3, LastEpoch: 5,
		Mode: WindowsEncryptedMediaGrantOneTime, ExpiresAt: windowsEncryptedMediaTestFuture}
	report := &WindowsEncryptedMediaReportTarget{ObjectID: windowsEncryptedMediaObject, Title: "Selected voice",
		CanReportMetadata: true, CanExportDecryptedEvidence: true, DecryptedEvidenceReady: true,
		ConsentVersion: "report-evidence-v1"}
	return WindowsEncryptedMediaSnapshot{
		State: TargetsInboxReady, SelectedPath: WindowsEncryptedMediaProtectedClip,
		CapabilityAdvertised: true, ReviewedSuiteSelected: true, RuntimeWiringApproved: true,
		Ownership: WindowsEncryptedMediaOwnershipCrossProcessLock,
		Components: WindowsEncryptedMediaComponents{KeyStateReady: true, ProtectedSendReady: true,
			ProtectedPlaybackReady: true, ProtectedLiveReady: true, SameRepositoryWitness: true},
		ThisDeviceVerification: WindowsEncryptedMediaVerified,
		Membership:             WindowsEncryptedMediaMemberCurrent, Epoch: 9,
		Devices: []WindowsEncryptedMediaDevice{
			windowsEncryptedMediaTestDevice(windowsEncryptedMediaThisDevice, WindowsEncryptedMediaVerified, true, false),
			windowsEncryptedMediaTestDevice(windowsEncryptedMediaNewDevice, WindowsEncryptedMediaVerified, false, true),
			windowsEncryptedMediaTestDevice(windowsEncryptedMediaLostDevice, WindowsEncryptedMediaVerified, false, true),
		},
		UnsupportedExclusionConfirmed: true,
		RecoveryModes:                 []WindowsEncryptedMediaRecoveryMode{WindowsEncryptedMediaDeviceTransfer, WindowsEncryptedMediaUserHeldRecovery},
		RecoveryTargetDeviceID:        windowsEncryptedMediaNewDevice, HistoryRecoverable: true,
		HistoryGrants: []WindowsEncryptedMediaHistoryGrant{{ID: windowsEncryptedMediaGrant,
			Title: "Selected voice", ObjectID: windowsEncryptedMediaObject, RecipientDeviceID: windowsEncryptedMediaNewDevice,
			FirstEpoch: 3, LastEpoch: 5, Mode: WindowsEncryptedMediaGrantOneTime,
			ExpiresAt: windowsEncryptedMediaTestFuture, Status: WindowsEncryptedMediaGrantActive}},
		HistoryGrantDraft: draft, ReportTarget: report, Actions: windowsEncryptedMediaTestActions(),
	}
}

func TestWindowsEncryptedMediaProtectedStatusFailsClosed(t *testing.T) {
	model := NewWindowsEncryptedMediaModel(WindowsEncryptedMediaSnapshot{}, windowsEncryptedMediaTestNow)
	if got := model.Availability(WindowsEncryptedMediaPlaintext); got != WindowsEncryptedMediaAvailablePlaintext {
		t.Fatalf("plaintext availability=%s", got)
	}
	if got := model.Availability(WindowsEncryptedMediaProtectedClip); got != WindowsEncryptedMediaBlocked {
		t.Fatalf("initial protected availability=%s", got)
	}

	snapshot := windowsEncryptedMediaReadySnapshot()
	snapshot.Ownership = WindowsEncryptedMediaOwnershipUnattested
	model.Replace(snapshot, windowsEncryptedMediaTestNow)
	normalized := model.Snapshot()
	if normalized.SelectedPath != WindowsEncryptedMediaProtectedClip || normalized.RuntimeWiringApproved || normalized.CapabilityAdvertised {
		t.Fatalf("unsafe ownership normalized=%s", normalized.String())
	}
	if got := model.PathFailure(WindowsEncryptedMediaProtectedClip); got != "runtime_disabled" {
		t.Fatalf("failure=%q", got)
	}

	snapshot = windowsEncryptedMediaReadySnapshot()
	snapshot.Components.SameRepositoryWitness = false
	model.Replace(snapshot, windowsEncryptedMediaTestNow)
	if _, ok := model.DeviceTransferCommand(); ok {
		t.Fatal("repository mismatch authorized device transfer")
	}
	if _, ok := model.CreateHistoryGrantCommand(true, windowsEncryptedMediaTestNow); ok {
		t.Fatal("repository mismatch authorized history grant")
	}
	if _, ok := model.DecryptedEvidenceExportCommand(true); ok {
		t.Fatal("repository mismatch authorized evidence export")
	}
}

func TestWindowsEncryptedMediaUnsupportedTargetsNeverDowngrade(t *testing.T) {
	snapshot := windowsEncryptedMediaReadySnapshot()
	snapshot.UnsupportedRecipients = []WindowsEncryptedMediaUnsupportedRecipient{
		{ID: "device_legacy_04", Label: windowsEncryptedMediaTestLabel()},
		{ID: "device_legacy_04", Label: windowsEncryptedMediaTestLabel()},
	}
	snapshot.UnsupportedExclusionConfirmed = false
	model := NewWindowsEncryptedMediaModel(snapshot, windowsEncryptedMediaTestNow)
	if got := model.Snapshot(); got.SelectedPath != WindowsEncryptedMediaProtectedClip || len(got.UnsupportedRecipients) != 1 {
		t.Fatalf("normalized=%s", got.String())
	}
	if model.Availability(WindowsEncryptedMediaProtectedClip) != WindowsEncryptedMediaBlocked {
		t.Fatal("unsupported recipient did not block encrypted path")
	}
	if _, ok := model.SelectPathCommand(WindowsEncryptedMediaProtectedClip); ok {
		t.Fatal("blocked protected path remained selectable")
	}
	command, ok := model.UnsupportedExclusionCommand(true)
	if !ok || len(command.DeviceIDs) != 1 || command.DeviceIDs[0] != "device_legacy_04" {
		t.Fatalf("exclusion command=%s", command.String())
	}
	snapshot.UnsupportedExclusionConfirmed = true
	model.Replace(snapshot, windowsEncryptedMediaTestNow)
	if model.Availability(WindowsEncryptedMediaProtectedClip) != WindowsEncryptedMediaAvailableEncrypted {
		t.Fatal("confirmed exclusion did not restore encrypted availability")
	}
	if command, ok := model.SelectPathCommand(WindowsEncryptedMediaPlaintext); !ok || command.Path != WindowsEncryptedMediaPlaintext {
		t.Fatal("explicit plaintext selection was not available")
	}
}

func TestWindowsEncryptedMediaDeviceLifecycleCommandsFailClosed(t *testing.T) {
	snapshot := windowsEncryptedMediaReadySnapshot()
	snapshot.Devices = append(snapshot.Devices,
		windowsEncryptedMediaTestDevice("device_pending_05", WindowsEncryptedMediaUnverified, false, false))
	model := NewWindowsEncryptedMediaModel(snapshot, windowsEncryptedMediaTestNow)
	if command, ok := model.VerifyDeviceCommand("device_pending_05"); !ok || command.DeviceID != "device_pending_05" {
		t.Fatal("unverified current device could not be verified")
	}
	if _, ok := model.VerifyDeviceCommand(windowsEncryptedMediaNewDevice); ok {
		t.Fatal("verified device was offered verify action")
	}
	if _, ok := model.RevokeDeviceCommand(windowsEncryptedMediaLostDevice, false); ok {
		t.Fatal("unconfirmed device revoke was authorized")
	}
	if _, ok := model.RevokeDeviceCommand(windowsEncryptedMediaLostDevice, true); !ok {
		t.Fatal("confirmed lost-device revoke was rejected")
	}
	if _, ok := model.RevokeDeviceCommand(windowsEncryptedMediaThisDevice, true); ok {
		t.Fatal("non-revocable current device was authorized")
	}
	if command, ok := model.DeviceTransferCommand(); !ok || command.DeviceID != windowsEncryptedMediaNewDevice {
		t.Fatal("current-epoch device transfer missing")
	}
	if _, ok := model.UserHeldRecoveryCommand(false); ok {
		t.Fatal("unconfirmed user-held recovery was authorized")
	}
	if _, ok := model.UserHeldRecoveryCommand(true); !ok {
		t.Fatal("confirmed user-held recovery was rejected")
	}
	snapshot.Membership = WindowsEncryptedMediaRotationNeeded
	model.Replace(snapshot, windowsEncryptedMediaTestNow)
	if _, ok := model.DeviceTransferCommand(); ok || model.Availability(WindowsEncryptedMediaProtectedClip) != WindowsEncryptedMediaBlocked {
		t.Fatal("rotation-required membership did not fail closed")
	}
}

func TestWindowsEncryptedMediaHistoryAndReportConsentAreSeparate(t *testing.T) {
	model := NewWindowsEncryptedMediaModel(windowsEncryptedMediaReadySnapshot(), windowsEncryptedMediaTestNow)
	if _, ok := model.CreateHistoryGrantCommand(false, windowsEncryptedMediaTestNow); ok {
		t.Fatal("unconfirmed history grant was authorized")
	}
	grant, ok := model.CreateHistoryGrantCommand(true, windowsEncryptedMediaTestNow)
	if !ok || grant.ObjectID != windowsEncryptedMediaObject || grant.RecipientDeviceID != windowsEncryptedMediaNewDevice ||
		grant.FirstEpoch != 3 || grant.LastEpoch != 5 {
		t.Fatalf("history grant=%s", grant.String())
	}
	if _, ok := model.RevokeHistoryGrantCommand(windowsEncryptedMediaGrant, false); ok {
		t.Fatal("unconfirmed grant revoke was authorized")
	}
	if _, ok := model.RevokeHistoryGrantCommand(windowsEncryptedMediaGrant, true); !ok {
		t.Fatal("confirmed grant revoke was rejected")
	}
	if metadata, ok := model.MetadataReportCommand(); !ok || metadata.ObjectID != windowsEncryptedMediaObject {
		t.Fatal("metadata-only report missing")
	}
	if _, ok := model.DecryptedEvidenceExportCommand(false); ok {
		t.Fatal("unconfirmed evidence export was authorized")
	}
	if evidence, ok := model.DecryptedEvidenceExportCommand(true); !ok || evidence.ConsentVersion != "report-evidence-v1" {
		t.Fatal("confirmed evidence export missing")
	}
	snapshot := windowsEncryptedMediaReadySnapshot()
	snapshot.ReportTarget.CanExportDecryptedEvidence = false
	model.Replace(snapshot, windowsEncryptedMediaTestNow)
	if _, ok := model.MetadataReportCommand(); !ok {
		t.Fatal("metadata report was coupled to decrypted evidence permission")
	}
	if _, ok := model.DecryptedEvidenceExportCommand(true); ok {
		t.Fatal("denied evidence export was authorized")
	}
}

func TestWindowsEncryptedMediaDescriptionsAndPresentationAreRedactedAndAccessible(t *testing.T) {
	model := NewWindowsEncryptedMediaModel(windowsEncryptedMediaReadySnapshot(), windowsEncryptedMediaTestNow)
	snapshot := model.Snapshot()
	rendered := []string{
		snapshot.String(), snapshot.Devices[0].String(), snapshot.HistoryGrants[0].String(),
		snapshot.ReportTarget.String(), (WindowsEncryptedMediaCommand{Kind: WindowsEncryptedMediaExportDecryptedEvidence,
			ObjectID: windowsEncryptedMediaObject, ConsentVersion: "report-evidence-v1"}).String(),
	}
	for _, value := range rendered {
		for _, secret := range []string{windowsEncryptedMediaThisDevice, windowsEncryptedMediaNewDevice, windowsEncryptedMediaObject, windowsEncryptedMediaGrant} {
			if strings.Contains(value, secret) {
				t.Fatalf("description leaked %q in %q", secret, value)
			}
		}
	}
	for _, locale := range []ShellLocale{ShellEnglish, ShellRussian} {
		presentation := model.Presentation(locale)
		if len(presentation.PathRows) != 4 || len(presentation.DeviceRows) != 3 ||
			strings.TrimSpace(presentation.AccessibleSummary) == "" || strings.TrimSpace(presentation.ReportSummary) == "" ||
			strings.TrimSpace(presentation.RecoverySummary) == "" {
			t.Fatalf("incomplete accessible presentation for %s: %+v", locale, presentation)
		}
		for _, row := range presentation.PathRows {
			if strings.TrimSpace(row.AccessibleName) == "" || strings.TrimSpace(row.AccessibleValue) == "" {
				t.Fatalf("inaccessible path row: %+v", row)
			}
		}
		presentationText := presentation.AccessibleSummary + presentation.ReportSummary + presentation.RecoverySummary
		for _, secret := range []string{windowsEncryptedMediaThisDevice, windowsEncryptedMediaNewDevice, windowsEncryptedMediaObject, windowsEncryptedMediaGrant} {
			if strings.Contains(presentationText, secret) {
				t.Fatalf("presentation leaked identifier %q", secret)
			}
		}
	}
}

type windowsEncryptedMediaApprovedDeriver struct{ windowsE2EELiveFixtureDeriver }

func (*windowsEncryptedMediaApprovedDeriver) ProductionApproved() bool { return true }

func TestWindowsEncryptedMediaCompositionUsesOneRepositoryAndStaysRuntimeDark(t *testing.T) {
	fixture := newWindowsE2EEFixture(t, 0x7a)
	root := t.TempDir()
	ciphertext := filepath.Join(root, "ciphertext")
	plaintext := filepath.Join(root, "plaintext")
	cache := filepath.Join(root, "cache")
	for _, directory := range []string{ciphertext, plaintext, cache} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	composition, err := NewWindowsEncryptedMediaClientPathComposition(WindowsEncryptedMediaClientPathOptions{
		KeyState: fixture.repository, Sealer: newWindowsProtectedFixtureSealer(),
		Uploader: &windowsProtectedFixtureUploader{}, Opener: &windowsProtectedPlaybackFixtureOpener{},
		PlaybackTransport: &windowsProtectedPlaybackTransport{}, LiveDerivation: &windowsEncryptedMediaApprovedDeriver{},
		LiveAuthorization: &windowsE2EELiveAuthorizationBox{}, CiphertextRoot: ciphertext,
		PlaintextDraftRoot: plaintext, PlaybackCacheRoot: cache,
		PlaybackCacheSecret: []byte("0123456789abcdef"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if composition.KeyState != fixture.repository || composition.ProtectedSend.keyState != fixture.repository ||
		composition.ProtectedPlayback.keyState != fixture.repository || composition.ProtectedLive.keyState != fixture.repository {
		t.Fatal("composition diverged from the single key-state repository")
	}
	dark := composition.ProductionDarkModel(windowsEncryptedMediaTestNow).Snapshot()
	if dark.State != TargetsInboxLoading || dark.CapabilityAdvertised || dark.RuntimeWiringApproved || dark.ReviewedSuiteSelected ||
		!dark.Components.SameRepositoryWitness || dark.Ownership != WindowsEncryptedMediaOwnershipCrossProcessLock {
		t.Fatalf("production-dark snapshot=%s", dark.String())
	}
	mainSource, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !windowsEncryptedMediaSourceIsRuntimeDark(string(mainSource)) {
		t.Fatal("encrypted-media composition was wired into main.go")
	}
}
