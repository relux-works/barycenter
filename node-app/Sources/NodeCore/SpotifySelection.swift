enum SpotifySelection {
    static func shouldReport(
        mode: String,
        uri: String?,
        expectedURI: String?,
        playback: PlayerCore.Playback,
        allowSameURI: Bool,
        playOrigin: String? = nil
    ) -> Bool {
        guard mode == "shared", let uri else { return false }
        // Every coordinator HTTP /player/play is stamped by go-librespot.
        // A stale internal load finishing after a newer command used to look
        // like a phone selection and resurrect an old album/context.
        if playOrigin == "go-librespot" { return false }
        let coordinatorOwnsURI = uri == expectedURI &&
            (!allowSameURI || playback == .loading || playback == .playing)
        return !coordinatorOwnsURI
    }

    static func startPosition(
        observedPosition: Int64?,
        uri: String,
        expectedURI: String?,
        audiblePosition: Int64
    ) -> Int64 {
        observedPosition != nil || uri == expectedURI ? audiblePosition : 0
    }

    static func displayTitle(name: String?, artistNames: [String]) -> String? {
        guard let name, !name.isEmpty else { return nil }
        guard !artistNames.isEmpty else { return name }
        return artistNames.joined(separator: ", ") + " — " + name
    }
}
