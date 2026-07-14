package bot

import (
	"errors"
	"strings"
	"testing"
)

const trackURL = "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT?si=x"
const trackURI = "spotify:track:4cOdK2wGLETKBW3PvgPWqT"

func TestParse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Command
	}{
		{"bare link", trackURL, Command{Kind: KindLink, URI: trackURI}},
		{"link in text", "зацени " + trackURL + " пушка", Command{Kind: KindLink, URI: trackURI}},
		{"chatter ignored", "ну че, как оно?", Command{Kind: KindIgnore}},
		{"empty ignored", "   ", Command{Kind: KindIgnore}},
		{"playnow", "/playnow " + trackURL, Command{Kind: KindPlayNow, URI: trackURI}},
		{"queue", "/queue", Command{Kind: KindQueue}},
		{"cancel", "/cancel 3", Command{Kind: KindCancel, Number: 3}},
		{"skip with botname", "/skip@duet_bot", Command{Kind: KindSkip}},
		{"pause", "/pause", Command{Kind: KindPause}},
		{"resume", "/resume", Command{Kind: KindResume}},
		{"vol own node", "/vol 65", Command{Kind: KindVol, Number: 65}},
		{"vol other node", "/vol 40 b", Command{Kind: KindVol, Number: 40, Target: "b"}},
		{"inject default", "/inject " + trackURL, Command{Kind: KindInject, URI: trackURI}},
		{"inject target", "/inject " + trackURL + " both", Command{Kind: KindInject, URI: trackURI, Target: "both"}},
		{"mode solo", "/mode solo", Command{Kind: KindMode, Target: "solo"}},
		{"mode shared uppercase", "/MODE Shared", Command{Kind: KindMode, Target: "shared"}},
		{"now", "/now", Command{Kind: KindNow}},
		{"status", "/status", Command{Kind: KindStatus}},
		{"sync", "/sync", Command{Kind: KindSync}},
		{"offset", "/offset b 250", Command{Kind: KindOffset, Number: 250, Target: "b"}},
		{"offset negative", "/offset a -50", Command{Kind: KindOffset, Number: -50, Target: "a"}},
		{"offset_test", "/offset_test", Command{Kind: KindOffsetTest}},
		{"playlist link", "https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M?si=x",
			Command{Kind: KindPlaylist, URI: "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M", Target: "playlist"}},
		{"album uri as base layer", "spotify:album:4cOdK2wGLETKBW3PvgPWqT",
			Command{Kind: KindPlaylist, URI: "spotify:album:4cOdK2wGLETKBW3PvgPWqT", Target: "album"}},
		{"takeover user", "/takeover user", Command{Kind: KindTakeover, Target: "user"}},
		{"takeover coordinator", "/takeover coordinator", Command{Kind: KindTakeover, Target: "coordinator"}},
		{"together = shared", "/together", Command{Kind: KindMode, Target: "shared"}},
		{"solo = solo", "/solo", Command{Kind: KindMode, Target: "solo"}},
		{"periastron = shared", "/periastron", Command{Kind: KindMode, Target: "shared"}},
		{"apoastron = solo", "/apoastron", Command{Kind: KindMode, Target: "solo"}},
		{"home = orbit", "/home", Command{Kind: KindOrbit}},
		{"orbit still works", "/orbit", Command{Kind: KindOrbit}},
		{"leave", "/leave", Command{Kind: KindLeave}},
		{"dissolve", "/dissolve", Command{Kind: KindDissolve}},
		{"make_primary by name", "/make_primary Катя", Command{Kind: KindMakePrimary, Target: "Катя"}},
		{"make_primary empty", "/make_primary", Command{Kind: KindMakePrimary}},
		{"provider switch", "/provider b yandex", Command{Kind: KindProvider, Target: "b", Provider: "yandex"}},
		{"provider case folded", "/provider A Spotify", Command{Kind: KindProvider, Target: "a", Provider: "spotify"}},
		{"yandex track link", "https://music.yandex.ru/album/1193829/track/10994777",
			Command{Kind: KindLink, URI: "yandex:track:10994777:1193829"}},
		{"approach propose", "/approach", Command{Kind: KindApproach}},
		{"approach claim uppercases", "/approach abcd2345", Command{Kind: KindApproach, Target: "ABCD2345"}},
		{"accept", "/accept", Command{Kind: KindAccept}},
		{"decline", "/decline", Command{Kind: KindDecline}},
		{"apart", "/apart", Command{Kind: KindApart}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Parse(c.in)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != c.want {
				t.Fatalf("got %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestParseUserErrors(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"episode link replies", "https://open.spotify.com/episode/" + "4cOdK2wGLETKBW3PvgPWqT"},
		{"playnow without link", "/playnow"},
		{"cancel garbage", "/cancel третий"},
		{"vol out of range", "/vol 150"},
		// M8: single letters a..z all parse (orbits pair up to five homes;
		// the loop checks existence) — only non-slot shapes are parse errors.
		{"vol bad node", "/vol 50 ab"},
		{"mode unknown", "/mode party"},
		{"offset missing args", "/offset a"},
		{"provider missing args", "/provider b"},
		{"provider unknown service", "/provider b tidal"},
		{"unknown command", "/dance"},
		{"resolve removed from surface", "/resolve"},
		{"help", "/help"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.in)
			var reply ErrReply
			if !errors.As(err, &reply) {
				t.Fatalf("want ErrReply, got %v", err)
			}
			if reply.Text == "" {
				t.Fatal("reply text empty")
			}
		})
	}
}

func TestHelpPublishesStablePolicyAndSupportLinks(t *testing.T) {
	for _, path := range []string{
		"/legal/privacy/ru",
		"/legal/terms/ru",
		"/legal/content-guidelines/ru",
		"/legal/upload-rights/ru",
		"/legal/support/ru",
	} {
		if !strings.Contains(helpText, "https://barycenter.live"+path) {
			t.Fatalf("help lacks %s", path)
		}
	}
}

func TestPersonalCaption(t *testing.T) {
	for _, yes := range []string{"лично", "ЛИЧНО", "  Лично  "} {
		if !IsPersonalCaption(yes) {
			t.Fatalf("%q must be personal", yes)
		}
	}
	for _, no := range []string{"", "лично тебе", "personal"} {
		if IsPersonalCaption(no) {
			t.Fatalf("%q must not be personal", no)
		}
	}
}
