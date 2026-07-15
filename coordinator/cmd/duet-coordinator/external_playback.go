package main

import (
	"fmt"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
	"relux.works/duet/coordinator/internal/store"
)

// handleExternalPlayback adopts Spotify selections into the shared air. In a
// busy group, the selecting home's own barycenter policy arbitrates.
func (l *loop) handleExternalPlayback(
	o *orbitState, key hub.NodeKey, uri string, positionMS int64, title string,
) {
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
		"uri": uri, "position_ms": positionMS, "title": title, "policy": policy,
	})

	busy := o.sess.Current != nil || o.sess.QueueLen() > 0 ||
		(o.sess.Playlist != nil && o.sess.Playlist.Cursor < len(o.sess.Playlist.Tracks))
	if policy == "user" || !busy {
		if _, err := l.st.AuthorizeInstallationAirAction(
			key.Orbit, string(key.Slot), store.AirPolicyReplace,
		); err != nil {
			l.log.Info("external playback denied by Air policy", "orbit", key.Orbit,
				"slot", key.Slot, "err", err)
			l.notify(o, fmt.Sprintf("дом %s выбрал трек, но политика Air запрещает замену эфира", l.peerName(o, id)))
			if o.sess.State == session.StatePlaying {
				l.apply(o, o.sess.CmdSync())
			} else {
				l.hub.Send(key, protocol.TypeStop, &protocol.StopPayload{})
			}
			return
		}
		l.adoptPulsarTrack(o, id, uri, positionMS, title)
		return
	}

	l.notify(o, fmt.Sprintf("дом %s вмешался с телефона — эфир восстановлен", l.peerName(o, id)))
	if o.sess.State == session.StatePlaying {
		l.apply(o, o.sess.CmdSync())
		return
	}
	l.hub.Send(key, protocol.TypeStop, &protocol.StopPayload{})
}

func (l *loop) adoptPulsarTrack(
	o *orbitState, id protocol.NodeID, uri string, positionMS int64, title string,
) {
	requestedBy := string(id)
	if orbitID, _, ok := splitComposite(id); ok {
		requestedBy = l.orbit(orbitID).title
	}
	el := l.newTrackElement(uri, requestedBy)
	el.Title = title
	l.st.InsertElement(el)
	l.log.Info("adopting Pulsar playback into shared air",
		"orbit", o.id, "peer", id, "uri", uri, "position_ms", positionMS)
	if o.sess.State == session.StateVoice {
		// A voice insert is an accepted FIFO promise. A phone selection during
		// it becomes the next music item instead of cutting the sender off.
		l.apply(o, o.sess.EnqueueTrackAfterVoices(el))
		l.notify(o, fmt.Sprintf("%s сыграет сразу после голосовых", trackLabel(el)))
		return
	}
	if !seamlessAdoptionReady(o, id) {
		// Rolling upgrade compatibility: pre-capability nodes decode the new
		// optional fields but interpret load as the old pause/reload command. A
		// leader would then remain paused because seamless mode intentionally
		// sends no resume_at to it. Use the old all-node barrier until every
		// currently participating peer has announced support. Offline homes do
		// not need adoption semantics: they use the ordinary catch-up path when
		// they return.
		l.log.Info("seamless adoption unavailable; using legacy barrier", "orbit", o.id, "leader", id)
		l.apply(o, o.sess.CmdPlayNowAt(el, positionMS))
		return
	}
	l.apply(o, o.sess.CmdAdoptPlaying(time.Now().UnixMilli(), id, el, positionMS))
}

func seamlessAdoptionReady(o *orbitState, leader protocol.NodeID) bool {
	if !o.seamless[leader] {
		return false
	}
	for _, peer := range o.sess.Peers {
		if (peer == leader || o.sess.IsOnline(peer)) && !o.seamless[peer] {
			return false
		}
	}
	return true
}
