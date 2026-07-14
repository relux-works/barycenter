package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

// Event is what the session loop receives from the bot. Membership and
// roles are resolved by the loop against the store (v2.1 multi-tenant);
// the bot is a pure transport + parser.
type Event struct {
	ChatID     int64
	ChatType   string
	MessageID  int64
	FromUserID int64
	FromName   string
	Command    Command // for text messages
	Voice      *VoiceEvent
	Attachment *AttachmentEvent
	Callback   *CallbackEvent
	// Reply sends a response into the chat the message came from.
	Reply func(text string)
	// DeleteSource best-effort removes a secret-bearing source message. It is
	// asynchronous and never changes the result of a committed consume.
	DeleteSource func()
}

// AttachmentKind is the transport-observed Telegram update shape. It is not a
// media classification: MIME type, filename, duration and size remain
// untrusted hints until the common ingest service inspects bounded bytes.
type AttachmentKind string

const (
	AttachmentAudio    AttachmentKind = "audio"
	AttachmentDocument AttachmentKind = "document"
)

type AttachmentEvent struct {
	Kind             AttachmentKind
	TGFileID         string
	OriginalUpdateID int64
	FileName         string
	MIMEType         string
	Duration         int
	SizeBytes        int64
	MediaGroupID     string
	Personal         bool
	Broadcast        bool
}

// CallbackEvent is a transport event only. The coordinator must resolve the
// clicking Telegram user into a fresh ActorContext before validating Data.
// Answer and ClearKeyboard are asynchronous and safe to call from the loop.
type CallbackEvent struct {
	QueryID          string
	Data             string
	OriginalUpdateID int64
	Answer           func(CallbackAnswerCode)
	ClearKeyboard    func()
}

type VoiceEvent struct {
	TGFileID         string
	OriginalUpdateID int64
	Duration         int // seconds, from Telegram metadata
	SizeBytes        int64
	Personal         bool // caption "лично" (spec 9.1)
	Broadcast        bool // caption "всем" — forces broadcast over the orbit default
}

type InlineButton struct {
	Text string `json:"text"`
	Data string `json:"callback_data"`
}

type InlineKeyboard [][]InlineButton

type InlineKeyboardBuilder func(messageID int64) (InlineKeyboard, error)

// API abstracts the Telegram Bot HTTP API for tests.
type API interface {
	GetUpdates(offset int64, timeoutS int) ([]Update, error)
	SendMessage(chatID int64, text string) error
	DeleteMessage(chatID, messageID int64) error
	FileURL(fileID string) (string, error)
	Download(fileURL, destPath string) error
	GetMe() (username string, err error)
	SetMyCommands(commandsJSON string) error
}

// Update mirrors the subset of Telegram's Update we need.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

type Message struct {
	MessageID    int64                  `json:"message_id"`
	From         *User                  `json:"from"`
	Chat         Chat                   `json:"chat"`
	Text         string                 `json:"text"`
	Caption      string                 `json:"caption"`
	Voice        *Voice                 `json:"voice"`
	Audio        *Audio                 `json:"audio"`
	Document     *Document              `json:"document"`
	Video        *UnsupportedAttachment `json:"video"`
	Animation    *UnsupportedAttachment `json:"animation"`
	Sticker      *UnsupportedAttachment `json:"sticker"`
	MediaGroupID string                 `json:"media_group_id"`
}

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type Voice struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
	FileSize int64  `json:"file_size"`
}

type Audio struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileName     string `json:"file_name"`
	MIMEType     string `json:"mime_type"`
	Duration     int    `json:"duration"`
	FileSize     int64  `json:"file_size"`
}

type Document struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	FileName     string `json:"file_name"`
	MIMEType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
}

type UnsupportedAttachment struct {
	FileID string `json:"file_id"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    *User    `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

type Bot struct {
	api API
	log *slog.Logger
	// Username of the bot account (@…), used to build invite deep links.
	Username string
	Events   chan Event
	// outbox is the asynchronous send queue: replies and orbit notifications
	// are enqueued here and delivered by a single dedicated goroutine (sender)
	// so the FSM loop never blocks on a slow Telegram POST (bugs #2 /
	// architecture #1.1). One drainer preserves per-chat message order.
	outbox chan outMsg
	// callbackOutbox is deliberately separate: a slow ordinary send must not
	// leave Telegram's callback spinner waiting behind the chat backlog.
	callbackOutbox chan outMsg
}

type outMsg struct {
	chatID        int64
	messageID     int64
	text          string
	delete        bool
	callbackID    string
	callbackText  string
	clearKeyboard bool
	inlineBuilder InlineKeyboardBuilder
}

func New(api API, log *slog.Logger) *Bot {
	b := &Bot{
		api:            api,
		log:            log,
		Events:         make(chan Event, 16),
		outbox:         make(chan outMsg, 1024),
		callbackOutbox: make(chan outMsg, 128),
	}
	if u, err := api.GetMe(); err == nil {
		b.Username = u
	} else {
		log.Warn("getMe failed, invite links will use the default username", "err", err)
	}
	if err := api.SetMyCommands(commandMenuJSON); err != nil {
		log.Warn("setMyCommands failed", "err", err)
	}
	return b
}

// commandMenuJSON is the Telegram client command menu (the "/" button). Only
// the everyday surface lives here (bot-ux redesign §3b): the rest stay typeable
// but off the menu so a non-technical user is not buried under setup/calibration
// /admin commands. Everyday first, then the one-time setup, then help.
const commandMenuJSON = `[
{"command":"now","description":"что сейчас играет"},
{"command":"skip","description":"следующий трек"},
{"command":"pause","description":"пауза"},
{"command":"resume","description":"продолжить"},
{"command":"queue","description":"очередь"},
{"command":"vol","description":"громкость (0–100)"},
{"command":"home","description":"кто на связи и что настроено"},
{"command":"pair","description":"подключить свой дом"},
{"command":"share","description":"пригласить партнёра"},
{"command":"help","description":"помощь"}
]`

// SendTo enqueues a message to any chat (orbit notifications go to each member's
// DM). Delivery is asynchronous — the caller (the FSM loop) never blocks on
// Telegram HTTP. The queue is bounded; an overflow (sustained Telegram outage)
// drops with a warning rather than stalling the loop.
func (b *Bot) SendTo(chatID int64, text string) {
	select {
	case b.outbox <- outMsg{chatID: chatID, text: text}:
	default:
		b.log.Warn("outbox full, dropping message", "operation", "sendMessage")
	}
}

// SendInlinePrompt sends the human prompt first, then mints and attaches
// message-bound opaque controls. The two-step flow is deliberately off the
// coordinator loop and never delays the already-committed default delivery.
func (b *Bot) SendInlinePrompt(chatID int64, text string, builder InlineKeyboardBuilder) {
	select {
	case b.outbox <- outMsg{chatID: chatID, text: text, inlineBuilder: builder}:
	default:
		b.log.Warn("outbox full, dropping inline prompt", "operation", "sendMessage")
	}
}

// sender is the single goroutine that drains the outbox and performs the
// blocking Telegram sends off the FSM loop's critical path.
func (b *Bot) sender(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case m := <-b.outbox:
			if m.delete {
				if err := b.api.DeleteMessage(m.chatID, m.messageID); err != nil {
					b.log.Debug("secret message deletion failed", "operation", "deleteMessage", "err", safeTelegramLogError("deleteMessage", err))
				}
				continue
			}
			if m.inlineBuilder != nil {
				api, ok := b.api.(interface {
					SendMessageResult(int64, string) (int64, error)
					SetInlineKeyboard(int64, int64, InlineKeyboard) error
				})
				if !ok {
					b.log.Warn("inline API unavailable", "operation", "sendMessage")
					continue
				}
				messageID, err := api.SendMessageResult(m.chatID, m.text)
				if err != nil {
					b.log.Warn("inline prompt send failed", "operation", "sendMessage", "err", safeTelegramLogError("sendMessage", err))
					continue
				}
				keyboard, err := m.inlineBuilder(messageID)
				if err != nil {
					b.log.Warn("inline keyboard build failed", "operation", "editMessageReplyMarkup")
					continue
				}
				if err := api.SetInlineKeyboard(m.chatID, messageID, keyboard); err != nil {
					b.log.Warn("inline keyboard send failed", "operation", "editMessageReplyMarkup", "err", safeTelegramLogError("editMessageReplyMarkup", err))
				}
				continue
			}
			if err := b.api.SendMessage(m.chatID, m.text); err != nil {
				b.log.Warn("send failed", "operation", "sendMessage", "err", safeTelegramLogError("sendMessage", err))
			}
		}
	}
}

func (b *Bot) callbackSender(stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case m := <-b.callbackOutbox:
			if m.callbackID != "" {
				api, ok := b.api.(interface {
					AnswerCallbackQuery(string, string) error
				})
				if !ok {
					b.log.Warn("callback API unavailable", "operation", "answerCallbackQuery")
					continue
				}
				if err := api.AnswerCallbackQuery(m.callbackID, m.callbackText); err != nil {
					b.log.Warn("callback answer failed", "operation", "answerCallbackQuery", "err", safeTelegramLogError("answerCallbackQuery", err))
				}
				continue
			}
			if m.clearKeyboard {
				api, ok := b.api.(interface {
					ClearInlineKeyboard(int64, int64) error
				})
				if !ok {
					b.log.Warn("callback API unavailable", "operation", "editMessageReplyMarkup")
					continue
				}
				if err := api.ClearInlineKeyboard(m.chatID, m.messageID); err != nil {
					b.log.Warn("keyboard cleanup failed", "operation", "editMessageReplyMarkup", "err", safeTelegramLogError("editMessageReplyMarkup", err))
				}
			}
		}
	}
}

// Run long-polls until stop closes (spec 3.3: outbound connection only).
func (b *Bot) Run(stop <-chan struct{}) {
	go b.sender(stop) // drain the outbox for the lifetime of the bot
	go b.callbackSender(stop)
	var offset int64
	for {
		select {
		case <-stop:
			return
		default:
		}
		updates, err := b.api.GetUpdates(offset, 50)
		if err != nil {
			b.log.Warn("getUpdates failed", "err", err)
			time.Sleep(3 * time.Second)
			continue
		}
		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			b.handleUpdate(u)
		}
	}
}

func (b *Bot) handleUpdate(u Update) {
	if query := u.CallbackQuery; query != nil {
		b.handleCallbackUpdate(u.UpdateID, query)
		return
	}
	msg := u.Message
	if msg == nil || msg.From == nil {
		return
	}
	chatID := msg.Chat.ID
	// Replies go through the async outbox (SendTo) so the FSM loop, which
	// invokes this closure, never blocks on a Telegram POST (bugs #2).
	reply := func(text string) { b.SendTo(chatID, text) }
	deleteSource := func() {
		select {
		case b.outbox <- outMsg{chatID: chatID, messageID: msg.MessageID, delete: true}:
		default:
			b.log.Warn("outbox full, cannot delete secret message", "operation", "deleteMessage")
		}
	}

	if msg.Voice != nil {
		b.Events <- Event{
			ChatID:     chatID,
			ChatType:   msg.Chat.Type,
			MessageID:  msg.MessageID,
			FromUserID: msg.From.ID,
			FromName:   msg.From.FirstName,
			Voice: &VoiceEvent{
				TGFileID: msg.Voice.FileID, OriginalUpdateID: u.UpdateID,
				Duration: msg.Voice.Duration, SizeBytes: msg.Voice.FileSize,
				Personal: IsPersonalCaption(msg.Caption), Broadcast: IsBroadcastCaption(msg.Caption),
			},
			Reply: reply,
		}
		return
	}
	if msg.Audio != nil {
		b.emitAttachment(u.UpdateID, msg, AttachmentEvent{
			Kind: AttachmentAudio, TGFileID: msg.Audio.FileID,
			FileName: msg.Audio.FileName, MIMEType: msg.Audio.MIMEType, Duration: msg.Audio.Duration,
			SizeBytes: msg.Audio.FileSize, MediaGroupID: msg.MediaGroupID,
			Personal: IsPersonalCaption(msg.Caption), Broadcast: IsBroadcastCaption(msg.Caption),
		})
		return
	}
	if msg.Document != nil {
		b.emitAttachment(u.UpdateID, msg, AttachmentEvent{
			Kind: AttachmentDocument, TGFileID: msg.Document.FileID,
			FileName: msg.Document.FileName, MIMEType: msg.Document.MIMEType, SizeBytes: msg.Document.FileSize,
			MediaGroupID: msg.MediaGroupID, Personal: IsPersonalCaption(msg.Caption),
			Broadcast: IsBroadcastCaption(msg.Caption),
		})
		return
	}
	if msg.Video != nil || msg.Animation != nil || msg.Sticker != nil {
		reply(AttachmentFailureText(AttachmentNotAudio))
		return
	}

	cmd, err := Parse(msg.Text)
	if err != nil {
		if r, ok := err.(ErrReply); ok {
			reply(r.Text)
		}
		return
	}
	if cmd.Kind == KindIgnore {
		return
	}
	b.Events <- Event{
		ChatID:       chatID,
		ChatType:     msg.Chat.Type,
		MessageID:    msg.MessageID,
		FromUserID:   msg.From.ID,
		FromName:     msg.From.FirstName,
		Command:      cmd,
		Reply:        reply,
		DeleteSource: deleteSource,
	}
}

func (b *Bot) emitAttachment(updateID int64, msg *Message, attachment AttachmentEvent) {
	chatID := msg.Chat.ID
	attachment.OriginalUpdateID = updateID
	b.Events <- Event{
		ChatID: chatID, ChatType: msg.Chat.Type, MessageID: msg.MessageID,
		FromUserID: msg.From.ID, FromName: msg.From.FirstName,
		Attachment: &attachment,
		Reply:      func(text string) { b.SendTo(chatID, text) },
	}
}

func (b *Bot) handleCallbackUpdate(updateID int64, query *CallbackQuery) {
	if query.From == nil || query.Message == nil || query.ID == "" {
		return
	}
	chatID := query.Message.Chat.ID
	messageID := query.Message.MessageID
	answer := func(code CallbackAnswerCode) {
		text := CallbackAnswerText(code)
		select {
		case b.callbackOutbox <- outMsg{callbackID: query.ID, callbackText: text}:
		default:
			b.log.Warn("outbox full, cannot answer callback", "operation", "answerCallbackQuery")
		}
	}
	clearKeyboard := func() {
		select {
		case b.callbackOutbox <- outMsg{chatID: chatID, messageID: messageID, clearKeyboard: true}:
		default:
			b.log.Warn("outbox full, cannot clear keyboard", "operation", "editMessageReplyMarkup")
		}
	}
	b.Events <- Event{
		ChatID: chatID, ChatType: query.Message.Chat.Type, MessageID: messageID,
		FromUserID: query.From.ID, FromName: query.From.FirstName,
		Callback: &CallbackEvent{QueryID: query.ID, Data: query.Data,
			OriginalUpdateID: updateID, Answer: answer, ClearKeyboard: clearKeyboard},
		Reply: func(text string) { b.SendTo(chatID, text) },
	}
}

// DownloadVoice fetches the raw voice file (ogg/opus) to destPath.
func (b *Bot) DownloadVoice(fileID, destPath string) error {
	u, err := b.api.FileURL(fileID)
	if err != nil {
		return err
	}
	return b.api.Download(u, destPath)
}

// DownloadVoiceBounded is the production Telegram-ingest path. HTTPAPI stops
// reading after maxBytes+1, so untrusted Telegram metadata cannot turn the
// common SubmitMedia validation boundary into an unbounded disk write. The
// fallback keeps injected/test API implementations source-compatible and is
// still rejected by the adapter after the actual file size is inspected.
func (b *Bot) DownloadVoiceBounded(fileID, destPath string, maxBytes int64) (int64, error) {
	if maxBytes <= 0 {
		return 0, newTelegramOperationError("download", "invalid size limit", errTelegramOperationFailure)
	}
	u, err := b.api.FileURL(fileID)
	if err != nil {
		return 0, err
	}
	if bounded, ok := b.api.(interface {
		DownloadBounded(string, string, int64) (int64, error)
	}); ok {
		return bounded.DownloadBounded(u, destPath, maxBytes)
	}
	downloadErr := b.api.Download(u, destPath)
	var size int64
	info, inspectErr := os.Lstat(destPath)
	if inspectErr == nil && info.Mode().IsRegular() {
		size = info.Size()
	}
	if downloadErr != nil {
		return size, downloadErr
	}
	if inspectErr != nil || !info.Mode().IsRegular() {
		return size, newTelegramOperationError("download", "destination inspect failed", inspectErr)
	}
	if size > maxBytes {
		return size, newTelegramOperationError("download", "response exceeds size limit", errTelegramOperationFailure)
	}
	return size, nil
}

// --- Real Telegram HTTP API ---

type HTTPAPI struct {
	Token  string
	Client *http.Client
}

func NewHTTPAPI(token string) *HTTPAPI {
	return &HTTPAPI{
		Token: token,
		// Long polling holds the request open for timeoutS; keep margin.
		Client: &http.Client{
			Timeout:       70 * time.Second,
			CheckRedirect: rejectTelegramRedirect,
		},
	}
}

// telegramOperationError deliberately keeps the rendered error independent of
// request URLs, form bodies, Telegram message text, file IDs, and bot tokens.
// Its error graph retains only a classified safe sentinel, never the raw
// transport or filesystem error that may embed a URL, form body, or path.
type telegramOperationError struct {
	operation string
	action    string
	cause     error
}

func (e *telegramOperationError) Error() string {
	return fmt.Sprintf("telegram %s: %s (%s)", e.operation, e.action, telegramCauseClass(e.cause))
}

func (e *telegramOperationError) Unwrap() error { return e.cause }

func newTelegramOperationError(operation, action string, cause error) error {
	return &telegramOperationError{
		operation: operation,
		action:    action,
		cause:     safeTelegramCause(cause),
	}
}

// safeTelegramLogError is a second redaction boundary for alternate API
// implementations and wrappers. The production HTTP adapter already returns
// telegramOperationError, which is retained. Any outer wrapper or injected API
// error is discarded and reduced to a non-secret cause class.
func safeTelegramLogError(operation string, err error) error {
	var safe *telegramOperationError
	if errors.As(err, &safe) {
		return safe
	}
	return newTelegramOperationError(operation, "request failed", err)
}

var (
	errTelegramOperationFailure  = errors.New("operation failure")
	errTelegramNetworkFailure    = errors.New("network failure")
	errTelegramNetworkTimeout    = errors.New("network timeout")
	errTelegramClientUnavailable = errors.New("HTTP client unavailable")
)

func safeTelegramCause(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(err, context.Canceled):
		return context.Canceled
	}
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		return errTelegramOperationFailure
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		if networkError.Timeout() {
			return errTelegramNetworkTimeout
		}
		return errTelegramNetworkFailure
	}
	return errTelegramOperationFailure
}

func telegramCauseClass(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline exceeded"
	case errors.Is(err, context.Canceled):
		return "request canceled"
	case errors.Is(err, errTelegramNetworkTimeout):
		return "network timeout"
	case errors.Is(err, errTelegramNetworkFailure):
		return "network failure"
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		if networkError.Timeout() {
			return "network timeout"
		}
		return "network failure"
	}
	return "operation failure"
}

func rejectTelegramRedirect(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}

func (a *HTTPAPI) noRedirectClient() (*http.Client, error) {
	if a.Client == nil {
		return nil, errTelegramClientUnavailable
	}
	// Clone the injected client so Transport, Timeout, Jar, and other caller
	// settings are preserved. Only redirects are made strictly fail-closed.
	client := *a.Client
	client.CheckRedirect = rejectTelegramRedirect
	return &client, nil
}

func (a *HTTPAPI) call(method string, params url.Values, out any) error {
	client, err := a.noRedirectClient()
	if err != nil {
		return newTelegramOperationError(method, "transport unavailable", err)
	}
	resp, err := client.PostForm(
		fmt.Sprintf("https://api.telegram.org/bot%s/%s", a.Token, method), params)
	if err != nil {
		return newTelegramOperationError(method, "transport request failed", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode < http.StatusBadRequest {
		return fmt.Errorf("telegram %s: redirect rejected (http %d)", method, resp.StatusCode)
	}
	var wrapper struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return newTelegramOperationError(method, "response decode failed", err)
	}
	if !wrapper.OK {
		return fmt.Errorf("telegram %s: API request rejected (http %d)", method, resp.StatusCode)
	}
	if out != nil {
		if err := json.Unmarshal(wrapper.Result, out); err != nil {
			return newTelegramOperationError(method, "result decode failed", err)
		}
	}
	return nil
}

func (a *HTTPAPI) GetMe() (string, error) {
	var me struct {
		Username string `json:"username"`
	}
	if err := a.call("getMe", url.Values{}, &me); err != nil {
		return "", err
	}
	return me.Username, nil
}

func (a *HTTPAPI) GetUpdates(offset int64, timeoutS int) ([]Update, error) {
	params := url.Values{
		"offset":  {strconv.FormatInt(offset, 10)},
		"timeout": {strconv.Itoa(timeoutS)},
	}
	var updates []Update
	if err := a.call("getUpdates", params, &updates); err != nil {
		return nil, err
	}
	return updates, nil
}

func (a *HTTPAPI) SendMessage(chatID int64, text string) error {
	_, err := a.SendMessageResult(chatID, text)
	return err
}

func (a *HTTPAPI) SendMessageResult(chatID int64, text string) (int64, error) {
	var message Message
	err := a.call("sendMessage", url.Values{
		"chat_id":                  {strconv.FormatInt(chatID, 10)},
		"text":                     {text},
		"parse_mode":               {"HTML"},
		"disable_web_page_preview": {"true"},
	}, &message)
	return message.MessageID, err
}

func (a *HTTPAPI) SetInlineKeyboard(chatID, messageID int64, keyboard InlineKeyboard) error {
	markup, err := json.Marshal(struct {
		InlineKeyboard InlineKeyboard `json:"inline_keyboard"`
	}{InlineKeyboard: keyboard})
	if err != nil {
		return err
	}
	return a.call("editMessageReplyMarkup", url.Values{
		"chat_id":      {strconv.FormatInt(chatID, 10)},
		"message_id":   {strconv.FormatInt(messageID, 10)},
		"reply_markup": {string(markup)},
	}, nil)
}

func (a *HTTPAPI) DeleteMessage(chatID, messageID int64) error {
	return a.call("deleteMessage", url.Values{
		"chat_id":    {strconv.FormatInt(chatID, 10)},
		"message_id": {strconv.FormatInt(messageID, 10)},
	}, nil)
}

func (a *HTTPAPI) AnswerCallbackQuery(callbackQueryID, text string) error {
	return a.call("answerCallbackQuery", url.Values{
		"callback_query_id": {callbackQueryID},
		"text":              {text},
	}, nil)
}

func (a *HTTPAPI) ClearInlineKeyboard(chatID, messageID int64) error {
	return a.call("editMessageReplyMarkup", url.Values{
		"chat_id":      {strconv.FormatInt(chatID, 10)},
		"message_id":   {strconv.FormatInt(messageID, 10)},
		"reply_markup": {`{"inline_keyboard":[]}`},
	}, nil)
}

// SetMyCommands registers the command menu shown by Telegram clients.
func (a *HTTPAPI) SetMyCommands(commandsJSON string) error {
	return a.call("setMyCommands", url.Values{"commands": {commandsJSON}}, nil)
}

func (a *HTTPAPI) FileURL(fileID string) (string, error) {
	var file struct {
		FilePath string `json:"file_path"`
	}
	if err := a.call("getFile", url.Values{"file_id": {fileID}}, &file); err != nil {
		return "", err
	}
	return fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", a.Token, file.FilePath), nil
}

func (a *HTTPAPI) Download(fileURL, destPath string) error {
	_, err := a.download(fileURL, destPath, 0)
	return err
}

func (a *HTTPAPI) DownloadBounded(fileURL, destPath string, maxBytes int64) (int64, error) {
	if maxBytes <= 0 {
		return 0, newTelegramOperationError("download", "invalid size limit", errTelegramOperationFailure)
	}
	return a.download(fileURL, destPath, maxBytes)
}

func (a *HTTPAPI) download(fileURL, destPath string, maxBytes int64) (int64, error) {
	client, err := a.noRedirectClient()
	if err != nil {
		return 0, newTelegramOperationError("download", "transport unavailable", err)
	}
	resp, err := client.Get(fileURL)
	if err != nil {
		return 0, newTelegramOperationError("download", "transport request failed", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode < http.StatusBadRequest {
		return 0, fmt.Errorf("telegram download: redirect rejected (http %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download: http %d", resp.StatusCode)
	}
	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, newTelegramOperationError("download", "destination create failed", err)
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return 0, newTelegramOperationError("download", "destination secure failed", err)
	}
	reader := io.Reader(resp.Body)
	if maxBytes > 0 {
		reader = io.LimitReader(resp.Body, maxBytes+1)
	}
	written, copyErr := io.Copy(f, reader)
	syncErr := f.Sync()
	closeErr := f.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		return written, newTelegramOperationError(
			"download", "response copy failed", errors.Join(copyErr, syncErr, closeErr),
		)
	}
	if maxBytes > 0 && written > maxBytes {
		return written, newTelegramOperationError("download", "response exceeds size limit", errTelegramOperationFailure)
	}
	return written, nil
}
