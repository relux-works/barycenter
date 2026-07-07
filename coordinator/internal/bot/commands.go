// Package bot implements the Telegram interface (spec ch. 9): long polling,
// two-user allowlist, command parsing, voice intake. Transport is behind an
// interface so command handling is unit-testable without Telegram.
package bot

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"relux.works/duet/coordinator/internal/links"
)

type CommandKind string

const (
	KindLink       CommandKind = "link"     // bare track link -> enqueue
	KindPlaylist   CommandKind = "playlist" // playlist/album link -> shared base layer (U10)
	KindTakeover   CommandKind = "takeover" // /takeover user|coordinator (U9)
	KindPlayNow    CommandKind = "playnow"
	KindQueue      CommandKind = "queue"
	KindCancel     CommandKind = "cancel"
	KindSkip       CommandKind = "skip"
	KindPause      CommandKind = "pause"
	KindResume     CommandKind = "resume"
	KindVol        CommandKind = "vol"
	KindInject     CommandKind = "inject"
	KindMode       CommandKind = "mode"
	KindNow        CommandKind = "now"
	KindStatus     CommandKind = "status"
	KindSync       CommandKind = "sync"
	KindOffset     CommandKind = "offset"
	KindOffsetTest CommandKind = "offset_test"
	KindIgnore     CommandKind = "ignore" // plain chatter: stay silent (spec 9.2)

	// v2.1 multi-tenant onboarding & orbit administration
	KindStart       CommandKind = "start"        // /start [invite payload]
	KindCreate      CommandKind = "create"       // /create — new orbit
	KindShare       CommandKind = "share"        // /share — member invite link
	KindPairCode    CommandKind = "pair"         // /pair — node pairing code
	KindRebind      CommandKind = "rebind"       // /rebind — revoke old, re-pair
	KindMakePrimary CommandKind = "make_primary" // /make_primary [tg id]
	KindRevoke      CommandKind = "revoke"       // /revoke <slot>
	KindOrbit       CommandKind = "orbit"        // /orbit — members & slots

	// Provider layer (spec-providers, behind DUET_PROVIDERS)
	KindProvider CommandKind = "provider" // /provider <slot> <spotify|yandex> (§6.3)
	KindResolve  CommandKind = "resolve"  // /resolve — manual mapping repair (reserved)

	// Approaches between barycenters (design §12, L1)
	KindApproach CommandKind = "approach" // /approach [code] — propose / claim an approach
	KindAccept   CommandKind = "accept"   // /accept — initiator confirms the approach
	KindDecline  CommandKind = "decline"  // /decline — initiator rejects the approach
	KindApart    CommandKind = "apart"    // /apart — dissolve the active approach
)

type Command struct {
	Kind     CommandKind
	URI      string // link, playnow, inject
	Number   int    // cancel position, vol value, offset ms
	Target   string // "a" | "b" | "both" | "" (vol/inject/offset target, provider slot)
	Provider string // /provider service id: "spotify" | "yandex"
}

// ErrReply: parsing failed in a way the user should hear about.
type ErrReply struct{ Text string }

func (e ErrReply) Error() string { return e.Text }

// Parse turns a text message into a Command (spec 9.1).
func Parse(text string) (Command, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return Command{Kind: KindIgnore}, nil
	}

	if !strings.HasPrefix(trimmed, "/") {
		ref, err := links.ParseRef(trimmed)
		switch {
		case err == nil && ref.Kind == "track":
			return Command{Kind: KindLink, URI: ref.URI}, nil
		case err == nil: // playlist | album -> shared base layer (U10)
			return Command{Kind: KindPlaylist, URI: ref.URI, Target: ref.Kind}, nil
		case errors.Is(err, links.ErrUnsupportedKind):
			return Command{}, ErrReply{"такие ссылки не поддерживаю — кидай трек, плейлист или альбом"}
		default:
			return Command{Kind: KindIgnore}, nil // chatter: silence
		}
	}

	fields := strings.Fields(trimmed)
	cmd := strings.ToLower(fields[0])
	// Group chats append @botname: /skip@duet_bot
	if i := strings.IndexByte(cmd, '@'); i > 0 {
		cmd = cmd[:i]
	}
	args := fields[1:]

	switch cmd {
	case "/start":
		payload := ""
		if len(args) > 0 {
			payload = args[0]
		}
		return Command{Kind: KindStart, Target: payload}, nil
	case "/create":
		return Command{Kind: KindCreate, Target: strings.Join(args, " ")}, nil
	case "/share":
		return Command{Kind: KindShare}, nil
	case "/pair":
		return Command{Kind: KindPairCode}, nil
	case "/rebind":
		return Command{Kind: KindRebind}, nil
	case "/make_primary":
		if len(args) == 0 {
			return Command{Kind: KindMakePrimary}, nil
		}
		id, err := strconv.Atoi(args[0])
		if err != nil {
			return Command{}, ErrReply{"так: /make_primary <id участника> (список в /orbit)"}
		}
		return Command{Kind: KindMakePrimary, Number: id}, nil
	case "/revoke":
		if len(args) != 1 {
			return Command{}, ErrReply{"так: /revoke <слот> (слоты в /orbit)"}
		}
		return Command{Kind: KindRevoke, Target: strings.ToLower(args[0])}, nil
	case "/orbit":
		return Command{Kind: KindOrbit}, nil

	// Approaches (design §12): codes come uppercase from randomCode, typing
	// is forgiven.
	case "/approach":
		code := ""
		if len(args) > 0 {
			code = strings.ToUpper(args[0])
		}
		return Command{Kind: KindApproach, Target: code}, nil
	case "/accept":
		return Command{Kind: KindAccept}, nil
	case "/decline":
		return Command{Kind: KindDecline}, nil
	case "/apart":
		return Command{Kind: KindApart}, nil

	case "/playnow":
		if len(args) == 0 {
			return Command{}, ErrReply{"нужна ссылка: /playnow <ссылка на трек>"}
		}
		uri, err := links.ParseTrack(strings.Join(args, " "))
		if err != nil {
			return Command{}, ErrReply{"не вижу ссылки на трек в /playnow"}
		}
		return Command{Kind: KindPlayNow, URI: uri}, nil

	case "/queue":
		return Command{Kind: KindQueue}, nil

	case "/cancel":
		if len(args) != 1 {
			return Command{}, ErrReply{"так: /cancel <номер из /queue>"}
		}
		n, err := strconv.Atoi(args[0])
		if err != nil || n < 1 {
			return Command{}, ErrReply{fmt.Sprintf("«%s» не похоже на номер элемента", args[0])}
		}
		return Command{Kind: KindCancel, Number: n}, nil

	case "/skip":
		return Command{Kind: KindSkip}, nil
	case "/pause":
		return Command{Kind: KindPause}, nil
	case "/resume":
		return Command{Kind: KindResume}, nil
	case "/sync":
		return Command{Kind: KindSync}, nil
	case "/now":
		return Command{Kind: KindNow}, nil
	case "/status":
		return Command{Kind: KindStatus}, nil
	case "/offset_test":
		return Command{Kind: KindOffsetTest}, nil

	case "/vol":
		if len(args) == 0 {
			return Command{}, ErrReply{"так: /vol <0-100> [a|b]"}
		}
		v, err := strconv.Atoi(args[0])
		if err != nil || v < 0 || v > 100 {
			return Command{}, ErrReply{fmt.Sprintf("громкость «%s» не в диапазоне 0-100", args[0])}
		}
		target := ""
		if len(args) > 1 {
			target = strings.ToLower(args[1])
			if target != "a" && target != "b" {
				return Command{}, ErrReply{"дом указывается как a или b: /vol 60 b"}
			}
		}
		return Command{Kind: KindVol, Number: v, Target: target}, nil

	case "/inject":
		if len(args) == 0 {
			return Command{}, ErrReply{"так: /inject <ссылка> [a|b|both]"}
		}
		uri, err := links.ParseTrack(args[0])
		if err != nil {
			return Command{}, ErrReply{"не вижу ссылки на трек в /inject"}
		}
		target := ""
		if len(args) > 1 {
			target = strings.ToLower(args[1])
			if target != "a" && target != "b" && target != "both" {
				return Command{}, ErrReply{"цель: a, b или both"}
			}
		}
		return Command{Kind: KindInject, URI: uri, Target: target}, nil

	case "/mode":
		if len(args) != 1 {
			return Command{}, ErrReply{"так: /mode shared или /mode solo (или красиво: /periastron и /apoastron)"}
		}
		m := strings.ToLower(args[0])
		if m != "shared" && m != "solo" {
			return Command{}, ErrReply{fmt.Sprintf("режима «%s» нет, есть shared и solo", args[0])}
		}
		return Command{Kind: KindMode, Target: m}, nil

	// Периастрон — точка наибольшего сближения орбит: общий поток.
	// Апоастрон — точка наибольшего удаления: каждый слушает своё.
	case "/periastron":
		return Command{Kind: KindMode, Target: "shared"}, nil
	case "/apoastron":
		return Command{Kind: KindMode, Target: "solo"}, nil

	case "/takeover":
		if len(args) != 1 {
			return Command{}, ErrReply{"так: /takeover user (телефон главнее) или /takeover coordinator (эфир главнее)"}
		}
		p := strings.ToLower(args[0])
		if p != "user" && p != "coordinator" {
			return Command{}, ErrReply{"политика: user или coordinator"}
		}
		return Command{Kind: KindTakeover, Target: p}, nil

	case "/offset":
		if len(args) != 2 {
			return Command{}, ErrReply{"так: /offset <a|b> <миллисекунды>"}
		}
		target := strings.ToLower(args[0])
		if target != "a" && target != "b" {
			return Command{}, ErrReply{"первый аргумент: a или b"}
		}
		ms, err := strconv.Atoi(args[1])
		if err != nil {
			return Command{}, ErrReply{fmt.Sprintf("«%s» не похоже на миллисекунды", args[1])}
		}
		return Command{Kind: KindOffset, Number: ms, Target: target}, nil

	case "/provider":
		if len(args) != 2 {
			return Command{}, ErrReply{"так: /provider <дом> <spotify|yandex>"}
		}
		p := strings.ToLower(args[1])
		if p != "spotify" && p != "yandex" {
			return Command{}, ErrReply{fmt.Sprintf("провайдера «%s» нет, есть spotify и yandex", args[1])}
		}
		return Command{Kind: KindProvider, Target: strings.ToLower(args[0]), Provider: p}, nil

	case "/resolve":
		// Reserved (spec-providers §8): argument parsing arrives with the
		// ctid queues; for now the loop answers with a stub.
		return Command{Kind: KindResolve}, nil

	case "/help":
		return Command{}, ErrReply{helpText}
	}

	return Command{}, ErrReply{"не знаю такой команды. /help покажет список"}
}

const helpText = `<b>Барицентр</b> — общий музыкальный эфир на несколько домов.
Сайт и приложение: barycenter.live

<b>Музыка</b>
Ссылка на трек — в очередь. Плейлист или альбом — общий поток.
Голосовое сообщение — вставка после текущего трека (подпись «лично» — только адресату, «всем» — во все дома).

<b>Эфир</b>
/playnow &lt;ссылка&gt; — включить немедленно
/queue — очередь · /cancel N — убрать номер N
/skip · /pause · /resume · /sync
/vol 0–100 [дом] — громкость
/now — что играет · /status — состояние системы

<b>Режимы</b>
/periastron — сближение: общий эфир
/apoastron — каждый слушает своё (/inject &lt;ссылка&gt; подкинет трек партнёру)
/takeover user|coordinator — кто главнее, если вмешались с телефона

<b>Орбит</b>
/create — создать свой барицентр (для новичков)
/orbit — участники и дома · /share — пригласить
/pair — код для подключения своего Пульсара · /rebind — переподключить дом заново
/make_primary — передать главную звезду · /revoke &lt;дом&gt; — отозвать доступ

<b>Сближение</b>
/approach — код сближения для другого барицентра (/approach КОД — принять его код)
инициатор подтверждает: /accept или /decline
/apart — завершить сближение, каждый у себя

<b>Калибровка</b>
/offset &lt;дом&gt; &lt;мс&gt; · /offset_test — синхронные клики`

// IsPersonalCaption: the "лично" caption on a voice message (spec 9.1).
func IsPersonalCaption(caption string) bool {
	return strings.EqualFold(strings.TrimSpace(caption), "лично")
}

// IsBroadcastCaption: the "всем" caption forces a broadcast voice insert
// over the orbit's personal-by-default setting (design §5).
func IsBroadcastCaption(caption string) bool {
	c := strings.TrimSpace(caption)
	return strings.EqualFold(c, "всем") || strings.EqualFold(c, "all")
}
