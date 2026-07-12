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

// Start launches the automatic generation, scheduled-topic, and publish ticks.
// Returns the cron so the caller can Stop() it.
func (s *Scheduler) Start(ctx context.Context) *cron.Cron {
	c := cron.New()
	// Frequent generation pass so auto-approved plan items draft promptly.
	_, _ = c.AddFunc("@every 5m", func() { s.TickGenerate(ctx) })
	// Scheduled-topic pass every 5 minutes so an operator-scheduled plan item
	// drafts near its slot instead of waiting for the daily buffer pass.
	_, _ = c.AddFunc("*/5 * * * *", func() { s.TickScheduledTopics(ctx) })
	// Daily SEO data sync and opportunity analysis after content generation.
	_, _ = c.AddFunc("0 3 * * *", func() { s.TickSEO(ctx) })
	// Finite Growth measurement lifecycle: advance due checkpoints and enforce
	// absolute terminal deadlines even when no workflow event is emitted.
	_, _ = c.AddFunc("@every 1h", func() { s.TickMeasurements(ctx) })
	// Weekly user-facing SEO Doctor health reports.
	_, _ = c.AddFunc("@weekly", func() { s.TickSEODoctor(ctx) })
	// Publish pass every 5 minutes so approved canonicals go out near their slot.
	_, _ = c.AddFunc("*/5 * * * *", func() { s.TickPublish(ctx) })
	// Site-fix PR reconcile every 5 minutes: detect merged/closed source-backed
	// PRs so the apply ledger advances without an operator telling us it landed.
	_, _ = c.AddFunc("*/5 * * * *", func() { s.TickSiteFixReconcile(ctx) })
	// Review overdue pass every 30 minutes so single-operator queues are visible.
	_, _ = c.AddFunc("@every 30m", func() { s.TickReviewOverdue(ctx) })
	// Review recovery pass every 2 minutes: re-run QA, repair, regenerate, and
	// auto-approve so a hands-off project drains its own review queue (§5.5).
	_, _ = c.AddFunc("@every 2m", func() { s.TickReviewRecovery(ctx) })
	// Lightweight Context refresh pass weekly for confirmed project domains.
	_, _ = c.AddFunc("@weekly", func() { s.TickContextRefresh(ctx) })
	// Notification worker pass every 10 seconds for webhook retry/dead handling.
	_, _ = c.AddFunc("@every 10s", func() { s.TickNotifications(ctx) })
	// Workflow worker pass every 10 seconds for durable growth-loop advancement.
	_, _ = c.AddFunc("@every 10s", func() { s.TickWorkflow(ctx) })
	c.Start()
	slog.Default().Info("scheduler started", "generate", "every 5m", "scheduled_topics", "every 5m", "seo", "daily@03:00", "measurements", "every 1h", "seo_doctor", "weekly", "publish", "every 5m", "review_overdue", "every 30m", "review_recovery", "every 2m", "context_refresh", "weekly", "notifications", "every 10s", "workflow", "every 10s")
	return c
}
