// Direct port of the macOS ClockSyncTests.swift expectations — the two
// implementations must agree on every number.
package main

import (
	"math"
	"testing"
)

func TestPerfectSymmetricLinkConverges(t *testing.T) {
	sync := NewClockSync()
	// Node clock is 250 ms ahead of coordinator; one-way delay 20 ms.
	// t1 node=1000 (coord 750), t2 coord=770, t3 coord=772, t4 node=1042.
	for i := 0; i < 20; i++ {
		base := int64(i * 1000)
		sync.AddSample(1000+base, 770+base, 772+base, 1042+base)
	}
	offset, ok := sync.OffsetMS()
	if !ok {
		t.Fatal("no offset after 20 samples")
	}
	if math.Abs(offset-250) >= 1 {
		t.Fatalf("offset %.2f, want ~250", offset)
	}
	if sync.LastRTTMS() != 40 {
		t.Fatalf("rtt %d, want 40", sync.LastRTTMS())
	}
}

func TestOutlierRTTRejected(t *testing.T) {
	sync := NewClockSync()
	for i := 0; i < 10; i++ {
		base := int64(i * 1000)
		sync.AddSample(1000+base, 770+base, 772+base, 1042+base)
	}
	before, _ := sync.OffsetMS()
	// Congestion spike: rtt 400 ms with a wildly wrong asymmetric offset.
	if sync.AddSample(100_000, 99_500, 99_502, 100_402) {
		t.Fatal("rtt 400 > 3x median(40) must be rejected")
	}
	after, _ := sync.OffsetMS()
	if math.Abs(after-before) >= 0.001 {
		t.Fatalf("rejected sample moved the offset: %.4f -> %.4f", before, after)
	}
}

func TestEMASmoothsJitter(t *testing.T) {
	sync := NewClockSync()
	sync.AddSample(1000, 770, 772, 1042)   // offset 250
	sync.AddSample(2000, 1760, 1762, 2042) // sample says 260
	offset, _ := sync.OffsetMS()
	// EMA alpha 0.2: 250 + 0.2*(260-250) = 252
	if math.Abs(offset-252) >= 0.5 {
		t.Fatalf("offset %.2f, want ~252", offset)
	}
}

func TestLocalDeadlineAppliesOffsetAndLatency(t *testing.T) {
	sync := NewClockSync()
	sync.AddSample(1000, 770, 772, 1042) // node ahead by 250
	got, ok := sync.LocalDeadline(10_000, 120)
	if !ok {
		t.Fatal("deadline expected after one accepted sample")
	}
	// T_local = T_coord + offset - latency = 10000 + 250 - 120 (spec 6.3)
	if got != 10_130 {
		t.Fatalf("deadline %d, want 10130", got)
	}
}

func TestNoSamplesMeansNoDeadline(t *testing.T) {
	sync := NewClockSync()
	if _, ok := sync.LocalDeadline(1, 0); ok {
		t.Fatal("deadline without samples must not exist")
	}
}

func TestNegativeRTTSampleIgnored(t *testing.T) {
	sync := NewClockSync()
	// rtt = (1500-1000) - (2000-900) = -600: an impossible exchange.
	if sync.AddSample(1000, 900, 2000, 1500) {
		t.Fatal("negative rtt must be rejected")
	}
	if _, ok := sync.OffsetMS(); ok {
		t.Fatal("rejected first sample must not create an offset")
	}
}
