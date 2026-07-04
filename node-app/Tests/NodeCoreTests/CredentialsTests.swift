import Foundation
import Testing
@testable import NodeCore

// Pairing credentials: JSON round-trip and config override (v2.1 M1).
struct CredentialsTests {
    @Test func roundTripAndOverride() throws {
        let dir = FileManager.default.temporaryDirectory
            .appendingPathComponent("creds-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let configPath = dir.appendingPathComponent("node.yml").path

        let yml = """
        node_id: a
        coordinator:
          url: ws://127.0.0.1:1/ws
          token: "\(String(repeating: "0", count: 64))"
        audio:
          fifo_path: \(dir.path)/f.fifo
          sample_rate: 44100
          format: f32le
          output_latency_offset_ms: 0
          ring_buffer_ms: 1000
        airfoil:
          enabled: false
          app_path: /Applications/Airfoil.app
          speakers: []
          poll_s: 10
        librespot:
          binary: /opt/homebrew/opt/go-librespot/bin/go-librespot
          api_port: 3678
          config_dir: \(dir.path)/ls
        cache_dir: \(dir.path)/cache
        log:
          level: info
          path: \(dir.path)/n.log
        """
        try yml.write(toFile: configPath, atomically: true, encoding: .utf8)

        let creds = NodeCredentials(
            orbitId: 7, slot: "c",
            token: String(repeating: "f", count: 64),
            wsUrl: "wss://barycenter.relux.works/ws")
        try creds.save(besideConfig: configPath)

        let loaded = NodeCredentials.load(besideConfig: configPath)
        #expect(loaded == creds)

        let cfg = try ConfigLoader.load(path: configPath, credentials: loaded)
        #expect(cfg.nodeId == "c")
        #expect(cfg.coordinator.url == "wss://barycenter.relux.works/ws")
        #expect(cfg.coordinator.token == creds.token)

        // Without credentials the yml stands as written.
        let plain = try ConfigLoader.load(path: configPath)
        #expect(plain.nodeId == "a")
    }
}
