package scheduler

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/robfig/cron/v3"
)

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ceilDiv returns ceil(a/b) for non-negative ints (b>0). Used for buffer-window
// slot math. Returns 0 when a<=0.
func ceilDiv(a, b int) int {
	if a <= 0 || b <= 0 {
		return 0
	}
	return (a + b - 1) / b
}

// Start launches the daily generation tick and a more frequent publish tick.
// Returns the cron so the caller can Stop() it. (PRD §5.4: Go cron daily tick.)
func (s *Scheduler) Start(ctx context.Context) *cron.Cron {
	c := cron.New()
	// Daily generation pass (02:00).
	_, _ = c.AddFunc("0 2 * * *", func() { s.TickGenerate(ctx) })
	// Daily SEO data sync and opportunity analysis after content generation.
	_, _ = c.AddFunc("0 3 * * *", func() { s.TickSEO(ctx) })
	// Publish pass every 5 minutes so approved canonicals go out near their slot.
	_, _ = c.AddFunc("*/5 * * * *", func() { s.TickPublish(ctx) })
	// Review overdue pass every 30 minutes so single-operator queues are visible.
	_, _ = c.AddFunc("@every 30m", func() { s.TickReviewOverdue(ctx) })
	// Review recovery pass every 2 minutes: re-run QA, repair, regenerate, and
	// auto-approve so a hands-off project drains its own review queue (§5.5).
	_, _ = c.AddFunc("@every 2m", func() { s.TickReviewRecovery(ctx) })
	// GEO visibility observation/analyzer pass weekly (§12.3).
	_, _ = c.AddFunc("@weekly", func() { s.TickGEO(ctx) })
	// Notification worker pass every 10 seconds for webhook retry/dead handling.
	_, _ = c.AddFunc("@every 10s", func() { s.TickNotifications(ctx) })
	// Workflow worker pass every 10 seconds for durable growth-loop advancement.
	_, _ = c.AddFunc("@every 10s", func() { s.TickWorkflow(ctx) })
	c.Start()
	slog.Default().Info("scheduler started", "generate", "daily@02:00", "seo", "daily@03:00", "publish", "every 5m", "review_overdue", "every 30m", "review_recovery", "every 2m", "geo", "weekly", "notifications", "every 10s", "workflow", "every 10s")
	return c
}
