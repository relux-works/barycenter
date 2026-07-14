// Generation-safe Windows hooks for the frozen P1 media lifecycle.
//
// The WebSocket reader and future audio callbacks only enqueue small control
// events. Downloads, hashing and decoder setup run on workers; delivery-specific
// render behavior stays behind MediaClipMixer for the later mixer tasks.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

const (
	maximumCanonicalClipBytes int64 = 34 << 20
	maximumP1ClipDurationMS   int64 = 180_000
)

// MediaClipFailure contains only a frozen public reason code. Error strings,
// URLs and local paths never cross the protocol boundary.
type MediaClipFailure struct{ Code string }

func (e *MediaClipFailure) Error() string { return e.Code }

func mediaClipFailure(code string) error { return &MediaClipFailure{Code: code} }

func mediaClipFailureCode(err error, fallback string) string {
	var failure *MediaClipFailure
	if errors.As(err, &failure) && frozenMediaFailureCode(failure.Code) {
		return failure.Code
	}
	return fallback
}

func frozenMediaFailureCode(code string) bool {
	switch code {
	case "media_download_failed", "media_auth_failed", "media_expired", "hash_mismatch",
		"decode_failed", "duration_mismatch", "clock_unsynchronized", "stale_play",
		"device_unavailable", "audio_graph_failed", "connection_lost", "capability_lost",
		"interrupt_capability_lost", "cancel_unacknowledged", "internal_error":
		return true
	default:
		return false
	}
}

type MediaClipFetchRequest struct {
	RemoteURL         string
	ExpectedSHA256    string
	ExpectedSizeBytes int64
}

type MediaClipFetcher interface {
	Fetch(context.Context, MediaClipFetchRequest) (string, error)
	Remove(string)
}

type mediaHTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type mediaHTTPOrigin struct {
	scheme string
	host   string
	port   string
}

func newMediaHTTPOrigin(coordinatorURL string) (mediaHTTPOrigin, error) {
	u, err := url.Parse(coordinatorURL)
	if err != nil || u.Opaque != "" || u.Host == "" || u.User != nil {
		return mediaHTTPOrigin{}, mediaClipFailure("media_auth_failed")
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "wss":
		scheme = "https"
	case "ws":
		scheme = "http"
	case "https", "http":
	default:
		return mediaHTTPOrigin{}, mediaClipFailure("media_auth_failed")
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return mediaHTTPOrigin{}, mediaClipFailure("media_auth_failed")
	}
	return mediaHTTPOrigin{scheme: scheme, host: host, port: effectiveMediaPort(u, scheme)}, nil
}

func effectiveMediaPort(u *url.URL, scheme string) string {
	if port := u.Port(); port != "" {
		return port
	}
	if scheme == "https" {
		return "443"
	}
	return "80"
}

func (o mediaHTTPOrigin) permits(u *url.URL) bool {
	if u == nil || u.Opaque != "" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return scheme == o.scheme && strings.ToLower(u.Hostname()) == o.host &&
		effectiveMediaPort(u, scheme) == o.port
}

// AuthenticatedMediaClipFetcher accepts only the exact HTTP origin derived
// from the authenticated coordinator WebSocket, refuses redirects and streams
// through a hard byte bound while hashing into an owned temporary file.
type AuthenticatedMediaClipFetcher struct {
	dir    string
	token  string
	origin mediaHTTPOrigin
	httpc  mediaHTTPDoer
}

func NewAuthenticatedMediaClipFetcher(cacheDirectory, nodeToken, coordinatorURL string) (*AuthenticatedMediaClipFetcher, error) {
	origin, err := newMediaHTTPOrigin(coordinatorURL)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cacheDirectory, 0o700); err != nil {
		return nil, mediaClipFailure("media_download_failed")
	}
	// Prepared generations are intentionally in-memory only. After a process
	// restart none can be verified, so old owned clip temporaries are stale.
	if entries, readErr := os.ReadDir(cacheDirectory); readErr == nil {
		for _, entry := range entries {
			name := entry.Name()
			if !entry.IsDir() && strings.HasPrefix(name, ".clip-") && strings.HasSuffix(name, ".media") {
				_ = os.Remove(filepath.Join(cacheDirectory, name))
			}
		}
	}
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &AuthenticatedMediaClipFetcher{
		dir: filepath.Clean(cacheDirectory), token: nodeToken, origin: origin, httpc: client,
	}, nil
}

func (f *AuthenticatedMediaClipFetcher) Fetch(ctx context.Context, request MediaClipFetchRequest) (path string, resultErr error) {
	if request.ExpectedSizeBytes <= 0 || request.ExpectedSizeBytes > maximumCanonicalClipBytes {
		return "", mediaClipFailure("media_download_failed")
	}
	if !validLowerSHA256(request.ExpectedSHA256) {
		return "", mediaClipFailure("hash_mismatch")
	}
	remote, err := url.Parse(request.RemoteURL)
	if err != nil || !f.origin.permits(remote) {
		return "", mediaClipFailure("media_auth_failed")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remote.String(), nil)
	if err != nil {
		return "", mediaClipFailure("media_auth_failed")
	}
	req.Header.Set("Authorization", "Bearer "+f.token)
	req.Header.Set("Accept", "application/octet-stream, audio/wav")
	resp, err := f.httpc.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", mediaClipFailure("media_download_failed")
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden:
		drainMediaResponse(resp.Body)
		return "", mediaClipFailure("media_auth_failed")
	case http.StatusNotFound, http.StatusGone:
		drainMediaResponse(resp.Body)
		return "", mediaClipFailure("media_expired")
	default:
		drainMediaResponse(resp.Body)
		return "", mediaClipFailure("media_download_failed")
	}
	if resp.ContentLength >= 0 && resp.ContentLength != request.ExpectedSizeBytes {
		return "", mediaClipFailure("media_download_failed")
	}

	file, err := os.CreateTemp(f.dir, ".clip-*.media")
	if err != nil {
		return "", mediaClipFailure("media_download_failed")
	}
	path = file.Name()
	remove := true
	defer func() {
		closeErr := file.Close()
		if resultErr == nil && closeErr != nil {
			resultErr = mediaClipFailure("media_download_failed")
		}
		if resultErr != nil || remove {
			_ = os.Remove(path)
			path = ""
		}
	}()
	_ = file.Chmod(0o600)

	hasher := sha256.New()
	limited := &io.LimitedReader{R: resp.Body, N: request.ExpectedSizeBytes + 1}
	written, err := io.Copy(io.MultiWriter(file, hasher), limited)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", mediaClipFailure("media_download_failed")
	}
	if written != request.ExpectedSizeBytes {
		return "", mediaClipFailure("media_download_failed")
	}
	if hex.EncodeToString(hasher.Sum(nil)) != request.ExpectedSHA256 {
		return "", mediaClipFailure("hash_mismatch")
	}
	if err := file.Sync(); err != nil {
		return "", mediaClipFailure("media_download_failed")
	}
	remove = false
	return path, nil
}

func validLowerSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, ch := range []byte(value) {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func drainMediaResponse(body io.Reader) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 1<<20))
}

func (f *AuthenticatedMediaClipFetcher) Remove(path string) {
	clean := filepath.Clean(path)
	if clean == "" || filepath.Dir(clean) != f.dir {
		return
	}
	_ = os.Remove(clean)
}

// PreparedMediaClip owns decoder-ready, immutable state. The current mixer
// keeps decoded PCM here; later render tasks can replace Decoder with their
// preallocated engine state without changing protocol ownership.
type PreparedMediaClip struct {
	LocalPath         string
	DecodedDurationMS int64
	Decoder           any
}

type MediaClipPlayPlan struct {
	Payload              protocol.PlayMediaAtPayload
	LocalStartMS         int64
	LocalStartDeadlineMS int64
}

type MediaClipMixer interface {
	DeliveryCapabilities() []string
	Prepare(localPath, delivery string) (*PreparedMediaClip, error)
	Arm(*PreparedMediaClip, MediaClipPlayPlan, func(localMS int64), func(localMS int64)) error
	Cancel(*PreparedMediaClip, protocol.CancelMediaPayload, func(mainResumed bool, err error))
	Dispose(*PreparedMediaClip)
}

// PreparedOnlyWindowsMediaClipMixer proves that authenticated bytes are
// decodable and prebuilds engine-format PCM. It intentionally cannot arm
// overlay or interrupt until the dedicated render/mixer tasks land.
type PreparedOnlyWindowsMediaClipMixer struct{}

func (PreparedOnlyWindowsMediaClipMixer) DeliveryCapabilities() []string { return nil }

func (PreparedOnlyWindowsMediaClipMixer) Prepare(localPath, delivery string) (*PreparedMediaClip, error) {
	if delivery != "overlay" && delivery != "interrupt" {
		return nil, mediaClipFailure("decode_failed")
	}
	raw, err := os.ReadFile(localPath)
	if err != nil {
		return nil, mediaClipFailure("decode_failed")
	}
	decoded, err := parseWAV(raw)
	if err != nil || decoded.channels <= 0 || decoded.sampleRate <= 0 {
		return nil, mediaClipFailure("decode_failed")
	}
	decodedFrames := int64(len(decoded.samples) / decoded.channels)
	duration := (decodedFrames*1000 + int64(decoded.sampleRate) - 1) / int64(decoded.sampleRate)
	if duration <= 0 || duration > maximumP1ClipDurationMS {
		return nil, mediaClipFailure("duration_mismatch")
	}
	samples := toEngineFormat(decoded)
	if len(samples) < channels {
		return nil, mediaClipFailure("decode_failed")
	}
	return &PreparedMediaClip{LocalPath: localPath, DecodedDurationMS: duration, Decoder: samples}, nil
}

func (PreparedOnlyWindowsMediaClipMixer) Arm(_ *PreparedMediaClip, plan MediaClipPlayPlan, _ func(int64), _ func(int64)) error {
	if plan.Payload.Delivery == "interrupt" {
		return mediaClipFailure("interrupt_capability_lost")
	}
	return mediaClipFailure("capability_lost")
}

func (PreparedOnlyWindowsMediaClipMixer) Cancel(_ *PreparedMediaClip, _ protocol.CancelMediaPayload, done func(bool, error)) {
	done(false, nil)
}

func (PreparedOnlyWindowsMediaClipMixer) Dispose(clip *PreparedMediaClip) {
	clip.Decoder = nil
}

type mediaClipPhase uint8

const (
	mediaClipPreparing mediaClipPhase = iota
	mediaClipReady
	mediaClipArmed
	mediaClipPlaying
	mediaClipCancelling
	mediaClipTerminal
)

type mediaClipEntry struct {
	transmissionID  string
	generation      int64
	prepare         *protocol.PrepareMediaPayload
	play            *protocol.PlayMediaAtPayload
	phase           mediaClipPhase
	cancelPrepare   context.CancelFunc
	prepared        *PreparedMediaClip
	deadlineTimer   *time.Timer
	localDeadlineMS int64
	startedSent     bool
	terminalSent    bool
}

type mediaClipWork func() bool

// MediaClipClient owns its lifecycle map on an unbounded serial control
// queue. Enqueue takes only a short mutex and never waits for network, disk,
// decoder, WebSocket or mixer work.
type MediaClipClient struct {
	fetcher MediaClipFetcher
	mixer   MediaClipMixer
	clock   deadlineClock
	send    func(string, any)
	log     *slog.Logger
	nowMS   func() int64

	outputLatencyOffsetMS int
	entries               map[string]*mediaClipEntry

	queueMu   sync.Mutex
	queue     []mediaClipWork
	wake      chan struct{}
	loopDone  chan struct{}
	accepting bool
	stopOnce  sync.Once
}

func NewMediaClipClient(fetcher MediaClipFetcher, mixer MediaClipMixer, log *slog.Logger, now func() int64) *MediaClipClient {
	if now == nil {
		now = nowMS
	}
	c := &MediaClipClient{
		fetcher: fetcher, mixer: mixer, log: log, nowMS: now,
		entries: map[string]*mediaClipEntry{}, wake: make(chan struct{}, 1),
		loopDone: make(chan struct{}), accepting: true,
	}
	go c.controlLoop()
	return c
}

func (c *MediaClipClient) AdvertisedCapabilities() []string {
	set := map[string]struct{}{protocol.CapabilityMediaClip: {}}
	for _, capability := range c.mixer.DeliveryCapabilities() {
		set[capability] = struct{}{}
	}
	values := make([]string, 0, len(set))
	for capability := range set {
		values = append(values, capability)
	}
	sort.Strings(values)
	return values
}

func (c *MediaClipClient) Bind(clock deadlineClock, send func(string, any), outputLatencyOffsetMS int) {
	c.post(func() bool {
		c.clock = clock
		c.send = send
		c.outputLatencyOffsetMS = outputLatencyOffsetMS
		return false
	})
}

func (c *MediaClipClient) SetOutputLatencyOffsetMS(value int) {
	c.post(func() bool { c.outputLatencyOffsetMS = value; return false })
}

func (c *MediaClipClient) Prepare(payload *protocol.PrepareMediaPayload) {
	if payload == nil {
		return
	}
	copyPayload := *payload
	c.post(func() bool { c.beginPrepare(copyPayload); return false })
}

func (c *MediaClipClient) Play(payload *protocol.PlayMediaAtPayload) {
	if payload == nil {
		return
	}
	copyPayload := clonePlayMediaPayload(*payload)
	c.post(func() bool { c.beginPlay(copyPayload); return false })
}

func (c *MediaClipClient) Cancel(payload *protocol.CancelMediaPayload) {
	if payload == nil {
		return
	}
	copyPayload := *payload
	c.post(func() bool { c.beginCancel(copyPayload); return false })
}

func (c *MediaClipClient) Synchronize() {
	done := make(chan struct{})
	if !c.post(func() bool { close(done); return false }) {
		return
	}
	<-done
}

func (c *MediaClipClient) Stop() {
	c.stopOnce.Do(func() {
		done := make(chan struct{})
		c.queueMu.Lock()
		c.accepting = false
		c.queue = append(c.queue, func() bool {
			for _, entry := range c.entries {
				c.discard(entry)
			}
			c.entries = map[string]*mediaClipEntry{}
			close(done)
			return true
		})
		c.queueMu.Unlock()
		c.signal()
		<-done
		<-c.loopDone
	})
}

func (c *MediaClipClient) post(work mediaClipWork) bool {
	c.queueMu.Lock()
	if !c.accepting {
		c.queueMu.Unlock()
		return false
	}
	c.queue = append(c.queue, work)
	c.queueMu.Unlock()
	c.signal()
	return true
}

func (c *MediaClipClient) signal() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *MediaClipClient) controlLoop() {
	defer close(c.loopDone)
	for range c.wake {
		for {
			c.queueMu.Lock()
			if len(c.queue) == 0 {
				c.queueMu.Unlock()
				break
			}
			work := c.queue[0]
			c.queue[0] = nil
			c.queue = c.queue[1:]
			c.queueMu.Unlock()
			if work() {
				return
			}
		}
	}
}

func (c *MediaClipClient) beginPrepare(payload protocol.PrepareMediaPayload) {
	if !validPrepareMedia(payload) {
		c.sendFailure(payload.TransmissionID, payload.Generation, "prepare", "internal_error")
		return
	}
	if current := c.entries[payload.TransmissionID]; current != nil {
		if payload.Generation < current.generation {
			return
		}
		if payload.Generation == current.generation {
			if current.prepare == nil || reflect.DeepEqual(*current.prepare, payload) {
				return
			}
			c.failAfterStopping(current, "prepare", "internal_error")
			return
		}
		c.discard(current)
	}

	copyPayload := payload
	entry := &mediaClipEntry{
		transmissionID: payload.TransmissionID, generation: payload.Generation,
		prepare: &copyPayload, phase: mediaClipPreparing,
	}
	c.entries[payload.TransmissionID] = entry
	nowCoord := c.estimatedCoordinatorNowMS()
	if nowCoord >= payload.MediaExpiresAtCoordMS {
		c.fail(entry, "prepare", "media_expired")
		return
	}
	if nowCoord >= payload.PrepareDeadlineCoordMS {
		c.abandonLatePrepare(entry)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	entry.cancelPrepare = cancel
	request := MediaClipFetchRequest{
		RemoteURL: payload.FileURL, ExpectedSHA256: payload.SHA256,
		ExpectedSizeBytes: payload.SizeBytes,
	}
	go c.prepareWorker(ctx, entry, payload, request)
}

func (c *MediaClipClient) prepareWorker(ctx context.Context, entry *mediaClipEntry, payload protocol.PrepareMediaPayload, request MediaClipFetchRequest) {
	path, err := c.fetcher.Fetch(ctx, request)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		code := mediaClipFailureCode(err, "media_download_failed")
		c.post(func() bool { c.failIfCurrent(entry, "prepare", code); return false })
		return
	}
	if ctx.Err() != nil {
		c.fetcher.Remove(path)
		return
	}
	prepared, err := c.mixer.Prepare(path, payload.Delivery)
	if err != nil {
		c.fetcher.Remove(path)
		if ctx.Err() != nil {
			return
		}
		code := mediaClipFailureCode(err, "decode_failed")
		c.post(func() bool { c.failIfCurrent(entry, "prepare", code); return false })
		return
	}
	if !c.post(func() bool { c.completePrepare(entry, prepared); return false }) {
		c.mixer.Dispose(prepared)
		c.fetcher.Remove(prepared.LocalPath)
	}
}

func (c *MediaClipClient) completePrepare(entry *mediaClipEntry, prepared *PreparedMediaClip) {
	if !c.isCurrent(entry) || entry.phase != mediaClipPreparing || entry.prepare == nil {
		c.mixer.Dispose(prepared)
		c.fetcher.Remove(prepared.LocalPath)
		return
	}
	entry.cancelPrepare = nil
	payload := entry.prepare
	nowCoord := c.estimatedCoordinatorNowMS()
	entry.prepared = prepared
	if nowCoord >= payload.MediaExpiresAtCoordMS {
		c.fail(entry, "prepare", "media_expired")
		return
	}
	if nowCoord >= payload.PrepareDeadlineCoordMS {
		c.abandonLatePrepare(entry)
		return
	}
	if prepared.DecodedDurationMS != payload.DurationMS {
		c.fail(entry, "prepare", "duration_mismatch")
		return
	}
	entry.phase = mediaClipReady
	c.sendMessage(protocol.TypeMediaReady, &protocol.MediaReadyPayload{
		TransmissionID: entry.transmissionID, Generation: entry.generation,
		DecodedDurationMS: prepared.DecodedDurationMS,
	})
}

func (c *MediaClipClient) beginPlay(payload protocol.PlayMediaAtPayload) {
	entry := c.entries[payload.TransmissionID]
	if entry == nil || entry.generation != payload.Generation {
		return
	}
	if entry.phase == mediaClipArmed || entry.phase == mediaClipPlaying {
		if entry.play == nil || !reflect.DeepEqual(*entry.play, payload) {
			c.failAfterStopping(entry, "schedule", "internal_error")
		}
		return
	}
	if entry.phase != mediaClipReady || entry.prepared == nil || entry.prepare == nil ||
		entry.prepare.Delivery != payload.Delivery || !validPlayMedia(payload) {
		return
	}

	required := protocol.CapabilityOverlayMix
	missingCode := "capability_lost"
	if payload.Delivery == "interrupt" {
		required = protocol.CapabilityInterruptResume
		missingCode = "interrupt_capability_lost"
	}
	if !containsCapability(c.mixer.DeliveryCapabilities(), required) {
		c.fail(entry, "schedule", missingCode)
		return
	}
	if c.clock == nil {
		c.fail(entry, "schedule", "clock_unsynchronized")
		return
	}
	localStart, startOK := c.clock.LocalDeadline(payload.TCoordMS, c.outputLatencyOffsetMS)
	localDeadline, deadlineOK := c.clock.LocalDeadline(payload.StartDeadlineCoordMS, c.outputLatencyOffsetMS)
	if !startOK || !deadlineOK {
		c.fail(entry, "schedule", "clock_unsynchronized")
		return
	}
	if c.estimatedCoordinatorNowMS() >= entry.prepare.MediaExpiresAtCoordMS || c.nowMS() > localDeadline {
		c.fail(entry, "schedule", "stale_play")
		return
	}

	copyPayload := clonePlayMediaPayload(payload)
	entry.play = &copyPayload
	entry.phase = mediaClipArmed
	entry.localDeadlineMS = localDeadline
	c.armDeadline(entry, localDeadline)
	plan := MediaClipPlayPlan{
		Payload: copyPayload, LocalStartMS: localStart, LocalStartDeadlineMS: localDeadline,
	}
	err := c.mixer.Arm(entry.prepared, plan,
		func(localMS int64) { c.post(func() bool { c.handleStarted(entry, localMS); return false }) },
		func(localMS int64) { c.post(func() bool { c.handleEnded(entry, localMS); return false }) })
	if err != nil {
		entry.phase = mediaClipReady
		c.fail(entry, "schedule", mediaClipFailureCode(err, "audio_graph_failed"))
	}
}

func (c *MediaClipClient) handleStarted(entry *mediaClipEntry, localMS int64) {
	if !c.isCurrent(entry) || entry.phase != mediaClipArmed || entry.startedSent {
		return
	}
	if localMS > entry.localDeadlineMS {
		c.failAfterStopping(entry, "schedule", "stale_play")
		return
	}
	if entry.deadlineTimer != nil {
		entry.deadlineTimer.Stop()
		entry.deadlineTimer = nil
	}
	entry.phase = mediaClipPlaying
	entry.startedSent = true
	c.sendMessage(protocol.TypeMediaStarted, &protocol.MediaStartedPayload{
		TransmissionID: entry.transmissionID, Generation: entry.generation,
		TFirstSampleCoordMS: c.coordinatorTimestamp(localMS),
	})
}

func (c *MediaClipClient) handleEnded(entry *mediaClipEntry, localMS int64) {
	if !c.isCurrent(entry) || entry.phase != mediaClipPlaying || !entry.startedSent || entry.terminalSent {
		return
	}
	c.finish(entry, protocol.TypeMediaEnded, &protocol.MediaEndedPayload{
		TransmissionID: entry.transmissionID, Generation: entry.generation,
		TLastSampleCoordMS: c.coordinatorTimestamp(localMS), Reason: "completed",
	})
}

func (c *MediaClipClient) beginCancel(payload protocol.CancelMediaPayload) {
	if payload.Generation <= 0 || (payload.Action != "disarm" && payload.Action != "fade_stop") {
		c.sendFailure(payload.TransmissionID, payload.Generation, "cancel", "internal_error")
		return
	}
	current := c.entries[payload.TransmissionID]
	if current == nil {
		c.installCancelTombstone(payload)
		return
	}
	if payload.Generation < current.generation {
		return
	}
	if payload.Generation > current.generation {
		c.discard(current)
		c.installCancelTombstone(payload)
		return
	}
	if current.phase == mediaClipTerminal {
		if !current.terminalSent {
			current.terminalSent = true
			c.sendMessage(protocol.TypeMediaCancelled, &protocol.MediaCancelledPayload{
				TransmissionID: payload.TransmissionID, Generation: payload.Generation,
				Reason: payload.Reason, Action: payload.Action, MainResumed: false,
			})
		}
		return
	}
	if current.phase == mediaClipCancelling {
		return
	}
	if current.cancelPrepare != nil {
		current.cancelPrepare()
		current.cancelPrepare = nil
	}
	if current.deadlineTimer != nil {
		current.deadlineTimer.Stop()
		current.deadlineTimer = nil
	}
	if current.prepared == nil {
		c.finish(current, protocol.TypeMediaCancelled, &protocol.MediaCancelledPayload{
			TransmissionID: payload.TransmissionID, Generation: payload.Generation,
			Reason: payload.Reason, Action: payload.Action, MainResumed: false,
		})
		return
	}
	current.phase = mediaClipCancelling
	c.mixer.Cancel(current.prepared, payload, func(mainResumed bool, err error) {
		c.post(func() bool {
			if !c.isCurrent(current) || current.phase != mediaClipCancelling {
				return false
			}
			if err != nil {
				c.fail(current, "cancel", mediaClipFailureCode(err, "internal_error"))
				return false
			}
			c.finish(current, protocol.TypeMediaCancelled, &protocol.MediaCancelledPayload{
				TransmissionID: payload.TransmissionID, Generation: payload.Generation,
				Reason: payload.Reason, Action: payload.Action, MainResumed: mainResumed,
			})
			return false
		})
	})
}

func (c *MediaClipClient) installCancelTombstone(payload protocol.CancelMediaPayload) {
	entry := &mediaClipEntry{
		transmissionID: payload.TransmissionID, generation: payload.Generation,
		phase: mediaClipTerminal, terminalSent: true,
	}
	c.entries[payload.TransmissionID] = entry
	c.sendMessage(protocol.TypeMediaCancelled, &protocol.MediaCancelledPayload{
		TransmissionID: payload.TransmissionID, Generation: payload.Generation,
		Reason: payload.Reason, Action: payload.Action, MainResumed: false,
	})
}

func (c *MediaClipClient) armDeadline(entry *mediaClipEntry, localDeadline int64) {
	delayMS := localDeadline - c.nowMS()
	if delayMS < 0 {
		delayMS = 0
	}
	maximumTimerMS := int64(math.MaxInt64 / int64(time.Millisecond))
	if delayMS > maximumTimerMS {
		delayMS = maximumTimerMS
	}
	entry.deadlineTimer = time.AfterFunc(time.Duration(delayMS)*time.Millisecond, func() {
		c.post(func() bool {
			if c.isCurrent(entry) && entry.phase == mediaClipArmed {
				c.failAfterStopping(entry, "schedule", "stale_play")
			}
			return false
		})
	})
}

func (c *MediaClipClient) failIfCurrent(entry *mediaClipEntry, stage, code string) {
	if c.isCurrent(entry) && entry.phase != mediaClipTerminal {
		c.fail(entry, stage, code)
	}
}

func (c *MediaClipClient) fail(entry *mediaClipEntry, stage, code string) {
	if entry.terminalSent {
		return
	}
	c.finish(entry, protocol.TypeMediaFailed, &protocol.MediaFailedPayload{
		TransmissionID: entry.transmissionID, Generation: entry.generation,
		Stage: stage, Code: code,
	})
}

func (c *MediaClipClient) failAfterStopping(entry *mediaClipEntry, stage, code string) {
	if (entry.phase != mediaClipArmed && entry.phase != mediaClipPlaying) || entry.prepared == nil {
		c.fail(entry, stage, code)
		return
	}
	wasPlaying := entry.phase == mediaClipPlaying
	entry.phase = mediaClipCancelling
	if entry.deadlineTimer != nil {
		entry.deadlineTimer.Stop()
		entry.deadlineTimer = nil
	}
	command := protocol.CancelMediaPayload{
		TransmissionID: entry.transmissionID, Generation: entry.generation,
		Reason: "coordinator_restarted", Action: "disarm",
	}
	if wasPlaying {
		command.Action = "fade_stop"
		command.ResumeMain = entry.play != nil && entry.play.Delivery == "interrupt"
	}
	c.mixer.Cancel(entry.prepared, command, func(_ bool, _ error) {
		c.post(func() bool {
			if c.isCurrent(entry) && entry.phase == mediaClipCancelling {
				c.fail(entry, stage, code)
			}
			return false
		})
	})
}

func (c *MediaClipClient) finish(entry *mediaClipEntry, messageType string, payload any) {
	if entry.terminalSent {
		return
	}
	entry.terminalSent = true
	entry.phase = mediaClipTerminal
	if entry.cancelPrepare != nil {
		entry.cancelPrepare()
		entry.cancelPrepare = nil
	}
	if entry.deadlineTimer != nil {
		entry.deadlineTimer.Stop()
		entry.deadlineTimer = nil
	}
	if entry.prepared != nil {
		c.mixer.Dispose(entry.prepared)
		c.fetcher.Remove(entry.prepared.LocalPath)
		entry.prepared = nil
	}
	c.sendMessage(messageType, payload)
}

func (c *MediaClipClient) abandonLatePrepare(entry *mediaClipEntry) {
	entry.phase = mediaClipTerminal
	if entry.cancelPrepare != nil {
		entry.cancelPrepare()
		entry.cancelPrepare = nil
	}
	if entry.prepared != nil {
		c.mixer.Dispose(entry.prepared)
		c.fetcher.Remove(entry.prepared.LocalPath)
		entry.prepared = nil
	}
	c.log.Debug("late media prepare discarded",
		"transmission_id", entry.transmissionID, "generation", entry.generation)
}

func (c *MediaClipClient) discard(entry *mediaClipEntry) {
	if entry.cancelPrepare != nil {
		entry.cancelPrepare()
		entry.cancelPrepare = nil
	}
	if entry.deadlineTimer != nil {
		entry.deadlineTimer.Stop()
		entry.deadlineTimer = nil
	}
	prepared := entry.prepared
	entry.prepared = nil
	wasPlaying := entry.phase == mediaClipPlaying
	entry.phase = mediaClipTerminal
	if prepared == nil {
		return
	}
	command := protocol.CancelMediaPayload{
		TransmissionID: entry.transmissionID, Generation: entry.generation,
		Reason: "coordinator_restarted", Action: "disarm",
	}
	if wasPlaying {
		command.Action = "fade_stop"
		command.ResumeMain = entry.play != nil && entry.play.Delivery == "interrupt"
	}
	c.mixer.Cancel(prepared, command, func(bool, error) {
		c.mixer.Dispose(prepared)
		c.fetcher.Remove(prepared.LocalPath)
	})
}

func (c *MediaClipClient) isCurrent(entry *mediaClipEntry) bool {
	return c.entries[entry.transmissionID] == entry
}

func (c *MediaClipClient) estimatedCoordinatorNowMS() int64 {
	return c.coordinatorTimestamp(c.nowMS())
}

func (c *MediaClipClient) coordinatorTimestamp(localMS int64) int64 {
	if c.clock == nil {
		return localMS
	}
	offset, ok := c.clock.OffsetMS()
	if !ok {
		return localMS
	}
	return localMS - int64(math.Round(offset))
}

func (c *MediaClipClient) sendFailure(transmissionID string, generation int64, stage, code string) {
	c.sendMessage(protocol.TypeMediaFailed, &protocol.MediaFailedPayload{
		TransmissionID: transmissionID, Generation: generation, Stage: stage, Code: code,
	})
}

func (c *MediaClipClient) sendMessage(messageType string, payload any) {
	if c.send != nil {
		c.send(messageType, payload)
	}
}

func validPrepareMedia(payload protocol.PrepareMediaPayload) bool {
	return payload.Generation > 0 && payload.TransmissionID != "" && payload.MediaID != "" &&
		(payload.Kind == "voice_clip" || payload.Kind == "audio_clip" || payload.Kind == "builtin_cue") &&
		(payload.Delivery == "overlay" || payload.Delivery == "interrupt") &&
		payload.SizeBytes > 0 && payload.SizeBytes <= maximumCanonicalClipBytes &&
		payload.DurationMS > 0 && payload.DurationMS <= maximumP1ClipDurationMS &&
		(payload.Delivery != "overlay" || payload.DurationMS <= 60_000) &&
		payload.MediaExpiresAtCoordMS > 0 && payload.PrepareDeadlineCoordMS > 0 &&
		payload.PrepareDeadlineCoordMS <= payload.MediaExpiresAtCoordMS && validLowerSHA256(payload.SHA256)
}

func validPlayMedia(payload protocol.PlayMediaAtPayload) bool {
	if payload.TCoordMS <= 0 || payload.StartDeadlineCoordMS-payload.TCoordMS != 100 ||
		payload.StartDeadlineCoordMS < payload.TCoordMS {
		return false
	}
	switch payload.Delivery {
	case "overlay":
		return payload.DuckDB != nil && !math.IsNaN(*payload.DuckDB) && !math.IsInf(*payload.DuckDB, 0) &&
			*payload.DuckDB <= 0 && payload.AttackMS != nil && *payload.AttackMS >= 0 &&
			payload.ReleaseMS != nil && *payload.ReleaseMS >= 0 &&
			payload.FadeOutMS == nil && payload.FadeInMS == nil
	case "interrupt":
		return payload.DuckDB == nil && payload.AttackMS == nil && payload.ReleaseMS == nil &&
			payload.FadeOutMS != nil && *payload.FadeOutMS >= 0 &&
			payload.FadeInMS != nil && *payload.FadeInMS >= 0
	default:
		return false
	}
}

func clonePlayMediaPayload(value protocol.PlayMediaAtPayload) protocol.PlayMediaAtPayload {
	cloneFloat := func(value *float64) *float64 {
		if value == nil {
			return nil
		}
		copyValue := *value
		return &copyValue
	}
	cloneInt := func(value *int64) *int64 {
		if value == nil {
			return nil
		}
		copyValue := *value
		return &copyValue
	}
	value.DuckDB = cloneFloat(value.DuckDB)
	value.AttackMS = cloneInt(value.AttackMS)
	value.ReleaseMS = cloneInt(value.ReleaseMS)
	value.FadeOutMS = cloneInt(value.FadeOutMS)
	value.FadeInMS = cloneInt(value.FadeInMS)
	return value
}

func containsCapability(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func (r MediaClipFetchRequest) String() string {
	return fmt.Sprintf("MediaClipFetchRequest{size:%d sha256:<redacted> url:<redacted>}", r.ExpectedSizeBytes)
}

func (r MediaClipFetchRequest) GoString() string { return r.String() }
