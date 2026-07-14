package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

type stubMediaClipFetcher struct {
	mu      sync.Mutex
	path    string
	release <-chan struct{}
	failure error
	fetches int
	removed []string
}

func (f *stubMediaClipFetcher) Fetch(ctx context.Context, _ MediaClipFetchRequest) (string, error) {
	f.mu.Lock()
	f.fetches++
	release, failure, path := f.release, f.failure, f.path
	f.mu.Unlock()
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if failure != nil {
		return "", failure
	}
	return path, nil
}

func (f *stubMediaClipFetcher) Remove(path string) {
	f.mu.Lock()
	f.removed = append(f.removed, path)
	f.mu.Unlock()
}

func (f *stubMediaClipFetcher) snapshot() (int, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fetches, append([]string(nil), f.removed...)
}

type stubMediaClipMixer struct {
	mu             sync.Mutex
	capabilities   []string
	durationMS     int64
	prepareFailure error
	armFailure     error
	cancelFailure  error
	mainResumed    bool
	started        func(int64)
	ended          func(int64)
	lastPlan       MediaClipPlayPlan
	armCount       int
	cancelCount    int
	disposeCount   int
}

func (m *stubMediaClipMixer) DeliveryCapabilities() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.capabilities...)
}

func (m *stubMediaClipMixer) Prepare(path, _ string) (*PreparedMediaClip, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.prepareFailure != nil {
		return nil, m.prepareFailure
	}
	duration := m.durationMS
	if duration == 0 {
		duration = 4200
	}
	return &PreparedMediaClip{LocalPath: path, DecodedDurationMS: duration, Decoder: struct{}{}}, nil
}

func (m *stubMediaClipMixer) Arm(_ *PreparedMediaClip, plan MediaClipPlayPlan, started, ended func(int64)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.armCount++
	m.lastPlan = plan
	m.started, m.ended = started, ended
	return m.armFailure
}

func (m *stubMediaClipMixer) plan() MediaClipPlayPlan {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastPlan
}

func (m *stubMediaClipMixer) Cancel(_ *PreparedMediaClip, _ protocol.CancelMediaPayload, done func(bool, error)) {
	m.mu.Lock()
	m.cancelCount++
	resumed, err := m.mainResumed, m.cancelFailure
	m.mu.Unlock()
	done(resumed, err)
}

func (m *stubMediaClipMixer) Dispose(_ *PreparedMediaClip) {
	m.mu.Lock()
	m.disposeCount++
	m.mu.Unlock()
}

func (m *stubMediaClipMixer) fireStarted(localMS int64) {
	m.mu.Lock()
	callback := m.started
	m.mu.Unlock()
	if callback != nil {
		callback(localMS)
	}
}

func (m *stubMediaClipMixer) fireEnded(localMS int64) {
	m.mu.Lock()
	callback := m.ended
	m.mu.Unlock()
	if callback != nil {
		callback(localMS)
	}
}

func (m *stubMediaClipMixer) counts() (arm, cancel, dispose int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.armCount, m.cancelCount, m.disposeCount
}

type recordedMediaEvent struct {
	typ         string
	generation  int64
	code        string
	timestamp   int64
	mainResumed bool
}

type mediaEventRecorder struct {
	mu     sync.Mutex
	events []recordedMediaEvent
}

func (r *mediaEventRecorder) send(messageType string, payload any) {
	event := recordedMediaEvent{typ: messageType}
	switch value := payload.(type) {
	case *protocol.MediaReadyPayload:
		event.generation = value.Generation
	case *protocol.MediaStartedPayload:
		event.generation, event.timestamp = value.Generation, value.TFirstSampleCoordMS
	case *protocol.MediaEndedPayload:
		event.generation, event.code, event.timestamp = value.Generation, value.Reason, value.TLastSampleCoordMS
	case *protocol.MediaFailedPayload:
		event.generation, event.code = value.Generation, value.Code
	case *protocol.MediaCancelledPayload:
		event.generation, event.code, event.mainResumed = value.Generation, value.Reason, value.MainResumed
	default:
		return
	}
	r.mu.Lock()
	r.events = append(r.events, event)
	r.mu.Unlock()
}

func (r *mediaEventRecorder) snapshot() []recordedMediaEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recordedMediaEvent(nil), r.events...)
}

func testPrepareMedia(generation int64) protocol.PrepareMediaPayload {
	return protocol.PrepareMediaPayload{
		TransmissionID: "tr_test", Generation: generation, MediaID: "m_test",
		Kind: "voice_clip", Delivery: "overlay",
		FileURL: "https://coord.example/v1/media/m_test",
		SHA256:  fmt.Sprintf("%064x", 1), SizeBytes: 100, DurationMS: 4200,
		MediaExpiresAtCoordMS: 30_000, PrepareDeadlineCoordMS: 20_000,
	}
}

func testOverlayPlay(generation int64) protocol.PlayMediaAtPayload {
	duck := -12.0
	attack, release := int64(250), int64(600)
	return protocol.PlayMediaAtPayload{
		TransmissionID: "tr_test", Generation: generation,
		TCoordMS: 11_000, StartDeadlineCoordMS: 11_100, Delivery: "overlay",
		DuckDB: &duck, AttackMS: &attack, ReleaseMS: &release,
	}
}

func testInterruptPlay(generation int64) protocol.PlayMediaAtPayload {
	fadeOut, fadeIn := int64(250), int64(120)
	return protocol.PlayMediaAtPayload{
		TransmissionID: "tr_test", Generation: generation,
		TCoordMS: 11_000, StartDeadlineCoordMS: 11_100, Delivery: "interrupt",
		FadeOutMS: &fadeOut, FadeInMS: &fadeIn,
	}
}

func newTestMediaClipClient(fetcher *stubMediaClipFetcher, mixer *stubMediaClipMixer, recorder *mediaEventRecorder, now func() int64, clock deadlineClock) *MediaClipClient {
	if fetcher.path == "" {
		fetcher.path = filepath.Join(os.TempDir(), "media-clip-test.wav")
	}
	if now == nil {
		now = func() int64 { return 10_000 }
	}
	client := NewMediaClipClient(fetcher, mixer, testLogger(), now)
	client.Bind(clock, recorder.send, 0)
	client.Synchronize()
	return client
}

func waitForMediaEvents(t *testing.T, recorder *mediaEventRecorder, count int) []recordedMediaEvent {
	t.Helper()
	waitFor(t, 3*time.Second, func() bool { return len(recorder.snapshot()) >= count }, "media event never arrived")
	return recorder.snapshot()
}

func TestMediaClipReadyWaitsForFetchAndDecoder(t *testing.T) {
	release := make(chan struct{})
	fetcher := &stubMediaClipFetcher{release: release}
	mixer := &stubMediaClipMixer{}
	recorder := &mediaEventRecorder{}
	client := newTestMediaClipClient(fetcher, mixer, recorder, nil, fixedClock{ok: true})
	defer client.Stop()

	payload := testPrepareMedia(1)
	client.Prepare(&payload)
	client.Synchronize()
	if events := recorder.snapshot(); len(events) != 0 {
		t.Fatalf("ready emitted before fetch completed: %+v", events)
	}
	close(release)
	events := waitForMediaEvents(t, recorder, 1)
	if events[0].typ != protocol.TypeMediaReady || events[0].generation != 1 {
		t.Fatalf("event %+v, want generation-one media_ready", events[0])
	}
}

func TestMediaClipPrepareFailuresAreTypedTerminalAndCleaned(t *testing.T) {
	t.Run("fetch hash", func(t *testing.T) {
		fetcher := &stubMediaClipFetcher{failure: mediaClipFailure("hash_mismatch")}
		mixer := &stubMediaClipMixer{}
		recorder := &mediaEventRecorder{}
		client := newTestMediaClipClient(fetcher, mixer, recorder, nil, fixedClock{ok: true})
		defer client.Stop()
		payload := testPrepareMedia(1)
		client.Prepare(&payload)
		waitForMediaEvents(t, recorder, 1)
		client.Prepare(&payload)
		client.Synchronize()
		events := recorder.snapshot()
		if len(events) != 1 || events[0].code != "hash_mismatch" {
			t.Fatalf("events %+v", events)
		}
	})

	t.Run("decoder", func(t *testing.T) {
		fetcher := &stubMediaClipFetcher{}
		mixer := &stubMediaClipMixer{prepareFailure: mediaClipFailure("decode_failed")}
		recorder := &mediaEventRecorder{}
		client := newTestMediaClipClient(fetcher, mixer, recorder, nil, fixedClock{ok: true})
		defer client.Stop()
		payload := testPrepareMedia(1)
		client.Prepare(&payload)
		events := waitForMediaEvents(t, recorder, 1)
		_, removed := fetcher.snapshot()
		if events[0].code != "decode_failed" || len(removed) != 1 {
			t.Fatalf("events=%+v removed=%v", events, removed)
		}
	})

	t.Run("duration", func(t *testing.T) {
		fetcher := &stubMediaClipFetcher{}
		mixer := &stubMediaClipMixer{durationMS: 4201}
		recorder := &mediaEventRecorder{}
		client := newTestMediaClipClient(fetcher, mixer, recorder, nil, fixedClock{ok: true})
		defer client.Stop()
		payload := testPrepareMedia(1)
		client.Prepare(&payload)
		events := waitForMediaEvents(t, recorder, 1)
		_, removed := fetcher.snapshot()
		_, _, disposed := mixer.counts()
		if events[0].code != "duration_mismatch" || len(removed) != 1 || disposed != 1 {
			t.Fatalf("events=%+v removed=%v disposed=%d", events, removed, disposed)
		}
	})
}

func TestMediaClipScheduledCallbacksAreGenerationSafeAndTerminalOnce(t *testing.T) {
	fetcher := &stubMediaClipFetcher{}
	mixer := &stubMediaClipMixer{capabilities: []string{protocol.CapabilityOverlayMix}}
	recorder := &mediaEventRecorder{}
	client := newTestMediaClipClient(fetcher, mixer, recorder, nil, fixedClock{ok: true})
	defer client.Stop()
	payload := testPrepareMedia(1)
	client.Prepare(&payload)
	waitForMediaEvents(t, recorder, 1)
	play := testOverlayPlay(1)
	client.Play(&play)
	client.Synchronize()
	arm, _, _ := mixer.counts()
	if arm != 1 {
		t.Fatalf("arm count %d, want 1", arm)
	}
	mixer.fireStarted(11_010)
	mixer.fireStarted(11_011)
	waitForMediaEvents(t, recorder, 2)
	mixer.fireEnded(15_210)
	mixer.fireEnded(15_211)
	events := waitForMediaEvents(t, recorder, 3)
	if got := []string{events[0].typ, events[1].typ, events[2].typ}; !reflect.DeepEqual(got,
		[]string{protocol.TypeMediaReady, protocol.TypeMediaStarted, protocol.TypeMediaEnded}) {
		t.Fatalf("event types %v", got)
	}
	if events[1].timestamp != 11_010 || events[2].timestamp != 15_210 {
		t.Fatalf("timestamps %+v", events)
	}
	_, removed := fetcher.snapshot()
	if len(removed) != 1 {
		t.Fatalf("terminal cleanup removed %v", removed)
	}
}

func TestMediaClipDuplicateAndReorderedCommandsAreIdempotent(t *testing.T) {
	fetcher := &stubMediaClipFetcher{}
	mixer := &stubMediaClipMixer{capabilities: []string{protocol.CapabilityOverlayMix}}
	recorder := &mediaEventRecorder{}
	client := newTestMediaClipClient(fetcher, mixer, recorder, nil, fixedClock{ok: true})
	defer client.Stop()

	prepare := testPrepareMedia(2)
	lowerPrepare := testPrepareMedia(1)
	client.Prepare(&prepare)
	client.Prepare(&prepare)
	client.Prepare(&lowerPrepare)
	waitForMediaEvents(t, recorder, 1)
	fetches, _ := fetcher.snapshot()
	if fetches != 1 {
		t.Fatalf("duplicate/lower prepare fetched %d times", fetches)
	}

	play := testOverlayPlay(2)
	lowerPlay := testOverlayPlay(1)
	client.Play(&play)
	client.Play(&play)
	client.Play(&lowerPlay)
	client.Synchronize()
	arm, cancelCount, _ := mixer.counts()
	if arm != 1 || cancelCount != 0 {
		t.Fatalf("duplicate/lower play arm=%d cancel=%d", arm, cancelCount)
	}

	lowerCancel := protocol.CancelMediaPayload{
		TransmissionID: "tr_test", Generation: 1, Reason: "sender_cancelled", Action: "disarm",
	}
	client.Cancel(&lowerCancel)
	client.Synchronize()
	if len(recorder.snapshot()) != 1 {
		t.Fatalf("lower cancel emitted an event: %+v", recorder.snapshot())
	}
	cancel := lowerCancel
	cancel.Generation = 2
	client.Cancel(&cancel)
	client.Cancel(&cancel)
	events := waitForMediaEvents(t, recorder, 2)
	_, cancelCount, _ = mixer.counts()
	if len(events) != 2 || events[1].typ != protocol.TypeMediaCancelled || cancelCount != 1 {
		t.Fatalf("events=%+v cancel=%d", events, cancelCount)
	}
}

func TestMediaClipTranslatesCoordinatorScheduleAndReceiptTimestamps(t *testing.T) {
	fetcher := &stubMediaClipFetcher{}
	mixer := &stubMediaClipMixer{capabilities: []string{protocol.CapabilityOverlayMix}}
	recorder := &mediaEventRecorder{}
	client := newTestMediaClipClient(
		fetcher, mixer, recorder, func() int64 { return 10_250 }, fixedClock{offset: 250, ok: true})
	defer client.Stop()
	prepare := testPrepareMedia(1)
	client.Prepare(&prepare)
	waitForMediaEvents(t, recorder, 1)
	play := testOverlayPlay(1)
	client.Play(&play)
	client.Synchronize()
	plan := mixer.plan()
	if plan.LocalStartMS != 11_250 || plan.LocalStartDeadlineMS != 11_350 {
		t.Fatalf("local plan %+v", plan)
	}
	mixer.fireStarted(11_260)
	events := waitForMediaEvents(t, recorder, 2)
	if events[1].timestamp != 11_010 {
		t.Fatalf("coordinator receipt timestamp %d, want 11010", events[1].timestamp)
	}
}

func TestMediaClipRoutesInterruptThroughExactMixerCapability(t *testing.T) {
	fetcher := &stubMediaClipFetcher{}
	mixer := &stubMediaClipMixer{
		capabilities: []string{protocol.CapabilityInterruptResume}, mainResumed: true,
	}
	recorder := &mediaEventRecorder{}
	client := newTestMediaClipClient(fetcher, mixer, recorder, nil, fixedClock{ok: true})
	defer client.Stop()
	prepare := testPrepareMedia(1)
	prepare.Delivery = "interrupt"
	client.Prepare(&prepare)
	waitForMediaEvents(t, recorder, 1)
	play := testInterruptPlay(1)
	client.Play(&play)
	client.Synchronize()
	if arm, _, _ := mixer.counts(); arm != 1 || mixer.plan().Payload.Delivery != "interrupt" {
		t.Fatalf("interrupt was not routed to mixer: arm=%d plan=%+v", arm, mixer.plan())
	}
	mixer.fireStarted(11_005)
	waitForMediaEvents(t, recorder, 2)
	cancel := protocol.CancelMediaPayload{
		TransmissionID: "tr_test", Generation: 1, Reason: "media_deleted",
		Action: "fade_stop", ResumeMain: true, FadeMS: 120,
	}
	client.Cancel(&cancel)
	events := waitForMediaEvents(t, recorder, 3)
	if events[2].typ != protocol.TypeMediaCancelled || !events[2].mainResumed {
		t.Fatalf("interrupt cancellation receipt %+v", events[2])
	}
}

func TestMediaClipScheduleRejectsStaleUnsyncedAndUnadvertisedStarts(t *testing.T) {
	cases := []struct {
		name         string
		now          int64
		clock        deadlineClock
		capabilities []string
		wantCode     string
	}{
		{name: "stale", now: 12_000, clock: fixedClock{ok: true}, capabilities: []string{protocol.CapabilityOverlayMix}, wantCode: "stale_play"},
		{name: "unsynchronized", now: 10_000, clock: fixedClock{ok: false}, capabilities: []string{protocol.CapabilityOverlayMix}, wantCode: "clock_unsynchronized"},
		{name: "capability lost", now: 10_000, clock: fixedClock{ok: true}, wantCode: "capability_lost"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fetcher := &stubMediaClipFetcher{}
			mixer := &stubMediaClipMixer{capabilities: tc.capabilities}
			recorder := &mediaEventRecorder{}
			client := newTestMediaClipClient(fetcher, mixer, recorder, func() int64 { return tc.now }, tc.clock)
			defer client.Stop()
			prepare := testPrepareMedia(1)
			client.Prepare(&prepare)
			waitForMediaEvents(t, recorder, 1)
			play := testOverlayPlay(1)
			client.Play(&play)
			events := waitForMediaEvents(t, recorder, 2)
			arm, _, _ := mixer.counts()
			if events[1].code != tc.wantCode || arm != 0 {
				t.Fatalf("events=%+v arm=%d", events, arm)
			}
		})
	}
}

func TestMediaClipDeadlineDisarmsBeforeStaleFailure(t *testing.T) {
	fetcher := &stubMediaClipFetcher{}
	mixer := &stubMediaClipMixer{capabilities: []string{protocol.CapabilityOverlayMix}}
	recorder := &mediaEventRecorder{}
	client := newTestMediaClipClient(fetcher, mixer, recorder, nil, fixedClock{ok: true})
	defer client.Stop()
	prepare := testPrepareMedia(1)
	client.Prepare(&prepare)
	waitForMediaEvents(t, recorder, 1)
	play := testOverlayPlay(1)
	play.TCoordMS, play.StartDeadlineCoordMS = 9_930, 10_030
	client.Play(&play)
	events := waitForMediaEvents(t, recorder, 2)
	arm, cancel, _ := mixer.counts()
	if events[1].code != "stale_play" || arm != 1 || cancel != 1 {
		t.Fatalf("events=%+v arm=%d cancel=%d", events, arm, cancel)
	}
}

func TestMediaClipCancelTombstoneAndActiveCancelAreIdempotent(t *testing.T) {
	t.Run("preparing", func(t *testing.T) {
		release := make(chan struct{})
		fetcher := &stubMediaClipFetcher{release: release}
		mixer := &stubMediaClipMixer{}
		recorder := &mediaEventRecorder{}
		client := newTestMediaClipClient(fetcher, mixer, recorder, nil, fixedClock{ok: true})
		defer client.Stop()
		prepare := testPrepareMedia(1)
		client.Prepare(&prepare)
		client.Synchronize()
		cancel := protocol.CancelMediaPayload{
			TransmissionID: "tr_test", Generation: 1, Reason: "media_deleted", Action: "disarm",
		}
		client.Cancel(&cancel)
		events := waitForMediaEvents(t, recorder, 1)
		close(release)
		client.Synchronize()
		time.Sleep(10 * time.Millisecond)
		if len(events) != 1 || events[0].typ != protocol.TypeMediaCancelled || len(recorder.snapshot()) != 1 {
			t.Fatalf("events=%+v final=%+v", events, recorder.snapshot())
		}
	})

	t.Run("tombstone", func(t *testing.T) {
		fetcher := &stubMediaClipFetcher{}
		mixer := &stubMediaClipMixer{}
		recorder := &mediaEventRecorder{}
		client := newTestMediaClipClient(fetcher, mixer, recorder, nil, fixedClock{ok: true})
		defer client.Stop()
		cancel := protocol.CancelMediaPayload{
			TransmissionID: "tr_test", Generation: 2, Reason: "media_deleted", Action: "disarm",
		}
		client.Cancel(&cancel)
		client.Cancel(&cancel)
		older, same := testPrepareMedia(1), testPrepareMedia(2)
		client.Prepare(&older)
		client.Prepare(&same)
		client.Synchronize()
		events := recorder.snapshot()
		fetches, _ := fetcher.snapshot()
		if len(events) != 1 || events[0].typ != protocol.TypeMediaCancelled || fetches != 0 {
			t.Fatalf("events=%+v fetches=%d", events, fetches)
		}
	})

	t.Run("active", func(t *testing.T) {
		fetcher := &stubMediaClipFetcher{}
		mixer := &stubMediaClipMixer{capabilities: []string{protocol.CapabilityOverlayMix}}
		recorder := &mediaEventRecorder{}
		client := newTestMediaClipClient(fetcher, mixer, recorder, nil, fixedClock{ok: true})
		defer client.Stop()
		prepare := testPrepareMedia(1)
		client.Prepare(&prepare)
		waitForMediaEvents(t, recorder, 1)
		play := testOverlayPlay(1)
		client.Play(&play)
		client.Synchronize()
		cancel := protocol.CancelMediaPayload{
			TransmissionID: "tr_test", Generation: 1, Reason: "sender_cancelled", Action: "disarm",
		}
		client.Cancel(&cancel)
		waitForMediaEvents(t, recorder, 2)
		mixer.fireStarted(11_000)
		mixer.fireEnded(15_000)
		client.Synchronize()
		events := recorder.snapshot()
		_, cancelCount, _ := mixer.counts()
		if len(events) != 2 || events[1].typ != protocol.TypeMediaCancelled || cancelCount != 1 {
			t.Fatalf("events=%+v cancel=%d", events, cancelCount)
		}
	})
}

func TestMediaClipExpiredAndLatePrepareNeverFetchOrReportReady(t *testing.T) {
	cases := []struct {
		name       string
		expiry     int64
		deadline   int64
		wantEvents int
		wantCode   string
	}{
		{name: "expired", expiry: 9_500, deadline: 9_000, wantEvents: 1, wantCode: "media_expired"},
		{name: "deadline", expiry: 30_000, deadline: 9_000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fetcher := &stubMediaClipFetcher{}
			mixer := &stubMediaClipMixer{}
			recorder := &mediaEventRecorder{}
			client := newTestMediaClipClient(fetcher, mixer, recorder, nil, fixedClock{ok: true})
			defer client.Stop()
			prepare := testPrepareMedia(1)
			prepare.MediaExpiresAtCoordMS, prepare.PrepareDeadlineCoordMS = tc.expiry, tc.deadline
			client.Prepare(&prepare)
			client.Synchronize()
			events := recorder.snapshot()
			fetches, _ := fetcher.snapshot()
			if len(events) != tc.wantEvents || fetches != 0 || (tc.wantCode != "" && events[0].code != tc.wantCode) {
				t.Fatalf("events=%+v fetches=%d", events, fetches)
			}
		})
	}
}

func TestMediaClipStopDisarmsAndCleansPreparedState(t *testing.T) {
	fetcher := &stubMediaClipFetcher{}
	mixer := &stubMediaClipMixer{}
	recorder := &mediaEventRecorder{}
	client := newTestMediaClipClient(fetcher, mixer, recorder, nil, fixedClock{ok: true})
	prepare := testPrepareMedia(1)
	client.Prepare(&prepare)
	waitForMediaEvents(t, recorder, 1)
	client.Stop()
	_, removed := fetcher.snapshot()
	_, cancelCount, disposeCount := mixer.counts()
	if len(removed) != 1 || cancelCount != 1 || disposeCount != 1 {
		t.Fatalf("removed=%v cancel=%d dispose=%d", removed, cancelCount, disposeCount)
	}
	client.Prepare(&prepare)
	client.Synchronize()
	fetches, _ := fetcher.snapshot()
	if fetches != 1 {
		t.Fatalf("stopped client accepted new work: fetches=%d", fetches)
	}
}

func TestPlayerRoutesClipPresenceAndDurableDNDAlongsideLegacyRuntime(t *testing.T) {
	daemon := newFakeDaemon()
	player, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})
	fetcher := &stubMediaClipFetcher{}
	mixer := &stubMediaClipMixer{}
	clips := NewMediaClipClient(fetcher, mixer, testLogger(), func() int64 { return 10_000 })
	store := NewNodePresenceStore(filepath.Join(t.TempDir(), "state.json"), testLogger())
	player.ConfigureTransmissionHooks(clips, store)

	prepare := testPrepareMedia(1)
	player.Handle(protocol.Envelope{Type: protocol.TypePrepareMedia}, &prepare)
	message := expectSent(t, sent, protocol.TypeMediaReady)
	if ready := message.Payload.(*protocol.MediaReadyPayload); ready.TransmissionID != "tr_test" {
		t.Fatalf("ready payload %+v", ready)
	}

	presence := testPresenceUpdate(7)
	player.Handle(protocol.Envelope{Type: protocol.TypePresenceUpdate}, presence)
	waitFor(t, 3*time.Second, func() bool {
		latest := player.LatestPresence()
		return latest != nil && latest.Revision == 7
	}, "presence was not persisted off the WebSocket path")

	if err := player.SetLocalDND("allow_all", nil); err != nil {
		t.Fatal(err)
	}
	dndMessage := expectSent(t, sent, protocol.TypeSetDND)
	if dnd := dndMessage.Payload.(*protocol.SetDNDPayload); dnd.Revision != 1 || dnd.Mode != "allow_all" {
		t.Fatalf("DND payload %+v", dnd)
	}
	player.ResendLocalDND()
	replayed := expectSent(t, sent, protocol.TypeSetDND).Payload.(*protocol.SetDNDPayload)
	if replayed.Revision != 1 || replayed.Mode != "allow_all" {
		t.Fatalf("replayed DND %+v", replayed)
	}
}

func TestPreparedOnlyWindowsMixerAdvertisesNoDeliveryModesAndDecodes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clip.wav")
	if err := os.WriteFile(path, makeWAV16(2, sampleRate, make([]int16, sampleRate*channels/10)), 0o600); err != nil {
		t.Fatal(err)
	}
	mixer := PreparedOnlyWindowsMediaClipMixer{}
	clip, err := mixer.Prepare(path, "overlay")
	if err != nil {
		t.Fatal(err)
	}
	if clip.DecodedDurationMS != 100 || len(mixer.DeliveryCapabilities()) != 0 {
		t.Fatalf("prepared clip duration=%d capabilities=%v", clip.DecodedDurationMS, mixer.DeliveryCapabilities())
	}
	client := NewMediaClipClient(&stubMediaClipFetcher{}, mixer, testLogger(), nil)
	defer client.Stop()
	if got := client.AdvertisedCapabilities(); !reflect.DeepEqual(got, []string{protocol.CapabilityMediaClip}) {
		t.Fatalf("advertised capabilities %v", got)
	}
	play := testOverlayPlay(1)
	if err := mixer.Arm(clip, MediaClipPlayPlan{Payload: play}, func(int64) {}, func(int64) {}); mediaClipFailureCode(err, "") != "capability_lost" {
		t.Fatalf("prepared-only arm error %v", err)
	}
	overlong := filepath.Join(t.TempDir(), "overlong.wav")
	if err := os.WriteFile(overlong, makeWAV16(2, 1_000, make([]int16, 180_001*2)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mixer.Prepare(overlong, "interrupt"); mediaClipFailureCode(err, "") != "duration_mismatch" {
		t.Fatalf("overlong decoder error %v", err)
	}
}

func TestAuthenticatedMediaClipFetcherEnforcesOriginBearerHashAndCleanup(t *testing.T) {
	body := []byte("hello")
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer node-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write(body)
		case "/unauthorized":
			w.WriteHeader(http.StatusUnauthorized)
		case "/gone":
			w.WriteHeader(http.StatusGone)
		case "/redirect":
			http.Redirect(w, r, "/ok", http.StatusFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	directory := filepath.Join(t.TempDir(), "clips")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(directory, ".clip-orphan.media")
	keep := filepath.Join(directory, "unrelated.txt")
	if err := os.WriteFile(orphan, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keep, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	fetcher, err := NewAuthenticatedMediaClipFetcher(directory, "node-token", wsURL(server))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphan); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale owned clip was not removed: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("unrelated cache file was removed: %v", err)
	}
	request := MediaClipFetchRequest{RemoteURL: server.URL + "/ok", ExpectedSHA256: digest, ExpectedSizeBytes: int64(len(body))}
	path, err := fetcher.Fetch(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(body) {
		t.Fatalf("downloaded %q", got)
	}
	fetcher.Remove(path)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned file still exists: %v", err)
	}

	typed := []struct {
		name    string
		request MediaClipFetchRequest
		code    string
	}{
		{name: "auth", request: MediaClipFetchRequest{RemoteURL: server.URL + "/unauthorized", ExpectedSHA256: digest, ExpectedSizeBytes: 5}, code: "media_auth_failed"},
		{name: "expired", request: MediaClipFetchRequest{RemoteURL: server.URL + "/gone", ExpectedSHA256: digest, ExpectedSizeBytes: 5}, code: "media_expired"},
		{name: "redirect", request: MediaClipFetchRequest{RemoteURL: server.URL + "/redirect", ExpectedSHA256: digest, ExpectedSizeBytes: 5}, code: "media_download_failed"},
		{name: "cross origin", request: MediaClipFetchRequest{RemoteURL: "https://other.example/v1/media/m", ExpectedSHA256: digest, ExpectedSizeBytes: 5}, code: "media_auth_failed"},
		{name: "url credentials", request: MediaClipFetchRequest{RemoteURL: strings.Replace(server.URL, "http://", "http://user:pass@", 1) + "/ok", ExpectedSHA256: digest, ExpectedSizeBytes: 5}, code: "media_auth_failed"},
		{name: "hash", request: MediaClipFetchRequest{RemoteURL: server.URL + "/ok", ExpectedSHA256: fmt.Sprintf("%064x", 1), ExpectedSizeBytes: 5}, code: "hash_mismatch"},
		{name: "size", request: MediaClipFetchRequest{RemoteURL: server.URL + "/ok", ExpectedSHA256: digest, ExpectedSizeBytes: 4}, code: "media_download_failed"},
	}
	for _, tc := range typed {
		t.Run(tc.name, func(t *testing.T) {
			_, err := fetcher.Fetch(context.Background(), tc.request)
			if got := mediaClipFailureCode(err, ""); got != tc.code {
				t.Fatalf("error=%v code=%q, want %q", err, got, tc.code)
			}
		})
	}
	mu.Lock()
	requestCount := requests
	mu.Unlock()
	if requestCount != 6 { // Cross-origin and credential-bearing URLs are rejected locally.
		t.Fatalf("server requests=%d, want 6", requestCount)
	}
}

func TestMediaClipFetchRequestFormattingRedactsURLAndHash(t *testing.T) {
	request := MediaClipFetchRequest{RemoteURL: "https://secret.example/path", ExpectedSHA256: "secret-hash", ExpectedSizeBytes: 42}
	formatted := fmt.Sprintf("%v", request)
	if formatted == "" {
		t.Fatalf("unexpected formatting %q", formatted)
	}
	if strings.Contains(formatted, request.RemoteURL) || strings.Contains(formatted, request.ExpectedSHA256) {
		t.Fatalf("sensitive media request leaked: %q", formatted)
	}
}
