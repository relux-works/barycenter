import Foundation

public struct MacCaptureDuplexLayout: Equatable, Sendable {
    public let inputSampleRate: Double
    public let inputChannels: UInt32
    public let outputSampleRate: Double
    public let outputChannels: UInt32

    public init(
        inputSampleRate: Double,
        inputChannels: UInt32,
        outputSampleRate: Double,
        outputChannels: UInt32
    ) {
        self.inputSampleRate = inputSampleRate
        self.inputChannels = inputChannels
        self.outputSampleRate = outputSampleRate
        self.outputChannels = outputChannels
    }

    public var isUsable: Bool {
        inputSampleRate > 0 && inputChannels > 0
            && outputSampleRate > 0 && outputChannels > 0
    }
}

public enum MacCaptureStartupStage: String, Equatable, Sendable {
    case inputOnly = "input_only"
    case voiceProcessing
    case consentedFallback = "consented_fallback"
    case unprocessed
    case inputSelection = "input_selection"
}

public enum MacCaptureStartupCause: Equatable, Sendable {
    case coreAudio(status: Int32)
    case invalidInputFormat(sampleRate: Double, channels: UInt32)
    case layoutChanged(
        expected: MacCaptureDuplexLayout,
        observed: MacCaptureDuplexLayout
    )
    case invalidLayout(MacCaptureDuplexLayout)
    case engine(domain: String, code: Int)
}

public struct MacCaptureStartupDiagnostic: Error, Equatable, Sendable {
    public let stage: MacCaptureStartupStage
    public let attempt: Int
    public let elapsedMilliseconds: Int
    public let cause: MacCaptureStartupCause

    public init(
        stage: MacCaptureStartupStage,
        attempt: Int,
        elapsedMilliseconds: Int,
        cause: MacCaptureStartupCause
    ) {
        self.stage = stage
        self.attempt = attempt
        self.elapsedMilliseconds = elapsedMilliseconds
        self.cause = cause
    }

    /// Only the observed CoreAudio 35 race and an invalid or changing duplex
    /// layout are safe to retry during startup. Other failures retain
    /// fail-closed behavior and cannot silently enter the degraded path.
    public var isBoundedRecoveryCandidate: Bool {
        switch cause {
        case .coreAudio(status: 35), .layoutChanged, .invalidLayout:
            true
        case .coreAudio, .invalidInputFormat, .engine:
            false
        }
    }

    public var isEligibleForConsentedFallback: Bool {
        stage == .voiceProcessing && isBoundedRecoveryCandidate
    }

    public func redactedLogFields(
        decision: MacCaptureStartupRecoveryDecision
    ) -> [String: Any] {
        var fields: [String: Any] = [
            "stage": stage.rawValue,
            "attempt": attempt,
            "elapsed_ms": elapsedMilliseconds,
            "decision": decision.logValue,
        ]
        switch cause {
        case .coreAudio(let status):
            fields["cause"] = "core_audio"
            fields["os_status"] = status
        case .invalidInputFormat(let sampleRate, let channels):
            fields["cause"] = "invalid_input_format"
            fields["observed_input_rate"] = sampleRate
            fields["observed_input_channels"] = channels
        case .layoutChanged(let expected, let observed):
            fields["cause"] = "layout_changed"
            Self.add(layout: expected, prefix: "expected", to: &fields)
            Self.add(layout: observed, prefix: "observed", to: &fields)
        case .invalidLayout(let layout):
            fields["cause"] = "invalid_layout"
            Self.add(layout: layout, prefix: "observed", to: &fields)
        case .engine(let domain, let code):
            fields["cause"] = "engine"
            fields["error_domain"] = domain
            fields["error_code"] = code
        }
        return fields
    }

    private static func add(
        layout: MacCaptureDuplexLayout,
        prefix: String,
        to fields: inout [String: Any]
    ) {
        fields["\(prefix)_input_rate"] = layout.inputSampleRate
        fields["\(prefix)_input_channels"] = layout.inputChannels
        fields["\(prefix)_output_rate"] = layout.outputSampleRate
        fields["\(prefix)_output_channels"] = layout.outputChannels
    }
}

public enum MacCaptureStartupRecoveryDecision: Equatable, Sendable {
    case retry(afterMilliseconds: Int)
    case fail

    fileprivate var logValue: String {
        switch self {
        case .retry: "retry"
        case .fail: "fail"
        }
    }
}

public enum MacCaptureStartupTerminalAction: Equatable, Sendable {
    case failClosed
    case startConsentedFallback
}

extension MacCaptureStartupDiagnostic {
    public func terminalAction(
        voiceProcessingEnabled: Bool,
        degradedConsent: Bool
    ) -> MacCaptureStartupTerminalAction {
        guard voiceProcessingEnabled,
            degradedConsent,
            isEligibleForConsentedFallback
        else {
            return .failClosed
        }
        return .startConsentedFallback
    }
}

public struct MacCaptureStartupRetryPolicy: Equatable, Sendable {
    public let maximumAttempts: Int
    public let maximumWindowMilliseconds: Int
    public let retryDelaysMilliseconds: [Int]

    public init(
        maximumAttempts: Int = 4,
        maximumWindowMilliseconds: Int = 125,
        retryDelaysMilliseconds: [Int] = [25, 35, 45]
    ) {
        self.maximumAttempts = max(1, maximumAttempts)
        self.maximumWindowMilliseconds = max(0, maximumWindowMilliseconds)
        self.retryDelaysMilliseconds = retryDelaysMilliseconds.map { max(0, $0) }
    }

    public func decision(
        for diagnostic: MacCaptureStartupDiagnostic
    ) -> MacCaptureStartupRecoveryDecision {
        guard diagnostic.isBoundedRecoveryCandidate,
            diagnostic.attempt < maximumAttempts,
            !retryDelaysMilliseconds.isEmpty
        else {
            return .fail
        }
        let index = min(diagnostic.attempt - 1, retryDelaysMilliseconds.count - 1)
        let delay = retryDelaysMilliseconds[index]
        guard diagnostic.elapsedMilliseconds + delay <= maximumWindowMilliseconds else {
            return .fail
        }
        return .retry(afterMilliseconds: delay)
    }
}

enum MacCaptureStartupAttemptError: Error {
    case layoutChanged(
        expected: MacCaptureDuplexLayout,
        observed: MacCaptureDuplexLayout
    )
    case invalidLayout(MacCaptureDuplexLayout)
}

/// Synchronous because AVAudioEngine startup is synchronous and this runs only
/// on the backend's private queue. The production sleep is bounded by policy;
/// tests inject a deterministic clock and sleeper.
struct MacCaptureStartupRecoverer {
    let policy: MacCaptureStartupRetryPolicy
    let nowMilliseconds: () -> Int
    let sleepMilliseconds: (Int) -> Void
    let onDiagnostic: (MacCaptureStartupDiagnostic, MacCaptureStartupRecoveryDecision) -> Void

    init(
        policy: MacCaptureStartupRetryPolicy = .init(),
        nowMilliseconds: @escaping () -> Int = {
            Int((ProcessInfo.processInfo.systemUptime * 1_000).rounded())
        },
        sleepMilliseconds: @escaping (Int) -> Void = {
            Thread.sleep(forTimeInterval: Double($0) / 1_000)
        },
        onDiagnostic:
            @escaping (
                MacCaptureStartupDiagnostic,
                MacCaptureStartupRecoveryDecision
            ) -> Void = { _, _ in }
    ) {
        self.policy = policy
        self.nowMilliseconds = nowMilliseconds
        self.sleepMilliseconds = sleepMilliseconds
        self.onDiagnostic = onDiagnostic
    }

    func start(
        stage: MacCaptureStartupStage,
        validateAndPrepare: () throws -> MacCaptureDuplexLayout,
        startEngine: (MacCaptureDuplexLayout) throws -> Void,
        resetAfterFailure: () -> Void
    ) throws -> MacCaptureDuplexLayout {
        let startedAt = nowMilliseconds()
        var attempt = 1
        while true {
            do {
                let layout = try validateAndPrepare()
                try startEngine(layout)
                return layout
            } catch {
                let diagnostic = Self.diagnostic(
                    from: error,
                    stage: stage,
                    attempt: attempt,
                    elapsedMilliseconds: max(0, nowMilliseconds() - startedAt))
                let decision = policy.decision(for: diagnostic)
                onDiagnostic(diagnostic, decision)
                resetAfterFailure()
                switch decision {
                case .retry(let delay):
                    sleepMilliseconds(delay)
                    attempt += 1
                case .fail:
                    throw diagnostic
                }
            }
        }
    }

    private static func diagnostic(
        from error: Error,
        stage: MacCaptureStartupStage,
        attempt: Int,
        elapsedMilliseconds: Int
    ) -> MacCaptureStartupDiagnostic {
        let cause: MacCaptureStartupCause
        switch error {
        case MacCaptureStartupAttemptError.layoutChanged(let expected, let observed):
            cause = .layoutChanged(expected: expected, observed: observed)
        case MacCaptureStartupAttemptError.invalidLayout(let layout):
            cause = .invalidLayout(layout)
        default:
            let cocoa = error as NSError
            if let status = coreAudioStatus(from: cocoa) {
                cause = .coreAudio(status: status)
            } else {
                cause = .engine(domain: cocoa.domain, code: cocoa.code)
            }
        }
        return MacCaptureStartupDiagnostic(
            stage: stage,
            attempt: attempt,
            elapsedMilliseconds: elapsedMilliseconds,
            cause: cause)
    }

    private static func coreAudioStatus(
        from error: NSError,
        depth: Int = 0
    ) -> Int32? {
        guard depth < 4 else { return nil }
        if error.domain == NSOSStatusErrorDomain,
            let status = Int32(exactly: error.code)
        {
            return status
        }
        if error.code == 35, error.domain.hasPrefix("com.apple.coreaudio") {
            return 35
        }
        if let underlying = error.userInfo[NSUnderlyingErrorKey] as? NSError {
            return coreAudioStatus(from: underlying, depth: depth + 1)
        }
        return nil
    }
}

/// Explicit release order for an AVAudioEngine attempt that reached prepare
/// but failed before or during start. `stopEngine` is intentionally
/// unconditional: AVAudioEngine.stop() releases resources allocated by
/// prepare even while `isRunning` is false.
struct MacCaptureStartupAttemptResources {
    let stopEngine: () -> Void
    let removeTap: () -> Void
    let resetEngine: () -> Void
    let resetMailbox: () -> Void

    func releaseAfterFailure() {
        stopEngine()
        removeTap()
        resetEngine()
        resetMailbox()
    }
}

/// Runs the initial startup stage and, only when policy and explicit consent
/// permit it, transitions once into the unprocessed fallback. The recoverer
/// releases failed-attempt resources before this type invokes the fallback
/// configuration closure.
struct MacCaptureStartupSequencer {
    let recoverer: MacCaptureStartupRecoverer

    func start(
        initialStage: MacCaptureStartupStage,
        voiceProcessingEnabled: Bool,
        degradedConsent: Bool,
        validateAndPrepare: () throws -> MacCaptureDuplexLayout,
        startEngine: (MacCaptureDuplexLayout) throws -> Void,
        releaseAfterFailure: () -> Void,
        configureConsentedFallback: () throws -> Void
    ) throws -> MacCaptureDuplexLayout {
        do {
            return try recoverer.start(
                stage: initialStage,
                validateAndPrepare: validateAndPrepare,
                startEngine: startEngine,
                resetAfterFailure: releaseAfterFailure)
        } catch let diagnostic as MacCaptureStartupDiagnostic {
            guard
                diagnostic.terminalAction(
                    voiceProcessingEnabled: voiceProcessingEnabled,
                    degradedConsent: degradedConsent
                ) == .startConsentedFallback
            else {
                throw diagnostic
            }
            try configureConsentedFallback()
            return try recoverer.start(
                stage: .consentedFallback,
                validateAndPrepare: validateAndPrepare,
                startEngine: startEngine,
                resetAfterFailure: releaseAfterFailure)
        }
    }
}
