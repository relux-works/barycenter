package session

import "relux.works/duet/coordinator/internal/protocol"

// CmdPlayNow keeps the Telegram /playnow path at the beginning of a track.
func (s *Session) CmdPlayNow(el Element) []Effect {
	return s.CmdPlayNowAt(el, 0)
}

// CmdPlayNowAt replaces the current shared element and synchronizes every
// participating home from positionMS. It is the link-free Spotify control
// path: selecting a track on one Pulsar becomes the shared-air leader event.
func (s *Session) CmdPlayNowAt(el Element, positionMS int64) []Effect {
	if s.Mode != ModeShared {
		return nil
	}
	var pre []Effect
	if s.Current != nil {
		pre = append(pre, EffElementDone{Element: *s.Current, Status: "skipped"})
		pre = append(pre, s.pauseBoth(300)...) // spec 7.3: fade 300 ms
		pre = append(pre, EffCancelReadyTimer{})
	}
	s.Queue = append([]Element{el}, s.Queue...)
	return append(pre, s.advanceAt(positionMS)...)
}

// CmdAdoptPlaying makes a Spotify selection that is already audible on
// leader the current shared element without stopping that leader. Other live
// homes load paused and later catch up at the leader's future position.
//
// This is deliberately separate from CmdPlayNowAt: a Telegram /playnow has no
// live leader and still needs the ordinary all-ready synchronized barrier.
func (s *Session) CmdAdoptPlaying(
	nowMS int64, leader protocol.NodeID, el Element, positionMS int64,
) []Effect {
	if s.Mode != ModeShared || !s.hasPeer(leader) {
		return nil
	}
	if positionMS < 0 {
		positionMS = 0
	}

	var effs []Effect
	if s.Current != nil {
		effs = append(effs, EffElementDone{Element: *s.Current, Status: "skipped"})
		effs = append(effs, EffCancelReadyTimer{})
	}

	s.resetElementTracking()
	s.Current = &el
	s.State = StateLoading
	s.SavedPositionMS = positionMS
	s.adoptionLeader = leader
	s.nodePos[leader] = positionMS
	s.nodePosAt[leader] = nowMS

	// Adoption is a living stream even for a strict orbit: the source is
	// already audible, so an offline peer must not make us stop it. Seal the
	// currently-online set explicitly; absent peers use the normal catch-up
	// path when they return.
	s.participants = map[protocol.NodeID]bool{}
	for _, n := range s.Peers {
		if s.online[n] || n == leader {
			s.participants[n] = true
		}
	}

	effs = append(effs, EffPersist{})
	for _, n := range s.Peers {
		if !s.counts(n) {
			continue
		}
		adopt := n == leader
		if adopt {
			s.ready[n] = true
		}
		effs = append(effs, EffLoad{
			To: n, ElementID: el.ID, URI: el.URI,
			PositionMS: positionMS, AdoptPlaying: adopt,
		})
	}
	effs = append(effs, EffArmReadyTimer{ElementID: el.ID})
	if armed := s.checkAllReady(nowMS, el.ID); armed != nil {
		effs = append(effs, armed...)
	}
	return effs
}
