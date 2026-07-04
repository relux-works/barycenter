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

    public init(capacityFloats: Int) {
        precondition(capacityFloats > 0)
        capacity = capacityFloats
        storage = .allocate(capacity: capacityFloats)
        storage.initialize(repeating: 0, count: capacityFloats)
        head = .allocate(byteCount: 8, alignment: 8)
        tail = .allocate(byteCount: 8, alignment: 8)
        ca_store_release(head, 0)
        ca_store_release(tail, 0)
    }

    deinit {
        storage.deallocate()
        head.deallocate()
        tail.deallocate()
    }

    /// Floats currently readable. Safe from any thread (approximate between ops).
    public var fill: Int {
        Int(ca_load_acquire(head) - ca_load_acquire(tail))
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
        let t = ca_load_acquire(tail)
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

    /// Drops all buffered audio (load of a new element, spec 6.3).
    /// Caller must guarantee the producer is parked while clearing.
    public func clear() {
        ca_store_release(tail, ca_load_acquire(head))
    }
}
