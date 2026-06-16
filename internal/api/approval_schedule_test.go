package api

import (
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCanonicalApprovalScheduleAutoModePublishesImmediately(t *testing.T) {
	now := time.Date(2026, 6, 12, 9, 30, 0, 0, time.UTC)
	cfg := config.Default()
	cfg.PublishMode = config.PublishModeAuto

	got := canonicalApprovalScheduleAt(pgtype.Timestamptz{}, pgtype.Timestamptz{}, cfg, now)
	if !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("schedule = %+v, want immediate due at %s", got, now)
	}
}

func TestCanonicalApprovalScheduleStaggersScheduledMode(t *testing.T) {
	now := time.Date(2026, 6, 12, 9, 30, 0, 0, time.UTC)
	cfg := config.Default()
	cfg.PublishMode = config.PublishModeScheduled
	cfg.PublishIntervalDays = 2

	first := canonicalApprovalScheduleAt(pgtype.Timestamptz{}, pgtype.Timestamptz{}, cfg, now)
	if !first.Valid || !first.Time.Equal(now) {
		t.Fatalf("first schedule = %+v, want now %s", first, now)
	}

	latest := pgtype.Timestamptz{Time: now, Valid: true}
	next := canonicalApprovalScheduleAt(pgtype.Timestamptz{}, latest, cfg, now)
	want := now.AddDate(0, 0, 2)
	if !next.Valid || !next.Time.Equal(want) {
		t.Fatalf("next schedule = %+v, want staggered %s", next, want)
	}
}

func TestCanonicalApprovalScheduleManualModeWaits(t *testing.T) {
	now := time.Date(2026, 6, 12, 9, 30, 0, 0, time.UTC)
	cfg := config.Default()
	cfg.PublishMode = config.PublishModeManual

	got := canonicalApprovalScheduleAt(pgtype.Timestamptz{}, pgtype.Timestamptz{}, cfg, now)
	if got.Valid {
		t.Fatalf("manual mode should leave the schedule unset, got %+v", got)
	}
}

func TestCanonicalApprovalSchedulePreservesExplicitTopicSchedule(t *testing.T) {
	now := time.Date(2026, 6, 12, 9, 30, 0, 0, time.UTC)
	explicit := pgtype.Timestamptz{Time: now.Add(48 * time.Hour), Valid: true}
	cfg := config.Default()

	got := canonicalApprovalScheduleAt(explicit, pgtype.Timestamptz{}, cfg, now)
	if !got.Valid || !got.Time.Equal(explicit.Time) {
		t.Fatalf("schedule = %+v, want explicit topic schedule %s", got, explicit.Time)
	}
}
