package main

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/bot"
	"relux.works/duet/coordinator/internal/config"
	"relux.works/duet/coordinator/internal/store"
)

func TestTelegramContentPolicyDisplayAcceptAndAttachmentGate(t *testing.T) {
	cfg := &config.Config{
		SelfServiceOnboarding: true,
		DBPath:                t.TempDir() + "/telegram-content-policy.db",
		MediaDir:              t.TempDir(),
		Media:                 config.Media{RetentionDays: 7},
	}
	st, err := store.OpenWithOptions(cfg.DBPath, store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CreateOrbit("Policy Barycenter", 101); err != nil {
		t.Fatal(err)
	}
	l := newLoop(slog.Default(), cfg, &fakeSender{}, st, nil, nil)
	l.warmup()
	adapter := newControlledTelegramAdapter(st)
	l.telegramMedia = adapter

	missingReplies := &replies{}
	l.handleBot(bot.Event{
		FromUserID: 101, FromName: "Policy user", Reply: missingReplies.fn,
		Attachment: &bot.AttachmentEvent{
			Kind: bot.AttachmentAudio, TGFileID: "must-not-download-before-consent",
			FileName: "untrusted.mp3",
		},
	})
	if !strings.Contains(missingReplies.last(t), "/content_policy") {
		t.Fatalf("missing policy reply=%q", missingReplies.last(t))
	}
	select {
	case accepted := <-adapter.accepted:
		t.Fatalf("attachment reached ingest before policy acceptance: %+v", accepted)
	default:
	}

	voiceReplies := &replies{}
	l.handleBot(bot.Event{
		FromUserID: 101, FromName: "Policy user", Reply: voiceReplies.fn,
		Voice: &bot.VoiceEvent{TGFileID: "legacy-voice-remains-frozen"},
	})
	voice := takeTelegramAcceptance(t, adapter)
	if voice.AttachmentKind != "voice" {
		t.Fatalf("legacy voice kind=%q", voice.AttachmentKind)
	}
	adapter.complete(voice.MediaID, controlledTelegramCompletion{code: "media_signature_unsupported"})
	l.handleMediaDone(takeMediaDone(t, l))

	displayReplies := &replies{}
	l.handleBot(telegramBotEvent(t, 101, "private", "/content_policy en", displayReplies))
	display := displayReplies.last(t)
	if !strings.Contains(display, "Upload and sharing rights") ||
		!strings.Contains(display, store.ContentPolicyTermsURL) ||
		!strings.Contains(display, store.ContentPolicyGuidelinesURL) ||
		!strings.Contains(display, store.CurrentContentPolicyHash) ||
		!strings.Contains(display, "/accept_content_policy en") {
		t.Fatalf("content policy display=%q", display)
	}

	acceptReplies := &replies{}
	l.handleBot(telegramBotEvent(t, 101, "private", "/accept_content_policy en", acceptReplies))
	if !strings.Contains(acceptReplies.last(t), "Accepted at") ||
		!strings.Contains(acceptReplies.last(t), "does not prove content ownership") {
		t.Fatalf("accept reply=%q", acceptReplies.last(t))
	}
	ctx, err := st.ResolveTelegramActorContext(101)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := st.RequireCurrentContentPolicy(ctx.ActorID, store.Identity{
		Kind: store.IdentityTelegram, TelegramUserID: 101,
	})
	if err != nil || !grant.Current || grant.Locale != store.ContentPolicyLocaleEN ||
		grant.AcceptedVia != "telegram" {
		t.Fatalf("telegram grant=%+v err=%v", grant, err)
	}

	acceptedReplies := &replies{}
	l.handleBot(bot.Event{
		FromUserID: 101, FromName: "Policy user", Reply: acceptedReplies.fn,
		Attachment: &bot.AttachmentEvent{
			Kind: bot.AttachmentDocument, TGFileID: "allowed-after-consent",
			FileName: "still-untrusted.bin",
		},
	})
	attachment := takeTelegramAcceptance(t, adapter)
	if attachment.AttachmentKind != string(bot.AttachmentDocument) {
		t.Fatalf("accepted attachment=%+v", attachment)
	}
	adapter.complete(attachment.MediaID, controlledTelegramCompletion{code: "media_signature_unsupported"})
	l.handleMediaDone(takeMediaDone(t, l))

	if _, err := st.RevokeContentPolicy(ctx.ActorID, store.Identity{
		Kind: store.IdentityTelegram, TelegramUserID: 101,
	}, store.ContentPolicyLocaleEN, time.Now().Add(time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	revokedReplies := &replies{}
	l.handleBot(bot.Event{
		FromUserID: 101, FromName: "Policy user", Reply: revokedReplies.fn,
		Attachment: &bot.AttachmentEvent{
			Kind: bot.AttachmentAudio, TGFileID: "must-not-download-after-revoke",
		},
	})
	if !strings.Contains(revokedReplies.last(t), "/content_policy") {
		t.Fatalf("revoked reply=%q", revokedReplies.last(t))
	}
	select {
	case accepted := <-adapter.accepted:
		t.Fatalf("revoked attachment reached ingest: %+v", accepted)
	default:
	}
}
