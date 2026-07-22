import Foundation
import Testing

@Test("macOS render callback keeps blocking and allocating APIs outside its boundary")
func renderCallbackSourceSafety() throws {
    let testsDirectory = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    let sourceURL = testsDirectory
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .appendingPathComponent("Sources/NodeCore/AudioEngine.swift")
    let source = try String(contentsOf: sourceURL, encoding: .utf8)
    let beginMarker = "// BEGIN RENDER CALLBACK"
    let endMarker = "// END RENDER CALLBACK"
    let begin = try #require(source.range(of: beginMarker)?.upperBound)
    let end = try #require(source.range(of: endMarker, range: begin..<source.endIndex)?.lowerBound)
    let callback = String(source[begin..<end])

    let forbidden = [
        ".async", ".sync", ".wait(", "NSLock", "DispatchSemaphore",
        ".allocate(", "AVAudioFile", "Data(contentsOf:", "URLSession",
        "FileHandle", "Thread.sleep", "usleep(", "open("
    ]
    for token in forbidden {
        #expect(!callback.contains(token), "render callback contains forbidden token \(token)")
    }
    #expect(callback.contains("self.ring.read"))
    #expect(!callback.contains("overlayPlayer"),
            "overlay state must never gate source-ring consumption")
}

@Test("macOS overlay graph keeps limiter before final local master gain")
func overlayGraphGainOrder() throws {
    let testsDirectory = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    let sourceURL = testsDirectory
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .appendingPathComponent("Sources/NodeCore/AudioEngine.swift")
    let source = try String(contentsOf: sourceURL, encoding: .utf8)
    #expect(source.contains("engine.connect(srcNode, to: programMixer"))
    #expect(source.contains("engine.connect(overlayPlayer, to: programMixer"))
    #expect(source.contains("engine.connect(programMixer, to: limiter"))
    #expect(source.contains("engine.connect(limiter, to: engine.mainMixerNode"))
    #expect(source.contains("kDynamicsProcessorParam_Threshold, value: -1.1"))
    #expect(source.contains("kDynamicsProcessorParam_HeadRoom, value: 0.1"))
    #expect(source.contains("engine.mainMixerNode.outputVolume ="))
}

@Test("macOS reader ownership is atomic and gain publication serializes all producers")
func renderControlPublicationSafety() throws {
    let testsDirectory = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    let sourceURL = testsDirectory
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .appendingPathComponent("Sources/NodeCore/AudioEngine.swift")
    let source = try String(contentsOf: sourceURL, encoding: .utf8)
    #expect(source.contains("private let readerActive = RenderAtomicInt64()"))
    #expect(!source.contains("UnsafeMutablePointer<Bool>"))
    #expect(source.contains("private let gainCommandProducerLock = NSLock()"))
    #expect(source.contains("gainCommandProducerLock.withLock"))
    #expect(source.contains("open(fifoPath, O_RDWR | O_NONBLOCK)"),
            "reader must hold a persistent writer ref (O_RDWR) so go-librespot can always attach, and stay non-blocking so shutdown does not depend on a FIFO writer connecting")

    let begin = try #require(source.range(of: "// BEGIN RENDER CALLBACK")?.upperBound)
    let end = try #require(
        source.range(of: "// END RENDER CALLBACK", range: begin..<source.endIndex)?.lowerBound)
    let callback = String(source[begin..<end])
    #expect(!callback.contains("gainCommandProducerLock"),
            "producer serialization must never enter the render callback")
}

@Test("macOS heartbeat reads player state as one queue-owned snapshot")
func playerStateSnapshotSafety() throws {
    let testsDirectory = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    let sourceURL = testsDirectory
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .appendingPathComponent("Sources/NodeCore/PlayerCore.swift")
    let source = try String(contentsOf: sourceURL, encoding: .utf8)
    let begin = try #require(source.range(of: "public func statePayload")?.lowerBound)
    let end = try #require(source.range(of: "\n    }\n}", range: begin..<source.endIndex)?.upperBound)
    let statePayload = String(source[begin..<end])
    #expect(statePayload.contains("queue.sync"))
    #expect(source.contains("private var playback = Playback.stopped"))
    #expect(source.contains("private var outputLatencyOffsetMs: Int"))
}

@Test("macOS streamed-track candidate render seam is fixed-storage only")
func streamTrackRenderSourceSafety() throws {
    let testsDirectory = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    let sourceURL = testsDirectory
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .appendingPathComponent("Sources/NodeCore/MacStreamTrackPlayer.swift")
    let source = try String(contentsOf: sourceURL, encoding: .utf8)
    let begin = try #require(source.range(of: "public func readPCM")?.lowerBound)
    let end = try #require(source.range(
        of: "\n    fileprivate var currentEpoch", range: begin..<source.endIndex)?.lowerBound)
    let render = String(source[begin..<end])
    let forbidden = [
        "queue.", ".sync", ".async", ".wait(", "NSLock", "Task {", "await ",
        "Data(", "URLSession", "FileHandle", ".allocate(", "sleep(", "usleep("
    ]
    for token in forbidden {
        #expect(!render.contains(token), "stream render seam contains forbidden token \(token)")
    }
    #expect(render.contains("ring.read"))
    #expect(render.contains("RenderAtomic") == false,
            "render uses preallocated atomic fields rather than constructing storage")
}

@Test("macOS live source callback is fixed-storage and shares the post-mix limiter")
func livePTTRenderSourceSafety() throws {
    let testsDirectory = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    let sourceURL = testsDirectory
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .appendingPathComponent("Sources/NodeCore/AudioEngine.swift")
    let source = try String(contentsOf: sourceURL, encoding: .utf8)
    let begin = try #require(
        source.range(of: "// BEGIN LIVE RENDER CALLBACK")?.upperBound)
    let end = try #require(source.range(
        of: "// END LIVE RENDER CALLBACK",
        range: begin..<source.endIndex)?.lowerBound)
    let callback = String(source[begin..<end])
    let forbidden = [
        "queue.", ".sync", ".async", ".wait(", "NSLock", "Task {", "await ",
        "Data(", "URLSession", "FileHandle", ".allocate(", "AVAudioConverter",
        "sleep(", "usleep(", "open("
    ]
    for token in forbidden {
        #expect(!callback.contains(token), "live render callback contains \(token)")
    }
    #expect(callback.contains("self.liveRing.read"))
    #expect(source.contains("engine.connect(liveNode, to: programMixer"))
    #expect(source.contains("engine.connect(programMixer, to: limiter"))
    #expect(source.contains("engine.connect(limiter, to: engine.mainMixerNode"))
    #expect(source.contains("setMusicGain(Float(pow(10, -12.0 / 20.0)), fadeMs: 60)"))
    #expect(source.contains("self.setMusicGain(1, fadeMs: 160)"))
}

@Test("macOS live jitter path is bounded and has no audio persistence client")
func livePTTJitterSourceSafety() throws {
    let testsDirectory = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    let sourceURL = testsDirectory
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .appendingPathComponent("Sources/NodeCore/MacLiveJitterReceiver.swift")
    let source = try String(contentsOf: sourceURL, encoding: .utf8)
    #expect(source.contains(
        "private static let packetWindow = Int(LivePTTConstants.maxGapFrames) + 1"))
    #expect(source.contains("private static let frameSamples = 960"))
    #expect(source.contains("private static let maxConsecutiveConcealments = 8"))
    #expect(source.contains("UnsafeMutablePointer<Float>.allocate"))
    for token in [
        "URLSession", "FileHandle", "FileManager", "UserDefaults", ".write(to:",
        "media_items", "transmissions"
    ] {
        #expect(!source.contains(token), "live jitter path contains persistence token \(token)")
    }
}

@Test("macOS live capture callback is bounded and has no transport or persistence work")
func livePTTCaptureSourceSafety() throws {
    let testsDirectory = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    let sourceURL = testsDirectory
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .appendingPathComponent("Sources/NodeCore/MacLiveCaptureSender.swift")
    let source = try String(contentsOf: sourceURL, encoding: .utf8)
    let begin = try #require(
        source.range(of: "// BEGIN LIVE CAPTURE CALLBACK")?.upperBound)
    let end = try #require(source.range(
        of: "// END LIVE CAPTURE CALLBACK", range: begin..<source.endIndex)?.lowerBound)
    let callback = String(source[begin..<end])
    for token in [
        "encoder.encode", "trySendFrame", "sendControl", "URLSession", "FileHandle",
        "FileManager", "Data(", ".write(to:", ".sync", ".wait(", "lock.lock"
    ] {
        #expect(!callback.contains(token), "live capture callback contains \(token)")
    }
    #expect(callback.contains("mailbox.offer(samples)"))
    #expect(source.contains("MacLiveSampleMailbox(capacity: 3_840)"))
    #expect(source.contains("private static let sendQueueLimit = 8"))
    #expect(source.contains("private static let frameSamples = 960"))
    for token in [
        "URLSession", "FileHandle", "FileManager", "UserDefaults", ".write(to:",
        "media_items", "transmissions"
    ] {
        #expect(!source.contains(token), "live capture path contains persistence token \(token)")
    }
}

@Test("macOS AV capture tap only uses fixed storage and nonblocking signals")
func macAVCaptureTapSafety() throws {
    let testsDirectory = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    let sourceURL = testsDirectory
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .appendingPathComponent("Sources/NodeCore/MacAVAudioCaptureBackend.swift")
    let source = try String(contentsOf: sourceURL, encoding: .utf8)
    let begin = try #require(
        source.range(of: "// BEGIN MAC AV CAPTURE CALLBACK")?.upperBound)
    let end = try #require(source.range(
        of: "// END MAC AV CAPTURE CALLBACK", range: begin..<source.endIndex)?.lowerBound)
    let callback = String(source[begin..<end])
    #expect(callback.contains("mailbox.offer(buffer)"))
    #expect(callback.contains("drainSignal.add"))
    for token in [
        "Array(", "[Float](", "Data(", "DispatchQueue", ".async", ".sync",
        ".wait(", "lock.lock", "onSamples", "resample", "process(", "FileHandle",
        "URLSession", ".write(to:",
    ] {
        #expect(!callback.contains(token), "capture tap contains forbidden token \(token)")
    }
    #expect(source.contains("UnsafeMutablePointer<Float>"))
    #expect(source.contains("guard lock.try()"))
}
