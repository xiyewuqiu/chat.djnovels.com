package ratelimit

import (
	"testing"
	"time"
)

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

func TestAllowStopsAtLimit(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	limiter := NewWithClock(clock.Now)
	limit := Limit{Count: 2, Window: time.Minute}

	for i := 0; i < 2; i++ {
		if ok, _ := limiter.Allow("k", limit); !ok {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	ok, retry := limiter.Allow("k", limit)
	if ok {
		t.Fatal("third attempt must be rejected")
	}
	if retry != time.Minute {
		t.Fatalf("retry after = %v, want 1m", retry)
	}
}

func TestWindowSlidesForward(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	limiter := NewWithClock(clock.Now)
	limit := Limit{Count: 1, Window: time.Minute}

	if ok, _ := limiter.Allow("k", limit); !ok {
		t.Fatal("first attempt rejected")
	}
	clock.Advance(30 * time.Second)
	if ok, retry := limiter.Allow("k", limit); ok || retry > 30*time.Second {
		t.Fatalf("attempt inside window: ok=%v retry=%v", ok, retry)
	}
	clock.Advance(30*time.Second + time.Millisecond)
	if ok, _ := limiter.Allow("k", limit); !ok {
		t.Fatal("attempt after window should be allowed")
	}
}

func TestKeysAreIndependent(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	limiter := NewWithClock(clock.Now)
	limit := Limit{Count: 1, Window: time.Minute}

	if ok, _ := limiter.Allow("a", limit); !ok {
		t.Fatal("a rejected on first attempt")
	}
	if ok, _ := limiter.Allow("b", limit); !ok {
		t.Fatal("b must not inherit a's window")
	}
}

func TestResetClearsWindow(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	limiter := NewWithClock(clock.Now)
	limit := Limit{Count: 1, Window: time.Minute}

	if ok, _ := limiter.Allow("k", limit); !ok {
		t.Fatal("first attempt rejected")
	}
	if ok, _ := limiter.Allow("k", limit); ok {
		t.Fatal("second attempt should be rejected")
	}
	limiter.Reset("k")
	if ok, _ := limiter.Allow("k", limit); !ok {
		t.Fatal("attempt after reset should be allowed")
	}
}

func TestSweepDropsStaleKeys(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	limiter := NewWithClock(clock.Now)
	limit := Limit{Count: 1, Window: time.Minute}

	if ok, _ := limiter.Allow("stale", limit); !ok {
		t.Fatal("first attempt rejected")
	}
	clock.Advance(2 * time.Minute)
	if ok, _ := limiter.Allow("fresh", limit); !ok {
		t.Fatal("fresh key rejected")
	}
	if _, exists := limiter.events["stale"]; exists {
		t.Fatal("stale key should have been swept")
	}
}

func TestZeroLimitIsDisabled(t *testing.T) {
	limiter := New()
	if ok, _ := limiter.Allow("k", Limit{}); !ok {
		t.Fatal("zero limit must allow every attempt")
	}
}
