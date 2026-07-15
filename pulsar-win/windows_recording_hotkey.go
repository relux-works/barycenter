package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
)

const (
	WindowsShortcutModAlt     uint32 = 0x0001
	WindowsShortcutModControl uint32 = 0x0002
	WindowsShortcutModShift   uint32 = 0x0004
	WindowsShortcutModWin     uint32 = 0x0008
	WindowsShortcutVKSpace    uint32 = 0x20
	WindowsShortcutVKR        uint32 = 0x52
)

var (
	ErrWindowsShortcutConflict    = errors.New("windows_recording_shortcut_conflict")
	ErrWindowsShortcutUnavailable = errors.New("windows_recording_shortcut_unavailable")
)

type WindowsRecordingShortcut struct {
	VirtualKey uint32 `json:"virtualKey"`
	Modifiers  uint32 `json:"modifiers"`
}

func DefaultWindowsRecordingShortcut() WindowsRecordingShortcut {
	return WindowsRecordingShortcut{
		VirtualKey: WindowsShortcutVKSpace,
		Modifiers:  WindowsShortcutModControl | WindowsShortcutModShift,
	}
}

func (s WindowsRecordingShortcut) Valid() bool {
	const supported = WindowsShortcutModAlt | WindowsShortcutModControl |
		WindowsShortcutModShift | WindowsShortcutModWin
	if s.Modifiers == 0 || s.Modifiers&^supported != 0 {
		return false
	}
	return s.VirtualKey == WindowsShortcutVKSpace ||
		(s.VirtualKey >= '0' && s.VirtualKey <= '9') ||
		(s.VirtualKey >= 'A' && s.VirtualKey <= 'Z') ||
		(s.VirtualKey >= 0x70 && s.VirtualKey <= 0x87) // F1...F24
}

type WindowsRecordingShortcutStatus string

const (
	WindowsShortcutInactive    WindowsRecordingShortcutStatus = "inactive"
	WindowsShortcutRegistered  WindowsRecordingShortcutStatus = "registered"
	WindowsShortcutConflict    WindowsRecordingShortcutStatus = "conflict"
	WindowsShortcutUnavailable WindowsRecordingShortcutStatus = "unavailable"
	WindowsShortcutSuspended   WindowsRecordingShortcutStatus = "suspended"
)

type WindowsShortcutSuspension string

const (
	WindowsShortcutSessionLocked WindowsShortcutSuspension = "session_locked"
	WindowsShortcutSystemSuspend WindowsShortcutSuspension = "system_suspend"
)

type WindowsShortcutRegistration uint32

type WindowsRecordingShortcutRegistrar interface {
	Register(WindowsRecordingShortcut) (WindowsShortcutRegistration, error)
	Unregister(WindowsShortcutRegistration) error
}

// WindowsRecordingShortcutController is called only by the Win32 window
// owner thread. The production registrar therefore registers, reconfigures
// and releases RegisterHotKey ownership on the same message queue.
type WindowsRecordingShortcutController struct {
	registrar WindowsRecordingShortcutRegistrar
	toggle    func()

	shortcut     WindowsRecordingShortcut
	status       WindowsRecordingShortcutStatus
	registration WindowsShortcutRegistration
	generation   uint64
	suspensions  map[WindowsShortcutSuspension]bool
}

func NewWindowsRecordingShortcutController(registrar WindowsRecordingShortcutRegistrar, shortcut WindowsRecordingShortcut, toggle func()) *WindowsRecordingShortcutController {
	if !shortcut.Valid() {
		shortcut = DefaultWindowsRecordingShortcut()
	}
	return &WindowsRecordingShortcutController{
		registrar: registrar, shortcut: shortcut, toggle: toggle,
		status:      WindowsShortcutInactive,
		suspensions: make(map[WindowsShortcutSuspension]bool),
	}
}

func (c *WindowsRecordingShortcutController) Shortcut() WindowsRecordingShortcut     { return c.shortcut }
func (c *WindowsRecordingShortcutController) Status() WindowsRecordingShortcutStatus { return c.status }

func (c *WindowsRecordingShortcutController) Start() { c.registerConfigured() }

func (c *WindowsRecordingShortcutController) Reconfigure(shortcut WindowsRecordingShortcut) bool {
	if !shortcut.Valid() {
		return false
	}
	if !c.unregisterCurrent() {
		return false
	}
	c.shortcut = shortcut
	if len(c.suspensions) != 0 {
		c.status = WindowsShortcutSuspended
		return true
	}
	c.registerConfigured()
	return true
}

func (c *WindowsRecordingShortcutController) Suspend(reason WindowsShortcutSuspension) {
	if reason == "" || c.suspensions[reason] {
		return
	}
	c.suspensions[reason] = true
	if !c.unregisterCurrent() {
		return
	}
	c.status = WindowsShortcutSuspended
}

func (s WindowsRecordingShortcut) Label() string {
	label := ""
	appendPart := func(part string) {
		if label != "" {
			label += "+"
		}
		label += part
	}
	if s.Modifiers&WindowsShortcutModControl != 0 {
		appendPart("Ctrl")
	}
	if s.Modifiers&WindowsShortcutModAlt != 0 {
		appendPart("Alt")
	}
	if s.Modifiers&WindowsShortcutModShift != 0 {
		appendPart("Shift")
	}
	if s.Modifiers&WindowsShortcutModWin != 0 {
		appendPart("Win")
	}
	if s.VirtualKey == WindowsShortcutVKSpace {
		appendPart("Space")
	} else if s.VirtualKey >= 0x70 && s.VirtualKey <= 0x87 {
		value := int(s.VirtualKey-0x70) + 1
		appendPart("F" + strconv.Itoa(value))
	} else if s.VirtualKey >= '0' && s.VirtualKey <= 'Z' {
		appendPart(string(rune(s.VirtualKey)))
	}
	return label
}

func (c *WindowsRecordingShortcutController) Resume(reason WindowsShortcutSuspension) {
	if !c.suspensions[reason] {
		return
	}
	delete(c.suspensions, reason)
	if len(c.suspensions) == 0 {
		c.registerConfigured()
	}
}

func (c *WindowsRecordingShortcutController) Stop() {
	clear(c.suspensions)
	if c.unregisterCurrent() {
		c.status = WindowsShortcutInactive
	}
}

func (c *WindowsRecordingShortcutController) HandleHotKey(registration WindowsShortcutRegistration) bool {
	if registration == 0 || registration != c.registration ||
		c.status != WindowsShortcutRegistered || len(c.suspensions) != 0 {
		return false
	}
	if c.toggle != nil {
		c.toggle()
	}
	return true
}

func (c *WindowsRecordingShortcutController) registerConfigured() {
	if c.registrar == nil || len(c.suspensions) != 0 || !c.unregisterCurrent() {
		if len(c.suspensions) != 0 {
			c.status = WindowsShortcutSuspended
		} else {
			c.status = WindowsShortcutUnavailable
		}
		return
	}
	c.generation++
	registration, err := c.registrar.Register(c.shortcut)
	if err != nil || registration == 0 {
		if errors.Is(err, ErrWindowsShortcutConflict) {
			c.status = WindowsShortcutConflict
		} else {
			c.status = WindowsShortcutUnavailable
		}
		return
	}
	c.registration = registration
	c.status = WindowsShortcutRegistered
}

func (c *WindowsRecordingShortcutController) unregisterCurrent() bool {
	c.generation++
	if c.registration == 0 {
		return true
	}
	registration := c.registration
	if c.registrar == nil || c.registrar.Unregister(registration) != nil {
		c.status = WindowsShortcutUnavailable
		return false
	}
	c.registration = 0
	return true
}

type WindowsRecordingShortcutStore struct{ Path string }

func (s WindowsRecordingShortcutStore) Load() WindowsRecordingShortcut {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return DefaultWindowsRecordingShortcut()
	}
	var shortcut WindowsRecordingShortcut
	if json.Unmarshal(data, &shortcut) != nil || !shortcut.Valid() {
		return DefaultWindowsRecordingShortcut()
	}
	return shortcut
}

func (s WindowsRecordingShortcutStore) Save(shortcut WindowsRecordingShortcut) error {
	if s.Path == "" || !shortcut.Valid() {
		return ErrWindowsShortcutUnavailable
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(shortcut)
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
