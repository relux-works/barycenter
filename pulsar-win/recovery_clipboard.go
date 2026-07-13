package main

import (
	"errors"
	"sync"
	"time"
)

const maximumRecoveryClipboardTTL = 300 * time.Second
const recoveryClipboardRetryDelay = time.Second

var recoveryClipboardRetryDelays = [...]time.Duration{
	recoveryClipboardRetryDelay,
	5 * time.Second,
	30 * time.Second,
}

var windowsClipboardExclusionFormats = []string{
	"ExcludeClipboardContentFromMonitorProcessing",
	"CanIncludeInClipboardHistory",
	"CanUploadToCloudClipboard",
}

const windowsClipboardExclusionDWORD uint32 = 0

type UIClipboardDispatcher interface {
	Invoke(func() error) error
}

type clipboardTimer interface{ Stop() bool }

type clipboardScheduler interface {
	AfterFunc(time.Duration, func()) clipboardTimer
}

type systemClipboardScheduler struct{}

func (systemClipboardScheduler) AfterFunc(delay time.Duration, callback func()) clipboardTimer {
	return time.AfterFunc(delay, callback)
}

type clipboardPublication struct {
	Sequence uint32
	Changed  bool
	Exposed  bool
}

type recoveryClipboardBackend interface {
	Publish(owner uintptr, payload string) (clipboardPublication, error)
	ClearIfUnchanged(owner uintptr, sequence uint32, payload string) (bool, error)
}

type recoveryClipboardLease struct {
	id             uint64
	sequence       uint32
	payload        string
	nextRetryIndex int
	timer          clipboardTimer
}

// RecoveryClipboardCleanupStatus exposes only generic lease health; it never
// contains recovery material, a lease identifier, clipboard content, or paths.
type RecoveryClipboardCleanupStatus string

const (
	RecoveryClipboardCleanupIdle            RecoveryClipboardCleanupStatus = "idle"
	RecoveryClipboardCleanupLeased          RecoveryClipboardCleanupStatus = "leased"
	RecoveryClipboardAutomaticCleanupFailed RecoveryClipboardCleanupStatus = "automatic_cleanup_failed"
)

func (s RecoveryClipboardCleanupStatus) String() string {
	return "RecoveryClipboardCleanupStatus(" + string(s) + ")"
}

func (s RecoveryClipboardCleanupStatus) GoString() string { return s.String() }

// RecoveryClipboard is explicit-only. Construction requires a real owner HWND
// and the future UI's dispatcher; it never calls OpenClipboard(NULL).
type RecoveryClipboard struct {
	// operationMu serializes publish/compare-and-clear operations without holding
	// the short-lived state mutex while a synchronous UI dispatcher is awaited.
	operationMu sync.Mutex
	mu          sync.Mutex
	owner       uintptr
	dispatcher  UIClipboardDispatcher
	backend     recoveryClipboardBackend
	scheduler   clipboardScheduler
	nextID      uint64
	current     *recoveryClipboardLease
	status      RecoveryClipboardCleanupStatus
}

func (c *RecoveryClipboard) String() string   { return "RecoveryClipboard{<redacted>}" }
func (c *RecoveryClipboard) GoString() string { return c.String() }

func NewRecoveryClipboard(owner uintptr, dispatcher UIClipboardDispatcher) (*RecoveryClipboard, error) {
	backend, err := newPlatformRecoveryClipboardBackend()
	if err != nil {
		return nil, err
	}
	return newRecoveryClipboard(owner, dispatcher, backend, systemClipboardScheduler{})
}

func newRecoveryClipboard(owner uintptr, dispatcher UIClipboardDispatcher, backend recoveryClipboardBackend, scheduler clipboardScheduler) (*RecoveryClipboard, error) {
	if owner == 0 || dispatcher == nil || backend == nil || scheduler == nil {
		return nil, errClipboardOwnerRequired
	}
	return &RecoveryClipboard{
		owner: owner, dispatcher: dispatcher, backend: backend, scheduler: scheduler,
		status: RecoveryClipboardCleanupIdle,
	}, nil
}

// CleanupStatus lets the owning UI surface a terminal automatic cleanup failure
// and offer an explicit retry without exposing the leased payload.
func (c *RecoveryClipboard) CleanupStatus() RecoveryClipboardCleanupStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

func (c *RecoveryClipboard) Copy(material *RecoveryMaterial, ttl time.Duration) (uint64, error) {
	if material == nil || ttl <= 0 || ttl > maximumRecoveryClipboardTTL {
		return 0, errClipboardCopyFailed
	}
	payloadBytes, err := material.exportJSON()
	if err != nil {
		return 0, err
	}
	payload := string(payloadBytes)
	zeroBytes(payloadBytes)

	c.operationMu.Lock()
	defer c.operationMu.Unlock()
	var publication clipboardPublication
	err = c.dispatcher.Invoke(func() error {
		var publishErr error
		publication, publishErr = c.backend.Publish(c.owner, payload)
		return publishErr
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	if publication.Changed && c.current != nil {
		c.current.timer.Stop()
		c.current = nil
		c.status = RecoveryClipboardCleanupIdle
	}
	if err != nil && !publication.Exposed {
		return 0, errClipboardCopyFailed
	}
	if !publication.Exposed {
		return 0, errClipboardCopyFailed
	}
	c.nextID++
	lease := &recoveryClipboardLease{id: c.nextID, sequence: publication.Sequence, payload: payload}
	leaseID := lease.id
	lease.timer = c.scheduler.AfterFunc(ttl, func() { c.expire(leaseID) })
	c.current = lease
	c.status = RecoveryClipboardCleanupLeased
	if err != nil {
		return lease.id, errClipboardCopyFailed
	}
	return lease.id, nil
}

func (c *RecoveryClipboard) Clear() error {
	c.mu.Lock()
	if c.current == nil {
		c.mu.Unlock()
		return nil
	}
	id := c.current.id
	c.mu.Unlock()
	return c.clearLease(id)
}

func (c *RecoveryClipboard) expire(id uint64) { _ = c.clearLease(id) }

func (c *RecoveryClipboard) clearLease(id uint64) error {
	c.operationMu.Lock()
	defer c.operationMu.Unlock()
	c.mu.Lock()
	lease := c.current
	if lease == nil || lease.id != id {
		c.mu.Unlock()
		return nil
	}
	sequence, payload := lease.sequence, lease.payload
	c.mu.Unlock()
	var cleared bool
	err := c.dispatcher.Invoke(func() error {
		var clearErr error
		cleared, clearErr = c.backend.ClearIfUnchanged(c.owner, sequence, payload)
		return clearErr
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	lease = c.current
	if lease == nil || lease.id != id {
		return nil
	}
	if err != nil {
		lease.timer.Stop()
		if lease.nextRetryIndex >= len(recoveryClipboardRetryDelays) {
			c.status = RecoveryClipboardAutomaticCleanupFailed
			return errClipboardClearFailed
		}
		delay := recoveryClipboardRetryDelays[lease.nextRetryIndex]
		lease.nextRetryIndex++
		lease.timer = c.scheduler.AfterFunc(delay, func() { c.expire(id) })
		return errClipboardClearFailed
	}
	// Whether cleared or replaced externally, this lease no longer owns data.
	lease.timer.Stop()
	c.current = nil
	c.status = RecoveryClipboardCleanupIdle
	_ = cleared
	return nil
}

var (
	errClipboardOwnerRequired = errors.New("clipboard requires a non-null owner window and UI dispatcher")
	errClipboardCopyFailed    = errors.New("clipboard copy failed")
	errClipboardClearFailed   = errors.New("clipboard clear failed")
	errClipboardUnsupported   = errors.New("clipboard is unavailable on this platform")
)
