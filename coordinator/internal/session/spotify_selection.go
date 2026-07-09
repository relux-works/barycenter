package session

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
