import Foundation
import NodeCore

/// Accountless capture owns no output graph until the recording workflow asks
/// to play its start cue. A missing output therefore becomes a cue failure
/// after input-only capture has started, rather than disabling Record.
final class LocalCaptureAudioRuntime: MacCaptureProgramDucking {
    let log: Logger
    private let config: NodeConfig
    private let lock = NSLock()
    private var engine: AudioEngine?
    private var captureDuckingActive = false
    lazy var output: MacLocalClipPlaying = MacDeferredLocalClipOutput { [weak self] in
        guard let self else { throw MacLocalClipOutputError.playbackFailed }
        return try self.makeOutput()
    }

    init(config: NodeConfig) {
        self.config = config
        log = Logger(level: Logger.Level(name: config.log.level), path: config.log.path)
    }

    func setCaptureDucking(active: Bool) {
        let engine = lock.withLock {
            captureDuckingActive = active
            return self.engine
        }
        engine?.setMusicGain(
            active ? Float(pow(10, -12.0 / 20.0)) : Float(1),
            fadeMs: active ? 100 : 160)
    }

    func stop() {
        let engine = lock.withLock {
            let current = self.engine
            self.engine = nil
            return current
        }
        engine?.stopEngine()
    }

    private func makeOutput() throws -> MacLocalClipPlaying {
        let engine = AudioEngine(
            fifoPath: config.audio.fifoPath,
            ringMs: config.audio.ringBufferMs,
            log: log)
        do {
            try engine.start()
        } catch {
            engine.stopEngine()
            throw error
        }
        let shouldDuck = lock.withLock {
            self.engine = engine
            return captureDuckingActive
        }
        if shouldDuck {
            engine.setMusicGain(Float(pow(10, -12.0 / 20.0)), fadeMs: 0)
        }

        return MacProductionLocalClipOutput(audio: engine, log: log)
    }
}
