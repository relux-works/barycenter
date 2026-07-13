package winprobe

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// artifactFileSystem is the narrow filesystem surface used by evidence
// writing and startup recovery. Keeping it injectable makes close, sync,
// remove, rename, and verification failures deterministic in regression tests.
type artifactFileSystem interface {
	MkdirAll(path string, perm os.FileMode) error
	Open(path string) (artifactFile, error)
	OpenFile(path string, flag int, perm os.FileMode) (artifactFile, error)
	ReadDir(path string) ([]fs.DirEntry, error)
	ReadFile(path string) ([]byte, error)
	Remove(path string) error
	RenameNoReplace(oldPath, newPath string) error
	Stat(path string) (os.FileInfo, error)
}

type artifactFile interface {
	io.Reader
	io.Writer
	io.Seeker
	Close() error
	Stat() (os.FileInfo, error)
	Sync() error
	Truncate(size int64) error
}

type osArtifactFileSystem struct{}

func (osArtifactFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osArtifactFileSystem) Open(path string) (artifactFile, error) {
	return os.Open(path)
}

func (osArtifactFileSystem) OpenFile(path string, flag int, perm os.FileMode) (artifactFile, error) {
	return os.OpenFile(path, flag, perm)
}

func (osArtifactFileSystem) ReadDir(path string) ([]fs.DirEntry, error) {
	return os.ReadDir(path)
}

func (osArtifactFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (osArtifactFileSystem) Remove(path string) error {
	return os.Remove(path)
}

func (osArtifactFileSystem) RenameNoReplace(oldPath, newPath string) error {
	return renameNoReplace(oldPath, newPath)
}

func (osArtifactFileSystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

var productionArtifactFS artifactFileSystem = osArtifactFileSystem{}

func removeAndVerifyAbsent(fileSystem artifactFileSystem, path string) error {
	var failures []error
	if err := fileSystem.Remove(path); err != nil && !os.IsNotExist(err) {
		failures = append(failures, fmt.Errorf("remove %s: %w", path, err))
	}
	if _, err := fileSystem.Stat(path); err == nil {
		failures = append(failures, fmt.Errorf("verify removal %s: path still exists", path))
	} else if !os.IsNotExist(err) {
		failures = append(failures, fmt.Errorf("verify removal %s: %w", path, err))
	}
	return errors.Join(failures...)
}

func removeOwnedAndVerifyAbsent(fileSystem artifactFileSystem, path string, identity os.FileInfo) error {
	current, err := fileSystem.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("verify ownership %s: %w", path, err)
	}
	if identity == nil || !os.SameFile(identity, current) {
		return fmt.Errorf("refuse to remove %s: owned path identity changed", path)
	}
	return removeAndVerifyAbsent(fileSystem, path)
}

// removePathsAndVerifyAbsent is intentionally ownership-agnostic. Callers may
// pass only paths their operation is authorized to remove. ArtifactWriter uses
// its stricter identity-tracked cleanup instead.
func removePathsAndVerifyAbsent(fileSystem artifactFileSystem, paths ...string) error {
	var failures []error
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, duplicate := seen[path]; duplicate {
			continue
		}
		seen[path] = struct{}{}
		if err := removeAndVerifyAbsent(fileSystem, path); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
