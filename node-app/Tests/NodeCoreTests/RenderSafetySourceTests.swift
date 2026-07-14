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
