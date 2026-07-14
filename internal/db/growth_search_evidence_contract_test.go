package db

import (
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestGrowthSearchUsageBindsNowAsTimestamp(t *testing.T) {
	raw, err := os.ReadFile("queries/growth_radar.sql")
	if err != nil {
		t.Fatal(err)
	}
	query := string(raw)
	if !strings.Contains(query, "sqlc.arg(now_at)::timestamptz - interval '1 day'") {
		t.Fatal("GetGrowthSearchUsage must cast now_at to timestamptz before interval arithmetic")
	}

	// Compile-time contract: sqlc must not regress NowAt back to interface{},
	// which lets PostgreSQL infer the parameter as interval at runtime.
	var _ pgtype.Timestamptz = GetGrowthSearchUsageParams{}.NowAt
}
