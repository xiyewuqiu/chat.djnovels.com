// Package ratelimit provides the in-process sliding-window limiter used to
// throttle authentication and verification-code endpoints.
//
// The limiter is deliberately stateful in memory: the business server runs as a
// single PM2 process, so a local window is accurate and costs no extra
// dependency. If the deployment ever grows to several processes, the limiter
// must be replaced by a shared store (Redis) rather than trusted per process.
package ratelimit

import (
	"sync"
	"time"
)

// Limit is a sliding window: at most Count events in any Window.
type Limit struct {
	Count  int
	Window time.Duration
}

// Limiter counts events per key inside per-key sliding windows.
type Limiter struct {
	mu         sync.Mutex
	events     map[string][]time.Time
	now        func() time.Time
	lastSweep  time.Time
	maxWindow  time.Duration
	sweepEvery time.Duration
}

// New returns a limiter driven by the wall clock.
func New() *Limiter {
	return NewWithClock(time.Now)
}

// NewWithClock returns a limiter driven by the given clock, which tests use to
// advance time without sleeping.
func NewWithClock(now func() time.Time) *Limiter {
	return &Limiter{
		events:     make(map[string][]time.Time),
		now:        now,
		sweepEvery: time.Minute,
	}
}

// Allow records one event for key and reports whether it stays inside limit.
// When rejected, retryAfter is the wait until the oldest event leaves the
// window. Events are recorded only when accepted.
func (l *Limiter) Allow(key string, limit Limit) (allowed bool, retryAfter time.Duration) {
	if limit.Count <= 0 || limit.Window <= 0 {
		return true, 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.trackWindow(limit.Window)
	l.sweep(now)

	cutoff := now.Add(-limit.Window)
	kept := prune(l.events[key], cutoff)
	if len(kept) >= limit.Count {
		l.events[key] = kept
		return false, nonNegative(kept[0].Add(limit.Window).Sub(now))
	}
	l.events[key] = append(kept, now)
	return true, 0
}

// Reset forgets every event recorded for key.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.events, key)
}

// trackWindow remembers the widest window seen so expired keys can be dropped.
func (l *Limiter) trackWindow(window time.Duration) {
	if window > l.maxWindow {
		l.maxWindow = window
	}
}

// sweep drops keys whose newest event is older than the widest window. It runs
// at most once per sweepEvery so the hot path stays O(1).
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < l.sweepEvery {
		return
	}
	l.lastSweep = now
	for key, events := range l.events {
		if len(events) == 0 || now.Sub(events[len(events)-1]) > l.maxWindow {
			delete(l.events, key)
		}
	}
}

// prune returns the events that are still inside the window, reusing the backing
// array so the slice does not grow without bound.
func prune(events []time.Time, cutoff time.Time) []time.Time {
	if len(events) == 0 {
		return nil
	}
	idx := 0
	for idx < len(events) && !events[idx].After(cutoff) {
		idx++
	}
	if idx == 0 {
		return events
	}
	return append(events[:0], events[idx:]...)
}

func nonNegative(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	return d
}
