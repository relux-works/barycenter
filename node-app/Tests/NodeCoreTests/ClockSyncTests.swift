import Testing
@testable import NodeCore

@Suite struct ClockSyncTests {
    @Test func perfectSymmetricLinkConverges() {
        var sync = ClockSync()
        // Node clock is 250 ms ahead of coordinator; one-way delay 20 ms.
        // t1 node=1000 (coord 750), t2 coord=770, t3 coord=772, t4 node=1042.
        for i in 0..<20 {
            let base = Int64(i * 1000)
            sync.addSample(t1: 1000 + base, t2: 770 + base, t3: 772 + base, t4: 1042 + base)
        }
        let offset = try! #require(sync.offsetMs)
        #expect(abs(offset - 250) < 1, "offset \(offset), want ~250")
        #expect(sync.lastRttMs == 40)
    }

    @Test func outlierRttRejected() {
        var sync = ClockSync()
        for i in 0..<10 {
            let base = Int64(i * 1000)
            sync.addSample(t1: 1000 + base, t2: 770 + base, t3: 772 + base, t4: 1042 + base)
        }
        let before = try! #require(sync.offsetMs)
        // Congestion spike: rtt 400 ms with a wildly wrong asymmetric offset.
        let accepted = sync.addSample(t1: 100_000, t2: 99_500, t3: 99_502, t4: 100_402)
        #expect(!accepted, "rtt 400 > 3x median(40) must be rejected")
        let after = try! #require(sync.offsetMs)
        #expect(abs(after - before) < 0.001, "rejected sample must not move the offset")
    }

    @Test func emaSmoothsJitter() {
        var sync = ClockSync()
        sync.addSample(t1: 1000, t2: 770, t3: 772, t4: 1042) // offset 250
        sync.addSample(t1: 2000, t2: 1760, t3: 1762, t4: 2042) // sample says 260
        let offset = try! #require(sync.offsetMs)
        // EMA alpha 0.2: 250 + 0.2*(260-250) = 252
        #expect(abs(offset - 252) < 0.5, "offset \(offset), want ~252")
    }

    @Test func localDeadlineAppliesOffsetAndLatency() {
        var sync = ClockSync()
        sync.addSample(t1: 1000, t2: 770, t3: 772, t4: 1042) // node ahead by 250
        let t = sync.localDeadline(forCoordinatorMs: 10_000, outputLatencyOffsetMs: 120)
        // T_local = T_coord + offset - latency = 10000 + 250 - 120 (spec 6.3)
        #expect(t == 10_130)
    }

    @Test func noSamplesMeansNoDeadline() {
        let sync = ClockSync()
        #expect(sync.localDeadline(forCoordinatorMs: 1, outputLatencyOffsetMs: 0) == nil)
    }
}
