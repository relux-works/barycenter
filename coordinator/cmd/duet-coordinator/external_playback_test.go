package main

import (
	"testing"

	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
)

// An idle together session has no conflict to arbitrate: selecting a track
// on any Pulsar starts the shared air even when busy-air policy is coordinator.
func TestIdleTogetherAdoptsPulsarPlayback(t *testing.T) {
	l, fake := newTestLoop(t)
	o := l.orbit(1)
	o.takeoverPolicy = "coordinator"
	l.handleExternalPlayback(o, hub.NodeKey{Orbit: 1, Slot: "a"}, "spotify:track:X", 42_000)

	loads := fake.ofType(protocol.TypeLoad)
	if len(loads) != 2 {
		t.Fatalf("selected track must load every home: %+v", fake.sent)
	}
	for _, m := range loads {
		load := m.payload.(*protocol.LoadPayload)
		if load.URI != "spotify:track:X" || load.PositionMS != 42_000 {
			t.Fatalf("load = %+v", load)
		}
	}
	if o.sess.Mode != session.ModeShared {
		t.Fatalf("Pulsar selection must stay together, mode=%s", o.sess.Mode)
	}
}

func TestUserPolicyAdoptsNewPulsarSelectionDuringBusyAir(t *testing.T) {
	l, fake := newTestLoop(t)
	o := l.orbit(1)
	l.apply(o, o.sess.EnqueueTrack(l.newTrackElement("spotify:track:old", "test")))
	fake.drain()

	l.handleExternalPlayback(o, hub.NodeKey{Orbit: 1, Slot: "a"}, "spotify:track:new", 12_345)

	if o.sess.Current == nil || o.sess.Current.URI != "spotify:track:new" {
		t.Fatalf("phone selection did not replace busy air: %+v", o.sess.Current)
	}
	loads := fake.ofType(protocol.TypeLoad)
	if len(loads) != 2 {
		t.Fatalf("replacement must reload every home: %+v", fake.sent)
	}
	for _, m := range loads {
		if got := m.payload.(*protocol.LoadPayload).PositionMS; got != 12_345 {
			t.Fatalf("replacement position = %d, want 12345", got)
		}
	}
}

func TestCoordinatorPolicyProtectsBusyAirFromPulsarSelection(t *testing.T) {
	l, fake := newTestLoop(t)
	o := l.orbit(1)
	o.takeoverPolicy = "coordinator"
	l.apply(o, o.sess.EnqueueTrack(l.newTrackElement("spotify:track:old", "test")))
	fake.drain()

	l.handleExternalPlayback(o, hub.NodeKey{Orbit: 1, Slot: "a"}, "spotify:track:new", 12_345)

	if o.sess.Current == nil || o.sess.Current.URI != "spotify:track:old" {
		t.Fatalf("coordinator policy lost the broadcast: %+v", o.sess.Current)
	}
	if got := fake.ofType(protocol.TypeStop); len(got) != 1 || got[0].key.Slot != "a" {
		t.Fatalf("interfering Pulsar must be stopped while old air loads: %+v", fake.sent)
	}
}

func TestTogetherIgnoresNonTrackSpotifyPlayback(t *testing.T) {
	for _, uri := range []string{"spotify:episode:podcast", "spotify:track:"} {
		l, fake := newTestLoop(t)
		o := l.orbit(1)
		l.handleExternalPlayback(o, hub.NodeKey{Orbit: 1, Slot: "a"}, uri, 12_345)
		if o.sess.Current != nil || len(fake.ofType(protocol.TypeLoad)) != 0 {
			t.Fatalf("only Spotify tracks can become shared elements: uri=%q current=%+v sent=%+v",
				uri, o.sess.Current, fake.sent)
		}
	}
}

// Selecting music on one Pulsar is the primary together-mode control surface:
// no Spotify link is sent to Telegram, and the linked friend's homes join at
// the initiating home's audible position through the normal ready barrier.
func TestApproachAdoptsPulsarPlaybackWithoutBotLink(t *testing.T) {
	l, fake, friendOrbit := twoOrbitLoop(t)
	r := &replies{}
	proposeAndAwait(t, l, r)
	l.handleBot(cmdEvent(t, "a", "/accept", r))
	fake.drain()

	positionMS := int64(31_500)
	l.handleNodeMessage(hub.EvMessage{
		Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeA},
		Payload: &protocol.ExternalPlaybackPayload{
			URI: "spotify:track:phone-selection", PositionMS: &positionMS,
		},
	})

	g := l.stateFor(1)
	if g.sess.Mode != session.ModeShared || g.sess.Current == nil ||
		g.sess.Current.URI != "spotify:track:phone-selection" {
		t.Fatalf("group did not adopt Pulsar playback: mode=%s current=%+v", g.sess.Mode, g.sess.Current)
	}
	loads := fake.ofType(protocol.TypeLoad)
	if len(loads) != 3 {
		t.Fatalf("selected track must fan out across both orbits: %+v", fake.sent)
	}
	seenFriend := false
	for _, m := range loads {
		load := m.payload.(*protocol.LoadPayload)
		if load.PositionMS != positionMS {
			t.Fatalf("load position = %d, want %d", load.PositionMS, positionMS)
		}
		seenFriend = seenFriend || m.key.Orbit == friendOrbit
	}
	if !seenFriend {
		t.Fatal("linked friend's Pulsar did not receive the selected track")
	}
}
