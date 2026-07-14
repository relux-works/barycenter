import CAtomics
import Foundation

/// Tiny fixed-storage atomic used by the audio render boundary. Allocation is
/// performed during graph construction; render operations are C11 atomics only.
final class RenderAtomicInt64 {
    private let storage: UnsafeMutableRawPointer

    init(_ value: Int64 = 0) {
        storage = .allocate(byteCount: MemoryLayout<Int64>.size,
                            alignment: MemoryLayout<Int64>.alignment)
        ca_store_release(storage, value)
    }

    deinit { storage.deallocate() }

    @inline(__always) func load() -> Int64 {
        ca_load_acquire(storage)
    }

    @inline(__always) func store(_ value: Int64) {
        ca_store_release(storage, value)
    }

    @discardableResult
    @inline(__always) func add(_ delta: Int64) -> Int64 {
        ca_fetch_add_relaxed(storage, delta)
    }

    @inline(__always) func compareExchange(expected: inout Int64, desired: Int64) -> Bool {
        ca_compare_exchange(storage, &expected, desired)
    }
}
