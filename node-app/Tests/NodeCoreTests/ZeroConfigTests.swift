import Foundation
import Testing
@testable import NodeCore

// R1 zero-yml mode: no config file -> built-in defaults; unpaired state is
// legal; pairing credentials complete the config.
struct ZeroConfigTests {
    @Test func missingFileYieldsDefaults() throws {
        let ghost = "/nonexistent/\(UUID().uuidString)/node.yml"
        let cfg = try ConfigLoader.load(path: ghost)
        #expect(cfg.coordinator.token.isEmpty)
        #expect(cfg.audio.fifoPath.hasSuffix("/Pulsar/spotify.fifo"))
        #expect(cfg.audio.sampleRate == 44100)
        #expect(cfg.airfoil.isEnabled == false)
        #expect(cfg.effectiveDeviceName == "Pulsar A")
    }

    @Test func credentialsCompleteDefaults() throws {
        let ghost = "/nonexistent/\(UUID().uuidString)/node.yml"
        let creds = NodeCredentials(orbitId: 3, slot: "b",
                                    token: String(repeating: "a", count: 64),
                                    wsUrl: "wss://barycenter.relux.works/ws")
        let cfg = try ConfigLoader.load(path: ghost, credentials: creds)
        #expect(cfg.nodeId == "b")
        #expect(cfg.coordinator.url == "wss://barycenter.relux.works/ws")
        #expect(cfg.effectiveDeviceName == "Pulsar B")
    }

    @Test func binaryResolutionFallsBackToBrew() throws {
        let ghost = "/nonexistent/\(UUID().uuidString)/node.yml"
        let cfg = try ConfigLoader.load(path: ghost)
        // Test binary is unbundled: resolution lands on the brew path.
        #expect(cfg.effectiveLibrespotBinary.hasSuffix("go-librespot"))
    }
}
