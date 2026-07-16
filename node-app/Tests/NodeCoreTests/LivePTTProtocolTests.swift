import Foundation
import Testing
@testable import NodeCore

private func livePTTHex(_ value: String) -> Data {
    var result = Data(); var index = value.startIndex
    while index < value.endIndex {
        let next = value.index(index, offsetBy: 2)
        result.append(UInt8(value[index..<next], radix: 16)!)
        index = next
    }
    return result
}

@Suite struct LivePTTProtocolTests {
    let start = "4250010500112233445566778899aabbccddeeff0000000100000000000f42400003140101010000f8fffe"
    let middle = "4250010400112233445566778899aabbccddeeff0000000200000000000f90600003140101010000f8fffd"
    let end = "4250010600112233445566778899aabbccddeeff0000000300000000000fde800003140101010000f8fffc"

    @Test func binaryVectorsRoundTripAndGuardGenerations() throws {
        let frames = try [start, middle, end].map { try LivePTTBinaryFrame.decode(livePTTHex($0)) }
        #expect(frames.map(\.sequence) == [1, 2, 3])
        #expect(try frames.map { try $0.encoded() } == [start, middle, end].map(livePTTHex))
        var guardState = LivePTTFrameGuard(sessionId: frames[0].sessionId, generation: 7)
        #expect(guardState.accept(frames[0]) == .apply)
        #expect(guardState.accept(frames[0]) == .duplicate)
        #expect(guardState.accept(frames[1]) == .apply)
        #expect(guardState.accept(frames[2]) == .apply)
        #expect(guardState.accept(frames[2]) == .stale)
    }

    @Test func malformedFramesAreRejected() {
        let valid = [UInt8](livePTTHex(start))
        var cases = [Data(valid.prefix(39))]
        for (offset, value) in [(2, UInt8(2)), (3, UInt8(132)), (34, UInt8(10)), (39, UInt8(1))] {
            var copy = valid; copy[offset] = value; cases.append(Data(copy))
        }
        var zeroSequence = valid; zeroSequence[20] = 0; zeroSequence[21] = 0; zeroSequence[22] = 0; zeroSequence[23] = 0
        cases.append(Data(zeroSequence))
        for value in cases { #expect(throws: LivePTTProtocolError.self) { try LivePTTBinaryFrame.decode(value) } }
    }
}
