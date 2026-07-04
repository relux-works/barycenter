// Spike S3: FIFO (f32le 44100 stereo interleaved) -> reader thread -> ring buffer
// -> AVAudioSourceNode -> mainMixer -> output.
//
// Proves the NodeApp music path without Spotify: feed the pipe with ffmpeg-generated
// PCM and watch the counters. Underrun must produce silence, never a crash.
// Locking is NSLock for spike simplicity; production ring buffer must be lock-free SPSC.
//
// Usage: fifo-player-spike <fifo-path> [output-volume 0..1, default 0] [run-seconds, default 12]

import AVFAudio
import Foundation

let args = CommandLine.arguments
let fifoPath = args.count > 1 ? args[1] : ".temp/spike/spotify.fifo"
let outVolume = args.count > 2 ? (Float(args[2]) ?? 0) : 0
let runSeconds = args.count > 3 ? (Int(args[3]) ?? 12) : 12

let sampleRate = 44100.0
let channels = 2

final class RingBuffer {
    private var storage: [Float]
    private var head = 0
    private var tail = 0
    private let lock = NSLock()

    private(set) var totalWritten: UInt64 = 0
    private(set) var totalRead: UInt64 = 0
    var underrunCallbacks: UInt64 = 0
    var nonZeroFrames: UInt64 = 0

    init(capacityFloats: Int) {
        storage = [Float](repeating: 0, count: capacityFloats)
    }

    var available: Int {
        lock.lock(); defer { lock.unlock() }
        return availableLocked
    }

    private var availableLocked: Int {
        (head - tail + storage.count) % storage.count
    }

    func write(_ data: UnsafePointer<Float>, count: Int) {
        lock.lock(); defer { lock.unlock() }
        let free = storage.count - 1 - availableLocked
        let toWrite = min(count, free) // overflow: drop newest (spike-only policy)
        for i in 0..<toWrite {
            storage[(head + i) % storage.count] = data[i]
        }
        head = (head + toWrite) % storage.count
        totalWritten += UInt64(toWrite)
    }

    /// Returns floats actually copied; caller zero-fills the rest.
    func read(into out: UnsafeMutablePointer<Float>, count: Int) -> Int {
        lock.lock(); defer { lock.unlock() }
        let toRead = min(count, availableLocked)
        for i in 0..<toRead {
            out[i] = storage[(tail + i) % storage.count]
        }
        tail = (tail + toRead) % storage.count
        totalRead += UInt64(toRead)
        return toRead
    }
}

let ring = RingBuffer(capacityFloats: Int(sampleRate) * channels) // 1.0 s

// MARK: - FIFO reader thread

let readerThread = Thread {
    let chunkBytes = 16384
    var byteBuf = [UInt8](repeating: 0, count: chunkBytes)
    var reopenCount = 0
    while true {
        // Blocks until a writer opens the pipe — same as waiting for go-librespot playback.
        let fd = open(fifoPath, O_RDONLY)
        if fd < 0 {
            FileHandle.standardError.write("reader: open failed errno=\(errno)\n".data(using: .utf8)!)
            Thread.sleep(forTimeInterval: 0.5)
            continue
        }
        if reopenCount > 0 {
            print("reader: pipe reopened (count \(reopenCount))")
        }
        while true {
            let n = byteBuf.withUnsafeMutableBytes { raw in
                read(fd, raw.baseAddress, chunkBytes)
            }
            if n <= 0 { break } // EOF (writer closed) or error -> reopen
            let floatCount = n / MemoryLayout<Float>.size
            byteBuf.withUnsafeBytes { raw in
                let floats = raw.baseAddress!.assumingMemoryBound(to: Float.self)
                ring.write(floats, count: floatCount)
            }
        }
        close(fd)
        reopenCount += 1
    }
}
readerThread.name = "fifo-reader"
readerThread.start()

// MARK: - Audio graph

let engine = AVAudioEngine()
// Deinterleaved standard format on the node; ring stays interleaved (librespot layout),
// render block splits channels.
guard let fmt = AVAudioFormat(standardFormatWithSampleRate: sampleRate, channels: AVAudioChannelCount(channels)) else {
    fatalError("format init failed")
}

var scratch = [Float](repeating: 0, count: 8192 * channels)

let srcNode = AVAudioSourceNode(format: fmt) { _, _, frameCount, audioBufferList -> OSStatus in
    let abl = UnsafeMutableAudioBufferListPointer(audioBufferList)
    let frames = Int(frameCount)
    let needFloats = frames * channels

    let got = scratch.withUnsafeMutableBufferPointer { buf -> Int in
        ring.read(into: buf.baseAddress!, count: needFloats)
    }
    if got < needFloats {
        ring.underrunCallbacks += 1
        for i in got..<needFloats { scratch[i] = 0 }
    }

    var nonZero: UInt64 = 0
    for f in 0..<frames where scratch[f * channels] != 0 { nonZero += 1 }
    ring.nonZeroFrames += nonZero

    for ch in 0..<min(channels, abl.count) {
        guard let mData = abl[ch].mData else { continue }
        let out = mData.assumingMemoryBound(to: Float.self)
        for f in 0..<frames {
            out[f] = scratch[f * channels + ch]
        }
        abl[ch].mDataByteSize = UInt32(frames * MemoryLayout<Float>.size)
    }
    return noErr
}

engine.attach(srcNode)
engine.connect(srcNode, to: engine.mainMixerNode, format: fmt)
engine.mainMixerNode.outputVolume = outVolume

do {
    try engine.start()
    print("engine started, output=\(engine.outputNode.outputFormat(forBus: 0).sampleRate) Hz, mixer volume=\(outVolume)")
} catch {
    fatalError("engine start failed: \(error)")
}

// MARK: - Report loop

for second in 1...runSeconds {
    Thread.sleep(forTimeInterval: 1.0)
    print(String(format: "t=%02ds written=%8llu read=%8llu avail=%6d underrunCbs=%5llu nonZeroFrames=%8llu",
                 second, ring.totalWritten, ring.totalRead, ring.available,
                 ring.underrunCallbacks, ring.nonZeroFrames))
}

let written = ring.totalWritten
let nonZero = ring.nonZeroFrames
print("SUMMARY floatsWritten=\(written) framesWritten=\(written / UInt64(channels)) nonZeroFramesRendered=\(nonZero) underrunCallbacks=\(ring.underrunCallbacks)")
print(nonZero > 0 && written > 0 ? "RESULT: PASS (data flowed FIFO -> ring -> render callback; underruns were silent)" : "RESULT: FAIL (no data reached render callback)")
exit(0)
