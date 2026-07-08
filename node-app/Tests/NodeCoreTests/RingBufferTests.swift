import Foundation
import Testing
@testable import NodeCore

@Suite struct RingBufferTests {
    @Test func writeReadRoundTrip() {
        let ring = RingBuffer(capacityFloats: 16)
        var input: [Float] = [1, 2, 3, 4, 5, 6]
        let written = input.withUnsafeBufferPointer { ring.write($0.baseAddress!, count: $0.count) }
        #expect(written == 6)
        #expect(ring.fill == 6)

        var out = [Float](repeating: 0, count: 6)
        let read = out.withUnsafeMutableBufferPointer { ring.read(into: $0.baseAddress!, count: 6) }
        #expect(read == 6)
        #expect(out == input)
        #expect(ring.fill == 0)
    }

    @Test func fullRingRefusesWithoutDropping() {
        let ring = RingBuffer(capacityFloats: 8)
        var chunk = [Float](repeating: 7, count: 8)
        let first = chunk.withUnsafeBufferPointer { ring.write($0.baseAddress!, count: 8) }
        #expect(first == 8)

        var extra: [Float] = [9, 9]
        let second = extra.withUnsafeBufferPointer { ring.write($0.baseAddress!, count: 2) }
        #expect(second == 0, "full ring must refuse, not overwrite (spec 6.3: drop forbidden)")

        var out = [Float](repeating: 0, count: 8)
        _ = out.withUnsafeMutableBufferPointer { ring.read(into: $0.baseAddress!, count: 8) }
        #expect(out.allSatisfy { $0 == 7 }, "backpressure must preserve the oldest audio")
    }

    @Test func partialWriteReportsCount() {
        let ring = RingBuffer(capacityFloats: 8)
        var chunk = [Float](repeating: 1, count: 6)
        _ = chunk.withUnsafeBufferPointer { ring.write($0.baseAddress!, count: 6) }
        var more = [Float](repeating: 2, count: 6)
        let n = more.withUnsafeBufferPointer { ring.write($0.baseAddress!, count: 6) }
        #expect(n == 2, "only the free space is written; caller retries the rest")
        #expect(ring.fill == 8)
    }

    @Test func emptyReadReturnsZero() {
        let ring = RingBuffer(capacityFloats: 8)
        var out = [Float](repeating: 42, count: 4)
        let n = out.withUnsafeMutableBufferPointer { ring.read(into: $0.baseAddress!, count: 4) }
        #expect(n == 0, "underrun: caller zero-fills, ring must not fabricate data")
    }

    @Test func wrapAroundKeepsOrder() {
        let ring = RingBuffer(capacityFloats: 8)
        var a: [Float] = [1, 2, 3, 4, 5, 6]
        _ = a.withUnsafeBufferPointer { ring.write($0.baseAddress!, count: 6) }
        var out4 = [Float](repeating: 0, count: 4)
        _ = out4.withUnsafeMutableBufferPointer { ring.read(into: $0.baseAddress!, count: 4) }

        var b: [Float] = [7, 8, 9, 10] // crosses the physical end
        let n = b.withUnsafeBufferPointer { ring.write($0.baseAddress!, count: 4) }
        #expect(n == 4)

        var out6 = [Float](repeating: 0, count: 6)
        let got = out6.withUnsafeMutableBufferPointer { ring.read(into: $0.baseAddress!, count: 6) }
        #expect(got == 6)
        #expect(out6 == [5, 6, 7, 8, 9, 10])
    }

    @Test func clearEmptiesTheRing() {
        let ring = RingBuffer(capacityFloats: 8)
        var a: [Float] = [1, 2, 3]
        _ = a.withUnsafeBufferPointer { ring.write($0.baseAddress!, count: 3) }
        ring.clear()
        #expect(ring.fill == 0)
    }

    // SPSC integrity under real concurrency: a monotonic sequence pushed with
    // backpressure retries must come out complete and ordered.
    @Test func concurrentProducerConsumerIntegrity() async {
        let ring = RingBuffer(capacityFloats: 1024)
        let total = 200_000

        let producer = Thread {
            var value: Float = 0
            var chunk = [Float](repeating: 0, count: 300)
            var sent = 0
            while sent < total {
                let n = min(300, total - sent)
                for i in 0..<n { chunk[i] = value + Float(i) }
                var offset = 0
                while offset < n {
                    let written = chunk.withUnsafeBufferPointer {
                        ring.write($0.baseAddress! + offset, count: n - offset)
                    }
                    if written == 0 {
                        usleep(200) // backpressure stall, never drop
                    }
                    offset += written
                }
                value += Float(n)
                sent += n
            }
        }
        producer.start()

        var received = 0
        var expected: Float = 0
        var out = [Float](repeating: 0, count: 257) // deliberately co-prime-ish chunk
        var corrupt = false
        while received < total {
            let n = out.withUnsafeMutableBufferPointer { ring.read(into: $0.baseAddress!, count: 257) }
            if n == 0 {
                usleep(100)
                continue
            }
            for i in 0..<n where out[i] != expected + Float(i) {
                corrupt = true
            }
            expected += Float(n)
            received += n
        }
        #expect(!corrupt, "sequence corrupted in SPSC transfer")
        #expect(received == total)
    }

    // M7 regression: clear is posted, consumer-applied — a third thread's
    // tail store raced read()'s own and could rewind tail below a concurrent
    // reader's position (stale-audio replay after load/seek).
    @Test func clearIsConsumerApplied() {
        let ring = RingBuffer(capacityFloats: 64)
        var input: [Float] = [1, 2, 3, 4]
        _ = input.withUnsafeBufferPointer { ring.write($0.baseAddress!, count: $0.count) }
        ring.clear()
        var out = [Float](repeating: 0, count: 64)
        let n = out.withUnsafeMutableBufferPointer { ring.read(into: $0.baseAddress!, count: 64) }
        #expect(n == 0, "read after clear must return nothing")
        #expect(ring.fill == 0)

        // Samples written AFTER the clear are kept intact.
        var fresh: [Float] = [7, 8]
        _ = fresh.withUnsafeBufferPointer { ring.write($0.baseAddress!, count: 2) }
        let m = out.withUnsafeMutableBufferPointer { ring.read(into: $0.baseAddress!, count: 64) }
        #expect(m == 2)
        #expect(out[0] == 7 && out[1] == 8)

        // Racing clears keep the later cut (watermark never lowers).
        var one: [Float] = [9]
        _ = one.withUnsafeBufferPointer { ring.write($0.baseAddress!, count: 1) }
        ring.clear()
        ring.clear()
        let z = out.withUnsafeMutableBufferPointer { ring.read(into: $0.baseAddress!, count: 64) }
        #expect(z == 0, "double clear leaked audio")
    }

    // Chaos: producer, consumer and a clearer all running — the exact shape
    // of the production load/seek/stopAll paths that the old contract forbade.
    @Test func clearConcurrentWithPumpAndRender() {
        let ring = RingBuffer(capacityFloats: 1 << 12)
        let group = DispatchGroup()
        DispatchQueue.global().async(group: group) { // pipe pump
            let chunk = [Float](repeating: 1, count: 128)
            for _ in 0..<20_000 {
                _ = chunk.withUnsafeBufferPointer { ring.write($0.baseAddress!, count: $0.count) }
            }
        }
        DispatchQueue.global().async(group: group) { // render loop
            var out = [Float](repeating: 0, count: 96)
            for _ in 0..<20_000 {
                _ = out.withUnsafeMutableBufferPointer { ring.read(into: $0.baseAddress!, count: 96) }
                precondition(ring.fill >= 0, "fill went negative")
            }
        }
        DispatchQueue.global().async(group: group) { // PlayerCore clears
            for _ in 0..<10_000 { ring.clear() }
        }
        #expect(group.wait(timeout: .now() + 30) == .success)
        #expect(ring.fill >= 0)
    }
}

