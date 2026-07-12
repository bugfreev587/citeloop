//go:build integration

package db

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestGrowthMeasurementEvidenceQueryAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("CITELOOP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CITELOOP_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var projectID uuid.UUID
	var targetURL string
	var latest time.Time
	err = pool.QueryRow(ctx, `
      select project_id, normalized_page_url, max(date)::timestamptz
      from page_performance_daily
      group by project_id, normalized_page_url
      having count(*) >= 7
      order by count(*) desc
      limit 1`).Scan(&projectID, &targetURL, &latest)
	if err != nil {
		t.Fatal(err)
	}
	afterEnd := dateOnlyForIntegration(latest)
	afterStart := afterEnd.AddDate(0, 0, -6)
	baselineEnd := afterStart.AddDate(0, 0, -1)
	baselineStart := baselineEnd.AddDate(0, 0, -6)
	evidence, err := New(pool).GetGrowthMeasurementEvidence(ctx, GetGrowthMeasurementEvidenceParams{
		ProjectID: projectID, TargetUrl: targetURL, ArticleID: pgtype.UUID{}, Query: "", GeoEvidenceIds: []string{},
		BaselineStart: pgtype.Date{Time: baselineStart, Valid: true}, BaselineEnd: pgtype.Date{Time: baselineEnd, Valid: true},
		AfterStart: pgtype.Date{Time: afterStart, Valid: true}, AfterEnd: pgtype.Date{Time: afterEnd, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(evidence, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"gsc", "ga4", "geo", "windows"} {
		if decoded[key] == nil {
			t.Fatalf("evidence envelope missing %q: %s", key, evidence)
		}
	}
}

func dateOnlyForIntegration(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}
