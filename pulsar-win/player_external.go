package main

import (
	"strings"
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
	playOrigin *string,
) {
	p.mu.Lock()
	if p.mode != "shared" || uri == nil {
		p.mu.Unlock()
		return
	}
	if playOrigin != nil && *playOrigin == "go-librespot" {
		p.mu.Unlock()
		return
	}
	sameURI := *uri == p.uri
	coordinatorOwnsURI := sameURI &&
		(!allowSameURI || p.playback == PlaybackLoading || p.playback == PlaybackPlaying)
	duplicate := *uri == p.lastExternalURI && time.Since(p.lastExternalReport) <= p.externalDebounce
	if coordinatorOwnsURI || duplicate {
		p.mu.Unlock()
		return
	}
	p.lastExternalReport = time.Now()
	p.lastExternalURI = *uri
	expected := p.uri
	positionMS := int64(0)
	if observedPosition == nil && p.metadataURI == *uri {
		observedPosition = p.metadataPosition
	}
	if observedPosition != nil || sameURI {
		positionMS = p.audiblePositionLocked()
	}
	title := ""
	if p.metadataURI == *uri {
		title = p.metadataTitle
	}
	p.mu.Unlock()

	if expected == "" {
		expected = "silence"
	}
	origin := "unknown"
	if playOrigin != nil && *playOrigin != "" {
		origin = *playOrigin
	}
	p.log.Warn("external playback detected in shared", "uri", *uri, "expected", expected, "play_origin", origin)
	p.send(protocol.TypeExternalPlayback, &protocol.ExternalPlaybackPayload{
		URI: *uri, PositionMS: &positionMS, Title: title,
	})
}

func selectionDisplayTitle(name *string, artists []string) string {
	if name == nil || *name == "" {
		return ""
	}
	if len(artists) == 0 {
		return *name
	}
	return strings.Join(artists, ", ") + " — " + *name
}
