// Production audio graph (spec 6.3):
//   FIFO -> reader thread (blocking, backpressure, no drops) -> SPSC ring
//   -> AVAudioSourceNode "music" (with fade gain) \
//                                                  -> mainMixer -> output
//   AVAudioPlayerNode "inserts" (voice WAVs, clicks) /
//
// Render callback copies from the ring only: no locks, no allocation, no I/O.
// Underrun = silence + counter. Volume: amplitude = (v/100)^2 on mainMixer.

import AVFAudio
import Foundation

public final class AudioEngine {
    public let sampleRate: Double = 44100
    public let channels = 2

    private let engine = AVAudioEngine()
    private let ring: RingBuffer
    private let insertPlayer = AVAudioPlayerNode()
    private var srcNode: AVAudioSourceNode!
    private let fifoPath: String
    private let log: Logger

    // Reader thread state.
    private var readerThread: Thread?
    private let readerActive = UnsafeMutablePointer<Bool>.allocate(capacity: 1)
    // Producer parks here instead of reading the pipe when the ring is full
    // (backpressure; spec 6.3: dropping is forbidden).
    private let backpressureSleepUS: UInt32 = 3000

    // Music branch fade gain (pause fade_ms / playnow 300 ms, spec 7.3).
    // Raised-cosine (S-curve) ramp: zero slope at both ends, so fades into
    // silence land softly instead of perceptually "cutting off".
    private var gainCurrent: Float = 1
    private var gainStart: Float = 1
    private var gainTarget: Float = 1
    private var gainRampTotal: Int = 0 // frames; 0 = snap
    private var gainRampDone: Int = 0

    // Underruns (spec 6.3/6.5) — read by heartbeat.
    public private(set) var underrunCallbacks: Int64 = 0
    /// Callbacks fully fed from the ring — glitch detector: fed and starved
    /// callbacks inside the same second = audible dropout, not idle silence.
    public private(set) var fedCallbacks: Int64 = 0
    /// Consecutive silence seconds estimate for audio_starvation (spec 6.6);
    /// only meaningful while `expectingMusic` is true (UNRESOLVED R4 gate).
    public var expectingMusic = false
    public private(set) var starvedCallbacksStreak: Int64 = 0

    // First-nonzero-sample detection for `started` (spec 6.3 item 4).
    private var armFirstSample = false
    public var onFirstMusicSample: ((_ hostTimeNow: UInt64) -> Void)?

    public init(fifoPath: String, ringMs: Int, log: Logger) {
        self.fifoPath = fifoPath
        self.log = log
        ring = RingBuffer(capacityFloats: Int(sampleRate) * channels * ringMs / 1000)
        readerActive.initialize(to: false)

        let fmt = AVAudioFormat(standardFormatWithSampleRate: sampleRate,
                                channels: AVAudioChannelCount(channels))!
        var scratch = [Float](repeating: 0, count: 8192 * channels)

        srcNode = AVAudioSourceNode(format: fmt) { [weak self] _, _, frameCount, abl -> OSStatus in
            guard let self else { return noErr }
            let buffers = UnsafeMutableAudioBufferListPointer(abl)
            let frames = Int(frameCount)
            let need = frames * self.channels

            let got = scratch.withUnsafeMutableBufferPointer {
                self.ring.read(into: $0.baseAddress!, count: need)
            }
            if got < need {
                self.underrunCallbacks += 1
                if self.expectingMusic { self.starvedCallbacksStreak += 1 }
                for i in got..<need { scratch[i] = 0 }
            } else {
                self.fedCallbacks += 1
                self.starvedCallbacksStreak = 0
            }

            if self.armFirstSample, got > 0 {
                var nonZero = false
                for i in 0..<got where scratch[i] != 0 { nonZero = true; break }
                if nonZero {
                    self.armFirstSample = false
                    self.onFirstMusicSample?(mach_absolute_time())
                }
            }

            // Music fade: raised-cosine ramp gainStart -> gainTarget.
            let target = self.gainTarget
            let start = self.gainStart
            let total = self.gainRampTotal
            var done = self.gainRampDone
            var g = self.gainCurrent
            for ch in 0..<min(self.channels, buffers.count) {
                guard let mData = buffers[ch].mData else { continue }
                let out = mData.assumingMemoryBound(to: Float.self)
                done = self.gainRampDone
                for f in 0..<frames {
                    if total > 0 && done < total {
                        let t = Float(done) / Float(total)
                        let s = 0.5 * (1 - cosf(.pi * t))
                        g = start + (target - start) * s
                        done += 1
                    } else {
                        g = target
                    }
                    out[f] = scratch[f * self.channels + ch] * g
                }
                buffers[ch].mDataByteSize = UInt32(frames * MemoryLayout<Float>.size)
            }
            self.gainRampDone = done
            if total > 0 && done >= total {
                self.gainRampTotal = 0
            }
            self.gainCurrent = g
            return noErr
        }

        engine.attach(srcNode)
        engine.attach(insertPlayer)
        engine.connect(srcNode, to: engine.mainMixerNode, format: fmt)
        engine.connect(insertPlayer, to: engine.mainMixerNode, format: nil)
    }

    deinit { readerActive.deallocate() }

    public func start() throws {
        try engine.start()
        insertPlayer.play()
        startReader()
        log.info("audio engine started", ["output_rate": engine.outputNode.outputFormat(forBus: 0).sampleRate])

        // Default-output device changes (user picks an AirPlay output, sample
        // rate switches) stop the engine; restart it on the new device.
        NotificationCenter.default.addObserver(
            forName: .AVAudioEngineConfigurationChange, object: engine, queue: nil
        ) { [weak self] _ in
            guard let self else { return }
            do {
                try self.engine.start()
                self.insertPlayer.play()
                self.log.info("audio engine restarted after output change",
                              ["output_rate": self.engine.outputNode.outputFormat(forBus: 0).sampleRate])
            } catch {
                self.log.error("engine restart after config change failed", ["err": "\(error)"])
            }
        }
    }

    public func stopEngine() {
        readerActive.pointee = false
        engine.stop()
    }

    // MARK: Volume / fade

    private var targetAmplitude: Float = 0.64 // (80/100)^2 default
    private var volumeTimer: DispatchSourceTimer?
    private let volumeQueue = DispatchQueue(label: "duet.volume-ramp")

    /// Master volume 0..100, amplitude = (v/100)^2 (spec 6.3), default 80.
    /// Changes glide exponentially (~200 ms) instead of stepping — Spotify
    /// Connect sends the phone slider as discrete jumps.
    public func setVolume(_ v: Int) {
        let clamped = Float(min(max(v, 0), 100)) / 100
        volumeQueue.async {
            self.targetAmplitude = clamped * clamped
            self.startVolumeRampLocked()
        }
    }

    /// Test hook: the mixer's current amplitude.
    public var currentAmplitude: Float { engine.mainMixerNode.outputVolume }

    private func startVolumeRampLocked() {
        guard volumeTimer == nil else { return }
        let t = DispatchSource.makeTimerSource(queue: volumeQueue)
        t.schedule(deadline: .now(), repeating: .milliseconds(16))
        t.setEventHandler { [weak self] in
            guard let self else { return }
            let current = self.engine.mainMixerNode.outputVolume
            let target = self.targetAmplitude
            let next = current + (target - current) * 0.18
            if abs(next - target) < 0.001 {
                self.engine.mainMixerNode.outputVolume = target
                self.volumeTimer?.cancel()
                self.volumeTimer = nil
            } else {
                self.engine.mainMixerNode.outputVolume = next
            }
        }
        t.resume()
        volumeTimer = t
    }

    /// Fades the music branch to `target` over `fadeMs` (0 = instant) along
    /// a raised-cosine curve (soft landing into silence).
    public func setMusicGain(_ target: Float, fadeMs: Int64) {
        if fadeMs <= 0 {
            gainCurrent = target
            gainTarget = target
            gainRampTotal = 0
            return
        }
        gainStart = gainCurrent
        gainTarget = target
        gainRampDone = 0
        gainRampTotal = max(1, Int(Float(sampleRate) * Float(fadeMs) / 1000))
    }

    // MARK: Ring accessors (PlayerCore uses these for audible_position/ended)

    public var ringFillMs: Int64 {
        Int64(Double(ring.fill / channels) / sampleRate * 1000)
    }

    /// Drops buffered audio before loading a new element (spec 6.3 load).
    public func clearRing() { ring.clear() }

    /// Arms first-nonzero-sample detection for the next start (spec 6.3).
    public func armStartDetection() { armFirstSample = true }

    // MARK: Voice inserts / clicks (AVAudioPlayerNode branch)

    /// Plays a WAV file; completion fires when playback finished (voice_ended).
    public func playInsert(fileURL: URL, at when: AVAudioTime? = nil, completion: @escaping () -> Void) throws {
        let file = try AVAudioFile(forReading: fileURL)
        insertPlayer.scheduleFile(file, at: when, completionCallbackType: .dataPlayedBack) { _ in
            completion()
        }
        if !insertPlayer.isPlaying { insertPlayer.play() }
    }

    /// offset_test clicks: `count` clicks starting at hostTime, every intervalMs.
    public func playClicks(count: Int, firstAtHostTime: UInt64, intervalMs: Int64) {
        let clickFrames = Int(sampleRate * 0.006) // 6 ms burst
        let fmt = insertPlayer.outputFormat(forBus: 0)
        guard let buf = AVAudioPCMBuffer(pcmFormat: fmt, frameCapacity: AVAudioFrameCount(clickFrames)) else { return }
        buf.frameLength = AVAudioFrameCount(clickFrames)
        for ch in 0..<Int(fmt.channelCount) {
            guard let data = buf.floatChannelData?[ch] else { continue }
            for i in 0..<clickFrames {
                // 1 kHz burst with a fast decay: sharp, easy to compare by ear.
                let t = Float(i) / Float(sampleRate)
                data[i] = sinf(2 * .pi * 1000 * t) * expf(-t * 600)
            }
        }
        for k in 0..<count {
            let hostTime = HostClock.addMs(firstAtHostTime, ms: intervalMs * Int64(k))
            insertPlayer.scheduleBuffer(buf, at: AVAudioTime(hostTime: hostTime), options: [])
        }
        if !insertPlayer.isPlaying { insertPlayer.play() }
    }

    // MARK: FIFO reader (spec 6.3: blocking open is the idle state; EOF -> reopen)

    private func startReader() {
        readerActive.pointee = true
        let thread = Thread { [weak self] in
            self?.readerLoop()
        }
        thread.name = "duet.fifo-reader"
        thread.qualityOfService = .userInteractive
        thread.start()
        readerThread = thread
    }

    private func readerLoop() {
        let chunkBytes = 16384
        var byteBuf = [UInt8](repeating: 0, count: chunkBytes)
        while readerActive.pointee {
            let fd = open(fifoPath, O_RDONLY) // blocks until a writer appears
            if fd < 0 {
                log.warn("fifo open failed", ["errno": errno])
                Thread.sleep(forTimeInterval: 0.5)
                continue
            }
            while readerActive.pointee {
                let n = byteBuf.withUnsafeMutableBytes { raw in
                    read(fd, raw.baseAddress, chunkBytes)
                }
                if n <= 0 { break } // EOF: writer closed -> reopen
                let floats = n / MemoryLayout<Float>.size
                var offset = 0
                byteBuf.withUnsafeBytes { raw in
                    let base = raw.baseAddress!.assumingMemoryBound(to: Float.self)
                    while offset < floats && self.readerActive.pointee {
                        let written = self.ring.write(base + offset, count: floats - offset)
                        if written == 0 {
                            // Ring full: stall here. The kernel pipe buffer fills
                            // and go-librespot blocks — natural backpressure.
                            usleep(self.backpressureSleepUS)
                        }
                        offset += written
                    }
                }
            }
            close(fd)
        }
    }
}

/// mach host time <-> wall clock helpers for scheduled starts.
public enum HostClock {
    private static let timebase: mach_timebase_info_data_t = {
        var tb = mach_timebase_info_data_t()
        mach_timebase_info(&tb)
        return tb
    }()

    public static func addMs(_ hostTime: UInt64, ms: Int64) -> UInt64 {
        let nanos = UInt64(ms) * 1_000_000
        let ticks = nanos * UInt64(timebase.denom) / UInt64(timebase.numer)
        return hostTime + ticks
    }

    /// Host time that corresponds to a wall-clock unix-ms deadline.
    public static func hostTime(forUnixMs deadline: Int64) -> UInt64 {
        let nowMs = Int64((Date().timeIntervalSince1970 * 1000).rounded())
        return addMs(mach_absolute_time(), ms: deadline - nowMs)
    }
}
