import Foundation
import Testing
@testable import NodeCore

@Suite struct ULIDTests {
    @Test func knownTimestampVector() {
        // ms = 1: canonical ULID timestamp part is nine zeros then "1".
        let ulid = ULID.new(now: Date(timeIntervalSince1970: 0.001))
        #expect(ulid.count == 26)
        #expect(ulid.hasPrefix("0000000001"), "got \(ulid)")
    }

    @Test func timestampPrefixSortsChronologically() {
        let earlier = ULID.new(now: Date(timeIntervalSince1970: 1_751_500_000.0))
        let later = ULID.new(now: Date(timeIntervalSince1970: 1_751_500_000.0 + 2))
        #expect(String(earlier.prefix(10)) < String(later.prefix(10)))
    }

    @Test func canonicalPrefixMatchesGoEncoder() {
        // Char 0 carries only the top 3 bits. For epochs 2004..2039 the
        // canonical timestamp prefix is "01" — same as the coordinator's Go
        // encoder produces (e.g. msg_01KW...).
        let ulid = ULID.new()
        #expect(ulid.hasPrefix("01"), "got \(ulid)")
    }
}
