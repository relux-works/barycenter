package main

import (
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
