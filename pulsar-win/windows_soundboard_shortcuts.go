package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const windowsSoundboardPreferenceVersion = 1

type WindowsSoundboardShortcutBinding struct {
	CueID    string                   `json:"cue_id"`
	Shortcut WindowsRecordingShortcut `json:"shortcut"`
}

type WindowsSoundboardPreferences struct {
	Version       int                                `json:"version"`
	SelectedCueID string                             `json:"selected_cue_id,omitempty"`
	Route         PhaseOneRoute                      `json:"route"`
	Delivery      PhaseOneDelivery                   `json:"delivery"`
	IncludeOrigin bool                               `json:"include_origin"`
	Shortcuts     []WindowsSoundboardShortcutBinding `json:"shortcuts"`
}

func DefaultWindowsSoundboardPreferences() WindowsSoundboardPreferences {
	return WindowsSoundboardPreferences{Version: windowsSoundboardPreferenceVersion,
		Route: PhaseOneOwnBarycenter, Delivery: PhaseOneOverlay, IncludeOrigin: true}
}

func (p WindowsSoundboardPreferences) valid() bool {
	if p.Version != windowsSoundboardPreferenceVersion || !validPhaseOneRoute(p.Route) ||
		!validPhaseOneDelivery(p.Delivery) || len(p.Shortcuts) > 16 ||
		(p.SelectedCueID != "" && !validPhaseOnePublicID(p.SelectedCueID, "cq_")) {
		return false
	}
	cues, keys := map[string]bool{}, map[WindowsRecordingShortcut]bool{}
	for _, binding := range p.Shortcuts {
		if !validPhaseOnePublicID(binding.CueID, "cq_") || !binding.Shortcut.Valid() ||
			cues[binding.CueID] || keys[binding.Shortcut] {
			return false
		}
		cues[binding.CueID], keys[binding.Shortcut] = true, true
	}
	return true
}

type WindowsSoundboardPreferenceStore struct{ Path string }

func (s WindowsSoundboardPreferenceStore) Load() WindowsSoundboardPreferences {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return DefaultWindowsSoundboardPreferences()
	}
	var result WindowsSoundboardPreferences
	if json.Unmarshal(data, &result) != nil || !result.valid() {
		return DefaultWindowsSoundboardPreferences()
	}
	return result
}

func (s WindowsSoundboardPreferenceStore) Save(value WindowsSoundboardPreferences) error {
	if s.Path == "" || !value.valid() {
		return ErrWindowsShortcutUnavailable
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temporary := s.Path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, s.Path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

type WindowsSoundboardShortcutState struct {
	CueID    string
	Shortcut WindowsRecordingShortcut
	Status   WindowsRecordingShortcutStatus
}

// WindowsSoundboardShortcutController uses RegisterHotKey only. It never
// installs a keyboard hook and therefore cannot observe or capture unrelated
// keystrokes. A failed registration leaves the always-available button path.
type WindowsSoundboardShortcutController struct {
	registrar  WindowsRecordingShortcutRegistrar
	trigger    func(string)
	recording  WindowsRecordingShortcut
	configured []WindowsSoundboardShortcutBinding
	states     []WindowsSoundboardShortcutState
	registered map[WindowsShortcutRegistration]string
	suspended  map[WindowsShortcutSuspension]bool
	started    bool
}

func NewWindowsSoundboardShortcutController(registrar WindowsRecordingShortcutRegistrar,
	recording WindowsRecordingShortcut, bindings []WindowsSoundboardShortcutBinding, trigger func(string)) *WindowsSoundboardShortcutController {
	result := &WindowsSoundboardShortcutController{registrar: registrar, recording: recording,
		trigger: trigger, registered: map[WindowsShortcutRegistration]string{}, suspended: map[WindowsShortcutSuspension]bool{}}
	result.Configure(bindings)
	return result
}

func (c *WindowsSoundboardShortcutController) Configure(bindings []WindowsSoundboardShortcutBinding) {
	c.stopRegistrations()
	c.configured = nil
	if len(bindings) > 16 {
		bindings = bindings[:16]
	}
	seenCue, seenKey := map[string]bool{}, map[WindowsRecordingShortcut]bool{}
	for _, binding := range bindings {
		if !validPhaseOnePublicID(binding.CueID, "cq_") || !binding.Shortcut.Valid() || seenCue[binding.CueID] || seenKey[binding.Shortcut] {
			continue
		}
		seenCue[binding.CueID], seenKey[binding.Shortcut] = true, true
		c.configured = append(c.configured, binding)
	}
	if c.started {
		c.registerConfigured()
	}
}

func (c *WindowsSoundboardShortcutController) States() []WindowsSoundboardShortcutState {
	return append([]WindowsSoundboardShortcutState(nil), c.states...)
}

func (c *WindowsSoundboardShortcutController) Start() {
	c.started = true
	c.registerConfigured()
}

func (c *WindowsSoundboardShortcutController) Suspend(reason WindowsShortcutSuspension) {
	if reason == "" || c.suspended[reason] {
		return
	}
	c.suspended[reason] = true
	c.stopRegistrations()
	c.states = c.states[:0]
	for _, binding := range c.configured {
		c.states = append(c.states, WindowsSoundboardShortcutState{binding.CueID, binding.Shortcut, WindowsShortcutSuspended})
	}
}

func (c *WindowsSoundboardShortcutController) Resume(reason WindowsShortcutSuspension) {
	delete(c.suspended, reason)
	if len(c.suspended) == 0 {
		c.registerConfigured()
	}
}

func (c *WindowsSoundboardShortcutController) Stop() {
	c.stopRegistrations()
	c.states = nil
	c.started = false
}

func (c *WindowsSoundboardShortcutController) HandleHotKey(registration WindowsShortcutRegistration) bool {
	cueID := c.registered[registration]
	if cueID == "" || len(c.suspended) != 0 {
		return false
	}
	if c.trigger != nil {
		c.trigger(cueID)
	}
	return true
}

func (c *WindowsSoundboardShortcutController) registerConfigured() {
	if len(c.suspended) != 0 {
		return
	}
	c.stopRegistrations()
	c.states = c.states[:0]
	for _, binding := range c.configured {
		state := WindowsSoundboardShortcutState{CueID: binding.CueID, Shortcut: binding.Shortcut}
		if binding.Shortcut == c.recording {
			state.Status = WindowsShortcutConflict
		} else if c.registrar == nil {
			state.Status = WindowsShortcutUnavailable
		} else if registration, err := c.registrar.Register(binding.Shortcut); err != nil || registration == 0 {
			if errors.Is(err, ErrWindowsShortcutConflict) {
				state.Status = WindowsShortcutConflict
			} else {
				state.Status = WindowsShortcutUnavailable
			}
		} else {
			state.Status = WindowsShortcutRegistered
			c.registered[registration] = binding.CueID
		}
		c.states = append(c.states, state)
	}
}

func (c *WindowsSoundboardShortcutController) stopRegistrations() {
	for registration := range c.registered {
		if c.registrar != nil {
			_ = c.registrar.Unregister(registration)
		}
		delete(c.registered, registration)
	}
}
