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

// Event is what the session loop receives from the bot.
type Event struct {
	From    string  // "a" | "b" (home of the sender)
	Command Command // for text messages
	Voice   *VoiceEvent
	// Reply sends a response into the chat the message came from.
	Reply func(text string)
}

type VoiceEvent struct {
	TGFileID  string
	Duration  int // seconds, from Telegram metadata
	SizeBytes int64
	Personal  bool // caption "лично" (spec 9.1)
}

// API abstracts the Telegram Bot HTTP API for tests.
type API interface {
	GetUpdates(offset int64, timeoutS int) ([]Update, error)
	SendMessage(chatID int64, text string) error
	FileURL(fileID string) (string, error)
	Download(fileURL, destPath string) error
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
	ID int64 `json:"id"`
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
	api    API
	log    *slog.Logger
	users  map[int64]string // telegram user id -> "a"|"b" (spec 9.2 allowlist)
	chatID int64            // notification chat (the shared group)
	Events chan Event
}

func New(api API, log *slog.Logger, users map[int64]string, chatID int64) *Bot {
	return &Bot{
		api:    api,
		log:    log,
		users:  users,
		chatID: chatID,
		Events: make(chan Event, 16),
	}
}

// Notify posts to the shared group (session events, spec 9.2).
func (b *Bot) Notify(text string) {
	if err := b.api.SendMessage(b.chatID, text); err != nil {
		b.log.Warn("notify failed", "err", err)
	}
}

// Run long-polls until stop closes (spec 3.3: outbound connection only).
func (b *Bot) Run(stop <-chan struct{}) {
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
	home, allowed := b.users[msg.From.ID]
	if !allowed {
		return // spec 9.2: silence for strangers
	}
	chatID := msg.Chat.ID
	reply := func(text string) {
		if err := b.api.SendMessage(chatID, text); err != nil {
			b.log.Warn("reply failed", "err", err)
		}
	}

	if msg.Voice != nil {
		b.Events <- Event{
			From: home,
			Voice: &VoiceEvent{
				TGFileID:  msg.Voice.FileID,
				Duration:  msg.Voice.Duration,
				SizeBytes: msg.Voice.FileSize,
				Personal:  IsPersonalCaption(msg.Caption),
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
	b.Events <- Event{From: home, Command: cmd, Reply: reply}
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
		"chat_id": {strconv.FormatInt(chatID, 10)},
		"text":    {text},
	}, nil)
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
