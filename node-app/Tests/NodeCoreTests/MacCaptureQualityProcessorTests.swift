import Foundation
import Testing

@testable import NodeCore

@Suite("macOS capture quality processor")
struct MacCaptureQualityProcessorTests {
    @Test("bounded AGC applies the input ceiling last and respects gain and slew")
    func boundedAGC() {
        let processor = MacCaptureInputSafetyProcessor()
        var loud = [Float](repeating: 1, count: 48_000)
        let loudMetrics = processor.process(&loud)
        #expect(loudMetrics.peakDBFS <= -3 + 0.001)
        #expect(loudMetrics.appliedGainDB <= 0.001)
        #expect(loud.allSatisfy { abs($0) <= Float(pow(10, -3.0 / 20.0)) + 0.000_001 })

        processor.reset()
        var quiet = [Float](repeating: 0.001, count: 48_000)
        let first = processor.process(&quiet)
        #expect(first.appliedGainDB <= 3.001)
        #expect(first.appliedGainDB >= 2.999)
        var settled = first
        for _ in 0..<4 {
            var next = [Float](repeating: 0.001, count: 48_000)
            settled = processor.process(&next)
        }
        #expect(settled.appliedGainDB <= MacCaptureInputSafetyProcessor.maximumGainDB + 0.001)
        #expect(settled.appliedGainDB >= MacCaptureInputSafetyProcessor.maximumGainDB - 0.001)
    }

    @Test("empty and silent inputs stay finite and content free")
    func emptyAndSilent() {
        let processor = MacCaptureInputSafetyProcessor()
        var empty: [Float] = []
        let emptyMetrics = processor.process(&empty)
        #expect(emptyMetrics.rmsDBFS == -240)
        #expect(emptyMetrics.peakDBFS == -240)
        var silence = [Float](repeating: 0, count: 960)
        let silenceMetrics = processor.process(&silence)
        #expect(silenceMetrics.rmsDBFS == -240)
        #expect(silenceMetrics.peakDBFS == -240)
        #expect(silenceMetrics.clippedFraction == 0)
    }

    @Test("route decisions never infer accepted speaker or unknown quality")
    func routeDecisions() throws {
        let auto = MacCaptureQualityRequest(mode: .auto)
        let headphone = MacCaptureQualityDecision.evaluate(
            request: auto, resolvedMode: "headphone", voiceProcessingEnabled: true)
        #expect(headphone.quality == "accepted")
        #expect(headphone.reason == "none")

        let speaker = MacCaptureQualityDecision.evaluate(
            request: auto, resolvedMode: "speaker", voiceProcessingEnabled: true)
        #expect(speaker.quality == "degraded")
        #expect(speaker.reason == "reference_unavailable")

        let unknown = MacCaptureQualityDecision.evaluate(
            request: auto, resolvedMode: "unknown", voiceProcessingEnabled: true)
        #expect(unknown.quality == "degraded")
        #expect(unknown.reason == "route_unknown")

        let unavailable = MacCaptureQualityDecision.evaluate(
            request: auto, resolvedMode: "headphone", voiceProcessingEnabled: false)
        #expect(unavailable.agc == "active")
        #expect(unavailable.aec == "unavailable")
        #expect(unavailable.reason == "aec_unavailable")

        let explicitMismatch = MacCaptureQualityDecision.evaluate(
            request: MacCaptureQualityRequest(mode: .speaker, degradedConsent: true),
            resolvedMode: "headphone", voiceProcessingEnabled: true)
        #expect(explicitMismatch.reason == "route_excluded")

        let legacy = MacCaptureQualityDecision.evaluate(
            request: .legacyUnprocessed,
            resolvedMode: "unknown", voiceProcessingEnabled: false)
        #expect(legacy.reason == "user_selected_unprocessed")
        #expect(legacy.agc == "unavailable")

        for (mode, decision) in [
            ("headphone", headphone), ("speaker", speaker), ("unknown", unknown),
        ] {
            let state = MacCaptureQualitySession(
                generation: 1, workflow: "live_ptt", request: auto,
                resolvedMode: mode, quality: decision.quality, aec: decision.aec,
                ns: decision.ns, agc: decision.agc, reason: decision.reason
            ).state(lifecycle: "capturing", nowMs: 1)
            try CaptureQualityContract.validate(state)
        }
    }

    @Test("quality generations are strictly fresh")
    func freshGenerations() {
        let first = MacCaptureQualityGeneration.next()
        let second = MacCaptureQualityGeneration.next()
        #expect(second == first + 1)
    }

    @Test("Degraded consent is fail-closed, attempt-scoped, and reset after one generation")
    func oneGenerationConsent() {
        var consent = MacCaptureOneGenerationConsent()
        #expect(!consent.isGranted)
        consent.beginAttempt()
        #expect(consent.shouldOfferAfterFailure)

        consent.setGranted(true)
        consent.beginAttempt()
        #expect(consent.isGranted)
        #expect(!consent.shouldOfferAfterFailure)

        let didReset = consent.resetGrantAfterGeneration()
        #expect(didReset)
        #expect(!consent.isGranted)
        #expect(!consent.shouldOfferAfterFailure)
        consent.finishAttempt()
        #expect(consent.shouldOfferAfterFailure)

        consent.setGranted(false)
        consent.beginAttempt()
        #expect(!consent.isGranted)
        #expect(consent.shouldOfferAfterFailure)
    }
}
