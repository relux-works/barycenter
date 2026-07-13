package main

import (
	"errors"
	"os"
	"sync"
)

type RecoveryLossNotice struct {
	English string
	Russian string
}

func (m *RecoveryMaterial) DismissWithoutBackup() RecoveryLossNotice {
	m.discard()
	return RecoveryLossNotice{English: RecoveryLossWarningEN, Russian: RecoveryLossWarningRU}
}

type directExportFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

type directExportFileSystem interface {
	CreateExclusive(string) (directExportFile, error)
	Delete(string) error
}

type osDirectExportFileSystem struct{}

func (osDirectExportFileSystem) CreateExclusive(path string) (directExportFile, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}

func (osDirectExportFileSystem) Delete(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// RecoveryExporter writes only to the exact user-selected destination. It
// creates no temp, sidecar, autosave, recent-document entry, or secret-derived
// filename. Existing files are never silently truncated.
type RecoveryExporter struct {
	files directExportFileSystem
	mu    sync.Mutex
}

func (e *RecoveryExporter) String() string   { return "RecoveryExporter{<redacted>}" }
func (e *RecoveryExporter) GoString() string { return e.String() }

func NewRecoveryExporter() *RecoveryExporter {
	return &RecoveryExporter{files: osDirectExportFileSystem{}}
}

func newRecoveryExporterForTesting(files directExportFileSystem) *RecoveryExporter {
	return &RecoveryExporter{files: files}
}

func (e *RecoveryExporter) SaveSelectedDestination(path string, material *RecoveryMaterial) error {
	if path == "" || material == nil {
		return errRecoveryExportFailed
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	payload, err := material.exportJSON()
	if err != nil {
		return err
	}
	defer zeroBytes(payload)
	file, err := e.files.CreateExclusive(path)
	if err != nil {
		return errRecoveryExportFailed
	}
	remove := true
	defer func() {
		if remove {
			_ = e.files.Delete(path)
		}
	}()
	written := 0
	for written < len(payload) {
		n, writeErr := file.Write(payload[written:])
		if writeErr != nil || n <= 0 || n > len(payload)-written {
			_ = file.Close()
			return errRecoveryExportFailed
		}
		written += n
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return errRecoveryExportFailed
	}
	if err := file.Close(); err != nil {
		return errRecoveryExportFailed
	}
	remove = false
	return nil
}

var errRecoveryExportFailed = errors.New("recovery export failed")
