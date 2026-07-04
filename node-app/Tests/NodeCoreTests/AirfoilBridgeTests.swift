import Foundation
import Testing
@testable import NodeCore

@Suite struct AirfoilBridgeTests {
    @Test func scriptsQuoteNamesSafely() {
        let s = AirfoilBridge.scriptConnect(speaker: #"Кухня "верхняя""#)
        #expect(s.contains(#"name is "Кухня \"верхняя\"""#))
        #expect(s.contains("connect to ("), "Airfoil 5.12.6 dictionary: command is 'connect to'")
        #expect(AirfoilBridge.scriptDisconnect(speaker: "X").contains("disconnect from ("))
        let src = AirfoilBridge.scriptSetSource(nodeAppPath: "/Users/duet/duet/NodeApp.app")
        #expect(src.contains(#"POSIX file "/Users/duet/duet/NodeApp.app""#))
        #expect(src.contains("current audio source"))
    }

    @Test func parseStatesTable() {
        let raw = "Гостиная\ttrue\t1.0\nКухня\tfalse\t0.5\nComputer\ttrue\t0,75\n"
        let states = AirfoilBridge.parseStates(raw)
        #expect(states == [
            AirfoilSpeaker(name: "Гостиная", connected: true, volume: 1.0),
            AirfoilSpeaker(name: "Кухня", connected: false, volume: 0.5),
            AirfoilSpeaker(name: "Computer", connected: true, volume: 0.75), // comma decimal (ru locale)
        ])
    }

    @Test func parseTolleratesGarbage() {
        #expect(AirfoilBridge.parseStates("").isEmpty)
        #expect(AirfoilBridge.parseStates("odd\tline\n").isEmpty)
        #expect(AirfoilBridge.parseStates("ok\ttrue\t1\nbroken row\n").count == 1)
    }

    @Test func backoffCapsAtSixtySeconds() {
        #expect(AirfoilBridge.backoffDelay(attempt: 0) == 5)
        #expect(AirfoilBridge.backoffDelay(attempt: 1) == 10)
        #expect(AirfoilBridge.backoffDelay(attempt: 2) == 20)
        #expect(AirfoilBridge.backoffDelay(attempt: 10) == 60)
    }
}
