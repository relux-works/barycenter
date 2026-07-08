// SPSC lock-free ring buffer for interleaved Float32 PCM (spec 6.3).
//
// Contract:
// - Exactly one producer (FIFO reader thread) and one consumer (render callback).
// - write() never drops: it returns how many floats fit; the producer loop
//   holds the remainder and retries — that stall is the backpressure that
//   ultimately blocks go-librespot on the kernel pipe buffer.
// - read() never blocks: it returns what is available; the caller zero-fills
//   the shortfall (underrun = silence).
// - Indices are monotonically increasing int64 totals; head is written only by
//   the producer (release), tail only by the consumer (release), each side
//   reads the other with acquire ordering via the CAtomics shim.

import CAtomics
import Foundation

public final class RingBuffer {
    public let capacity: Int

    private let storage: UnsafeMutablePointer<Float>
    private let head: UnsafeMutableRawPointer // total floats written
    private let tail: UnsafeMutableRawPointer // total floats read
    // clearThrough: a head snapshot the consumer discards up to on its next
    // read (M7). clear() is called from PlayerCore's queue while both the
    // FIFO reader and the render callback keep running — a third-party tail
    // store raced read()'s own tail store and could rewind tail below a
    // concurrent reader's position, replaying a chunk of the previous track
    // after a load/seek. Only the consumer may move tail; clear() posts the cut.
    private let clearThrough: UnsafeMutableRawPointer

    public init(capacityFloats: Int) {
        precondition(capacityFloats > 0)
        capacity = capacityFloats
        storage = .allocate(capacity: capacityFloats)
        storage.initialize(repeating: 0, count: capacityFloats)
        head = .allocate(byteCount: 8, alignment: 8)
        tail = .allocate(byteCount: 8, alignment: 8)
        clearThrough = .allocate(byteCount: 8, alignment: 8)
        ca_store_release(head, 0)
        ca_store_release(tail, 0)
        ca_store_release(clearThrough, 0)
    }

    deinit {
        storage.deallocate()
        head.deallocate()
        tail.deallocate()
        clearThrough.deallocate()
    }

    /// Floats currently readable. Safe from any thread (approximate between
    /// ops). A pending clear counts as already applied — the next read will
    /// discard that span anyway.
    public var fill: Int {
        let h = ca_load_acquire(head)
        var t = ca_load_acquire(tail)
        let ct = ca_load_acquire(clearThrough)
        if ct > t { t = ct }
        return h > t ? Int(h - t) : 0
    }

    /// Producer side. Copies as much of buffer as fits; returns the count copied.
    public func write(_ buffer: UnsafePointer<Float>, count: Int) -> Int {
        let h = ca_load_acquire(head)
        let t = ca_load_acquire(tail)
        let free = capacity - Int(h - t)
        let n = min(count, free)
        if n <= 0 { return 0 }
        var idx = Int(h % Int64(capacity))
        var remaining = n
        var src = buffer
        while remaining > 0 {
            let chunk = min(remaining, capacity - idx)
            (storage + idx).update(from: src, count: chunk)
            src += chunk
            idx = (idx + chunk) % capacity
            remaining -= chunk
        }
        ca_store_release(head, h + Int64(n))
        return n
    }

    /// Consumer side (render callback: no locks, no allocation). Returns floats copied.
    public func read(into out: UnsafeMutablePointer<Float>, count: Int) -> Int {
        let h = ca_load_acquire(head)
        var t = ca_load_acquire(tail)
        // Apply a posted clear (M7): jump past everything buffered before the
        // cut. ct may exceed our stale h snapshot (a clear landed between the
        // two loads) — available then goes non-positive and we return 0.
        let ct = ca_load_acquire(clearThrough)
        if ct > t {
            t = ct
            ca_store_release(tail, t)
        }
        let available = Int(h - t)
        let n = min(count, available)
        if n <= 0 { return 0 }
        var idx = Int(t % Int64(capacity))
        var remaining = n
        var dst = out
        while remaining > 0 {
            let chunk = min(remaining, capacity - idx)
            dst.update(from: storage + idx, count: chunk)
            dst += chunk
            idx = (idx + chunk) % capacity
            remaining -= chunk
        }
        ca_store_release(tail, t + Int64(n))
        return n
    }

    /// Schedules everything buffered SO FAR to be dropped (load of a new
    /// element, spec 6.3). Safe from any thread (M7): the consumer applies the
    /// cut on its next read, samples written after this call are kept. The CAS
    /// loop only ever raises the watermark — racing clears keep the later cut.
    public func clear() {
        let h = ca_load_acquire(head)
        var cur = ca_load_acquire(clearThrough)
        while cur < h {
            if ca_compare_exchange(clearThrough, &cur, h) { return }
        }
    }
}
