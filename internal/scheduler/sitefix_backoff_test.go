package scheduler

import (
	"testing"
	"time"
)

func TestNextSiteFixPollAtFollowsFibonacciThenDaily(t *testing.T) {
	created := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	// Right after creation the next poll is the first checkpoint (5 min).
	if next, ok := nextSiteFixPollAt(created, created); !ok || !next.Equal(created.Add(5*time.Minute)) {
		t.Fatalf("first poll = %v ok=%v", next, ok)
	}
	// 12 minutes in, the next checkpoint is 15 minutes from creation.
	if next, ok := nextSiteFixPollAt(created, created.Add(12*time.Minute)); !ok || !next.Equal(created.Add(15*time.Minute)) {
		t.Fatalf("mid poll = %v ok=%v", next, ok)
	}
	// Past the last Fibonacci checkpoint (3050 min) but before give-up: daily.
	now := created.Add(60 * time.Hour)
	if next, ok := nextSiteFixPollAt(created, now); !ok || !next.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("daily poll = %v ok=%v", next, ok)
	}
	// After 14 days: give up.
	if _, ok := nextSiteFixPollAt(created, created.Add(14*24*time.Hour)); ok {
		t.Fatal("expected give-up after 14 days")
	}
}

func TestNextSiteFixNotifyAtTwelveHourlyThenDaily(t *testing.T) {
	created := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	// Within the first 3 days: every 12h.
	now := created.Add(6 * time.Hour)
	if next, ok := nextSiteFixNotifyAt(created, now); !ok || !next.Equal(now.Add(12*time.Hour)) {
		t.Fatalf("early nag = %v ok=%v", next, ok)
	}
	// After 3 days: daily.
	now = created.Add(4 * 24 * time.Hour)
	if next, ok := nextSiteFixNotifyAt(created, now); !ok || !next.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("late nag = %v ok=%v", next, ok)
	}
	// After 14 days: stop nagging.
	if _, ok := nextSiteFixNotifyAt(created, created.Add(14*24*time.Hour)); ok {
		t.Fatal("expected nag give-up after 14 days")
	}
}

func TestSiteFixPollCheckpointsAreFibonacciWithinThreeDays(t *testing.T) {
	for i := 2; i < len(siteFixPRPollCheckpoints); i++ {
		if siteFixPRPollCheckpoints[i] != siteFixPRPollCheckpoints[i-1]+siteFixPRPollCheckpoints[i-2] {
			t.Fatalf("checkpoint %d is not Fibonacci: %v", i, siteFixPRPollCheckpoints[i])
		}
	}
	last := siteFixPRPollCheckpoints[len(siteFixPRPollCheckpoints)-1]
	if last >= siteFixPRFibonacciWindow {
		t.Fatalf("last checkpoint %v should stay within the 3-day window", last)
	}
}
