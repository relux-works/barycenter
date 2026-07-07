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

extension ZeroConfigTests {
    @Test func retiresLocalConfigKeepsRemote() throws {
        let dir = FileManager.default.temporaryDirectory
            .appendingPathComponent("retire-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let p = dir.appendingPathComponent("node.yml").path
        let localYaml = "node_id: a\ncoordinator:\n  url: ws://127.0.0.1:8091/ws\n  token: \"" + String(repeating: "a", count: 64) + "\"\naudio:\n  fifo_path: /tmp/f\n  sample_rate: 44100\n  format: f32le\n  output_latency_offset_ms: 0\n  ring_buffer_ms: 1000\nairfoil:\n  app_path: /x\n  speakers: []\n  poll_s: 10\nlibrespot:\n  binary: /x\n  api_port: 3678\ncache_dir: /tmp/c\nlog:\n  level: info\n  path: /tmp/l\n"
        try localYaml.write(toFile: p, atomically: true, encoding: .utf8)
        #expect(ConfigLoader.retireLegacyLocalConfig(path: p) == true)
        #expect(!FileManager.default.fileExists(atPath: p))
        #expect(FileManager.default.fileExists(atPath: p + ".retired"))

        // A remote-pointing config is left alone.
        let remote = localYaml.replacingOccurrences(of: "ws://127.0.0.1:8091/ws", with: "wss://barycenter.relux.works/ws")
        try remote.write(toFile: p, atomically: true, encoding: .utf8)
        #expect(ConfigLoader.retireLegacyLocalConfig(path: p) == false)
        #expect(FileManager.default.fileExists(atPath: p))
    }
}
