import Foundation
import Testing
@testable import NodeCore

private let validYAML = """
node_id: a
coordinator:
  url: ws://coord:8080/ws
  token: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
audio:
  fifo_path: /Users/duet/duet/spotify.fifo
  sample_rate: 44100
  format: f32le
  output_latency_offset_ms: 0
  ring_buffer_ms: 1000
airfoil:
  app_path: /Applications/Airfoil.app
  speakers: ["Гостиная", "Кухня"]
  poll_s: 10
librespot:
  binary: /opt/homebrew/bin/go-librespot
  api_port: 3678
  device_name: "Дом A"
cache_dir: /Users/duet/duet/cache
log:
  level: info
  path: /Users/duet/duet/nodeapp.log
"""

private func loadFromString(_ yaml: String) throws -> NodeConfig {
    let dir = FileManager.default.temporaryDirectory
        .appendingPathComponent("duet-config-tests", isDirectory: true)
    try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    let file = dir.appendingPathComponent(UUID().uuidString + ".yml")
    try yaml.write(to: file, atomically: true, encoding: .utf8)
    defer { try? FileManager.default.removeItem(at: file) }
    return try ConfigLoader.load(path: file.path)
}

@Suite struct ConfigTests {
    @Test func validSpecExampleLoads() throws {
        let cfg = try loadFromString(validYAML)
        #expect(cfg.nodeId == "a")
        #expect(cfg.airfoil.speakers == ["Гостиная", "Кухня"])
        #expect(cfg.audio.ringBufferMs == 1000)
    }

    @Test func badTokenProducesHumanMessage() {
        do {
            _ = try loadFromString(validYAML.replacingOccurrences(
                of: "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
                with: "deadbeef"))
            Issue.record("must throw")
        } catch let err as ConfigError {
            #expect(err.description.contains("coordinator.token"))
            #expect(err.description.contains("64 hex"))
        } catch {
            Issue.record("wrong error type: \(error)")
        }
    }

    @Test func badNodeIdAndSchemeCollectBothProblems() {
        do {
            _ = try loadFromString(validYAML
                .replacingOccurrences(of: "node_id: a", with: "node_id: c")
                .replacingOccurrences(of: "ws://coord:8080/ws", with: "http://coord:8080/ws"))
            Issue.record("must throw")
        } catch let err as ConfigError {
            #expect(err.problems.count == 2, "both problems reported at once: \(err.problems)")
            #expect(err.description.contains("node_id"))
            #expect(err.description.contains("ws://"))
        } catch {
            Issue.record("wrong error type: \(error)")
        }
    }

    @Test func emptySpeakersRejectedOnlyWhenAirfoilEnabled() throws {
        // v1.3 direct mode (default): empty speakers list is fine.
        let direct = try loadFromString(validYAML.replacingOccurrences(
            of: #"speakers: ["Гостиная", "Кухня"]"#, with: "speakers: []"))
        #expect(direct.airfoil.isEnabled == false)

        // airfoil mode still demands speakers.
        do {
            _ = try loadFromString(validYAML
                .replacingOccurrences(of: #"speakers: ["Гостиная", "Кухня"]"#, with: "speakers: []")
                .replacingOccurrences(of: "airfoil:", with: "airfoil:\n  enabled: true"))
            Issue.record("must throw")
        } catch let err as ConfigError {
            #expect(err.description.contains("airfoil.speakers"))
        } catch {
            Issue.record("wrong error type: \(error)")
        }
    }

    @Test func deviceNameDefaultsToPulsarByNode() throws {
        let cfg = try loadFromString(validYAML.replacingOccurrences(
            of: "  device_name: \"Дом A\"\n", with: ""))
        #expect(cfg.effectiveDeviceName == "Pulsar A")
        let explicit = try loadFromString(validYAML)
        #expect(explicit.effectiveDeviceName == "Дом A")
    }

    @Test func directModeOutputDevice() throws {
        let cfg = try loadFromString(validYAML.replacingOccurrences(
            of: "audio:", with: "audio:\n  output_device: \"Tima's JBL\""))
        #expect(cfg.audio.outputDevice == "Tima's JBL")
        #expect(cfg.airfoil.isEnabled == false)
    }

    @Test func missingFileExplainsItself() {
        do {
            _ = try ConfigLoader.load(path: "/nonexistent/duet/node.yml")
            Issue.record("must throw")
        } catch let err as ConfigError {
            #expect(err.description.contains("cannot read"))
        } catch {
            Issue.record("wrong error type: \(error)")
        }
    }
}
