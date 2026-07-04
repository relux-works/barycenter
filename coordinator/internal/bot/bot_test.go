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

func newTestBot() (*Bot, *mockAPI) {
	api := &mockAPI{}
	b := New(api, slog.Default(), map[int64]string{111: "a", 222: "b"}, -100500)
	return b, api
}

func msgFrom(userID int64, text string) Update {
	return Update{UpdateID: 1, Message: &Message{
		From: &User{ID: userID}, Chat: Chat{ID: -100500}, Text: text,
	}}
}

func TestStrangerSilentlyIgnored(t *testing.T) {
	b, api := newTestBot()
	b.handleUpdate(msgFrom(999, "/skip"))
	select {
	case ev := <-b.Events:
		t.Fatalf("stranger produced event %+v (spec 9.2)", ev)
	default:
	}
	if len(api.sent) != 0 {
		t.Fatalf("stranger got replies: %v", api.sent)
	}
}

func TestCommandMapsSenderToHome(t *testing.T) {
	b, _ := newTestBot()
	b.handleUpdate(msgFrom(222, "/skip"))
	ev := <-b.Events
	if ev.From != "b" || ev.Command.Kind != KindSkip {
		t.Fatalf("ev = %+v", ev)
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

func TestVoicePersonalFlag(t *testing.T) {
	b, _ := newTestBot()
	b.handleUpdate(Update{UpdateID: 2, Message: &Message{
		From: &User{ID: 111}, Chat: Chat{ID: -100500},
		Caption: "Лично",
		Voice:   &Voice{FileID: "f1", Duration: 12, FileSize: 30000},
	}})
	ev := <-b.Events
	if ev.Voice == nil || !ev.Voice.Personal || ev.From != "a" || ev.Voice.TGFileID != "f1" {
		t.Fatalf("ev = %+v voice=%+v", ev, ev.Voice)
	}
}
