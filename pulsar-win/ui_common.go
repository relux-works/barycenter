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
const (
	uiWindowTitle = "Pulsar"
	uiTitleText   = "Пульсар"
	uiIntroText   = "Пульсар — общий музыкальный эфир для ваших домов. Чтобы подключить этот компьютер, нужен код из телеграм-бота:\r\n" +
		"1. Открой бота и напиши /create — если создаёшь свой барицентр.\r\n" +
		"2. Или открой инвайт-ссылку от партнёра и напиши /pair.\r\n" +
		"3. Введи код сюда — и музыка дома."
	// F5/F6 DoD: the network hint is part of the window text itself.
	uiNetworkHintText = "Телефон и компьютер — в одной Wi-Fi; проверь файрвол и VPN."
	uiBotLinkText     = "Открыть @barycenter_bot"
	uiGuideLinkText   = "Гид по подключению: barycenter.live/guide"
	uiCodeLabelText   = "КОД ИЗ БОТА"
	uiSubmitText      = "Подключить"
	uiSubmitBusyText  = "Подключаю…"
	uiBadCodeError    = "код — 8 букв и цифр из бота"
	uiSaveErrorPrefix = "не смог сохранить учётные данные: "
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
