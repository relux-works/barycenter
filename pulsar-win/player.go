// Player: executes coordinator commands on this node — a minimal port of the
// macOS PlayerCore (spec 6.2 item 5, mechanics 6.3):
//   - load = two-step (play paused via daemon HTTP + seek), then ready
//   - resume_at = fire daemon resume at T_local = T_coord + offset - latency
//     (delay computed once, armed on Go's monotonic timer)
//   - audible_position = daemon position anchor - ring fill (drain counter)
//   - pause/stop/set_volume/seek/set_mode/set_offset/solo_inject
//
// Not ported yet (needs the Windows audio engine beyond the skeleton):
// voice inserts (play_voice/solo_voice), wait, offset_test clicks, fade
// gains, ended-after-drain (needs the daemon /events stream), takeover
// detection (U9). Each logs a warning when commanded.
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
	clock  deadlineClock
	send   func(msgType string, payload any)
	log    *slog.Logger

	// Poll knobs (shrunk by tests). Defaults mirror the macOS PlayerCore:
	// ready 20x500ms, confirm 10x300ms, play retry after 2 s.
	readyPollInterval   time.Duration
	readyPollAttempts   int
	confirmPollInterval time.Duration
	confirmPollAttempts int

	underruns atomic.Int64

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
	startedPending        bool
}

func NewPlayer(daemon daemonAPI, ring *Ring, clock deadlineClock,
	send func(msgType string, payload any), outputLatencyOffsetMS int, log *slog.Logger) *Player {
	return &Player{
		daemon:                daemon,
		ring:                  ring,
		clock:                 clock,
		send:                  send,
		log:                   log,
		readyPollInterval:     500 * time.Millisecond,
		readyPollAttempts:     20,
		confirmPollInterval:   300 * time.Millisecond,
		confirmPollAttempts:   10,
		mode:                  "shared",
		playback:              PlaybackStopped,
		volume:                80,
		outputLatencyOffsetMS: outputLatencyOffsetMS,
	}
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
	case *protocol.SetVolumePayload:
		p.SetVolume(m.Volume)
	case *protocol.SetModePayload:
		p.setMode(m.Mode)
	case *protocol.StopPayload:
		p.stopAll()
	case *protocol.SetOffsetPayload:
		p.setOffset(m)
	case *protocol.SoloInjectPayload:
		go p.daemon.AddToQueue(context.Background(), m.URI)
	case *protocol.PlayVoicePayload, *protocol.SoloVoicePayload,
		*protocol.WaitPayload, *protocol.OffsetTestPayload:
		p.log.Warn("command not implemented in the Windows skeleton yet", "type", env.Type)
	default:
		p.log.Debug("ignoring non-command", "type", env.Type)
	}
}

// ApplyWelcome reconciles the session snapshot after (re)connect (spec 8.6).
// Skeleton scope: adopt the broadcast volume; the coordinator re-issues
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
	p.cancelResumeLocked()
	p.loadGen++
	gen := p.loadGen
	p.playback = PlaybackLoading
	p.elementID = m.ElementID
	p.uri = m.URI
	p.startedPending = false
	p.anchorPosMS = m.PositionMS
	p.anchorAt = time.Now()
	p.extrapolate = false
	p.mu.Unlock()
	// The producer is the daemon feeding the pipe; a fresh load means the old
	// element's tail must not sound (spec 6.3).
	p.ring.Clear()

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
	go p.daemon.Resume(context.Background())
}

// NoteRendered is called by the audio render loop with the number of floats
// actually pulled from the ring. The first non-empty pull after resume is
// the audible start -> report started(t_first_sample_coord_ms) (spec 8.4).
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
	p.cancelResumeLocked()
	p.playback = PlaybackPaused
	p.extrapolate = false
	p.anchorPosMS = p.audiblePositionLocked()
	p.anchorAt = time.Now()
	p.mu.Unlock()
	// TODO(audio): raised-cosine fade over m.FadeMS before the daemon pause
	// (the macOS engine fades the mixer; the skeleton has no gain stage yet).
	go p.daemon.Pause(context.Background())
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

func (p *Player) stopAll() {
	p.mu.Lock()
	p.cancelResumeLocked()
	p.loadGen++ // invalidate any in-flight load
	p.elementID = ""
	p.uri = ""
	p.playback = PlaybackStopped
	p.startedPending = false
	p.extrapolate = false
	p.anchorPosMS = 0
	p.mu.Unlock()
	p.ring.Clear()
	go p.daemon.Stop(context.Background())
}

func (p *Player) SetVolume(v int) {
	p.mu.Lock()
	p.volume = v
	p.mu.Unlock()
	// TODO(audio): apply as a software gain in the render loop (or the WASAPI
	// session volume) once the engine grows a gain stage.
}

func (p *Player) setMode(m string) {
	p.mu.Lock()
	p.mode = m
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

func (p *Player) StatePayload(rttMS int64) protocol.StatePayload {
	p.mu.Lock()
	playback := string(p.playback)
	uri := p.uri
	volume := p.volume
	p.mu.Unlock()

	var uriPtr *string
	if uri != "" {
		uriPtr = &uri
	}
	return protocol.StatePayload{
		Playback:   playback,
		URI:        uriPtr,
		PositionMS: p.AudiblePositionMS(),
		Volume:     volume,
		Degraded:   false,
		Underruns:  p.underruns.Load(),
		RTTMS:      rttMS,
		Speakers:   []protocol.Speaker{}, // no Airfoil on Windows; WASAPI default device
	}
}

func boolVal(b *bool) bool { return b != nil && *b }
