package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/bot"
	"relux.works/duet/coordinator/internal/config"
	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
	"relux.works/duet/coordinator/internal/spotify"
	"relux.works/duet/coordinator/internal/store"
	"relux.works/duet/coordinator/internal/ulid"
)

// nodeSender abstracts the ws-hub for loop tests (integration tests drive
// every phase-1 bot command against the real FSM with a fake transport).
type nodeSender interface {
	Send(node protocol.NodeID, msgType string, payload any) bool
	Online() map[protocol.NodeID]bool
}

// loop serializes every session-affecting event: node messages, bot commands,
// media completions, ready timeouts (spec 7.2: the FSM is single-threaded).
type loop struct {
	log  *slog.Logger
	cfg  *config.Config
	hub  nodeSender
	sess *session.Session
	st   *store.Store
	bot  *bot.Bot        // nil when telegram is disabled (dev mode)
	sp   *spotify.Client // nil when spotify app creds are not configured (U10)

	// takeoverPolicy: "user" | "coordinator" (U9), persisted in settings.
	takeoverPolicy string

	volumes  map[protocol.NodeID]int
	offsets  map[protocol.NodeID]int64
	lastSeen map[protocol.NodeID]*protocol.StatePayload
	versions map[protocol.NodeID]string
	// restoredPaused: after coordinator restart, resume position must follow
	// live heartbeats until the user resumes (spec 7.2).
	restoredPaused bool
	lastDesyncMS   int64

	readyTimer   *time.Timer
	timerElement string
	timeouts     chan string
	mediaCh      chan mediaDone
	playlistCh   chan playlistDone
}

type playlistDone struct {
	uri    string
	title  string
	tracks []string
	err    error
	reply  func(string)
}

type mediaDone struct {
	mediaID  string
	from     protocol.NodeID
	personal bool
	result   media.Result
	err      error
	reply    func(string)
}

func newLoop(log *slog.Logger, cfg *config.Config, h nodeSender, st *store.Store, b *bot.Bot, sp *spotify.Client) *loop {
	s := session.New()
	s.StartMarginMS = int64(cfg.Timings.StartMarginMS)
	return &loop{
		log:            log,
		cfg:            cfg,
		hub:            h,
		sess:           s,
		st:             st,
		bot:            b,
		sp:             sp,
		takeoverPolicy: "user", // customer default 2026-07-03: the phone wins
		volumes:        map[protocol.NodeID]int{protocol.NodeA: 80, protocol.NodeB: 80},
		offsets:        map[protocol.NodeID]int64{},
		lastSeen:       map[protocol.NodeID]*protocol.StatePayload{},
		versions:       map[protocol.NodeID]string{},
		timeouts:       make(chan string, 4),
		mediaCh:        make(chan mediaDone, 8),
		playlistCh:     make(chan playlistDone, 4),
	}
}

// restore pulls the persisted session and settings (spec 7.2 restart rule).
func (l *loop) restore() {
	for _, n := range []protocol.NodeID{protocol.NodeA, protocol.NodeB} {
		if v, _ := l.st.GetSetting("volume_" + string(n)); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				l.volumes[n] = i
			}
		}
		if v, _ := l.st.GetSetting("offset_" + string(n)); v != "" {
			if i, err := strconv.ParseInt(v, 10, 64); err == nil {
				l.offsets[n] = i
			}
		}
	}
	if v, _ := l.st.GetSetting("takeover_policy"); v == "user" || v == "coordinator" {
		l.takeoverPolicy = v
	}
	snap, err := l.st.LoadSession()
	if err != nil {
		l.log.Error("session restore failed, starting fresh", "err", err)
		return
	}
	if snap == nil {
		return
	}
	l.sess.Mode = snap.Mode
	l.sess.State = snap.State
	l.sess.Current = snap.Current
	l.sess.SavedPositionMS = snap.SavedPositionMS
	l.sess.Queue = snap.Queue
	l.sess.Playlist = snap.Playlist
	if snap.State == session.StatePaused && snap.Current != nil {
		l.restoredPaused = true
		l.notify("координатор перезапустился: эфир на паузе, /resume чтобы продолжить")
	}
	l.log.Info("session restored", "mode", snap.Mode, "state", snap.State, "queue_len", len(snap.Queue))
}

func (l *loop) run(stop <-chan struct{}, nodeEvents <-chan hub.Event) {
	var botEvents chan bot.Event
	if l.bot != nil {
		botEvents = l.bot.Events
	}
	for {
		select {
		case <-stop:
			return
		case ev := <-nodeEvents:
			l.handleNode(ev)
		case elementID := <-l.timeouts:
			l.apply(l.sess.OnReadyTimeout(elementID))
		case ev := <-botEvents:
			l.handleBot(ev)
		case done := <-l.mediaCh:
			l.handleMediaDone(done)
		case d := <-l.playlistCh:
			l.handlePlaylistDone(d)
		}
	}
}

func (l *loop) handlePlaylistDone(d playlistDone) {
	if d.err != nil {
		l.log.Warn("playlist expansion failed", "uri", d.uri, "err", d.err)
		d.reply(fmt.Sprintf("не смог раскрыть плейлист: %v", d.err))
		return
	}
	l.apply(l.sess.SetPlaylist(d.uri, d.title, d.tracks))
}

func (l *loop) notify(text string) {
	if l.bot != nil {
		l.bot.Notify(text)
	}
	l.log.Info("notify", "text", text)
}

func (l *loop) persist() {
	err := l.st.SaveSession(store.SessionSnapshot{
		Mode:            l.sess.Mode,
		State:           l.sess.State,
		Current:         l.sess.Current,
		SavedPositionMS: l.sess.SavedPositionMS,
		Queue:           l.sess.Queue,
		Playlist:        l.sess.Playlist,
	})
	if err != nil {
		l.log.Error("persist failed", "err", err)
	}
}

// --- Node events ---

func (l *loop) handleNode(ev hub.Event) {
	switch e := ev.(type) {
	case hub.EvRegistered:
		l.log.Info("node registered", "node", e.Node, "app", e.AppVersion, "librespot", e.LibrespotVersion)
		l.versions[e.Node] = e.AppVersion + "/librespot " + e.LibrespotVersion
		snap := l.sess.Snapshot(l.volumes[e.Node])
		l.hub.Send(e.Node, protocol.TypeWelcome, &protocol.WelcomePayload{SessionSnapshot: snap})
		if off, ok := l.offsets[e.Node]; ok {
			l.hub.Send(e.Node, protocol.TypeSetOffset, &protocol.SetOffsetPayload{OffsetMS: off})
		}
	case hub.EvOnline:
		l.apply(l.sess.OnNodeBack(e.Node))
	case hub.EvOffline:
		l.log.Warn("node offline", "node", e.Node)
		l.st.LogEvent(string(e.Node), "offline", nil)
		l.apply(l.sess.OnNodeOffline(e.Node))
	case hub.EvMessage:
		l.handleNodeMessage(e)
	}
}

func (l *loop) handleNodeMessage(m hub.EvMessage) {
	now := time.Now().UnixMilli()
	switch p := m.Payload.(type) {
	case *protocol.StatePayload:
		l.lastSeen[m.Node] = p
		l.volumes[m.Node] = p.Volume
		l.apply(l.sess.OnHeartbeat(m.Node, p.PositionMS, p.RTTMS))
		if l.restoredPaused {
			l.sess.RefreshSavedPosition()
		}
	case *protocol.ReadyPayload:
		l.apply(l.sess.OnReady(now, m.Node, p.ElementID))
	case *protocol.StartedPayload:
		l.apply(l.sess.OnStarted(m.Node, p.ElementID, p.TFirstSampleCoordMS))
	case *protocol.EndedPayload:
		l.apply(l.sess.OnEnded(m.Node, p.ElementID, p.Reason))
	case *protocol.VoiceStartedPayload:
		l.log.Info("voice started", "node", m.Node, "element", p.ElementID)
	case *protocol.VoiceEndedPayload:
		l.apply(l.sess.OnVoiceEnded(m.Node, p.ElementID))
	case *protocol.WaitEndedPayload:
		l.apply(l.sess.OnWaitEnded(m.Node, p.ElementID))
	case *protocol.ErrorPayload:
		l.log.Warn("node error", "node", m.Node, "code", p.Code, "msg", p.Message, "element", p.ElementID)
		l.st.LogEvent(string(m.Node), "node_error", p)
		l.apply(l.sess.OnNodeError(m.Node, p.Code, p.ElementID))
	case *protocol.ExternalPlaybackPayload:
		l.handleExternalPlayback(m.Node, p.URI)
	default:
		l.log.Debug("unhandled message", "node", m.Node, "type", m.Env.Type)
	}
}

// handleExternalPlayback applies the takeover policy (U9): the partner's
// phone started its own playback on a node while the session is shared.
func (l *loop) handleExternalPlayback(node protocol.NodeID, uri string) {
	if l.sess.Mode != session.ModeShared {
		return
	}
	l.st.LogEvent(string(node), "external_playback", map[string]string{"uri": uri, "policy": l.takeoverPolicy})
	if l.takeoverPolicy == "user" {
		l.notify(fmt.Sprintf("дом %s забрал управление (играет с телефона) — режим solo", node))
		l.apply(l.sess.SetModeSolo())
		return
	}
	l.notify(fmt.Sprintf("дом %s вмешался с телефона — эфир восстановлен", node))
	if l.sess.State == session.StatePlaying {
		l.apply(l.sess.CmdSync()) // restart current element in both homes at the live position
		return
	}
	// Broadcast is not playing a track right now: just silence the intruder.
	l.hub.Send(node, protocol.TypeStop, &protocol.StopPayload{})
}

// --- Bot events (spec ch. 9) ---

func (l *loop) handleBot(ev bot.Event) {
	if ev.Voice != nil {
		l.handleVoice(ev)
		return
	}
	from := protocol.NodeID(ev.From)
	cmd := ev.Command
	switch cmd.Kind {

	case bot.KindLink:
		if l.sess.Mode != session.ModeShared {
			ev.Reply("сейчас режим solo: /inject подкинет трек партнёру, /mode shared вернёт общий эфир")
			return
		}
		el := l.newTrackElement(cmd.URI, from)
		l.st.InsertElement(el)
		l.apply(l.sess.EnqueueTrack(el))
		if l.sess.Current != nil && l.sess.Current.ID == el.ID {
			ev.Reply("очередь пуста — ставлю сразу: " + trackLabel(el))
		} else {
			ev.Reply(fmt.Sprintf("добавил в очередь под номером %d: %s", l.sess.QueueLen(), trackLabel(el)))
		}

	case bot.KindPlayNow:
		if l.sess.Mode != session.ModeShared {
			ev.Reply("/playnow работает в shared. Сейчас solo")
			return
		}
		el := l.newTrackElement(cmd.URI, from)
		l.st.InsertElement(el)
		l.apply(l.sess.CmdPlayNow(el))
		ev.Reply("врубаю немедленно: " + trackLabel(el))

	case bot.KindPlaylist:
		if l.sess.Mode != session.ModeShared {
			ev.Reply("общий плейлист — фича shared-режима. Сейчас solo")
			return
		}
		if l.sp == nil {
			ev.Reply("плейлисты заработают после настройки Spotify-приложения: client_id/client_secret в coordinator.yml (developer.spotify.com)")
			return
		}
		ev.Reply("раскрываю плейлист…")
		uri := cmd.URI
		kind := cmd.Target // "playlist" | "album"
		id := uri[strings.LastIndex(uri, ":")+1:]
		reply := ev.Reply
		go func() {
			var exp *spotify.Expansion
			var err error
			if kind == "album" {
				exp, err = l.sp.ExpandAlbum(id)
			} else {
				exp, err = l.sp.ExpandPlaylist(id)
			}
			d := playlistDone{uri: uri, err: err, reply: reply}
			if exp != nil {
				d.title = exp.Title
				d.tracks = exp.Tracks
			}
			l.playlistCh <- d
		}()

	case bot.KindTakeover:
		l.takeoverPolicy = cmd.Target
		l.st.SetSetting("takeover_policy", cmd.Target)
		if cmd.Target == "user" {
			ev.Reply("политика: телефон главнее — вмешательство переключает систему в solo (с уведомлением)")
		} else {
			ev.Reply("политика: эфир главнее — вмешательство с телефона откатывается (с уведомлением)")
		}

	case bot.KindQueue:
		ev.Reply(l.queueText())

	case bot.KindCancel:
		if _, err := l.sess.Cancel(cmd.Number); err != nil {
			ev.Reply(fmt.Sprintf("в очереди %d элементов, номера %d нет", l.sess.QueueLen(), cmd.Number))
			return
		}
		l.persist()
		ev.Reply(fmt.Sprintf("убрал элемент %d, в очереди осталось %d", cmd.Number, l.sess.QueueLen()))

	case bot.KindSkip:
		if effs := l.sess.CmdSkip(); effs != nil {
			l.apply(effs)
			ev.Reply("пропустил")
		} else {
			ev.Reply("нечего пропускать")
		}

	case bot.KindPause:
		if effs := l.sess.CmdPause(); effs != nil {
			l.apply(effs)
			ev.Reply("пауза")
		} else {
			ev.Reply("и так не играет")
		}

	case bot.KindResume:
		if effs := l.sess.CmdResume(); effs != nil {
			l.restoredPaused = false
			l.apply(effs)
			ev.Reply("продолжаю")
		} else {
			ev.Reply("нечего продолжать — пришли ссылку на трек")
		}

	case bot.KindSync:
		if effs := l.sess.CmdSync(); effs != nil {
			l.apply(effs)
			ev.Reply("пересинхронизирую с текущей позиции")
		} else {
			ev.Reply("/sync работает во время игры трека")
		}

	case bot.KindVol:
		target := from
		if cmd.Target != "" {
			target = protocol.NodeID(cmd.Target)
		}
		l.volumes[target] = cmd.Number
		l.st.SetSetting("volume_"+string(target), strconv.Itoa(cmd.Number))
		if !l.hub.Send(target, protocol.TypeSetVolume, &protocol.SetVolumePayload{Volume: cmd.Number}) {
			ev.Reply(fmt.Sprintf("дом %s офлайн, громкость применю при подключении", target))
			return
		}
		ev.Reply(fmt.Sprintf("громкость дома %s: %d", target, cmd.Number))

	case bot.KindMode:
		var effs []session.Effect
		if cmd.Target == "solo" {
			effs = l.sess.SetModeSolo()
		} else {
			effs = l.sess.SetModeShared()
		}
		if effs == nil {
			ev.Reply("уже в этом режиме")
			return
		}
		l.apply(effs)

	case bot.KindNow:
		ev.Reply(l.nowText())

	case bot.KindStatus:
		ev.Reply(l.statusText())

	case bot.KindOffset:
		target := protocol.NodeID(cmd.Target)
		l.offsets[target] = int64(cmd.Number)
		l.st.SetSetting("offset_"+string(target), strconv.Itoa(cmd.Number))
		l.hub.Send(target, protocol.TypeSetOffset, &protocol.SetOffsetPayload{OffsetMS: int64(cmd.Number)})
		ev.Reply(fmt.Sprintf("offset дома %s = %d мс, действует со следующего старта", target, cmd.Number))

	case bot.KindOffsetTest:
		t := time.Now().UnixMilli() + 2000
		payload := &protocol.OffsetTestPayload{TCoordMS: t, Clicks: 5, IntervalMS: 1000}
		okA := l.hub.Send(protocol.NodeA, protocol.TypeOffsetTest, payload)
		okB := l.hub.Send(protocol.NodeB, protocol.TypeOffsetTest, payload)
		if !okA || !okB {
			ev.Reply("обе ноды должны быть онлайн для клик-теста (/status покажет)")
			return
		}
		ev.Reply("5 синхронных кликов через 2 секунды — слушайте")

	case bot.KindInject:
		if l.sess.Mode != session.ModeSolo {
			ev.Reply("в shared просто кинь ссылку в чат; /inject — для solo")
			return
		}
		targets := []protocol.NodeID{otherHome(from)}
		switch cmd.Target {
		case "a":
			targets = []protocol.NodeID{protocol.NodeA}
		case "b":
			targets = []protocol.NodeID{protocol.NodeB}
		case "both":
			targets = []protocol.NodeID{protocol.NodeA, protocol.NodeB}
		}
		sent := 0
		for _, t := range targets {
			if l.hub.Send(t, protocol.TypeSoloInject, &protocol.SoloInjectPayload{URI: cmd.URI}) {
				sent++
			}
		}
		if sent == 0 {
			ev.Reply("целевая нода офлайн")
			return
		}
		ev.Reply("подкинул в очередь")
	}
}

// --- Voice flow (spec ch. 10) ---

func (l *loop) handleVoice(ev bot.Event) {
	v := ev.Voice
	from := protocol.NodeID(ev.From)
	if v.Duration > l.cfg.Media.MaxVoiceS {
		ev.Reply(fmt.Sprintf("голосовое длиннее %d минут, не возьму", l.cfg.Media.MaxVoiceS/60))
		return
	}
	if v.SizeBytes > 20*1024*1024 {
		ev.Reply("файл больше 20 МБ, не возьму")
		return
	}
	mediaID := ulid.NewMediaID(time.Now())
	rec := store.MediaRecord{
		ID:        mediaID,
		TGFileID:  v.TGFileID,
		CreatedAt: time.Now().UnixMilli(),
		ExpiresAt: time.Now().AddDate(0, 0, l.cfg.Media.RetentionDays).UnixMilli(),
		Status:    "processing",
	}
	if err := l.st.InsertMedia(rec); err != nil {
		l.log.Error("media insert failed", "err", err)
		ev.Reply("внутренняя ошибка, голосовое не принято")
		return
	}
	personal := v.Personal
	reply := ev.Reply
	go func() {
		oga := filepath.Join(l.cfg.MediaDir, mediaID+".oga")
		wav := filepath.Join(l.cfg.MediaDir, mediaID+".wav")
		var res media.Result
		err := l.bot.DownloadVoice(v.TGFileID, oga)
		if err == nil {
			res, err = media.Process(oga, wav, media.Preset(l.cfg.Media.Preset))
		}
		if err == nil {
			os.Remove(oga) // spec 5.3: source deleted after successful processing
		}
		l.mediaCh <- mediaDone{mediaID: mediaID, from: from, personal: personal, result: res, err: err, reply: reply}
	}()
}

func (l *loop) handleMediaDone(d mediaDone) {
	if d.err != nil {
		l.log.Error("voice processing failed", "media", d.mediaID, "err", d.err)
		l.st.UpdateMedia(store.MediaRecord{ID: d.mediaID, Status: "failed"})
		d.reply("не смог обработать голосовое, оставил исходник для разбора")
		return
	}
	l.st.UpdateMedia(store.MediaRecord{
		ID: d.mediaID, DurationMS: d.result.DurationMS,
		PathWAV: d.result.WAVPath, LoudnormJSON: d.result.LoudnormJSON, Status: "ready",
	})
	target := "both"
	if d.personal {
		target = string(otherHome(d.from))
	}
	el := session.Element{
		ID:          ulid.NewElementID(time.Now()),
		Kind:        session.KindVoice,
		MediaID:     d.mediaID,
		DurationMS:  d.result.DurationMS,
		RequestedBy: d.from,
		Target:      target,
		CreatedAt:   time.Now().UnixMilli(),
	}
	l.st.InsertElement(el)

	if l.sess.Mode == session.ModeShared {
		l.apply(l.sess.EnqueueVoice(el))
		if d.personal {
			d.reply("личная вставка встанет после текущего трека")
		} else {
			d.reply("вставка встанет после текущего трека")
		}
		return
	}
	// Solo: deliver to the target node(s) for boundary interception (spec 4.2).
	payload := &protocol.SoloVoicePayload{ElementID: el.ID, FileURL: l.mediaURL(d.mediaID)}
	targets := []protocol.NodeID{protocol.NodeA, protocol.NodeB}
	if target != "both" {
		targets = []protocol.NodeID{protocol.NodeID(target)}
	}
	sent := 0
	for _, t := range targets {
		if l.hub.Send(t, protocol.TypeSoloVoice, payload) {
			sent++
		}
	}
	if sent == 0 {
		d.reply("нода-адресат офлайн, вставка не доставлена")
		return
	}
	d.reply("вставка уйдёт на ближайшей границе трека")
}

func (l *loop) mediaURL(mediaID string) string {
	if l.cfg.PublicURL != "" {
		return fmt.Sprintf("%s/media/%s.wav", strings.TrimRight(l.cfg.PublicURL, "/"), mediaID)
	}
	return fmt.Sprintf("http://%s/media/%s.wav", l.cfg.Listen, mediaID)
}

// --- Effects ---

func (l *loop) apply(effs []session.Effect) {
	for _, eff := range effs {
		switch e := eff.(type) {
		case session.EffLoad:
			l.hub.Send(e.To, protocol.TypeLoad, &protocol.LoadPayload{ElementID: e.ElementID, URI: e.URI, PositionMS: e.PositionMS})
		case session.EffResumeAt:
			l.hub.Send(e.To, protocol.TypeResumeAt, &protocol.ResumeAtPayload{ElementID: e.ElementID, TCoordMS: e.TCoordMS})
		case session.EffPause:
			l.hub.Send(e.To, protocol.TypePause, &protocol.PausePayload{ElementID: e.ElementID, FadeMS: e.FadeMS})
		case session.EffPlayVoice:
			l.hub.Send(e.To, protocol.TypePlayVoice, &protocol.PlayVoicePayload{
				ElementID: e.ElementID,
				FileURL:   l.mediaURL(e.MediaID),
			})
		case session.EffWait:
			l.hub.Send(e.To, protocol.TypeWait, &protocol.WaitPayload{ElementID: e.ElementID, DurationMS: e.DurationMS})
		case session.EffStop:
			l.hub.Send(e.To, protocol.TypeStop, &protocol.StopPayload{})
		case session.EffSetMode:
			l.hub.Send(e.To, protocol.TypeSetMode, &protocol.SetModePayload{Mode: string(e.Mode)})
		case session.EffNotify:
			l.notify(e.Text)
		case session.EffArmReadyTimer:
			l.armReadyTimer(e.ElementID)
		case session.EffCancelReadyTimer:
			l.cancelReadyTimer()
		case session.EffLogDesync:
			l.lastDesyncMS = e.DeltaMS
			l.log.Info("start desync measured", "delta_ms", e.DeltaMS)
			l.st.LogEvent("session", "desync", map[string]int64{"delta_ms": e.DeltaMS})
		case session.EffElementDone:
			l.st.MarkElementDone(e.Element.ID, e.Status, time.Now().UnixMilli())
		case session.EffPersist:
			l.persist()
		}
	}
}

func (l *loop) armReadyTimer(elementID string) {
	l.cancelReadyTimer()
	l.timerElement = elementID
	d := time.Duration(l.cfg.Timings.ReadyTimeoutS) * time.Second
	l.readyTimer = time.AfterFunc(d, func() { l.timeouts <- elementID })
}

func (l *loop) cancelReadyTimer() {
	if l.readyTimer != nil {
		l.readyTimer.Stop()
		l.readyTimer = nil
		l.timerElement = ""
	}
}

// --- Status texts (spec 9.1 /now /queue /status) ---

func (l *loop) newTrackElement(uri string, from protocol.NodeID) session.Element {
	return session.Element{
		ID:          ulid.NewElementID(time.Now()),
		Kind:        session.KindTrack,
		URI:         uri,
		RequestedBy: from,
		Target:      "both",
		CreatedAt:   time.Now().UnixMilli(),
	}
}

func trackLabel(el session.Element) string {
	if el.Title != "" {
		return el.Title
	}
	return el.URI // spec 9.1: before load, showing the id is acceptable
}

func otherHome(n protocol.NodeID) protocol.NodeID {
	if n == protocol.NodeA {
		return protocol.NodeB
	}
	return protocol.NodeA
}

func fmtMS(ms int64) string {
	s := ms / 1000
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

func (l *loop) queueText() string {
	var b strings.Builder
	if cur := l.sess.Current; cur != nil {
		fmt.Fprintf(&b, "сейчас: %s\n", elementLabel(*cur))
	}
	if l.sess.QueueLen() == 0 {
		b.WriteString("очередь вставок пуста")
	} else {
		b.WriteString("очередь:\n")
		for i, el := range l.sess.Queue {
			fmt.Fprintf(&b, "%d. %s (от %s)\n", i+1, elementLabel(el), el.RequestedBy)
		}
	}
	if p := l.sess.Playlist; p != nil {
		if p.Cursor < len(p.Tracks) {
			fmt.Fprintf(&b, "\nплейлист: «%s», дальше трек %d/%d", p.Title, p.Cursor+1, len(p.Tracks))
		} else {
			fmt.Fprintf(&b, "\nплейлист «%s» доигран", p.Title)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func elementLabel(el session.Element) string {
	if el.Kind == session.KindVoice {
		who := "обоим"
		if el.Target != "both" {
			who = "лично " + el.Target
		}
		return fmt.Sprintf("голосовое %s (%s)", fmtMS(el.DurationMS), who)
	}
	return trackLabel(el)
}

func (l *loop) nowText() string {
	if l.sess.Mode == session.ModeSolo {
		var b strings.Builder
		b.WriteString("режим solo\n")
		for _, n := range []protocol.NodeID{protocol.NodeA, protocol.NodeB} {
			st := l.lastSeen[n]
			if st == nil || st.URI == nil {
				fmt.Fprintf(&b, "дом %s: тишина\n", n)
				continue
			}
			fmt.Fprintf(&b, "дом %s: %s @ %s\n", n, *st.URI, fmtMS(st.PositionMS))
		}
		return strings.TrimRight(b.String(), "\n")
	}
	if cur := l.sess.Current; cur != nil {
		return fmt.Sprintf("сейчас: %s @ %s (%s)", elementLabel(*cur), fmtMS(l.livePosition()), l.sess.State)
	}
	return "тишина — пришли ссылку на трек"
}

func (l *loop) livePosition() int64 {
	var best int64
	for _, st := range l.lastSeen {
		if st != nil && st.PositionMS > best {
			best = st.PositionMS
		}
	}
	return best
}

func (l *loop) statusText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "режим %s, состояние %s\n", l.sess.Mode, l.sess.State)
	online := l.hub.Online()
	for _, n := range []protocol.NodeID{protocol.NodeA, protocol.NodeB} {
		if !online[n] {
			fmt.Fprintf(&b, "дом %s: офлайн\n", n)
			continue
		}
		st := l.lastSeen[n]
		if st == nil {
			fmt.Fprintf(&b, "дом %s: онлайн, ждём heartbeat\n", n)
			continue
		}
		mark := ""
		if st.Degraded {
			mark = " [degraded]"
		}
		var speakers []string
		for _, sp := range st.Speakers {
			c := "✗"
			if sp.Connected {
				c = "✓"
			}
			speakers = append(speakers, sp.Name+c)
		}
		fmt.Fprintf(&b, "дом %s: онлайн%s, поз %s, громкость %d, rtt %d мс, offset %d мс, колонки: %s\n",
			n, mark, fmtMS(st.PositionMS), st.Volume, st.RTTMS, l.offsets[n], strings.Join(speakers, " "))
	}
	if l.lastDesyncMS > 0 {
		fmt.Fprintf(&b, "рассинхрон последнего старта: %d мс\n", l.lastDesyncMS)
	}
	fmt.Fprintf(&b, "координатор %s", version)
	for n, v := range l.versions {
		fmt.Fprintf(&b, ", нода %s %s", n, v)
	}
	return b.String()
}
