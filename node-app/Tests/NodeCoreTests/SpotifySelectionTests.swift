import Testing
@testable import NodeCore

@Suite struct SpotifySelectionTests {
    @Test func coordinatorOwnedPlaybackIsNotReported() {
        #expect(!SpotifySelection.shouldReport(
            mode: "shared", uri: "spotify:track:x", expectedURI: "spotify:track:x",
            playback: .playing, allowSameURI: true))
        #expect(!SpotifySelection.shouldReport(
            mode: "shared", uri: "spotify:track:x", expectedURI: "spotify:track:x",
            playback: .loading, allowSameURI: false))
    }

    @Test func stoppedSameTrackIsANewSelection() {
        #expect(SpotifySelection.shouldReport(
            mode: "shared", uri: "spotify:track:x", expectedURI: "spotify:track:x",
            playback: .stopped, allowSameURI: true))
    }

    @Test func onlyMatchingMetadataCarriesPosition() {
        #expect(SpotifySelection.startPosition(
            observedPosition: 63_000, uri: "spotify:track:new",
            expectedURI: "spotify:track:old", audiblePosition: 62_500) == 62_500)
        #expect(SpotifySelection.startPosition(
            observedPosition: nil, uri: "spotify:track:new",
            expectedURI: "spotify:track:old", audiblePosition: 80_000) == 0)
    }

    @Test func soloPlaybackIsNotReported() {
        #expect(!SpotifySelection.shouldReport(
            mode: "solo", uri: "spotify:track:new", expectedURI: nil,
            playback: .stopped, allowSameURI: true))
    }
}
