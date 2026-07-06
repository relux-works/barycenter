package resolver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOdesliLinksParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"linksByPlatform":{
			"spotify":{"url":"https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT"},
			"yandex":{"url":"https://music.yandex.ru/album/1193829/track/10994777"}}}`))
	}))
	defer srv.Close()
	o := NewOdesli("")
	o.Base = srv.URL
	links, err := o.Links("https://open.spotify.com/track/x")
	if err != nil {
		t.Fatal(err)
	}
	if links["spotify"] != "spotify:track:4cOdK2wGLETKBW3PvgPWqT" || links["yandex"] != "10994777:1193829" {
		t.Fatalf("%v", links)
	}
}

func TestYandexTrackAndSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/tracks/10994777":
			w.Write([]byte(`{"result":[{"id":10994777,"title":"Astral Entrance","durationMs":213800,
				"available":true,"albums":[{"id":1193829}],"artists":[{"name":"Beast In Black"}]}]}`))
		case r.URL.Path == "/search":
			w.Write([]byte(`{"result":{"tracks":{"results":[{"id":1,"title":"T","durationMs":1000,
				"available":true,"albums":[{"id":2}],"artists":[{"name":"A"}]}]}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	y := NewYandex("tok")
	y.Base = srv.URL
	c, isrc, err := y.TrackByRef("10994777:1193829")
	if err != nil || isrc != "" || c.Ref != "10994777:1193829" || c.DurationMS != 213800 {
		t.Fatalf("%+v %q %v", c, isrc, err)
	}
	cands, err := y.Search("A T")
	if err != nil || len(cands) != 1 || cands[0].Ref != "1:2" {
		t.Fatalf("%+v %v", cands, err)
	}
}
