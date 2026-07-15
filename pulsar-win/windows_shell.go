package main

import (
	"fmt"
	"sort"
	"strings"
	"sync"
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
	ShellSettings   ShellSection = "settings"
)

var shellSections = []ShellSection{
	ShellHome, ShellCreate, ShellJoin, ShellTryLocally, ShellHistory, ShellSettings,
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
}

type ShellPhaseOneHistoryItem struct {
	Title             string
	SenderName        string
	Direction         string
	Status            string
	RequestedDelivery string
	EffectiveDelivery string
	DowngradeReason   string
	PlayedCount       int
	OtherCount        int
	CanDelete         bool
	CanReplay         bool
	CanReport         bool
	CanBlock          bool
}

type ShellSnapshot struct {
	Connection               ShellConnection
	ConnectionDetail         string
	Identity                 string
	PresenceOnline           int
	PresenceTotal            int
	PresenceReady            int
	PresenceAvailable        bool
	RouteName                string
	NowPlaying               string
	PlaybackState            string
	HistoryCount             int
	DND                      ShellDND
	Recording                ShellRecording
	RecordingAvailable       bool
	RecordingShortcut        WindowsRecordingShortcutStatus
	RecordingShortcutKey     WindowsRecordingShortcut
	SelfTestAvailable        bool
	SelfTestPhase            WindowsLocalSelfTestPhase
	SelfTestMeter            float32
	LocalDraftAvailable      bool
	LocalDraftName           string
	RecordingDraftAvailable  bool
	LocalFailure             string
	CaptureInputs            []WindowsCaptureInput
	SelectedCaptureInput     int
	AudioOutputs             []WindowsAudioOutput
	SelectedAudioOutput      int
	Volume                   int
	IdentityOperation        ShellIdentityOperation
	IdentityFailure          string
	RecoveryExportRequired   bool
	PhaseOneDrafts           []ShellPhaseOneDraft
	SelectedPhaseOneDraft    int
	SelectedPhaseOneRoute    PhaseOneRoute
	SelectedPhaseOneDelivery PhaseOneDelivery
	PhaseOneHistory          []ShellPhaseOneHistoryItem
	SelectedHistoryItem      int
	SelectedReportReason     PhaseOneModerationReason
	PhaseOneActionOutcome    string
	PhaseOneFailure          string
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
	return s
}

type ShellActions struct {
	Create                     func(string)
	Join                       func(string)
	SaveRecovery               func(string)
	TryLocally                 func()
	PlayBuiltinCue             func()
	ChooseLocalFile            func()
	ChooseOutgoingFile         func()
	AcceptDroppedFile          func(WindowsBrokeredAudioFile)
	DeleteLocalDraft           func()
	SelectNextInput            func()
	SelectNextOutput           func()
	ToggleRecording            func()
	CancelRecording            func()
	SetDND                     func(ShellDND)
	SendSelectedDraft          func()
	DeleteSelectedDraft        func()
	SelectNextPhaseOneDraft    func()
	SelectNextPhaseOneRoute    func()
	SelectNextPhaseOneDelivery func()
	SelectNextHistoryItem      func()
	SelectNextReportReason     func()
	DeleteSelectedHistoryItem  func()
	ReportSelectedHistoryItem  func(string)
	ReplaySelectedHistoryItem  func()
	BlockSelectedHistoryActor  func()
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
)

var shellTextKeys = []shellText{
	txtApp, txtHome, txtCreate, txtJoin, txtTry, txtHistory, txtSettings, txtOpen,
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
	return c.Text(map[ShellSection]shellText{
		ShellHome: txtHome, ShellCreate: txtCreate, ShellJoin: txtJoin,
		ShellTryLocally: txtTry, ShellHistory: txtHistory, ShellSettings: txtSettings,
	}[section])
}

func (c ShellCopy) Connection(snapshot ShellSnapshot) string {
	label := map[ShellConnection]string{
		ShellUnpaired:     "[?] " + c.Text(txtUnpaired),
		ShellReconnecting: "[~] " + c.Text(txtReconnecting),
		ShellOnline:       "[OK] " + c.Text(txtOnline),
		ShellDegraded:     "[!] " + c.Text(txtDegraded),
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
		if !snapshot.SelfTestAvailable {
			return c.Text(txtTryBody) + "\r\n\r\n" + c.Text(txtSelfTestUnavailable)
		}
		return c.Text(txtTryBody) + "\r\n\r\n" + c.LocalSelfTest(snapshot)
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
	case ShellSettings:
		return c.Text(txtLanguage) + "\r\n\r\n" + c.Text(txtDND) + ": " + c.DND(snapshot.DND) +
			"\r\n" + c.Text(txtVolume) + fmt.Sprintf(": %d%%", snapshot.Volume) +
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
	return line
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
	case ShellCreate, ShellJoin, ShellTryLocally, ShellHistory, ShellSettings, ShellHome:
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

func dip(value, dpi int) int { return (value*dpi + 48) / 96 }

func layoutWindowsShell(clientWidth, clientHeight, dpi int) ShellLayout {
	if dpi <= 0 {
		dpi = 96
	}
	minWidth, minHeight := dip(620, dpi), dip(460, dpi)
	if clientWidth < minWidth {
		clientWidth = minWidth
	}
	if clientHeight < minHeight {
		clientHeight = minHeight
	}
	margin, gap := dip(16, dpi), dip(12, dpi)
	sidebarWidth := dip(184, dpi)
	contentX := sidebarWidth + margin*2
	contentWidth := clientWidth - contentX - margin
	layout := ShellLayout{
		DPI: dpi, Client: ShellRect{Width: clientWidth, Height: clientHeight},
		Sidebar: ShellRect{X: margin, Y: margin, Width: sidebarWidth, Height: clientHeight - margin*2},
		Content: ShellRect{X: contentX, Y: margin, Width: contentWidth, Height: clientHeight - margin*2},
	}
	layout.Header = ShellRect{X: contentX, Y: margin, Width: contentWidth, Height: dip(42, dpi)}
	layout.Banner = ShellRect{X: contentX, Y: layout.Header.Bottom() + gap, Width: contentWidth, Height: dip(64, dpi)}
	bodyY := layout.Banner.Bottom() + gap
	layout.Body = ShellRect{X: contentX, Y: bodyY, Width: contentWidth, Height: dip(140, dpi)}
	cardY := layout.Body.Bottom() + gap
	cardWidth := (contentWidth - gap*2) / 3
	for i := range layout.Cards {
		layout.Cards[i] = ShellRect{X: contentX + i*(cardWidth+gap), Y: cardY, Width: cardWidth, Height: dip(92, dpi)}
	}
	layout.Footer = ShellRect{X: contentX, Y: layout.Cards[0].Bottom() + gap, Width: contentWidth, Height: clientHeight - (layout.Cards[0].Bottom() + gap) - margin}
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
	{Key: "T", Control: true, Shift: true, Section: ShellTryLocally, Command: "section"},
	{Key: "R", Control: true, Shift: true, Command: "record"},
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
		txtHistory: "History", txtSettings: "Settings", txtOpen: "Open Pulsar", txtPrimary: "Primary actions",
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
		txtUnpairedHelp:  "Create or join an air, try local audio, or open settings. Pairing is not required for those paths.",
		txtDegradedHelp:  "Local controls and settings remain available while Pulsar reconnects.",
		txtRecordingHelp: "Recording is active. Stop remains available in this window and the tray.",
		txtShortcut:      "Recording shortcut", txtShortcutRegistered: "active", txtShortcutConflict: "in use; buttons still work",
		txtShortcutUnavailable: "unavailable; buttons still work", txtShortcutSuspended: "paused while Windows is locked or asleep",
		txtShortcutInactive: "inactive",
		txtPair:             "Connect...", txtRepair: "Connect again...", txtHowToSound: "Optional Spotify integration...",
		txtNoPulsar: "Troubleshoot optional Spotify integration", txtPrivacy: "Privacy", txtTerms: "Terms of use",
		txtGuidelines: "Content guidelines", txtUploadRights: "Recording and upload rights",
		txtSupport: "Support and safety", txtQuit: "Quit Pulsar",
	},
	ShellRussian: {
		txtApp: "Пульсар", txtHome: "Главная", txtCreate: "Создать", txtJoin: "Присоединиться", txtTry: "Попробовать локально",
		txtHistory: "История", txtSettings: "Настройки", txtOpen: "Открыть Пульсар", txtPrimary: "Основные действия",
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
		txtUnpairedHelp:  "Создай эфир, присоединись, проверь локальный звук или открой настройки - для этих путей подключение не требуется.",
		txtDegradedHelp:  "Локальные настройки остаются доступны, пока Пульсар переподключается.",
		txtRecordingHelp: "Запись активна. Остановка остаётся доступна в этом окне и в области уведомлений.",
		txtShortcut:      "Комбинация записи", txtShortcutRegistered: "активна", txtShortcutConflict: "занята; кнопки продолжают работать",
		txtShortcutUnavailable: "недоступна; кнопки продолжают работать", txtShortcutSuspended: "приостановлена, пока Windows заблокирована или спит",
		txtShortcutInactive: "не активна",
		txtPair:             "Подключить...", txtRepair: "Подключить заново...", txtHowToSound: "Необязательная интеграция Spotify...",
		txtNoPulsar: "Диагностика необязательной интеграции Spotify", txtPrivacy: "Конфиденциальность", txtTerms: "Условия использования",
		txtGuidelines: "Правила содержимого", txtUploadRights: "Права на запись и загрузку",
		txtSupport: "Поддержка и безопасность", txtQuit: "Выйти из Пульсара",
	},
}
