import CryptoKit
import Foundation
import Testing
@testable import NodeCore

@Suite("macOS integrated capture-quality adapter")
struct MacCaptureQualityIntegratedAdapterTests {
    private let fixtureIDs = [
        "far_end_only", "near_end_only", "double_talk", "echo_path_change",
        "route_change", "clock_drift", "clipping", "too_quiet", "silence",
        "device_loss", "processor_overrun", "missing_reference", "effect_failure",
        "live_packet_cancel",
    ]

    @Test("all frozen fixtures cross every workflow and route without a C3 claim")
    func integratedAdapter() throws {
        let environment = ProcessInfo.processInfo.environment
        guard let corpusValue = environment["CAPTURE_QUALITY_CORPUS"],
              let outputValue = environment["CAPTURE_QUALITY_ADAPTER_OUTPUT"]
        else {
            // The ordinary package suite covers the processor directly. The
            // cross-platform orchestrator supplies the content-locked corpus.
            return
        }
        #expect(CaptureQualityContract.inputCeilingDBFS == -3)
        #expect(CaptureQualityContract.outputCeilingDBFS == -1)
        let corpus = URL(fileURLWithPath: corpusValue, isDirectory: true)
        let started = DispatchTime.now().uptimeNanoseconds
        var latencies: [Double] = []
        var cells: [[String: Any]] = []
        var generations = Set<Int64>()
        for workflow in ["recorded_clip", "local_self_test", "live_ptt"] {
            for route in ["speaker", "headphone", "unknown"] {
                let request = MacCaptureQualityRequest(mode: .auto)
                let decision = MacCaptureQualityDecision.evaluate(
                    request: request, resolvedMode: route, voiceProcessingEnabled: false)
                #expect(decision.quality == "degraded")
                #expect(decision.reason == "aec_unavailable")
                let session = MacCaptureQualitySession(
                    generation: MacCaptureQualityGeneration.next(), workflow: workflow,
                    request: request, resolvedMode: route, quality: decision.quality,
                    aec: decision.aec, ns: decision.ns, agc: decision.agc,
                    reason: decision.reason)
                #expect(generations.insert(session.generation).inserted)
                try CaptureQualityContract.validate(
                    session.state(lifecycle: "preparing", nowMs: 1))
                try CaptureQualityContract.validate(
                    session.state(lifecycle: "failed", nowMs: 2))
                let consentedRequest = MacCaptureQualityRequest(
                    mode: .auto, degradedConsent: true)
                let consentedDecision = MacCaptureQualityDecision.evaluate(
                    request: consentedRequest, resolvedMode: route,
                    voiceProcessingEnabled: false)
                let consented = MacCaptureQualitySession(
                    generation: MacCaptureQualityGeneration.next(), workflow: workflow,
                    request: consentedRequest, resolvedMode: route,
                    quality: consentedDecision.quality, aec: consentedDecision.aec,
                    ns: consentedDecision.ns, agc: consentedDecision.agc,
                    reason: consentedDecision.reason)
                for lifecycle in ["preparing", "capturing", "stopping"] {
                    try CaptureQualityContract.validate(
                        consented.state(lifecycle: lifecycle, nowMs: 3))
                }

                var cases: [[String: Any]] = []
                for fixtureID in fixtureIDs {
                    var samples = try decodeFloat32(
                        Data(contentsOf: corpus.appendingPathComponent(
                            "\(fixtureID).capture.f32le")))
                    let processor = MacCaptureInputSafetyProcessor(sampleRate: 48_000)
                    var output = Data(capacity: samples.count * 4)
                    var maximumPeak = 0.0
                    var maximumGain = 0.0
                    var maximumSlew = 0.0
                    var clipped = 0
                    var previousGain = processor.currentGainDB
                    var start = 0
                    while start < samples.count {
                        let end = min(start + 480, samples.count)
                        var block = Array(samples[start..<end])
                        let before = DispatchTime.now().uptimeNanoseconds
                        let metrics = processor.process(&block)
                        let after = DispatchTime.now().uptimeNanoseconds
                        latencies.append(Double(after - before) / 1_000_000)
                        maximumPeak = max(maximumPeak, pow(10, metrics.peakDBFS / 20))
                        maximumGain = max(maximumGain, processor.currentGainDB)
                        let seconds = Double(block.count) / 48_000
                        maximumSlew = max(
                            maximumSlew,
                            abs(processor.currentGainDB - previousGain) / seconds)
                        previousGain = processor.currentGainDB
                        for (offset, sample) in block.enumerated() {
                            #expect(sample.isFinite)
                            #expect(abs(sample) <= Float(pow(10, -3.0 / 20.0)) + 0.000_001)
                            if abs(Double(sample)) >= pow(10, -3.0 / 20.0) - 1e-7 {
                                clipped += 1
                            }
                            appendFloat32(sample, to: &output)
                            samples[start + offset] = sample
                        }
                        start = end
                    }
                    let maximumPeakDBFS = db(maximumPeak)
                    #expect(maximumPeakDBFS <= MacCaptureInputSafetyProcessor.peakCeilingDBFS + 0.001)
                    #expect(maximumGain <= MacCaptureInputSafetyProcessor.maximumGainDB + 0.001)
                    #expect(maximumSlew <= MacCaptureInputSafetyProcessor.maximumGainChangeDBPerSecond + 0.001)
                    cases.append([
                        "id": fixtureID,
                        "sampleCount": samples.count,
                        "processedSHA256": SHA256.hash(data: output).hex,
                        "maximumPeakDBFS": rounded(maximumPeakDBFS),
                        "maximumAppliedGainDB": rounded(maximumGain),
                        "maximumGainSlewDBPerSecond": rounded(maximumSlew),
                        "clippedFraction": rounded(Double(clipped) / Double(max(1, samples.count))),
                        "safetyStagePassed": true,
                        "c3Status": "unsupported-native-effects-not-exercised",
                    ])
                }
                cells.append([
                    "workflow": workflow,
                    "route": route,
                    "quality": decision.quality,
                    "reason": decision.reason,
                    "supported": false,
                    "blocker": "signed-vpio-not-exercised",
                    "failClosedWithoutConsent": true,
                    "freshGeneration": session.generation,
                    "cases": cases,
                ])
            }
        }
        latencies.sort()
        let report: [String: Any] = [
            "schemaVersion": 1,
            "contract": "p3-capture-quality-platform-adapter.v1",
            "platform": "macos",
            "build": environment["CAPTURE_QUALITY_BUILD"] ?? "unknown",
            "fixtureLockSHA256": environment["CAPTURE_QUALITY_FIXTURE_LOCK_SHA256"] ?? "",
            "manualEvidence": "not-run",
            "cells": cells,
            "runtime": [
                "adapterDurationMS": Int64(DispatchTime.now().uptimeNanoseconds - started) / 1_000_000,
                "processorBlockLatencyP95MS": percentile(latencies, fraction: 0.95),
                "callbackAllocations": 0,
                "callbackAllocationMeasurement": "source-guard-only",
                "callbackBlockingWaits": 0,
                "measurementSource": "repository-test-adapter",
                "physicalCPUAndMemoryEvidence": "not-run",
            ],
        ]
        let data = try JSONSerialization.data(
            withJSONObject: report, options: [.prettyPrinted, .sortedKeys])
        var terminated = data
        terminated.append(0x0A)
        try terminated.write(to: URL(fileURLWithPath: outputValue), options: .atomic)
    }

    private func decodeFloat32(_ data: Data) throws -> [Float] {
        guard data.count.isMultiple(of: 4) else {
            throw AdapterError.invalidFloat32Length
        }
        var result: [Float] = []
        result.reserveCapacity(data.count / 4)
        for offset in stride(from: 0, to: data.count, by: 4) {
            let bits = UInt32(data[offset])
                | UInt32(data[offset + 1]) << 8
                | UInt32(data[offset + 2]) << 16
                | UInt32(data[offset + 3]) << 24
            let value = Float(bitPattern: bits)
            guard value.isFinite, abs(value) <= 1 else {
                throw AdapterError.invalidFloat32Value
            }
            result.append(value)
        }
        return result
    }

    private func appendFloat32(_ value: Float, to data: inout Data) {
        let bits = value.bitPattern.littleEndian
        data.append(UInt8(truncatingIfNeeded: bits))
        data.append(UInt8(truncatingIfNeeded: bits >> 8))
        data.append(UInt8(truncatingIfNeeded: bits >> 16))
        data.append(UInt8(truncatingIfNeeded: bits >> 24))
    }

    private func db(_ value: Double) -> Double {
        20 * log10(max(value, 1e-12))
    }

    private func rounded(_ value: Double) -> Double {
        (value * 1_000_000).rounded() / 1_000_000
    }

    private func percentile(_ values: [Double], fraction: Double) -> Double {
        guard !values.isEmpty else { return 0 }
        let index = Int(floor(Double(values.count - 1) * fraction))
        return rounded(values[index])
    }
}

private enum AdapterError: Error {
    case invalidFloat32Length
    case invalidFloat32Value
}

private extension Digest {
    var hex: String { map { String(format: "%02x", $0) }.joined() }
}
