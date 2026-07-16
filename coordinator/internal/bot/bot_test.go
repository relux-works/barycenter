package bot

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockAPI struct {
	sent     []string
	deleted  chan [2]int64
	answered chan [2]string
	cleared  chan [2]int64
}

func (m *mockAPI) GetUpdates(offset int64, timeoutS int) ([]Update, error) { return nil, nil }
func (m *mockAPI) SendMessage(chatID int64, text string) error {
	m.sent = append(m.sent, text)
	return nil
}
func (m *mockAPI) DeleteMessage(chatID, messageID int64) error {
	if m.deleted != nil {
		m.deleted <- [2]int64{chatID, messageID}
	}
	return nil
}
func (m *mockAPI) AnswerCallbackQuery(queryID, text string) error {
	if m.answered != nil {
		m.answered <- [2]string{queryID, text}
	}
	return nil
}
func (m *mockAPI) ClearInlineKeyboard(chatID, messageID int64) error {
	if m.cleared != nil {
		m.cleared <- [2]int64{chatID, messageID}
	}
	return nil
}
func (m *mockAPI) FileURL(fileID string) (string, error)   { return "http://x/" + fileID, nil }
func (m *mockAPI) Download(fileURL, destPath string) error { return nil }
func (m *mockAPI) GetMe() (string, error)                  { return "barycenter_bot", nil }

func newTestBot() (*Bot, *mockAPI) {
	api := &mockAPI{
		deleted: make(chan [2]int64, 1), answered: make(chan [2]string, 1),
		cleared: make(chan [2]int64, 1),
	}
	b := New(api, slog.Default())
	return b, api
}

func TestAudioAndDocumentProduceTypedHintOnlyEvents(t *testing.T) {
	b, _ := newTestBot()
	b.handleUpdate(Update{UpdateID: 10, Message: &Message{
		MessageID: 8, From: &User{ID: 111, FirstName: "Ivan"},
		Chat: Chat{ID: -100500, Type: "private"}, Caption: "всем rights",
		Audio: &Audio{FileID: "audio-file", FileUniqueID: "unique-audio",
			FileName: "declared.mp3", MIMEType: "audio/mpeg", Duration: 999,
			FileSize: 99 << 20},
	}})
	audio := <-b.Events
	if audio.Attachment == nil || audio.Attachment.Kind != AttachmentAudio ||
		audio.Attachment.TGFileID != "audio-file" || !audio.Attachment.Broadcast ||
		!audio.Attachment.RightsAcknowledged ||
		audio.Attachment.Duration != 999 || audio.Attachment.SizeBytes != 99<<20 {
		t.Fatalf("audio event=%+v attachment=%+v", audio, audio.Attachment)
	}

	b.handleUpdate(Update{UpdateID: 11, Message: &Message{
		MessageID: 9, From: &User{ID: 111, FirstName: "Ivan"},
		Chat: Chat{ID: -100500, Type: "private"}, MediaGroupID: "group-hint",
		Document: &Document{FileID: "document-file", FileUniqueID: "unique-document",
			FileName: "looks-like-audio.ogg", MIMEType: "application/octet-stream", FileSize: 123},
	}})
	document := <-b.Events
	if document.Attachment == nil || document.Attachment.Kind != AttachmentDocument ||
		document.Attachment.MediaGroupID != "group-hint" ||
		document.Attachment.MIMEType != "application/octet-stream" {
		t.Fatalf("document event=%+v attachment=%+v", document, document.Attachment)
	}
	// Transport preserved contradictory metadata instead of treating it as
	// proof. The common bounded ingest decides whether this is actually audio.
}

func TestNonAudioTelegramAttachmentGetsHonestUnsupportedReply(t *testing.T) {
	b, _ := newTestBot()
	b.handleUpdate(Update{UpdateID: 12, Message: &Message{
		MessageID: 10, From: &User{ID: 111, FirstName: "Ivan"},
		Chat:  Chat{ID: -100500, Type: "private"},
		Video: &UnsupportedAttachment{FileID: "private-video-file-id"},
	}})
	select {
	case event := <-b.Events:
		t.Fatalf("unsupported attachment produced an ingest event: %+v", event)
	default:
	}
	if len(b.outbox) != 1 {
		t.Fatalf("queued replies=%d", len(b.outbox))
	}
	reply := <-b.outbox
	if reply.text != AttachmentFailureText(AttachmentNotAudio) ||
		strings.Contains(reply.text, "private-video-file-id") {
		t.Fatalf("reply=%q", reply.text)
	}
}

func TestCallbackUpdateAnswersPromptlyAndClearsTerminalKeyboard(t *testing.T) {
	b, api := newTestBot()
	b.handleUpdate(Update{UpdateID: 12, CallbackQuery: &CallbackQuery{
		ID: "opaque-query-id", From: &User{ID: 111, FirstName: "Ivan"},
		Data:    "tg1_0123456789abcdefghijklmnopqrstuv",
		Message: &Message{MessageID: 44, Chat: Chat{ID: -100500, Type: "private"}},
	}})
	event := <-b.Events
	if event.Callback == nil || event.Callback.QueryID != "opaque-query-id" ||
		event.Callback.OriginalUpdateID != 12 || event.FromUserID != 111 ||
		event.ChatID != -100500 || event.MessageID != 44 {
		t.Fatalf("callback event=%+v callback=%+v", event, event.Callback)
	}
	stop := make(chan struct{})
	go b.callbackSender(stop)
	event.Callback.Answer(CallbackUnsupported)
	event.Callback.ClearKeyboard()
	select {
	case got := <-api.answered:
		if got != [2]string{"opaque-query-id", CallbackAnswerText(CallbackUnsupported)} {
			t.Fatalf("answer=%q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("callback query was not answered")
	}
	select {
	case got := <-api.cleared:
		if got != [2]int64{-100500, 44} {
			t.Fatalf("keyboard cleanup=%v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("terminal inline keyboard was not cleared")
	}
	close(stop)
}

func TestHTTPAPIInlinePromptUsesMessageBoundOpaqueKeyboard(t *testing.T) {
	type captured struct {
		method string
		form   url.Values
	}
	calls := make(chan captured, 4)
	api := &HTTPAPI{Token: "test-token", Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.ParseForm(); err != nil {
			t.Fatal(err)
		}
		method := strings.TrimPrefix(req.URL.Path, "/bottest-token/")
		calls <- captured{method: method, form: req.PostForm}
		body := `{"ok":true,"result":true}`
		if method == "sendMessage" {
			body = `{"ok":true,"result":{"message_id":77}}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header), Request: req,
		}, nil
	})}}
	messageID, err := api.SendMessageResult(-100500, "Выбери маршрут")
	if err != nil || messageID != 77 {
		t.Fatalf("message id=%d err=%v", messageID, err)
	}
	keyboard := InlineKeyboard{{{Text: "После текущего · Мой Барицентр",
		Data: "tg1_0123456789abcdefghijklmnopqrstuv"}}}
	if err := api.SetInlineKeyboard(-100500, messageID, keyboard); err != nil {
		t.Fatal(err)
	}
	if err := api.AnswerCallbackQuery("opaque-query", "Кнопка устарела"); err != nil {
		t.Fatal(err)
	}
	if err := api.ClearInlineKeyboard(-100500, 44); err != nil {
		t.Fatal(err)
	}
	send := <-calls
	if send.method != "sendMessage" || send.form.Get("chat_id") != "-100500" ||
		send.form.Get("text") != "Выбери маршрут" {
		t.Fatalf("send call=%+v", send)
	}
	set := <-calls
	if set.method != "editMessageReplyMarkup" || set.form.Get("message_id") != "77" ||
		!strings.Contains(set.form.Get("reply_markup"), `"callback_data":"tg1_`) ||
		strings.Contains(set.form.Get("reply_markup"), "actor_id") {
		t.Fatalf("set keyboard call=%+v", set)
	}
	answer := <-calls
	if answer.method != "answerCallbackQuery" || answer.form.Get("callback_query_id") != "opaque-query" ||
		answer.form.Get("text") != "Кнопка устарела" {
		t.Fatalf("answer call=%+v", answer)
	}
	clear := <-calls
	if clear.method != "editMessageReplyMarkup" || clear.form.Get("chat_id") != "-100500" ||
		clear.form.Get("message_id") != "44" || clear.form.Get("reply_markup") != `{"inline_keyboard":[]}` {
		t.Fatalf("clear call=%+v", clear)
	}
}

func TestAttachmentFailureVocabularyUsesCommonIngestProof(t *testing.T) {
	tests := []struct {
		kind   AttachmentKind
		ingest string
		want   AttachmentFailureCode
	}{
		{AttachmentAudio, "media_input_oversized", AttachmentTooLarge},
		{AttachmentDocument, "media_signature_unsupported", AttachmentNotAudio},
		{AttachmentAudio, "media_signature_unsupported", AttachmentDecodeFailed},
		{AttachmentAudio, "media_duration_exceeded", AttachmentTrackPhase2},
		{AttachmentAudio, "codec_profile_unavailable", AttachmentTrackPhase2},
		{AttachmentDocument, "media_input_unavailable", AttachmentDownloadFailed},
	}
	for _, test := range tests {
		if got := AttachmentFailureFromIngest(test.kind, test.ingest); got != test.want {
			t.Errorf("kind=%s ingest=%s got=%s want=%s", test.kind, test.ingest, got, test.want)
		}
		if text := AttachmentFailureText(test.want); text == "" || strings.Contains(text, "file-id") {
			t.Errorf("unsafe/empty text for %s: %q", test.want, text)
		}
	}
}

func msgFrom(userID int64, text string) Update {
	return Update{UpdateID: 1, Message: &Message{
		MessageID: 7, From: &User{ID: userID, FirstName: "U"}, Chat: Chat{ID: -100500, Type: "private"}, Text: text,
	}}
}

func randomTelegramTestCode(t *testing.T) string {
	t.Helper()
	const alphabet = "ABCDEFGHJKMNPQRSTVWXYZ23456789"
	code := make([]byte, 27)
	for i := range code {
		for {
			var candidate [1]byte
			if _, err := rand.Read(candidate[:]); err != nil {
				t.Fatal(err)
			}
			if candidate[0] < 240 {
				code[i] = alphabet[int(candidate[0])%len(alphabet)]
				break
			}
		}
	}
	return string(code)
}

// v2.1: the bot is a pure transport — every sender reaches the loop, which
// resolves membership. Strangers are the loop's business now.
func TestEverySenderProducesEvent(t *testing.T) {
	b, _ := newTestBot()
	b.handleUpdate(msgFrom(999, "/skip"))
	ev := <-b.Events
	if ev.FromUserID != 999 || ev.Command.Kind != KindSkip || ev.ChatID != -100500 {
		t.Fatalf("ev = %+v", ev)
	}
}

func TestUsernameFromGetMe(t *testing.T) {
	b, _ := newTestBot()
	if b.Username != "barycenter_bot" {
		t.Fatalf("username = %q", b.Username)
	}
}

func TestChatterProducesNothing(t *testing.T) {
	b, api := newTestBot()
	b.handleUpdate(msgFrom(111, "как дела?"))
	select {
	case ev := <-b.Events:
		t.Fatalf("chatter produced event %+v", ev)
	default:
	}
	if len(api.sent) != 0 {
		t.Fatalf("chatter got replies: %v", api.sent)
	}
}

func TestParseErrorRepliesInChat(t *testing.T) {
	b, _ := newTestBot()
	b.handleUpdate(msgFrom(111, "/vol 500"))
	// Replies are queued to the async outbox (the sender goroutine drains it
	// in Run); the parse-error reply must land there.
	if len(b.outbox) != 1 {
		t.Fatalf("want one queued reply, got %d", len(b.outbox))
	}
}

func TestVoiceFlags(t *testing.T) {
	b, _ := newTestBot()
	b.handleUpdate(Update{UpdateID: 2, Message: &Message{
		From: &User{ID: 111, FirstName: "Ivan"}, Chat: Chat{ID: -100500},
		Caption: "Лично",
		Voice:   &Voice{FileID: "f1", Duration: 12, FileSize: 30000},
	}})
	ev := <-b.Events
	if ev.Voice == nil || !ev.Voice.Personal || ev.Voice.Broadcast || ev.FromUserID != 111 || ev.Voice.TGFileID != "f1" {
		t.Fatalf("ev = %+v voice=%+v", ev, ev.Voice)
	}
	b.handleUpdate(Update{UpdateID: 3, Message: &Message{
		From: &User{ID: 111, FirstName: "Ivan"}, Chat: Chat{ID: -100500},
		Caption: "всем",
		Voice:   &Voice{FileID: "f2", Duration: 5, FileSize: 10000},
	}})
	ev = <-b.Events
	if ev.Voice == nil || ev.Voice.Personal || !ev.Voice.Broadcast {
		t.Fatalf("broadcast caption: %+v", ev.Voice)
	}
}

func TestTelegramLinkCodeCarriesVerifiedUpdateMetadata(t *testing.T) {
	code := randomTelegramTestCode(t)

	t.Run("private", func(t *testing.T) {
		b, api := newTestBot()
		b.handleUpdate(msgFrom(111, code))
		ev := <-b.Events
		if ev.Command.Kind != KindTelegramLink || ev.Command.Target != code || ev.ChatType != "private" || ev.MessageID != 7 {
			t.Fatalf("link event kind=%s chat=%q message=%d", ev.Command.Kind, ev.ChatType, ev.MessageID)
		}
		stop := make(chan struct{})
		go b.sender(stop)
		ev.DeleteSource()
		select {
		case got := <-api.deleted:
			if got != [2]int64{-100500, 7} {
				t.Fatalf("deleted=%v", got)
			}
		case <-time.After(time.Second):
			t.Fatal("secret-bearing message deletion was not attempted")
		}
		close(stop)
	})

	t.Run("group", func(t *testing.T) {
		b, _ := newTestBot()
		update := msgFrom(111, code)
		update.Message.Chat.Type = "group"
		b.handleUpdate(update)
		ev := <-b.Events
		if ev.Command.Kind != KindTelegramLink || ev.ChatType != "group" {
			t.Fatalf("group metadata event kind=%s chat=%q", ev.Command.Kind, ev.ChatType)
		}
		if len(b.outbox) != 0 {
			t.Fatalf("transport must not bypass feature-gated consume: replies=%d", len(b.outbox))
		}
	})
}

func TestParseTelegramLinkCodeNormalizationShape(t *testing.T) {
	code := randomTelegramTestCode(t)
	formatted := strings.ToLower(code[:9] + "-" + code[9:18] + " " + code[18:])
	command, err := Parse(formatted)
	if err != nil || command.Kind != KindTelegramLink || command.Target != formatted {
		t.Fatalf("formatted link command kind=%s err=%v", command.Kind, err)
	}
	invalid, err := Parse(code[:26] + "O")
	if err != nil || invalid.Kind != KindIgnore {
		t.Fatalf("excluded alphabet character kind=%s err=%v", invalid.Kind, err)
	}
}

func (m *mockAPI) SetMyCommands(string) error { return nil }

var errInjectedTelegramTransport = errors.New("injected telegram transport cause")

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type telegramLogCapture struct {
	mu      sync.Mutex
	records []string
	handled chan struct{}
}

func (h *telegramLogCapture) Enabled(context.Context, slog.Level) bool { return true }

func (h *telegramLogCapture) Handle(_ context.Context, record slog.Record) error {
	var line strings.Builder
	line.WriteString(record.Message)
	record.Attrs(func(attr slog.Attr) bool {
		line.WriteByte(' ')
		line.WriteString(attr.Key)
		line.WriteByte('=')
		line.WriteString(fmt.Sprint(attr.Value.Any()))
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, line.String())
	h.mu.Unlock()
	if h.handled != nil {
		select {
		case h.handled <- struct{}{}:
		default:
		}
	}
	return nil
}

func (h *telegramLogCapture) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *telegramLogCapture) WithGroup(string) slog.Handler      { return h }

func (h *telegramLogCapture) String() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return strings.Join(h.records, "\n")
}

func leakingTelegramClient() *http.Client {
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body string
		if req.Body != nil {
			raw, _ := io.ReadAll(req.Body)
			body = string(raw)
		}
		return nil, fmt.Errorf("request_url=%s request_body=%s: %w", req.URL.String(), body, errInjectedTelegramTransport)
	})}
}

func assertTelegramSecretsRedacted(t *testing.T, rendered string, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(rendered, secret) {
			t.Fatal("secret-bearing Telegram material leaked")
		}
	}
}

func telegramErrorGraph(err error) []error {
	var graph []error
	queue := []error{err}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == nil {
			continue
		}
		graph = append(graph, current)
		switch unwrapped := current.(type) {
		case interface{ Unwrap() []error }:
			queue = append(queue, unwrapped.Unwrap()...)
		case interface{ Unwrap() error }:
			queue = append(queue, unwrapped.Unwrap())
		}
	}
	return graph
}

func assertTelegramErrorGraphRedacted(t *testing.T, err error, secrets ...string) {
	t.Helper()
	for _, reachable := range telegramErrorGraph(err) {
		assertTelegramSecretsRedacted(t, reachable.Error(), secrets...)
	}
}

func telegramIdentifierCanaries(chatID, messageID int64, extras ...string) []string {
	chat := strconv.FormatInt(chatID, 10)
	message := strconv.FormatInt(messageID, 10)
	return append(extras,
		chat,
		message,
		"chat="+chat,
		"message="+message,
		"chat_id="+chat,
		"message_id="+message,
		`"chat":`+chat,
		`"message":`+message,
		`"chat_id":`+chat,
		`"message_id":`+message,
	)
}

type privacyFailingAPI struct {
	sendErr   error
	deleteErr error
}

func (a *privacyFailingAPI) GetUpdates(int64, int) ([]Update, error) { return nil, nil }
func (a *privacyFailingAPI) SendMessage(int64, string) error         { return a.sendErr }
func (a *privacyFailingAPI) DeleteMessage(int64, int64) error        { return a.deleteErr }
func (a *privacyFailingAPI) FileURL(string) (string, error)          { return "", nil }
func (a *privacyFailingAPI) Download(string, string) error           { return nil }
func (a *privacyFailingAPI) GetMe() (string, error)                  { return "barycenter_bot", nil }
func (a *privacyFailingAPI) SetMyCommands(string) error              { return nil }

func TestOutboxSenderFailureLogsRedactUpdateIdentifiersAndErrorGraph(t *testing.T) {
	const (
		chatID          int64 = 6_712_345_678_901_234
		messageID       int64 = 5_987_654_321_098_765
		botToken              = "SENTINEL_BOT_TOKEN_R6"
		requestBody           = "SENTINEL_REQUEST_BODY_R6"
		fileID                = "SENTINEL_FILE_ID_R6"
		destinationPath       = "SENTINEL_DESTINATION_PATH_R6"
		rawCause              = "SENTINEL_RAW_CAUSE_R6"
	)
	linkCode := randomTelegramTestCode(t)
	raw := fmt.Errorf("%s url=https://api.telegram.org/bot%s/sendMessage?chat_id=%d body=%s&text=%s file_id=%s destination=%s message_id=%d: %w",
		rawCause, botToken, chatID, requestBody, linkCode, fileID, destinationPath, messageID, errInjectedTelegramTransport)
	canaries := telegramIdentifierCanaries(chatID, messageID,
		botToken, requestBody, fileID, destinationPath, rawCause, linkCode,
		"api.telegram.org/bot"+botToken, url.QueryEscape(linkCode))

	for _, test := range []struct {
		name      string
		message   string
		operation string
		out       outMsg
		api       *privacyFailingAPI
	}{
		{
			name:      "reply send failure",
			message:   "send failed",
			operation: "sendMessage",
			out:       outMsg{chatID: chatID, text: linkCode},
			api:       &privacyFailingAPI{sendErr: raw},
		},
		{
			name:      "secret delete failure",
			message:   "secret message deletion failed",
			operation: "deleteMessage",
			out:       outMsg{chatID: chatID, messageID: messageID, delete: true},
			api:       &privacyFailingAPI{deleteErr: raw},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			safe := safeTelegramLogError(test.operation, raw)
			if errors.Is(safe, errInjectedTelegramTransport) {
				t.Fatal("raw Telegram API cause remained reachable from the safe error graph")
			}
			assertTelegramErrorGraphRedacted(t, safe, canaries...)
			handler := &telegramLogCapture{handled: make(chan struct{}, 1)}
			b := &Bot{
				api:    test.api,
				log:    slog.New(handler),
				outbox: make(chan outMsg, 1),
			}
			stop := make(chan struct{})
			go b.sender(stop)
			b.outbox <- test.out
			select {
			case <-handler.handled:
			case <-time.After(time.Second):
				close(stop)
				t.Fatal("outbox failure was not logged")
			}
			close(stop)

			logged := handler.String()
			if !strings.Contains(logged, test.message) || !strings.Contains(logged, "operation="+test.operation) {
				t.Fatal("outbox failure log omitted its constant operation fields")
			}
			assertTelegramSecretsRedacted(t, logged, canaries...)
		})
	}
}

func TestOutboxOverflowLogsRedactPrivateUpdateIdentifiersAndPayload(t *testing.T) {
	const (
		chatID          int64 = 6_712_345_678_901_234
		messageID       int64 = 5_987_654_321_098_765
		botToken              = "SENTINEL_OVERFLOW_TOKEN_R6"
		requestBody           = "SENTINEL_OVERFLOW_BODY_R6"
		fileID                = "SENTINEL_OVERFLOW_FILE_R6"
		destinationPath       = "SENTINEL_OVERFLOW_PATH_R6"
	)
	linkCode := randomTelegramTestCode(t)
	canaries := telegramIdentifierCanaries(chatID, messageID,
		botToken, requestBody, fileID, destinationPath, linkCode,
		"api.telegram.org/bot"+botToken, url.QueryEscape(linkCode))

	t.Run("SendTo queue overflow", func(t *testing.T) {
		handler := &telegramLogCapture{}
		b := &Bot{log: slog.New(handler), outbox: make(chan outMsg, 1)}
		b.outbox <- outMsg{chatID: 1, text: "filler"}
		b.SendTo(chatID, requestBody+linkCode)

		logged := handler.String()
		if !strings.Contains(logged, "outbox full, dropping message") || !strings.Contains(logged, "operation=sendMessage") {
			t.Fatal("send overflow log omitted its constant operation fields")
		}
		assertTelegramSecretsRedacted(t, logged, canaries...)
	})

	t.Run("secret-delete queue overflow", func(t *testing.T) {
		handler := &telegramLogCapture{}
		b := &Bot{
			log:    slog.New(handler),
			Events: make(chan Event, 1),
			outbox: make(chan outMsg, 1),
		}
		b.handleUpdate(Update{UpdateID: 1, Message: &Message{
			MessageID: messageID,
			From:      &User{ID: chatID, FirstName: requestBody},
			Chat:      Chat{ID: chatID, Type: "private"},
			Text:      linkCode,
		}})
		event := <-b.Events
		b.outbox <- outMsg{chatID: 1, text: "filler"}
		event.DeleteSource()

		logged := handler.String()
		if !strings.Contains(logged, "outbox full, cannot delete secret message") || !strings.Contains(logged, "operation=deleteMessage") {
			t.Fatal("delete overflow log omitted its constant operation fields")
		}
		assertTelegramSecretsRedacted(t, logged, canaries...)
	})
}

func TestHTTPAPITransportErrorsRedactTokenURLsAndRequestMaterial(t *testing.T) {
	const (
		token       = "SENTINEL_BOT_TOKEN_R1"
		linkCode    = "SENTINEL_LINK_CODE_R1"
		messageText = "consume " + linkCode
		fileID      = "SENTINEL_FILE_ID_R1"
		callbackID  = "SENTINEL_CALLBACK_QUERY_R1"
	)
	api := &HTTPAPI{Token: token, Client: leakingTelegramClient()}
	fileURL := "https://api.telegram.org/file/bot" + token + "/voice/" + linkCode
	handler := &telegramLogCapture{}
	logger := slog.New(handler)

	operations := []struct {
		name string
		call func() error
	}{
		{name: "getUpdates", call: func() error {
			_, err := api.GetUpdates(17, 50)
			return err
		}},
		{name: "sendMessage", call: func() error { return api.SendMessage(42, messageText) }},
		{name: "deleteMessage", call: func() error { return api.DeleteMessage(42, 99) }},
		{name: "answerCallbackQuery", call: func() error {
			return api.AnswerCallbackQuery(callbackID, CallbackAnswerText(CallbackExpired))
		}},
		{name: "editMessageReplyMarkup", call: func() error { return api.ClearInlineKeyboard(42, 99) }},
		{name: "getFile", call: func() error {
			_, err := api.FileURL(fileID)
			return err
		}},
		{name: "download", call: func() error {
			return api.Download(fileURL, t.TempDir()+"/voice.ogg")
		}},
	}
	secretFragments := []string{
		token,
		linkCode,
		messageText,
		fileID,
		callbackID,
		"api.telegram.org/bot" + token,
		"file/bot" + token,
		fileURL,
		url.QueryEscape(messageText),
		"chat_id=42",
		"file_id=" + fileID,
		"callback_query_id=" + callbackID,
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			err := operation.call()
			hasSafeCause := errors.Is(err, errTelegramOperationFailure) ||
				errors.Is(err, errTelegramNetworkFailure) ||
				errors.Is(err, errTelegramNetworkTimeout) ||
				errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded)
			if err == nil || errors.Is(err, errInjectedTelegramTransport) || !hasSafeCause {
				t.Fatalf("error=%v raw_cause=%v safe_cause=%v", err, errors.Is(err, errInjectedTelegramTransport), hasSafeCause)
			}
			if !strings.Contains(err.Error(), operation.name) {
				t.Fatalf("operation missing from error %q", err)
			}
			assertTelegramErrorGraphRedacted(t, err, secretFragments...)
			logger.Error("captured transport failure", "err", err)
			assertTelegramSecretsRedacted(t, handler.String(), secretFragments...)
		})
	}
}

func TestHTTPAPIRedirectsFailClosedWithoutReachingTarget(t *testing.T) {
	const token = "SENTINEL_REDIRECT_TOKEN_R1"
	const linkCode = "SENTINEL_REDIRECT_LINK_R1"

	for _, operation := range []string{"sendMessage", "download"} {
		t.Run(operation, func(t *testing.T) {
			targetRequests := make(chan *http.Request, 1)
			target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
				targetRequests <- req.Clone(context.Background())
			}))
			defer target.Close()

			sourceReferer := make(chan string, 1)
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				sourceReferer <- req.Referer()
				http.Redirect(w, req, target.URL+"/capture", http.StatusFound)
			}))
			defer source.Close()
			sourceURL, err := url.Parse(source.URL)
			if err != nil {
				t.Fatal(err)
			}

			transportCalls := 0
			transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				transportCalls++
				if req.URL.Host == "api.telegram.org" {
					clone := req.Clone(req.Context())
					urlCopy := *req.URL
					urlCopy.Scheme = sourceURL.Scheme
					urlCopy.Host = sourceURL.Host
					clone.URL = &urlCopy
					return http.DefaultTransport.RoundTrip(clone)
				}
				return http.DefaultTransport.RoundTrip(req)
			})
			const injectedTimeout = 9 * time.Second
			api := &HTTPAPI{Token: token, Client: &http.Client{Transport: transport, Timeout: injectedTimeout}}
			var callErr error
			if operation == "sendMessage" {
				callErr = api.SendMessage(42, "link "+linkCode)
			} else {
				callErr = api.Download(source.URL+"/file/bot"+token+"/"+linkCode, t.TempDir()+"/voice.ogg")
			}
			if callErr == nil || !strings.Contains(callErr.Error(), operation) || !strings.Contains(callErr.Error(), "redirect rejected") {
				t.Fatalf("redirect error=%v", callErr)
			}
			if api.Client.Timeout != injectedTimeout {
				t.Fatal("injected HTTP client timeout was replaced")
			}
			if transportCalls != 1 {
				t.Fatalf("transport calls=%d, redirect was followed", transportCalls)
			}
			select {
			case req := <-targetRequests:
				t.Fatalf("redirect target received request url=%q referer=%q", req.URL.String(), req.Referer())
			default:
			}
			select {
			case referer := <-sourceReferer:
				if referer != "" {
					t.Fatalf("source request unexpectedly had Referer %q", referer)
				}
			default:
				t.Fatal("redirect source was not reached")
			}
			assertTelegramErrorGraphRedacted(t, callErr,
				token,
				linkCode,
				"api.telegram.org/bot"+token,
				"file/bot"+token,
			)
		})
	}
}

func TestHTTPAPIFilesystemErrorGraphDoesNotExposeDestinationPath(t *testing.T) {
	const destinationSecret = "SENTINEL_DESTINATION_PATH_R1"
	api := &HTTPAPI{
		Token: "SENTINEL_DEST_TOKEN_R1",
		Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("audio")),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})},
	}
	destination := t.TempDir() + "/" + destinationSecret + "/voice.ogg"
	err := api.Download("https://example.invalid/audio", destination)
	if err == nil || !errors.Is(err, errTelegramOperationFailure) {
		t.Fatalf("error=%v", err)
	}
	assertTelegramErrorGraphRedacted(t, err, destination, destinationSecret)
}

func TestHTTPAPIDownloadBoundedStopsAtLimitPlusOneAndSecuresPartial(t *testing.T) {
	const (
		secretURL = "https://example.invalid/SENTINEL_BOUNDED_DOWNLOAD"
		limit     = int64(16)
	)
	api := &HTTPAPI{
		Token: "bounded-token",
		Client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", 1024))),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		})},
	}
	destination := t.TempDir() + "/bounded.voice"
	written, err := api.DownloadBounded(secretURL, destination, limit)
	if err == nil || written != limit+1 {
		t.Fatalf("bounded download written=%d err=%v", written, err)
	}
	assertTelegramErrorGraphRedacted(t, err, secretURL, destination)
	info, statErr := os.Stat(destination)
	if statErr != nil || info.Size() != limit+1 || info.Mode().Perm() != 0o600 {
		t.Fatalf("bounded partial info=%+v err=%v", info, statErr)
	}
}

func TestHTTPAPIRejectedResponseDoesNotEchoTelegramDescription(t *testing.T) {
	const token = "SENTINEL_REJECTED_TOKEN_R1"
	const secretText = "SENTINEL_REJECTED_MESSAGE_R1"
	api := &HTTPAPI{
		Token: token,
		Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			body := `{"ok":false,"description":"` + token + ` ` + secretText + `"}`
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		})},
	}
	err := api.SendMessage(42, secretText)
	if err == nil || !strings.Contains(err.Error(), "sendMessage") {
		t.Fatalf("error=%v", err)
	}
	assertTelegramSecretsRedacted(t, err.Error(), token, secretText)
}

func TestBestEffortDeleteFailureLogUsesSanitizedHTTPAdapterError(t *testing.T) {
	const token = "SENTINEL_DELETE_TOKEN_R1"
	const linkCode = "SENTINEL_DELETE_LINK_R1"
	api := &HTTPAPI{Token: token, Client: leakingTelegramClient()}
	handler := &telegramLogCapture{handled: make(chan struct{}, 1)}
	b := &Bot{
		api:    api,
		log:    slog.New(handler),
		outbox: make(chan outMsg, 1),
	}
	stop := make(chan struct{})
	go b.sender(stop)
	b.outbox <- outMsg{chatID: 42, messageID: 99, text: linkCode, delete: true}
	select {
	case <-handler.handled:
	case <-time.After(time.Second):
		close(stop)
		t.Fatal("delete failure was not logged")
	}
	close(stop)
	logged := handler.String()
	if !strings.Contains(logged, "secret message deletion failed") || !strings.Contains(logged, "deleteMessage") {
		t.Fatal("delete failure log omitted its constant operation fields")
	}
	assertTelegramSecretsRedacted(t, logged,
		token,
		linkCode,
		"api.telegram.org/bot"+token,
		"chat_id=42",
		"message_id=99",
	)
}
