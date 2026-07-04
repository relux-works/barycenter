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
)

type Command struct {
	Kind   CommandKind
	URI    string // link, playnow, inject
	Number int    // cancel position, vol value, offset ms
	Target string // "a" | "b" | "both" | "" (vol/inject/offset target)
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

	case "/start", "/help":
		return Command{}, ErrReply{helpText}
	}

	return Command{}, ErrReply{"не знаю такой команды. /help покажет список"}
}

const helpText = `duet: общий эфир на два дома.
Ссылка на трек — в очередь; плейлист/альбом — общий поток. Голосовое — вставка после текущего трека («лично» — только партнёру).
/periastron — сближение: общий эфир; /apoastron — каждый своё (/inject подкидывает партнёру)
/playnow <ссылка> — немедленно; /queue, /cancel N; /skip, /pause, /resume, /sync
/vol 0-100 [a|b]; /now, /status; /takeover user|coordinator — кто главнее при вмешательстве с телефона
/offset a|b <мс>, /offset_test — калибровка синхры`

// IsPersonalCaption: the "лично" caption on a voice message (spec 9.1).
func IsPersonalCaption(caption string) bool {
	return strings.EqualFold(strings.TrimSpace(caption), "лично")
}
