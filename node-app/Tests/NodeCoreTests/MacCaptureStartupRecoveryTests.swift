import Foundation
import Testing

@testable import NodeCore

@Suite("macOS capture startup recovery")
struct MacCaptureStartupRecoveryTests {
    private let stableLayout = MacCaptureDuplexLayout(
        inputSampleRate: 48_000,
        inputChannels: 1,
        outputSampleRate: 44_100,
        outputChannels: 2)

    @Test("A prepared non-running failure releases resources before retry")
    func preparedNonRunningFailureReleasesResources() throws {
        var events: [String] = []
        var startCount = 0
        var prepared = false
        var isRunning = false
        let resources = MacCaptureStartupAttemptResources(
            stopEngine: {
                events.append("stop")
                prepared = false
                isRunning = false
            },
            removeTap: { events.append("remove-tap") },
            resetEngine: { events.append("reset-engine") },
            resetMailbox: { events.append("reset-mailbox") })
        let recoverer = MacCaptureStartupRecoverer(
            nowMilliseconds: { 0 },
            sleepMilliseconds: { _ in })

        _ = try recoverer.start(
            stage: .voiceProcessing,
            validateAndPrepare: {
                events.append("prepare-\(startCount + 1)")
                prepared = true
                return stableLayout
            },
            startEngine: { _ in
                startCount += 1
                events.append("start-\(startCount)")
                #expect(prepared)
                #expect(!isRunning)
                if startCount == 1 {
                    throw NSError(domain: NSOSStatusErrorDomain, code: 35)
                }
                isRunning = true
            },
            resetAfterFailure: resources.releaseAfterFailure)

        #expect(
            events == [
                "prepare-1",
                "start-1",
                "stop",
                "remove-tap",
                "reset-engine",
                "reset-mailbox",
                "prepare-2",
                "start-2",
            ])
        #expect(isRunning)
    }

    @Test("Terminal VPIO release precedes disabling processing and fallback")
    func releasePrecedesConsentedFallback() throws {
        var events: [String] = []
        var fallbackConfigured = false
        let resources = MacCaptureStartupAttemptResources(
            stopEngine: { events.append("stop") },
            removeTap: { events.append("remove-tap") },
            resetEngine: { events.append("reset-engine") },
            resetMailbox: { events.append("reset-mailbox") })
        let recoverer = MacCaptureStartupRecoverer(
            policy: MacCaptureStartupRetryPolicy(maximumAttempts: 1),
            nowMilliseconds: { 0 },
            sleepMilliseconds: { _ in })
        let sequencer = MacCaptureStartupSequencer(recoverer: recoverer)

        let result = try sequencer.start(
            initialStage: .voiceProcessing,
            voiceProcessingEnabled: true,
            degradedConsent: true,
            validateAndPrepare: {
                events.append(fallbackConfigured ? "prepare-fallback" : "prepare-vpio")
                return stableLayout
            },
            startEngine: { _ in
                events.append(fallbackConfigured ? "start-fallback" : "start-vpio")
                if !fallbackConfigured {
                    throw NSError(domain: NSOSStatusErrorDomain, code: 35)
                }
            },
            releaseAfterFailure: resources.releaseAfterFailure,
            configureConsentedFallback: {
                events.append("disable-vpio")
                fallbackConfigured = true
            })

        #expect(result == stableLayout)
        #expect(
            events == [
                "prepare-vpio",
                "start-vpio",
                "stop",
                "remove-tap",
                "reset-engine",
                "reset-mailbox",
                "disable-vpio",
                "prepare-fallback",
                "start-fallback",
            ])
    }

    @Test("A late-valid aggregate retries CoreAudio error 35 inside the safe window")
    func lateValidAggregate() throws {
        var now = 0
        var startCount = 0
        var resetCount = 0
        var decisions: [MacCaptureStartupRecoveryDecision] = []
        let recoverer = MacCaptureStartupRecoverer(
            nowMilliseconds: { now },
            sleepMilliseconds: { delay in now += delay },
            onDiagnostic: { _, decision in decisions.append(decision) })

        let result = try recoverer.start(
            stage: .voiceProcessing,
            validateAndPrepare: { stableLayout },
            startEngine: { _ in
                startCount += 1
                if startCount == 1 {
                    let status = NSError(domain: NSOSStatusErrorDomain, code: 35)
                    throw NSError(
                        domain: "com.apple.coreaudio.avfaudio",
                        code: -1,
                        userInfo: [NSUnderlyingErrorKey: status])
                }
            },
            resetAfterFailure: { resetCount += 1 })

        #expect(result == stableLayout)
        #expect(startCount == 2)
        #expect(resetCount == 1)
        #expect(now == 25)
        #expect(decisions == [.retry(afterMilliseconds: 25)])
    }

    @Test("Repeated channel-layout churn terminates with the last typed mismatch")
    func repeatedLayoutMismatch() {
        var now = 0
        var validationCount = 0
        var startCount = 0
        var resetCount = 0
        var decisions: [MacCaptureStartupRecoveryDecision] = []
        let observed = MacCaptureDuplexLayout(
            inputSampleRate: 48_000,
            inputChannels: 2,
            outputSampleRate: 44_100,
            outputChannels: 2)
        let recoverer = MacCaptureStartupRecoverer(
            nowMilliseconds: { now },
            sleepMilliseconds: { delay in now += delay },
            onDiagnostic: { _, decision in decisions.append(decision) })

        do {
            _ = try recoverer.start(
                stage: .voiceProcessing,
                validateAndPrepare: {
                    validationCount += 1
                    throw MacCaptureStartupAttemptError.layoutChanged(
                        expected: stableLayout,
                        observed: observed)
                },
                startEngine: { _ in startCount += 1 },
                resetAfterFailure: { resetCount += 1 })
            Issue.record("repeated layout churn unexpectedly started capture")
        } catch let diagnostic as MacCaptureStartupDiagnostic {
            #expect(diagnostic.stage == .voiceProcessing)
            #expect(diagnostic.attempt == 4)
            #expect(diagnostic.elapsedMilliseconds == 105)
            #expect(
                diagnostic.cause
                    == .layoutChanged(
                        expected: stableLayout,
                        observed: observed))
            #expect(diagnostic.isEligibleForConsentedFallback)
        } catch {
            Issue.record("unexpected diagnostic type: \(error)")
        }

        #expect(validationCount == 4)
        #expect(startCount == 0)
        #expect(resetCount == 4)
        #expect(
            decisions == [
                .retry(afterMilliseconds: 25),
                .retry(afterMilliseconds: 35),
                .retry(afterMilliseconds: 45),
                .fail,
            ])
    }

    @Test("Unrelated engine failures remain typed and never enter retry or fallback")
    func nonRetryableDiagnostic() {
        var sleepCount = 0
        var captured: MacCaptureStartupDiagnostic?
        let recoverer = MacCaptureStartupRecoverer(
            nowMilliseconds: { 10 },
            sleepMilliseconds: { _ in sleepCount += 1 },
            onDiagnostic: { diagnostic, _ in captured = diagnostic })

        do {
            _ = try recoverer.start(
                stage: .voiceProcessing,
                validateAndPrepare: { stableLayout },
                startEngine: { _ in
                    throw NSError(
                        domain: "com.apple.coreaudio.avfaudio",
                        code: -10_875)
                },
                resetAfterFailure: {})
            Issue.record("unrelated engine failure unexpectedly started capture")
        } catch let diagnostic as MacCaptureStartupDiagnostic {
            #expect(diagnostic == captured)
            #expect(
                diagnostic.cause
                    == .engine(
                        domain: "com.apple.coreaudio.avfaudio",
                        code: -10_875))
            #expect(!diagnostic.isBoundedRecoveryCandidate)
            #expect(!diagnostic.isEligibleForConsentedFallback)
            let fields = diagnostic.redactedLogFields(decision: .fail)
            #expect(fields["cause"] as? String == "engine")
            #expect(fields["decision"] as? String == "fail")
            #expect(fields["device_id"] == nil)
            #expect(fields["device_name"] == nil)
        } catch {
            Issue.record("unexpected diagnostic type: \(error)")
        }
        #expect(sleepCount == 0)
    }

    @Test("An unrelated NSError code 35 remains fail-closed")
    func unrelatedCode35() {
        var sleepCount = 0
        let recoverer = MacCaptureStartupRecoverer(
            nowMilliseconds: { 10 },
            sleepMilliseconds: { _ in sleepCount += 1 })

        do {
            _ = try recoverer.start(
                stage: .voiceProcessing,
                validateAndPrepare: { stableLayout },
                startEngine: { _ in
                    throw NSError(domain: NSCocoaErrorDomain, code: 35)
                },
                resetAfterFailure: {})
            Issue.record("unrelated NSError 35 unexpectedly started capture")
        } catch let diagnostic as MacCaptureStartupDiagnostic {
            #expect(
                diagnostic.cause
                    == .engine(
                        domain: NSCocoaErrorDomain,
                        code: 35))
            #expect(!diagnostic.isBoundedRecoveryCandidate)
            #expect(!diagnostic.isEligibleForConsentedFallback)
        } catch {
            Issue.record("unexpected diagnostic type: \(error)")
        }
        #expect(sleepCount == 0)
    }

    @Test("Only explicit consent unlocks fallback after a bounded startup race")
    func consentedFallbackGate() {
        let race = MacCaptureStartupDiagnostic(
            stage: .voiceProcessing,
            attempt: 4,
            elapsedMilliseconds: 105,
            cause: .coreAudio(status: 35))
        #expect(
            race.terminalAction(
                voiceProcessingEnabled: true,
                degradedConsent: false) == .failClosed)
        #expect(
            race.terminalAction(
                voiceProcessingEnabled: true,
                degradedConsent: true) == .startConsentedFallback)
        #expect(
            race.terminalAction(
                voiceProcessingEnabled: false,
                degradedConsent: true) == .failClosed)

        let fallbackRace = MacCaptureStartupDiagnostic(
            stage: .consentedFallback,
            attempt: 4,
            elapsedMilliseconds: 105,
            cause: .coreAudio(status: 35))
        #expect(!fallbackRace.isEligibleForConsentedFallback)

        let inputSelectionRace = MacCaptureStartupDiagnostic(
            stage: .inputSelection,
            attempt: 1,
            elapsedMilliseconds: 0,
            cause: .coreAudio(status: 35))
        #expect(!inputSelectionRace.isEligibleForConsentedFallback)

        let unrelated = MacCaptureStartupDiagnostic(
            stage: .voiceProcessing,
            attempt: 1,
            elapsedMilliseconds: 0,
            cause: .engine(domain: "com.apple.coreaudio.avfaudio", code: -1))
        #expect(
            unrelated.terminalAction(
                voiceProcessingEnabled: true,
                degradedConsent: true) == .failClosed)
    }
}
