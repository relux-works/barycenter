package main

import (
	"crypto/tls"
	"fmt"
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

// M3: only the loopback TLS terminator can split source buckets with its
// canonical X-Forwarded-Proto/X-Forwarded-For pair. Remote peers, direct TLS,
// X-Real-Ip, and ambiguous forwarding frames remain keyed to the direct peer.
func TestClientIPTrustedProxy(t *testing.T) {
	req := func(remote, proto, xff, realIP string) *http.Request {
		r := httptest.NewRequest("POST", "/pair", nil)
		r.RemoteAddr = remote
		if proto != "" {
			r.Header.Set("X-Forwarded-Proto", proto)
		}
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		if realIP != "" {
			r.Header.Set("X-Real-Ip", realIP)
		}
		return r
	}
	// Untrusted: headers ignored.
	if ip := clientIP(req("10.0.0.1:5555", "https", "6.6.6.6", "6.6.6.6"), false); ip != "10.0.0.1" {
		t.Fatalf("untrusted must use RemoteAddr, got %q", ip)
	}
	// A remote peer is not promoted to a proxy merely by the config flag.
	if ip := clientIP(req("10.0.0.1:5555", "https", "203.0.113.7", "203.0.113.8"), true); ip != "10.0.0.1" {
		t.Fatalf("remote spoofed forwarding headers changed source, got %q", ip)
	}
	// Trusted: the loopback TLS terminator's LAST XFF hop wins; earlier entries
	// remain client-forgeable and are ignored.
	if ip := clientIP(req("127.0.0.1:5555", "https", "1.2.3.4, 203.0.113.9", "198.51.100.7"), true); ip != "203.0.113.9" {
		t.Fatalf("trusted last XFF hop, got %q", ip)
	}
	// Missing secure scheme, malformed/duplicate XFF and X-Real-Ip alone all
	// collapse to the proxy's bucket instead of creating attacker keys.
	for name, request := range map[string]*http.Request{
		"missing scheme":   req("127.0.0.1:5555", "", "203.0.113.1", ""),
		"plaintext scheme": req("127.0.0.1:5555", "http", "203.0.113.2", ""),
		"malformed xff":    req("127.0.0.1:5555", "https", "not-an-ip", ""),
		"real ip only":     req("127.0.0.1:5555", "https", "", "203.0.113.3"),
	} {
		if ip := clientIP(request, true); ip != "127.0.0.1" {
			t.Fatalf("%s changed source to %q", name, ip)
		}
	}
	duplicate := req("127.0.0.1:5555", "https", "203.0.113.4", "")
	duplicate.Header.Add("X-Forwarded-For", "203.0.113.5")
	if ip := clientIP(duplicate, true); ip != "127.0.0.1" {
		t.Fatalf("duplicate XFF changed source to %q", ip)
	}
	directTLS := req("127.0.0.1:5555", "https", "203.0.113.6", "")
	directTLS.TLS = &tls.ConnectionState{}
	if ip := clientIP(directTLS, true); ip != "127.0.0.1" {
		t.Fatalf("direct TLS trusted forwarding headers: %q", ip)
	}
	// Two "clients" behind one proxy get separate buckets when trusted.
	rl := newRateLimiter(1, time.Minute)
	if !rl.allow(clientIP(req("127.0.0.1:5555", "https", "198.51.100.1", ""), true)) {
		t.Fatal("first client blocked")
	}
	if !rl.allow(clientIP(req("127.0.0.1:5555", "https", "198.51.100.2", ""), true)) {
		t.Fatal("second client must have its own bucket")
	}
}

func TestRateLimiterBoundsAttackerControlledKeysAndRejectedAttempts(t *testing.T) {
	rl := newRateLimiter(1, time.Hour)
	for i := 0; i < rl.cap*3; i++ {
		if !rl.allow(fmt.Sprintf("198.51.%d.%d", (i/256)%256, i%256)) {
			t.Fatalf("fresh key %d was rejected", i)
		}
	}
	if len(rl.hits) != rl.cap {
		t.Fatalf("source-key state=%d want hard cap=%d", len(rl.hits), rl.cap)
	}
	const abusive = "203.0.113.200"
	if !rl.allow(abusive) || rl.allow(abusive) || rl.allow(abusive) {
		t.Fatal("per-source admission/rejection boundary is not enforced")
	}
	entry := rl.hits[abusive]
	if len(entry.timestamps) != rl.limit+1 {
		t.Fatalf("rejected-attempt state=%d want=%d", len(entry.timestamps), rl.limit+1)
	}
}

func TestCoordinatorHTTPServerHasBoundedPublicTransport(t *testing.T) {
	server := newCoordinatorHTTPServer("127.0.0.1:0", http.NewServeMux())
	if server.ReadHeaderTimeout <= 0 || server.ReadTimeout <= 0 ||
		server.WriteTimeout <= 0 || server.IdleTimeout <= 0 ||
		server.MaxHeaderBytes <= 0 || server.Handler == nil {
		t.Fatalf("unbounded coordinator server: %+v", server)
	}
	if server.ReadTimeout < 2*time.Minute || server.WriteTimeout < time.Minute {
		t.Fatalf("security timeouts break documented ingest bounds: read=%v write=%v",
			server.ReadTimeout, server.WriteTimeout)
	}
}
