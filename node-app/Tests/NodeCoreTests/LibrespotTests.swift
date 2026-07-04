import Foundation
import Testing
@testable import NodeCore

@Suite struct LibrespotConfigRendererTests {
    @Test func rendersSpecA2Keys() {
        let yaml = LibrespotConfigRenderer.render(
            deviceName: "Дом A", apiPort: 3678, fifoPath: "/Users/duet/duet/spotify.fifo")
        for want in [
            "device_name: \"Дом A\"",
            "device_type: speaker",
            "type: zeroconf",
            "persist_credentials: true",
            "address: 127.0.0.1",
            "port: 3678",
            "audio_backend: pipe",
            "audio_output_pipe: /Users/duet/duet/spotify.fifo",
            "audio_output_pipe_format: f32le",
            "external_volume: true",
        ] {
            #expect(yaml.contains(want), "missing \(want) in rendered config")
        }
    }
}

@Suite struct LibrespotEventParsingTests {
    private func parse(_ json: String) -> LibrespotEvent? {
        LibrespotClient.parseEvent(Data(json.utf8))
    }

    @Test func knownEvents() throws {
        guard case .some(.notPlaying(let uri)) =
            parse(#"{"type":"not_playing","data":{"uri":"spotify:track:X"}}"#) else {
            Issue.record("not_playing failed")
            return
        }
        #expect(uri == "spotify:track:X")

        guard case .some(.metadata(let mUri, let name, let pos, let dur)) =
            parse(#"{"type":"metadata","data":{"uri":"spotify:track:Y","name":"Song","position":63000,"duration":214000}}"#) else {
            Issue.record("metadata failed")
            return
        }
        #expect(mUri == "spotify:track:Y")
        #expect(name == "Song")
        #expect(pos == 63000)
        #expect(dur == 214000)
    }

    @Test func unknownEventTolerated() {
        guard case .some(.other(let t)) = parse(#"{"type":"hologram","data":{}}"#) else {
            Issue.record("unknown event must map to .other")
            return
        }
        #expect(t == "hologram")
        #expect(parse("not json at all") == nil)
    }
}

@Suite struct VoiceCacheTests {
    @Test func mediaIDExtraction() {
        #expect(VoiceCache.mediaID(fromFileURL: "http://coord:8080/media/m_01ABC.wav") == "m_01ABC")
        #expect(VoiceCache.mediaID(fromFileURL: "m_02XYZ") == "m_02XYZ")
    }

    @Test func lruEviction() throws {
        let dir = FileManager.default.temporaryDirectory
            .appendingPathComponent("duet-voice-cache-test-\(UUID().uuidString)").path
        let cache = VoiceCache(cacheDir: dir, nodeToken: "t", capacity: 3,
                               log: Logger(level: .error, path: nil))
        for i in 1...5 {
            try cache.seed(id: "m\(i)", data: Data([0x00]))
        }
        #expect(cache.cachedIDs == ["m3", "m4", "m5"], "got \(cache.cachedIDs)")
    }
}

extension LibrespotEvent: Equatable {
    public static func == (l: LibrespotEvent, r: LibrespotEvent) -> Bool {
        switch (l, r) {
        case (.active, .active), (.inactive, .inactive), (.stopped, .stopped):
            return true
        case (.other(let a), .other(let b)):
            return a == b
        default:
            return false
        }
    }
}
