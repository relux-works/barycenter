package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
)

type WindowsIdentitySnapshot struct {
	Operation              ShellIdentityOperation
	FailureCode            string
	RecoveryExportRequired bool
}

type windowsIdentityServicing interface {
	Create(context.Context, string, string) (CreateOrbitResult, error)
	Join(context.Context, string) (JoinOrbitResult, error)
	RotateStoredRecovery(context.Context) (*RecoveryMaterial, error)
	AcknowledgeRecoveryBackup(int64, string) error
}

type windowsRecoveryExporting interface {
	SaveSelectedDestination(string, *RecoveryMaterial) error
}

// WindowsIdentityComposition keeps one installation attempt stable across a
// retry. Protected credentials activate immediately; recovery export remains a
// resumable safety action and rotates fresh one-time material after restart.
type WindowsIdentityComposition struct {
	mu              sync.RWMutex
	service         windowsIdentityServicing
	exporter        windowsRecoveryExporting
	ctx             context.Context
	cancel          context.CancelFunc
	onActive        func()
	snapshot        WindowsIdentitySnapshot
	busy            bool
	attemptTitle    string
	attemptID       string
	pendingRecovery *RecoveryMaterial
}

func NewWindowsIdentityComposition(service windowsIdentityServicing, exporter windowsRecoveryExporting, onActive func()) (*WindowsIdentityComposition, error) {
	if service == nil || exporter == nil {
		return nil, errOnboardingServiceIncoherent
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &WindowsIdentityComposition{service: service, exporter: exporter, ctx: ctx, cancel: cancel, onActive: onActive, snapshot: WindowsIdentitySnapshot{Operation: ShellIdentityIdle}}, nil
}

func newProductionWindowsIdentityComposition(dir, coordinatorBase string, onActive func()) (*WindowsIdentityComposition, error) {
	repository, err := newDefaultCredentialRepository(dir)
	if err != nil {
		return nil, err
	}
	client, err := NewOnboardingClient(coordinatorBase, nil)
	if err != nil {
		return nil, err
	}
	recovery, err := NewRecoveryService(repository, client, nil)
	if err != nil {
		return nil, err
	}
	service, err := NewWindowsOnboardingService(client, repository, recovery)
	if err != nil {
		return nil, err
	}
	return NewWindowsIdentityComposition(service, NewRecoveryExporter(), onActive)
}

func (c *WindowsIdentityComposition) Snapshot() WindowsIdentitySnapshot {
	if c == nil {
		return WindowsIdentitySnapshot{Operation: ShellIdentityFailed, FailureCode: "identity_unavailable"}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snapshot
}

func (c *WindowsIdentityComposition) ApplyShellSnapshot(shell *ShellSnapshot) {
	if shell == nil {
		return
	}
	state := c.Snapshot()
	if state.Operation == ShellIdentityIdle && shell.Connection != ShellUnpaired {
		return
	}
	shell.IdentityOperation = state.Operation
	shell.IdentityFailure = state.FailureCode
	shell.RecoveryExportRequired = state.RecoveryExportRequired
}

func (c *WindowsIdentityComposition) Create(title string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.busy || c.ctx.Err() != nil {
		c.mu.Unlock()
		return
	}
	if c.attemptTitle != title || c.attemptID == "" {
		attempt, err := newWindowsInstallationAttemptID()
		if err != nil {
			c.snapshot = WindowsIdentitySnapshot{Operation: ShellIdentityFailed, FailureCode: "identity_random_failed"}
			c.mu.Unlock()
			return
		}
		c.attemptTitle, c.attemptID = title, attempt
	}
	attempt := c.attemptID
	c.busy = true
	c.snapshot = WindowsIdentitySnapshot{Operation: ShellIdentityWorking}
	c.mu.Unlock()
	go func() {
		result, err := c.service.Create(c.ctx, title, attempt)
		c.mu.Lock()
		c.busy = false
		if result.Recovery != nil {
			c.pendingRecovery = result.Recovery
		}
		if err != nil {
			c.snapshot = WindowsIdentitySnapshot{Operation: ShellIdentityRecoveryRequired, FailureCode: windowsIdentityFailureCode(err), RecoveryExportRequired: result.Recovery != nil}
			if result.Recovery == nil {
				c.snapshot.Operation = ShellIdentityFailed
			}
			c.mu.Unlock()
			return
		}
		if result.Recovery == nil {
			c.snapshot = WindowsIdentitySnapshot{Operation: ShellIdentityFailed, FailureCode: windowsIdentityFailureCode(err)}
			c.mu.Unlock()
			return
		}
		c.attemptTitle, c.attemptID = "", ""
		c.snapshot = WindowsIdentitySnapshot{Operation: ShellIdentityRecoveryRequired, RecoveryExportRequired: true}
		onActive := c.onActive
		c.mu.Unlock()
		if onActive != nil {
			onActive()
		}
	}()
}

func (c *WindowsIdentityComposition) Join(invite string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.busy || c.ctx.Err() != nil {
		c.mu.Unlock()
		return
	}
	c.busy = true
	c.snapshot = WindowsIdentitySnapshot{Operation: ShellIdentityWorking}
	c.mu.Unlock()
	go func() {
		_, err := c.service.Join(c.ctx, invite)
		c.mu.Lock()
		c.busy = false
		if err != nil {
			c.snapshot = WindowsIdentitySnapshot{Operation: ShellIdentityFailed, FailureCode: windowsIdentityFailureCode(err)}
			c.mu.Unlock()
			return
		}
		c.snapshot = WindowsIdentitySnapshot{Operation: ShellIdentityActive}
		onActive := c.onActive
		c.mu.Unlock()
		if onActive != nil {
			onActive()
		}
	}()
}

func (c *WindowsIdentityComposition) SaveRecovery(path string) {
	if c == nil || path == "" {
		return
	}
	c.mu.Lock()
	if c.busy || c.ctx.Err() != nil {
		c.mu.Unlock()
		return
	}
	material := c.pendingRecovery
	c.busy = true
	c.snapshot = WindowsIdentitySnapshot{Operation: ShellIdentityWorking, RecoveryExportRequired: true}
	c.mu.Unlock()
	go func() {
		if material == nil {
			var err error
			material, err = c.service.RotateStoredRecovery(c.ctx)
			if err != nil || material == nil {
				c.mu.Lock()
				c.busy = false
				c.snapshot = WindowsIdentitySnapshot{
					Operation:              ShellIdentityRecoveryRequired,
					FailureCode:            windowsIdentityFailureCode(err),
					RecoveryExportRequired: true,
				}
				c.mu.Unlock()
				return
			}
			c.mu.Lock()
			if c.ctx.Err() != nil {
				c.mu.Unlock()
				material.discard()
				return
			}
			c.pendingRecovery = material
			c.mu.Unlock()
		}
		actorID, recoveryID, ok := material.metadata()
		err := c.exporter.SaveSelectedDestination(path, material)
		if err == nil && ok {
			err = c.service.AcknowledgeRecoveryBackup(actorID, recoveryID)
		}
		c.mu.Lock()
		c.busy = false
		if err != nil || !ok {
			c.snapshot = WindowsIdentitySnapshot{Operation: ShellIdentityRecoveryRequired, FailureCode: "recovery_export_failed", RecoveryExportRequired: true}
			c.mu.Unlock()
			return
		}
		material.discard()
		c.pendingRecovery = nil
		c.snapshot = WindowsIdentitySnapshot{Operation: ShellIdentityActive}
		c.mu.Unlock()
	}()
}

func (c *WindowsIdentityComposition) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	if c.pendingRecovery != nil {
		c.pendingRecovery.discard()
		c.pendingRecovery = nil
	}
	c.mu.Unlock()
}

func newWindowsInstallationAttemptID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "windows-create-" + hex.EncodeToString(raw), nil
}

func windowsIdentityFailureCode(err error) string {
	if err == nil {
		return "invalid_identity_response"
	}
	var client *OnboardingClientError
	if errors.As(err, &client) {
		return string(client.Kind)
	}
	if errors.Is(err, errCredentialStorageConflict) || errors.Is(err, errCredentialStorageUnavailable) {
		return "protected_storage_failed"
	}
	return "identity_unavailable"
}
