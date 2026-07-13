package winprobe

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestArtifactFileSystemRenameNoReplacePreservesExistingDestination(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	destination := filepath.Join(dir, "destination")
	if err := os.WriteFile(source, []byte("source bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, []byte("destination bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := productionArtifactFS.RenameNoReplace(source, destination); err == nil {
		t.Fatal("RenameNoReplace unexpectedly replaced its destination")
	}
	for path, want := range map[string]string{source: "source bytes", destination: "destination bytes"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("%s changed: content=%q err=%v", path, got, err)
		}
	}
}

type faultArtifactFS struct {
	base artifactFileSystem
	mu   sync.Mutex
	fail map[string][]error
	seen map[string]int
}

func newFaultArtifactFS() *faultArtifactFS {
	return &faultArtifactFS{
		base: osArtifactFileSystem{},
		fail: make(map[string][]error),
		seen: make(map[string]int),
	}
}

func faultKey(operation, path string) string { return operation + "\x00" + path }

func (f *faultArtifactFS) failNext(operation, path string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := faultKey(operation, path)
	f.fail[key] = append(f.fail[key], err)
}

func (f *faultArtifactFS) operationCount(operation, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seen[faultKey(operation, path)]
}

func (f *faultArtifactFS) injected(operation, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := faultKey(operation, path)
	f.seen[key]++
	queued := f.fail[key]
	if len(queued) == 0 {
		return nil
	}
	err := queued[0]
	f.fail[key] = queued[1:]
	return err
}

func (f *faultArtifactFS) MkdirAll(path string, perm os.FileMode) error {
	if err := f.injected("mkdir", path); err != nil {
		return err
	}
	return f.base.MkdirAll(path, perm)
}

func (f *faultArtifactFS) Open(path string) (artifactFile, error) {
	if err := f.injected("open", path); err != nil {
		return nil, err
	}
	file, err := f.base.Open(path)
	if err != nil {
		return nil, err
	}
	return &faultArtifactFile{ArtifactFile: file, owner: f, path: path}, nil
}

func (f *faultArtifactFS) OpenFile(path string, flag int, perm os.FileMode) (artifactFile, error) {
	if err := f.injected("openFile", path); err != nil {
		return nil, err
	}
	file, err := f.base.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}
	return &faultArtifactFile{ArtifactFile: file, owner: f, path: path}, nil
}

func (f *faultArtifactFS) ReadDir(path string) ([]fs.DirEntry, error) {
	if err := f.injected("readDir", path); err != nil {
		return nil, err
	}
	return f.base.ReadDir(path)
}

func (f *faultArtifactFS) ReadFile(path string) ([]byte, error) {
	if err := f.injected("readFile", path); err != nil {
		return nil, err
	}
	return f.base.ReadFile(path)
}

func (f *faultArtifactFS) Remove(path string) error {
	if err := f.injected("remove", path); err != nil {
		return err
	}
	return f.base.Remove(path)
}

func (f *faultArtifactFS) RenameNoReplace(oldPath, newPath string) error {
	if err := f.injected("renameNoReplace", oldPath+"->"+newPath); err != nil {
		return err
	}
	return f.base.RenameNoReplace(oldPath, newPath)
}

func (f *faultArtifactFS) Stat(path string) (os.FileInfo, error) {
	if err := f.injected("stat", path); err != nil {
		return nil, err
	}
	return f.base.Stat(path)
}

type faultArtifactFile struct {
	ArtifactFile artifactFile
	owner        *faultArtifactFS
	path         string
}

func (f *faultArtifactFile) Read(buffer []byte) (int, error) {
	if err := f.owner.injected("read", f.path); err != nil {
		return 0, err
	}
	return f.ArtifactFile.Read(buffer)
}

func (f *faultArtifactFile) Write(buffer []byte) (int, error) {
	if err := f.owner.injected("write", f.path); err != nil {
		return 0, err
	}
	return f.ArtifactFile.Write(buffer)
}

func (f *faultArtifactFile) Seek(offset int64, whence int) (int64, error) {
	if err := f.owner.injected("seek", f.path); err != nil {
		return 0, err
	}
	return f.ArtifactFile.Seek(offset, whence)
}

func (f *faultArtifactFile) Close() error {
	closeErr := f.ArtifactFile.Close()
	return errors.Join(closeErr, f.owner.injected("close", f.path))
}

func (f *faultArtifactFile) Stat() (os.FileInfo, error) {
	if err := f.owner.injected("fileStat", f.path); err != nil {
		return nil, err
	}
	return f.ArtifactFile.Stat()
}

func (f *faultArtifactFile) Sync() error {
	if err := f.owner.injected("sync", f.path); err != nil {
		return err
	}
	return f.ArtifactFile.Sync()
}

func (f *faultArtifactFile) Truncate(size int64) error {
	if err := f.owner.injected("truncate", f.path); err != nil {
		return err
	}
	return f.ArtifactFile.Truncate(size)
}
