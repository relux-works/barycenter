// Portable half of the Windows onboarding/tray UX (goal v2.1 F6): the
// Russian strings and the pure helpers live outside the //go:build windows
// fence so the darwin/linux unit suites pin them — the blind Win32 layer in
// ui_windows.go stays plumbing that consumes tested pieces.
//
// Wording mirrors the macOS shell (OnboardingWindow.swift, StatusMenu.swift):
// both shells speak the same language to the same two users.
package main

import (
	"net/url"
	"strings"
)

const (
	// pairCodeLen: the bot issues 8-char alphanumeric one-time codes; the
	// macOS onboarding field enforces the same length.
	pairCodeLen = 8

	// uiGuideURL opens from the onboarding window (F5: the full firewall /
	// same-Wi-Fi / VPN walkthrough lives in the guide; the window carries
	// the one-line hint).
	uiGuideURL = "https://barycenter.live/guide"
	uiBotURL   = "https://t.me/barycenter_bot"
)

// Onboarding window strings. CRLF, not LF: Win32 STATIC controls only break
// lines on \r\n.
//
// The couple's first session is the INVITE path (product decision 2026-07-08):
// Katya opens her partner's invite link, the bot adds her to HIS shared air as
// a companion, she runs /pair for a code, enters it here — two homes, one
// stream, no /approach. /create stays the secondary (solo/host) path. Both
// shells (this window + OnboardingWindow.swift) now tell that one story.
const (
	uiWindowTitle   = "Pulsar"
	uiTitleText     = "Пульсар"
	uiIntroSubtitle = "Общий музыкальный эфир для твоих колонок"
	uiIntroText     = "Чтобы слушать вместе с партнёром:\r\n\r\n" +
		"1.  Партнёр открывает @barycenter_bot и командой /share присылает тебе ссылку-приглашение.\r\n" +
		"2.  Открой ссылку и напиши боту /pair — он пришлёт код для этого компьютера.\r\n" +
		"3.  Введи код ниже — и музыка заиграет у вас обоих.\r\n\r\n" +
		"Свой эфир с нуля? Напиши боту /create, потом /pair — и введи код."
	// F5/F6 DoD: the network hint is part of the window text itself. Leads with
	// the failure the user actually hits ("Pulsar не виден в Spotify") so it
	// reads as a checklist, not a footnote.
	uiNetworkHintText = "Не видишь «Pulsar» в Spotify? Телефон и компьютер — в одной Wi-Fi; разреши Pulsar в брандмауэре Windows; выключи VPN. Подробнее — barycenter.live/guide"
	uiBotLinkText     = "Открыть @barycenter_bot"
	uiGuideLinkText   = "Гид по подключению: barycenter.live/guide"
	uiCodeLabelText   = "КОД ИЗ БОТА"
	uiSubmitText      = "Подключить"
	uiSubmitBusyText  = "Подключаю…"
	uiBadCodeError    = "код — 8 букв и цифр из бота"
	uiSaveErrorPrefix = "не смог сохранить учётные данные: "
)

// Post-pair "one more step" copy (#4): pairing only links this computer to the
// air — until the user picks "Pulsar" in Spotify once and presses play, every
// track fails as track_unavailable and reads as "broken". Shown as a modal
// right after a successful pair and always reachable from the tray ("Как
// включить звук"). MessageBoxW breaks lines on \n (not \r\n).
const (
	uiSpotifyStepTitle = "Готово! Остался один шаг"
	uiSpotifyStepBody  = "Компьютер подключён к эфиру. Чтобы пошёл звук:\n\n" +
		"1.  Открой Spotify (нужен Spotify Premium).\n" +
		"2.  В списке устройств выбери «Pulsar».\n" +
		"3.  Включи любой трек — это нужно один раз, чтобы Spotify запомнил колонку.\n\n" +
		"Не видишь «Pulsar» в списке? Телефон и компьютер должны быть в одной Wi-Fi, " +
		"разреши Pulsar в брандмауэре Windows и выключи VPN. Подробнее — barycenter.live/guide"
)

// Extra tray menu strings (#4/#6): the Spotify step and the firewall/zeroconf
// help stay one click away for the whole run, not only at pairing time.
const (
	uiMenuHowToSound = "Как включить звук…"
	uiMenuNoPulsar   = "Не вижу Pulsar в Spotify?"
)

// Tray menu strings (mirror StatusMenuController.menuNeedsUpdate).
const (
	uiTrayConnected    = "Барицентр: в сети"
	uiTrayReconnecting = "Барицентр: переподключение…"
	uiTrayUnpaired     = "не спарен — введи код из @barycenter_bot"
	uiMenuRepair       = "Подключить заново…"
	uiMenuPair         = "Подключить…"
	uiMenuQuit         = "Выйти"
)

// normalizePairCode mirrors the macOS onboarding filter (uppercase, letters
// and digits only, cap at pairCodeLen): spaces, dashes and anything else
// pasted from Telegram are dropped; the submit handler validates the final
// length. Non-ASCII letters are dropped too — bot codes are ASCII.
func normalizePairCode(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
			r -= 'a' - 'A'
		default:
			continue
		}
		b.WriteRune(r)
		if b.Len() == pairCodeLen {
			break
		}
	}
	return b.String()
}

// trayStatusLine is the first (disabled) tray menu line — the coordinator
// link state, read live on every menu open.
func trayStatusLine(connected bool) string {
	if connected {
		return uiTrayConnected
	}
	return uiTrayReconnecting
}

// identityLine renders the connection identity menu line: coordinator host
// · дом slot (F3: the menu answers "where does this node belong" without
// opening any config file).
func identityLine(c Credentials) string {
	host := c.WSURL
	if u, err := url.Parse(c.WSURL); err == nil && u.Host != "" {
		host = u.Host
	}
	return host + " · дом " + c.Slot
}
