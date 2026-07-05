package bot

import (
	"log/slog"
	"testing"
)

type mockAPI struct {
	sent []string
}

func (m *mockAPI) GetUpdates(offset int64, timeoutS int) ([]Update, error) { return nil, nil }
func (m *mockAPI) SendMessage(chatID int64, text string) error {
	m.sent = append(m.sent, text)
	return nil
}
func (m *mockAPI) FileURL(fileID string) (string, error)   { return "http://x/" + fileID, nil }
func (m *mockAPI) Download(fileURL, destPath string) error { return nil }
func (m *mockAPI) GetMe() (string, error)                  { return "barycenter_bot", nil }

func newTestBot() (*Bot, *mockAPI) {
	api := &mockAPI{}
	b := New(api, slog.Default())
	return b, api
}

func msgFrom(userID int64, text string) Update {
	return Update{UpdateID: 1, Message: &Message{
		From: &User{ID: userID, FirstName: "U"}, Chat: Chat{ID: -100500}, Text: text,
	}}
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
	b, api := newTestBot()
	b.handleUpdate(msgFrom(111, "/vol 500"))
	if len(api.sent) != 1 {
		t.Fatalf("want one reply, got %v", api.sent)
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

func (m *mockAPI) SetMyCommands(string) error { return nil }
