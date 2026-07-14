package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

type testTelegramDownloader struct {
	raw   []byte
	err   error
	calls int
	paths []string
	limit int64
	size  int64
}

func (downloader *testTelegramDownloader) DownloadVoiceBounded(
	_ string,
	destination string,
	maxBytes int64,
) (int64, error) {
	downloader.calls++
	downloader.paths = append(downloader.paths, destination)
	downloader.limit = maxBytes
	if downloader.raw != nil {
		if err := os.WriteFile(destination, downloader.raw, 0o666); err != nil {
			return 0, err
		}
	}
	size := int64(len(downloader.raw))
	if downloader.size != 0 {
		size = downloader.size
	}
	return size, downloader.err
}

type failingTelegramSubmitter struct{ err error }

func (submitter failingTelegramSubmitter) SubmitMedia(context.Context, Submission) (store.MediaItem, error) {
	return store.MediaItem{}, submitter.err
}

type telegramAdapterHarness struct {
	store      *store.Store
	orbit      *store.Orbit
	mediaDir   string
	service    *SubmitService
	adapter    *TelegramAdapter
	downloader *testTelegramDownloader
	clock      int64
}

func newTelegramAdapterHarness(t *testing.T, raw []byte) *telegramAdapterHarness {
	t.Helper()
	root := t.TempDir()
	st, err := store.Open(filepath.Join(root, "coordinator.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	orbit, err := st.BootstrapLegacyOrbit(
		map[string]string{"a": strings.Repeat("a", 64)},
		map[int64]string{7001: "a"},
	)
	if err != nil || orbit == nil {
		t.Fatalf("bootstrap orbit=%+v err=%v", orbit, err)
	}
	if err := st.SetMemberName(orbit.ID, 7001, "Telegram sender"); err != nil {
		t.Fatal(err)
	}
	mediaDir := filepath.Join(root, "media")
	runner := newFakeCommandRunner()
	service, err := newSubmitService(
		st, mediaDir, PresetDefault,
		newProcessorForTest(runner, DefaultLimits()),
	)
	if err != nil {
		t.Fatal(err)
	}
	downloader := &testTelegramDownloader{raw: raw}
	adapter, err := NewTelegramAdapter(st, mediaDir, downloader, service)
	if err != nil {
		t.Fatal(err)
	}
	harness := &telegramAdapterHarness{
		store: st, orbit: orbit, mediaDir: mediaDir, service: service,
		adapter: adapter, downloader: downloader, clock: time.Now().UnixMilli(),
	}
	service.now = harness.now
	adapter.now = harness.now
	return harness
}

func (harness *telegramAdapterHarness) now() time.Time {
	harness.clock++
	return time.UnixMilli(harness.clock)
}

func (harness *telegramAdapterHarness) accept(t *testing.T) TelegramAcceptance {
	t.Helper()
	acceptedAt := harness.now().UnixMilli()
	accepted, err := harness.adapter.Accept(TelegramVoice{
		OwnerOrbitID: harness.orbit.ID, TelegramUserID: 7001,
		TelegramFileID: "opaque-file-id", Title: "Telegram sender",
		AcceptedAt: acceptedAt,
		ExpiresAt:  acceptedAt + int64((30*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	return accepted
}

func TestTelegramAdapterUsesSubmitMediaAndReturnsLegacyCompatibilityWAV(t *testing.T) {
	harness := newTelegramAdapterHarness(t, testWAVBytes(100))
	accepted := harness.accept(t)

	before, err := harness.store.GetMediaItem(accepted.MediaID)
	if err != nil || before == nil || before.Status != store.MediaStatusProcessing ||
		before.Source != store.MediaSourceTelegram {
		t.Fatalf("accepted generic media=%+v err=%v", before, err)
	}
	legacy, err := harness.store.GetMedia(accepted.MediaID)
	if err != nil || legacy == nil || legacy.Status != "processing" ||
		legacy.TGFileID != "opaque-file-id" || legacy.ID != before.ID {
		t.Fatalf("accepted legacy media=%+v err=%v", legacy, err)
	}

	result, err := harness.adapter.Submit(context.Background(), accepted)
	if err != nil {
		t.Fatal(err)
	}
	after, err := harness.store.GetMediaItem(accepted.MediaID)
	if err != nil || after == nil || after.Status != store.MediaStatusReady ||
		after.StorageKey == "" || after.DurationMS != result.DurationMS ||
		after.SizeBytes != result.SizeBytes || after.SHA256 != result.SHA256 ||
		result.WAVPath == "" {
		t.Fatalf("ready generic=%+v result=%+v err=%v", after, result, err)
	}
	info, err := os.Stat(result.WAVPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 ||
		info.Size() != result.SizeBytes {
		t.Fatalf("compatibility WAV info=%+v err=%v", info, err)
	}
	if harness.downloader.calls != 1 || harness.downloader.limit != maxTelegramVoiceBytes {
		t.Fatalf("download calls=%d limit=%d", harness.downloader.calls, harness.downloader.limit)
	}
	if _, err := os.Stat(harness.downloader.paths[0]); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful Telegram source was not cleaned: %v", err)
	}

	// A replay resolves the already-ready common item and never downloads or
	// transcodes the Telegram source again.
	replayed, err := harness.adapter.Submit(context.Background(), accepted)
	if err != nil || replayed.WAVPath != result.WAVPath || harness.downloader.calls != 1 {
		t.Fatalf("replayed result=%+v calls=%d err=%v", replayed, harness.downloader.calls, err)
	}
}

func TestTelegramAdapterPersistsSanitizedDownloadFailureAndRetainsPrivateSource(t *testing.T) {
	harness := newTelegramAdapterHarness(t, []byte("partial Telegram bytes"))
	harness.downloader.err = errors.New("secret bot token and /private/source/path")
	accepted := harness.accept(t)

	_, err := harness.adapter.Submit(context.Background(), accepted)
	assertProcessingCode(t, err, "media_input_unavailable")
	if err.Error() != "media_input_unavailable" || strings.Contains(err.Error(), "secret") ||
		strings.Contains(err.Error(), "private") {
		t.Fatalf("unsanitized Telegram failure=%q", err)
	}
	item, lookupErr := harness.store.GetMediaItem(accepted.MediaID)
	if lookupErr != nil || item == nil || item.Status != store.MediaStatusFailed ||
		item.FailureCode != "media_input_unavailable" {
		t.Fatalf("failed generic media=%+v err=%v", item, lookupErr)
	}
	info, statErr := os.Stat(harness.downloader.paths[0])
	if statErr != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("retained Telegram source info=%+v err=%v", info, statErr)
	}
}

func TestTelegramAdapterClassifiesBoundedDownloadOverflow(t *testing.T) {
	harness := newTelegramAdapterHarness(t, []byte("bounded partial source"))
	harness.downloader.size = maxTelegramVoiceBytes + 1
	harness.downloader.err = errors.New("bounded download stopped")
	accepted := harness.accept(t)

	_, err := harness.adapter.Submit(context.Background(), accepted)
	assertProcessingCode(t, err, "media_input_oversized")
	item, lookupErr := harness.store.GetMediaItem(accepted.MediaID)
	if lookupErr != nil || item == nil || item.Status != store.MediaStatusFailed ||
		item.FailureCode != "media_input_oversized" {
		t.Fatalf("bounded overflow media=%+v err=%v", item, lookupErr)
	}
}

func TestTelegramAdapterRejectsAcceptanceFileIdentitySubstitution(t *testing.T) {
	harness := newTelegramAdapterHarness(t, testWAVBytes(10))
	accepted := harness.accept(t)
	accepted.TelegramFileID = "substituted-file-id"

	_, err := harness.adapter.Submit(context.Background(), accepted)
	assertProcessingCode(t, err, "media_unavailable")
	if harness.downloader.calls != 0 {
		t.Fatalf("substituted Telegram identity triggered %d download(s)", harness.downloader.calls)
	}
	item, lookupErr := harness.store.GetMediaItem(accepted.MediaID)
	if lookupErr != nil || item == nil || item.Status != store.MediaStatusProcessing {
		t.Fatalf("substituted identity changed media=%+v err=%v", item, lookupErr)
	}
}

func TestTelegramAdapterPreservesCommonFailureCode(t *testing.T) {
	harness := newTelegramAdapterHarness(t, []byte("not an audio container"))
	accepted := harness.accept(t)

	_, err := harness.adapter.Submit(context.Background(), accepted)
	assertProcessingCode(t, err, "media_signature_unsupported")
	item, lookupErr := harness.store.GetMediaItem(accepted.MediaID)
	if lookupErr != nil || item == nil || item.Status != store.MediaStatusFailed ||
		item.FailureCode != "media_signature_unsupported" {
		t.Fatalf("common failure media=%+v err=%v", item, lookupErr)
	}
}

func TestTelegramAdapterTerminatesUnclassifiedSubmitFailure(t *testing.T) {
	harness := newTelegramAdapterHarness(t, testWAVBytes(10))
	adapter, err := NewTelegramAdapter(
		harness.store, harness.mediaDir, harness.downloader,
		failingTelegramSubmitter{err: errors.New("private worker capacity detail")},
	)
	if err != nil {
		t.Fatal(err)
	}
	adapter.now = harness.now
	harness.adapter = adapter
	accepted := harness.accept(t)

	_, err = adapter.Submit(context.Background(), accepted)
	assertProcessingCode(t, err, "media_processing_unavailable")
	item, lookupErr := harness.store.GetMediaItem(accepted.MediaID)
	if lookupErr != nil || item == nil || item.Status != store.MediaStatusFailed ||
		item.FailureCode != "media_processing_unavailable" {
		t.Fatalf("unclassified failure media=%+v err=%v", item, lookupErr)
	}
}
