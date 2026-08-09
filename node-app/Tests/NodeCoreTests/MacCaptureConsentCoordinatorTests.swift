import Testing

@testable import NodeCore

@Suite("macOS capture consent coordinator")
struct MacCaptureConsentCoordinatorTests {
    @Test("Built-in speaker quality rejection offers actionable consent")
    func builtInSpeakerRejection() {
        var coordinator = MacCaptureConsentCoordinator()
        let request = coordinator.configure(
            mode: .speaker,
            degradedConsent: false)

        coordinator.beginAttempt()
        let prompt = coordinator.finishAttempt(
            after: .captureQualityUnsupported)

        #expect(request == MacCaptureQualityRequest(mode: .speaker))
        #expect(prompt == .captureQuality)
        #expect(coordinator.prompt == .captureQuality)
        #expect(!coordinator.isDegradedConsentGranted)
    }

    @Test("Accepted headphone capture completes without a consent prompt")
    func acceptedHeadphoneRoute() {
        var coordinator = MacCaptureConsentCoordinator()
        let request = coordinator.configure(
            mode: .headphone,
            degradedConsent: false)

        coordinator.beginAttempt()
        coordinator.finishAttempt()

        #expect(request == MacCaptureQualityRequest(mode: .headphone))
        #expect(coordinator.selectedMode == .headphone)
        #expect(coordinator.prompt == nil)
        #expect(!coordinator.isDegradedConsentGranted)
        #expect(coordinator.pendingResetRequest == nil)
    }

    @Test("Cancel stays fail-closed on headphones and never retries")
    func cancelStaysFailClosed() {
        var coordinator = MacCaptureConsentCoordinator()
        _ = coordinator.configure(mode: .speaker, degradedConsent: false)
        coordinator.beginAttempt()
        _ = coordinator.finishAttempt(after: .captureQualityUnsupported)

        let resolution = coordinator.resolvePrompt(
            allowLimitedRecording: false)
        let repeatedResolution = coordinator.resolvePrompt(
            allowLimitedRecording: true)
        var retryCount = 0
        if case .retry = resolution { retryCount += 1 }
        if case .retry = repeatedResolution { retryCount += 1 }

        #expect(
            resolution
                == .cancel(
                    MacCaptureQualityRequest(
                        mode: .headphone,
                        degradedConsent: false)))
        #expect(repeatedResolution == nil)
        #expect(retryCount == 0)
        #expect(coordinator.selectedMode == .headphone)
        #expect(!coordinator.isDegradedConsentGranted)
        #expect(coordinator.prompt == nil)
    }

    @Test("Allow retries once and revokes consent through a deferred reset")
    func retryAndDeferredReset() {
        var coordinator = MacCaptureConsentCoordinator()
        _ = coordinator.configure(mode: .speaker, degradedConsent: false)
        coordinator.beginAttempt()
        _ = coordinator.finishAttempt(after: .captureQualityUnsupported)

        let resolution = coordinator.resolvePrompt(
            allowLimitedRecording: true)
        let repeatedResolution = coordinator.resolvePrompt(
            allowLimitedRecording: true)
        var retryCount = 0
        if case .retry = resolution { retryCount += 1 }
        if case .retry = repeatedResolution { retryCount += 1 }

        #expect(
            resolution
                == .retry(
                    MacCaptureQualityRequest(
                        mode: .speaker,
                        degradedConsent: true)))
        #expect(repeatedResolution == nil)
        #expect(retryCount == 1)
        #expect(coordinator.isDegradedConsentGranted)

        let resetRequest = coordinator.qualityGenerationClosed()
        #expect(
            resetRequest
                == MacCaptureQualityRequest(
                    mode: .speaker,
                    degradedConsent: false))
        #expect(!coordinator.isDegradedConsentGranted)
        #expect(coordinator.pendingResetRequest == resetRequest)

        coordinator.didApplyPendingReset(false)
        #expect(coordinator.pendingResetRequest == resetRequest)
        coordinator.finishAttempt()
        coordinator.didApplyPendingReset(true)
        #expect(coordinator.pendingResetRequest == nil)
    }

    @Test("A consented terminal fallback failure cannot prompt again")
    func consentedTerminalFallbackCannotReprompt() {
        var coordinator = MacCaptureConsentCoordinator()
        _ = coordinator.configure(mode: .speaker, degradedConsent: false)
        coordinator.beginAttempt()
        _ = coordinator.finishAttempt(after: .captureQualityUnsupported)
        _ = coordinator.resolvePrompt(allowLimitedRecording: true)
        _ = coordinator.qualityGenerationClosed()

        let terminalFallback = MacCaptureStartupDiagnostic(
            stage: .consentedFallback,
            attempt: 4,
            elapsedMilliseconds: 105,
            cause: .coreAudio(status: 35))
        let prompt = coordinator.finishAttempt(
            after: .backendStartupFailed(terminalFallback))

        #expect(prompt == nil)
        #expect(coordinator.prompt == nil)
        #expect(!coordinator.isDegradedConsentGranted)
    }

    @Test("An initial bounded VPIO failure offers the startup fallback")
    func initialVPIOFailureOffersFallback() {
        var coordinator = MacCaptureConsentCoordinator()
        _ = coordinator.configure(mode: .speaker, degradedConsent: false)
        coordinator.beginAttempt()
        let initialFailure = MacCaptureStartupDiagnostic(
            stage: .voiceProcessing,
            attempt: 4,
            elapsedMilliseconds: 105,
            cause: .coreAudio(status: 35))

        let prompt = coordinator.finishAttempt(
            after: .backendStartupFailed(initialFailure))

        #expect(prompt == .startupFallback)
        #expect(coordinator.prompt == .startupFallback)
    }
}
