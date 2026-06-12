package scheduler

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCeilDiv(t *testing.T) {
	cases := []struct {
		a, b, want int
	}{
		{3 * 5, 7, 3},  // cadence 3/wk, buffer 5d -> ceil(15/7)=3
		{3 * 7, 7, 3},  // exactly a week
		{3 * 14, 7, 6}, // two weeks
		{3 * 0, 7, 0},  // buffer 0 -> stock nothing (operator-driven)
		{0, 7, 0},
		{1, 7, 1},
	}
	for _, c := range cases {
		if got := ceilDiv(c.a, c.b); got != c.want {
			t.Errorf("ceilDiv(%d,%d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSchedulerExposesNotificationTick(t *testing.T) {
	var _ func(*Scheduler, context.Context) = (*Scheduler).TickNotifications
}

func TestSchedulerExposesWorkflowTick(t *testing.T) {
	var _ func(*Scheduler, context.Context) = (*Scheduler).TickWorkflow
}

func TestSchedulerExposesReviewOverdueTick(t *testing.T) {
	var _ func(*Scheduler, context.Context) = (*Scheduler).TickReviewOverdue
}

func TestSchedulerExposesSEOTick(t *testing.T) {
	var _ func(*Scheduler, context.Context) = (*Scheduler).TickSEO
}

func TestSchedulerExposesGEOTick(t *testing.T) {
	var _ func(*Scheduler, context.Context) = (*Scheduler).TickGEO
}

func TestStartRegistersNotificationTick(t *testing.T) {
	raw, err := os.ReadFile("helpers.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "TickNotifications") || !strings.Contains(source, "@every 10s") {
		t.Fatal("Start must register TickNotifications every 10 seconds")
	}
	if !strings.Contains(source, "TickWorkflow") || !strings.Contains(source, "@every 10s") {
		t.Fatal("Start must register TickWorkflow every 10 seconds")
	}
	if !strings.Contains(source, "TickReviewOverdue") || !strings.Contains(source, "@every 30m") {
		t.Fatal("Start must register TickReviewOverdue every 30 minutes")
	}
	if !strings.Contains(source, "TickSEO") || !strings.Contains(source, "0 3 * * *") {
		t.Fatal("Start must register TickSEO daily after generation")
	}
	if !strings.Contains(source, "TickGEO") || !strings.Contains(source, "@weekly") {
		t.Fatal("Start must register TickGEO weekly")
	}
}
