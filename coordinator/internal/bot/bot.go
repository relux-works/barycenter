package bot

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	FromUserID int64
	FromName   string
	Command    Command // for text messages
	Voice      *VoiceEvent
	// Reply sends a response into the chat the message came from.
	Reply func(text string)
}

type VoiceEvent struct {
	TGFileID  string
	Duration  int // seconds, from Telegram metadata
	SizeBytes int64
	Personal  bool // caption "лично" (spec 9.1)
	Broadcast bool // caption "всем" — forces broadcast over the orbit default
}

// API abstracts the Telegram Bot HTTP API for tests.
type API interface {
	GetUpdates(offset int64, timeoutS int) ([]Update, error)
	SendMessage(chatID int64, text string) error
	FileURL(fileID string) (string, error)
	Download(fileURL, destPath string) error
	GetMe() (username string, err error)
	SetMyCommands(commandsJSON string) error
}

// Update mirrors the subset of Telegram's Update we need.
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	From      *User  `json:"from"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
	Caption   string `json:"caption"`
	Voice     *Voice `json:"voice"`
}

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type Voice struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
	FileSize int64  `json:"file_size"`
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
}

type outMsg struct {
	chatID int64
	text   string
}

func New(api API, log *slog.Logger) *Bot {
	b := &Bot{
		api:    api,
		log:    log,
		Events: make(chan Event, 16),
		outbox: make(chan outMsg, 1024),
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
		b.log.Warn("outbox full, dropping message", "chat", chatID)
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
			if err := b.api.SendMessage(m.chatID, m.text); err != nil {
				b.log.Warn("send failed", "chat", m.chatID, "err", err)
			}
		}
	}
}

// Run long-polls until stop closes (spec 3.3: outbound connection only).
func (b *Bot) Run(stop <-chan struct{}) {
	go b.sender(stop) // drain the outbox for the lifetime of the bot
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
	msg := u.Message
	if msg == nil || msg.From == nil {
		return
	}
	chatID := msg.Chat.ID
	// Replies go through the async outbox (SendTo) so the FSM loop, which
	// invokes this closure, never blocks on a Telegram POST (bugs #2).
	reply := func(text string) { b.SendTo(chatID, text) }

	if msg.Voice != nil {
		b.Events <- Event{
			ChatID:     chatID,
			FromUserID: msg.From.ID,
			FromName:   msg.From.FirstName,
			Voice: &VoiceEvent{
				TGFileID:  msg.Voice.FileID,
				Duration:  msg.Voice.Duration,
				SizeBytes: msg.Voice.FileSize,
				Personal:  IsPersonalCaption(msg.Caption),
				Broadcast: IsBroadcastCaption(msg.Caption),
			},
			Reply: reply,
		}
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
	b.Events <- Event{ChatID: chatID, FromUserID: msg.From.ID, FromName: msg.From.FirstName, Command: cmd, Reply: reply}
}

// DownloadVoice fetches the raw voice file (ogg/opus) to destPath.
func (b *Bot) DownloadVoice(fileID, destPath string) error {
	u, err := b.api.FileURL(fileID)
	if err != nil {
		return err
	}
	return b.api.Download(u, destPath)
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
		Client: &http.Client{Timeout: 70 * time.Second},
	}
}

func (a *HTTPAPI) call(method string, params url.Values, out any) error {
	resp, err := a.Client.PostForm(
		fmt.Sprintf("https://api.telegram.org/bot%s/%s", a.Token, method), params)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var wrapper struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	if !wrapper.OK {
		return fmt.Errorf("telegram %s: %s", method, wrapper.Description)
	}
	if out != nil {
		return json.Unmarshal(wrapper.Result, out)
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
	return a.call("sendMessage", url.Values{
		"chat_id":                  {strconv.FormatInt(chatID, 10)},
		"text":                     {text},
		"parse_mode":               {"HTML"},
		"disable_web_page_preview": {"true"},
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
	resp, err := a.Client.Get(fileURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: http %d", resp.StatusCode)
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}
