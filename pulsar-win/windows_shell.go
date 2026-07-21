package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

type ShellLocale string

const (
	ShellEnglish ShellLocale = "en"
	ShellRussian ShellLocale = "ru"
)

type ShellSection string

const (
	ShellHome       ShellSection = "home"
	ShellCreate     ShellSection = "create"
	ShellJoin       ShellSection = "join"
	ShellTryLocally ShellSection = "try_locally"
	ShellHistory    ShellSection = "history"
	ShellSoundboard ShellSection = "soundboard"
	ShellInbox      ShellSection = "inbox"
	ShellAirs       ShellSection = "airs"
	ShellAutomation ShellSection = "automation"
	ShellSettings   ShellSection = "settings"
)

var shellSections = []ShellSection{
	ShellHome, ShellCreate, ShellJoin, ShellTryLocally, ShellSoundboard, ShellHistory, ShellInbox, ShellAirs, ShellAutomation, ShellSettings,
}

type ShellConnection string

const (
	ShellUnpaired     ShellConnection = "unpaired"
	ShellReconnecting ShellConnection = "reconnecting"
	ShellOnline       ShellConnection = "online"
	ShellDegraded     ShellConnection = "degraded"
)

type ShellRecording string

const (
	ShellRecordingUnavailable ShellRecording = "unavailable"
	ShellRecordingIdle        ShellRecording = "idle"
	ShellRecordingActive      ShellRecording = "recording"
	ShellRecordingProcessing  ShellRecording = "processing"
	ShellRecordingFailed      ShellRecording = "failed"
)

type ShellDND string

const (
	ShellDNDAllowAll     ShellDND = "allow_all"
	ShellDNDMessagesOnly ShellDND = "messages_only"
	ShellDNDMutedUntil   ShellDND = "muted_until"
)

type ShellIdentityOperation string

const (
	ShellIdentityIdle             ShellIdentityOperation = "idle"
	ShellIdentityWorking          ShellIdentityOperation = "working"
	ShellIdentityRecoveryRequired ShellIdentityOperation = "recovery_required"
	ShellIdentityActive           ShellIdentityOperation = "active"
	ShellIdentityFailed           ShellIdentityOperation = "failed"
)

type ShellPhaseOneDraft struct {
	Title                         string
	State                         PhaseOneDraftState
	Route                         PhaseOneRoute
	RequestedDelivery             PhaseOneDelivery
	EffectiveDelivery             PhaseOneDelivery
	DowngradeReason               string
	Status                        string
	FailureCode                   string
	LocalBytesRetained            bool
	FallbackConfirmationAvailable bool
	ExplicitTargetCount           int
}

type ShellPhaseOneHistoryItem struct {
	Title               string
	SenderName          string
	Direction           string
	Status              string
	RequestedDelivery   string
	EffectiveDelivery   string
	DowngradeReason     string
	PlayedCount         int
	OtherCount          int
	CanDelete           bool
	CanReplay           bool
	CanReport           bool
	CanBlock            bool
	AutomationTrigger   string
	AutomationActor     string
	AutomationSchedule  string
	AutomationCue       string
	AutomationReason    string
	CanDisableSchedule  bool
	CanRevokePrincipal  bool
	CanEmergencyDisable bool
}

type ShellSoundboardCue struct {
	Title          string
	SourceKind     string
	ShortcutLabel  string
	ShortcutStatus WindowsRecordingShortcutStatus
}

type ShellAirItem struct {
	AirID              string
	Title              string
	Status             string
	Revision           int64
	MembershipStatus   AirMembershipStatus
	MembershipRevision int64
	Role               AirRole
	MemberCount        int
	ActiveMemberCount  int
	OnlinePulsarCount  int
	Capacity           AirCapacity
	Policy             AirPolicy
	Current            bool
}

type ShellPendingAirJoin struct {
	AirID                 string
	Title                 string
	OwnerDisplayName      string
	Role                  AirRole
	MembershipRevision    int64
	MemberCount           int
	Capacity              AirCapacity
	ActivationWouldSwitch bool
}

type ShellSnapshot struct {
	Connection                     ShellConnection
	ConnectionDetail               string
	Identity                       string
	PresenceOnline                 int
	PresenceTotal                  int
	PresenceReady                  int
	PresenceAvailable              bool
	RouteName                      string
	NowPlaying                     string
	PlaybackState                  string
	HistoryCount                   int
	DND                            ShellDND
	Recording                      ShellRecording
	RecordingAvailable             bool
	RecordingShortcut              WindowsRecordingShortcutStatus
	RecordingShortcutKey           WindowsRecordingShortcut
	CaptureQualityMode             WindowsCaptureQualityMode
	CaptureQualityDegradedConsent  bool
	CaptureQualityBackendAvailable bool
	CaptureQualityState            *protocol.CaptureQualityState
	SelfTestAvailable              bool
	SelfTestPhase                  WindowsLocalSelfTestPhase
	SelfTestMeter                  float32
	LocalDraftAvailable            bool
	LocalDraftName                 string
	RecordingDraftAvailable        bool
	LocalFailure                   string
	CaptureInputs                  []WindowsCaptureInput
	SelectedCaptureInput           int
	AudioOutputs                   []WindowsAudioOutput
	SelectedAudioOutput            int
	Volume                         int
	IdentityOperation              ShellIdentityOperation
	IdentityFailure                string
	RecoveryExportRequired         bool
	PhaseOneDrafts                 []ShellPhaseOneDraft
	SelectedPhaseOneDraft          int
	SelectedPhaseOneRoute          PhaseOneRoute
	SelectedPhaseOneDelivery       PhaseOneDelivery
	PhaseOneHistory                []ShellPhaseOneHistoryItem
	SelectedHistoryItem            int
	SelectedReportReason           PhaseOneModerationReason
	PhaseOneActionOutcome          string
	PhaseOneFailure                string
	SoundboardCues                 []ShellSoundboardCue
	SelectedSoundboardCue          int
	SoundboardRoute                PhaseOneRoute
	SoundboardDelivery             PhaseOneDelivery
	SoundboardIncludeOrigin        bool
	SoundboardBusy                 bool
	SoundboardOutcome              string
	SoundboardFailure              string
	SoundboardHistoryCount         int
	TargetsInbox                   TargetsInboxSnapshot
	SelectedTarget                 int
	SelectedInbox                  int
	SelectedTargetsHistory         int
	TargetsInboxDelivery           PhaseOneDelivery
	TargetsInboxReason             PhaseOneModerationReason
	TargetsInboxActionOutcome      string
	TargetsInboxFailure            string
	TargetsInboxBusy               bool
	StreamTrack                    StreamTrackSnapshot
	SelectedStreamTrackTarget      int
	StreamTrackBusy                bool
	StreamTrackOutcome             string
	Airs                           []ShellAirItem
	SelectedAir                    int
	PendingAirJoin                 *ShellPendingAirJoin
	AirInviteAvailable             bool
	AirInviteExpires               time.Time
	AirInviteRole                  AirRole
	AirAvailable                   bool
	AirBusy                        bool
	AirConfirmAction               string
	AirOutcome                     string
	AirFailure                     string
	Automation                     WindowsAutomationSnapshot
}

func (s ShellSnapshot) normalized() ShellSnapshot {
	switch s.Connection {
	case ShellUnpaired, ShellReconnecting, ShellOnline, ShellDegraded:
	default:
		s.Connection = ShellUnpaired
	}
	switch s.DND {
	case ShellDNDAllowAll, ShellDNDMessagesOnly, ShellDNDMutedUntil:
	default:
		s.DND = ShellDNDAllowAll
	}
	switch s.Recording {
	case ShellRecordingUnavailable, ShellRecordingIdle, ShellRecordingActive,
		ShellRecordingProcessing, ShellRecordingFailed:
	default:
		s.Recording = ShellRecordingUnavailable
	}
	switch s.RecordingShortcut {
	case WindowsShortcutInactive, WindowsShortcutRegistered, WindowsShortcutConflict,
		WindowsShortcutUnavailable, WindowsShortcutSuspended:
	default:
		s.RecordingShortcut = WindowsShortcutInactive
	}
	if s.CaptureQualityMode != WindowsCaptureQualitySpeaker && s.CaptureQualityMode != WindowsCaptureQualityHeadphone {
		s.CaptureQualityMode = WindowsCaptureQualityAuto
	}
	if protocol.ValidateCaptureQualityState(s.CaptureQualityState) != nil {
		s.CaptureQualityState = nil
		s.CaptureQualityBackendAvailable = false
	} else {
		s.CaptureQualityState = protocol.CloneCaptureQualityState(s.CaptureQualityState)
	}
	if s.Volume < 0 {
		s.Volume = 0
	}
	if s.Volume > 100 {
		s.Volume = 100
	}
	if s.PresenceOnline < 0 {
		s.PresenceOnline = 0
	}
	if s.PresenceTotal < s.PresenceOnline {
		s.PresenceTotal = s.PresenceOnline
	}
	if s.PresenceReady < 0 {
		s.PresenceReady = 0
	}
	if s.PresenceReady > s.PresenceOnline {
		s.PresenceReady = s.PresenceOnline
	}
	if s.SelfTestMeter < 0 {
		s.SelfTestMeter = 0
	}
	if s.SelfTestMeter > 1 {
		s.SelfTestMeter = 1
	}
	if s.SelectedCaptureInput < 0 || s.SelectedCaptureInput >= len(s.CaptureInputs) {
		s.SelectedCaptureInput = 0
	}
	if s.SelectedAudioOutput < 0 || s.SelectedAudioOutput >= len(s.AudioOutputs) {
		s.SelectedAudioOutput = 0
	}
	switch s.IdentityOperation {
	case ShellIdentityIdle, ShellIdentityWorking, ShellIdentityRecoveryRequired, ShellIdentityActive, ShellIdentityFailed:
	default:
		s.IdentityOperation = ShellIdentityIdle
	}
	if s.SelectedPhaseOneDraft < 0 || s.SelectedPhaseOneDraft >= len(s.PhaseOneDrafts) {
		s.SelectedPhaseOneDraft = 0
	}
	if s.SelectedHistoryItem < 0 || s.SelectedHistoryItem >= len(s.PhaseOneHistory) {
		s.SelectedHistoryItem = 0
	}
	if !validPhaseOneRoute(s.SelectedPhaseOneRoute) {
		s.SelectedPhaseOneRoute = PhaseOneThisPulsar
	}
	if !validPhaseOneDelivery(s.SelectedPhaseOneDelivery) {
		s.SelectedPhaseOneDelivery = PhaseOneOverlay
	}
	if !validPhaseOneModerationReason(s.SelectedReportReason) {
		s.SelectedReportReason = PhaseOneReportSpam
	}
	if s.SelectedSoundboardCue < 0 || s.SelectedSoundboardCue >= len(s.SoundboardCues) {
		s.SelectedSoundboardCue = 0
	}
	if !validPhaseOneRoute(s.SoundboardRoute) {
		s.SoundboardRoute = PhaseOneOwnBarycenter
	}
	if !validPhaseOneDelivery(s.SoundboardDelivery) {
		s.SoundboardDelivery = PhaseOneOverlay
	}
	if s.SelectedTarget < 0 || s.SelectedTarget >= len(s.TargetsInbox.Targets) {
		s.SelectedTarget = 0
	}
	if s.SelectedInbox < 0 || s.SelectedInbox >= len(s.TargetsInbox.Inbox) {
		s.SelectedInbox = 0
	}
	if s.SelectedTargetsHistory < 0 || s.SelectedTargetsHistory >= len(s.TargetsInbox.History) {
		s.SelectedTargetsHistory = 0
	}
	if s.SelectedStreamTrackTarget < 0 || s.SelectedStreamTrackTarget >= len(s.StreamTrack.Targets) {
		s.SelectedStreamTrackTarget = 0
	}
	if !validPhaseOneDelivery(s.TargetsInboxDelivery) {
		s.TargetsInboxDelivery = PhaseOneOverlay
	}
	if !validPhaseOneModerationReason(s.TargetsInboxReason) {
		s.TargetsInboxReason = PhaseOneReportSpam
	}
	if s.SelectedAir < 0 || s.SelectedAir >= len(s.Airs) {
		s.SelectedAir = 0
	}
	if s.AirInviteRole != AirRoleAdmin {
		s.AirInviteRole = AirRoleMember
	}
	if s.Automation.SelectedSchedule < 0 || s.Automation.SelectedSchedule >= len(s.Automation.Schedules) {
		s.Automation.SelectedSchedule = 0
	}
	if s.Automation.SelectedPrincipal < 0 || s.Automation.SelectedPrincipal >= len(s.Automation.Principals) {
		s.Automation.SelectedPrincipal = 0
	}
	if s.Automation.SelectedHistory < 0 || s.Automation.SelectedHistory >= len(s.Automation.History) {
		s.Automation.SelectedHistory = 0
	}
	return s
}

type ShellActions struct {
	Create                          func(string)
	Join                            func(string)
	SaveRecovery                    func(string)
	TryLocally                      func()
	PlayBuiltinCue                  func()
	ChooseLocalFile                 func()
	ChooseOutgoingFile              func()
	AcceptDroppedFile               func(WindowsBrokeredAudioFile)
	DeleteLocalDraft                func()
	SelectNextInput                 func()
	SelectNextOutput                func()
	ToggleRecording                 func()
	CancelRecording                 func()
	SetCaptureQuality               func(WindowsCaptureQualityMode, bool)
	StopActiveCapture               func()
	SetDND                          func(ShellDND)
	SendSelectedDraft               func()
	DeleteSelectedDraft             func()
	SelectNextPhaseOneDraft         func()
	SelectNextPhaseOneRoute         func()
	SelectNextPhaseOneDelivery      func()
	SelectNextHistoryItem           func()
	SelectNextReportReason          func()
	DeleteSelectedHistoryItem       func()
	ReportSelectedHistoryItem       func(string)
	ReplaySelectedHistoryItem       func()
	BlockSelectedHistoryActor       func()
	SelectNextSoundboardCue         func()
	TriggerSelectedSoundboardCue    func()
	SelectNextSoundboardRoute       func()
	SelectNextSoundboardDelivery    func()
	ToggleSoundboardIncludeOrigin   func()
	DeleteSelectedSoundboardCue     func()
	MoveSelectedSoundboardCue       func(int)
	CycleSelectedSoundboardShortcut func()
	ChooseSoundboardFile            func()
	RenameSelectedSoundboardCue     func(string)
	RefreshTargetsInbox             func()
	SelectNextTargetAudience        func()
	SelectNextTarget                func()
	ToggleSelectedTarget            func()
	ToggleTargetIncludeOrigin       func()
	SelectNextTargetsDelivery       func()
	SendTargetsDraft                func()
	SelectNextInboxItem             func()
	ReplaySelectedInbox             func()
	DismissSelectedInbox            func()
	ReportSelectedInbox             func(string)
	MuteSelectedInbox               func()
	LoadMoreInbox                   func()
	SelectNextTargetsHistory        func()
	DeleteSelectedTargetsHistory    func()
	ReportSelectedTargetsHistory    func(string)
	MuteSelectedTargetsHistory      func()
	LoadMoreTargetsHistory          func()
	LoadMoreTargetReceipts          func()
	SelectNextTargetsReason         func()
	ChooseStreamTrackFile           func()
	AcceptDroppedStreamTrack        func(WindowsBrokeredAudioFile)
	RefreshStreamTrack              func()
	AcceptStreamTrackPolicy         func()
	UploadStreamTrack               func()
	DeleteStreamTrack               func(bool)
	SelectNextStreamTrackAudience   func()
	SelectNextStreamTrackTarget     func()
	ToggleStreamTrackTarget         func()
	SelectNextStreamTrackInsertion  func()
	QueueStreamTrack                func()
	ReplaceStreamTrack              func()
	PauseStreamTrack                func()
	SeekStreamTrack                 func()
	ResumeStreamTrack               func()
	RetryStreamTrack                func()
	ReportStreamTrack               func(string)
	SelectNextAir                   func()
	CreateAir                       func(string)
	ConsumeAirInvite                func(string)
	ConfirmAirJoin                  func(bool)
	DeclineAirJoin                  func()
	SelectNextAirInviteRole         func()
	IssueAirInvite                  func()
	CopyAirInvite                   func()
	HideAirInvite                   func()
	WithdrawAirInvite               func()
	RequestAirActivation            func()
	RequestAirLeave                 func()
	RequestAirDissolve              func()
	CycleAirPolicy                  func()
	ConfirmAirDisruptive            func()
	CancelAirDisruptive             func()
	RefreshAutomation               func()
	SelectNextAutomationSchedule    func()
	SaveAutomationSchedule          func(string, string, string, string, string)
	RequestAutomationAction         func(string)
	ConfirmAutomationAction         func(string)
	CancelAutomationConfirmation    func()
	SelectNextAutomationPrincipal   func()
	SelectNextAutomationHistory     func()
	SaveAutomationFeature           func(string, string)
	CopyAutomationSecret            func()
	HideAutomationSecret            func()
}

type WindowsShell struct {
	mu       sync.RWMutex
	locale   ShellLocale
	section  ShellSection
	snapshot func() ShellSnapshot
	actions  ShellActions
}

func NewWindowsShell(locale ShellLocale, snapshot func() ShellSnapshot, actions ShellActions) *WindowsShell {
	if locale != ShellRussian {
		locale = ShellEnglish
	}
	if snapshot == nil {
		snapshot = func() ShellSnapshot { return ShellSnapshot{} }
	}
	return &WindowsShell{locale: locale, section: ShellHome, snapshot: snapshot, actions: actions}
}

func (s *WindowsShell) Locale() ShellLocale {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.locale
}

func (s *WindowsShell) SetLocale(locale ShellLocale) {
	if locale != ShellEnglish && locale != ShellRussian {
		return
	}
	s.mu.Lock()
	s.locale = locale
	s.mu.Unlock()
}

func (s *WindowsShell) Section() ShellSection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.section
}

func (s *WindowsShell) Select(section ShellSection) {
	for _, candidate := range shellSections {
		if candidate == section {
			s.mu.Lock()
			s.section = section
			s.mu.Unlock()
			return
		}
	}
}

func (s *WindowsShell) Snapshot() ShellSnapshot { return s.snapshot().normalized() }
func (s *WindowsShell) Actions() ShellActions   { return s.actions }

type shellText string

const (
	txtApp                  shellText = "app"
	txtHome                 shellText = "home"
	txtCreate               shellText = "create"
	txtJoin                 shellText = "join"
	txtTry                  shellText = "try"
	txtHistory              shellText = "history"
	txtSoundboard           shellText = "soundboard"
	txtInbox                shellText = "inbox"
	txtAirs                 shellText = "airs"
	txtSettings             shellText = "settings"
	txtOpen                 shellText = "open"
	txtPrimary              shellText = "primary"
	txtStatus               shellText = "status"
	txtPresence             shellText = "presence"
	txtRouting              shellText = "routing"
	txtNowPlaying           shellText = "now_playing"
	txtLocalControls        shellText = "local_controls"
	txtNoHistory            shellText = "no_history"
	txtNoRoute              shellText = "no_route"
	txtSilence              shellText = "silence"
	txtVolume               shellText = "volume"
	txtDND                  shellText = "dnd"
	txtRecording            shellText = "recording"
	txtStartRecording       shellText = "start_recording"
	txtStopRecording        shellText = "stop_recording"
	txtCancelRecording      shellText = "cancel_recording"
	txtRecordingUnavailable shellText = "recording_unavailable"
	txtSelfTestUnavailable  shellText = "self_test_unavailable"
	txtCreateTitle          shellText = "create_title"
	txtCreateBody           shellText = "create_body"
	txtCreateAction         shellText = "create_action"
	txtJoinTitle            shellText = "join_title"
	txtJoinBody             shellText = "join_body"
	txtJoinAction           shellText = "join_action"
	txtTryTitle             shellText = "try_title"
	txtTryBody              shellText = "try_body"
	txtTryAction            shellText = "try_action"
	txtHistoryTitle         shellText = "history_title"
	txtSettingsTitle        shellText = "settings_title"
	txtLanguage             shellText = "language"
	txtIntegrations         shellText = "integrations"
	txtSpotifyOptional      shellText = "spotify_optional"
	txtTelegramOptional     shellText = "telegram_optional"
	txtReport               shellText = "report"
	txtUnpaired             shellText = "unpaired"
	txtReconnecting         shellText = "reconnecting"
	txtOnline               shellText = "online"
	txtDegraded             shellText = "degraded"
	txtDNDAllow             shellText = "dnd_allow"
	txtDNDMessages          shellText = "dnd_messages"
	txtDNDMuted             shellText = "dnd_muted"
	txtRecordingIdle        shellText = "recording_idle"
	txtRecordingActive      shellText = "recording_active"
	txtRecordingProcessing  shellText = "recording_processing"
	txtRecordingFailed      shellText = "recording_failed"
	txtUnpairedHelp         shellText = "unpaired_help"
	txtDegradedHelp         shellText = "degraded_help"
	txtRecordingHelp        shellText = "recording_help"
	txtShortcutRegistered   shellText = "shortcut_registered"
	txtShortcutConflict     shellText = "shortcut_conflict"
	txtShortcutUnavailable  shellText = "shortcut_unavailable"
	txtShortcutSuspended    shellText = "shortcut_suspended"
	txtShortcutInactive     shellText = "shortcut_inactive"
	txtShortcut             shellText = "shortcut"
	txtPair                 shellText = "pair"
	txtRepair               shellText = "repair"
	txtHowToSound           shellText = "how_to_sound"
	txtNoPulsar             shellText = "no_pulsar"
	txtPrivacy              shellText = "privacy"
	txtTerms                shellText = "terms"
	txtGuidelines           shellText = "guidelines"
	txtUploadRights         shellText = "upload_rights"
	txtSupport              shellText = "support"
	txtQuit                 shellText = "quit"
	txtCaptureQuality       shellText = "capture_quality"
	txtCaptureMode          shellText = "capture_mode"
	txtCaptureAllowDegraded shellText = "capture_allow_degraded"
	txtCaptureStopLocal     shellText = "capture_stop_local"
	txtCaptureInputCeiling  shellText = "capture_input_ceiling"
	txtCaptureOutputCeiling shellText = "capture_output_ceiling"
	txtCaptureCeilingHelp   shellText = "capture_ceiling_help"
	txtCaptureConsentHelp   shellText = "capture_consent_help"
)

var shellTextKeys = []shellText{
	txtApp, txtHome, txtCreate, txtJoin, txtTry, txtSoundboard, txtHistory, txtInbox, txtAirs, txtSettings, txtOpen,
	txtPrimary, txtStatus, txtPresence, txtRouting, txtNowPlaying, txtLocalControls,
	txtNoHistory, txtNoRoute, txtSilence, txtVolume, txtDND, txtRecording,
	txtStartRecording, txtStopRecording, txtCancelRecording, txtRecordingUnavailable, txtSelfTestUnavailable,
	txtCreateTitle, txtCreateBody, txtCreateAction, txtJoinTitle, txtJoinBody,
	txtJoinAction, txtTryTitle, txtTryBody, txtTryAction, txtHistoryTitle,
	txtSettingsTitle, txtLanguage, txtIntegrations, txtSpotifyOptional, txtTelegramOptional, txtReport,
	txtUnpaired, txtReconnecting, txtOnline, txtDegraded,
	txtDNDAllow, txtDNDMessages, txtDNDMuted, txtRecordingIdle, txtRecordingActive,
	txtRecordingProcessing, txtRecordingFailed, txtUnpairedHelp, txtDegradedHelp,
	txtRecordingHelp, txtShortcutRegistered, txtShortcutConflict, txtShortcutUnavailable,
	txtShortcutSuspended, txtShortcutInactive, txtShortcut, txtPair, txtRepair, txtHowToSound, txtNoPulsar, txtPrivacy,
	txtTerms, txtGuidelines, txtUploadRights, txtSupport, txtQuit,
	txtCaptureQuality, txtCaptureMode, txtCaptureAllowDegraded, txtCaptureStopLocal,
	txtCaptureInputCeiling, txtCaptureOutputCeiling, txtCaptureCeilingHelp, txtCaptureConsentHelp,
}

type ShellCopy struct{ locale ShellLocale }

func NewShellCopy(locale ShellLocale) ShellCopy {
	if locale != ShellRussian {
		locale = ShellEnglish
	}
	return ShellCopy{locale: locale}
}

func (c ShellCopy) Text(key shellText) string {
	value := shellCatalog[c.locale][key]
	if value == "" {
		return "[missing:" + string(key) + "]"
	}
	return value
}

func (c ShellCopy) Section(section ShellSection) string {
	if section == ShellAutomation {
		if c.locale == ShellRussian {
			return "Автоматизация"
		}
		return "Automation"
	}
	return c.Text(map[ShellSection]shellText{
		ShellHome: txtHome, ShellCreate: txtCreate, ShellJoin: txtJoin,
		ShellTryLocally: txtTry, ShellSoundboard: txtSoundboard, ShellHistory: txtHistory, ShellInbox: txtInbox, ShellAirs: txtAirs, ShellSettings: txtSettings,
	}[section])
}

func (c ShellCopy) Connection(snapshot ShellSnapshot) string {
	label := map[ShellConnection]string{
		ShellUnpaired:     c.Text(txtUnpaired),
		ShellReconnecting: c.Text(txtReconnecting),
		ShellOnline:       c.Text(txtOnline),
		ShellDegraded:     c.Text(txtDegraded),
	}[snapshot.Connection]
	if snapshot.ConnectionDetail != "" {
		label += ": " + snapshot.ConnectionDetail
	}
	return label
}

func (c ShellCopy) Recording(snapshot ShellSnapshot) string {
	key := map[ShellRecording]shellText{
		ShellRecordingUnavailable: txtRecordingUnavailable,
		ShellRecordingIdle:        txtRecordingIdle,
		ShellRecordingActive:      txtRecordingActive,
		ShellRecordingProcessing:  txtRecordingProcessing,
		ShellRecordingFailed:      txtRecordingFailed,
	}[snapshot.Recording]
	prefix := "[MIC] "
	if snapshot.Recording == ShellRecordingActive {
		prefix = "[REC] "
	} else if snapshot.Recording == ShellRecordingFailed {
		prefix = "[!] "
	}
	return prefix + c.Text(key)
}

func (c ShellCopy) RecordingShortcut(status WindowsRecordingShortcutStatus, shortcut WindowsRecordingShortcut) string {
	key := map[WindowsRecordingShortcutStatus]shellText{
		WindowsShortcutRegistered:  txtShortcutRegistered,
		WindowsShortcutConflict:    txtShortcutConflict,
		WindowsShortcutUnavailable: txtShortcutUnavailable,
		WindowsShortcutSuspended:   txtShortcutSuspended,
		WindowsShortcutInactive:    txtShortcutInactive,
	}[status]
	if key == "" {
		key = txtShortcutInactive
	}
	if !shortcut.Valid() {
		shortcut = DefaultWindowsRecordingShortcut()
	}
	return c.Text(txtShortcut) + ": " + shortcut.Label() + " — " + c.Text(key)
}

func (c ShellCopy) DND(mode ShellDND) string {
	return c.Text(map[ShellDND]shellText{
		ShellDNDAllowAll: txtDNDAllow, ShellDNDMessagesOnly: txtDNDMessages,
		ShellDNDMutedUntil: txtDNDMuted,
	}[mode])
}

func (c ShellCopy) CaptureQualityMode(mode WindowsCaptureQualityMode) string {
	en := map[WindowsCaptureQualityMode]string{
		WindowsCaptureQualityAuto: "Auto", WindowsCaptureQualitySpeaker: "Speaker",
		WindowsCaptureQualityHeadphone: "Headphones",
	}
	ru := map[WindowsCaptureQualityMode]string{
		WindowsCaptureQualityAuto: "Авто", WindowsCaptureQualitySpeaker: "Динамики",
		WindowsCaptureQualityHeadphone: "Наушники",
	}
	if c.locale == ShellRussian {
		return ru[mode]
	}
	return en[mode]
}

func (c ShellCopy) CaptureQualityLabel(quality string) string {
	en := map[string]string{
		protocol.CaptureQualityAccepted:    "[OK] Accepted processing",
		protocol.CaptureQualityDegraded:    "[!] Degraded processing",
		protocol.CaptureQualityUnsupported: "[X] Processing unavailable",
	}
	ru := map[string]string{
		protocol.CaptureQualityAccepted:    "[OK] Обработка принята",
		protocol.CaptureQualityDegraded:    "[!] Ограниченная обработка",
		protocol.CaptureQualityUnsupported: "[X] Обработка недоступна",
	}
	if c.locale == ShellRussian {
		return ru[quality]
	}
	return en[quality]
}

func (c ShellCopy) CaptureQualityReason(reason string) string {
	en := map[string]string{
		"none":                      "The exact route and effects are accepted for this generation.",
		"mixed_version":             "This build cannot expose the reviewed capture-quality contract. Recording stays fail-closed.",
		"permission_denied":         "Microphone permission is denied. Allow Pulsar in Windows Settings.",
		"no_device":                 "No usable microphone is available.",
		"reference_unavailable":     "The speaker render reference is not proven. Use headphones or explicitly allow degraded capture.",
		"reference_stale":           "The speaker render reference is stale. Stop or use headphones.",
		"route_unknown":             "The output route is ambiguous. Choose headphones or explicitly allow degraded capture.",
		"route_excluded":            "The resolved route does not match the selected capture mode.",
		"aec_unavailable":           "Echo cancellation is not verified on this Windows path.",
		"ns_unavailable":            "Noise suppression is unavailable.",
		"agc_unavailable":           "Input gain control is unavailable.",
		"silent":                    "No microphone signal is detected.",
		"too_quiet":                 "Microphone input is too quiet. Move closer or select another microphone.",
		"clipping":                  "Microphone input is clipping. Reduce input level or move farther away.",
		"clock_unstable":            "Capture and render clocks are unstable. Stop and retry on a local route.",
		"processor_overrun":         "Capture processing could not keep up. Audio capture was stopped.",
		"device_lost":               "The microphone or output device was disconnected.",
		"user_selected_unprocessed": "Processing is disabled for this capture.",
		"rearm_timeout":             "The route change could not be applied safely. Stop and retry.",
	}
	ru := map[string]string{
		"none":                      "Маршрут и эффекты приняты для этого поколения записи.",
		"mixed_version":             "Эта сборка не показывает проверенный контракт качества. Запись блокируется безопасно.",
		"permission_denied":         "Нет разрешения на микрофон. Разрешите Пульсар в настройках Windows.",
		"no_device":                 "Подходящий микрофон недоступен.",
		"reference_unavailable":     "Опорный звук динамиков не доказан. Используйте наушники или явно разрешите ограниченную запись.",
		"reference_stale":           "Опорный звук динамиков устарел. Остановите запись или используйте наушники.",
		"route_unknown":             "Маршрут выхода неоднозначен. Выберите наушники или явно разрешите ограниченную запись.",
		"route_excluded":            "Определённый маршрут не совпадает с выбранным режимом записи.",
		"aec_unavailable":           "Подавление эха не подтверждено для этого пути Windows.",
		"ns_unavailable":            "Подавление шума недоступно.",
		"agc_unavailable":           "Управление входным усилением недоступно.",
		"silent":                    "Сигнал микрофона не обнаружен.",
		"too_quiet":                 "Сигнал микрофона слишком тихий. Подойдите ближе или выберите другой микрофон.",
		"clipping":                  "Сигнал микрофона перегружен. Уменьшите уровень или отойдите дальше.",
		"clock_unstable":            "Часы записи и воспроизведения нестабильны. Остановите и повторите на локальном маршруте.",
		"processor_overrun":         "Обработка не успевает за записью. Запись звука остановлена.",
		"device_lost":               "Микрофон или устройство вывода отключено.",
		"user_selected_unprocessed": "Обработка для этой записи выключена.",
		"rearm_timeout":             "Не удалось безопасно применить смену маршрута. Остановите и повторите.",
	}
	if c.locale == ShellRussian {
		if value := ru[reason]; value != "" {
			return value
		}
		return "Состояние обработки неизвестно; запись не считается принятой."
	}
	if value := en[reason]; value != "" {
		return value
	}
	return "Processing state is unknown and is not treated as accepted."
}

func (c ShellCopy) CaptureEffect(state string) string {
	en := map[string]string{
		protocol.CaptureEffectActive: "active", protocol.CaptureEffectNotRequired: "not required on this route",
		protocol.CaptureEffectUnavailable: "unavailable", protocol.CaptureEffectFaulted: "failed during capture",
	}
	ru := map[string]string{
		protocol.CaptureEffectActive: "активно", protocol.CaptureEffectNotRequired: "не требуется для этого маршрута",
		protocol.CaptureEffectUnavailable: "недоступно", protocol.CaptureEffectFaulted: "сбой во время записи",
	}
	if c.locale == ShellRussian {
		return ru[state]
	}
	return en[state]
}

func (c ShellCopy) CaptureLifecycle(lifecycle string) string {
	en := map[string]string{
		protocol.CaptureLifecycleIdle:             "local capture idle",
		protocol.CaptureLifecyclePreparing:        "preparing local capture",
		protocol.CaptureLifecycleAwaitingFallback: "waiting for degraded-capture consent",
		protocol.CaptureLifecycleCapturing:        "capturing locally",
		protocol.CaptureLifecycleReconfiguring:    "applying route change",
		protocol.CaptureLifecycleStopping:         "stopping local capture",
		protocol.CaptureLifecycleFailed:           "capture stopped with an error",
	}
	ru := map[string]string{
		protocol.CaptureLifecycleIdle:             "ожидание локальной записи",
		protocol.CaptureLifecyclePreparing:        "подготовка локальной записи",
		protocol.CaptureLifecycleAwaitingFallback: "ожидание согласия на ограниченную запись",
		protocol.CaptureLifecycleCapturing:        "локальная запись",
		protocol.CaptureLifecycleReconfiguring:    "применение смены маршрута",
		protocol.CaptureLifecycleStopping:         "остановка локальной записи",
		protocol.CaptureLifecycleFailed:           "запись остановлена с ошибкой",
	}
	if c.locale == ShellRussian {
		return ru[lifecycle]
	}
	return en[lifecycle]
}

func (c ShellCopy) CaptureResolvedMode(mode string) string {
	if mode == protocol.CaptureRouteSpeaker || mode == protocol.CaptureRouteHeadphone {
		return c.CaptureQualityMode(WindowsCaptureQualityMode(mode))
	}
	if c.locale == ShellRussian {
		return "Неизвестный маршрут"
	}
	return "Unknown route"
}

func (c ShellCopy) CaptureQualityProjection(snapshot ShellSnapshot) string {
	presentation := presentWindowsCaptureQuality(snapshot)
	line := c.Text(txtCaptureQuality) + ": " + c.CaptureQualityLabel(presentation.Quality)
	line += "\r\n" + c.Text(txtCaptureMode) + ": " + c.CaptureQualityMode(presentation.Mode)
	line += " · " + c.CaptureResolvedMode(presentation.ResolvedMode)
	line += "\r\n" + c.CaptureLifecycle(presentation.Lifecycle)
	line += "\r\nAEC: " + c.CaptureEffect(presentation.AEC) + " · NS: " + c.CaptureEffect(presentation.NS) + " · AGC: " + c.CaptureEffect(presentation.AGC)
	line += fmt.Sprintf("\r\n%s: %.0f dBFS · %s: %.0f dBFS", c.Text(txtCaptureInputCeiling), presentation.InputCeilingDBFS, c.Text(txtCaptureOutputCeiling), presentation.ReceiverOutputCeilingDBFS)
	line += "\r\n" + c.CaptureQualityReason(presentation.Reason)
	line += "\r\n" + c.Text(txtCaptureCeilingHelp)
	if presentation.RequiresDegradedConsent {
		line += "\r\n[!] " + c.Text(txtCaptureConsentHelp)
	}
	return line
}

func (c ShellCopy) Presence(snapshot ShellSnapshot) string {
	if !snapshot.PresenceAvailable {
		return c.Connection(snapshot)
	}
	if c.locale == ShellRussian {
		return fmt.Sprintf("В сети %d из %d · готовы %d", snapshot.PresenceOnline, snapshot.PresenceTotal, snapshot.PresenceReady)
	}
	return fmt.Sprintf("%d of %d online · %d ready", snapshot.PresenceOnline, snapshot.PresenceTotal, snapshot.PresenceReady)
}

func (c ShellCopy) Body(section ShellSection, snapshot ShellSnapshot) string {
	switch section {
	case ShellCreate:
		return c.Text(txtCreateBody) + c.IdentityStatus(snapshot)
	case ShellJoin:
		return c.Text(txtJoinBody) + c.IdentityStatus(snapshot)
	case ShellTryLocally:
		quality := "\r\n\r\n" + c.CaptureQualityProjection(snapshot)
		if !snapshot.SelfTestAvailable {
			return c.Text(txtTryBody) + "\r\n\r\n" + c.Text(txtSelfTestUnavailable) + quality
		}
		return c.Text(txtTryBody) + "\r\n\r\n" + c.LocalSelfTest(snapshot) + quality
	case ShellHistory:
		if len(snapshot.PhaseOneHistory) == 0 {
			body := c.Text(txtNoHistory)
			if snapshot.PhaseOneActionOutcome != "" {
				body += "\r\n\r\n[+] " + c.PhaseOneActionMessage(snapshot.PhaseOneActionOutcome)
			}
			if snapshot.PhaseOneFailure != "" {
				body += "\r\n\r\n[!] " + c.PhaseOneActionMessage(snapshot.PhaseOneFailure)
			}
			return body
		}
		item := snapshot.PhaseOneHistory[snapshot.SelectedHistoryItem]
		body := c.HistoryItem(item, snapshot.SelectedHistoryItem+1, len(snapshot.PhaseOneHistory))
		if snapshot.PhaseOneActionOutcome != "" {
			body += "\r\n\r\n[+] " + c.PhaseOneActionMessage(snapshot.PhaseOneActionOutcome)
		}
		if snapshot.PhaseOneFailure != "" {
			body += "\r\n\r\n[!] " + c.PhaseOneActionMessage(snapshot.PhaseOneFailure)
		}
		return body
	case ShellSoundboard:
		return c.SoundboardProjection(snapshot)
	case ShellInbox:
		return c.TargetsInboxProjection(snapshot)
	case ShellAirs:
		return c.AirProjection(snapshot)
	case ShellAutomation:
		return c.AutomationProjection(snapshot)
	case ShellSettings:
		return c.Text(txtLanguage) + "\r\n\r\n" + c.Text(txtDND) + ": " + c.DND(snapshot.DND) +
			"\r\n" + c.Text(txtVolume) + fmt.Sprintf(": %d%%", snapshot.Volume) +
			"\r\n\r\n" + c.CaptureQualityProjection(snapshot) +
			"\r\n\r\n" + c.Text(txtIntegrations) + "\r\n" +
			"• " + c.Text(txtSpotifyOptional) + "\r\n• " + c.Text(txtTelegramOptional)
	default:
		if snapshot.Connection == ShellUnpaired {
			return c.Text(txtUnpairedHelp)
		}
		if snapshot.Connection != ShellOnline {
			return c.Text(txtDegradedHelp)
		}
		return ""
	}
}

func (c ShellCopy) AutomationProjection(snapshot ShellSnapshot) string {
	state := snapshot.Automation
	if !state.Available {
		if c.locale == ShellRussian {
			return "Управление автоматизацией недоступно. Конфигурация не раскрывается без текущих прав primary/control.\r\n\r\n[!] " + state.Failure
		}
		return "Automation administration is unavailable. Configuration is not disclosed without current primary/control authority.\r\n\r\n[!] " + state.Failure
	}
	featureState := "enabled"
	if !state.Feature.AutomationEnabled {
		featureState = "disabled"
	}
	if state.Feature.EmergencyDisabled {
		featureState = "emergency disabled"
	}
	body := "Automation: " + featureState + " · soundboard " + map[bool]string{true: "enabled", false: "disabled"}[state.Feature.SoundboardEnabled] +
		"\r\nPolicy: " + state.Feature.Timezone + fmt.Sprintf(" · %d quiet-hour windows · revision %d", len(state.Feature.QuietHours), state.Feature.Revision) +
		"\r\nEditor: name · IANA timezone · Sun,Mon,… weekdays · HH:MM · quiet windows like Mon 22:00-06:00; Tue 12:00-13:00."
	if c.locale == ShellRussian {
		featureState = map[string]string{"enabled": "включена", "disabled": "выключена", "emergency disabled": "аварийно выключена"}[featureState]
		body = "Автоматизация: " + featureState + " · soundboard " + map[bool]string{true: "включён", false: "выключен"}[state.Feature.SoundboardEnabled] +
			"\r\nПолитика: " + state.Feature.Timezone + fmt.Sprintf(" · окон тишины %d · ревизия %d", len(state.Feature.QuietHours), state.Feature.Revision) +
			"\r\nРедактор: имя · IANA timezone · дни Sun,Mon,… · HH:MM · окна тишины Mon 22:00-06:00; Tue 12:00-13:00."
	}
	if len(state.Schedules) == 0 {
		if c.locale == ShellRussian {
			body += "\r\n\r\nРасписаний нет. Новое расписание создаётся выключенным."
		} else {
			body += "\r\n\r\nNo schedules. A new schedule is created disarmed."
		}
	} else {
		item := state.Schedules[state.SelectedSchedule]
		status := map[bool]string{true: "enabled", false: "disabled"}[item.Schedule.Enabled]
		next := "no next run while disabled"
		if item.NextRunAvailable {
			next = item.NextRun.In(time.Local).Format("2006-01-02 15:04 MST")
		}
		if item.QuietHoursSkip {
			next += " · skipped by quiet hours"
		}
		line := fmt.Sprintf("\r\n\r\nSchedule %d/%d: %s · %s\r\n%s %s · next: %s", state.SelectedSchedule+1, len(state.Schedules), item.Schedule.DisplayName, status, item.Schedule.Timezone, item.Schedule.LocalTime, next)
		if c.locale == ShellRussian {
			line = strings.NewReplacer("Schedule ", "Расписание ", " · enabled", " · включено", " · disabled", " · выключено", " · next: ", " · следующий запуск: ", "no next run while disabled", "нет запуска, пока выключено", " · skipped by quiet hours", " · будет пропущено из-за часов тишины").Replace(line)
		}
		body += line
	}
	if len(state.Principals) == 0 {
		if c.locale == ShellRussian {
			body += "\r\n\r\nScoped principals отсутствуют."
		} else {
			body += "\r\n\r\nNo scoped principals."
		}
	} else {
		principal := state.Principals[state.SelectedPrincipal]
		status := "active"
		if !principal.RevokedAt.IsZero() {
			status = "revoked"
		} else if !principal.DisabledAt.IsZero() {
			status = "disabled"
		} else if !principal.ExpiresAt.After(time.Now()) {
			status = "expired"
		}
		line := fmt.Sprintf("\r\n\r\nPrincipal %d/%d: %s · %s\r\nScopes: %d cues · %d audiences · max %d targets · expires %s", state.SelectedPrincipal+1, len(state.Principals), principal.DisplayName, status, len(principal.AllowedCueIDs), len(principal.AllowedAudiences), principal.MaxTargetCount, principal.ExpiresAt.Local().Format("2006-01-02 15:04 MST"))
		if c.locale == ShellRussian {
			line = strings.NewReplacer("Scopes:", "Права:", " cues · ", " звуков · ", " audiences · max ", " аудиторий · максимум ", " targets · expires ", " получателей · истекает ").Replace(line)
		}
		body += line
	}
	if len(state.History) > 0 {
		item := state.History[state.SelectedHistory]
		attribution := "manual"
		if item.Automation != nil {
			attribution = item.Automation.TriggerKind + " · " + item.Automation.CueLabel
			if item.Automation.PrincipalLabel != "" {
				attribution += " · " + item.Automation.PrincipalLabel
			}
			if item.Automation.ScheduleLabel != "" {
				attribution += " · " + item.Automation.ScheduleLabel
			}
		}
		body += fmt.Sprintf("\r\n\r\nHistory %d/%d: %s · %s · %s", state.SelectedHistory+1, len(state.History), item.Title, item.Status, attribution)
	}
	if state.SecretAvailable {
		if c.locale == ShellRussian {
			body += "\r\n\r\n[!] Одноразовый secret готов: скопируйте или скройте. Значение исключено из текста и accessibility."
		} else {
			body += "\r\n\r\n[!] One-time secret is ready: copy or hide it. The value is excluded from text and accessibility."
		}
	}
	if c.locale == ShellRussian {
		body += "\r\n\r\nDST: весенний пропуск не запускается; при осеннем повторе запускается только первое UTC-соответствие. Ручной Soundboard остаётся отдельным и доступен при выключенной automation."
	} else {
		body += "\r\n\r\nDST: a spring-forward gap does not run; a fall-back fold runs only its first UTC mapping. Manual Soundboard stays separate and available while automation is disabled."
	}
	if state.Busy {
		body += "\r\n\r\n[~] Updating…"
	}
	if state.ConfirmAction != "" {
		body += "\r\n\r\n[!] Confirm: " + state.ConfirmAction
	}
	if state.Outcome != "" {
		body += "\r\n\r\n[+] " + state.Outcome
	}
	if state.Failure != "" {
		body += "\r\n\r\n[!] " + state.Failure
	}
	return body
}

func (c ShellCopy) IdentityStatus(snapshot ShellSnapshot) string {
	if snapshot.IdentityOperation == ShellIdentityIdle && snapshot.IdentityFailure == "" {
		return ""
	}
	labelsEN := map[ShellIdentityOperation]string{
		ShellIdentityWorking: "Request in progress", ShellIdentityRecoveryRequired: "Save the recovery file before this installation becomes active",
		ShellIdentityActive: "Identity saved securely", ShellIdentityFailed: "Identity request failed",
	}
	labelsRU := map[ShellIdentityOperation]string{
		ShellIdentityWorking: "Запрос выполняется", ShellIdentityRecoveryRequired: "Сохраните файл восстановления до активации установки",
		ShellIdentityActive: "Идентификатор защищённо сохранён", ShellIdentityFailed: "Ошибка запроса идентификатора",
	}
	label := labelsEN[snapshot.IdentityOperation]
	if c.locale == ShellRussian {
		label = labelsRU[snapshot.IdentityOperation]
	}
	if snapshot.IdentityFailure != "" {
		label += ": " + snapshot.IdentityFailure
	}
	return "\r\n\r\n" + label
}

func (c ShellCopy) Draft(snapshot ShellSnapshot) string {
	if len(snapshot.PhaseOneDrafts) == 0 {
		if c.locale == ShellRussian {
			return "Нет черновика для отправки"
		}
		return "No outgoing draft"
	}
	draft := snapshot.PhaseOneDrafts[snapshot.SelectedPhaseOneDraft]
	line := draft.Title + " — " + string(draft.State)
	line += "\r\n" + c.Route(snapshot.SelectedPhaseOneRoute) + " · " + c.Delivery(snapshot.SelectedPhaseOneDelivery)
	if draft.RequestedDelivery != "" {
		line += "\r\n" + c.requestedLabel() + ": " + c.Delivery(draft.RequestedDelivery)
	}
	if draft.EffectiveDelivery != "" {
		line += " · " + c.effectiveLabel() + ": " + c.Delivery(draft.EffectiveDelivery)
	}
	if draft.DowngradeReason != "" {
		line += "\r\n[~] " + draft.DowngradeReason
	}
	if draft.FailureCode != "" {
		line += "\r\n[!] " + draft.FailureCode
	}
	if draft.ExplicitTargetCount > 0 {
		if c.locale == ShellRussian {
			line += fmt.Sprintf("\r\nТочный повтор: %d получателей", draft.ExplicitTargetCount)
		} else {
			line += fmt.Sprintf("\r\nExact retry: %d recipients", draft.ExplicitTargetCount)
		}
	}
	return line
}

func (c ShellCopy) StreamTrackProjection(snapshot ShellSnapshot) string {
	projection := snapshot.StreamTrack
	heading := "Long track"
	if c.locale == ShellRussian {
		heading = "Длинный трек"
	}
	line := heading + ": " + projection.StateLabel.Text(c.locale)
	if projection.StateLabel.Text(c.locale) == "" {
		line = heading + ": " + string(projection.State)
	}
	if projection.Draft == nil {
		if c.locale == ShellRussian {
			line += " · файл не выбран"
		} else {
			line += " · no file selected"
		}
	} else {
		draft := projection.Draft
		phase := draft.PhaseLabel.Text(c.locale)
		if phase == "" {
			phase = string(draft.Phase)
		}
		line += "\r\n" + draft.Title + " · " + phase
		line += fmt.Sprintf(" · %d/%d bytes", draft.UploadOffset, draft.LocalByteCount)
		if draft.Phase == StreamTrackDraftProcessing {
			line += fmt.Sprintf(" · %d%%", draft.ProcessingPercent)
		}
	}
	playback := projection.Playback.PhaseLabel.Text(c.locale)
	if playback == "" {
		playback = string(projection.Playback.Phase)
	}
	line += fmt.Sprintf("\r\n%s · %d/%d ms", playback, projection.Playback.AudiblePositionMS, projection.Playback.DurationMS)
	if projection.Failure != "" {
		failure := projection.FailureLabel.Text(c.locale)
		if failure == "" {
			failure = string(projection.Failure)
		}
		line += "\r\n[!] " + failure
	}
	if snapshot.StreamTrackOutcome != "" {
		line += "\r\n" + snapshot.StreamTrackOutcome
	}
	return line
}

func (c ShellCopy) Route(route PhaseOneRoute) string {
	en := map[PhaseOneRoute]string{PhaseOneThisPulsar: "This Pulsar", PhaseOneOwnBarycenter: "My Barycenter", PhaseOneCurrentAir: "Current air"}
	ru := map[PhaseOneRoute]string{PhaseOneThisPulsar: "Этот Пульсар", PhaseOneOwnBarycenter: "Мой Барицентр", PhaseOneCurrentAir: "Текущий эфир"}
	if c.locale == ShellRussian {
		return ru[route]
	}
	return en[route]
}

func (c ShellCopy) Delivery(delivery PhaseOneDelivery) string {
	en := map[PhaseOneDelivery]string{PhaseOneOverlay: "Overlay", PhaseOneInterrupt: "Interrupt", PhaseOneAfterCurrent: "After current"}
	ru := map[PhaseOneDelivery]string{PhaseOneOverlay: "Поверх", PhaseOneInterrupt: "Прервать", PhaseOneAfterCurrent: "После текущего"}
	if c.locale == ShellRussian {
		return ru[delivery]
	}
	return en[delivery]
}

func (c ShellCopy) HistoryItem(item ShellPhaseOneHistoryItem, index, count int) string {
	line := fmt.Sprintf("%d/%d · %s — %s", index, count, item.Title, item.Status)
	if item.SenderName != "" {
		line += "\r\n" + item.SenderName
	}
	if item.RequestedDelivery != "" {
		line += "\r\n" + c.requestedLabel() + ": " + c.Delivery(PhaseOneDelivery(item.RequestedDelivery))
	}
	if item.EffectiveDelivery != "" {
		line += " · " + c.effectiveLabel() + ": " + c.Delivery(PhaseOneDelivery(item.EffectiveDelivery))
	}
	if item.DowngradeReason != "" {
		line += "\r\n[~] " + item.DowngradeReason
	}
	line += fmt.Sprintf("\r\nplayed %d · other %d", item.PlayedCount, item.OtherCount)
	if item.AutomationTrigger != "" {
		line += "\r\nAutomation: " + item.AutomationTrigger
		if item.AutomationCue != "" {
			line += " · " + item.AutomationCue
		}
		if item.AutomationActor != "" {
			line += "\r\nBy: " + item.AutomationActor
		}
		if item.AutomationSchedule != "" {
			line += " · schedule: " + item.AutomationSchedule
		}
		if item.AutomationReason != "" {
			line += "\r\n[!] " + item.AutomationReason
		}
		var controls []string
		if item.CanDisableSchedule {
			controls = append(controls, "disable schedule")
		}
		if item.CanRevokePrincipal {
			controls = append(controls, "revoke principal")
		}
		if item.CanEmergencyDisable {
			controls = append(controls, "emergency disable")
		}
		if len(controls) > 0 {
			line += "\r\nQuick controls: " + strings.Join(controls, " · ")
		}
	}
	return line
}

func (c ShellCopy) SoundboardProjection(snapshot ShellSnapshot) string {
	if len(snapshot.SoundboardCues) == 0 {
		if c.locale == ShellRussian {
			return "Нет доступных звуков. Встроенный сигнал появится после включения soundboard владельцем.\r\n\r\n[!] " + snapshot.SoundboardFailure
		}
		return "No soundboard cues are available. The built-in cue appears after an owner enables soundboard.\r\n\r\n[!] " + snapshot.SoundboardFailure
	}
	cue := snapshot.SoundboardCues[snapshot.SelectedSoundboardCue]
	shortcut := cue.ShortcutLabel
	if shortcut == "" {
		shortcut = "button only"
		if c.locale == ShellRussian {
			shortcut = "только кнопка"
		}
	}
	result := "Cue " + fmt.Sprintf("%d/%d", snapshot.SelectedSoundboardCue+1, len(snapshot.SoundboardCues)) + ": " + cue.Title +
		"\r\nSource: " + cue.SourceKind + "\r\nShortcut: " + shortcut + " (" + string(cue.ShortcutStatus) + ")" +
		"\r\nRoute: " + c.Route(snapshot.SoundboardRoute) + "\r\nDelivery: " + c.Delivery(snapshot.SoundboardDelivery) +
		fmt.Sprintf("\r\nInclude this Pulsar: %t\r\nAutomation history items: %d", snapshot.SoundboardIncludeOrigin, snapshot.SoundboardHistoryCount)
	if c.locale == ShellRussian {
		result = "Звук " + fmt.Sprintf("%d/%d", snapshot.SelectedSoundboardCue+1, len(snapshot.SoundboardCues)) + ": " + cue.Title +
			"\r\nИсточник: " + cue.SourceKind + "\r\nГорячая клавиша: " + shortcut + " (" + string(cue.ShortcutStatus) + ")" +
			"\r\nМаршрут: " + c.Route(snapshot.SoundboardRoute) + "\r\nДоставка: " + c.Delivery(snapshot.SoundboardDelivery) +
			fmt.Sprintf("\r\nВключая этот Пульсар: %t\r\nЗаписей automation в истории: %d", snapshot.SoundboardIncludeOrigin, snapshot.SoundboardHistoryCount)
	}
	if snapshot.SoundboardOutcome != "" {
		result += "\r\n\r\n[+] " + snapshot.SoundboardOutcome
	}
	if snapshot.SoundboardFailure != "" {
		result += "\r\n\r\n[!] " + snapshot.SoundboardFailure
	}
	return result
}

func (c ShellCopy) ModerationReason(reason PhaseOneModerationReason) string {
	en := map[PhaseOneModerationReason]string{
		PhaseOneReportSpam: "Spam", PhaseOneReportHarassment: "Harassment",
		PhaseOneReportIllegal: "Illegal content", PhaseOneReportSexualContent: "Sexual content",
		PhaseOneReportViolence: "Violence", PhaseOneReportOther: "Other",
	}
	ru := map[PhaseOneModerationReason]string{
		PhaseOneReportSpam: "Спам", PhaseOneReportHarassment: "Преследование",
		PhaseOneReportIllegal: "Незаконный контент", PhaseOneReportSexualContent: "Сексуальный контент",
		PhaseOneReportViolence: "Насилие", PhaseOneReportOther: "Другое",
	}
	if c.locale == ShellRussian {
		return ru[reason]
	}
	return en[reason]
}

func (c ShellCopy) PhaseOneActionMessage(code string) string {
	en := map[string]string{
		"media_deleted":           "Media deleted. It can no longer be replayed.",
		"report_received":         "Report received for moderation.",
		"report_already_received": "This item was already reported; the existing report remains active.",
		"sender_blocked":          "Sender blocked. New deliveries from this sender are stopped.",
		"sender_already_blocked":  "Sender was already blocked.",
		"replay_accepted":         "Replay accepted.", "replay_already_accepted": "Replay was already accepted.",
		"inbox_dismissed":            "Inbox item dismissed.",
		"action_not_allowed":         "This action is not available for the selected item.",
		"history_action_unavailable": "The item changed and this action is no longer available.",
		"coordinator_unavailable":    "Cannot reach the coordinator. Check the connection and try again.",
		"unauthorized":               "Your current account is not allowed to perform this action.",
		"forbidden":                  "Your current account is not allowed to perform this action.",
		"insufficient_capability":    "Your current account is not allowed to perform this action.",
		"invalid_request":            "Check the report details and try again.",
	}
	ru := map[string]string{
		"media_deleted":           "Медиа удалено. Его больше нельзя повторно воспроизвести.",
		"report_received":         "Жалоба принята на модерацию.",
		"report_already_received": "На этот материал уже подана жалоба; существующая жалоба остаётся активной.",
		"sender_blocked":          "Отправитель заблокирован. Новые доставки от него остановлены.",
		"sender_already_blocked":  "Отправитель уже был заблокирован.",
		"replay_accepted":         "Повтор принят.", "replay_already_accepted": "Повтор уже был принят.",
		"inbox_dismissed":            "Входящий материал убран.",
		"action_not_allowed":         "Это действие недоступно для выбранного материала.",
		"history_action_unavailable": "Материал изменился, и действие больше недоступно.",
		"coordinator_unavailable":    "Нет связи с координатором. Проверьте подключение и повторите попытку.",
		"unauthorized":               "Текущей учётной записи это действие недоступно.",
		"forbidden":                  "Текущей учётной записи это действие недоступно.",
		"insufficient_capability":    "Текущей учётной записи это действие недоступно.",
		"invalid_request":            "Проверьте сведения жалобы и повторите попытку.",
	}
	if c.locale == ShellRussian {
		if message := ru[code]; message != "" {
			return message
		}
		return "Не удалось выполнить действие. Повторите попытку."
	}
	if message := en[code]; message != "" {
		return message
	}
	return "The action failed. Try again."
}

func (c ShellCopy) TargetsInboxProjection(snapshot ShellSnapshot) string {
	projection := snapshot.TargetsInbox
	state := projection.StateLabel.Text(c.locale)
	if strings.TrimSpace(state) == "" {
		state = targetsStateLabel(projection.State).Text(c.locale)
	}
	audience := "—"
	for _, choice := range projection.AvailableAudiences {
		if choice.Kind == projection.SelectedAudience {
			audience = choice.Label.Text(c.locale)
			break
		}
	}
	policy := projection.ContentPolicyState
	track := projection.TargetedTrackPolicy
	if c.locale == ShellRussian {
		if policy == "current" {
			policy = "принята текущая версия"
		} else if policy == "required" {
			policy = "требуется принятие"
		} else {
			policy = "требует обновления"
		}
		if track == "unsupported" {
			track = "очередь/замена недоступны до поддержки потоковых треков"
		}
	} else {
		if policy == "current" {
			policy = "current version accepted"
		} else if policy == "required" {
			policy = "acceptance required"
		} else {
			policy = "update required"
		}
		if track == "unsupported" {
			track = "queue/replace unavailable until streamed tracks are supported"
		}
	}
	var body string
	if c.locale == ShellRussian {
		body = "Состояние: " + state + "\r\nПолучатели: " + audience + fmt.Sprintf(" · выбрано %d", len(projection.SelectedReferences)) +
			" · origin " + map[bool]string{true: "включён", false: "не включён"}[projection.IncludeOrigin] + "\r\nПолитика: " + policy + " · " + track
	} else {
		body = "State: " + state + "\r\nAudience: " + audience + fmt.Sprintf(" · %d selected", len(projection.SelectedReferences)) +
			" · origin " + map[bool]string{true: "included", false: "not included"}[projection.IncludeOrigin] + "\r\nPolicy: " + policy + " · " + track
	}
	if len(projection.Targets) > 0 {
		target := projection.Targets[snapshot.SelectedTarget]
		selected := targetIsSelected(projection, target.Reference)
		if c.locale == ShellRussian {
			body += fmt.Sprintf("\r\n\r\nПолучатель %d/%d: %s · %s · %s", snapshot.SelectedTarget+1, len(projection.Targets), target.Label.Text(c.locale), target.CapabilityState, map[bool]string{true: "выбран", false: "не выбран"}[selected])
		} else {
			body += fmt.Sprintf("\r\n\r\nTarget %d/%d: %s · %s · %s", snapshot.SelectedTarget+1, len(projection.Targets), target.Label.Text(c.locale), target.CapabilityState, map[bool]string{true: "selected", false: "not selected"}[selected])
		}
		body += "\r\n" + strings.Join(target.Capabilities, ", ")
	}
	if len(projection.Inbox) > 0 {
		item := projection.Inbox[snapshot.SelectedInbox]
		if c.locale == ShellRussian {
			body += fmt.Sprintf("\r\n\r\nВходящие %d/%d: %s · %s\r\n%s · %s · %s · %s", snapshot.SelectedInbox+1, len(projection.Inbox), item.Title, item.Availability, item.Sender.Text(c.locale), item.Source.Text(c.locale), item.EffectiveDelivery.Text(c.locale), item.Receipt.Text(c.locale))
		} else {
			body += fmt.Sprintf("\r\n\r\nInbox %d/%d: %s · %s\r\n%s · %s · %s · %s", snapshot.SelectedInbox+1, len(projection.Inbox), item.Title, item.Availability, item.Sender.Text(c.locale), item.Source.Text(c.locale), item.EffectiveDelivery.Text(c.locale), item.Receipt.Text(c.locale))
		}
	}
	if len(projection.History) > 0 {
		item := projection.History[snapshot.SelectedTargetsHistory]
		if c.locale == ShellRussian {
			body += fmt.Sprintf("\r\n\r\nИстория %d/%d: %s · %s · проиграно %d · прочее %d", snapshot.SelectedTargetsHistory+1, len(projection.History), item.Title, item.Status.Text(c.locale), item.Played, item.Other)
		} else {
			body += fmt.Sprintf("\r\n\r\nHistory %d/%d: %s · %s · played %d · other %d", snapshot.SelectedTargetsHistory+1, len(projection.History), item.Title, item.Status.Text(c.locale), item.Played, item.Other)
		}
		for _, receipt := range item.ReceiptPage.Items {
			body += "\r\n• " + receipt.TargetLabel + ": " + receipt.Status.Text(c.locale)
		}
	}
	if snapshot.TargetsInboxBusy {
		if c.locale == ShellRussian {
			body += "\r\n\r\n[~] Выполняется действие…"
		} else {
			body += "\r\n\r\n[~] Action in progress…"
		}
	}
	if snapshot.TargetsInboxActionOutcome != "" {
		body += "\r\n\r\n[+] " + c.PhaseOneActionMessage(snapshot.TargetsInboxActionOutcome)
	}
	if snapshot.TargetsInboxFailure != "" {
		body += "\r\n\r\n[!] " + c.PhaseOneActionMessage(snapshot.TargetsInboxFailure)
	}
	body += "\r\n\r\n" + c.Draft(snapshot)
	return body
}

func (c ShellCopy) AirProjection(snapshot ShellSnapshot) string {
	var body string
	if snapshot.PendingAirJoin != nil {
		pending := snapshot.PendingAirJoin
		if c.locale == ShellRussian {
			body = "Ожидает подтверждения: " + pending.Title + fmt.Sprintf("\r\nУчастники: %d из %d · роль: %s", pending.MemberCount, pending.Capacity.Barycenters, c.AirRole(pending.Role))
		} else {
			body = "Pending confirmation: " + pending.Title + fmt.Sprintf("\r\nMembers: %d of %d · role: %s", pending.MemberCount, pending.Capacity.Barycenters, c.AirRole(pending.Role))
		}
		if pending.OwnerDisplayName != "" {
			if c.locale == ShellRussian {
				body += "\r\nВладелец: " + pending.OwnerDisplayName
			} else {
				body += "\r\nOwner: " + pending.OwnerDisplayName
			}
		}
		if pending.ActivationWouldSwitch {
			if c.locale == ShellRussian {
				body += "\r\nАктивация сменит текущий эфир и требует отдельного подтверждения."
			} else {
				body += "\r\nActivation will switch the current Air and requires separate confirmation."
			}
		}
	} else if len(snapshot.Airs) == 0 {
		if c.locale == ShellRussian {
			body = "Нет сохранённых эфиров. Создайте эфир или введите одноразовое приглашение."
		} else {
			body = "No saved Airs. Create one or enter a one-time invite."
		}
	} else {
		air := snapshot.Airs[snapshot.SelectedAir]
		current := ""
		if air.Current {
			if c.locale == ShellRussian {
				current = " · текущий"
			} else {
				current = " · current"
			}
		}
		if c.locale == ShellRussian {
			body = fmt.Sprintf("%d из %d: %s%s\r\nРоль: %s · участников %d из %d, активны %d · Пульсаров онлайн %d из %d",
				snapshot.SelectedAir+1, len(snapshot.Airs), air.Title, current, c.AirRole(air.Role),
				air.MemberCount, air.Capacity.Barycenters, air.ActiveMemberCount, air.OnlinePulsarCount, air.Capacity.OnlinePulsars)
			body += "\r\nПолитики: приглашения " + c.AirInvitePolicy(air.Policy.Invite) + " · поверх " + c.AirPlaybackPolicy(air.Policy.Overlay) + " · очередь " + c.AirPlaybackPolicy(air.Policy.Queue) + " · замена " + c.AirPlaybackPolicy(air.Policy.Replace)
		} else {
			body = fmt.Sprintf("%d of %d: %s%s\r\nRole: %s · %d of %d members, %d active · %d of %d Pulsars online",
				snapshot.SelectedAir+1, len(snapshot.Airs), air.Title, current, c.AirRole(air.Role),
				air.MemberCount, air.Capacity.Barycenters, air.ActiveMemberCount, air.OnlinePulsarCount, air.Capacity.OnlinePulsars)
			body += "\r\nPolicies: invite " + c.AirInvitePolicy(air.Policy.Invite) + " · overlay " + c.AirPlaybackPolicy(air.Policy.Overlay) + " · queue " + c.AirPlaybackPolicy(air.Policy.Queue) + " · replace " + c.AirPlaybackPolicy(air.Policy.Replace)
		}
		if air.Current {
			playing := snapshot.NowPlaying
			if playing == "" {
				playing = c.Text(txtSilence)
			}
			if c.locale == ShellRussian {
				body += "\r\nТекущее воспроизведение: " + playing + ". Смена или отключение эфира меняет Air-маршрутизацию."
			} else {
				body += "\r\nCurrent playback: " + playing + ". Switching or deactivating changes Air routing."
			}
		}
	}
	if snapshot.AirInviteAvailable {
		expires := ""
		if !snapshot.AirInviteExpires.IsZero() {
			expires = snapshot.AirInviteExpires.Local().Format("2006-01-02 15:04 MST")
		}
		if c.locale == ShellRussian {
			body += "\r\n\r\n[!] Одноразовое приглашение готово. Явно скопируйте или скройте его; в журнал и подписи оно не попадает."
			if expires != "" {
				body += " Срок: " + expires + "."
			}
		} else {
			body += "\r\n\r\n[!] One-time invite ready. Explicitly copy or hide it; it is excluded from logs and labels."
			if expires != "" {
				body += " Expires: " + expires + "."
			}
		}
	}
	if snapshot.AirBusy {
		if c.locale == ShellRussian {
			body += "\r\n\r\n[~] Обновление эфира…"
		} else {
			body += "\r\n\r\n[~] Updating Air…"
		}
	}
	if snapshot.AirConfirmAction != "" {
		body += "\r\n\r\n[!] " + c.AirConfirmation(snapshot.AirConfirmAction)
	}
	if snapshot.AirOutcome != "" {
		body += "\r\n\r\n[+] " + c.AirActionMessage(snapshot.AirOutcome)
	}
	if snapshot.AirFailure != "" {
		body += "\r\n\r\n[!] " + c.AirActionMessage(snapshot.AirFailure)
	}
	return body
}

func (c ShellCopy) AirRole(role AirRole) string {
	en := map[AirRole]string{AirRoleOwner: "owner", AirRoleAdmin: "admin", AirRoleMember: "member"}
	ru := map[AirRole]string{AirRoleOwner: "владелец", AirRoleAdmin: "администратор", AirRoleMember: "участник"}
	if c.locale == ShellRussian {
		return ru[role]
	}
	return en[role]
}

func (c ShellCopy) AirInvitePolicy(policy AirInvitePolicy) string {
	en := map[AirInvitePolicy]string{AirInviteOwnerPrimary: "owner", AirInviteAdminPrimary: "admins", AirInviteAllMemberPrimarys: "all members"}
	ru := map[AirInvitePolicy]string{AirInviteOwnerPrimary: "владелец", AirInviteAdminPrimary: "администраторы", AirInviteAllMemberPrimarys: "все участники"}
	if c.locale == ShellRussian {
		return ru[policy]
	}
	return en[policy]
}

func (c ShellCopy) AirPlaybackPolicy(policy AirPlaybackPolicy) string {
	en := map[AirPlaybackPolicy]string{AirPlaybackOwnerPrimary: "owner", AirPlaybackAdminPrimary: "admins", AirPlaybackAllMemberPrimarys: "all members", AirPlaybackPrimaryCompanion: "primary companion", AirPlaybackDisabled: "disabled"}
	ru := map[AirPlaybackPolicy]string{AirPlaybackOwnerPrimary: "владелец", AirPlaybackAdminPrimary: "администраторы", AirPlaybackAllMemberPrimarys: "все участники", AirPlaybackPrimaryCompanion: "компаньон primary", AirPlaybackDisabled: "выключено"}
	if c.locale == ShellRussian {
		return ru[policy]
	}
	return en[policy]
}

func (c ShellCopy) AirConfirmation(action string) string {
	en := map[string]string{"switch": "Confirm switching the active Air. Current playback authority changes immediately.", "deactivate": "Confirm deactivating this Air. Current Air playback stops.", "leave": "Confirm leaving this Air. Its saved history remains governed by server policy.", "dissolve": "Confirm dissolving this Air for every member. This cannot be undone.", "join_switch": "Confirm joining and switching the active Air."}
	ru := map[string]string{"switch": "Подтвердите смену активного эфира. Источник управления воспроизведением изменится сразу.", "deactivate": "Подтвердите отключение эфира. Текущее воспроизведение эфира остановится.", "leave": "Подтвердите выход из эфира. История остаётся под политикой сервера.", "dissolve": "Подтвердите роспуск эфира для всех участников. Это необратимо.", "join_switch": "Подтвердите присоединение и смену активного эфира."}
	if c.locale == ShellRussian {
		return ru[action]
	}
	return en[action]
}

func (c ShellCopy) AirActionMessage(code string) string {
	en := map[string]string{
		"created": "Air created and saved.", "invite_reviewed": "Invite accepted for review; confirm before joining.",
		"join_confirmed": "Air membership confirmed.", "join_declined": "Pending membership declined.",
		"invite_issued": "One-time invite created.", "invite_withdrawn": "Invite withdrawn.",
		"activated": "Active Air changed.", "deactivated": "Air deactivated.", "left": "You left the Air.",
		"dissolved": "Air dissolved.", "policy_updated": "Air policy updated.",
		"membership_confirmation_required": "Review the pending membership first.",
		"invite_unavailable":               "This invite is expired, used, withdrawn, or unavailable. Ask for a new one.",
		"air_barycenter_capacity_reached":  "This Air already has eight Barycenters.", "air_online_pulsar_capacity_reached": "Activation would exceed the online Pulsar capacity.",
		"revision_conflict":  "The Air changed elsewhere. The latest state is being loaded.",
		"active_air_changed": "The active Air changed elsewhere. Review the latest state.", "air_dissolved": "This Air was dissolved.",
		"already_member": "This identity already has a saved or pending membership.", "owner_transfer_required": "Transfer ownership or dissolve the Air before leaving.",
		"air_parked": "This Air is saved but not active for playback.", "policy_denied": "The current Air policy denies this action.",
		"idempotency_conflict": "This retry key was already used for a different Air action.", "membership_not_found": "This membership is no longer available.", "air_not_found": "This Air is no longer available.",
		"coordinator_unavailable": "Cannot reach the coordinator. Check the connection and try again.",
		"credential_unavailable":  "Air management is unavailable until this installation has an active control identity.",
		"invalid_request":         "Check the title or invite and try again.", "invalid_response": "The coordinator returned an invalid Air response.",
		"redirect_rejected": "A redirected Air request was rejected for safety.", "response_too_large": "The Air response exceeded the safe limit.",
		"forbidden": "Your Air role does not permit this action.", "unauthenticated": "Your active control identity is no longer authenticated.",
		"clipboard_copied": "Invite copied; it will be cleared automatically if unchanged.", "clipboard_failed": "The invite could not be copied safely.",
	}
	ru := map[string]string{
		"created": "Эфир создан и сохранён.", "invite_reviewed": "Приглашение принято для проверки; подтвердите вступление.",
		"join_confirmed": "Участие в эфире подтверждено.", "join_declined": "Ожидающее участие отклонено.",
		"invite_issued": "Одноразовое приглашение создано.", "invite_withdrawn": "Приглашение отозвано.",
		"activated": "Активный эфир изменён.", "deactivated": "Эфир отключён.", "left": "Вы вышли из эфира.",
		"dissolved": "Эфир распущен.", "policy_updated": "Политика эфира обновлена.",
		"membership_confirmation_required": "Сначала проверьте ожидающее участие.",
		"invite_unavailable":               "Приглашение истекло, использовано, отозвано или недоступно. Запросите новое.",
		"air_barycenter_capacity_reached":  "В эфире уже восемь Барицентров.", "air_online_pulsar_capacity_reached": "Активация превысит лимит Пульсаров онлайн.",
		"revision_conflict":  "Эфир изменён в другом месте. Загружается актуальное состояние.",
		"active_air_changed": "Активный эфир изменён в другом месте. Проверьте актуальное состояние.", "air_dissolved": "Этот эфир распущен.",
		"already_member": "У identity уже есть сохранённое или ожидающее участие.", "owner_transfer_required": "Перед выходом передайте владение или распустите эфир.",
		"air_parked": "Эфир сохранён, но не активен для воспроизведения.", "policy_denied": "Текущая политика эфира запрещает действие.",
		"idempotency_conflict": "Ключ повтора уже использован для другого действия.", "membership_not_found": "Участие больше недоступно.", "air_not_found": "Эфир больше недоступен.",
		"coordinator_unavailable": "Нет связи с координатором. Проверьте подключение и повторите.",
		"credential_unavailable":  "Управление эфирами недоступно без активного control identity.",
		"invalid_request":         "Проверьте название или приглашение и повторите.", "invalid_response": "Координатор вернул некорректный ответ эфира.",
		"redirect_rejected": "Перенаправленный запрос эфира отклонён из соображений безопасности.", "response_too_large": "Ответ эфира превысил безопасный размер.",
		"forbidden": "Ваша роль в эфире не разрешает это действие.", "unauthenticated": "Активный control identity больше не аутентифицирован.",
		"clipboard_copied": "Приглашение скопировано и будет автоматически очищено, если не изменится.", "clipboard_failed": "Не удалось безопасно скопировать приглашение.",
	}
	if c.locale == ShellRussian {
		if message := ru[code]; message != "" {
			return message
		}
		return "Не удалось выполнить действие. Состояние не было принято как успешное."
	}
	if message := en[code]; message != "" {
		return message
	}
	return "The action failed and was not treated as successful."
}

func (c ShellCopy) requestedLabel() string {
	if c.locale == ShellRussian {
		return "запрошено"
	}
	return "requested"
}

func (c ShellCopy) effectiveLabel() string {
	if c.locale == ShellRussian {
		return "фактически"
	}
	return "effective"
}

func (c ShellCopy) LocalSelfTest(snapshot ShellSnapshot) string {
	input := "Default input"
	if c.locale == ShellRussian {
		input = "Вход по умолчанию"
	}
	if snapshot.SelectedCaptureInput >= 0 && snapshot.SelectedCaptureInput < len(snapshot.CaptureInputs) {
		candidate := snapshot.CaptureInputs[snapshot.SelectedCaptureInput].Name
		if candidate != "" {
			input = candidate
		}
	}
	output := snapshot.RouteName
	if snapshot.SelectedAudioOutput >= 0 && snapshot.SelectedAudioOutput < len(snapshot.AudioOutputs) {
		output = snapshot.AudioOutputs[snapshot.SelectedAudioOutput].Name
	}
	if output == "" {
		output = "Default output"
		if c.locale == ShellRussian {
			output = "Выход по умолчанию"
		}
	}
	phase := string(snapshot.SelfTestPhase)
	if phase == "" {
		phase = string(WindowsLocalSelfTestIdle)
	}
	meter := int(snapshot.SelfTestMeter*100 + .5)
	result := "Input: " + input + "\r\nOutput: " + output + "\r\nSelf-test: " + phase + fmt.Sprintf(" · level %d%%", meter)
	if c.locale == ShellRussian {
		result = "Вход: " + input + "\r\nВыход: " + output + "\r\nСамопроверка: " + phase + fmt.Sprintf(" · уровень %d%%", meter)
	}
	if snapshot.LocalDraftAvailable {
		name := snapshot.LocalDraftName
		if name == "" {
			name = "local draft"
		}
		if c.locale == ShellRussian {
			result += "\r\nЧерновик: " + name
		} else {
			result += "\r\nDraft: " + name
		}
	}
	if snapshot.RecordingDraftAvailable {
		if c.locale == ShellRussian {
			result += "\r\nЧерновик записи готов к отправке"
		} else {
			result += "\r\nRecording draft is ready to send"
		}
	}
	if snapshot.LocalFailure != "" {
		result += "\r\n[!] " + snapshot.LocalFailure
	}
	return result
}

func shellPrimaryAction(section ShellSection) shellText {
	return map[ShellSection]shellText{
		ShellCreate: txtCreateAction, ShellJoin: txtJoinAction, ShellTryLocally: txtTryAction,
	}[section]
}

func shellActionEnabled(snapshot ShellSnapshot, action ShellSection) bool {
	switch action {
	case ShellCreate, ShellJoin, ShellTryLocally, ShellSoundboard, ShellHistory, ShellInbox, ShellAirs, ShellAutomation, ShellSettings, ShellHome:
		return true
	default:
		return false
	}
}

func shellRecordingEnabled(snapshot ShellSnapshot) bool {
	if snapshot.Recording == ShellRecordingActive || snapshot.Recording == ShellRecordingProcessing {
		return true
	}
	return snapshot.RecordingAvailable && !shellLocalCaptureBusy(snapshot)
}

func shellLocalCaptureBusy(snapshot ShellSnapshot) bool {
	return snapshot.SelfTestPhase != "" && !windowsLocalSelfTestCanStart(snapshot.SelfTestPhase)
}

func shellDNDEnabled(snapshot ShellSnapshot) bool { return snapshot.Connection != ShellUnpaired }

func airInviteAllowed(air ShellAirItem) bool {
	if air.MembershipStatus != AirJoined {
		return false
	}
	switch air.Policy.Invite {
	case AirInviteOwnerPrimary:
		return air.Role == AirRoleOwner
	case AirInviteAdminPrimary:
		return air.Role == AirRoleOwner || air.Role == AirRoleAdmin
	case AirInviteAllMemberPrimarys:
		return air.Role == AirRoleOwner || air.Role == AirRoleAdmin || air.Role == AirRoleMember
	default:
		return false
	}
}

type ShellRect struct{ X, Y, Width, Height int }

func (r ShellRect) Right() int  { return r.X + r.Width }
func (r ShellRect) Bottom() int { return r.Y + r.Height }

type ShellLayout struct {
	DPI       int
	Client    ShellRect
	Sidebar   ShellRect
	Content   ShellRect
	Header    ShellRect
	Banner    ShellRect
	Body      ShellRect
	Cards     [3]ShellRect
	Footer    ShellRect
	Collapsed bool
}

type AirControlLayout struct {
	TitleLabel, TitleInput, Create ShellRect
	CodeLabel, CodeInput, Consume  ShellRect
	Manage                         [4]ShellRect
	Invite                         [5]ShellRect
	Pending                        [4]ShellRect
	Confirm, Cancel                ShellRect
}

type TargetsInboxControlLayout struct{ Rect [21]ShellRect }

type StreamTrackControlLayout struct{ Rect [16]ShellRect }

func (layout StreamTrackControlLayout) Rects() []ShellRect { return layout.Rect[:] }

func layoutWindowsStreamTrackControls(content ShellRect, startY, dpi int) StreamTrackControlLayout {
	gap, height := dip(4, dpi), dip(40, dpi)
	columnWidth := (content.Width - gap*3) / 4
	var result StreamTrackControlLayout
	for index := range result.Rect {
		row, column := index/4, index%4
		result.Rect[index] = ShellRect{
			X: content.X + column*(columnWidth+gap), Y: startY + row*(height+gap),
			Width: columnWidth, Height: height,
		}
	}
	return result
}

func (layout TargetsInboxControlLayout) Rects() []ShellRect { return layout.Rect[:] }

func layoutWindowsTargetsInboxControls(content ShellRect, bodyBottom, dpi int) TargetsInboxControlLayout {
	gap, height := dip(4, dpi), dip(40, dpi)
	columnWidth := (content.Width - gap*3) / 4
	cell := func(column, row int) ShellRect {
		return ShellRect{X: content.X + column*(columnWidth+gap), Y: bodyBottom + gap + row*(height+gap), Width: columnWidth, Height: height}
	}
	var result TargetsInboxControlLayout
	for index := 0; index < 18; index++ {
		result.Rect[index] = cell(index%4, index/4)
	}
	result.Rect[18] = cell(2, 4)
	result.Rect[18].Width = columnWidth*2 + gap
	result.Rect[19] = cell(0, 5)
	result.Rect[19].Width = columnWidth*2 + gap
	result.Rect[20] = cell(2, 5)
	result.Rect[20].Width = columnWidth*2 + gap
	return result
}

func (l AirControlLayout) Rects() []ShellRect {
	result := []ShellRect{l.TitleLabel, l.TitleInput, l.Create, l.CodeLabel, l.CodeInput, l.Consume}
	result = append(result, l.Manage[:]...)
	result = append(result, l.Invite[:]...)
	result = append(result, l.Pending[:]...)
	return append(result, l.Confirm, l.Cancel)
}

func layoutWindowsAirControls(content ShellRect, bodyBottom, dpi int) AirControlLayout {
	gap, y := dip(8, dpi), bodyBottom+dip(8, dpi)
	labelWidth, inputWidth, buttonWidth := dip(100, dpi), dip(280, dpi), dip(150, dpi)
	result := AirControlLayout{
		TitleLabel: ShellRect{X: content.X, Y: y + dip(7, dpi), Width: labelWidth, Height: dip(34, dpi)},
		TitleInput: ShellRect{X: content.X + labelWidth, Y: y + dip(3, dpi), Width: inputWidth, Height: dip(34, dpi)},
		Create:     ShellRect{X: content.X + labelWidth + inputWidth + gap, Y: y, Width: buttonWidth, Height: dip(42, dpi)},
	}
	y += dip(50, dpi)
	result.CodeLabel = ShellRect{X: content.X, Y: y + dip(7, dpi), Width: labelWidth, Height: dip(34, dpi)}
	result.CodeInput = ShellRect{X: content.X + labelWidth, Y: y + dip(3, dpi), Width: inputWidth, Height: dip(34, dpi)}
	result.Consume = ShellRect{X: content.X + labelWidth + inputWidth + gap, Y: y, Width: buttonWidth, Height: dip(42, dpi)}
	y += dip(50, dpi)
	for index := range result.Manage {
		result.Manage[index] = ShellRect{X: content.X + index*dip(142, dpi), Y: y, Width: dip(134, dpi), Height: dip(42, dpi)}
	}
	y += dip(50, dpi)
	for index := range result.Invite {
		result.Invite[index] = ShellRect{X: content.X + index*dip(114, dpi), Y: y, Width: dip(106, dpi), Height: dip(42, dpi)}
	}
	y += dip(50, dpi)
	for index := range result.Pending {
		result.Pending[index] = ShellRect{X: content.X + index*dip(142, dpi), Y: y, Width: dip(134, dpi), Height: dip(42, dpi)}
	}
	y += dip(50, dpi)
	result.Confirm = ShellRect{X: content.X, Y: y, Width: dip(260, dpi), Height: dip(44, dpi)}
	result.Cancel = ShellRect{X: content.X + dip(272, dpi), Y: y, Width: dip(180, dpi), Height: dip(44, dpi)}
	return result
}

func dip(value, dpi int) int { return (value*dpi + 48) / 96 }

func layoutWindowsShell(clientWidth, clientHeight, dpi int) ShellLayout {
	if dpi <= 0 {
		dpi = 96
	}
	minWidth, minHeight := windowsShellMinimumClient(dpi)
	if clientWidth < minWidth {
		clientWidth = minWidth
	}
	if clientHeight < minHeight {
		clientHeight = minHeight
	}
	metrics := windowsShellMetrics(dpi)
	margin, gap := metrics.Margin, metrics.Gutter
	sidebarWidth := metrics.SidebarWidth
	contentX := sidebarWidth + margin*2
	contentWidth := clientWidth - contentX - margin
	layout := ShellLayout{
		DPI: dpi, Client: ShellRect{Width: clientWidth, Height: clientHeight},
		Sidebar: ShellRect{X: margin, Y: margin, Width: sidebarWidth, Height: clientHeight - margin*2},
		Content: ShellRect{X: contentX, Y: margin, Width: contentWidth, Height: clientHeight - margin*2},
	}
	layout.Header = ShellRect{X: contentX, Y: margin, Width: contentWidth, Height: metrics.HeaderHeight}
	layout.Banner = ShellRect{X: contentX, Y: layout.Header.Bottom() + gap, Width: contentWidth, Height: dip(56, dpi)}
	bodyY := layout.Banner.Bottom() + gap
	layout.Body = ShellRect{X: contentX, Y: bodyY, Width: contentWidth, Height: dip(144, dpi)}
	cardY := layout.Body.Bottom() + gap
	cardWidth := (contentWidth - gap*2) / 3
	for i := range layout.Cards {
		layout.Cards[i] = ShellRect{X: contentX + i*(cardWidth+gap), Y: cardY, Width: cardWidth, Height: metrics.CardHeight}
	}
	footerHeight := clientHeight - (layout.Cards[0].Bottom() + gap) - margin
	if maximum := dip(160, dpi); footerHeight > maximum {
		footerHeight = maximum
	}
	layout.Footer = ShellRect{X: contentX, Y: layout.Cards[0].Bottom() + gap, Width: contentWidth, Height: footerHeight}
	return layout
}

type ShellShortcut struct {
	Key     string
	Control bool
	Shift   bool
	Section ShellSection
	Command string
}

var shellShortcuts = []ShellShortcut{
	{Key: "0", Control: true, Command: "open"},
	{Key: "1", Control: true, Section: ShellCreate, Command: "section"},
	{Key: "2", Control: true, Section: ShellJoin, Command: "section"},
	{Key: "3", Control: true, Section: ShellAirs, Command: "section"},
	{Key: "4", Control: true, Section: ShellInbox, Command: "section"},
	{Key: "5", Control: true, Section: ShellSoundboard, Command: "section"},
	{Key: "6", Control: true, Section: ShellAutomation, Command: "section"},
	{Key: "T", Control: true, Shift: true, Section: ShellTryLocally, Command: "section"},
	{Key: "R", Control: true, Shift: true, Command: "record"},
	{Key: "R", Control: true, Command: "refresh_targets_inbox"},
	{Key: "L", Control: true, Shift: true, Section: ShellHistory, Command: "choose_stream_track"},
	{Key: "D", Control: true, Shift: true, Command: "dnd"},
	{Key: ",", Control: true, Section: ShellSettings, Command: "section"},
}

func catalogMissing(locale ShellLocale) []string {
	var missing []string
	for _, key := range shellTextKeys {
		if strings.TrimSpace(shellCatalog[locale][key]) == "" {
			missing = append(missing, string(key))
		}
	}
	sort.Strings(missing)
	return missing
}

var shellCatalog = map[ShellLocale]map[shellText]string{
	ShellEnglish: {
		txtApp: "Pulsar", txtHome: "Home", txtCreate: "Create", txtJoin: "Join", txtTry: "Try locally",
		txtHistory: "History", txtSoundboard: "Soundboard", txtInbox: "Inbox & targets", txtAirs: "Airs", txtSettings: "Settings", txtOpen: "Open Pulsar", txtPrimary: "Get started",
		txtStatus: "Status", txtPresence: "Presence", txtRouting: "Routing", txtNowPlaying: "Now playing",
		txtLocalControls: "Local controls", txtNoHistory: "No recent activity", txtNoRoute: "No output route",
		txtSilence: "Nothing is playing", txtVolume: "Volume", txtDND: "Do Not Disturb", txtRecording: "Recording",
		txtStartRecording: "Start recording", txtStopRecording: "Stop recording", txtCancelRecording: "Cancel recording",
		txtRecordingUnavailable: "Recording is not configured yet", txtSelfTestUnavailable: "Local self-test is not configured yet",
		txtCreateTitle: "Create a Barycenter", txtCreateBody: "Enter a title. Pulsar stores the identity with Windows protection and requires an explicit recovery-file export before activation.",
		txtCreateAction: "Create securely", txtJoinTitle: "Join a Barycenter",
		txtJoinBody: "Enter the device invitation. Pulsar activates this installation only after the identity is protected on this PC.", txtJoinAction: "Join securely",
		txtTryTitle: "Try Pulsar locally", txtTryBody: "Record five seconds and play them only on this PC before sending anything.",
		txtTryAction: "Run local self-test", txtHistoryTitle: "Recent activity", txtSettingsTitle: "Pulsar settings",
		txtLanguage: "Language", txtIntegrations: "Optional integrations",
		txtSpotifyOptional:  "Spotify is an optional music source; Pulsar audio and local review work without it.",
		txtTelegramOptional: "Telegram is an optional companion control; Create, Join, routing, history, and reports remain available in Pulsar.",
		txtReport:           "Report",
		txtUnpaired:         "Not paired", txtReconnecting: "Reconnecting", txtOnline: "Connected",
		txtDegraded: "Needs attention", txtDNDAllow: "Allow all audio", txtDNDMessages: "Messages only", txtDNDMuted: "Muted",
		txtRecordingIdle: "Not recording", txtRecordingActive: "Recording - press Stop to finish",
		txtRecordingProcessing: "Preparing recording", txtRecordingFailed: "Recording failed",
		txtUnpairedHelp:  "Create an Air, join one, or test audio locally. You can start without connecting an account.",
		txtDegradedHelp:  "Local controls and settings remain available while Pulsar reconnects.",
		txtRecordingHelp: "Recording is active. Stop remains available in this window and the tray.",
		txtShortcut:      "Recording shortcut", txtShortcutRegistered: "active", txtShortcutConflict: "in use; buttons still work",
		txtShortcutUnavailable: "unavailable; buttons still work", txtShortcutSuspended: "paused while Windows is locked or asleep",
		txtShortcutInactive: "inactive",
		txtPair:             "Connect...", txtRepair: "Connect again...", txtHowToSound: "Optional Spotify integration...",
		txtNoPulsar: "Troubleshoot optional Spotify integration", txtPrivacy: "Privacy", txtTerms: "Terms of use",
		txtGuidelines: "Content guidelines", txtUploadRights: "Recording and upload rights",
		txtSupport: "Support and safety", txtQuit: "Quit Pulsar",
		txtCaptureQuality: "Capture quality", txtCaptureMode: "Capture mode",
		txtCaptureAllowDegraded: "Allow degraded capture", txtCaptureStopLocal: "Stop local capture",
		txtCaptureInputCeiling: "Input AGC ceiling", txtCaptureOutputCeiling: "Receiver output ceiling",
		txtCaptureCeilingHelp: "Input gain is capped separately from receiver output; changing receiver volume never raises microphone gain.",
		txtCaptureConsentHelp: "Speaker processing is degraded or unsupported. Capture remains blocked until you explicitly allow this one local attempt.",
	},
	ShellRussian: {
		txtApp: "Пульсар", txtHome: "Главная", txtCreate: "Создать", txtJoin: "Присоединиться", txtTry: "Попробовать локально",
		txtHistory: "История", txtSoundboard: "Звуки", txtInbox: "Входящие", txtAirs: "Эфиры", txtSettings: "Настройки", txtOpen: "Открыть Пульсар", txtPrimary: "Начало работы",
		txtStatus: "Статус", txtPresence: "Присутствие", txtRouting: "Маршрут звука", txtNowPlaying: "Сейчас играет",
		txtLocalControls: "Локальные настройки", txtNoHistory: "Недавних событий нет", txtNoRoute: "Выход звука не выбран",
		txtSilence: "Сейчас ничего не играет", txtVolume: "Громкость", txtDND: "Не беспокоить", txtRecording: "Запись",
		txtStartRecording: "Начать запись", txtStopRecording: "Остановить запись", txtCancelRecording: "Отменить запись",
		txtRecordingUnavailable: "Запись пока не настроена", txtSelfTestUnavailable: "Локальная самопроверка пока не настроена",
		txtCreateTitle: "Создать Барицентр", txtCreateBody: "Введи название. Пульсар защищённо сохраняет идентификатор средствами Windows и требует явно экспортировать файл восстановления до активации.",
		txtCreateAction: "Создать защищённо", txtJoinTitle: "Присоединиться к Барицентру",
		txtJoinBody: "Введи приглашение устройства. Пульсар активирует эту установку только после защищённого сохранения идентификатора на этом ПК.", txtJoinAction: "Присоединиться защищённо",
		txtTryTitle: "Проверить Пульсар локально", txtTryBody: "Запиши пять секунд и воспроизведи их только на этом ПК до любой отправки.",
		txtTryAction: "Запустить самопроверку", txtHistoryTitle: "Недавние события", txtSettingsTitle: "Настройки Пульсара",
		txtLanguage: "Язык", txtIntegrations: "Необязательные интеграции",
		txtSpotifyOptional:  "Spotify — необязательный источник музыки; звук Пульсара и локальная проверка работают без него.",
		txtTelegramOptional: "Telegram — необязательный пульт; создание, присоединение, маршрутизация, история и жалобы доступны в Пульсаре.",
		txtReport:           "Пожаловаться",
		txtUnpaired:         "Не подключён", txtReconnecting: "Переподключение", txtOnline: "Подключён",
		txtDegraded: "Нужно внимание", txtDNDAllow: "Разрешить весь звук", txtDNDMessages: "Только сообщения", txtDNDMuted: "Звук выключен",
		txtRecordingIdle: "Запись не идёт", txtRecordingActive: "Идёт запись - нажми «Остановить», чтобы закончить",
		txtRecordingProcessing: "Подготавливаю запись", txtRecordingFailed: "Ошибка записи",
		txtUnpairedHelp:  "Создайте эфир, присоединитесь или проверьте звук локально. Начать можно без подключения аккаунта.",
		txtDegradedHelp:  "Локальные настройки остаются доступны, пока Пульсар переподключается.",
		txtRecordingHelp: "Запись активна. Остановка остаётся доступна в этом окне и в области уведомлений.",
		txtShortcut:      "Комбинация записи", txtShortcutRegistered: "активна", txtShortcutConflict: "занята; кнопки продолжают работать",
		txtShortcutUnavailable: "недоступна; кнопки продолжают работать", txtShortcutSuspended: "приостановлена, пока Windows заблокирована или спит",
		txtShortcutInactive: "не активна",
		txtPair:             "Подключить...", txtRepair: "Подключить заново...", txtHowToSound: "Необязательная интеграция Spotify...",
		txtNoPulsar: "Диагностика необязательной интеграции Spotify", txtPrivacy: "Конфиденциальность", txtTerms: "Условия использования",
		txtGuidelines: "Правила содержимого", txtUploadRights: "Права на запись и загрузку",
		txtSupport: "Поддержка и безопасность", txtQuit: "Выйти из Пульсара",
		txtCaptureQuality: "Качество записи", txtCaptureMode: "Режим записи",
		txtCaptureAllowDegraded: "Разрешить ограниченную запись", txtCaptureStopLocal: "Остановить локальную запись",
		txtCaptureInputCeiling: "Предел входного AGC", txtCaptureOutputCeiling: "Предел выхода получателя",
		txtCaptureCeilingHelp: "Усиление входа ограничено отдельно от выхода получателя; громкость воспроизведения не повышает усиление микрофона.",
		txtCaptureConsentHelp: "Обработка динамиков ограничена или недоступна. Запись заблокирована, пока вы явно не разрешите одну локальную попытку.",
	},
}
