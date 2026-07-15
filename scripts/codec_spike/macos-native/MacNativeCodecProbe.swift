import AVFoundation
import AudioToolbox
import CoreMedia
import Darwin
import Foundation
import Security
import UniformTypeIdentifiers

private let contractName = "p2-macos-native-streaming-decoder-probe.v1"
private let maximumReadBytes = 65_536

private struct FixtureSpec: Codable {
    let id: String
    let file: String
    let codec: String
    let container: String
}

private struct ProbeContract: Codable {
    let schemaVersion: Int
    let contract: String
    let candidateId: String
    let fixtures: [FixtureSpec]
}

private struct FixtureEvidence: Codable {
    let id: String
    let codec: String
    let container: String
    let outcome: String
    let errorDomain: String?
    let errorCode: Int?
    let errorDescription: String?
    let sourceBytes: Int64
    let bytesBeforeFirstSample: Int64
    let totalSourceBytesRead: Int64
    let readOperations: Int
    let maximumReadBytes: Int
    let samples: Int
    let pcmBytes: Int64
    let firstTimestampSeconds: Double?
    let durationSeconds: Double?
    let trackStartMS: Int?
    let scheduledSkewMS: Int?
    let pausedWithoutRead: Bool
    let seekGeneration: Int
    let seekToSampleMS: Int?
    let resumed: Bool
    let drained: Bool
    let cancelled: Bool
    let startBeforeFullFile: Bool
    let passedLifecycle: Bool
}

private struct ProbeEvidence: Codable {
    let schemaVersion: Int
    let contract: String
    let candidateId: String
    let claimClass: String
    let osVersion: String
    let architecture: String
    let bundleIdentifier: String
    let sandboxEntitlement: Bool
    let networkClientEntitlement: Bool
    let renderCallbackUsed: Bool
    let decoderOwnsNetwork: Bool
    let maximumUnderlyingReadBytes: Int
    let rssStartBytes: Int64
    let peakRSSBytes: Int64
    let fixtures: [FixtureEvidence]
    let shippingDecision: String
    let passed: Bool
}

private final class LockedMetrics {
    private let lock = NSLock()
    private var bytesReadValue: Int64 = 0
    private var operationsValue = 0
    private var maximumReadValue = 0

    func record(_ count: Int) {
        lock.lock()
        bytesReadValue += Int64(count)
        operationsValue += 1
        maximumReadValue = max(maximumReadValue, count)
        lock.unlock()
    }

    func snapshot() -> (bytes: Int64, operations: Int, maximumRead: Int) {
        lock.lock()
        defer { lock.unlock() }
        return (bytesReadValue, operationsValue, maximumReadValue)
    }
}

private final class RangeResourceLoader: NSObject, AVAssetResourceLoaderDelegate {
    let sourceURL: URL
    let contentType: String
    let size: Int64
    let metrics = LockedMetrics()
    private let queue: DispatchQueue

    init(sourceURL: URL, fixtureID: String) throws {
        self.sourceURL = sourceURL
        let attributes = try FileManager.default.attributesOfItem(atPath: sourceURL.path)
        self.size = (attributes[.size] as? NSNumber)?.int64Value ?? 0
        self.contentType = UTType(filenameExtension: sourceURL.pathExtension)?.identifier ?? "public.data"
        self.queue = DispatchQueue(label: "live.barycenter.codec.range.\(fixtureID)")
    }

    func install(on asset: AVURLAsset) {
        asset.resourceLoader.setDelegate(self, queue: queue)
    }

    func resourceLoader(
        _ resourceLoader: AVAssetResourceLoader,
        shouldWaitForLoadingOfRequestedResource loadingRequest: AVAssetResourceLoadingRequest
    ) -> Bool {
        queue.async { [self] in
            do {
                if let information = loadingRequest.contentInformationRequest {
                    information.contentType = contentType
                    information.contentLength = size
                    information.isByteRangeAccessSupported = true
                }

                if let request = loadingRequest.dataRequest {
                    let requestedStart = max(request.requestedOffset, request.currentOffset)
                    let requestedEnd = min(size, request.requestedOffset + Int64(request.requestedLength))
                    if requestedStart < 0 || requestedStart > size || requestedEnd < requestedStart {
                        throw NSError(domain: "PulsarRangeLoader", code: 416, userInfo: [
                            NSLocalizedDescriptionKey: "invalid byte range \(requestedStart)..<\(requestedEnd) for \(size)"
                        ])
                    }

                    let handle = try FileHandle(forReadingFrom: sourceURL)
                    defer { try? handle.close() }
                    try handle.seek(toOffset: UInt64(requestedStart))
                    var remaining = Int(requestedEnd - requestedStart)
                    while remaining > 0 {
                        let requested = min(maximumReadBytes, remaining)
                        guard let data = try handle.read(upToCount: requested), !data.isEmpty else {
                            throw NSError(domain: "PulsarRangeLoader", code: 502, userInfo: [
                                NSLocalizedDescriptionKey: "unexpected EOF with \(remaining) bytes remaining"
                            ])
                        }
                        metrics.record(data.count)
                        request.respond(with: data)
                        remaining -= data.count
                    }
                }
                loadingRequest.finishLoading()
            } catch {
                loadingRequest.finishLoading(with: error)
            }
        }
        return true
    }

    func resourceLoader(
        _ resourceLoader: AVAssetResourceLoader,
        didCancel loadingRequest: AVAssetResourceLoadingRequest
    ) {}
}

private struct LoadedAsset {
    let asset: AVURLAsset
    let loader: RangeResourceLoader
    let track: AVAssetTrack
    let duration: CMTime
}

private func loadAsset(sourceURL: URL, fixtureID: String) throws -> LoadedAsset {
    let escapedName = sourceURL.lastPathComponent.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? fixtureID
    guard let virtualURL = URL(string: "pulsar-range://fixture/\(escapedName)") else {
        throw NSError(domain: "PulsarNativeProbe", code: 1, userInfo: [NSLocalizedDescriptionKey: "invalid virtual URL"])
    }
    let asset = AVURLAsset(url: virtualURL)
    let loader = try RangeResourceLoader(sourceURL: sourceURL, fixtureID: fixtureID)
    loader.install(on: asset)

    let semaphore = DispatchSemaphore(value: 0)
    var result: Result<(AVAssetTrack, CMTime), Error>?
    Task {
        do {
            guard let track = try await asset.loadTracks(withMediaType: .audio).first else {
                throw NSError(domain: "PulsarNativeProbe", code: 2, userInfo: [NSLocalizedDescriptionKey: "asset has no audio track"])
            }
            let duration = try await asset.load(.duration)
            result = .success((track, duration))
        } catch {
            result = .failure(error)
        }
        semaphore.signal()
    }
    semaphore.wait()
    let loaded = try result!.get()
    return LoadedAsset(asset: asset, loader: loader, track: loaded.0, duration: loaded.1)
}

private func outputSettings() -> [String: Any] {
    [
        AVFormatIDKey: kAudioFormatLinearPCM,
        AVLinearPCMIsFloatKey: true,
        AVLinearPCMBitDepthKey: 32,
        AVLinearPCMIsNonInterleaved: false,
        AVSampleRateKey: 48_000,
        AVNumberOfChannelsKey: 2,
    ]
}

private func makeReader(_ loaded: LoadedAsset, startSeconds: Double = 0) throws -> (AVAssetReader, AVAssetReaderTrackOutput) {
    let reader = try AVAssetReader(asset: loaded.asset)
    let output = AVAssetReaderTrackOutput(track: loaded.track, outputSettings: outputSettings())
    output.alwaysCopiesSampleData = false
    guard reader.canAdd(output) else {
        throw NSError(domain: "PulsarNativeProbe", code: 3, userInfo: [NSLocalizedDescriptionKey: "AVAssetReader rejected PCM output"])
    }
    reader.add(output)
    if startSeconds > 0 {
        let start = CMTime(seconds: startSeconds, preferredTimescale: 48_000)
        reader.timeRange = CMTimeRange(start: start, duration: CMTimeSubtract(loaded.duration, start))
    }
    return (reader, output)
}

private func monotonicNS() -> UInt64 {
    DispatchTime.now().uptimeNanoseconds
}

private func milliseconds(_ later: UInt64, since earlier: UInt64) -> Int {
    Int((later - earlier) / 1_000_000)
}

private func decodeFixture(_ spec: FixtureSpec, resources: URL) -> FixtureEvidence {
    let sourceURL = resources.appendingPathComponent(spec.file)
    let sourceSize = ((try? FileManager.default.attributesOfItem(atPath: sourceURL.path)[.size]) as? NSNumber)?.int64Value ?? 0
    var loader: RangeResourceLoader?
    do {
        let loaded = try loadAsset(sourceURL: sourceURL, fixtureID: spec.id)
        loader = loaded.loader
        let pair = try makeReader(loaded)
        let reader = pair.0
        let output = pair.1

        let scheduled = monotonicNS() + 25_000_000
        while monotonicNS() < scheduled {
            usleep(500)
        }
        let startedAt = monotonicNS()
        guard reader.startReading() else {
            throw reader.error ?? NSError(domain: "PulsarNativeProbe", code: 4, userInfo: [NSLocalizedDescriptionKey: "reader did not start"])
        }
        guard let first = output.copyNextSampleBuffer() else {
            throw reader.error ?? NSError(domain: "PulsarNativeProbe", code: 5, userInfo: [NSLocalizedDescriptionKey: "reader produced no PCM"])
        }
        let firstAt = monotonicNS()
        let bytesBeforeFirst = loaded.loader.metrics.snapshot().bytes
        let beforePause = loaded.loader.metrics.snapshot().bytes
        usleep(20_000)
        let pausedWithoutRead = loaded.loader.metrics.snapshot().bytes == beforePause

        var samples = 1
        var pcmBytes = Int64(CMSampleBufferGetTotalSampleSize(first))
        let firstTimestamp = CMSampleBufferGetPresentationTimeStamp(first).seconds
        var lastEnd = CMTimeAdd(
            CMSampleBufferGetPresentationTimeStamp(first),
            CMSampleBufferGetDuration(first)
        )
        while let sample = output.copyNextSampleBuffer() {
            samples += 1
            pcmBytes += Int64(CMSampleBufferGetTotalSampleSize(sample))
            lastEnd = CMTimeAdd(
                CMSampleBufferGetPresentationTimeStamp(sample),
                CMSampleBufferGetDuration(sample)
            )
        }
        let drained = reader.status == .completed
        if !drained {
            throw reader.error ?? NSError(domain: "PulsarNativeProbe", code: 6, userInfo: [NSLocalizedDescriptionKey: "reader did not drain"])
        }

        let seekPair = try makeReader(loaded, startSeconds: 5.0)
        let seekStarted = monotonicNS()
        guard seekPair.0.startReading(), seekPair.1.copyNextSampleBuffer() != nil else {
            throw seekPair.0.error ?? NSError(domain: "PulsarNativeProbe", code: 7, userInfo: [NSLocalizedDescriptionKey: "seek reader produced no PCM"])
        }
        let seekMS = milliseconds(monotonicNS(), since: seekStarted)
        seekPair.0.cancelReading()
        let cancelled = seekPair.0.status == .cancelled
        let metrics = loaded.loader.metrics.snapshot()
        let startMS = milliseconds(firstAt, since: startedAt)
        let skewMS = milliseconds(firstAt, since: scheduled)
        let startBeforeFullFile = bytesBeforeFirst < sourceSize
        let lifecycle = pausedWithoutRead && drained && cancelled && startMS <= 5_000 && seekMS <= 3_000 && skewMS <= 100

        return FixtureEvidence(
            id: spec.id,
            codec: spec.codec,
            container: spec.container,
            outcome: "decode",
            errorDomain: nil,
            errorCode: nil,
            errorDescription: nil,
            sourceBytes: sourceSize,
            bytesBeforeFirstSample: bytesBeforeFirst,
            totalSourceBytesRead: metrics.bytes,
            readOperations: metrics.operations,
            maximumReadBytes: metrics.maximumRead,
            samples: samples,
            pcmBytes: pcmBytes,
            firstTimestampSeconds: firstTimestamp,
            durationSeconds: lastEnd.seconds,
            trackStartMS: startMS,
            scheduledSkewMS: skewMS,
            pausedWithoutRead: pausedWithoutRead,
            seekGeneration: 2,
            seekToSampleMS: seekMS,
            resumed: true,
            drained: drained,
            cancelled: cancelled,
            startBeforeFullFile: startBeforeFullFile,
            passedLifecycle: lifecycle
        )
    } catch {
        let nsError = error as NSError
        let metrics = loader?.metrics.snapshot() ?? (bytes: 0, operations: 0, maximumRead: 0)
        return FixtureEvidence(
            id: spec.id,
            codec: spec.codec,
            container: spec.container,
            outcome: "reject",
            errorDomain: nsError.domain,
            errorCode: nsError.code,
            errorDescription: nsError.localizedDescription,
            sourceBytes: sourceSize,
            bytesBeforeFirstSample: metrics.bytes,
            totalSourceBytesRead: metrics.bytes,
            readOperations: metrics.operations,
            maximumReadBytes: metrics.maximumRead,
            samples: 0,
            pcmBytes: 0,
            firstTimestampSeconds: nil,
            durationSeconds: nil,
            trackStartMS: nil,
            scheduledSkewMS: nil,
            pausedWithoutRead: false,
            seekGeneration: 0,
            seekToSampleMS: nil,
            resumed: false,
            drained: false,
            cancelled: false,
            startBeforeFullFile: false,
            passedLifecycle: false
        )
    }
}

private func entitlement(_ name: String) -> Bool {
    guard let task = SecTaskCreateFromSelf(nil),
          let value = SecTaskCopyValueForEntitlement(task, name as CFString, nil) else {
        return false
    }
    return (value as? Bool) == true
}

private func peakRSSBytes() -> Int64 {
    var usage = rusage()
    guard getrusage(RUSAGE_SELF, &usage) == 0 else { return 0 }
    return Int64(usage.ru_maxrss)
}

private func architecture() -> String {
    var value = utsname()
    uname(&value)
    return withUnsafePointer(to: &value.machine) {
        $0.withMemoryRebound(to: CChar.self, capacity: 1) { String(cString: $0) }
    }
}

private func run() throws {
    guard let resources = Bundle.main.resourceURL else {
        throw NSError(domain: "PulsarNativeProbe", code: 8, userInfo: [NSLocalizedDescriptionKey: "bundle resources unavailable"])
    }
    let contractURL = resources.appendingPathComponent("macos-native-probe-v1.json")
    let contract = try JSONDecoder().decode(ProbeContract.self, from: Data(contentsOf: contractURL))
    guard contract.contract == contractName else {
        throw NSError(domain: "PulsarNativeProbe", code: 9, userInfo: [NSLocalizedDescriptionKey: "contract mismatch"])
    }

    let rssStart = peakRSSBytes()
    let fixtures = contract.fixtures.map { decodeFixture($0, resources: resources) }
    let allDecoded = fixtures.allSatisfy { $0.outcome == "decode" }
    let allLifecycle = fixtures.filter { $0.outcome == "decode" }.allSatisfy { $0.passedLifecycle }
    let noFullFilePrepare = fixtures.filter { $0.outcome == "decode" }.allSatisfy { $0.startBeforeFullFile }
    let sandboxed = entitlement("com.apple.security.app-sandbox")
    let networkClient = entitlement("com.apple.security.network.client")
    let passed = sandboxed && !networkClient && allDecoded && allLifecycle && noFullFilePrepare
    let decision: String
    if !allDecoded {
        decision = "rejected-unsupported-native-format"
    } else if !noFullFilePrepare && !allLifecycle {
        decision = "rejected-full-file-and-lifecycle-gates"
    } else if !noFullFilePrepare {
        decision = "rejected-full-file-before-first-sample"
    } else if !allLifecycle {
        decision = "rejected-lifecycle-or-timing-gate"
    } else if !sandboxed || networkClient {
        decision = "rejected-sandbox-contract"
    } else {
        decision = "engineering-candidate-only-manual-matrix-required"
    }

    let evidence = ProbeEvidence(
        schemaVersion: 1,
        contract: contractName,
        candidateId: contract.candidateId,
        claimClass: "repository-engineering-prototype",
        osVersion: ProcessInfo.processInfo.operatingSystemVersionString,
        architecture: architecture(),
        bundleIdentifier: Bundle.main.bundleIdentifier ?? "unknown",
        sandboxEntitlement: sandboxed,
        networkClientEntitlement: networkClient,
        renderCallbackUsed: false,
        decoderOwnsNetwork: false,
        maximumUnderlyingReadBytes: maximumReadBytes,
        rssStartBytes: rssStart,
        peakRSSBytes: peakRSSBytes(),
        fixtures: fixtures,
        shippingDecision: decision,
        passed: passed
    )
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes]
    FileHandle.standardOutput.write(try encoder.encode(evidence))
    FileHandle.standardOutput.write(Data("\n".utf8))
}

do {
    try run()
} catch {
    FileHandle.standardError.write(Data("macOS native codec probe failed: \(error)\n".utf8))
    exit(1)
}
