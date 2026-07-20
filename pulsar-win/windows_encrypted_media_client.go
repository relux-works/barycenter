package main

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type WindowsEncryptedMediaPath string

const (
	WindowsEncryptedMediaPlaintext      WindowsEncryptedMediaPath = "plaintext"
	WindowsEncryptedMediaProtectedClip  WindowsEncryptedMediaPath = "protected_clip"
	WindowsEncryptedMediaProtectedTrack WindowsEncryptedMediaPath = "protected_track"
	WindowsEncryptedMediaProtectedLive  WindowsEncryptedMediaPath = "protected_live"
)

func (path WindowsEncryptedMediaPath) Protected() bool {
	return path != WindowsEncryptedMediaPlaintext
}

type WindowsEncryptedMediaVerification string

const (
	WindowsEncryptedMediaVerified   WindowsEncryptedMediaVerification = "verified"
	WindowsEncryptedMediaUnverified WindowsEncryptedMediaVerification = "unverified"
	WindowsEncryptedMediaRevoked    WindowsEncryptedMediaVerification = "revoked"
)

type WindowsEncryptedMediaMembership string

const (
	WindowsEncryptedMediaMemberCurrent    WindowsEncryptedMediaMembership = "current"
	WindowsEncryptedMediaRotationNeeded   WindowsEncryptedMediaMembership = "rotation_required"
	WindowsEncryptedMediaMemberRemoved    WindowsEncryptedMediaMembership = "removed"
	WindowsEncryptedMediaMembershipForked WindowsEncryptedMediaMembership = "forked"
)

type WindowsEncryptedMediaOwnership string

const (
	WindowsEncryptedMediaOwnershipUnattested       WindowsEncryptedMediaOwnership = "unattested"
	WindowsEncryptedMediaOwnershipSingleInstance   WindowsEncryptedMediaOwnership = "single_application_instance"
	WindowsEncryptedMediaOwnershipCrossProcessLock WindowsEncryptedMediaOwnership = "cross_process_serialized"
)

func (ownership WindowsEncryptedMediaOwnership) Safe() bool {
	return ownership == WindowsEncryptedMediaOwnershipSingleInstance ||
		ownership == WindowsEncryptedMediaOwnershipCrossProcessLock
}

type WindowsEncryptedMediaAvailability string

const (
	WindowsEncryptedMediaAvailablePlaintext WindowsEncryptedMediaAvailability = "plaintext"
	WindowsEncryptedMediaAvailableEncrypted WindowsEncryptedMediaAvailability = "encrypted"
	WindowsEncryptedMediaBlocked            WindowsEncryptedMediaAvailability = "blocked"
)

type WindowsEncryptedMediaGrantMode string

const (
	WindowsEncryptedMediaGrantOneTime   WindowsEncryptedMediaGrantMode = "one_time"
	WindowsEncryptedMediaGrantTimeBound WindowsEncryptedMediaGrantMode = "time_bound"
)

type WindowsEncryptedMediaGrantStatus string

const (
	WindowsEncryptedMediaGrantActive  WindowsEncryptedMediaGrantStatus = "active"
	WindowsEncryptedMediaGrantExpired WindowsEncryptedMediaGrantStatus = "expired"
	WindowsEncryptedMediaGrantRevoked WindowsEncryptedMediaGrantStatus = "revoked"
)

type WindowsEncryptedMediaRecoveryMode string

const (
	WindowsEncryptedMediaDeviceTransfer   WindowsEncryptedMediaRecoveryMode = "device_transfer"
	WindowsEncryptedMediaUserHeldRecovery WindowsEncryptedMediaRecoveryMode = "user_held_recovery"
)

type WindowsEncryptedMediaComponents struct {
	KeyStateReady          bool
	ProtectedSendReady     bool
	ProtectedPlaybackReady bool
	ProtectedLiveReady     bool
	SameRepositoryWitness  bool
}

type WindowsEncryptedMediaDevice struct {
	ID            string
	Label         TargetsInboxLocalizedLabel
	Verification  WindowsEncryptedMediaVerification
	CurrentMember bool
	ThisDevice    bool
	CanRevoke     bool
	Revision      uint64
}

func (WindowsEncryptedMediaDevice) String() string         { return "WindowsEncryptedMediaDevice{<opaque>}" }
func (value WindowsEncryptedMediaDevice) GoString() string { return value.String() }

type WindowsEncryptedMediaUnsupportedRecipient struct {
	ID    string
	Label TargetsInboxLocalizedLabel
}

func (WindowsEncryptedMediaUnsupportedRecipient) String() string {
	return "WindowsEncryptedMediaUnsupportedRecipient{<opaque>}"
}
func (value WindowsEncryptedMediaUnsupportedRecipient) GoString() string { return value.String() }

type WindowsEncryptedMediaHistoryGrant struct {
	ID                string
	Title             string
	ObjectID          string
	RecipientDeviceID string
	FirstEpoch        uint64
	LastEpoch         uint64
	Mode              WindowsEncryptedMediaGrantMode
	ExpiresAt         time.Time
	Status            WindowsEncryptedMediaGrantStatus
}

func (WindowsEncryptedMediaHistoryGrant) String() string {
	return "WindowsEncryptedMediaHistoryGrant{<opaque>}"
}
func (value WindowsEncryptedMediaHistoryGrant) GoString() string { return value.String() }

type WindowsEncryptedMediaHistoryGrantDraft struct {
	ObjectID          string
	Title             string
	RecipientDeviceID string
	FirstEpoch        uint64
	LastEpoch         uint64
	Mode              WindowsEncryptedMediaGrantMode
	ExpiresAt         time.Time
}

func (WindowsEncryptedMediaHistoryGrantDraft) String() string {
	return "WindowsEncryptedMediaHistoryGrantDraft{<opaque>}"
}
func (value WindowsEncryptedMediaHistoryGrantDraft) GoString() string { return value.String() }

type WindowsEncryptedMediaReportTarget struct {
	ObjectID                   string
	Title                      string
	CanReportMetadata          bool
	CanExportDecryptedEvidence bool
	DecryptedEvidenceReady     bool
	ConsentVersion             string
}

func (WindowsEncryptedMediaReportTarget) String() string {
	return "WindowsEncryptedMediaReportTarget{<opaque>}"
}
func (value WindowsEncryptedMediaReportTarget) GoString() string { return value.String() }

type WindowsEncryptedMediaSnapshot struct {
	State                         TargetsInboxSurfaceState
	SelectedPath                  WindowsEncryptedMediaPath
	CapabilityAdvertised          bool
	ReviewedSuiteSelected         bool
	RuntimeWiringApproved         bool
	Ownership                     WindowsEncryptedMediaOwnership
	Components                    WindowsEncryptedMediaComponents
	ThisDeviceVerification        WindowsEncryptedMediaVerification
	Membership                    WindowsEncryptedMediaMembership
	Epoch                         uint64
	Devices                       []WindowsEncryptedMediaDevice
	UnsupportedRecipients         []WindowsEncryptedMediaUnsupportedRecipient
	UnsupportedExclusionConfirmed bool
	RecoveryModes                 []WindowsEncryptedMediaRecoveryMode
	RecoveryTargetDeviceID        string
	HistoryRecoverable            bool
	HistoryGrants                 []WindowsEncryptedMediaHistoryGrant
	HistoryGrantDraft             *WindowsEncryptedMediaHistoryGrantDraft
	ReportTarget                  *WindowsEncryptedMediaReportTarget
	Actions                       []TargetsInboxActionCapability
	ActionOutcome                 TargetsInboxLocalizedLabel
	ActionFailure                 TargetsInboxLocalizedLabel
	CommandInFlight               bool
}

func (WindowsEncryptedMediaSnapshot) String() string {
	return "WindowsEncryptedMediaSnapshot{<opaque>}"
}
func (value WindowsEncryptedMediaSnapshot) GoString() string { return value.String() }

type WindowsEncryptedMediaCommandKind string

const (
	WindowsEncryptedMediaRefresh                     WindowsEncryptedMediaCommandKind = "refresh"
	WindowsEncryptedMediaSelectPath                  WindowsEncryptedMediaCommandKind = "select_path"
	WindowsEncryptedMediaVerifyDevice                WindowsEncryptedMediaCommandKind = "verify_device"
	WindowsEncryptedMediaRevokeDevice                WindowsEncryptedMediaCommandKind = "revoke_device"
	WindowsEncryptedMediaBeginDeviceTransfer         WindowsEncryptedMediaCommandKind = "device_transfer"
	WindowsEncryptedMediaBeginUserHeldRecovery       WindowsEncryptedMediaCommandKind = "user_held_recovery"
	WindowsEncryptedMediaConfirmUnsupportedExclusion WindowsEncryptedMediaCommandKind = "confirm_unsupported_exclusion"
	WindowsEncryptedMediaCreateHistoryGrant          WindowsEncryptedMediaCommandKind = "create_history_grant"
	WindowsEncryptedMediaRevokeHistoryGrant          WindowsEncryptedMediaCommandKind = "revoke_history_grant"
	WindowsEncryptedMediaReportMetadata              WindowsEncryptedMediaCommandKind = "report_metadata"
	WindowsEncryptedMediaExportDecryptedEvidence     WindowsEncryptedMediaCommandKind = "export_decrypted_evidence"
)

type WindowsEncryptedMediaCommand struct {
	Kind              WindowsEncryptedMediaCommandKind
	Path              WindowsEncryptedMediaPath
	DeviceID          string
	DeviceIDs         []string
	ObjectID          string
	GrantID           string
	RecipientDeviceID string
	FirstEpoch        uint64
	LastEpoch         uint64
	GrantMode         WindowsEncryptedMediaGrantMode
	ExpiresAt         time.Time
	ConsentVersion    string
}

func (command WindowsEncryptedMediaCommand) String() string {
	if command.Kind == WindowsEncryptedMediaSelectPath {
		return "WindowsEncryptedMediaCommand{" + string(command.Kind) + "," + string(command.Path) + "}"
	}
	return "WindowsEncryptedMediaCommand{" + string(command.Kind) + ",<opaque>}"
}
func (command WindowsEncryptedMediaCommand) GoString() string { return command.String() }

type WindowsEncryptedMediaModel struct {
	mu       sync.RWMutex
	snapshot WindowsEncryptedMediaSnapshot
}

const (
	windowsEncryptedMediaMaximumDevices       = 64
	windowsEncryptedMediaMaximumHistoryGrants = 100
	windowsEncryptedMediaMaximumHistoryDays   = 30
)

var (
	windowsEncryptedMediaIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)
	windowsEncryptedMediaActionPattern     = regexp.MustCompile(`^[a-z_]{1,64}$`)
	windowsEncryptedMediaConsentPattern    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
)

func NewWindowsEncryptedMediaModel(snapshot WindowsEncryptedMediaSnapshot, now time.Time) *WindowsEncryptedMediaModel {
	model := &WindowsEncryptedMediaModel{}
	model.Replace(snapshot, now)
	return model
}

func (model *WindowsEncryptedMediaModel) Replace(snapshot WindowsEncryptedMediaSnapshot, now time.Time) {
	if model == nil {
		return
	}
	normalized := normalizeWindowsEncryptedMediaSnapshot(snapshot, now)
	model.mu.Lock()
	model.snapshot = normalized
	model.mu.Unlock()
}

func (model *WindowsEncryptedMediaModel) Snapshot() WindowsEncryptedMediaSnapshot {
	if model == nil {
		return WindowsEncryptedMediaSnapshot{State: TargetsInboxCoordinatorError}
	}
	model.mu.RLock()
	defer model.mu.RUnlock()
	return cloneWindowsEncryptedMediaSnapshot(model.snapshot)
}

func normalizeWindowsEncryptedMediaSnapshot(snapshot WindowsEncryptedMediaSnapshot, now time.Time) WindowsEncryptedMediaSnapshot {
	snapshot = cloneWindowsEncryptedMediaSnapshot(snapshot)
	if now.IsZero() {
		now = time.Now()
	}
	switch snapshot.State {
	case TargetsInboxLoading, TargetsInboxReady, TargetsInboxStale, TargetsInboxOffline, TargetsInboxCoordinatorError:
	default:
		snapshot.State = TargetsInboxCoordinatorError
	}
	switch snapshot.SelectedPath {
	case WindowsEncryptedMediaPlaintext, WindowsEncryptedMediaProtectedClip,
		WindowsEncryptedMediaProtectedTrack, WindowsEncryptedMediaProtectedLive:
	default:
		snapshot.SelectedPath = WindowsEncryptedMediaPlaintext
	}

	seenDevices := map[string]struct{}{}
	devices := make([]WindowsEncryptedMediaDevice, 0, len(snapshot.Devices))
	for _, device := range snapshot.Devices {
		if !windowsEncryptedMediaIdentifierPattern.MatchString(device.ID) || device.Revision == 0 {
			continue
		}
		if _, duplicate := seenDevices[device.ID]; duplicate {
			continue
		}
		seenDevices[device.ID] = struct{}{}
		devices = append(devices, device)
		if len(devices) == windowsEncryptedMediaMaximumDevices {
			break
		}
	}
	snapshot.Devices = devices

	seenRecipients := map[string]struct{}{}
	recipients := make([]WindowsEncryptedMediaUnsupportedRecipient, 0, len(snapshot.UnsupportedRecipients))
	for _, recipient := range snapshot.UnsupportedRecipients {
		if !windowsEncryptedMediaIdentifierPattern.MatchString(recipient.ID) {
			continue
		}
		if _, duplicate := seenRecipients[recipient.ID]; duplicate {
			continue
		}
		seenRecipients[recipient.ID] = struct{}{}
		recipients = append(recipients, recipient)
		if len(recipients) == windowsEncryptedMediaMaximumDevices {
			break
		}
	}
	snapshot.UnsupportedRecipients = recipients

	modes := map[WindowsEncryptedMediaRecoveryMode]struct{}{}
	for _, mode := range snapshot.RecoveryModes {
		if mode == WindowsEncryptedMediaDeviceTransfer || mode == WindowsEncryptedMediaUserHeldRecovery {
			modes[mode] = struct{}{}
		}
	}
	snapshot.RecoveryModes = snapshot.RecoveryModes[:0]
	for mode := range modes {
		snapshot.RecoveryModes = append(snapshot.RecoveryModes, mode)
	}
	sort.Slice(snapshot.RecoveryModes, func(i, j int) bool { return snapshot.RecoveryModes[i] < snapshot.RecoveryModes[j] })

	seenGrants := map[string]struct{}{}
	grants := make([]WindowsEncryptedMediaHistoryGrant, 0, len(snapshot.HistoryGrants))
	for _, grant := range snapshot.HistoryGrants {
		if !validWindowsEncryptedMediaGrant(grant, now) {
			continue
		}
		if _, duplicate := seenGrants[grant.ID]; duplicate {
			continue
		}
		seenGrants[grant.ID] = struct{}{}
		grants = append(grants, grant)
		if len(grants) == windowsEncryptedMediaMaximumHistoryGrants {
			break
		}
	}
	snapshot.HistoryGrants = grants

	seenActions := map[string]struct{}{}
	actions := make([]TargetsInboxActionCapability, 0, len(snapshot.Actions))
	for _, action := range snapshot.Actions {
		if !windowsEncryptedMediaActionPattern.MatchString(action.Action) {
			continue
		}
		if _, duplicate := seenActions[action.Action]; duplicate {
			continue
		}
		seenActions[action.Action] = struct{}{}
		actions = append(actions, action)
	}
	snapshot.Actions = actions

	if snapshot.Epoch == 0 || snapshot.ThisDeviceVerification != WindowsEncryptedMediaVerified ||
		snapshot.Membership != WindowsEncryptedMediaMemberCurrent {
		snapshot.UnsupportedExclusionConfirmed = false
	}
	if !snapshot.Ownership.Safe() || !snapshot.Components.SameRepositoryWitness {
		snapshot.RuntimeWiringApproved = false
		snapshot.CapabilityAdvertised = false
	}
	if !snapshot.RuntimeWiringApproved || !snapshot.ReviewedSuiteSelected {
		snapshot.CapabilityAdvertised = false
	}
	if snapshot.State != TargetsInboxReady {
		snapshot.CommandInFlight = false
	}
	if snapshot.RecoveryTargetDeviceID != "" {
		if _, exists := seenDevices[snapshot.RecoveryTargetDeviceID]; !exists {
			snapshot.RecoveryTargetDeviceID = ""
		}
	}
	if snapshot.HistoryGrantDraft != nil &&
		!validWindowsEncryptedMediaHistoryDraft(*snapshot.HistoryGrantDraft, snapshot.Devices, now) {
		snapshot.HistoryGrantDraft = nil
	}
	if snapshot.ReportTarget != nil &&
		(!windowsEncryptedMediaIdentifierPattern.MatchString(snapshot.ReportTarget.ObjectID) ||
			!windowsEncryptedMediaConsentPattern.MatchString(snapshot.ReportTarget.ConsentVersion)) {
		snapshot.ReportTarget = nil
	}
	return snapshot
}

func cloneWindowsEncryptedMediaSnapshot(snapshot WindowsEncryptedMediaSnapshot) WindowsEncryptedMediaSnapshot {
	snapshot.Devices = append([]WindowsEncryptedMediaDevice(nil), snapshot.Devices...)
	snapshot.UnsupportedRecipients = append([]WindowsEncryptedMediaUnsupportedRecipient(nil), snapshot.UnsupportedRecipients...)
	snapshot.RecoveryModes = append([]WindowsEncryptedMediaRecoveryMode(nil), snapshot.RecoveryModes...)
	snapshot.HistoryGrants = append([]WindowsEncryptedMediaHistoryGrant(nil), snapshot.HistoryGrants...)
	snapshot.Actions = append([]TargetsInboxActionCapability(nil), snapshot.Actions...)
	if snapshot.HistoryGrantDraft != nil {
		copy := *snapshot.HistoryGrantDraft
		snapshot.HistoryGrantDraft = &copy
	}
	if snapshot.ReportTarget != nil {
		copy := *snapshot.ReportTarget
		snapshot.ReportTarget = &copy
	}
	return snapshot
}

func validWindowsEncryptedMediaGrant(grant WindowsEncryptedMediaHistoryGrant, now time.Time) bool {
	return windowsEncryptedMediaIdentifierPattern.MatchString(grant.ID) &&
		windowsEncryptedMediaIdentifierPattern.MatchString(grant.ObjectID) &&
		windowsEncryptedMediaIdentifierPattern.MatchString(grant.RecipientDeviceID) &&
		grant.FirstEpoch > 0 && grant.LastEpoch >= grant.FirstEpoch &&
		(grant.Status != WindowsEncryptedMediaGrantActive || grant.ExpiresAt.After(now))
}

func validWindowsEncryptedMediaHistoryDraft(draft WindowsEncryptedMediaHistoryGrantDraft, devices []WindowsEncryptedMediaDevice, now time.Time) bool {
	if !windowsEncryptedMediaIdentifierPattern.MatchString(draft.ObjectID) ||
		!windowsEncryptedMediaIdentifierPattern.MatchString(draft.RecipientDeviceID) ||
		draft.FirstEpoch == 0 || draft.LastEpoch < draft.FirstEpoch || !draft.ExpiresAt.After(now) ||
		draft.ExpiresAt.Sub(now) > windowsEncryptedMediaMaximumHistoryDays*24*time.Hour ||
		(draft.Mode != WindowsEncryptedMediaGrantOneTime && draft.Mode != WindowsEncryptedMediaGrantTimeBound) {
		return false
	}
	for _, device := range devices {
		if device.ID == draft.RecipientDeviceID {
			return device.Verification == WindowsEncryptedMediaVerified && device.CurrentMember
		}
	}
	return false
}

func (model *WindowsEncryptedMediaModel) Availability(path WindowsEncryptedMediaPath) WindowsEncryptedMediaAvailability {
	if !path.Protected() {
		return WindowsEncryptedMediaAvailablePlaintext
	}
	snapshot := model.Snapshot()
	if !windowsEncryptedMediaProtectedFoundationReady(snapshot) {
		return WindowsEncryptedMediaBlocked
	}
	switch path {
	case WindowsEncryptedMediaProtectedClip, WindowsEncryptedMediaProtectedTrack:
		if snapshot.Components.ProtectedSendReady && snapshot.Components.ProtectedPlaybackReady {
			return WindowsEncryptedMediaAvailableEncrypted
		}
	case WindowsEncryptedMediaProtectedLive:
		if snapshot.Components.ProtectedLiveReady {
			return WindowsEncryptedMediaAvailableEncrypted
		}
	}
	return WindowsEncryptedMediaBlocked
}

func (model *WindowsEncryptedMediaModel) PathFailure(path WindowsEncryptedMediaPath) string {
	if !path.Protected() {
		return ""
	}
	snapshot := model.Snapshot()
	if snapshot.State != TargetsInboxReady {
		return "surface_not_ready"
	}
	if !snapshot.RuntimeWiringApproved {
		return "runtime_disabled"
	}
	if !snapshot.Ownership.Safe() || !snapshot.Components.SameRepositoryWitness {
		return "ownership_unattested"
	}
	if !snapshot.ReviewedSuiteSelected || !snapshot.CapabilityAdvertised {
		return "capability_unavailable"
	}
	if !snapshot.Components.KeyStateReady {
		return "secure_key_state_unavailable"
	}
	if snapshot.ThisDeviceVerification != WindowsEncryptedMediaVerified {
		return "device_unverified"
	}
	if snapshot.Membership != WindowsEncryptedMediaMemberCurrent || snapshot.Epoch == 0 {
		return "membership_stale"
	}
	if len(snapshot.UnsupportedRecipients) > 0 && !snapshot.UnsupportedExclusionConfirmed {
		return "unsupported_recipients_require_choice"
	}
	if (path == WindowsEncryptedMediaProtectedClip || path == WindowsEncryptedMediaProtectedTrack) &&
		!snapshot.Components.ProtectedSendReady {
		return "protected_send_unavailable"
	}
	if (path == WindowsEncryptedMediaProtectedClip || path == WindowsEncryptedMediaProtectedTrack) &&
		!snapshot.Components.ProtectedPlaybackReady {
		return "protected_playback_unavailable"
	}
	if path == WindowsEncryptedMediaProtectedLive && !snapshot.Components.ProtectedLiveReady {
		return "protected_live_unavailable"
	}
	return ""
}

func windowsEncryptedMediaReadyForCommand(snapshot WindowsEncryptedMediaSnapshot) bool {
	return snapshot.State == TargetsInboxReady && !snapshot.CommandInFlight
}

func windowsEncryptedMediaRecoveryFoundationReady(snapshot WindowsEncryptedMediaSnapshot) bool {
	return windowsEncryptedMediaReadyForCommand(snapshot) && snapshot.RuntimeWiringApproved &&
		snapshot.ReviewedSuiteSelected && snapshot.CapabilityAdvertised &&
		snapshot.Components.KeyStateReady && snapshot.Components.SameRepositoryWitness && snapshot.Ownership.Safe()
}

func windowsEncryptedMediaProtectedCryptographyReady(snapshot WindowsEncryptedMediaSnapshot) bool {
	return windowsEncryptedMediaRecoveryFoundationReady(snapshot) &&
		snapshot.ThisDeviceVerification == WindowsEncryptedMediaVerified &&
		snapshot.Membership == WindowsEncryptedMediaMemberCurrent && snapshot.Epoch > 0
}

func windowsEncryptedMediaProtectedFoundationReady(snapshot WindowsEncryptedMediaSnapshot) bool {
	return windowsEncryptedMediaProtectedCryptographyReady(snapshot) &&
		(len(snapshot.UnsupportedRecipients) == 0 || snapshot.UnsupportedExclusionConfirmed)
}

func windowsEncryptedMediaHasAction(snapshot WindowsEncryptedMediaSnapshot, name string) bool {
	for _, action := range snapshot.Actions {
		if action.Action == name {
			return true
		}
	}
	return false
}

func (model *WindowsEncryptedMediaModel) RefreshCommand() (WindowsEncryptedMediaCommand, bool) {
	snapshot := model.Snapshot()
	return WindowsEncryptedMediaCommand{Kind: WindowsEncryptedMediaRefresh},
		!snapshot.CommandInFlight && windowsEncryptedMediaHasAction(snapshot, string(WindowsEncryptedMediaRefresh))
}

func (model *WindowsEncryptedMediaModel) SelectPathCommand(path WindowsEncryptedMediaPath) (WindowsEncryptedMediaCommand, bool) {
	snapshot := model.Snapshot()
	if !windowsEncryptedMediaReadyForCommand(snapshot) || !windowsEncryptedMediaHasAction(snapshot, string(WindowsEncryptedMediaSelectPath)) ||
		(path.Protected() && model.Availability(path) != WindowsEncryptedMediaAvailableEncrypted) {
		return WindowsEncryptedMediaCommand{}, false
	}
	return WindowsEncryptedMediaCommand{Kind: WindowsEncryptedMediaSelectPath, Path: path}, true
}

func (model *WindowsEncryptedMediaModel) VerifyDeviceCommand(id string) (WindowsEncryptedMediaCommand, bool) {
	snapshot := model.Snapshot()
	if !windowsEncryptedMediaReadyForCommand(snapshot) || !windowsEncryptedMediaHasAction(snapshot, string(WindowsEncryptedMediaVerifyDevice)) {
		return WindowsEncryptedMediaCommand{}, false
	}
	for _, device := range snapshot.Devices {
		if device.ID == id && device.Verification == WindowsEncryptedMediaUnverified && device.CurrentMember {
			return WindowsEncryptedMediaCommand{Kind: WindowsEncryptedMediaVerifyDevice, DeviceID: id}, true
		}
	}
	return WindowsEncryptedMediaCommand{}, false
}

func (model *WindowsEncryptedMediaModel) RevokeDeviceCommand(id string, confirmed bool) (WindowsEncryptedMediaCommand, bool) {
	snapshot := model.Snapshot()
	if !confirmed || !windowsEncryptedMediaReadyForCommand(snapshot) || !windowsEncryptedMediaHasAction(snapshot, string(WindowsEncryptedMediaRevokeDevice)) {
		return WindowsEncryptedMediaCommand{}, false
	}
	for _, device := range snapshot.Devices {
		if device.ID == id && device.Verification == WindowsEncryptedMediaVerified && device.CanRevoke {
			return WindowsEncryptedMediaCommand{Kind: WindowsEncryptedMediaRevokeDevice, DeviceID: id}, true
		}
	}
	return WindowsEncryptedMediaCommand{}, false
}

func (model *WindowsEncryptedMediaModel) DeviceTransferCommand() (WindowsEncryptedMediaCommand, bool) {
	snapshot := model.Snapshot()
	if !windowsEncryptedMediaProtectedCryptographyReady(snapshot) ||
		!windowsEncryptedMediaHasAction(snapshot, string(WindowsEncryptedMediaBeginDeviceTransfer)) ||
		!containsWindowsEncryptedMediaRecoveryMode(snapshot.RecoveryModes, WindowsEncryptedMediaDeviceTransfer) {
		return WindowsEncryptedMediaCommand{}, false
	}
	for _, device := range snapshot.Devices {
		if device.ID == snapshot.RecoveryTargetDeviceID && device.Verification == WindowsEncryptedMediaVerified &&
			device.CurrentMember && !device.ThisDevice {
			return WindowsEncryptedMediaCommand{Kind: WindowsEncryptedMediaBeginDeviceTransfer, DeviceID: device.ID}, true
		}
	}
	return WindowsEncryptedMediaCommand{}, false
}

func (model *WindowsEncryptedMediaModel) UserHeldRecoveryCommand(confirmed bool) (WindowsEncryptedMediaCommand, bool) {
	snapshot := model.Snapshot()
	if !confirmed || !windowsEncryptedMediaRecoveryFoundationReady(snapshot) ||
		!windowsEncryptedMediaHasAction(snapshot, string(WindowsEncryptedMediaBeginUserHeldRecovery)) ||
		!containsWindowsEncryptedMediaRecoveryMode(snapshot.RecoveryModes, WindowsEncryptedMediaUserHeldRecovery) {
		return WindowsEncryptedMediaCommand{}, false
	}
	return WindowsEncryptedMediaCommand{Kind: WindowsEncryptedMediaBeginUserHeldRecovery}, true
}

func (model *WindowsEncryptedMediaModel) UnsupportedExclusionCommand(confirmed bool) (WindowsEncryptedMediaCommand, bool) {
	snapshot := model.Snapshot()
	if !confirmed || !windowsEncryptedMediaReadyForCommand(snapshot) || len(snapshot.UnsupportedRecipients) == 0 ||
		!windowsEncryptedMediaHasAction(snapshot, string(WindowsEncryptedMediaConfirmUnsupportedExclusion)) {
		return WindowsEncryptedMediaCommand{}, false
	}
	ids := make([]string, 0, len(snapshot.UnsupportedRecipients))
	for _, recipient := range snapshot.UnsupportedRecipients {
		ids = append(ids, recipient.ID)
	}
	return WindowsEncryptedMediaCommand{Kind: WindowsEncryptedMediaConfirmUnsupportedExclusion, DeviceIDs: ids}, true
}

func (model *WindowsEncryptedMediaModel) CreateHistoryGrantCommand(confirmed bool, now time.Time) (WindowsEncryptedMediaCommand, bool) {
	snapshot := model.Snapshot()
	if !confirmed || !windowsEncryptedMediaProtectedCryptographyReady(snapshot) || !snapshot.HistoryRecoverable ||
		snapshot.HistoryGrantDraft == nil || !windowsEncryptedMediaHasAction(snapshot, string(WindowsEncryptedMediaCreateHistoryGrant)) ||
		!validWindowsEncryptedMediaHistoryDraft(*snapshot.HistoryGrantDraft, snapshot.Devices, now) {
		return WindowsEncryptedMediaCommand{}, false
	}
	draft := snapshot.HistoryGrantDraft
	return WindowsEncryptedMediaCommand{Kind: WindowsEncryptedMediaCreateHistoryGrant, ObjectID: draft.ObjectID,
		RecipientDeviceID: draft.RecipientDeviceID, FirstEpoch: draft.FirstEpoch, LastEpoch: draft.LastEpoch,
		GrantMode: draft.Mode, ExpiresAt: draft.ExpiresAt}, true
}

func (model *WindowsEncryptedMediaModel) RevokeHistoryGrantCommand(id string, confirmed bool) (WindowsEncryptedMediaCommand, bool) {
	snapshot := model.Snapshot()
	if !confirmed || !windowsEncryptedMediaReadyForCommand(snapshot) ||
		!windowsEncryptedMediaHasAction(snapshot, string(WindowsEncryptedMediaRevokeHistoryGrant)) {
		return WindowsEncryptedMediaCommand{}, false
	}
	for _, grant := range snapshot.HistoryGrants {
		if grant.ID == id && grant.Status == WindowsEncryptedMediaGrantActive {
			return WindowsEncryptedMediaCommand{Kind: WindowsEncryptedMediaRevokeHistoryGrant, GrantID: id}, true
		}
	}
	return WindowsEncryptedMediaCommand{}, false
}

func (model *WindowsEncryptedMediaModel) MetadataReportCommand() (WindowsEncryptedMediaCommand, bool) {
	snapshot := model.Snapshot()
	if !windowsEncryptedMediaReadyForCommand(snapshot) || snapshot.ReportTarget == nil || !snapshot.ReportTarget.CanReportMetadata ||
		!windowsEncryptedMediaHasAction(snapshot, string(WindowsEncryptedMediaReportMetadata)) {
		return WindowsEncryptedMediaCommand{}, false
	}
	return WindowsEncryptedMediaCommand{Kind: WindowsEncryptedMediaReportMetadata, ObjectID: snapshot.ReportTarget.ObjectID}, true
}

func (model *WindowsEncryptedMediaModel) DecryptedEvidenceExportCommand(confirmed bool) (WindowsEncryptedMediaCommand, bool) {
	snapshot := model.Snapshot()
	if !confirmed || !windowsEncryptedMediaProtectedCryptographyReady(snapshot) || snapshot.ReportTarget == nil ||
		!snapshot.ReportTarget.CanExportDecryptedEvidence || !snapshot.ReportTarget.DecryptedEvidenceReady ||
		!windowsEncryptedMediaHasAction(snapshot, string(WindowsEncryptedMediaExportDecryptedEvidence)) {
		return WindowsEncryptedMediaCommand{}, false
	}
	return WindowsEncryptedMediaCommand{Kind: WindowsEncryptedMediaExportDecryptedEvidence,
		ObjectID: snapshot.ReportTarget.ObjectID, ConsentVersion: snapshot.ReportTarget.ConsentVersion}, true
}

func containsWindowsEncryptedMediaRecoveryMode(modes []WindowsEncryptedMediaRecoveryMode, wanted WindowsEncryptedMediaRecoveryMode) bool {
	for _, mode := range modes {
		if mode == wanted {
			return true
		}
	}
	return false
}

// WindowsEncryptedMediaPresentation deliberately contains display labels and
// status values only. Stable identifiers, key material and decrypted bytes are
// not representable in the future native-control projection.
type WindowsEncryptedMediaPresentation struct {
	State             TargetsInboxSurfaceState
	SelectedPathLabel string
	PathRows          []WindowsEncryptedMediaPathPresentation
	DeviceRows        []WindowsEncryptedMediaDevicePresentation
	HistoryWarning    string
	ReportSummary     string
	RecoverySummary   string
	AccessibleSummary string
}

type WindowsEncryptedMediaPathPresentation struct {
	Label           string
	AccessibleName  string
	AccessibleValue string
	Availability    WindowsEncryptedMediaAvailability
	FailureCode     string
	Selectable      bool
}

type WindowsEncryptedMediaDevicePresentation struct {
	Label           string
	AccessibleName  string
	AccessibleValue string
	Verification    WindowsEncryptedMediaVerification
	CanVerify       bool
	CanRevoke       bool
}

func (model *WindowsEncryptedMediaModel) Presentation(locale ShellLocale) WindowsEncryptedMediaPresentation {
	snapshot := model.Snapshot()
	paths := []WindowsEncryptedMediaPath{
		WindowsEncryptedMediaPlaintext, WindowsEncryptedMediaProtectedClip,
		WindowsEncryptedMediaProtectedTrack, WindowsEncryptedMediaProtectedLive,
	}
	presentation := WindowsEncryptedMediaPresentation{State: snapshot.State}
	for _, path := range paths {
		label := windowsEncryptedMediaPathLabel(locale, path)
		availability := model.Availability(path)
		_, selectable := model.SelectPathCommand(path)
		presentation.PathRows = append(presentation.PathRows, WindowsEncryptedMediaPathPresentation{
			Label: label, AccessibleName: label, AccessibleValue: windowsEncryptedMediaAvailabilityLabel(locale, availability),
			Availability: availability, FailureCode: model.PathFailure(path), Selectable: selectable,
		})
		if path == snapshot.SelectedPath {
			presentation.SelectedPathLabel = label
		}
	}
	for _, device := range snapshot.Devices {
		_, canVerify := model.VerifyDeviceCommand(device.ID)
		_, canRevoke := model.RevokeDeviceCommand(device.ID, true)
		label := device.Label.Text(locale)
		presentation.DeviceRows = append(presentation.DeviceRows, WindowsEncryptedMediaDevicePresentation{
			Label: label, AccessibleName: label,
			AccessibleValue: windowsEncryptedMediaVerificationLabel(locale, device.Verification),
			Verification:    device.Verification, CanVerify: canVerify, CanRevoke: canRevoke,
		})
	}
	if !snapshot.HistoryRecoverable {
		presentation.HistoryWarning = windowsEncryptedMediaCopy(locale, "History cannot be recovered without an authorized peer or user-held recovery.", "Историю нельзя восстановить без авторизованного устройства или пользовательского восстановления.")
	}
	presentation.ReportSummary = windowsEncryptedMediaCopy(locale,
		"Metadata-only report is separate from a confirmed decrypted-evidence copy.",
		"Жалоба только с метаданными отделена от подтверждённой копии расшифрованного доказательства.")
	presentation.RecoverySummary = windowsEncryptedMediaCopy(locale,
		"Device transfer copies current access only; history requires an explicit grant.",
		"Перенос на устройство копирует только текущий доступ; для истории нужен отдельный грант.")
	presentation.AccessibleSummary = presentation.SelectedPathLabel + ": " +
		windowsEncryptedMediaAvailabilityLabel(locale, model.Availability(snapshot.SelectedPath))
	return presentation
}

func windowsEncryptedMediaPathLabel(locale ShellLocale, path WindowsEncryptedMediaPath) string {
	switch path {
	case WindowsEncryptedMediaPlaintext:
		return windowsEncryptedMediaCopy(locale, "Plaintext", "Открытый текст")
	case WindowsEncryptedMediaProtectedClip:
		return windowsEncryptedMediaCopy(locale, "Encrypted clip", "Зашифрованный клип")
	case WindowsEncryptedMediaProtectedTrack:
		return windowsEncryptedMediaCopy(locale, "Encrypted track", "Зашифрованный трек")
	case WindowsEncryptedMediaProtectedLive:
		return windowsEncryptedMediaCopy(locale, "Encrypted live PTT", "Зашифрованный live PTT")
	default:
		return windowsEncryptedMediaCopy(locale, "Unavailable path", "Недоступный путь")
	}
}

func windowsEncryptedMediaAvailabilityLabel(locale ShellLocale, availability WindowsEncryptedMediaAvailability) string {
	switch availability {
	case WindowsEncryptedMediaAvailableEncrypted:
		return windowsEncryptedMediaCopy(locale, "Encrypted and available", "Зашифровано и доступно")
	case WindowsEncryptedMediaAvailablePlaintext:
		return windowsEncryptedMediaCopy(locale, "Not end-to-end encrypted", "Без сквозного шифрования")
	default:
		return windowsEncryptedMediaCopy(locale, "Blocked; no plaintext fallback", "Заблокировано; без перехода на открытый текст")
	}
}

func windowsEncryptedMediaVerificationLabel(locale ShellLocale, verification WindowsEncryptedMediaVerification) string {
	switch verification {
	case WindowsEncryptedMediaVerified:
		return windowsEncryptedMediaCopy(locale, "Verified device", "Проверенное устройство")
	case WindowsEncryptedMediaRevoked:
		return windowsEncryptedMediaCopy(locale, "Revoked device", "Отозванное устройство")
	default:
		return windowsEncryptedMediaCopy(locale, "Unverified device", "Непроверенное устройство")
	}
}

func windowsEncryptedMediaCopy(locale ShellLocale, english, russian string) string {
	if locale == ShellRussian {
		return russian
	}
	return english
}

var ErrWindowsEncryptedMediaClientComposition = errors.New("Windows encrypted-media client composition is unavailable")

// WindowsEncryptedMediaClientPathComposition is intentionally absent from
// main.go. It creates accepted services around one repository but cannot select
// a provider, suite, container, capability or runtime route.
type WindowsEncryptedMediaClientPathComposition struct {
	KeyState          *WindowsE2EEKeyStateRepository
	ProtectedSend     *WindowsProtectedMediaSendService
	ProtectedPlayback *WindowsProtectedMediaPlaybackService
	ProtectedLive     *WindowsE2EELiveSessionFactory
}

type WindowsEncryptedMediaClientPathOptions struct {
	KeyState            *WindowsE2EEKeyStateRepository
	Sealer              WindowsProtectedMediaSealer
	Uploader            WindowsProtectedMediaUploader
	Opener              WindowsProtectedMediaOpening
	PlaybackTransport   WindowsProtectedMediaPlaybackTransport
	LiveDerivation      WindowsE2EELiveSessionDeriver
	LiveAuthorization   WindowsE2EELiveAuthorizationChecker
	CiphertextRoot      string
	PlaintextDraftRoot  string
	PlaybackCacheRoot   string
	PlaybackCacheSecret []byte
}

func NewWindowsEncryptedMediaClientPathComposition(options WindowsEncryptedMediaClientPathOptions) (*WindowsEncryptedMediaClientPathComposition, error) {
	if options.KeyState == nil {
		return nil, ErrWindowsEncryptedMediaClientComposition
	}
	send, err := NewWindowsProtectedMediaSendService(WindowsProtectedMediaSendOptions{
		KeyState: options.KeyState, Sealer: options.Sealer, Uploader: options.Uploader,
		CiphertextRoot: options.CiphertextRoot, PlaintextDraftRoot: options.PlaintextDraftRoot,
	})
	if err != nil {
		return nil, ErrWindowsEncryptedMediaClientComposition
	}
	playback, err := NewWindowsProtectedMediaPlaybackService(WindowsProtectedMediaPlaybackOptions{
		KeyState: options.KeyState, Opener: options.Opener, Transport: options.PlaybackTransport,
		CiphertextCacheRoot: options.PlaybackCacheRoot, CacheInstallationSecret: options.PlaybackCacheSecret,
	})
	if err != nil {
		return nil, ErrWindowsEncryptedMediaClientComposition
	}
	live, err := NewWindowsE2EELiveSessionFactory(options.KeyState, options.LiveDerivation, options.LiveAuthorization)
	if err != nil {
		return nil, ErrWindowsEncryptedMediaClientComposition
	}
	return &WindowsEncryptedMediaClientPathComposition{
		KeyState: options.KeyState, ProtectedSend: send, ProtectedPlayback: playback, ProtectedLive: live,
	}, nil
}

func (composition *WindowsEncryptedMediaClientPathComposition) ProductionDarkModel(now time.Time) *WindowsEncryptedMediaModel {
	componentsReady := composition != nil && composition.KeyState != nil && composition.ProtectedSend != nil &&
		composition.ProtectedPlayback != nil && composition.ProtectedLive != nil
	return NewWindowsEncryptedMediaModel(WindowsEncryptedMediaSnapshot{
		State: TargetsInboxLoading, SelectedPath: WindowsEncryptedMediaPlaintext,
		CapabilityAdvertised: false, ReviewedSuiteSelected: false, RuntimeWiringApproved: false,
		Ownership: WindowsEncryptedMediaOwnershipCrossProcessLock,
		Components: WindowsEncryptedMediaComponents{KeyStateReady: componentsReady, ProtectedSendReady: componentsReady,
			ProtectedPlaybackReady: componentsReady, ProtectedLiveReady: componentsReady, SameRepositoryWitness: componentsReady},
	}, now)
}

func windowsEncryptedMediaSourceIsRuntimeDark(source string) bool {
	return !strings.Contains(source, "NewWindowsEncryptedMediaClientPathComposition") &&
		!strings.Contains(source, "WindowsEncryptedMediaClientPathComposition")
}
