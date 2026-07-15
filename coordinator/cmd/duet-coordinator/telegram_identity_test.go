package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/bot"
	"relux.works/duet/coordinator/internal/config"
	"relux.works/duet/coordinator/internal/store"
)

type telegramPairResponse struct {
	OrbitID int64  `json:"orbit_id"`
	Slot    string `json:"slot"`
	Token   string `json:"token"`
}

func newTelegramIdentityLoop(t *testing.T, cfg *config.Config, st *store.Store) (*loop, *fakeSender) {
	t.Helper()
	fake := &fakeSender{}
	l := newLoop(slog.Default(), cfg, fake, st, nil, nil)
	l.warmup()
	return l, fake
}

func telegramBotEvent(t *testing.T, userID int64, chatType, text string, replies *replies) bot.Event {
	t.Helper()
	command, err := bot.Parse(text)
	if err != nil {
		t.Fatal("parse Telegram identity event")
	}
	return bot.Event{
		ChatID:     userID,
		ChatType:   chatType,
		MessageID:  42,
		FromUserID: userID,
		FromName:   "Telegram User",
		Command:    command,
		Reply:      replies.fn,
	}
}

type telegramR6RoundTripFunc func(*http.Request) (*http.Response, error)

func (f telegramR6RoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type telegramR6OutboundCall struct {
	chatID    int64
	messageID int64
	text      string
}

type telegramR6BotAPI struct {
	http        *bot.HTTPAPI
	updates     chan []bot.Update
	pollStop    chan struct{}
	sent        chan telegramR6OutboundCall
	deleted     chan telegramR6OutboundCall
	blockSend   bool
	blockMu     sync.Mutex
	blocked     bool
	sendStarted chan struct{}
	releaseSend chan struct{}
	releaseOnce sync.Once
}

func newTelegramR6BotAPI(token, rawCause string, blockFirstSend bool) *telegramR6BotAPI {
	client := &http.Client{Transport: telegramR6RoundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		return nil, fmt.Errorf("%s url=%s body=%s", rawCause, request.URL.String(), string(body))
	})}
	api := &telegramR6BotAPI{
		http:        &bot.HTTPAPI{Token: token, Client: client},
		updates:     make(chan []bot.Update, 8),
		pollStop:    make(chan struct{}),
		sent:        make(chan telegramR6OutboundCall, 2048),
		deleted:     make(chan telegramR6OutboundCall, 16),
		blockSend:   blockFirstSend,
		sendStarted: make(chan struct{}),
		releaseSend: make(chan struct{}),
	}
	return api
}

func (a *telegramR6BotAPI) GetUpdates(int64, int) ([]bot.Update, error) {
	select {
	case updates := <-a.updates:
		return updates, nil
	case <-a.pollStop:
		return nil, nil
	}
}

func (a *telegramR6BotAPI) SendMessage(chatID int64, text string) error {
	a.sent <- telegramR6OutboundCall{chatID: chatID, text: text}
	shouldBlock := false
	if a.blockSend {
		a.blockMu.Lock()
		if !a.blocked {
			a.blocked = true
			shouldBlock = true
			close(a.sendStarted)
		}
		a.blockMu.Unlock()
	}
	if shouldBlock {
		<-a.releaseSend
	}
	return a.http.SendMessage(chatID, text)
}

func (a *telegramR6BotAPI) DeleteMessage(chatID, messageID int64) error {
	a.deleted <- telegramR6OutboundCall{chatID: chatID, messageID: messageID}
	return a.http.DeleteMessage(chatID, messageID)
}

func (a *telegramR6BotAPI) FileURL(fileID string) (string, error) {
	return a.http.FileURL(fileID)
}

func (a *telegramR6BotAPI) Download(fileURL, destinationPath string) error {
	return a.http.Download(fileURL, destinationPath)
}

func (a *telegramR6BotAPI) GetMe() (string, error)     { return "barycenter_bot", nil }
func (a *telegramR6BotAPI) SetMyCommands(string) error { return nil }

func (a *telegramR6BotAPI) releaseBlockedSender() {
	a.releaseOnce.Do(func() { close(a.releaseSend) })
}

type telegramR6LogCapture struct {
	mu      sync.Mutex
	records []string
	events  chan string
}

func newTelegramR6LogCapture() *telegramR6LogCapture {
	return &telegramR6LogCapture{events: make(chan string, 4096)}
}

func (h *telegramR6LogCapture) Enabled(context.Context, slog.Level) bool { return true }

func (h *telegramR6LogCapture) Handle(_ context.Context, record slog.Record) error {
	var line strings.Builder
	line.WriteString(record.Message)
	record.Attrs(func(attribute slog.Attr) bool {
		line.WriteByte(' ')
		line.WriteString(attribute.Key)
		line.WriteByte('=')
		line.WriteString(fmt.Sprint(attribute.Value.Any()))
		return true
	})
	rendered := line.String()
	h.mu.Lock()
	h.records = append(h.records, rendered)
	h.mu.Unlock()
	h.events <- rendered
	return nil
}

func (h *telegramR6LogCapture) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *telegramR6LogCapture) WithGroup(string) slog.Handler      { return h }

func (h *telegramR6LogCapture) String() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return strings.Join(h.records, "\n")
}

func (h *telegramR6LogCapture) waitFor(t *testing.T, fragment string) {
	t.Helper()
	for {
		select {
		case rendered := <-h.events:
			if strings.Contains(rendered, fragment) {
				return
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("expected log operation %q was not observed", fragment)
		}
	}
}

type telegramR6BotHarness struct {
	bot      *bot.Bot
	api      *telegramR6BotAPI
	logs     *telegramR6LogCapture
	stop     chan struct{}
	botDone  chan struct{}
	loopDone chan struct{}
}

func startTelegramR6BotHarness(
	t *testing.T,
	cfg *config.Config,
	st *store.Store,
	api *telegramR6BotAPI,
) *telegramR6BotHarness {
	t.Helper()
	logs := newTelegramR6LogCapture()
	logger := slog.New(logs)
	realBot := bot.New(api, logger)
	l := newLoop(logger, cfg, &fakeSender{}, st, realBot, nil)
	l.warmup()
	harness := &telegramR6BotHarness{
		bot:      realBot,
		api:      api,
		logs:     logs,
		stop:     make(chan struct{}),
		botDone:  make(chan struct{}),
		loopDone: make(chan struct{}),
	}
	go func() {
		l.run(harness.stop, nil)
		close(harness.loopDone)
	}()
	go func() {
		realBot.Run(harness.stop)
		close(harness.botDone)
	}()
	t.Cleanup(func() {
		close(harness.stop)
		close(api.pollStop)
		api.releaseBlockedSender()
		select {
		case <-harness.loopDone:
		case <-time.After(3 * time.Second):
			t.Error("Telegram loop did not stop")
		}
		select {
		case <-harness.botDone:
		case <-time.After(3 * time.Second):
			t.Error("Telegram bot did not stop")
		}
	})
	return harness
}

func (h *telegramR6BotHarness) submit(userID, messageID int64, displayName, text string) {
	h.api.updates <- []bot.Update{{
		UpdateID: messageID,
		Message: &bot.Message{
			MessageID: messageID,
			From:      &bot.User{ID: userID, FirstName: displayName},
			Chat:      bot.Chat{ID: userID, Type: "private"},
			Text:      text,
		},
	}}
}

func waitTelegramR6Outbound(t *testing.T, calls <-chan telegramR6OutboundCall, operation string) telegramR6OutboundCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(3 * time.Second):
		t.Fatalf("Telegram %s attempt was not observed", operation)
		return telegramR6OutboundCall{}
	}
}

func assertTelegramR6Redacted(t *testing.T, rendered string, identifiers ...string) {
	t.Helper()
	for _, identifier := range identifiers {
		if identifier != "" && strings.Contains(rendered, identifier) {
			t.Fatalf("private Telegram material was present in captured logs")
		}
	}
}

func telegramR6IdentifierCanaries(userID, messageID int64, extras ...string) []string {
	user := strconv.FormatInt(userID, 10)
	message := strconv.FormatInt(messageID, 10)
	return append(extras,
		user,
		message,
		"chat="+user,
		"message="+message,
		"chat_id="+user,
		"message_id="+message,
		`"chat":`+user,
		`"message":`+message,
		`"chat_id":`+user,
		`"message_id":`+message,
	)
}

func pairCodeFromReply(t *testing.T, reply string) string {
	t.Helper()
	const open, close = "<code>", "</code>"
	start := strings.Index(reply, open)
	if start < 0 {
		t.Fatal("pair response did not contain a code element")
	}
	start += len(open)
	end := strings.Index(reply[start:], close)
	if end <= 0 {
		t.Fatal("pair response contained an invalid code element")
	}
	return reply[start : start+end]
}

func consumePairCode(t *testing.T, st *store.Store, cfg *config.Config, code string) telegramPairResponse {
	t.Helper()
	body, err := json.Marshal(map[string]string{"code": code})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/pair", bytes.NewReader(body))
	response := httptest.NewRecorder()
	pairHandler(slog.Default(), st, cfg).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("pair status=%d", response.Code)
	}
	var result telegramPairResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMigratedTelegramActorContextKeepsLegacyAndSelfServicePairCompatibility(t *testing.T) {
	cfg := testConfig(t)
	cfg.SelfServiceOnboarding = true
	st, err := store.OpenWithOptions(cfg.DBPath, store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	orbit, err := st.BootstrapLegacyOrbit(
		map[string]string{"a": cfg.Nodes["a"].Token, "b": cfg.Nodes["b"].Token},
		cfg.Telegram.Users,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddMember(orbit.ID, 333, "satellite"); err != nil {
		t.Fatal(err)
	}
	l, _ := newTelegramIdentityLoop(t, cfg, st)

	var primaryID, companionID int64
	for _, userID := range []int64{111, 222, 333} {
		legacy, err := st.MemberOf(userID)
		if err != nil || legacy == nil {
			t.Fatalf("legacy authorization user=%d member=%+v err=%v", userID, legacy, err)
		}
		member, err := l.telegramCommandMember(userID)
		if err != nil || member == nil || member.OrbitID != orbit.ID || member.Role != legacy.Role {
			t.Fatalf("actor-context authorization user=%d member=%+v err=%v", userID, member, err)
		}
		switch legacy.Role {
		case "primary":
			primaryID = userID
		case "companion":
			companionID = userID
		}
	}
	if primaryID == 0 || companionID == 0 {
		t.Fatal("migrated legacy roles did not contain primary and companion")
	}
	shareReplies := &replies{}
	l.handleBot(telegramBotEvent(t, companionID, "private", "/share", shareReplies))
	if !strings.Contains(shareReplies.last(t), "https://t.me/") || !strings.Contains(shareReplies.last(t), "?start=inv") {
		t.Fatal("migrated companion did not retain member-share authorization")
	}

	legacyReplies := &replies{}
	l.handleBot(telegramBotEvent(t, companionID, "private", "/pair", legacyReplies))
	pair := consumePairCode(t, st, cfg, pairCodeFromReply(t, legacyReplies.last(t)))
	if pair.OrbitID != orbit.ID || pair.Slot != "c" || len(pair.Token) != 64 {
		t.Fatalf("mixed legacy pair coordinates orbit=%d slot=%q token_length=%d", pair.OrbitID, pair.Slot, len(pair.Token))
	}
	ctx, err := st.ResolveTokenActorContext(pair.Token)
	if err != nil || ctx.OrbitID != orbit.ID || ctx.Role != "companion" || ctx.Capabilities != store.CapabilityNode {
		t.Fatalf("self-service pair context=%+v err=%v", ctx, err)
	}
	for slot, token := range map[string]string{"a": cfg.Nodes["a"].Token, "b": cfg.Nodes["b"].Token} {
		gotOrbit, gotSlot, found, err := st.LookupPlaybackToken(token)
		if err != nil || !found || gotOrbit != orbit.ID || gotSlot != slot {
			t.Fatalf("legacy node %s changed: orbit=%d slot=%q found=%v err=%v", slot, gotOrbit, gotSlot, found, err)
		}
	}

	satelliteReplies := &replies{}
	l.handleBot(telegramBotEvent(t, 333, "private", "/pause", satelliteReplies))
	if !strings.Contains(satelliteReplies.last(t), "companion") {
		t.Fatalf("satellite role gate reply=%q", satelliteReplies.last(t))
	}

	// Leave the legacy membership intact while revoking only its additive
	// actor. A bot path that fell back to MemberOf would still authorize it.
	inspect, err := sql.Open("sqlite", cfg.DBPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	res, err := inspect.Exec(`UPDATE actors SET revoked_at = ?
WHERE kind = 'telegram_user' AND external_ref = ?`, time.Now().UnixMilli(), strconv.FormatInt(primaryID, 10))
	if err != nil {
		t.Fatal(err)
	}
	if affected, err := res.RowsAffected(); err != nil || affected != 1 {
		t.Fatalf("revoked actor rows=%d err=%v", affected, err)
	}
	legacy, err := st.MemberOf(primaryID)
	if err != nil || legacy == nil || legacy.Role != "primary" {
		t.Fatalf("legacy control row unexpectedly changed: member=%+v err=%v", legacy, err)
	}
	member, err := l.telegramCommandMember(primaryID)
	if !errors.Is(err, errTelegramActorLifecycleDenied) || member != nil {
		t.Fatalf("revoked actor authorized through bot: member=%+v err=%v", member, err)
	}
	revokedReplies := &replies{}
	l.handleBot(telegramBotEvent(t, primaryID, "private", "/pair", revokedReplies))
	if len(revokedReplies.texts) != 0 {
		t.Fatalf("revoked actor received bot reply=%q", revokedReplies.texts)
	}
}

func TestTelegramActorLifecycleControlsStrangerOnboarding(t *testing.T) {
	cfg := testConfig(t)
	cfg.SelfServiceOnboarding = true
	st, err := store.OpenWithOptions(cfg.DBPath, store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	l, _ := newTelegramIdentityLoop(t, cfg, st)

	const (
		issuerID   int64 = 8100
		unknownID  int64 = 8101
		leftID     int64 = 8102
		revokedID  int64 = 8103
		disabledID int64 = 8104
	)
	target, err := st.CreateOrbit("Lifecycle target", issuerID)
	if err != nil {
		t.Fatal(err)
	}

	inspect, err := sql.Open("sqlite", cfg.DBPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	countRows := func(query string, args ...any) int {
		t.Helper()
		var count int
		if err := inspect.QueryRow(query, args...).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	inviteUnused := func(code string) bool {
		t.Helper()
		var usedAt sql.NullInt64
		if err := inspect.QueryRow(`SELECT used_at FROM invites WHERE code = ?`, code).Scan(&usedAt); err != nil {
			t.Fatal(err)
		}
		return !usedAt.Valid
	}

	unknownInvite, err := st.NewInvite(target.ID, issuerID)
	if err != nil {
		t.Fatal(err)
	}
	unknownReplies := &replies{}
	l.handleBot(telegramBotEvent(t, unknownID, "private", "/start "+unknownInvite, unknownReplies))
	if len(unknownReplies.texts) != 1 || inviteUnused(unknownInvite) {
		t.Fatalf("unknown actor did not enter invite onboarding: replies=%q invite_unused=%t", unknownReplies.texts, inviteUnused(unknownInvite))
	}
	unknownMember, err := st.MemberOf(unknownID)
	if err != nil || unknownMember == nil || unknownMember.OrbitID != target.ID || unknownMember.Role != "companion" {
		t.Fatalf("unknown actor membership=%+v err=%v", unknownMember, err)
	}

	leftOrbit, err := st.CreateOrbit("Actor that leaves", leftID)
	if err != nil {
		t.Fatal(err)
	}
	if dissolved, _, err := st.LeaveOrbit(leftOrbit.ID, leftID); err != nil || !dissolved {
		t.Fatalf("leave setup dissolved=%t err=%v", dissolved, err)
	}
	leftCtx, err := st.ResolveTelegramActorContext(leftID)
	if !errors.Is(err, store.ErrInsufficientCapability) || leftCtx.ActorID == 0 || leftCtx.OrbitID != 0 {
		t.Fatalf("left actor classification=%+v err=%v", leftCtx, err)
	}
	leftInvite, err := st.NewInvite(target.ID, issuerID)
	if err != nil {
		t.Fatal(err)
	}
	leftReplies := &replies{}
	l.handleBot(telegramBotEvent(t, leftID, "private", "/start "+leftInvite, leftReplies))
	if len(leftReplies.texts) != 1 || inviteUnused(leftInvite) {
		t.Fatalf("left actor did not enter re-onboarding: replies=%q invite_unused=%t", leftReplies.texts, inviteUnused(leftInvite))
	}
	leftMember, err := st.MemberOf(leftID)
	if err != nil || leftMember == nil || leftMember.OrbitID != target.ID || leftMember.Role != "companion" {
		t.Fatalf("left actor re-onboarding membership=%+v err=%v", leftMember, err)
	}

	setupBlocked := func(t *testing.T, telegramID int64, disable bool) {
		t.Helper()
		owned, err := st.CreateOrbit("Blocked actor", telegramID)
		if err != nil {
			t.Fatal(err)
		}
		var actorID int64
		if err := inspect.QueryRow(`SELECT id FROM actors WHERE kind = 'telegram_user' AND external_ref = ?`, strconv.FormatInt(telegramID, 10)).Scan(&actorID); err != nil {
			t.Fatal(err)
		}
		if disable {
			if _, err := inspect.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, owned.ID); err != nil {
				t.Fatal(err)
			}
		} else if _, err := inspect.Exec(`UPDATE actors SET revoked_at = ? WHERE id = ?`, time.Now().UnixMilli(), actorID); err != nil {
			t.Fatal(err)
		}
		invite, err := st.NewInvite(target.ID, issuerID)
		if err != nil {
			t.Fatal(err)
		}
		orbitsBefore := countRows(`SELECT COUNT(*) FROM orbits`)
		membersBefore := countRows(`SELECT COUNT(*) FROM members`)
		membershipsBefore := countRows(`SELECT COUNT(*) FROM memberships`)
		startReplies := &replies{}
		l.handleBot(telegramBotEvent(t, telegramID, "private", "/start "+invite, startReplies))
		createReplies := &replies{}
		l.handleBot(telegramBotEvent(t, telegramID, "private", "/create forbidden", createReplies))
		if len(startReplies.texts) != 0 || len(createReplies.texts) != 0 {
			t.Fatalf("lifecycle-blocked actor received replies: start=%q create=%q", startReplies.texts, createReplies.texts)
		}
		if !inviteUnused(invite) {
			t.Fatal("lifecycle-blocked actor burned a member invite")
		}
		if got := countRows(`SELECT COUNT(*) FROM orbits`); got != orbitsBefore {
			t.Fatalf("lifecycle-blocked actor created orbit: before=%d after=%d", orbitsBefore, got)
		}
		if got := countRows(`SELECT COUNT(*) FROM members`); got != membersBefore {
			t.Fatalf("lifecycle-blocked actor changed legacy members: before=%d after=%d", membersBefore, got)
		}
		if got := countRows(`SELECT COUNT(*) FROM memberships`); got != membershipsBefore {
			t.Fatalf("lifecycle-blocked actor changed memberships: before=%d after=%d", membershipsBefore, got)
		}
	}
	t.Run("disabled", func(t *testing.T) { setupBlocked(t, disabledID, true) })
	t.Run("revoked", func(t *testing.T) { setupBlocked(t, revokedID, false) })
}

func TestAppOwnedOrbitTelegramLinkUsesTrustedAdapterAndPreservesPairOwnership(t *testing.T) {
	cfg := testConfig(t)
	cfg.SelfServiceOnboarding = true
	st, err := store.OpenWithOptions(cfg.DBPath, store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	owner, err := st.CreateSelfServiceOrbit("App-owned orbit")
	if err != nil {
		t.Fatal(err)
	}
	l, _ := newTelegramIdentityLoop(t, cfg, st)

	issued, err := st.IssueTelegramLink(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	groupReplies := &replies{}
	group := telegramBotEvent(t, 444, "group", issued.Code, groupReplies)
	groupDeleted := false
	group.DeleteSource = func() { groupDeleted = true }
	l.handleBot(group)
	if groupDeleted || groupReplies.last(t) != "The provided credential is not valid." || strings.Contains(groupReplies.last(t), issued.Code) {
		t.Fatal("group consume was not rejected uniformly")
	}

	privateReplies := &replies{}
	private := telegramBotEvent(t, 444, "private", issued.Code, privateReplies)
	privateDeleted := false
	private.DeleteSource = func() { privateDeleted = true }
	l.handleBot(private)
	if !privateDeleted || !strings.Contains(privateReplies.last(t), "linked") || strings.Contains(privateReplies.last(t), issued.Code) {
		t.Fatal("private consume did not confirm and delete without echoing the credential")
	}
	telegramContext, err := st.ResolveTelegramActorContext(444)
	if err != nil || telegramContext.OrbitID != owner.OrbitID || telegramContext.Role != "companion" || telegramContext.Capabilities != store.CapabilityTelegram {
		t.Fatalf("linked Telegram context=%+v err=%v", telegramContext, err)
	}

	pairReplies := &replies{}
	l.handleBot(telegramBotEvent(t, 444, "private", "/pair", pairReplies))
	pair := consumePairCode(t, st, cfg, pairCodeFromReply(t, pairReplies.last(t)))
	if pair.OrbitID != owner.OrbitID || pair.Slot != "b" || len(pair.Token) != 64 {
		t.Fatalf("linked pair coordinates orbit=%d slot=%q token_length=%d", pair.OrbitID, pair.Slot, len(pair.Token))
	}
	pairedContext, err := st.ResolveTokenActorContext(pair.Token)
	if err != nil || pairedContext.ActorID == owner.ActorID || pairedContext.OrbitID != owner.OrbitID || pairedContext.Role != "companion" || pairedContext.Capabilities != store.CapabilityNode {
		t.Fatalf("linked pair context=%+v owner_actor=%d err=%v", pairedContext, owner.ActorID, err)
	}
	ownerContext, err := st.ResolveTokenActorContext(owner.ControlToken)
	if err != nil || ownerContext.ActorID != owner.ActorID || ownerContext.Role != "primary" || ownerContext.Capabilities&store.CapabilityControl == 0 {
		t.Fatalf("app owner context changed=%+v err=%v", ownerContext, err)
	}

	redactionLink, err := st.IssueTelegramLink(owner.ActorID, owner.ControlToken, "satellite")
	if err != nil {
		t.Fatal(err)
	}
	inspect, err := sql.Open("sqlite", cfg.DBPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	if _, err := inspect.Exec(`CREATE TRIGGER telegram_bot_log_redaction
BEFORE INSERT ON audit_events
WHEN NEW.type = 'telegram_link.consumed'
BEGIN SELECT RAISE(ABORT, 'injected audit failure'); END`); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	redactionLoop := newLoop(slog.New(slog.NewTextHandler(&logs, nil)), cfg, &fakeSender{}, st, nil, nil)
	redactionLoop.warmup()
	redactionReplies := &replies{}
	redactionEvent := telegramBotEvent(t, 445, "private", redactionLink.Code, redactionReplies)
	redactionDeleted := false
	redactionEvent.DeleteSource = func() { redactionDeleted = true }
	redactionLoop.handleBot(redactionEvent)
	if redactionDeleted || redactionReplies.last(t) != "The provided credential is not valid." {
		t.Fatal("failed consume did not use the uniform response without deletion")
	}
	if strings.Contains(logs.String(), redactionLink.Code) || strings.Contains(redactionReplies.last(t), redactionLink.Code) {
		t.Fatal("Telegram link credential entered logs or error output")
	}
}

func TestTelegramLinkRateLimitAuditFailureUsesGenericAdapterResponse(t *testing.T) {
	cfg := testConfig(t)
	cfg.SelfServiceOnboarding = true
	st, err := store.OpenWithOptions(cfg.DBPath, store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	const (
		userID         int64 = 8_323_456_789
		failureMessage int64 = 7_194_628_501
		successMessage int64 = 7_194_628_502
		botToken             = "SENTINEL_RATE_LIMIT_BOT_TOKEN_R6"
		rawCause             = "SENTINEL_RATE_LIMIT_RAW_CAUSE_R6"
		displayName          = "SENTINEL_RATE_LIMIT_DISPLAY_NAME_R6"
	)
	subject := strconv.FormatInt(userID, 10)
	seedCode := randomIdentityHumanSecret(t)
	for attempt := 1; attempt <= 10; attempt++ {
		if _, err := st.ConsumeTelegramLink(userID, "Limited", "private", seedCode); !errors.Is(err, store.ErrTelegramLinkInvalid) {
			t.Fatalf("priming attempt %d error=%v", attempt, err)
		}
	}

	inspect, err := sql.Open("sqlite", cfg.DBPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	if _, err := inspect.Exec(`CREATE TRIGGER telegram_adapter_rate_audit_failure
BEFORE INSERT ON rate_limit_audit_events
BEGIN SELECT RAISE(ABORT, 'injected adapter audit failure'); END`); err != nil {
		t.Fatal(err)
	}

	api := newTelegramR6BotAPI(botToken, rawCause, false)
	harness := startTelegramR6BotHarness(t, cfg, st, api)
	failureCode := randomIdentityHumanSecret(t)
	harness.submit(userID, failureMessage, displayName, failureCode)
	failureReply := waitTelegramR6Outbound(t, api.sent, "generic persistence-failure reply")
	if failureReply.chatID != userID || failureReply.text != "The provided credential is not valid." {
		t.Fatal("persistence failure did not use the generic Bot response")
	}
	harness.logs.waitFor(t, "send failed")
	var audits int
	if err := inspect.QueryRow(`SELECT COUNT(*) FROM rate_limit_audit_events`).Scan(&audits); err != nil || audits != 0 {
		t.Fatalf("failed adapter audit rows=%d err=%v", audits, err)
	}
	if _, err := inspect.Exec(`DROP TRIGGER telegram_adapter_rate_audit_failure`); err != nil {
		t.Fatal(err)
	}

	successCode := randomIdentityHumanSecret(t)
	harness.submit(userID, successMessage, displayName, successCode)
	rateReply := waitTelegramR6Outbound(t, api.sent, "durably audited rate-limit reply")
	if rateReply.chatID != userID || rateReply.text != "Too many attempts. Please wait before retrying." {
		t.Fatal("durably audited rejection did not use the rate-limit Bot response")
	}
	harness.logs.waitFor(t, "send failed")
	var eventType, class, digest string
	var orbitID, actorID sql.NullInt64
	if err := inspect.QueryRow(`SELECT event_type, limiter_class, subject_digest, orbit_id, actor_id
FROM rate_limit_audit_events`).Scan(&eventType, &class, &digest, &orbitID, &actorID); err != nil {
		t.Fatal(err)
	}
	if eventType != "security.rate_limited" || class != string(store.RateLimitTelegramLinkConsumeTelegram) ||
		digest != expectedRateLimitDigest(store.RateLimitTelegramLinkConsumeTelegram, subject) ||
		orbitID.Valid || actorID.Valid {
		t.Fatalf("adapter audit event=%q class=%q digest_matches=%t orbit=%v actor=%v",
			eventType, class,
			digest == expectedRateLimitDigest(store.RateLimitTelegramLinkConsumeTelegram, subject),
			orbitID, actorID)
	}
	var legacyAudits int
	if err := inspect.QueryRow(`SELECT COUNT(*) FROM events
WHERE type = 'telegram_link.rate_limited'`).Scan(&legacyAudits); err != nil || legacyAudits != 0 {
		t.Fatalf("legacy adapter audit rows=%d err=%v", legacyAudits, err)
	}
	rendered := strings.Join([]string{
		harness.logs.String(),
		failureReply.text,
		rateReply.text,
	}, "|")
	canaries := telegramR6IdentifierCanaries(userID, failureMessage,
		botToken, rawCause, displayName, failureCode, successCode, seedCode,
		"api.telegram.org/bot"+botToken, "SENTINEL_RATE_LIMIT_REQUEST_PATH_R6")
	canaries = append(canaries, telegramR6IdentifierCanaries(userID, successMessage)...)
	assertTelegramR6Redacted(t, rendered, canaries...)
}

func TestTelegramLinkSuccessfulConsumeKeepsCommitWhenRealBotDeleteFails(t *testing.T) {
	cfg := testConfig(t)
	cfg.SelfServiceOnboarding = true
	st, err := store.OpenWithOptions(cfg.DBPath, store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	owner, err := st.CreateSelfServiceOrbit("R6 delete failure")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := st.IssueTelegramLink(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}

	const (
		userID          int64 = 8_423_456_789
		messageID       int64 = 7_294_628_501
		botToken              = "SENTINEL_DELETE_BOT_TOKEN_R6"
		rawCause              = "SENTINEL_DELETE_RAW_CAUSE_R6"
		displayName           = "SENTINEL_DELETE_DISPLAY_NAME_R6"
		fileID                = "SENTINEL_DELETE_FILE_ID_R6"
		destinationPath       = "SENTINEL_DELETE_DESTINATION_R6"
	)
	api := newTelegramR6BotAPI(botToken, rawCause, false)
	harness := startTelegramR6BotHarness(t, cfg, st, api)
	harness.submit(userID, messageID, displayName, issued.Code)

	deleted := waitTelegramR6Outbound(t, api.deleted, "best-effort source deletion")
	if deleted.chatID != userID || deleted.messageID != messageID {
		t.Fatal("best-effort deletion did not target the verified private update")
	}
	reply := waitTelegramR6Outbound(t, api.sent, "successful link reply")
	if reply.chatID != userID || !strings.Contains(reply.text, "Telegram account linked") ||
		!strings.Contains(reply.text, "companion") || strings.Contains(reply.text, issued.Code) {
		t.Fatal("successful link response did not preserve the constant non-secret contract")
	}
	harness.logs.waitFor(t, "secret message deletion failed")
	harness.logs.waitFor(t, "send failed")

	linked, err := st.ResolveTelegramActorContext(userID)
	if err != nil || linked.OrbitID != owner.OrbitID || linked.Role != "companion" || linked.Capabilities != store.CapabilityTelegram {
		t.Fatal("identity commit did not survive best-effort Telegram transport failures")
	}
	rendered := harness.logs.String() + "|" + reply.text
	canaries := telegramR6IdentifierCanaries(userID, messageID,
		botToken, rawCause, displayName, issued.Code, owner.ControlToken,
		fileID, destinationPath, "api.telegram.org/bot"+botToken)
	assertTelegramR6Redacted(t, rendered, canaries...)
}

func TestTelegramLinkRealBotQueueSaturationDoesNotBlockCommittedConsume(t *testing.T) {
	cfg := testConfig(t)
	cfg.SelfServiceOnboarding = true
	st, err := store.OpenWithOptions(cfg.DBPath, store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	owner, err := st.CreateSelfServiceOrbit("R6 saturated outbox")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := st.IssueTelegramLink(owner.ActorID, owner.ControlToken, "satellite")
	if err != nil {
		t.Fatal(err)
	}

	const (
		userID      int64 = 8_523_456_789
		messageID   int64 = 7_394_628_501
		botToken          = "SENTINEL_SATURATION_BOT_TOKEN_R6"
		rawCause          = "SENTINEL_SATURATION_RAW_CAUSE_R6"
		displayName       = "SENTINEL_SATURATION_DISPLAY_NAME_R6"
		requestPath       = "SENTINEL_SATURATION_REQUEST_PATH_R6"
	)
	api := newTelegramR6BotAPI(botToken, rawCause, true)
	harness := startTelegramR6BotHarness(t, cfg, st, api)
	harness.bot.SendTo(1, "block sender")
	select {
	case <-api.sendStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("Telegram sender did not enter the deterministic saturation barrier")
	}
	for index := 0; index < 1024; index++ {
		harness.bot.SendTo(2, "bounded filler")
	}

	harness.submit(userID, messageID, displayName, issued.Code)
	harness.logs.waitFor(t, "outbox full, cannot delete secret message")
	harness.logs.waitFor(t, "outbox full, dropping message")
	linked, err := st.ResolveTelegramActorContext(userID)
	if err != nil || linked.OrbitID != owner.OrbitID || linked.Role != "satellite" || linked.Capabilities != store.CapabilityTelegram {
		t.Fatal("queue saturation blocked or rolled back the identity consume")
	}
	canaries := telegramR6IdentifierCanaries(userID, messageID,
		botToken, rawCause, displayName, issued.Code, owner.ControlToken,
		requestPath, "api.telegram.org/bot"+botToken)
	assertTelegramR6Redacted(t, harness.logs.String(), canaries...)
}

func TestTelegramLinkFeatureOffKeepsCodeShapedChatterSilent(t *testing.T) {
	cfg := testConfig(t)
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.BootstrapLegacyOrbit(
		map[string]string{"a": cfg.Nodes["a"].Token, "b": cfg.Nodes["b"].Token},
		cfg.Telegram.Users,
	); err != nil {
		t.Fatal(err)
	}
	l, _ := newTelegramIdentityLoop(t, cfg, st)
	inspect, err := sql.Open("sqlite", cfg.DBPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	if _, err := inspect.Exec(`UPDATE actors SET revoked_at = ?
WHERE kind = 'telegram_user' AND external_ref = ?`, time.Now().UnixMilli(), "111"); err != nil {
		t.Fatal(err)
	}
	member, err := l.telegramCommandMember(111)
	legacy, legacyErr := st.MemberOf(111)
	if err != nil || legacyErr != nil || member == nil || legacy == nil || member.OrbitID != legacy.OrbitID || member.Role != legacy.Role {
		t.Fatalf("feature-off actor model changed legacy authorization: member=%+v err=%v", member, err)
	}
	replies := &replies{}
	l.handleBot(telegramBotEvent(t, 111, "group", randomIdentityHumanSecret(t), replies))
	if len(replies.texts) != 0 {
		t.Fatalf("feature-off code-shaped chatter replies=%d", len(replies.texts))
	}
}
