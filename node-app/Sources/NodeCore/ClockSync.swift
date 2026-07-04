// Clock offset estimation against the coordinator (spec 8.5):
// NTP-style four timestamps, EMA smoothing (alpha 0.2), and rejection of
// samples with rtt > 3x the median of the last ten.

import Foundation

public struct ClockSync {
    public private(set) var offsetMs: Double? // node clock minus coordinator clock
    public private(set) var lastRttMs: Int64 = 0

    private var recentRtts: [Int64] = []
    private let alpha: Double

    public init(alpha: Double = 0.2) {
        self.alpha = alpha
    }

    /// Feeds one ping/pong exchange. Returns true if the sample was accepted.
    @discardableResult
    public mutating func addSample(t1: Int64, t2: Int64, t3: Int64, t4: Int64) -> Bool {
        let rtt = (t4 - t1) - (t3 - t2)
        guard rtt >= 0 else { return false }

        lastRttMs = rtt

        if recentRtts.count >= 10 { recentRtts.removeFirst() }
        recentRtts.append(rtt)

        if let m = median(), recentRtts.count >= 4, Double(rtt) > 3 * m, m > 0 {
            return false // transient congestion: keep the offset we trust
        }

        let sample = (Double(t1 - t2) + Double(t4 - t3)) / 2
        if let current = offsetMs {
            offsetMs = current + alpha * (sample - current)
        } else {
            offsetMs = sample
        }
        return true
    }

    private func median() -> Double? {
        guard !recentRtts.isEmpty else { return nil }
        let sorted = recentRtts.sorted()
        let mid = sorted.count / 2
        if sorted.count % 2 == 0 {
            return Double(sorted[mid - 1] + sorted[mid]) / 2
        }
        return Double(sorted[mid])
    }

    /// T_local = T_coord + clock_offset - output_latency_offset (spec 6.3/5.4).
    public func localDeadline(forCoordinatorMs tCoord: Int64, outputLatencyOffsetMs: Int) -> Int64? {
        guard let off = offsetMs else { return nil }
        return tCoord + Int64(off.rounded()) - Int64(outputLatencyOffsetMs)
    }
}
