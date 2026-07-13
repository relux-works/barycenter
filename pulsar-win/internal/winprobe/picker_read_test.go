package winprobe

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type failingReader struct {
	data []byte
	done bool
}

func (r *failingReader) Read(buffer []byte) (int, error) {
	if r.done {
		return 0, errors.New("provider hydration failed")
	}
	r.done = true
	return copy(buffer, r.data), nil
}

func TestReadAndHashPickedFileEnforcesActualBytes(t *testing.T) {
	t.Parallel()
	hash, read, err := ReadAndHashPickedFile(bytes.NewReader([]byte("abc")), 3)
	if err != nil || read != 3 || hash != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("ReadAndHashPickedFile() = (%q, %d, %v)", hash, read, err)
	}
	if _, read, err := ReadAndHashPickedFile(bytes.NewReader([]byte("abcd")), 3); err == nil || read != 4 {
		t.Fatalf("over-limit read = (%d, %v)", read, err)
	}
	if _, read, err := ReadAndHashPickedFile(bytes.NewReader(nil), 3); err == nil || read != 0 {
		t.Fatalf("empty read = (%d, %v)", read, err)
	}
	if _, read, err := ReadAndHashPickedFile(&failingReader{data: []byte("abc")}, 10); err == nil || read != 3 {
		t.Fatalf("provider failure read = (%d, %v)", read, err)
	}
	if _, _, err := ReadAndHashPickedFile(zeroReader{}, 10); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("zero progress error = %v", err)
	}
}

type zeroReader struct{}

func (zeroReader) Read([]byte) (int, error) { return 0, nil }
