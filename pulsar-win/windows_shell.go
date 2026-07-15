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

type ShellSnapshot struct {
	Connection           ShellConnection
	ConnectionDetail     string
	Identity             string
	PresenceOnline       int
	PresenceTotal        int
	PresenceAvailable    bool
	RouteName            string
	NowPlaying           string
	PlaybackState        string
	HistoryCount         int
	DND                  ShellDND
	Recording            ShellRecording
	RecordingAvailable   bool
	RecordingShortcut    WindowsRecordingShortcutStatus
	RecordingShortcutKey WindowsRecordingShortcut
	SelfTestAvailable    bool
	Volume               int
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
	return s
}

type ShellActions struct {
	Create          func()
	Join            func()
	TryLocally      func()
	ToggleRecording func()
	CancelRecording func()
	SetDND          func(ShellDND)
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
	txtSettingsTitle, txtLanguage, txtUnpaired, txtReconnecting, txtOnline, txtDegraded,
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
		return fmt.Sprintf("В сети %d из %d", snapshot.PresenceOnline, snapshot.PresenceTotal)
	}
	return fmt.Sprintf("%d of %d online", snapshot.PresenceOnline, snapshot.PresenceTotal)
}

func (c ShellCopy) Body(section ShellSection, snapshot ShellSnapshot) string {
	switch section {
	case ShellCreate:
		return c.Text(txtCreateBody)
	case ShellJoin:
		return c.Text(txtJoinBody)
	case ShellTryLocally:
		if !snapshot.SelfTestAvailable {
			return c.Text(txtTryBody) + "\r\n\r\n" + c.Text(txtSelfTestUnavailable)
		}
		return c.Text(txtTryBody)
	case ShellHistory:
		if snapshot.HistoryCount == 0 {
			return c.Text(txtNoHistory)
		}
		return fmt.Sprintf("%s: %d", c.Text(txtHistoryTitle), snapshot.HistoryCount)
	case ShellSettings:
		return c.Text(txtLanguage) + "\r\n\r\n" + c.Text(txtDND) + ": " + c.DND(snapshot.DND) +
			"\r\n" + c.Text(txtVolume) + fmt.Sprintf(": %d%%", snapshot.Volume)
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
	return snapshot.RecordingAvailable || snapshot.Recording == ShellRecordingActive
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
	layout.Body = ShellRect{X: contentX, Y: bodyY, Width: contentWidth, Height: dip(104, dpi)}
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
		txtCreateTitle: "Create an air", txtCreateBody: "Open the Barycenter bot and send /create to start a shared audio space.",
		txtCreateAction: "Open Barycenter bot", txtJoinTitle: "Join an air",
		txtJoinBody: "Open an invitation or ask the Barycenter bot for a pairing code.", txtJoinAction: "Open Barycenter bot",
		txtTryTitle: "Try Pulsar locally", txtTryBody: "Record five seconds and play them only on this PC before sending anything.",
		txtTryAction: "Run local self-test", txtHistoryTitle: "Recent activity", txtSettingsTitle: "Pulsar settings",
		txtLanguage: "Language", txtUnpaired: "Not paired", txtReconnecting: "Reconnecting", txtOnline: "Connected",
		txtDegraded: "Needs attention", txtDNDAllow: "Allow all audio", txtDNDMessages: "Messages only", txtDNDMuted: "Muted",
		txtRecordingIdle: "Not recording", txtRecordingActive: "Recording - press Stop to finish",
		txtRecordingProcessing: "Preparing recording", txtRecordingFailed: "Recording failed",
		txtUnpairedHelp:  "Create or join an air, try local audio, or open settings. Pairing is not required for those paths.",
		txtDegradedHelp:  "Local controls and settings remain available while Pulsar reconnects.",
		txtRecordingHelp: "Recording is active. Stop remains available in this window and the tray.",
		txtShortcut:      "Recording shortcut", txtShortcutRegistered: "active", txtShortcutConflict: "in use; buttons still work",
		txtShortcutUnavailable: "unavailable; buttons still work", txtShortcutSuspended: "paused while Windows is locked or asleep",
		txtShortcutInactive: "inactive",
		txtPair:             "Connect...", txtRepair: "Connect again...", txtHowToSound: "How to enable sound...",
		txtNoPulsar: "Cannot see Pulsar in Spotify?", txtPrivacy: "Privacy", txtTerms: "Terms of use",
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
		txtCreateTitle: "Создать эфир", txtCreateBody: "Открой бота Барицентра и отправь /create, чтобы создать общее аудиопространство.",
		txtCreateAction: "Открыть бота Барицентра", txtJoinTitle: "Присоединиться к эфиру",
		txtJoinBody: "Открой приглашение или запроси код подключения в боте Барицентра.", txtJoinAction: "Открыть бота Барицентра",
		txtTryTitle: "Проверить Пульсар локально", txtTryBody: "Запиши пять секунд и воспроизведи их только на этом ПК до любой отправки.",
		txtTryAction: "Запустить самопроверку", txtHistoryTitle: "Недавние события", txtSettingsTitle: "Настройки Пульсара",
		txtLanguage: "Язык", txtUnpaired: "Не подключён", txtReconnecting: "Переподключение", txtOnline: "Подключён",
		txtDegraded: "Нужно внимание", txtDNDAllow: "Разрешить весь звук", txtDNDMessages: "Только сообщения", txtDNDMuted: "Звук выключен",
		txtRecordingIdle: "Запись не идёт", txtRecordingActive: "Идёт запись - нажми «Остановить», чтобы закончить",
		txtRecordingProcessing: "Подготавливаю запись", txtRecordingFailed: "Ошибка записи",
		txtUnpairedHelp:  "Создай эфир, присоединись, проверь локальный звук или открой настройки - для этих путей подключение не требуется.",
		txtDegradedHelp:  "Локальные настройки остаются доступны, пока Пульсар переподключается.",
		txtRecordingHelp: "Запись активна. Остановка остаётся доступна в этом окне и в области уведомлений.",
		txtShortcut:      "Комбинация записи", txtShortcutRegistered: "активна", txtShortcutConflict: "занята; кнопки продолжают работать",
		txtShortcutUnavailable: "недоступна; кнопки продолжают работать", txtShortcutSuspended: "приостановлена, пока Windows заблокирована или спит",
		txtShortcutInactive: "не активна",
		txtPair:             "Подключить...", txtRepair: "Подключить заново...", txtHowToSound: "Как включить звук...",
		txtNoPulsar: "Не вижу Pulsar в Spotify?", txtPrivacy: "Конфиденциальность", txtTerms: "Условия использования",
		txtGuidelines: "Правила содержимого", txtUploadRights: "Права на запись и загрузку",
		txtSupport: "Поддержка и безопасность", txtQuit: "Выйти из Пульсара",
	},
}
