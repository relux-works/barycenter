// Clock offset estimation against the coordinator (spec 8.5) — a port of
// the macOS node's ClockSync.swift, same estimator and same numbers:
// NTP-style four timestamps, EMA smoothing (alpha 0.2), rejection of samples
// with rtt > 3x the median of the last ten (once at least four are seen).
//
// Sign convention (matches the Swift source): offset = node clock minus
// coordinator clock, so T_local = T_coord + offset - output_latency_offset.
package main

import (
	"math"
	"sort"
	"sync"
	"time"
)

const clockSyncRTTWindow = 10

// ClockSync is safe for concurrent use (the WS read loop feeds it, the
// player reads deadlines from another goroutine).
type ClockSync struct {
	mu         sync.Mutex
	offsetMS   float64
	hasOffset  bool
	lastRTTMS  int64
	recentRTTs []int64
	alpha      float64
}

func NewClockSync() *ClockSync {
	return &ClockSync{alpha: 0.2}
}

// AddSample feeds one ping/pong exchange. Returns true if the sample was
// accepted (moved the offset).
func (c *ClockSync) AddSample(t1, t2, t3, t4 int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	rtt := (t4 - t1) - (t3 - t2)
	if rtt < 0 {
		return false
	}

	c.lastRTTMS = rtt

	if len(c.recentRTTs) >= clockSyncRTTWindow {
		c.recentRTTs = c.recentRTTs[1:]
	}
	c.recentRTTs = append(c.recentRTTs, rtt)

	if m, ok := c.medianLocked(); ok && len(c.recentRTTs) >= 4 && float64(rtt) > 3*m && m > 0 {
		return false // transient congestion: keep the offset we trust
	}

	sample := (float64(t1-t2) + float64(t4-t3)) / 2
	if c.hasOffset {
		c.offsetMS += c.alpha * (sample - c.offsetMS)
	} else {
		c.offsetMS = sample
		c.hasOffset = true
	}
	return true
}

func (c *ClockSync) medianLocked() (float64, bool) {
	if len(c.recentRTTs) == 0 {
		return 0, false
	}
	sorted := append([]int64(nil), c.recentRTTs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return float64(sorted[mid-1]+sorted[mid]) / 2, true
	}
	return float64(sorted[mid]), true
}

// OffsetMS returns the smoothed offset and whether any sample was accepted.
func (c *ClockSync) OffsetMS() (float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.offsetMS, c.hasOffset
}

// LastRTTMS returns the rtt of the last sample (accepted or not).
func (c *ClockSync) LastRTTMS() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastRTTMS
}

// LocalDeadline is T_local = T_coord + clock_offset - output_latency_offset
// (spec 6.3/5.4). Returns ok=false before the first accepted sample.
func (c *ClockSync) LocalDeadline(tCoordMS int64, outputLatencyOffsetMS int) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.hasOffset {
		return 0, false
	}
	return tCoordMS + int64(math.Round(c.offsetMS)) - int64(outputLatencyOffsetMS), true
}

// nowMS is the wall-clock timestamp used across the protocol (unix ms).
func nowMS() int64 { return time.Now().UnixMilli() }
