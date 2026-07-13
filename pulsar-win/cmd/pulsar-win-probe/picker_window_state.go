package main

// pickerWindowState owns restoration of the tray-only hidden state across
// both synchronous initiation failures and asynchronous picker completion.
type pickerWindowState struct {
	operationStarted bool
	restoreHidden    bool
}

func (s *pickerWindowState) begin(wasHidden bool) {
	s.operationStarted = false
	s.restoreHidden = wasHidden
}

func (s *pickerWindowState) initiated(success bool) (rehide bool) {
	if success {
		s.operationStarted = true
		return false
	}
	rehide = s.restoreHidden
	*s = pickerWindowState{}
	return rehide
}

func (s *pickerWindowState) complete() (rehide bool) {
	if !s.operationStarted {
		return false
	}
	rehide = s.restoreHidden
	*s = pickerWindowState{}
	return rehide
}
