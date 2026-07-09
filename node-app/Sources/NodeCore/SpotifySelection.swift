enum SpotifySelection {
    static func shouldReport(
        mode: String,
        uri: String?,
        expectedURI: String?,
        playback: PlayerCore.Playback,
        allowSameURI: Bool
    ) -> Bool {
        guard mode == "shared", let uri else { return false }
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
}
