package api

import (
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCanonicalApprovalScheduleUsesImmediateDueWhenAutoAdvanceEnabled(t *testing.T) {
	now := time.Date(2026, 6, 12, 9, 30, 0, 0, time.UTC)
	cfg := config.Default()
	cfg.AutoAdvanceEnabled = true
	cfg.BufferDays = 5

	got := canonicalApprovalScheduleAt(pgtype.Timestamptz{}, cfg, now)
	if !got.Valid || !got.Time.Equal(now) {
		t.Fatalf("schedule = %+v, want immediate due at %s", got, now)
	}
}

func TestCanonicalApprovalScheduleUsesBufferWhenAutoAdvanceDisabled(t *testing.T) {
	now := time.Date(2026, 6, 12, 9, 30, 0, 0, time.UTC)
	cfg := config.Default()
	cfg.AutoAdvanceEnabled = false
	cfg.BufferDays = 5

	got := canonicalApprovalScheduleAt(pgtype.Timestamptz{}, cfg, now)
	want := now.Add(5 * 24 * time.Hour)
	if !got.Valid || !got.Time.Equal(want) {
		t.Fatalf("schedule = %+v, want buffer-delayed due at %s", got, want)
	}
}

func TestCanonicalApprovalSchedulePreservesExplicitTopicSchedule(t *testing.T) {
	now := time.Date(2026, 6, 12, 9, 30, 0, 0, time.UTC)
	explicit := pgtype.Timestamptz{Time: now.Add(48 * time.Hour), Valid: true}
	cfg := config.Default()
	cfg.AutoAdvanceEnabled = true

	got := canonicalApprovalScheduleAt(explicit, cfg, now)
	if !got.Valid || !got.Time.Equal(explicit.Time) {
		t.Fatalf("schedule = %+v, want explicit topic schedule %s", got, explicit.Time)
	}
}
