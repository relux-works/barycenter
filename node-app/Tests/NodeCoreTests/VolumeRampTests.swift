import Foundation
import Testing
@testable import NodeCore

@Suite struct VolumeRampTests {
    @Test func volumeGlidesToSquaredTarget() async throws {
        let dir = FileManager.default.temporaryDirectory
            .appendingPathComponent("duet-vol-test-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let fifo = dir.appendingPathComponent("f.fifo").path
        mkfifo(fifo, 0o600)
        let engine = AudioEngine(fifoPath: fifo, ringMs: 200, log: Logger(level: .error, path: nil))
        // Engine not started: the ramp drives mixer volume regardless.

        engine.setVolume(100)
        try await Task.sleep(nanoseconds: 500_000_000)
        #expect(abs(engine.currentAmplitude - 1.0) < 0.01, "got \(engine.currentAmplitude)")

        engine.setVolume(50)
        try await Task.sleep(nanoseconds: 80_000_000)
        let mid = engine.currentAmplitude
        #expect(mid < 0.99 && mid > 0.25, "ramp should be in flight, got \(mid)")

        try await Task.sleep(nanoseconds: 500_000_000)
        #expect(abs(engine.currentAmplitude - 0.25) < 0.01, "amplitude = (50/100)^2, got \(engine.currentAmplitude)")
    }
}
