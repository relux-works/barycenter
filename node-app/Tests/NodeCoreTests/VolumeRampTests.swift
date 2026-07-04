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

        // CI runners tick DispatchSourceTimer slower than 16ms — poll until
        // convergence with a deadline instead of trusting wall-clock sleeps.
        func settle(to target: Float, within seconds: Double) async throws -> Bool {
            let deadline = Date().addingTimeInterval(seconds)
            while Date() < deadline {
                if abs(engine.currentAmplitude - target) < 0.01 { return true }
                try await Task.sleep(nanoseconds: 50_000_000)
            }
            return abs(engine.currentAmplitude - target) < 0.01
        }

        engine.setVolume(100)
        #expect(try await settle(to: 1.0, within: 5), "got \(engine.currentAmplitude)")

        engine.setVolume(50)
        try await Task.sleep(nanoseconds: 80_000_000)
        let mid = engine.currentAmplitude
        #expect(mid > 0.2, "ramp should have left zero, got \(mid)")

        #expect(try await settle(to: 0.25, within: 5), "amplitude = (50/100)^2, got \(engine.currentAmplitude)")
    }
}
