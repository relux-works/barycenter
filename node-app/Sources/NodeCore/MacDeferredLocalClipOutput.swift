import Foundation

/// Defers construction of the output graph until a cue is actually requested.
/// A failed output startup is a cue failure; it never makes microphone capture
/// unavailable before the user presses Record.
public final class MacDeferredLocalClipOutput: MacLocalClipPlaying, @unchecked Sendable {
    private let makeOutput: () throws -> MacLocalClipPlaying
    private let lock = NSLock()
    private var output: MacLocalClipPlaying?
    private var creating = false
    private var generation: UInt64 = 0

    public init(makeOutput: @escaping () throws -> MacLocalClipPlaying) {
        self.makeOutput = makeOutput
    }

    public func play(
        fileURL: URL,
        completion: @escaping @Sendable (Result<Void, MacLocalClipOutputError>) -> Void
    ) {
        let reservation: (output: MacLocalClipPlaying?, generation: UInt64)? = lock.withLock {
            if let output {
                return (output, generation)
            }
            guard !creating else { return nil }
            creating = true
            return (nil, generation)
        }
        guard let reservation else {
            completion(.failure(.busy))
            return
        }
        if let output = reservation.output {
            output.play(fileURL: fileURL, completion: completion)
            return
        }

        do {
            let output = try makeOutput()
            let shouldPlay = lock.withLock {
                self.output = output
                creating = false
                return generation == reservation.generation
            }
            guard shouldPlay else {
                output.cancel()
                completion(.failure(.playbackFailed))
                return
            }
            output.play(fileURL: fileURL, completion: completion)
        } catch {
            lock.withLock { creating = false }
            completion(.failure(.playbackFailed))
        }
    }

    public func cancel() {
        let output = lock.withLock {
            generation &+= 1
            return self.output
        }
        output?.cancel()
    }
}
