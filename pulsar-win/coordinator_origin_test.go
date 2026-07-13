package main

import "testing"

func TestCanonicalCoordinatorOriginSharedVectors(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://coord.example.com", "https://coord.example.com"},
		{"https://coord.example.com:443", "https://coord.example.com"},
		{"https://coord.example.com:443/", "https://coord.example.com"},
		{"https://coord.example.com:8443", "https://coord.example.com:8443"},
		{"http://coord.example.com", "http://coord.example.com"},
		{"http://coord.example.com:80", "http://coord.example.com"},
		{"http://coord.example.com:8080", "http://coord.example.com:8080"},
		{"https://COORD.Example.COM", "https://coord.example.com"},
		{"https://coord.example.com.", "https://coord.example.com"},
		{"https://coord.example.com。", "https://coord.example.com"},
		{"https://coord.example.com．", "https://coord.example.com"},
		{"https://coord.example.com｡", "https://coord.example.com"},
		{"https://127.0.0.1", "https://127.0.0.1"},
		{"https://[::1]:8443", "https://[::1]:8443"},
		{"https://[0:0:0:0:0:0:0:1]:8443", "https://[::1]:8443"},
		{"https://münchen.example.com", "https://xn--mnchen-3ya.example.com"},
		{"https://coord.example.com/path?q=1#frag", "https://coord.example.com"},
	}
	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			origin, err := CanonicalCoordinatorOrigin(test.input)
			if err != nil || origin.String() != test.want {
				t.Fatalf("got %q, %v; want %q", origin.String(), err, test.want)
			}
		})
	}
}

func TestCanonicalCoordinatorOriginRejectsAmbiguity(t *testing.T) {
	inputs := []string{
		"https://user:pass@coord.example.com",
		"ftp://coord.example.com",
		"https://[::1%25eth0]:8443",
		"coord.example.com",
		"https:coord.example.com",
		"https://coord%2eexample.com",
		"https://coord.example.com:bad",
		"https://coord.example.com:0",
		"https://127.1",
		"https://0177.0.0.1",
		"https://0x7f000001",
		"https://2130706433",
		"https://coord..example.com",
		"https://-coord.example.com",
		"https://coord_example.com",
		"https://coord.example.com..",
		"https://coord.example.com。。",
		"https://coord..example.com。",
		"https://coord.example.com\\evil",
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			if origin, err := CanonicalCoordinatorOrigin(input); err == nil {
				t.Fatalf("accepted as %q", origin.String())
			}
		})
	}
}

func TestCoordinatorOriginSecretTransportAndWebSocketDerivation(t *testing.T) {
	for _, test := range []struct {
		input, websocket string
		allowed          bool
	}{
		{"https://coord.example", "wss://coord.example/ws", true},
		{"http://127.0.0.1:8080", "ws://127.0.0.1:8080/ws", true},
		{"http://[::1]:8080", "ws://[::1]:8080/ws", true},
		{"http://127.0.0.2:8080", "", false},
		{"http://127.1.2.3:8080", "", false},
		{"http://localhost:8080", "", false},
		{"http://coord.example", "", false},
	} {
		origin, err := CanonicalCoordinatorOrigin(test.input)
		if err != nil {
			t.Fatalf("canonicalize %q: %v", test.input, err)
		}
		if origin.permitsSecrets() != test.allowed {
			t.Fatalf("permitsSecrets(%q)=%t", test.input, origin.permitsSecrets())
		}
		ws, wsErr := origin.WebSocketURL()
		if test.allowed && (wsErr != nil || ws != test.websocket) {
			t.Fatalf("WebSocketURL(%q)=%q %v", test.input, ws, wsErr)
		}
		if !test.allowed && wsErr == nil {
			t.Fatalf("plaintext non-loopback derived %q", ws)
		}
	}
}
