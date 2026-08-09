public enum MacCaptureConsentPrompt: Equatable, Sendable {
    case captureQuality
    case startupFallback
}

public enum MacCaptureConsentFailure: Equatable, Sendable {
    case captureQualityUnsupported
    case backendStartupFailed(MacCaptureStartupDiagnostic)
    case other
}

public enum MacCaptureConsentResolution: Equatable, Sendable {
    case cancel(MacCaptureQualityRequest)
    case retry(MacCaptureQualityRequest)
}

/// Owns the fail-closed, one-generation consent state used by the production
/// macOS composition. UI presentation and workflow execution remain adapters
/// around this deterministic state machine.
public struct MacCaptureConsentCoordinator: Equatable, Sendable {
    public private(set) var selectedMode: MacCaptureQualityMode
    public private(set) var prompt: MacCaptureConsentPrompt?
    private var oneGenerationConsent: MacCaptureOneGenerationConsent
    private var resetPending = false

    public init(
        selectedMode: MacCaptureQualityMode = .auto,
        degradedConsent: Bool = false
    ) {
        self.selectedMode = selectedMode
        oneGenerationConsent = MacCaptureOneGenerationConsent(
            isGranted: degradedConsent)
    }

    public var isDegradedConsentGranted: Bool {
        oneGenerationConsent.isGranted
    }

    @discardableResult
    public mutating func configure(
        mode: MacCaptureQualityMode,
        degradedConsent: Bool
    ) -> MacCaptureQualityRequest {
        selectedMode = mode
        prompt = nil
        resetPending = false
        oneGenerationConsent = MacCaptureOneGenerationConsent(
            isGranted: degradedConsent)
        return request(degradedConsent: degradedConsent)
    }

    public mutating func beginAttempt() {
        prompt = nil
        oneGenerationConsent.beginAttempt()
    }

    @discardableResult
    public mutating func finishAttempt(
        after failure: MacCaptureConsentFailure
    ) -> MacCaptureConsentPrompt? {
        if oneGenerationConsent.shouldOfferAfterFailure {
            switch failure {
            case .captureQualityUnsupported:
                prompt = .captureQuality
            case .backendStartupFailed(let diagnostic)
            where diagnostic.isEligibleForConsentedFallback:
                prompt = .startupFallback
            case .backendStartupFailed, .other:
                prompt = nil
            }
        } else {
            prompt = nil
        }
        oneGenerationConsent.finishAttempt()
        return prompt
    }

    public mutating func finishAttempt() {
        oneGenerationConsent.finishAttempt()
    }

    public mutating func resolvePrompt(
        allowLimitedRecording: Bool
    ) -> MacCaptureConsentResolution? {
        guard prompt != nil else { return nil }
        prompt = nil
        resetPending = false

        guard allowLimitedRecording else {
            selectedMode = .headphone
            oneGenerationConsent = MacCaptureOneGenerationConsent()
            return .cancel(request(degradedConsent: false))
        }

        oneGenerationConsent.setGranted(true)
        oneGenerationConsent.beginAttempt()
        return .retry(request(degradedConsent: true))
    }

    /// Revoke consent as soon as the backend closes its quality generation.
    /// The returned request may be deferred by the production workflow while
    /// it publishes the terminal recording event.
    @discardableResult
    public mutating func qualityGenerationClosed() -> MacCaptureQualityRequest? {
        guard oneGenerationConsent.resetGrantAfterGeneration() else { return nil }
        resetPending = true
        return request(degradedConsent: false)
    }

    public var pendingResetRequest: MacCaptureQualityRequest? {
        resetPending ? request(degradedConsent: false) : nil
    }

    public mutating func didApplyPendingReset(_ applied: Bool) {
        if applied { resetPending = false }
    }

    private func request(degradedConsent: Bool) -> MacCaptureQualityRequest {
        MacCaptureQualityRequest(
            mode: selectedMode,
            processingRequested: true,
            degradedConsent: degradedConsent)
    }
}
