// Player: executes coordinator commands on this node — the port of the
// macOS PlayerCore (spec 6.2 item 5, mechanics 6.3):
//   - load = two-step (play paused via daemon HTTP + seek), then ready
//   - resume_at = fire daemon resume at T_local = T_coord + offset - latency
//     (delay computed once, armed on Go's monotonic timer)
//   - audible_position = daemon position anchor - ring fill (drain counter)
//   - pause/stop with raised-cosine fades on the engine's music branch
//   - play_voice (cache download -> WAV decode -> engine voice insert),
//     wait (timer -> wait_ended), offset_test (scheduled clicks)
//   - daemon /events wiring: metadata/seek anchors, paused/playing fades,
//     external volume, shared-air Spotify selections, ended-after-ring-drain
//
// Not ported: solo_voice (phase 2 on the mac node too) and the 2 s /status
// position polling (the /events metadata anchors cover it here).
package main

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

type Playback string

const (
	PlaybackStopped Playback = "stopped"
	PlaybackLoading Playback = "loading"
	PlaybackPaused  Playback = "paused"
	PlaybackPlaying Playback = "playing"
	PlaybackVoice   Playback = "voice"
	PlaybackWait    Playback = "wait"
)

// daemonAPI is what the player needs from the go-librespot client
// (interface so tests inject a fake daemon).
type daemonAPI interface {
	PlaybackReady(ctx context.Context) bool
	Status(ctx context.Context) (DaemonStatus, error)
	PlayPaused(ctx context.Context, uri string) error
	Seek(ctx context.Context, positionMS int64) error
	Resume(ctx context.Context) error
	Pause(ctx context.Context) error
	Stop(ctx context.Context) error
	AddToQueue(ctx context.Context, uri string) error
}

// deadlineClock is the ClockSync surface the player consumes.
type deadlineClock interface {
	LocalDeadline(tCoordMS int64, outputLatencyOffsetMS int) (int64, bool)
	OffsetMS() (float64, bool)
}

type Player struct {
	daemon daemonAPI
	ring   *Ring
	engine *Engine
	cache  *VoiceCache
	clock  deadlineClock
	send   func(msgType string, payload any)
	log    *slog.Logger

	// Poll knobs (shrunk by tests). Defaults mirror the macOS PlayerCore:
	// ready 20x500ms, confirm 10x300ms, play retry after 2 s.
	readyPollInterval   time.Duration
	readyPollAttempts   int
	confirmPollInterval time.Duration
	confirmPollAttempts int
	// Watcher knobs (shrunk by tests): drain 100 ms, telemetry 1 s,
	// Spotify-selection debounce 5 s — the macOS timer values.
	drainInterval     time.Duration
	telemetryInterval time.Duration
	externalDebounce  time.Duration

	underruns atomic.Int64
	done      chan struct{}
	closeOnce sync.Once

	mu                    sync.Mutex
	mode                  string
	playback              Playback
	elementID             string
	uri                   string
	volume                int
	outputLatencyOffsetMS int
	anchorPosMS           int64
	anchorAt              time.Time
	extrapolate           bool
	loadGen               int64 // stale-load guard (a newer command wins)
	resumeTimer           *time.Timer
	// pausedLocally (personal pause, 2026-07-10): the USER paused this Pulsar
	// in the Spotify app while the shared air was playing. Cleared by any
	// coordinator ownership act (load / pause command / stop / mode switch).
	pausedLocally      bool
	waitTimer          *time.Timer
	pauseTimer         *time.Timer
	startedPending     bool
	draining           bool // daemon says ended; ring tail still sounding
	lastExternalReport time.Time
	lastExternalURI    string
	metadataURI        string
	metadataPosition   *int64
	metadataTitle      string
	speakerName        string
}

func NewPlayer(daemon daemonAPI, ring *Ring, engine *Engine, cache *VoiceCache,
	clock deadlineClock, send func(msgType string, payload any),
	outputLatencyOffsetMS int, log *slog.Logger) *Player {
	p := &Player{
		daemon:                daemon,
		ring:                  ring,
		engine:                engine,
		cache:                 cache,
		clock:                 clock,
		send:                  send,
		log:                   log,
		readyPollInterval:     500 * time.Millisecond,
		readyPollAttempts:     20,
		confirmPollInterval:   300 * time.Millisecond,
		confirmPollAttempts:   10,
		drainInterval:         100 * time.Millisecond,
		telemetryInterval:     time.Second,
		externalDebounce:      5 * time.Second,
		done:                  make(chan struct{}),
		mode:                  "shared",
		playback:              PlaybackStopped,
		volume:                80,
		outputLatencyOffsetMS: outputLatencyOffsetMS,
		speakerName:           "Default output",
	}
	return p
}

// Start launches the background watchers (drain -> ended, dropout
// telemetry). Separate from NewPlayer so callers/tests can tune the
// interval knobs first.
func (p *Player) Start() {
	go p.drainWatch()
	go p.telemetryWatch()
}

// Close stops the background watchers (shutdown / test hygiene).
func (p *Player) Close() {
	p.closeOnce.Do(func() { close(p.done) })
}

// Handle dispatches one incoming coordinator envelope (spec 8.3).
func (p *Player) Handle(env protocol.Envelope, payload any) {
	switch m := payload.(type) {
	case *protocol.LoadPayload:
		p.load(m)
	case *protocol.ResumeAtPayload:
		p.resumeAt(m)
	case *protocol.PausePayload:
		p.pauseCmd(m)
	case *protocol.SeekPayload:
		p.seekCmd(m)
	case *protocol.PlayVoicePayload:
		p.playVoice(m)
	case *protocol.WaitPayload:
		p.waitCmd(m)
	case *protocol.SetVolumePayload:
		p.SetVolume(m.Volume)
	case *protocol.SetModePayload:
		p.setMode(m.Mode)
	case *protocol.StopPayload:
		p.stopAll()
	case *protocol.SetOffsetPayload:
		p.setOffset(m)
	case *protocol.OffsetTestPayload:
		p.offsetTest(m)
	case *protocol.SoloInjectPayload:
		go p.daemon.AddToQueue(context.Background(), m.URI)
	case *protocol.SoloVoicePayload:
		// Phase 2 (goal §8): boundary interception lands with solo scope.
		p.log.Warn("solo_voice not implemented until phase 2", "element", m.ElementID)
	case *protocol.PrepareMediaPayload, *protocol.PlayMediaAtPayload, *protocol.CancelMediaPayload:
		// Dedicated client hooks land later. This build does not advertise
		// media_clip_v1, so receiving a clip command is a coordinator routing
		// error and must never fall back locally to legacy play_voice.
		p.log.Warn("ignoring unadvertised clip command", "type", env.Type)
	default:
		p.log.Debug("ignoring non-command", "type", env.Type)
	}
}

// ApplyWelcome reconciles the session snapshot after (re)connect (spec 8.6).
// Scope: adopt the broadcast volume and mode; the coordinator re-issues
// load/resume for anything in flight.
func (p *Player) ApplyWelcome(w *protocol.WelcomePayload) {
	p.SetVolume(w.SessionSnapshot.Volume)
	p.mu.Lock()
	p.mode = w.SessionSnapshot.Mode
	p.mu.Unlock()
	p.log.Info("welcome",
		"mode", w.SessionSnapshot.Mode, "state", w.SessionSnapshot.State,
		"volume", w.SessionSnapshot.Volume)
}

// --- commands ---

func (p *Player) load(m *protocol.LoadPayload) {
	p.mu.Lock()
	p.cancelTimersLocked()
	p.pausedLocally = false
	p.loadGen++
	gen := p.loadGen
	p.elementID = m.ElementID
	p.uri = m.URI
	p.startedPending = false
	p.draining = false
	p.anchorPosMS = m.PositionMS
	p.anchorAt = time.Now()
	if m.AdoptPlaying {
		p.playback = PlaybackPlaying
		p.extrapolate = true
		p.mu.Unlock()
		p.engine.StopVoice()
		p.engine.gain.SetMusicGain(1, 0)
		p.engine.SetExpectingMusic(true)
		p.send(protocol.TypeReady, &protocol.ReadyPayload{ElementID: m.ElementID})
		return
	}
	p.playback = PlaybackLoading
	p.extrapolate = false
	p.mu.Unlock()
	// The producer is the daemon feeding the pipe; a fresh load means the old
	// element's tail must not sound (spec 6.3).
	p.ring.Clear()
	p.engine.StopVoice()
	p.engine.SetExpectingMusic(false)
	p.engine.gain.SetMusicGain(1, 0)

	go p.doLoad(gen, m)
}

func (p *Player) doLoad(gen int64, m *protocol.LoadPayload) {
	ctx := context.Background()

	// The daemon needs seconds after (re)start to authenticate; a load racing
	// that window must wait, not fail as track_unavailable (R0 prod finding).
	for i := 0; i < p.readyPollAttempts && !p.daemon.PlaybackReady(ctx); i++ {
		if p.staleLoad(gen) {
			return
		}
		time.Sleep(p.readyPollInterval)
	}

	err := p.daemon.PlayPaused(ctx, m.URI)
	if err != nil {
		// One local retry: transient daemon stalls (transfer storms) must not
		// surface as track_unavailable. 4x ready interval = 2 s at defaults.
		time.Sleep(4 * p.readyPollInterval)
		if p.staleLoad(gen) {
			return
		}
		err = p.daemon.PlayPaused(ctx, m.URI)
	}
	if err == nil && m.PositionMS > 0 {
		err = p.daemon.Seek(ctx, m.PositionMS)
	}
	if err == nil {
		err = p.confirmPausedLoaded(ctx, gen, m.URI)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if gen != p.loadGen {
		return // stale: a newer command took over
	}
	if err != nil {
		p.playback = PlaybackStopped
		p.log.Error("load failed", "element", m.ElementID, "err", err)
		p.send(protocol.TypeError, &protocol.ErrorPayload{
			Code: "load_failed", Message: err.Error(), ElementID: m.ElementID,
		})
		return
	}
	p.playback = PlaybackPaused
	p.send(protocol.TypeReady, &protocol.ReadyPayload{ElementID: m.ElementID})
}

// confirmPausedLoaded polls /status until the daemon confirms the
// paused-loaded state (spec 6.3 load step 1), tolerating an empty track.
func (p *Player) confirmPausedLoaded(ctx context.Context, gen int64, uri string) error {
	for i := 0; i < p.confirmPollAttempts; i++ {
		if p.staleLoad(gen) {
			return nil
		}
		st, err := p.daemon.Status(ctx)
		if err == nil &&
			(boolVal(st.Paused) || boolVal(st.Buffering)) &&
			(st.Track == nil || st.Track.URI == uri) {
			return nil
		}
		time.Sleep(p.confirmPollInterval)
	}
	return &daemonConfirmError{uri: uri}
}

type daemonConfirmError struct{ uri string }

func (e *daemonConfirmError) Error() string {
	return "daemon did not confirm paused load of " + e.uri
}

func (p *Player) staleLoad(gen int64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return gen != p.loadGen
}

func (p *Player) resumeAt(m *protocol.ResumeAtPayload) {
	p.mu.Lock()
	if m.ElementID != p.elementID {
		p.mu.Unlock()
		return // idempotency (spec 7.2)
	}
	gen := p.loadGen
	p.mu.Unlock()

	if m.PositionMS != nil {
		position := *m.PositionMS
		p.ring.Clear()
		go func() {
			if err := p.daemon.Seek(context.Background(), position); err != nil {
				p.log.Warn("catch-up seek failed; resuming best effort", "element", m.ElementID, "err", err)
			}
			p.mu.Lock()
			if gen != p.loadGen || m.ElementID != p.elementID {
				p.mu.Unlock()
				return
			}
			p.anchorPosMS = position
			p.anchorAt = time.Now()
			p.extrapolate = false
			p.mu.Unlock()
			p.armResume(m)
		}()
		return
	}
	p.armResume(m)
}

func (p *Player) armResume(m *protocol.ResumeAtPayload) {
	p.mu.Lock()
	if m.ElementID != p.elementID {
		p.mu.Unlock()
		return
	}
	latency := p.outputLatencyOffsetMS
	p.mu.Unlock()

	tLocal, ok := p.clock.LocalDeadline(m.TCoordMS, latency)
	if !ok {
		p.log.Warn("resume_at without clock sync, starting immediately", "element", m.ElementID)
		p.fireResume(m.ElementID)
		return
	}
	delay := time.Duration(tLocal-nowMS()) * time.Millisecond
	if delay < 0 {
		delay = 0
	}
	p.mu.Lock()
	p.cancelResumeLocked()
	el := m.ElementID
	// time.AfterFunc arms Go's monotonic runtime timer: the wall-to-deadline
	// difference is computed once, then NTP steps cannot move the shot.
	p.resumeTimer = time.AfterFunc(delay, func() { p.fireResume(el) })
	p.mu.Unlock()
	p.log.Info("resume armed", "element", m.ElementID, "in_ms", delay.Milliseconds())
}

func (p *Player) fireResume(elementID string) {
	p.mu.Lock()
	if elementID != p.elementID {
		p.mu.Unlock()
		return
	}
	p.playback = PlaybackPlaying
	p.startedPending = true
	p.anchorAt = time.Now()
	p.extrapolate = true
	p.mu.Unlock()
	p.engine.gain.SetMusicGain(1, 0)
	p.engine.SetExpectingMusic(true)
	go p.daemon.Resume(context.Background())
}

// NoteRendered is called by the audio render loop with the number of floats
// actually pulled from the music ring. The first non-empty pull after resume
// is the audible start -> report started(t_first_sample_coord_ms) (spec 8.4).
func (p *Player) NoteRendered(n int) {
	if n <= 0 {
		return
	}
	p.mu.Lock()
	if !p.startedPending {
		p.mu.Unlock()
		return
	}
	p.startedPending = false
	el := p.elementID
	p.mu.Unlock()

	tCoord := nowMS()
	if off, ok := p.clock.OffsetMS(); ok {
		tCoord -= int64(off + 0.5) // node = coord + offset (spec 8.5)
	}
	p.send(protocol.TypeStarted, &protocol.StartedPayload{
		ElementID: el, TFirstSampleCoordMS: tCoord,
	})
}

// NoteStarved is called by the render loop when it zero-filled a period.
// Only counts while music is expected (UNRESOLVED R4: idle silence is not
// an underrun).
func (p *Player) NoteStarved() {
	p.mu.Lock()
	playing := p.playback == PlaybackPlaying
	p.mu.Unlock()
	if playing {
		p.underruns.Add(1)
	}
}

func (p *Player) pauseCmd(m *protocol.PausePayload) {
	p.mu.Lock()
	if m.ElementID != p.elementID && m.ElementID != "" {
		p.mu.Unlock()
		return
	}
	p.cancelTimersLocked()
	p.pausedLocally = false
	p.playback = PlaybackPaused
	p.extrapolate = false
	p.anchorPosMS = p.audiblePositionLocked()
	p.anchorAt = time.Now()
	fade := m.FadeMS
	// The daemon pause lands only after the fade finished sounding
	// (fade_ms + 20, the macOS timing), so the ramp has samples to shape.
	p.pauseTimer = time.AfterFunc(time.Duration(fade+20)*time.Millisecond, func() {
		p.daemon.Pause(context.Background())
	})
	p.mu.Unlock()
	p.engine.gain.SetMusicGain(0, int(fade))
	p.engine.SetExpectingMusic(false)
}

func (p *Player) seekCmd(m *protocol.SeekPayload) {
	p.mu.Lock()
	if m.ElementID != p.elementID {
		p.mu.Unlock()
		return
	}
	p.anchorPosMS = m.PositionMS
	p.anchorAt = time.Now()
	p.extrapolate = p.playback == PlaybackPlaying
	p.mu.Unlock()
	p.ring.Clear()
	go p.daemon.Seek(context.Background(), m.PositionMS)
}

func (p *Player) playVoice(m *protocol.PlayVoicePayload) {
	p.mu.Lock()
	p.cancelTimersLocked()
	p.loadGen++ // voice supersedes any in-flight track load
	gen := p.loadGen
	p.elementID = m.ElementID
	p.playback = PlaybackVoice
	p.draining = false
	latency := p.outputLatencyOffsetMS
	p.mu.Unlock()
	p.engine.StopVoice()
	p.ring.Clear()
	p.engine.SetExpectingMusic(false)
	p.engine.gain.SetMusicGain(0, 0)
	p.pauseForInsert()

	el := m.ElementID
	go func() {
		if p.cache == nil {
			p.sendError("media_download_failed", "voice cache not configured", el)
			return
		}
		path, err := p.cache.Fetch(context.Background(), m.FileURL)
		if err != nil {
			p.sendError("media_download_failed", err.Error(), el)
			return
		}
		samples, err := loadVoiceFile(path)
		if err != nil {
			p.sendError("media_download_failed", err.Error(), el)
			return
		}

		var startAt time.Time // zero = next render pull
		if m.TCoordMS != nil {
			if tLocal, ok := p.clock.LocalDeadline(*m.TCoordMS, latency); ok {
				startAt = time.UnixMilli(tLocal)
			}
		}
		p.mu.Lock()
		if p.loadGen != gen || p.playback != PlaybackVoice || p.elementID != el {
			p.mu.Unlock()
			return
		}
		p.send(protocol.TypeVoiceStarted, &protocol.VoiceStartedPayload{ElementID: el})
		p.engine.PlayVoice(samples, startAt, func() {
			// The engine fired this after the LAST sample rendered — that is
			// the audible end, so voice_ended needs no extra drain wait.
			p.mu.Lock()
			fire := p.playback == PlaybackVoice && p.elementID == el
			if fire {
				p.playback = PlaybackStopped
			}
			p.mu.Unlock()
			if fire {
				p.send(protocol.TypeVoiceEnded, &protocol.VoiceEndedPayload{ElementID: el})
			}
		})
		p.mu.Unlock()
	}()
}

func (p *Player) waitCmd(m *protocol.WaitPayload) {
	p.mu.Lock()
	p.cancelTimersLocked()
	p.loadGen++
	p.elementID = m.ElementID
	p.playback = PlaybackWait
	el := m.ElementID
	p.waitTimer = time.AfterFunc(time.Duration(m.DurationMS)*time.Millisecond, func() {
		p.mu.Lock()
		fire := p.playback == PlaybackWait && p.elementID == el
		if fire {
			p.playback = PlaybackStopped
		}
		p.mu.Unlock()
		if fire {
			p.send(protocol.TypeWaitEnded, &protocol.WaitEndedPayload{ElementID: el})
		}
	})
	p.mu.Unlock()
	p.engine.StopVoice()
	p.ring.Clear()
	p.engine.SetExpectingMusic(false)
	p.engine.gain.SetMusicGain(0, 0)
	p.pauseForInsert()
}

func (p *Player) pauseForInsert() {
	// Coordinator commands arrive serially on the websocket reader. Complete
	// the local pause before returning so a later stop+load cannot be overtaken
	// by an old asynchronous pause and silence the newly loaded element.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.daemon.Pause(ctx); err != nil {
		p.log.Warn("pause for insert failed", "err", err)
	}
}

func (p *Player) offsetTest(m *protocol.OffsetTestPayload) {
	p.mu.Lock()
	latency := p.outputLatencyOffsetMS
	p.mu.Unlock()
	tLocal, ok := p.clock.LocalDeadline(m.TCoordMS, latency)
	if !ok {
		p.log.Warn("offset_test without clock sync, skipping")
		return
	}
	p.engine.PlayClicks(m.Clicks, time.UnixMilli(tLocal), m.IntervalMS)
}

func (p *Player) stopAll() {
	p.mu.Lock()
	wasInsert := p.playback == PlaybackVoice || p.playback == PlaybackWait
	p.cancelTimersLocked()
	p.pausedLocally = false
	p.loadGen++ // invalidate any in-flight load
	p.elementID = ""
	p.uri = ""
	p.playback = PlaybackStopped
	p.startedPending = false
	p.extrapolate = false
	p.draining = false
	p.anchorPosMS = 0
	p.mu.Unlock()
	p.engine.SetExpectingMusic(false)
	p.engine.StopVoice()
	if wasInsert {
		// Voice/wait already silenced the music branch. Drop the insert and
		// reset synchronously so a coordinator cancellation can load the next
		// element immediately without a delayed stop timer erasing that load.
		p.ring.Clear()
		p.engine.gain.SetMusicGain(1, 0)
		return
	}
	// Mode switches yank someone's music away (spec 4.3) — land it softly:
	// raised-cosine fade out, then drop the tail and re-arm the gain.
	p.engine.gain.SetMusicGain(0, 250)
	time.AfterFunc(300*time.Millisecond, func() {
		p.ring.Clear()
		p.engine.gain.SetMusicGain(1, 0) // ready for the next element
	})
	go p.daemon.Stop(context.Background())
}

func (p *Player) SetVolume(v int) {
	p.mu.Lock()
	p.volume = v
	p.mu.Unlock()
	p.engine.gain.SetVolume(v)
}

// SetExternalVolume applies a daemon volume event (external_volume: true
// hands volume to our mixer, spec A.2/6.3): the phone's Spotify Connect
// slider arrives as value/max — last writer (phone or coordinator) wins;
// the heartbeat reports the result.
func (p *Player) SetExternalVolume(value, max int) {
	if max <= 0 {
		return
	}
	p.SetVolume(int(float64(value)/float64(max)*100 + 0.5))
}

func (p *Player) setMode(m string) {
	p.mu.Lock()
	p.mode = m
	p.pausedLocally = false
	p.mu.Unlock()
	p.log.Info("mode set", "mode", m)
}

func (p *Player) setOffset(m *protocol.SetOffsetPayload) {
	p.mu.Lock()
	p.outputLatencyOffsetMS = int(m.OffsetMS)
	p.mu.Unlock()
	p.log.Info("offset set", "offset_ms", m.OffsetMS)
}

func (p *Player) cancelResumeLocked() {
	if p.resumeTimer != nil {
		p.resumeTimer.Stop()
		p.resumeTimer = nil
	}
}

func (p *Player) cancelWaitLocked() {
	if p.waitTimer != nil {
		p.waitTimer.Stop()
		p.waitTimer = nil
	}
}

// cancelTimersLocked mirrors the macOS cancelTimers(): load/pause/stop kill
// both the armed resume and a pending wait. A scheduled daemon pause is
// cancelled too — the newer command owns the daemon now.
func (p *Player) cancelTimersLocked() {
	p.cancelResumeLocked()
	p.cancelWaitLocked()
	if p.pauseTimer != nil {
		p.pauseTimer.Stop()
		p.pauseTimer = nil
	}
}

func (p *Player) sendError(code, message, elementID string) {
	p.log.Error("node error", "code", code, "msg", message)
	p.send(protocol.TypeError, &protocol.ErrorPayload{
		Code: code, Message: message, ElementID: elementID,
	})
}

// --- librespot /events wiring (port of PlayerCore.handleLibrespotEvent) ---

// HandleLibrespotEvent consumes one parsed /events frame.
func (p *Player) HandleLibrespotEvent(ev LibrespotEvent) {
	switch ev.Type {
	case "metadata":
		p.mu.Lock()
		if p.mode != "shared" && ev.URI != nil {
			p.uri = *ev.URI // solo: the daemon queue drives the uri
		}
		if ev.Position != nil {
			p.anchorPosMS = *ev.Position
			p.anchorAt = time.Now()
			p.extrapolate = p.playback == PlaybackPlaying
		}
		if ev.URI != nil {
			p.metadataURI = *ev.URI
		}
		p.metadataPosition = ev.Position
		p.metadataTitle = selectionDisplayTitle(ev.Name, ev.ArtistNames)
		p.mu.Unlock()

	case "seek":
		if ev.Position != nil {
			p.mu.Lock()
			p.anchorPosMS = *ev.Position
			p.anchorAt = time.Now()
			p.extrapolate = p.playback == PlaybackPlaying
			p.mu.Unlock()
		}

	case "not_playing", "stopped":
		// Track over at the daemon: the ring tail must finish sounding
		// before we report ended (spec 6.3 item 5) — the drain watcher fires.
		p.mu.Lock()
		if p.playback == PlaybackPlaying && p.elementID != "" {
			p.draining = true
			p.extrapolate = false
		}
		p.mu.Unlock()

	case "paused":
		// Personal pause (2026-07-10): reaching here with playback still
		// PlaybackPlaying means the DAEMON acted on the user, not on us —
		// every coordinator-driven pause flips playback before the daemon
		// echoes the event. Report it and cancel any resume_at in flight (a
		// scheduled fireResume overriding a fresh user pause was one of the
		// ghost-resume mechanics).
		p.mu.Lock()
		personal := p.mode == "shared" && p.playback == PlaybackPlaying &&
			!p.pausedLocally && p.elementID != ""
		el := p.elementID
		if personal {
			p.pausedLocally = true
			p.cancelResumeLocked()
		}
		fade := p.mode == "solo" || p.playback == PlaybackPlaying
		if fade {
			p.extrapolate = false
		}
		p.mu.Unlock()
		if personal {
			p.log.Info("personal pause", "element", el)
			p.send(protocol.TypeUserPause, &protocol.UserPausePayload{ElementID: el})
		}
		if fade {
			p.engine.gain.SetMusicGain(0, 250)
		}

	case "playing":
		// Personal resume: play in Spotify returns THIS home to the air — the
		// coordinator answers with a catch-up load at the live position. A
		// DIFFERENT track picked while paused falls through to adoption.
		p.mu.Lock()
		resumed := false
		if p.pausedLocally {
			p.pausedLocally = false
			if ev.URI != nil && *ev.URI == p.uri {
				resumed = true
			}
		}
		el := p.elementID
		p.mu.Unlock()
		if resumed {
			p.log.Info("personal resume", "element", el)
			p.send(protocol.TypeUserResume, &protocol.UserResumePayload{ElementID: el})
			p.engine.gain.SetMusicGain(1, 120)
			break
		}
		// resume_at marks PlaybackPlaying before asking the daemon to resume,
		// so its own playing event is ignored. A same-URI playing event while
		// stopped/paused is a fresh Spotify selection and must be adopted.
		p.mu.Lock()
		insertionActive := p.playback == PlaybackVoice || p.playback == PlaybackWait
		p.mu.Unlock()
		p.reportExternalSelection(ev.URI, nil, true, ev.PlayOrigin)
		if insertionActive {
			p.engine.gain.SetMusicGain(0, 0)
			go p.daemon.Pause(context.Background())
			break
		}
		p.engine.gain.SetMusicGain(1, 120)
		p.mu.Lock()
		if p.playback == PlaybackPlaying {
			p.anchorAt = time.Now()
			p.extrapolate = true
		}
		p.mu.Unlock()

	case "volume":
		if ev.Value != nil && ev.Max != nil {
			p.SetExternalVolume(*ev.Value, *ev.Max)
		}

	default:
		// active/inactive/will_play/unknown: no node-side action.
	}
}

// drainWatch sends ended(eof) once the daemon reported the track over AND
// the ring drained — the audible end, not the delivery end (spec 6.3 item 5).
func (p *Player) drainWatch() {
	ticker := time.NewTicker(p.drainInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
		}
		p.mu.Lock()
		fire := p.draining && p.elementID != "" && p.ring.FillMS(sampleRate, channels) == 0
		var el string
		if fire {
			el = p.elementID
			p.draining = false
			p.playback = PlaybackStopped
		}
		p.mu.Unlock()
		if fire {
			p.engine.SetExpectingMusic(false)
			p.send(protocol.TypeEnded, &protocol.EndedPayload{ElementID: el, Reason: "eof"})
		}
	}
}

// --- heartbeat snapshot (spec 8.4 state) ---

// AudiblePositionMS = daemon position anchor + wall extrapolation - ring fill
// (spec 6.3: what the listener actually hears, not what the daemon delivered).
func (p *Player) AudiblePositionMS() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.audiblePositionLocked()
}

func (p *Player) audiblePositionLocked() int64 {
	pos := p.anchorPosMS
	if p.extrapolate {
		pos += time.Since(p.anchorAt).Milliseconds()
	}
	pos -= p.ring.FillMS(sampleRate, channels)
	if pos < 0 {
		return 0
	}
	return pos
}

func boolVal(b *bool) bool { return b != nil && *b }
