package main

import (
	"testing"

	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
)

func enableSeamlessForTest(o *orbitState) {
	for _, peer := range o.sess.Peers {
		o.seamless[peer] = true
	}
}

// An idle together session has no conflict to arbitrate: selecting a track
// on any Pulsar starts the shared air even when busy-air policy is coordinator.
func TestIdleTogetherAdoptsPulsarPlayback(t *testing.T) {
	l, fake := newTestLoop(t)
	o := l.orbit(1)
	enableSeamlessForTest(o)
	o.takeoverPolicy = "coordinator"
	l.handleExternalPlayback(o, hub.NodeKey{Orbit: 1, Slot: "a"}, "spotify:track:X", 42_000, "Artist — Track")

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
	if !loads[0].payload.(*protocol.LoadPayload).AdoptPlaying ||
		loads[1].payload.(*protocol.LoadPayload).AdoptPlaying {
		t.Fatalf("initiating home alone must adopt live playback: %+v", fake.sent)
	}
	if got := o.sess.Current.Title; got != "Artist — Track" {
		t.Fatalf("selection title = %q", got)
	}
	if o.sess.Mode != session.ModeShared {
		t.Fatalf("Pulsar selection must stay together, mode=%s", o.sess.Mode)
	}
}

func TestUserPolicyAdoptsNewPulsarSelectionDuringBusyAir(t *testing.T) {
	l, fake := newTestLoop(t)
	o := l.orbit(1)
	enableSeamlessForTest(o)
	l.apply(o, o.sess.EnqueueTrack(l.newTrackElement("spotify:track:old", "test")))
	fake.drain()

	l.handleExternalPlayback(o, hub.NodeKey{Orbit: 1, Slot: "a"}, "spotify:track:new", 12_345, "")

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

	l.handleExternalPlayback(o, hub.NodeKey{Orbit: 1, Slot: "a"}, "spotify:track:new", 12_345, "")

	if o.sess.Current == nil || o.sess.Current.URI != "spotify:track:old" {
		t.Fatalf("coordinator policy lost the broadcast: %+v", o.sess.Current)
	}
	if got := fake.ofType(protocol.TypeStop); len(got) != 1 || got[0].key.Slot != "a" {
		t.Fatalf("interfering Pulsar must be stopped while old air loads: %+v", fake.sent)
	}
}

func TestMixedVersionGroupUsesLegacyBarrier(t *testing.T) {
	l, fake := newTestLoop(t)
	o := l.orbit(1)
	o.seamless[protocol.NodeA] = true // b is still an older build
	l.apply(o, o.sess.EnqueueTrack(l.newTrackElement("spotify:track:old", "test")))
	fake.drain()

	l.handleExternalPlayback(o, hub.NodeKey{Orbit: 1, Slot: protocol.NodeA},
		"spotify:track:new", 12_345, "Artist — New")

	for _, m := range fake.ofType(protocol.TypeLoad) {
		if m.payload.(*protocol.LoadPayload).AdoptPlaying {
			t.Fatalf("mixed versions must not receive seamless semantics: %+v", fake.sent)
		}
	}
	if len(fake.ofType(protocol.TypePause)) != 2 {
		t.Fatalf("legacy barrier must remain safe during rollout: %+v", fake.sent)
	}
}

func TestOfflineOldPeerDoesNotBlockSeamlessLeader(t *testing.T) {
	l, fake := newTestLoop(t)
	o := l.orbit(1)
	o.seamless[protocol.NodeA] = true
	l.handleNode(hub.EvOffline{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeB}})
	fake.drain()

	l.handleExternalPlayback(o, hub.NodeKey{Orbit: 1, Slot: protocol.NodeA},
		"spotify:track:new", 12_345, "Artist — New")

	loads := fake.ofType(protocol.TypeLoad)
	if len(loads) != 1 || loads[0].key.Slot != protocol.NodeA ||
		!loads[0].payload.(*protocol.LoadPayload).AdoptPlaying {
		t.Fatalf("offline old peer must not stop the capable leader: %+v", fake.sent)
	}
	if len(fake.ofType(protocol.TypePause)) != 0 || o.sess.State != session.StatePlaying {
		t.Fatalf("leader did not stay live: state=%s sent=%+v", o.sess.State, fake.sent)
	}
}

func TestTogetherIgnoresNonTrackSpotifyPlayback(t *testing.T) {
	for _, uri := range []string{"spotify:episode:podcast", "spotify:track:"} {
		l, fake := newTestLoop(t)
		o := l.orbit(1)
		l.handleExternalPlayback(o, hub.NodeKey{Orbit: 1, Slot: "a"}, uri, 12_345, "")
		if o.sess.Current != nil || len(fake.ofType(protocol.TypeLoad)) != 0 {
			t.Fatalf("only Spotify tracks can become shared elements: uri=%q current=%+v sent=%+v",
				uri, o.sess.Current, fake.sent)
		}
	}
}

func TestPhoneSelectionDuringVoiceQueuesAfterVoiceInsteadOfCuttingIt(t *testing.T) {
	l, fake := newTestLoop(t)
	o := l.orbit(1)
	voice := session.Element{
		ID: "el_voice", Kind: session.KindVoice, MediaID: "m_voice",
		Target: "both", DurationMS: 5_000,
	}
	l.apply(o, o.sess.EnqueueVoice(voice))
	fake.drain()

	l.handleExternalPlayback(o, hub.NodeKey{Orbit: 1, Slot: "a"},
		"spotify:track:after-voice", 0, "Artist — After Voice")

	if o.sess.Current == nil || o.sess.Current.ID != "el_voice" {
		t.Fatalf("voice was preempted: current=%+v", o.sess.Current)
	}
	if o.sess.QueueLen() != 1 || o.sess.Queue[0].URI != "spotify:track:after-voice" {
		t.Fatalf("phone selection was not queued after voice: %+v", o.sess.Queue)
	}
	if len(fake.ofType(protocol.TypeLoad)) != 0 {
		t.Fatalf("voice must not be replaced by a load: %+v", fake.sent)
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
	g := l.stateFor(1)
	enableSeamlessForTest(g)

	positionMS := int64(31_500)
	l.handleNodeMessage(hub.EvMessage{
		Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeA},
		Payload: &protocol.ExternalPlaybackPayload{
			URI: "spotify:track:phone-selection", PositionMS: &positionMS,
			Title: "Phone Artist — Phone Track",
		},
	})

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
