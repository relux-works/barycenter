import Foundation
import Observation

public enum PulsarShellLocale: String, CaseIterable, Identifiable, Sendable {
    case en
    case ru

    public var id: Self { self }

    public static func preferred(from locale: Locale = .current) -> Self {
        locale.language.languageCode == .russian ? .ru : .en
    }
}

public enum PulsarShellSection: String, CaseIterable, Identifiable, Sendable {
    case home
    case create
    case join
    case tryLocally
    case history
    case settings

    public var id: Self { self }
}

public enum PulsarConnectionState: Equatable, Sendable {
    case unpaired
    case reconnecting
    case online
    case degraded(String)

    public var isPaired: Bool {
        if case .unpaired = self { return false }
        return true
    }
}

public enum PulsarRecordingState: Equatable, Sendable {
    case unavailable
    case idle
    case recording
    case processing
    case failed(String)
}

public enum PulsarDNDMode: String, CaseIterable, Identifiable, Sendable {
    case allowAll = "allow_all"
    case messagesOnly = "messages_only"
    case mutedUntil = "muted_until"

    public var id: Self { self }
}

public struct PulsarHistoryItem: Equatable, Identifiable, Sendable {
    public let id: String
    public let title: String
    public let detail: String
    public let occurredAt: Date

    public init(id: String, title: String, detail: String, occurredAt: Date) {
        self.id = id
        self.title = title
        self.detail = detail
        self.occurredAt = occurredAt
    }
}

public struct PulsarShellSnapshot: Equatable, Sendable {
    public var connection: PulsarConnectionState
    public var connectionIdentity: String?
    public var presenceSummary: String?
    public var routeName: String?
    public var nowPlaying: String?
    public var playbackState: String
    public var history: [PulsarHistoryItem]
    public var dndMode: PulsarDNDMode
    public var recording: PulsarRecordingState
    public var recordingAvailable: Bool
    public var selfTestAvailable: Bool
    public var volume: Int

    public init(
        connection: PulsarConnectionState = .unpaired,
        connectionIdentity: String? = nil,
        presenceSummary: String? = nil,
        routeName: String? = nil,
        nowPlaying: String? = nil,
        playbackState: String = "stopped",
        history: [PulsarHistoryItem] = [],
        dndMode: PulsarDNDMode = .allowAll,
        recording: PulsarRecordingState = .unavailable,
        recordingAvailable: Bool = false,
        selfTestAvailable: Bool = false,
        volume: Int = 80
    ) {
        self.connection = connection
        self.connectionIdentity = connectionIdentity
        self.presenceSummary = presenceSummary
        self.routeName = routeName
        self.nowPlaying = nowPlaying
        self.playbackState = playbackState
        self.history = history
        self.dndMode = dndMode
        self.recording = recording
        self.recordingAvailable = recordingAvailable
        self.selfTestAvailable = selfTestAvailable
        self.volume = min(max(volume, 0), 100)
    }
}

@MainActor
@Observable
public final class PulsarShellModel {
    public var locale: PulsarShellLocale
    public var selectedSection: PulsarShellSection
    public private(set) var snapshot: PulsarShellSnapshot

    public init(
        locale: PulsarShellLocale = .preferred(),
        selectedSection: PulsarShellSection = .home,
        snapshot: PulsarShellSnapshot = .init()
    ) {
        self.locale = locale
        self.selectedSection = selectedSection
        self.snapshot = snapshot
    }

    public func replaceSnapshot(_ snapshot: PulsarShellSnapshot) {
        self.snapshot = snapshot
    }

    public func updateConnection(_ connection: PulsarConnectionState, identity: String? = nil) {
        snapshot.connection = connection
        snapshot.connectionIdentity = identity
    }

    public func updateRuntime(
        routeName: String?,
        nowPlaying: String?,
        playbackState: String,
        dndMode: PulsarDNDMode,
        volume: Int
    ) {
        snapshot.routeName = routeName
        snapshot.nowPlaying = nowPlaying
        snapshot.playbackState = playbackState
        snapshot.dndMode = dndMode
        snapshot.volume = min(max(volume, 0), 100)
    }

    public func setHistory(_ items: [PulsarHistoryItem]) {
        snapshot.history = items
    }

    public func setRecording(_ state: PulsarRecordingState, available: Bool) {
        snapshot.recording = state
        snapshot.recordingAvailable = available
    }

    public func setSelfTestAvailable(_ available: Bool) {
        snapshot.selfTestAvailable = available
    }

    public func setDNDMode(_ mode: PulsarDNDMode) {
        snapshot.dndMode = mode
    }

    public func setVolume(_ volume: Int) {
        snapshot.volume = min(max(volume, 0), 100)
    }
}

@MainActor
public final class PulsarShellActions {
    private let onCreateOrbit: () -> Void
    private let onJoinOrbit: () -> Void
    private let onTryLocally: () -> Void
    private let onSetDND: (PulsarDNDMode) -> Void
    private let onSetVolume: (Int) -> Void
    private let onToggleRecording: () -> Void

    public init(
        createOrbit: @escaping @MainActor () -> Void = {},
        joinOrbit: @escaping @MainActor () -> Void = {},
        tryLocally: @escaping @MainActor () -> Void = {},
        setDND: @escaping @MainActor (PulsarDNDMode) -> Void = { _ in },
        setVolume: @escaping @MainActor (Int) -> Void = { _ in },
        toggleRecording: @escaping @MainActor () -> Void = {}
    ) {
        self.onCreateOrbit = createOrbit
        self.onJoinOrbit = joinOrbit
        self.onTryLocally = tryLocally
        self.onSetDND = setDND
        self.onSetVolume = setVolume
        self.onToggleRecording = toggleRecording
    }

    public func createOrbit() { onCreateOrbit() }
    public func joinOrbit() { onJoinOrbit() }
    public func tryLocally() { onTryLocally() }
    public func setDND(_ mode: PulsarDNDMode) { onSetDND(mode) }
    public func setVolume(_ volume: Int) { onSetVolume(volume) }
    public func toggleRecording() { onToggleRecording() }
}

public enum PulsarShellText: String, CaseIterable, Sendable {
    case appName, home, create, join, tryLocally, history, settings
    case openMainWindow, primaryActions, status, presence, routing, nowPlaying
    case localControls, noHistory, noRoute, silence, volume, dnd, recording
    case startRecording, stopRecording, recordingUnavailable, selfTestUnavailable
    case createTitle, createBody, createAction, joinTitle, joinBody, joinAction
    case tryTitle, tryBody, tryAction, historyTitle, settingsTitle, language
    case connectionUnpaired, connectionReconnecting, connectionOnline, connectionDegraded
    case dndAllowAll, dndMessagesOnly, dndMutedUntil
    case recordingIdle, recordingActive, recordingProcessing, recordingFailed
    case unpairedHelp, degradedHelp, recordingHelp, quit
}

public struct PulsarShellCopy: Sendable {
    public let locale: PulsarShellLocale

    public init(locale: PulsarShellLocale) { self.locale = locale }

    public func text(_ key: PulsarShellText) -> String {
        switch locale {
        case .en: Self.en[key]!
        case .ru: Self.ru[key]!
        }
    }

    public func title(for section: PulsarShellSection) -> String {
        switch section {
        case .home: text(.home)
        case .create: text(.create)
        case .join: text(.join)
        case .tryLocally: text(.tryLocally)
        case .history: text(.history)
        case .settings: text(.settings)
        }
    }

    public func connectionLabel(_ state: PulsarConnectionState) -> String {
        switch state {
        case .unpaired: text(.connectionUnpaired)
        case .reconnecting: text(.connectionReconnecting)
        case .online: text(.connectionOnline)
        case .degraded(let reason): "\(text(.connectionDegraded)): \(reason)"
        }
    }

    public func connectionSymbol(_ state: PulsarConnectionState) -> String {
        switch state {
        case .unpaired: "person.crop.circle.badge.questionmark"
        case .reconnecting: "arrow.triangle.2.circlepath"
        case .online: "checkmark.circle.fill"
        case .degraded: "exclamationmark.triangle.fill"
        }
    }

    public func dndLabel(_ mode: PulsarDNDMode) -> String {
        switch mode {
        case .allowAll: text(.dndAllowAll)
        case .messagesOnly: text(.dndMessagesOnly)
        case .mutedUntil: text(.dndMutedUntil)
        }
    }

    public func recordingLabel(_ state: PulsarRecordingState) -> String {
        switch state {
        case .unavailable: text(.recordingUnavailable)
        case .idle: text(.recordingIdle)
        case .recording: text(.recordingActive)
        case .processing: text(.recordingProcessing)
        case .failed(let reason): "\(text(.recordingFailed)): \(reason)"
        }
    }

    public func recordingSymbol(_ state: PulsarRecordingState) -> String {
        switch state {
        case .unavailable: "mic.slash"
        case .idle: "mic"
        case .recording: "record.circle.fill"
        case .processing: "waveform"
        case .failed: "exclamationmark.triangle.fill"
        }
    }

    private static let en: [PulsarShellText: String] = [
        .appName: "Pulsar", .home: "Home", .create: "Create", .join: "Join",
        .tryLocally: "Try locally", .history: "History", .settings: "Settings",
        .openMainWindow: "Open Pulsar", .primaryActions: "Primary actions",
        .status: "Status", .presence: "Presence", .routing: "Routing",
        .nowPlaying: "Now playing", .localControls: "Local controls",
        .noHistory: "No recent activity", .noRoute: "No output route",
        .silence: "Nothing is playing", .volume: "Volume", .dnd: "Do Not Disturb",
        .recording: "Recording", .startRecording: "Start recording",
        .stopRecording: "Stop recording", .recordingUnavailable: "Recording is not configured yet",
        .selfTestUnavailable: "Local self-test is not configured yet",
        .createTitle: "Create an air", .createBody: "Open the Barycenter bot and send /create to start a shared audio space.",
        .createAction: "Open Barycenter bot", .joinTitle: "Join an air",
        .joinBody: "Open an invitation or ask the Barycenter bot for a pairing code.",
        .joinAction: "Open Barycenter bot", .tryTitle: "Try Pulsar locally",
        .tryBody: "Record five seconds and play them only on this Mac before sending anything.",
        .tryAction: "Run local self-test", .historyTitle: "Recent activity",
        .settingsTitle: "Pulsar settings", .language: "Language",
        .connectionUnpaired: "Not paired", .connectionReconnecting: "Reconnecting",
        .connectionOnline: "Connected", .connectionDegraded: "Needs attention",
        .dndAllowAll: "Allow all audio", .dndMessagesOnly: "Messages only",
        .dndMutedUntil: "Muted", .recordingIdle: "Not recording",
        .recordingActive: "Recording — press Stop to finish",
        .recordingProcessing: "Preparing recording", .recordingFailed: "Recording failed",
        .unpairedHelp: "Create or join an air, try local audio, or open settings. Pairing is not required for those paths.",
        .degradedHelp: "Local controls and settings remain available while Pulsar reconnects.",
        .recordingHelp: "Recording is active. The Stop control remains available in this window and the menu bar.",
        .quit: "Quit Pulsar",
    ]

    private static let ru: [PulsarShellText: String] = [
        .appName: "Пульсар", .home: "Главная", .create: "Создать", .join: "Присоединиться",
        .tryLocally: "Попробовать локально", .history: "История", .settings: "Настройки",
        .openMainWindow: "Открыть Пульсар", .primaryActions: "Основные действия",
        .status: "Статус", .presence: "Присутствие", .routing: "Маршрут звука",
        .nowPlaying: "Сейчас играет", .localControls: "Локальные настройки",
        .noHistory: "Недавних событий нет", .noRoute: "Выход звука не выбран",
        .silence: "Сейчас ничего не играет", .volume: "Громкость", .dnd: "Не беспокоить",
        .recording: "Запись", .startRecording: "Начать запись",
        .stopRecording: "Остановить запись", .recordingUnavailable: "Запись пока не настроена",
        .selfTestUnavailable: "Локальная самопроверка пока не настроена",
        .createTitle: "Создать эфир", .createBody: "Открой бота Барицентра и отправь /create, чтобы создать общее аудиопространство.",
        .createAction: "Открыть бота Барицентра", .joinTitle: "Присоединиться к эфиру",
        .joinBody: "Открой приглашение или запроси код подключения в боте Барицентра.",
        .joinAction: "Открыть бота Барицентра", .tryTitle: "Проверить Пульсар локально",
        .tryBody: "Запиши пять секунд и воспроизведи их только на этом маке до любой отправки.",
        .tryAction: "Запустить самопроверку", .historyTitle: "Недавние события",
        .settingsTitle: "Настройки Пульсара", .language: "Язык",
        .connectionUnpaired: "Не подключён", .connectionReconnecting: "Переподключение",
        .connectionOnline: "Подключён", .connectionDegraded: "Нужно внимание",
        .dndAllowAll: "Разрешить весь звук", .dndMessagesOnly: "Только сообщения",
        .dndMutedUntil: "Звук выключен", .recordingIdle: "Запись не идёт",
        .recordingActive: "Идёт запись — нажми «Остановить», чтобы закончить",
        .recordingProcessing: "Подготавливаю запись", .recordingFailed: "Ошибка записи",
        .unpairedHelp: "Создай эфир, присоединись, проверь локальный звук или открой настройки — для этих путей подключение не требуется.",
        .degradedHelp: "Локальные настройки остаются доступны, пока Пульсар переподключается.",
        .recordingHelp: "Запись активна. Кнопка остановки остаётся доступна в этом окне и в строке меню.",
        .quit: "Выйти из Пульсара",
    ]
}
