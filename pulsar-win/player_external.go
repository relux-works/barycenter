package main

import (
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

// reportExternalSelection turns a Spotify selection on this Pulsar into a
// shared-air leader event. observedPosition is present on metadata events; a
// playing event without matching metadata safely starts a foreign URI at zero.
func (p *Player) reportExternalSelection(
	uri *string,
	observedPosition *int64,
	allowSameURI bool,
) {
	p.mu.Lock()
	if p.mode != "shared" || uri == nil {
		p.mu.Unlock()
		return
	}
	sameURI := *uri == p.uri
	coordinatorOwnsURI := sameURI &&
		(!allowSameURI || p.playback == PlaybackLoading || p.playback == PlaybackPlaying)
	if coordinatorOwnsURI || time.Since(p.lastExternalReport) <= p.externalDebounce {
		p.mu.Unlock()
		return
	}
	p.lastExternalReport = time.Now()
	expected := p.uri
	positionMS := int64(0)
	if observedPosition != nil || sameURI {
		positionMS = p.audiblePositionLocked()
	}
	p.mu.Unlock()

	if expected == "" {
		expected = "silence"
	}
	p.log.Warn("external playback detected in shared", "uri", *uri, "expected", expected)
	p.send(protocol.TypeExternalPlayback, &protocol.ExternalPlaybackPayload{
		URI: *uri, PositionMS: &positionMS,
	})
}
