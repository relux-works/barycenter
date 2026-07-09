package main

import (
	"fmt"
	"strings"

	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
)

// handleExternalPlayback adopts Spotify selections into the shared air. In a
// busy group, the selecting home's own barycenter policy arbitrates.
func (l *loop) handleExternalPlayback(o *orbitState, key hub.NodeKey, uri string, positionMS int64) {
	trackID, isTrack := strings.CutPrefix(uri, "spotify:track:")
	if o.sess.Mode != session.ModeShared || !isTrack || trackID == "" {
		return
	}
	if positionMS < 0 {
		positionMS = 0
	}
	id := l.peerFor(o, key)
	policy := o.takeoverPolicy
	if o.group() {
		policy = l.orbit(key.Orbit).takeoverPolicy
	}
	l.st.LogEvent(string(key.Slot), "external_playback", map[string]any{
		"uri": uri, "position_ms": positionMS, "policy": policy,
	})

	busy := o.sess.Current != nil || o.sess.QueueLen() > 0 ||
		(o.sess.Playlist != nil && o.sess.Playlist.Cursor < len(o.sess.Playlist.Tracks))
	if policy == "user" || !busy {
		l.adoptPulsarTrack(o, id, uri, positionMS)
		return
	}

	l.notify(o, fmt.Sprintf("дом %s вмешался с телефона — эфир восстановлен", l.peerName(o, id)))
	if o.sess.State == session.StatePlaying {
		l.apply(o, o.sess.CmdSync())
		return
	}
	l.hub.Send(key, protocol.TypeStop, &protocol.StopPayload{})
}

func (l *loop) adoptPulsarTrack(o *orbitState, id protocol.NodeID, uri string, positionMS int64) {
	el := l.newTrackElement(uri, string(id))
	l.st.InsertElement(el)
	l.log.Info("adopting Pulsar playback into shared air",
		"orbit", o.id, "peer", id, "uri", uri, "position_ms", positionMS)
	l.apply(o, o.sess.CmdPlayNowAt(el, positionMS))
}
