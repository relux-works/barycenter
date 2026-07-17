// Production audio graph (spec 6.3):
//   FIFO -> interruptible reader thread (backpressure, no drops) -> SPSC ring
//   -> AVAudioSourceNode "music" (with fade/duck gain) \
//   48 kHz mono live ring -> AVAudioSourceNode "live"   \
//   AVAudioPlayerNode "overlay" + legacy inserts       -> program mixer
//   -> post-mix limiter -> final mainMixer volume -> output
//
// Render callback copies from the ring only: no locks, no allocation, no I/O.
// Underrun = silence + counter. Volume: amplitude = (v/100)^2 on mainMixer.

import AVFAudio
import AudioToolbox
import Foundation

public final class AudioEngine {
    public let sampleRate: Double = 44100
    public let channels = 2

    private let engine = AVAudioEngine()
    private let ring: RingBuffer
    private let liveRing = RingBuffer(capacityFloats: 48_000 * 320 / 1_000)
    private let insertPlayer = AVAudioPlayerNode()
    private let overlayPlayer = AVAudioPlayerNode()
    private let programMixer = AVAudioMixerNode()
    private let limiter = AVAudioUnitEffect(audioComponentDescription: AudioComponentDescription(
        componentType: kAudioUnitType_Effect,
        componentSubType: kAudioUnitSubType_DynamicsProcessor,
        componentManufacturer: kAudioUnitManufacturer_Apple,
        componentFlags: 0,
        componentFlagsMask: 0))
    private var srcNode: AVAudioSourceNode!
    private var liveNode: AVAudioSourceNode!
    private let fifoPath: String
    private let log: Logger

    // Reader thread state.
    private var readerThread: Thread?
    private let readerActive = RenderAtomicInt64()
    // Producer parks here instead of reading the pipe when the ring is full
    // (backpressure; spec 6.3: dropping is forbidden).
    private let backpressureSleepUS: UInt32 = 3000

    // Music branch fade gain (pause fade_ms / playnow 300 ms, spec 7.3).
    // Raised-cosine (S-curve) ramp: zero slope at both ends, so fades into
    // silence land softly instead of perceptually "cutting off". Control
    // enqueues fixed-size commands; only render mutates the ramp itself.
    private struct MusicGainCommand {
        var target: Float
        var rampFrames: Int64
    }
    private let gainCommandCapacity = 64
    private let gainCommands: UnsafeMutablePointer<MusicGainCommand>
    private let gainCommandHead = RenderAtomicInt64()
    private let gainCommandTail = RenderAtomicInt64()
    // Multiple control queues can publish music fades (player commands,
    // overlay/interrupt and route recovery). Serialize only those producers;
    // the render consumer remains lock-free.
    private let gainCommandProducerLock = NSLock()
    private var gainCurrent: Float = 1
    private var gainStart: Float = 1
    private var gainTarget: Float = 1
    private var gainRampTotal: Int = 0 // frames; 0 = snap
    private var gainRampDone: Int = 0

    // Live PTT uses its own fixed SPSC ring and render-owned gain ramp. The
    // decoder is the only producer; this source callback is the only consumer.
    private let liveActive = RenderAtomicInt64()
    private let liveRouteGeneration = RenderAtomicInt64()
    private let liveGainCommandGeneration = RenderAtomicInt64()
    private let liveGainCommandProducerLock = NSLock()
    private let liveGainTargetBits = RenderAtomicInt64(Int64(Float(0).bitPattern))
    private let liveGainRampFrames = RenderAtomicInt64()
    private var liveGainSeenGeneration: Int64 = 0
    private var liveGainCurrent: Float = 0
    private var liveGainStart: Float = 0
    private var liveGainTarget: Float = 0
    private var liveGainRampTotal: Int = 0
    private var liveGainRampDone: Int = 0
    private let liveUnderrunCounter = RenderAtomicInt64()
    private let liveRenderedFrameCounter = RenderAtomicInt64()
    private let liveControlQueue = DispatchQueue(label: "duet.live-audio-control")

    // Underruns (spec 6.3/6.5) — read by heartbeat.
    private let underrunCounter = RenderAtomicInt64()
    public var underrunCallbacks: Int64 { underrunCounter.load() }
    /// Callbacks fully fed from the ring — glitch detector: fed and starved
    /// callbacks inside the same second = audible dropout, not idle silence.
    private let fedCounter = RenderAtomicInt64()
    public var fedCallbacks: Int64 { fedCounter.load() }
    /// Consecutive silence seconds estimate for audio_starvation (spec 6.6);
    /// only meaningful while `expectingMusic` is true (UNRESOLVED R4 gate).
    private let expectingMusicState = RenderAtomicInt64()
    public var expectingMusic: Bool {
        get { expectingMusicState.load() != 0 }
        set {
            expectingMusicState.store(newValue ? 1 : 0)
            if !newValue { starvedCounter.store(0) }
        }
    }
    private let starvedCounter = RenderAtomicInt64()
    public var starvedCallbacksStreak: Int64 { starvedCounter.load() }

    // First-nonzero-sample detection for `started` (spec 6.3 item 4).
    private let armFirstSampleState = RenderAtomicInt64()
    private let firstSampleHostTime = RenderAtomicInt64()
    private let firstSampleQueue = DispatchQueue(label: "duet.first-sample-dispatch")
    private var firstSampleTimer: DispatchSourceTimer?
    public var onFirstMusicSample: ((_ hostTimeNow: UInt64) -> Void)?

    public init(fifoPath: String, ringMs: Int, log: Logger) {
        self.fifoPath = fifoPath
        self.log = log
        ring = RingBuffer(capacityFloats: Int(sampleRate) * channels * ringMs / 1000)
        gainCommands = .allocate(capacity: gainCommandCapacity)
        gainCommands.initialize(
            repeating: MusicGainCommand(target: 1, rampFrames: 0),
            count: gainCommandCapacity)

        let fmt = AVAudioFormat(standardFormatWithSampleRate: sampleRate,
                                channels: AVAudioChannelCount(channels))!
        var scratch = [Float](repeating: 0, count: 8192 * channels)

        // BEGIN RENDER CALLBACK (checked by RenderSafetySourceTests)
        srcNode = AVAudioSourceNode(format: fmt) { [weak self] _, _, frameCount, abl -> OSStatus in
            guard let self else { return noErr }
            let buffers = UnsafeMutableAudioBufferListPointer(abl)
            let frames = Int(frameCount)
            let need = frames * self.channels

            let got = scratch.withUnsafeMutableBufferPointer {
                self.ring.read(into: $0.baseAddress!, count: need)
            }
            if got < need {
                self.underrunCounter.add(1)
                if self.expectingMusicState.load() != 0 {
                    self.starvedCounter.add(1)
                } else {
                    self.starvedCounter.store(0)
                }
                for i in got..<need { scratch[i] = 0 }
            } else {
                self.fedCounter.add(1)
                self.starvedCounter.store(0)
            }

            if self.armFirstSampleState.load() != 0, got > 0 {
                var nonZero = false
                for i in 0..<got where scratch[i] != 0 { nonZero = true; break }
                if nonZero {
                    var armed: Int64 = 1
                    if self.armFirstSampleState.compareExchange(expected: &armed, desired: 0) {
                        self.firstSampleHostTime.store(Int64(bitPattern: mach_absolute_time()))
                    }
                }
            }

            // Consume every command published before this callback. Storage is
            // fixed at init; there is no allocation, lock, I/O, or wait here.
            var commandTail = self.gainCommandTail.load()
            let commandHead = self.gainCommandHead.load()
            while commandTail < commandHead {
                let command = self.gainCommands[Int(commandTail % Int64(self.gainCommandCapacity))]
                self.gainStart = self.gainCurrent
                self.gainTarget = command.target
                self.gainRampDone = 0
                self.gainRampTotal = Int(command.rampFrames)
                if command.rampFrames == 0 { self.gainCurrent = command.target }
                commandTail += 1
            }
            self.gainCommandTail.store(commandTail)

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
        // END RENDER CALLBACK

        let liveFormat = AVAudioFormat(
            standardFormatWithSampleRate: 48_000, channels: 1)!
        var liveScratch = [Float](repeating: 0, count: 8_192)
        // BEGIN LIVE RENDER CALLBACK (checked by RenderSafetySourceTests)
        liveNode = AVAudioSourceNode(format: liveFormat) {
            [weak self] _, _, frameCount, abl -> OSStatus in
            guard let self else { return noErr }
            let buffers = UnsafeMutableAudioBufferListPointer(abl)
            let need = Int(frameCount)
            guard need <= liveScratch.count else { return kAudio_ParamError }

            let commandGeneration = self.liveGainCommandGeneration.load()
            if commandGeneration != self.liveGainSeenGeneration {
                self.liveGainSeenGeneration = commandGeneration
                self.liveGainStart = self.liveGainCurrent
                self.liveGainTarget = Float(bitPattern: UInt32(
                    truncatingIfNeeded: self.liveGainTargetBits.load()))
                self.liveGainRampTotal = Int(self.liveGainRampFrames.load())
                self.liveGainRampDone = 0
                if self.liveGainRampTotal == 0 {
                    self.liveGainCurrent = self.liveGainTarget
                }
            }

            let got: Int
            if self.liveActive.load() != 0 {
                got = liveScratch.withUnsafeMutableBufferPointer {
                    self.liveRing.read(into: $0.baseAddress!, count: need)
                }
                if got > 0 { self.liveRenderedFrameCounter.add(Int64(got)) }
                if got < need { self.liveUnderrunCounter.add(1) }
            } else {
                _ = liveScratch.withUnsafeMutableBufferPointer {
                    self.liveRing.read(into: $0.baseAddress!, count: 0)
                }
                got = 0
            }
            if got < need {
                for index in got..<need { liveScratch[index] = 0 }
            }

            var done = self.liveGainRampDone
            var gain = self.liveGainCurrent
            for frame in 0..<need {
                if self.liveGainRampTotal > 0 && done < self.liveGainRampTotal {
                    let position = Float(done) / Float(self.liveGainRampTotal)
                    let curve = 0.5 * (1 - cosf(.pi * position))
                    gain = self.liveGainStart
                        + (self.liveGainTarget - self.liveGainStart) * curve
                    done += 1
                } else {
                    gain = self.liveGainTarget
                }
                liveScratch[frame] *= gain
            }
            self.liveGainRampDone = done
            if self.liveGainRampTotal > 0 && done >= self.liveGainRampTotal {
                self.liveGainRampTotal = 0
            }
            self.liveGainCurrent = gain
            for index in buffers.indices {
                guard let data = buffers[index].mData else { continue }
                liveScratch.withUnsafeBufferPointer { scratch in
                    data.assumingMemoryBound(to: Float.self).update(
                        from: scratch.baseAddress!, count: need)
                }
                buffers[index].mDataByteSize = UInt32(
                    need * MemoryLayout<Float>.size)
            }
            return noErr
        }
        // END LIVE RENDER CALLBACK

        engine.attach(srcNode)
        engine.attach(liveNode)
        engine.attach(insertPlayer)
        engine.attach(overlayPlayer)
        engine.attach(programMixer)
        engine.attach(limiter)
        // DynamicsProcessor guarantees threshold + headroom. Its minimum
        // headroom is 0.1 dB, so -1.1 + 0.1 freezes the local -1 dBFS ceiling.
        setLimiterParameter(kDynamicsProcessorParam_Threshold, value: -1.1)
        setLimiterParameter(kDynamicsProcessorParam_HeadRoom, value: 0.1)
        setLimiterParameter(kDynamicsProcessorParam_AttackTime, value: 0.001)
        setLimiterParameter(kDynamicsProcessorParam_ReleaseTime, value: 0.05)
        setLimiterParameter(kDynamicsProcessorParam_OverallGain, value: 0)
        engine.connect(srcNode, to: programMixer, format: fmt)
        engine.connect(liveNode, to: programMixer, format: liveFormat)
        engine.connect(insertPlayer, to: programMixer, format: nil)
        engine.connect(overlayPlayer, to: programMixer, format: nil)
        engine.connect(programMixer, to: limiter, format: nil)
        engine.connect(limiter, to: engine.mainMixerNode, format: nil)
        startFirstSampleDispatcher()
    }

    deinit {
        firstSampleTimer?.cancel()
        gainCommands.deinitialize(count: gainCommandCapacity)
        gainCommands.deallocate()
    }

    private func startFirstSampleDispatcher() {
        let timer = DispatchSource.makeTimerSource(queue: firstSampleQueue)
        timer.schedule(deadline: .now(), repeating: .milliseconds(2))
        timer.setEventHandler { [weak self] in
            guard let self else { return }
            let hostTime = self.firstSampleHostTime.load()
            guard hostTime != 0 else { return }
            var expected = hostTime
            guard self.firstSampleHostTime.compareExchange(expected: &expected, desired: 0) else { return }
            self.onFirstMusicSample?(UInt64(bitPattern: hostTime))
        }
        timer.resume()
        firstSampleTimer = timer
    }

    public func start() throws {
        try engine.start()
        insertPlayer.play()
        overlayPlayer.play()
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
                self.overlayPlayer.play()
                self.log.info("audio engine restarted after output change",
                              ["output_rate": self.engine.outputNode.outputFormat(forBus: 0).sampleRate])
            } catch {
                self.log.error("engine restart after config change failed", ["err": "\(error)"])
            }
        }
    }

    public func stopEngine() {
        readerActive.store(0)
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
        let frames = fadeMs <= 0
            ? 0
            : max(1, Int64(Float(sampleRate) * Float(fadeMs) / 1000))
        let published = gainCommandProducerLock.withLock { () -> Bool in
            let head = gainCommandHead.load()
            let tail = gainCommandTail.load()
            guard head - tail < Int64(gainCommandCapacity) else { return false }
            gainCommands[Int(head % Int64(gainCommandCapacity))] =
                MusicGainCommand(target: target, rampFrames: frames)
            gainCommandHead.store(head + 1)
            return true
        }
        if !published {
            log.error("music gain command queue full", ["capacity": gainCommandCapacity])
        }
    }

    // MARK: Ring accessors (PlayerCore uses these for audible_position/ended)

    public var ringFillMs: Int64 {
        Int64(Double(ring.fill / channels) / sampleRate * 1000)
    }

    /// Drops buffered audio before loading a new element (spec 6.3 load).
    public func clearRing() { ring.clear() }

    /// Arms first-nonzero-sample detection for the next start (spec 6.3).
    public func armStartDetection() { armFirstSampleState.store(1) }

    // MARK: Voice inserts / clicks (AVAudioPlayerNode branch)

    /// Plays a WAV file; completion fires when playback finished (voice_ended).
    public func playInsert(fileURL: URL, at when: AVAudioTime? = nil, completion: @escaping () -> Void) throws {
        let file = try AVAudioFile(forReading: fileURL)
        insertPlayer.scheduleFile(file, at: when, completionCallbackType: .dataPlayedBack) { _ in
            completion()
        }
        if !insertPlayer.isPlaying { insertPlayer.play() }
    }

    /// Cancels a voice/click insert superseded by a newer element.
    public func stopInsert() {
        insertPlayer.stop()
        insertPlayer.reset()
        insertPlayer.play()
    }

    // MARK: Prepared media overlay branch

    func scheduleOverlay(
        _ buffer: AVAudioPCMBuffer,
        at when: AVAudioTime?,
        completion: @escaping () -> Void
    ) {
        overlayPlayer.scheduleBuffer(
            buffer,
            at: when,
            options: [],
            completionCallbackType: .dataPlayedBack
        ) { _ in completion() }
        if !overlayPlayer.isPlaying { overlayPlayer.play() }
    }

    func stopOverlay() {
        overlayPlayer.stop()
        overlayPlayer.reset()
        overlayPlayer.play()
    }

    func setOverlayGain(_ gain: Float) {
        overlayPlayer.volume = min(max(gain, 0), 1)
    }

    // MARK: Live PTT source branch

    var livePCMCapacityFrames: Int { liveRing.capacity }
    var livePCMBufferedFrames: Int { liveRing.fill }
    var livePCMUnderrunCallbacks: Int64 { liveUnderrunCounter.load() }
    var livePCMRenderedFrames: Int64 { liveRenderedFrameCounter.load() }

    func prepareLivePCM() -> Int64 {
        let generation = liveRouteGeneration.add(1)
        liveRing.clear()
        liveActive.store(0)
        publishLiveGain(0, fadeMs: 0)
        setMusicGain(Float(pow(10, -12.0 / 20.0)), fadeMs: 60)
        return generation
    }

    func activateLivePCM(generation: Int64) {
        guard liveRouteGeneration.load() == generation else { return }
        publishLiveGain(1, fadeMs: 5)
        liveActive.store(1)
    }

    func writeLivePCM(
        generation: Int64, samples: UnsafePointer<Float>, count: Int
    ) -> Int {
        guard liveRouteGeneration.load() == generation, count > 0 else { return 0 }
        return liveRing.write(samples, count: count)
    }

    func stopLivePCM(generation: Int64, discard: Bool) {
        guard liveRouteGeneration.load() == generation else { return }
        publishLiveGain(0, fadeMs: 8)
        liveControlQueue.asyncAfter(deadline: .now() + .milliseconds(12)) { [weak self] in
            guard let self, self.liveRouteGeneration.load() == generation else { return }
            if discard || self.liveRing.fill > 0 { self.liveRing.clear() }
            self.liveActive.store(0)
            self.setMusicGain(1, fadeMs: 160)
        }
    }

    private func publishLiveGain(_ target: Float, fadeMs: Int64) {
        let clamped = min(max(target, 0), 1)
        liveGainCommandProducerLock.withLock {
            liveGainTargetBits.store(Int64(clamped.bitPattern))
            liveGainRampFrames.store(fadeMs <= 0 ? 0 : max(1, fadeMs * 48))
            liveGainCommandGeneration.store(liveGainCommandGeneration.load() + 1)
        }
    }

    private func setLimiterParameter(_ parameter: AudioUnitParameterID, value: AudioUnitParameterValue) {
        AudioUnitSetParameter(
            limiter.audioUnit, parameter, kAudioUnitScope_Global, 0, value, 0)
    }

    var limiterReductionDB: Float {
        var value: AudioUnitParameterValue = 0
        AudioUnitGetParameter(
            limiter.audioUnit,
            kDynamicsProcessorParam_CompressionAmount,
            kAudioUnitScope_Global,
            0,
            &value)
        return value
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

    // MARK: FIFO reader (spec 6.3: interruptible idle; EOF -> reopen)

    private func startReader() {
        readerActive.store(1)
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
        while readerActive.load() != 0 {
            // A blocking FIFO open cannot observe stopEngine() when no writer
            // ever connects. Non-blocking open/read keeps shutdown bounded;
            // the ring-full loop below still provides lossless backpressure.
            let fd = open(fifoPath, O_RDONLY | O_NONBLOCK)
            if fd < 0 {
                log.warn("fifo open failed", ["errno": errno])
                Thread.sleep(forTimeInterval: 0.5)
                continue
            }
            while readerActive.load() != 0 {
                let n = byteBuf.withUnsafeMutableBytes { raw in
                    read(fd, raw.baseAddress, chunkBytes)
                }
                if n < 0, errno == EAGAIN || errno == EWOULDBLOCK {
                    usleep(backpressureSleepUS)
                    continue
                }
                if n <= 0 { break } // EOF/error: writer closed -> reopen
                let floats = n / MemoryLayout<Float>.size
                var offset = 0
                byteBuf.withUnsafeBytes { raw in
                    let base = raw.baseAddress!.assumingMemoryBound(to: Float.self)
                    while offset < floats && self.readerActive.load() != 0 {
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
            if readerActive.load() != 0 {
                Thread.sleep(forTimeInterval: 0.05)
            }
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
        let (nanos, nanosOverflow) = ms.magnitude.multipliedReportingOverflow(by: 1_000_000)
        let (scaled, scaledOverflow) = nanos.multipliedReportingOverflow(
            by: UInt64(timebase.denom))
        guard !nanosOverflow, !scaledOverflow else {
            return ms >= 0 ? UInt64.max : 0
        }
        let ticks = scaled / UInt64(timebase.numer)
        if ms >= 0 {
            let (result, overflow) = hostTime.addingReportingOverflow(ticks)
            return overflow ? UInt64.max : result
        }
        return ticks >= hostTime ? 0 : hostTime - ticks
    }

    /// Host time that corresponds to a wall-clock unix-ms deadline.
    public static func hostTime(forUnixMs deadline: Int64) -> UInt64 {
        let nowMs = Int64((Date().timeIntervalSince1970 * 1000).rounded())
        let (delta, overflow) = deadline.subtractingReportingOverflow(nowMs)
        if overflow { return deadline >= 0 ? UInt64.max : 0 }
        return addMs(mach_absolute_time(), ms: delta)
    }
}
