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

	uiPrivacyURL           = "https://barycenter.live/legal/privacy/ru"
	uiTermsURL             = "https://barycenter.live/legal/terms/ru"
	uiContentGuidelinesURL = "https://barycenter.live/legal/content-guidelines/ru"
	uiUploadRightsURL      = "https://barycenter.live/legal/upload-rights/ru"
	uiSupportURL           = "https://barycenter.live/legal/support/ru"
)

// Onboarding window strings. CRLF, not LF: Win32 STATIC controls only break
// lines on \r\n.
//
// This legacy code-entry window is an optional Telegram companion path. The
// primary shell owns Create and Join and does not require Telegram.
const (
	uiWindowTitle   = "Pulsar"
	uiTitleText     = "Пульсар"
	uiIntroSubtitle = "Необязательное подключение Telegram"
	uiIntroText     = "В главном окне Пульсара можно создать Барицентр или подключить это устройство. Общий эфир создаётся позже.\r\n\r\n" +
		"Если ты решил использовать Telegram как дополнительный пульт, открой @barycenter_bot, " +
		"получи командой /pair код для этого компьютера и введи его ниже.\r\n\r\n" +
		"Локальная проверка, маршрутизация и история доступны без Telegram."
	// F5/F6 DoD: the network hint is part of the window text itself. Leads with
	// the failure the user actually hits ("Pulsar не виден в Spotify") so it
	// reads as a checklist, not a footnote.
	uiNetworkHintText = "Необязательная интеграция Spotify использует локальную сеть. Для неё телефон и компьютер должны быть в одной Wi-Fi. Подробнее — barycenter.live/guide"
	uiBotLinkText     = "Открыть @barycenter_bot"
	uiGuideLinkText   = "Гид по подключению: barycenter.live/guide"
	uiCodeLabelText   = "КОД ИЗ БОТА"
	uiSubmitText      = "Подключить"
	uiSubmitBusyText  = "Подключаю…"
	uiBadCodeError    = "код — 8 букв и цифр из бота"
	uiSaveErrorPrefix = "не смог сохранить учётные данные: "
)

// Optional Spotify help stays available from the tray. It is never presented
// as a prerequisite and is not shown automatically after pairing.
const (
	uiSpotifyStepTitle = "Необязательная интеграция Spotify"
	uiSpotifyStepBody  = "Звук Пульсара и локальная проверка работают без Spotify. Если хочешь использовать Spotify как источник музыки:\n\n" +
		"1.  Открой Spotify (для Spotify Connect нужен Spotify Premium).\n" +
		"2.  В списке устройств выбери «Pulsar».\n" +
		"3.  Включи любой трек — это нужно один раз, чтобы Spotify запомнил колонку.\n\n" +
		"Не видишь «Pulsar» в списке? Телефон и компьютер должны быть в одной Wi-Fi, " +
		"разреши Pulsar в брандмауэре Windows и выключи VPN. Подробнее — barycenter.live/guide"
)

// Extra tray menu strings (#4/#6): the Spotify step and the firewall/zeroconf
// help stay one click away for the whole run, not only at pairing time.
const (
	uiMenuHowToSound = "Необязательная интеграция Spotify…"
	uiMenuNoPulsar   = "Диагностика интеграции Spotify"
	uiMenuPrivacy    = "Конфиденциальность"
	uiMenuTerms      = "Условия использования"
	uiMenuGuidelines = "Правила содержимого"
	uiMenuUpload     = "Права на запись и загрузку"
	uiMenuSupport    = "Поддержка и безопасность"
)

var uiPublicPolicyURLs = []string{
	uiPrivacyURL,
	uiTermsURL,
	uiContentGuidelinesURL,
	uiUploadRightsURL,
	uiSupportURL,
}

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
