package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterBlocksBurst(t *testing.T) {
	rl := newRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.allow("1.2.3.4") {
			t.Fatalf("hit %d should pass", i)
		}
	}
	if rl.allow("1.2.3.4") {
		t.Fatal("4th hit must be blocked")
	}
	if !rl.allow("5.6.7.8") {
		t.Fatal("a different IP must not be affected")
	}
}

// M3: behind the TLS-terminating proxy every request's RemoteAddr is the
// proxy — all clients shared one bucket. With trusted_proxy the key follows
// the proxy-appended headers; without it headers are ignored (forgeable).
func TestClientIPTrustedProxy(t *testing.T) {
	req := func(remote, xff, realIP string) *http.Request {
		r := httptest.NewRequest("POST", "/pair", nil)
		r.RemoteAddr = remote
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		if realIP != "" {
			r.Header.Set("X-Real-Ip", realIP)
		}
		return r
	}
	// Untrusted: headers ignored.
	if ip := clientIP(req("10.0.0.1:5555", "6.6.6.6", "6.6.6.6"), false); ip != "10.0.0.1" {
		t.Fatalf("untrusted must use RemoteAddr, got %q", ip)
	}
	// Trusted: X-Real-Ip first.
	if ip := clientIP(req("10.0.0.1:5555", "", "203.0.113.7"), true); ip != "203.0.113.7" {
		t.Fatalf("trusted X-Real-Ip, got %q", ip)
	}
	// Trusted: LAST X-Forwarded-For hop (ours); earlier entries are forgeable.
	if ip := clientIP(req("10.0.0.1:5555", "1.2.3.4, 203.0.113.9", ""), true); ip != "203.0.113.9" {
		t.Fatalf("trusted last XFF hop, got %q", ip)
	}
	// Trusted but no headers (direct hit): RemoteAddr.
	if ip := clientIP(req("10.0.0.1:5555", "", ""), true); ip != "10.0.0.1" {
		t.Fatalf("trusted fallback, got %q", ip)
	}
	// Two "clients" behind one proxy get separate buckets when trusted.
	rl := newRateLimiter(1, time.Minute)
	if !rl.allow(clientIP(req("10.0.0.1:5555", "", "198.51.100.1"), true)) {
		t.Fatal("first client blocked")
	}
	if !rl.allow(clientIP(req("10.0.0.1:5555", "", "198.51.100.2"), true)) {
		t.Fatal("second client must have its own bucket")
	}
}
