package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type boundedTrackReader struct {
	remaining int64
	maximum   int
}

func (r *boundedTrackReader) Read(buffer []byte) (int, error) {
	if len(buffer) > r.maximum {
		return 0, fmt.Errorf("unbounded read request: %d", len(buffer))
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}
	count := min(int64(len(buffer)), r.remaining)
	for index := int64(0); index < count; index++ {
		buffer[index] = byte(index % 251)
	}
	r.remaining -= count
	return int(count), nil
}

func TestWindowsStreamTrackDraftStoreUsesBoundedCopyAndSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	store, err := NewWindowsStreamTrackDraftStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	const size = int64(12<<20 + 37)
	released := false
	draft, err := store.Import(WindowsBrokeredAudioFile{
		DisplayName: "one-hour.opus", SizeBytes: size,
		Open: func() (io.ReadCloser, error) {
			return io.NopCloser(&boundedTrackReader{remaining: size, maximum: 64 << 10}), nil
		},
		Release: func() { released = true },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !released || draft.LocalByteCount != size || draft.UploadOffset != 0 || draft.ClientMIME != "audio/ogg" {
		t.Fatalf("draft=%+v released=%v", draft, released)
	}
	info, err := os.Stat(filepath.Join(dir, "stream-track-draft", draft.LocalID+".bin"))
	if err != nil || info.Size() != size {
		t.Fatalf("durable bytes info=%v err=%v", info, err)
	}
	reopened, err := NewWindowsStreamTrackDraftStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := reopened.Load()
	if err != nil || recovered == nil || *recovered != draft {
		t.Fatalf("recovered=%+v err=%v want=%+v", recovered, err, draft)
	}
	recovered.UploadOffset = 4 << 20
	if err := reopened.Update(*recovered); err != nil {
		t.Fatal(err)
	}
	restartedAgain, _ := NewWindowsStreamTrackDraftStore(dir)
	if got, err := restartedAgain.Load(); err != nil || got == nil || got.UploadOffset != 4<<20 {
		t.Fatalf("resumed metadata=%+v err=%v", got, err)
	}
}

func TestWindowsStreamTrackDraftStoreRejectsIneligibleWithoutRetainingCapability(t *testing.T) {
	store, err := NewWindowsStreamTrackDraftStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	released := false
	_, err = store.Import(WindowsBrokeredAudioFile{
		DisplayName: "not-audio.txt", SizeBytes: 10,
		Open:    func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("0123456789")), nil },
		Release: func() { released = true },
	})
	if err == nil || !released {
		t.Fatalf("err=%v released=%v", err, released)
	}
	if recovered, loadErr := store.Load(); loadErr != nil || recovered != nil {
		t.Fatalf("rejected file persisted: %+v err=%v", recovered, loadErr)
	}
}

type trackUploadDoer struct {
	t       *testing.T
	mu      sync.Mutex
	offsets []int64
	total   int64
}

func (d *trackUploadDoer) Do(request *http.Request) (*http.Response, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	uploadID := "up_" + strings.Repeat("A", 26)
	mediaID := "m_" + strings.Repeat("B", 26)
	token := strings.Repeat("ef", 32)
	if request.Method == http.MethodPost {
		body, _ := io.ReadAll(io.LimitReader(request.Body, 2048))
		if !strings.Contains(string(body), `"kind":"audio_track"`) || !strings.Contains(string(body), `"rights_acknowledged":true`) {
			d.t.Fatalf("create body=%s", body)
		}
		// Coordinator already owns the first chunk from a previous process.
		return phaseOneJSONResponse(request, http.StatusOK, fmt.Sprintf(`{"upload_id":%q,"media_id":%q,"upload_token":%q,"upload_offset":%d,"upload_length":%d,"status":"open","reused":true}`, uploadID, mediaID, token, 4<<20, d.total)), nil
	}
	if request.Method != http.MethodPut || request.URL.Path != "/v1/media/uploads/"+uploadID {
		d.t.Fatalf("unexpected request %s %s", request.Method, request.URL)
	}
	offset := int64(0)
	if _, err := fmt.Sscan(request.Header.Get("Upload-Offset"), &offset); err != nil {
		d.t.Fatal(err)
	}
	wantLength := min(windowsStreamTrackChunkBytes, d.total-offset)
	if request.ContentLength != wantLength || request.ContentLength > windowsStreamTrackChunkBytes {
		d.t.Fatalf("content length=%d want=%d", request.ContentLength, wantLength)
	}
	written, err := io.CopyBuffer(io.Discard, request.Body, make([]byte, 32<<10))
	if err != nil || written != wantLength {
		d.t.Fatalf("streamed=%d err=%v want=%d", written, err, wantLength)
	}
	d.offsets = append(d.offsets, offset)
	next := offset + wantLength
	status := "open"
	if next == d.total {
		status = "completed"
	}
	return phaseOneJSONResponse(request, http.StatusOK, fmt.Sprintf(`{"upload_id":%q,"media_id":%q,"upload_offset":%d,"upload_length":%d,"status":%q,"reused":true}`, uploadID, mediaID, next, d.total, status)), nil
}

func TestWindowsStreamTrackUploadResumesInBoundedChunks(t *testing.T) {
	const total = int64(9<<20 + 17)
	path := filepath.Join(t.TempDir(), "long-track.opus")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(total); err != nil || file.Close() != nil {
		t.Fatal(err)
	}
	doer := &trackUploadDoer{t: t, total: total}
	client, err := NewWindowsStreamTrackAppClient(phaseOneTestBundle(), doer)
	if err != nil {
		t.Fatal(err)
	}
	var progress []int64
	confirmation, err := client.UploadTrack(context.Background(), path, "long-track.opus", "track:0123456789abcdef0123456789abcdef", func(offset, _ int64) {
		progress = append(progress, offset)
	})
	if err != nil || confirmation.MediaID != "m_"+strings.Repeat("B", 26) {
		t.Fatalf("confirmation=%+v err=%v", confirmation, err)
	}
	wantOffsets := []int64{4 << 20, 8 << 20}
	if fmt.Sprint(doer.offsets) != fmt.Sprint(wantOffsets) || progress[len(progress)-1] != total {
		t.Fatalf("offsets=%v progress=%v", doer.offsets, progress)
	}
}

func TestWindowsStreamTrackSurfaceIsLocalizedKeyboardReachableAndNoGoHonest(t *testing.T) {
	actions := streamTrackUploadActions()
	if targetsInboxHasAction(actions, "queue") || targetsInboxHasAction(actions, "replace") || targetsInboxHasAction(actions, "resume") {
		t.Fatalf("no-go production actions advertise playback: %+v", actions)
	}
	snapshot := ShellSnapshot{
		StreamTrack: StreamTrackSnapshot{
			State: TargetsInboxReady, Draft: &StreamTrackDraft{
				LocalID: strings.Repeat("a", 32), LocalByteCount: 10, RetainedLocalBytes: true,
				Title: "long.opus", Phase: StreamTrackDraftProcessing, MediaID: "opaque-media",
			},
			Playback: StreamTrackPlayback{Phase: StreamTrackPlaybackIdle}, Failure: StreamTrackVariantUnavailable,
		},
	}
	for _, locale := range []ShellLocale{ShellEnglish, ShellRussian} {
		projection := NewShellCopy(locale).StreamTrackProjection(snapshot)
		if !strings.Contains(projection, "long.opus") || !strings.Contains(projection, "variant_unavailable") || strings.Contains(projection, strings.Repeat("a", 32)) || strings.Contains(projection, "opaque-media") {
			t.Fatalf("locale=%s projection=%q", locale, projection)
		}
	}
	foundShortcut := false
	for _, shortcut := range shellShortcuts {
		foundShortcut = foundShortcut || shortcut.Key == "L" && shortcut.Control && shortcut.Shift && shortcut.Command == "choose_stream_track"
	}
	if !foundShortcut {
		t.Fatal("long-track picker has no stable keyboard shortcut")
	}
	for _, dpi := range []int{96, 144, 192} {
		layout := layoutWindowsStreamTrackControls(ShellRect{X: dip(220, dpi), Width: dip(700, dpi)}, dip(420, dpi), dpi)
		for index, left := range layout.Rects() {
			if left.Width <= 0 || left.Height < dip(40, dpi) {
				t.Fatalf("dpi=%d invalid control %d: %+v", dpi, index, left)
			}
			for other := index + 1; other < len(layout.Rect); other++ {
				right := layout.Rect[other]
				if left.X < right.Right() && left.Right() > right.X && left.Y < right.Bottom() && left.Bottom() > right.Y {
					t.Fatalf("dpi=%d overlap %d/%d: %+v %+v", dpi, index, other, left, right)
				}
			}
		}
	}
}

func TestWindowsStreamTrackDraftRetentionAcrossOfflineReplacement(t *testing.T) {
	now := time.Now()
	model := NewStreamTrackModel()
	model.Replace(StreamTrackSnapshot{
		State: TargetsInboxReady, ContentPolicyState: "current", Actions: streamTrackUploadActions(),
		Draft:    &StreamTrackDraft{LocalID: strings.Repeat("a", 32), LocalByteCount: 42, RetainedLocalBytes: true, Title: "kept.opus", Phase: StreamTrackDraftRetained},
		Playback: StreamTrackPlayback{Phase: StreamTrackPlaybackIdle},
	}, now)
	model.Replace(StreamTrackSnapshot{State: TargetsInboxOffline, Failure: StreamTrackOffline, Playback: StreamTrackPlayback{Phase: StreamTrackPlaybackIdle}}, now.Add(time.Second))
	if got := model.Snapshot(); got.Draft == nil || got.Draft.Title != "kept.opus" || !got.Draft.RetainedLocalBytes {
		t.Fatalf("offline replacement lost durable draft: %+v", got)
	}
}
