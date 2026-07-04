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

// nodeSender abstracts the ws-hub for loop tests.
type nodeSender interface {
	Send(key hub.NodeKey, msgType string, payload any) bool
	Online(orbitID int64) map[protocol.NodeID]bool
}

// orbitState is everything the loop tracks per orbit (v2.1 multi-tenant):
// one FSM session plus its knobs, timers and last-seen node telemetry.
type orbitState struct {
	id    int64
	title string
	sess  *session.Session

	takeoverPolicy string // user | coordinator (per-orbit, orbits table)
	voiceDefault   string // personal | broadcast

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
}

// loop serializes every session-affecting event across all orbits
// (spec 7.2: the FSM is single-threaded; one goroutine, many orbits).
type loop struct {
	log *slog.Logger
	cfg *config.Config
	hub nodeSender
	st  *store.Store
	bot *bot.Bot        // nil when telegram is disabled (dev mode)
	sp  *spotify.Client // nil when spotify app creds are not configured (U10)

	states map[int64]*orbitState

	timeouts   chan orbitTimeout
	mediaCh    chan mediaDone
	playlistCh chan playlistDone
}

type orbitTimeout struct {
	orbit     int64
	elementID string
}

type playlistDone struct {
	orbit  int64
	uri    string
	title  string
	tracks []string
	err    error
	reply  func(string)
}

type mediaDone struct {
	orbit    int64
	mediaID  string
	from     int64  // tg user id of the sender
	fromName string
	personal bool
	result   media.Result
	err      error
	reply    func(string)
}

func newLoop(log *slog.Logger, cfg *config.Config, h nodeSender, st *store.Store, b *bot.Bot, sp *spotify.Client) *loop {
	return &loop{
		log:        log,
		cfg:        cfg,
		hub:        h,
		st:         st,
		bot:        b,
		sp:         sp,
		states:     map[int64]*orbitState{},
		timeouts:   make(chan orbitTimeout, 8),
		mediaCh:    make(chan mediaDone, 8),
		playlistCh: make(chan playlistDone, 4),
	}
}

// orbit returns the live state for an orbit, restoring it from the store on
// first touch (session snapshot, knobs, per-orbit settings).
func (l *loop) orbit(id int64) *orbitState {
	if o, ok := l.states[id]; ok {
		return o
	}
	o := &orbitState{
		id:             id,
		title:          "Барицентр",
		sess:           session.New(),
		takeoverPolicy: "user",
		voiceDefault:   "personal",
		volumes:        map[protocol.NodeID]int{},
		offsets:        map[protocol.NodeID]int64{},
		lastSeen:       map[protocol.NodeID]*protocol.StatePayload{},
		versions:       map[protocol.NodeID]string{},
	}
	o.sess.StartMarginMS = int64(l.cfg.Timings.StartMarginMS)
	if rec, err := l.st.GetOrbit(id); err == nil && rec != nil {
		o.title = rec.Title
		o.takeoverPolicy = rec.TakeoverPolicy
		o.voiceDefault = rec.VoiceDefault
	}
	slots, _ := l.st.ActiveSlots(id)
	// M2: the broadcast machinery runs over the orbit's real slot set.
	o.sess.SetPeers(slots)
	for _, sl := range slots {
		n := protocol.NodeID(sl)
		o.volumes[n] = 80
		if v, _ := l.st.GetSetting(fmt.Sprintf("volume_%d_%s", id, sl)); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				o.volumes[n] = i
			}
		}
		if v, _ := l.st.GetSetting(fmt.Sprintf("offset_%d_%s", id, sl)); v != "" {
			if i, err := strconv.ParseInt(v, 10, 64); err == nil {
				o.offsets[n] = i
			}
		}
	}
	if snap, err := l.st.LoadSession(id); err == nil && snap != nil {
		o.sess.Mode = snap.Mode
		o.sess.State = snap.State
		o.sess.Current = snap.Current
		o.sess.SavedPositionMS = snap.SavedPositionMS
		o.sess.Queue = snap.Queue
		o.sess.Playlist = snap.Playlist
		if snap.State == session.StatePaused && snap.Current != nil {
			o.restoredPaused = true
			l.notify(o, "координатор перезапустился: эфир на паузе, /resume чтобы продолжить")
		}
		l.log.Info("session restored", "orbit", id, "state", snap.State, "queue_len", len(snap.Queue))
	}
	l.states[id] = o
	return o
}

// warmup restores every known orbit at startup.
func (l *loop) warmup() {
	ids, err := l.st.OrbitIDs()
	if err != nil {
		l.log.Error("orbit warmup failed", "err", err)
		return
	}
	for _, id := range ids {
		l.orbit(id)
	}
	l.log.Info("orbits warmed up", "count", len(ids))
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
		case to := <-l.timeouts:
			o := l.orbit(to.orbit)
			l.apply(o, o.sess.OnReadyTimeout(to.elementID))
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
	o := l.orbit(d.orbit)
	l.apply(o, o.sess.SetPlaylist(d.uri, d.title, d.tracks))
}

// notify DMs every member of the orbit (group chat binding comes in M4).
func (l *loop) notify(o *orbitState, text string) {
	l.log.Info("notify", "orbit", o.id, "text", text)
	if l.bot == nil {
		return
	}
	members, err := l.st.Members(o.id)
	if err != nil {
		l.log.Error("members lookup failed", "orbit", o.id, "err", err)
		return
	}
	for _, m := range members {
		l.bot.SendTo(m.TGUserID, text)
	}
}

func (l *loop) persist(o *orbitState) {
	err := l.st.SaveSession(o.id, store.SessionSnapshot{
		Mode:            o.sess.Mode,
		State:           o.sess.State,
		Current:         o.sess.Current,
		SavedPositionMS: o.sess.SavedPositionMS,
		Queue:           o.sess.Queue,
		Playlist:        o.sess.Playlist,
	})
	if err != nil {
		l.log.Error("persist failed", "orbit", o.id, "err", err)
	}
}

// --- Node events ---

func (l *loop) handleNode(ev hub.Event) {
	switch e := ev.(type) {
	case hub.EvRegistered:
		o := l.orbit(e.Key.Orbit)
		o.sess.EnsurePeer(e.Key.Slot) // a slot paired after orbit warm-up
		l.log.Info("node registered", "orbit", e.Key.Orbit, "slot", e.Key.Slot, "app", e.AppVersion, "librespot", e.LibrespotVersion)
		o.versions[e.Key.Slot] = e.AppVersion + "/librespot " + e.LibrespotVersion
		vol, ok := o.volumes[e.Key.Slot]
		if !ok {
			vol = 80
			o.volumes[e.Key.Slot] = vol
		}
		snap := o.sess.Snapshot(vol)
		l.hub.Send(e.Key, protocol.TypeWelcome, &protocol.WelcomePayload{SessionSnapshot: snap})
		if off, ok := o.offsets[e.Key.Slot]; ok {
			l.hub.Send(e.Key, protocol.TypeSetOffset, &protocol.SetOffsetPayload{OffsetMS: off})
		}
	case hub.EvOnline:
		o := l.orbit(e.Key.Orbit)
		l.apply(o, o.sess.OnNodeBack(e.Key.Slot))
	case hub.EvOffline:
		o := l.orbit(e.Key.Orbit)
		l.log.Warn("node offline", "orbit", e.Key.Orbit, "slot", e.Key.Slot)
		l.st.LogEvent(string(e.Key.Slot), "offline", nil)
		l.apply(o, o.sess.OnNodeOffline(e.Key.Slot))
	case hub.EvMessage:
		l.handleNodeMessage(e)
	}
}

func (l *loop) handleNodeMessage(m hub.EvMessage) {
	o := l.orbit(m.Key.Orbit)
	slot := m.Key.Slot
	now := time.Now().UnixMilli()
	switch p := m.Payload.(type) {
	case *protocol.StatePayload:
		o.lastSeen[slot] = p
		o.volumes[slot] = p.Volume
		l.apply(o, o.sess.OnHeartbeat(slot, p.PositionMS, p.RTTMS))
		if o.restoredPaused {
			o.sess.RefreshSavedPosition()
		}
	case *protocol.ReadyPayload:
		l.apply(o, o.sess.OnReady(now, slot, p.ElementID))
	case *protocol.StartedPayload:
		l.apply(o, o.sess.OnStarted(slot, p.ElementID, p.TFirstSampleCoordMS))
	case *protocol.EndedPayload:
		l.apply(o, o.sess.OnEnded(slot, p.ElementID, p.Reason))
	case *protocol.VoiceStartedPayload:
		l.log.Info("voice started", "orbit", o.id, "slot", slot, "element", p.ElementID)
	case *protocol.VoiceEndedPayload:
		l.apply(o, o.sess.OnVoiceEnded(slot, p.ElementID))
	case *protocol.WaitEndedPayload:
		l.apply(o, o.sess.OnWaitEnded(slot, p.ElementID))
	case *protocol.ErrorPayload:
		l.log.Warn("node error", "orbit", o.id, "slot", slot, "code", p.Code, "msg", p.Message, "element", p.ElementID)
		l.st.LogEvent(string(slot), "node_error", p)
		l.apply(o, o.sess.OnNodeError(slot, p.Code, p.ElementID))
	case *protocol.ExternalPlaybackPayload:
		l.handleExternalPlayback(o, slot, p.URI)
	default:
		l.log.Debug("unhandled message", "slot", slot, "type", m.Env.Type)
	}
}

// handleExternalPlayback applies the takeover policy (U9).
func (l *loop) handleExternalPlayback(o *orbitState, slot protocol.NodeID, uri string) {
	if o.sess.Mode != session.ModeShared {
		return
	}
	l.st.LogEvent(string(slot), "external_playback", map[string]string{"uri": uri, "policy": o.takeoverPolicy})
	if o.takeoverPolicy == "user" {
		l.notify(o, fmt.Sprintf("дом %s забрал управление (играет с телефона) — режим solo", slot))
		l.apply(o, o.sess.SetModeSolo())
		return
	}
	l.notify(o, fmt.Sprintf("дом %s вмешался с телефона — эфир восстановлен", slot))
	if o.sess.State == session.StatePlaying {
		l.apply(o, o.sess.CmdSync())
		return
	}
	l.hub.Send(hub.NodeKey{Orbit: o.id, Slot: slot}, protocol.TypeStop, &protocol.StopPayload{})
}

// --- Bot events: onboarding, roles, commands (spec ch. 9 + v2.1 M1) ---

const strangerHello = `Привет! Я Барицентр — общий музыкальный эфир на несколько домов: синхронное звучание, очередь треков из этого чата, голосовые вставки между песнями.

/create — создать свой барицентр
Или открой инвайт-ссылку от того, кто уже в системе.`

func (l *loop) handleBot(ev bot.Event) {
	member, err := l.st.MemberOf(ev.FromUserID)
	if err != nil {
		l.log.Error("membership lookup failed", "err", err)
		return
	}
	if member == nil {
		l.handleStranger(ev)
		return
	}
	o := l.orbit(member.OrbitID)

	if ev.Voice != nil {
		if member.Role == "satellite" && false { // satellites may voice: allowed by design
			return
		}
		l.handleVoice(o, ev)
		return
	}

	cmd := ev.Command

	// Role gate: satellites contribute (tracks, voices, info) but do not
	// steer the air (design §2).
	if member.Role == "satellite" {
		switch cmd.Kind {
		case bot.KindLink, bot.KindQueue, bot.KindNow, bot.KindStatus,
			bot.KindStart, bot.KindShare, bot.KindOrbit, bot.KindPairCode:
		default:
			ev.Reply("это управление эфиром — оно у companion'ов. Твоё оружие: треки и голосовые")
			return
		}
	}

	switch cmd.Kind {

	case bot.KindStart:
		ev.Reply(fmt.Sprintf("ты уже в орбите «%s». /help — команды, /orbit — участники", o.title))

	case bot.KindCreate:
		ev.Reply(fmt.Sprintf("у тебя уже есть орбит «%s» — вторая вселенная пока не положена (M4)", o.title))

	case bot.KindShare:
		code, err := l.st.NewInvite(o.id, ev.FromUserID)
		if err != nil {
			ev.Reply("не смог создать приглашение")
			return
		}
		link := fmt.Sprintf("https://t.me/%s?start=%s", l.botUsername(), code)
		ev.Reply(fmt.Sprintf("приглашение в «%s» (48 часов, одноразовое):\n%s", o.title, link))

	case bot.KindPairCode:
		code, err := l.st.NewPairCode(o.id, ev.FromUserID)
		if err != nil {
			ev.Reply("не смог создать код")
			return
		}
		ev.Reply(fmt.Sprintf("код для твоего Пульсара (5 минут):\n\n%s\n\nВведи его в приложении Pulsar при первом запуске — и твой дом подключится к эфиру", code))

	case bot.KindOrbit:
		ev.Reply(l.orbitText(o))

	case bot.KindMakePrimary:
		if member.Role != "primary" {
			ev.Reply("передать главную звезду может только primary")
			return
		}
		if cmd.Number == 0 {
			ev.Reply(l.orbitText(o) + "\n\n/make_primary <id> передаст титул")
			return
		}
		if err := l.st.TransferPrimary(o.id, int64(cmd.Number)); err != nil {
			ev.Reply("этот id не из нашего орбита (/orbit покажет список)")
			return
		}
		l.notify(o, "главная звезда орбита теперь "+strconv.Itoa(cmd.Number))

	case bot.KindRevoke:
		if member.Role != "primary" {
			ev.Reply("отзывать дома может только primary")
			return
		}
		if err := l.st.RevokeSlot(o.id, cmd.Target); err != nil {
			ev.Reply("не получилось")
			return
		}
		// The revoked slot leaves the peer set immediately so it stops
		// blocking ready barriers and offline gates (M2).
		l.apply(o, o.sess.RemovePeer(time.Now().UnixMilli(), protocol.NodeID(cmd.Target)))
		ev.Reply(fmt.Sprintf("токен дома %s отозван; /pair выдаст новый код", cmd.Target))

	case bot.KindLink:
		if o.sess.Mode != session.ModeShared {
			ev.Reply("сейчас режим solo: /inject подкинет трек партнёру, /periastron вернёт общий эфир")
			return
		}
		el := l.newTrackElement(cmd.URI, ev.FromName)
		l.st.InsertElement(el)
		l.apply(o, o.sess.EnqueueTrack(el))
		if o.sess.Current != nil && o.sess.Current.ID == el.ID {
			ev.Reply("очередь пуста — ставлю сразу: " + trackLabel(el))
		} else {
			ev.Reply(fmt.Sprintf("добавил в очередь под номером %d: %s", o.sess.QueueLen(), trackLabel(el)))
		}

	case bot.KindPlayNow:
		if o.sess.Mode != session.ModeShared {
			ev.Reply("/playnow работает в shared. Сейчас solo")
			return
		}
		el := l.newTrackElement(cmd.URI, ev.FromName)
		l.st.InsertElement(el)
		l.apply(o, o.sess.CmdPlayNow(el))
		ev.Reply("врубаю немедленно: " + trackLabel(el))

	case bot.KindPlaylist:
		if o.sess.Mode != session.ModeShared {
			ev.Reply("общий плейлист — фича shared-режима. Сейчас solo")
			return
		}
		if l.sp == nil {
			ev.Reply("плейлисты заработают после настройки Spotify-приложения на сервере")
			return
		}
		ev.Reply("раскрываю плейлист…")
		uri := cmd.URI
		kind := cmd.Target
		id := uri[strings.LastIndex(uri, ":")+1:]
		reply := ev.Reply
		orbitID := o.id
		go func() {
			var exp *spotify.Expansion
			var err error
			if kind == "album" {
				exp, err = l.sp.ExpandAlbum(id)
			} else {
				exp, err = l.sp.ExpandPlaylist(id)
			}
			d := playlistDone{orbit: orbitID, uri: uri, err: err, reply: reply}
			if exp != nil {
				d.title = exp.Title
				d.tracks = exp.Tracks
			}
			l.playlistCh <- d
		}()

	case bot.KindTakeover:
		o.takeoverPolicy = cmd.Target
		l.st.SetOrbitSetting(o.id, "takeover_policy", cmd.Target)
		if cmd.Target == "user" {
			ev.Reply("политика: телефон главнее — вмешательство переключает орбит в solo (с уведомлением)")
		} else {
			ev.Reply("политика: эфир главнее — вмешательство с телефона откатывается (с уведомлением)")
		}

	case bot.KindQueue:
		ev.Reply(l.queueText(o))

	case bot.KindCancel:
		if _, err := o.sess.Cancel(cmd.Number); err != nil {
			ev.Reply(fmt.Sprintf("в очереди %d элементов, номера %d нет", o.sess.QueueLen(), cmd.Number))
			return
		}
		l.persist(o)
		ev.Reply(fmt.Sprintf("убрал элемент %d, в очереди осталось %d", cmd.Number, o.sess.QueueLen()))

	case bot.KindSkip:
		if effs := o.sess.CmdSkip(); effs != nil {
			l.apply(o, effs)
			ev.Reply("пропустил")
		} else {
			ev.Reply("нечего пропускать")
		}

	case bot.KindPause:
		if effs := o.sess.CmdPause(); effs != nil {
			l.apply(o, effs)
			ev.Reply("пауза")
		} else {
			ev.Reply("и так не играет")
		}

	case bot.KindResume:
		if effs := o.sess.CmdResume(); effs != nil {
			o.restoredPaused = false
			l.apply(o, effs)
			ev.Reply("продолжаю")
		} else {
			ev.Reply("нечего продолжать — пришли ссылку на трек")
		}

	case bot.KindSync:
		if effs := o.sess.CmdSync(); effs != nil {
			l.apply(o, effs)
			ev.Reply("пересинхронизирую с текущей позиции")
		} else {
			ev.Reply("/sync работает во время игры трека")
		}

	case bot.KindVol:
		target := protocol.NodeID(cmd.Target)
		if cmd.Target == "" {
			slot, _ := l.st.SlotOf(o.id, ev.FromUserID)
			if slot == "" {
				ev.Reply("у тебя нет своего дома в орбите — укажи слот: /vol " + strconv.Itoa(cmd.Number) + " a")
				return
			}
			target = protocol.NodeID(slot)
		}
		o.volumes[target] = cmd.Number
		l.st.SetSetting(fmt.Sprintf("volume_%d_%s", o.id, target), strconv.Itoa(cmd.Number))
		if !l.hub.Send(hub.NodeKey{Orbit: o.id, Slot: target}, protocol.TypeSetVolume, &protocol.SetVolumePayload{Volume: cmd.Number}) {
			ev.Reply(fmt.Sprintf("дом %s офлайн, громкость применю при подключении", target))
			return
		}
		ev.Reply(fmt.Sprintf("громкость дома %s: %d", target, cmd.Number))

	case bot.KindMode:
		var effs []session.Effect
		if cmd.Target == "solo" {
			effs = o.sess.SetModeSolo()
		} else {
			effs = o.sess.SetModeShared()
		}
		if effs == nil {
			ev.Reply("уже в этом режиме")
			return
		}
		l.apply(o, effs)

	case bot.KindNow:
		ev.Reply(l.nowText(o))

	case bot.KindStatus:
		ev.Reply(l.statusText(o))

	case bot.KindOffset:
		target := protocol.NodeID(cmd.Target)
		o.offsets[target] = int64(cmd.Number)
		l.st.SetSetting(fmt.Sprintf("offset_%d_%s", o.id, target), strconv.Itoa(cmd.Number))
		l.hub.Send(hub.NodeKey{Orbit: o.id, Slot: target}, protocol.TypeSetOffset, &protocol.SetOffsetPayload{OffsetMS: int64(cmd.Number)})
		ev.Reply(fmt.Sprintf("offset дома %s = %d мс, действует со следующего старта", target, cmd.Number))

	case bot.KindOffsetTest:
		t := time.Now().UnixMilli() + 2000
		payload := &protocol.OffsetTestPayload{TCoordMS: t, Clicks: 5, IntervalMS: 1000}
		slots, _ := l.st.ActiveSlots(o.id)
		sent := 0
		for _, sl := range slots {
			if l.hub.Send(hub.NodeKey{Orbit: o.id, Slot: protocol.NodeID(sl)}, protocol.TypeOffsetTest, payload) {
				sent++
			}
		}
		if sent < len(slots) || sent == 0 {
			ev.Reply("все ноды орбита должны быть онлайн для клик-теста (/status покажет)")
			return
		}
		ev.Reply("5 синхронных кликов через 2 секунды — слушайте")

	case bot.KindInject:
		if o.sess.Mode != session.ModeSolo {
			ev.Reply("в shared просто кинь ссылку в чат; /inject — для solo")
			return
		}
		targets := l.injectTargets(o, ev.FromUserID, cmd.Target)
		sent := 0
		for _, t := range targets {
			if l.hub.Send(hub.NodeKey{Orbit: o.id, Slot: t}, protocol.TypeSoloInject, &protocol.SoloInjectPayload{URI: cmd.URI}) {
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

// handleStranger is the zero-context onboarding path (design §4).
func (l *loop) handleStranger(ev bot.Event) {
	if ev.Voice != nil {
		return // strangers' voices are ignored silently
	}
	switch ev.Command.Kind {
	case bot.KindStart:
		payload := ev.Command.Target
		if strings.HasPrefix(payload, "inv") {
			orbitID, _, err := l.st.ConsumeInvite(payload, "member")
			if err != nil || orbitID == 0 {
				ev.Reply("ссылка-приглашение истекла или уже использована — попроси новую (/share у любого участника)")
				return
			}
			if err := l.st.AddMember(orbitID, ev.FromUserID, "companion"); err != nil {
				ev.Reply("не смог добавить в орбит (возможно, он полон)")
				return
			}
			o := l.orbit(orbitID)
			l.notify(o, fmt.Sprintf("%s теперь в орбите", ev.FromName))
			ev.Reply(fmt.Sprintf("добро пожаловать в «%s»! Кидай ссылки на треки прямо сюда.\nСвой дом в эфире: поставь приложение Pulsar и набери /pair — дам код", o.title))
			return
		}
		ev.Reply(strangerHello)
	case bot.KindCreate:
		title := strings.TrimSpace(ev.Command.Target)
		if title == "" {
			title = "Барицентр " + ev.FromName
		}
		o, err := l.st.CreateOrbit(title, ev.FromUserID)
		if err != nil {
			ev.Reply("не смог создать орбит")
			return
		}
		code, _ := l.st.NewPairCode(o.ID, ev.FromUserID)
		ev.Reply(fmt.Sprintf("орбит «%s» создан, ты — primary.\n\nКод для твоего Пульсара (5 минут): %s\n\n/share пригласит партнёра, /pair выдаст новый код, /help — всё остальное", o.Title, code))
	default:
		ev.Reply("это приватная система общих эфиров. /start расскажет, /create создаст твою собственную")
	}
}

func (l *loop) botUsername() string {
	if l.bot != nil && l.bot.Username != "" {
		return l.bot.Username
	}
	return "barycenter_bot"
}

// injectTargets: explicit slot, "both"=every active slot, default = every
// active slot except the sender's own home.
func (l *loop) injectTargets(o *orbitState, from int64, target string) []protocol.NodeID {
	slots, _ := l.st.ActiveSlots(o.id)
	switch target {
	case "", "both":
		mine := ""
		if target == "" {
			mine, _ = l.st.SlotOf(o.id, from)
		}
		var out []protocol.NodeID
		for _, sl := range slots {
			if sl != mine {
				out = append(out, protocol.NodeID(sl))
			}
		}
		return out
	default:
		return []protocol.NodeID{protocol.NodeID(target)}
	}
}

// --- Voice flow (spec ch. 10) ---

func (l *loop) handleVoice(o *orbitState, ev bot.Event) {
	v := ev.Voice
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
	// Personal is the orbit default (design §5): an explicit «лично» caption
	// forces it; «всем» forces broadcast.
	personal := v.Personal || (o.voiceDefault == "personal" && !v.Broadcast)
	if v.Broadcast {
		personal = false
	}
	reply := ev.Reply
	orbitID := o.id
	from := ev.FromUserID
	fromName := ev.FromName
	go func() {
		oga := filepath.Join(l.cfg.MediaDir, mediaID+".oga")
		wav := filepath.Join(l.cfg.MediaDir, mediaID+".wav")
		var res media.Result
		err := l.bot.DownloadVoice(v.TGFileID, oga)
		if err == nil {
			res, err = media.Process(oga, wav, media.Preset(l.cfg.Media.Preset))
		}
		if err == nil {
			os.Remove(oga)
		}
		l.mediaCh <- mediaDone{orbit: orbitID, mediaID: mediaID, from: from, fromName: fromName, personal: personal, result: res, err: err, reply: reply}
	}()
}

func (l *loop) handleMediaDone(d mediaDone) {
	if d.err != nil {
		l.log.Error("voice processing failed", "media", d.mediaID, "err", d.err)
		l.st.UpdateMedia(store.MediaRecord{ID: d.mediaID, Status: "failed"})
		d.reply("не смог обработать голосовое, оставил исходник для разбора")
		return
	}
	o := l.orbit(d.orbit)
	l.st.UpdateMedia(store.MediaRecord{
		ID: d.mediaID, DurationMS: d.result.DurationMS,
		PathWAV: d.result.WAVPath, LoudnormJSON: d.result.LoudnormJSON, Status: "ready",
	})
	// Personal target: every active slot except the sender's own home.
	// In a two-home orbit that is exactly the partner (design §5).
	target := "both"
	if d.personal {
		mine, _ := l.st.SlotOf(o.id, d.from)
		slots, _ := l.st.ActiveSlots(o.id)
		var others []string
		for _, sl := range slots {
			if sl != mine {
				others = append(others, sl)
			}
		}
		if len(others) == 1 {
			target = others[0]
		} else if len(others) == 0 {
			d.reply("в орбите пока только твой дом — отправлю всем, когда появятся другие")
			return
		}
		// >1 recipients for a personal voice: M1 keeps it simple — broadcast
		// to others is not expressible per-element yet, ship to all.
	}
	el := session.Element{
		ID:          ulid.NewElementID(time.Now()),
		Kind:        session.KindVoice,
		MediaID:     d.mediaID,
		DurationMS:  d.result.DurationMS,
		RequestedBy: protocol.NodeID(d.fromName),
		Target:      target,
		CreatedAt:   time.Now().UnixMilli(),
	}
	l.st.InsertElement(el)

	if o.sess.Mode == session.ModeShared {
		l.apply(o, o.sess.EnqueueVoice(el))
		if target != "both" {
			d.reply("личная вставка встанет после текущего трека")
		} else {
			d.reply("вставка встанет после текущего трека")
		}
		return
	}
	payload := &protocol.SoloVoicePayload{ElementID: el.ID, FileURL: l.mediaURL(d.mediaID)}
	targets, _ := l.st.ActiveSlots(o.id)
	if target != "both" {
		targets = []string{target}
	}
	sent := 0
	for _, t := range targets {
		if l.hub.Send(hub.NodeKey{Orbit: o.id, Slot: protocol.NodeID(t)}, protocol.TypeSoloVoice, payload) {
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

func (l *loop) apply(o *orbitState, effs []session.Effect) {
	key := func(to protocol.NodeID) hub.NodeKey { return hub.NodeKey{Orbit: o.id, Slot: to} }
	for _, eff := range effs {
		switch e := eff.(type) {
		case session.EffLoad:
			l.hub.Send(key(e.To), protocol.TypeLoad, &protocol.LoadPayload{ElementID: e.ElementID, URI: e.URI, PositionMS: e.PositionMS})
		case session.EffResumeAt:
			l.hub.Send(key(e.To), protocol.TypeResumeAt, &protocol.ResumeAtPayload{ElementID: e.ElementID, TCoordMS: e.TCoordMS})
		case session.EffPause:
			l.hub.Send(key(e.To), protocol.TypePause, &protocol.PausePayload{ElementID: e.ElementID, FadeMS: e.FadeMS})
		case session.EffPlayVoice:
			l.hub.Send(key(e.To), protocol.TypePlayVoice, &protocol.PlayVoicePayload{
				ElementID: e.ElementID,
				FileURL:   l.mediaURL(e.MediaID),
			})
		case session.EffWait:
			l.hub.Send(key(e.To), protocol.TypeWait, &protocol.WaitPayload{ElementID: e.ElementID, DurationMS: e.DurationMS})
		case session.EffStop:
			l.hub.Send(key(e.To), protocol.TypeStop, &protocol.StopPayload{})
		case session.EffSetMode:
			l.hub.Send(key(e.To), protocol.TypeSetMode, &protocol.SetModePayload{Mode: string(e.Mode)})
		case session.EffNotify:
			l.notify(o, e.Text)
		case session.EffArmReadyTimer:
			l.armReadyTimer(o, e.ElementID)
		case session.EffCancelReadyTimer:
			l.cancelReadyTimer(o)
		case session.EffLogDesync:
			o.lastDesyncMS = e.DeltaMS
			l.log.Info("start desync measured", "orbit", o.id, "delta_ms", e.DeltaMS)
			l.st.LogEvent("session", "desync", map[string]int64{"delta_ms": e.DeltaMS, "orbit": o.id})
		case session.EffElementDone:
			l.st.MarkElementDone(e.Element.ID, e.Status, time.Now().UnixMilli())
		case session.EffPersist:
			l.persist(o)
		}
	}
}

func (l *loop) armReadyTimer(o *orbitState, elementID string) {
	l.cancelReadyTimer(o)
	o.timerElement = elementID
	d := time.Duration(l.cfg.Timings.ReadyTimeoutS) * time.Second
	orbitID := o.id
	o.readyTimer = time.AfterFunc(d, func() { l.timeouts <- orbitTimeout{orbit: orbitID, elementID: elementID} })
}

func (l *loop) cancelReadyTimer(o *orbitState) {
	if o.readyTimer != nil {
		o.readyTimer.Stop()
		o.readyTimer = nil
		o.timerElement = ""
	}
}

// --- Status texts (spec 9.1 /now /queue /status /orbit) ---

func (l *loop) newTrackElement(uri string, fromName string) session.Element {
	return session.Element{
		ID:          ulid.NewElementID(time.Now()),
		Kind:        session.KindTrack,
		URI:         uri,
		RequestedBy: protocol.NodeID(fromName),
		Target:      "both",
		CreatedAt:   time.Now().UnixMilli(),
	}
}

func trackLabel(el session.Element) string {
	if el.Title != "" {
		return el.Title
	}
	return el.URI
}

func fmtMS(ms int64) string {
	s := ms / 1000
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

func (l *loop) orbitText(o *orbitState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "орбит «%s»\n", o.title)
	members, _ := l.st.Members(o.id)
	for _, m := range members {
		slot, _ := l.st.SlotOf(o.id, m.TGUserID)
		home := "без дома"
		if slot != "" {
			home = "дом " + slot
		}
		fmt.Fprintf(&b, "· %d — %s (%s)\n", m.TGUserID, m.Role, home)
	}
	slots, _ := l.st.ActiveSlots(o.id)
	online := l.hub.Online(o.id)
	var parts []string
	for _, sl := range slots {
		mark := "офлайн"
		if online[protocol.NodeID(sl)] {
			mark = "в сети"
		}
		parts = append(parts, sl+": "+mark)
	}
	if len(parts) > 0 {
		fmt.Fprintf(&b, "пульсары: %s", strings.Join(parts, ", "))
	} else {
		b.WriteString("пульсаров пока нет — /pair выдаст код")
	}
	return b.String()
}

func (l *loop) queueText(o *orbitState) string {
	var b strings.Builder
	if cur := o.sess.Current; cur != nil {
		fmt.Fprintf(&b, "сейчас: %s\n", elementLabel(*cur))
	}
	if o.sess.QueueLen() == 0 {
		b.WriteString("очередь вставок пуста")
	} else {
		b.WriteString("очередь:\n")
		for i, el := range o.sess.Queue {
			fmt.Fprintf(&b, "%d. %s (от %s)\n", i+1, elementLabel(el), el.RequestedBy)
		}
	}
	if p := o.sess.Playlist; p != nil {
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
		who := "всем"
		if el.Target != "both" {
			who = "лично в дом " + el.Target
		}
		return fmt.Sprintf("голосовое %s (%s)", fmtMS(el.DurationMS), who)
	}
	return trackLabel(el)
}

func (l *loop) nowText(o *orbitState) string {
	if o.sess.Mode == session.ModeSolo {
		var b strings.Builder
		b.WriteString("апоастрон: каждый слушает своё\n")
		slots, _ := l.st.ActiveSlots(o.id)
		for _, sl := range slots {
			st := o.lastSeen[protocol.NodeID(sl)]
			if st == nil || st.URI == nil {
				fmt.Fprintf(&b, "дом %s: тишина\n", sl)
				continue
			}
			fmt.Fprintf(&b, "дом %s: %s @ %s\n", sl, *st.URI, fmtMS(st.PositionMS))
		}
		return strings.TrimRight(b.String(), "\n")
	}
	if cur := o.sess.Current; cur != nil {
		return fmt.Sprintf("сейчас: %s @ %s (%s)", elementLabel(*cur), fmtMS(l.livePosition(o)), o.sess.State)
	}
	return "тишина — пришли ссылку на трек"
}

func (l *loop) livePosition(o *orbitState) int64 {
	var best int64
	for _, st := range o.lastSeen {
		if st != nil && st.PositionMS > best {
			best = st.PositionMS
		}
	}
	return best
}

func (l *loop) statusText(o *orbitState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "«%s»: режим %s, состояние %s\n", o.title, o.sess.Mode, o.sess.State)
	online := l.hub.Online(o.id)
	slots, _ := l.st.ActiveSlots(o.id)
	for _, sl := range slots {
		n := protocol.NodeID(sl)
		if !online[n] {
			fmt.Fprintf(&b, "дом %s: офлайн\n", n)
			continue
		}
		st := o.lastSeen[n]
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
			n, mark, fmtMS(st.PositionMS), st.Volume, st.RTTMS, o.offsets[n], strings.Join(speakers, " "))
	}
	if o.lastDesyncMS > 0 {
		fmt.Fprintf(&b, "рассинхрон последнего старта: %d мс\n", o.lastDesyncMS)
	}
	fmt.Fprintf(&b, "координатор %s", version)
	for n, v := range o.versions {
		fmt.Fprintf(&b, ", нода %s %s", n, v)
	}
	return b.String()
}
