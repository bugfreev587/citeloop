package growthradar

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/search"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type DBSearchEvidenceStore struct{ Q *db.Queries }

func (s DBSearchEvidenceStore) FindSearchEvidence(ctx context.Context, projectID uuid.UUID, requestHash string, now time.Time) (*EvidenceSet, error) {
	row, err := s.Q.GetCachedGrowthSearchEvidence(ctx, db.GetCachedGrowthSearchEvidenceParams{ProjectID: projectID, RequestHash: requestHash, NowAt: pgtype.Timestamptz{Time: now, Valid: true}})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var results []search.Result
	if err := json.Unmarshal(row.Results, &results); err != nil {
		return nil, err
	}
	return &EvidenceSet{
		ProjectID: row.ProjectID, NormalizedQuery: row.NormalizedQuery, RequestHash: row.RequestHash,
		ResultSetHash: row.ResultSetHash, Provider: row.Provider, ProviderOrderIsRank: !row.ProviderOrderNotRank,
		Results: results, Synthetic: row.Synthetic, UsableForScoring: !row.Synthetic, Status: "cached",
		CostUSD: pgutil.Float(row.RequestCostUsd), FetchedAt: row.FetchedAt.Time, ExpiresAt: row.ExpiresAt.Time, Trigger: row.TriggerKind,
	}, nil
}

func (s DBSearchEvidenceStore) SearchUsage(ctx context.Context, projectID uuid.UUID, now time.Time) (SearchBudget, error) {
	row, err := s.Q.GetGrowthSearchUsage(ctx, db.GetGrowthSearchUsageParams{ProjectID: projectID, NowAt: pgtype.Timestamptz{Time: now, Valid: true}})
	if err != nil {
		return SearchBudget{}, err
	}
	return SearchBudget{DailyRequests: int(row.DailyRequests), WeeklyRebuildRequests: int(row.WeeklyRebuildRequests), RollingRequests: int(row.RollingRequests), RollingCostUSD: pgutil.Float(row.RollingCostUsd), InstallationCostUSD: pgutil.Float(row.InstallationCostUsd)}, nil
}

func (s DBSearchEvidenceStore) SaveSearchEvidence(ctx context.Context, set EvidenceSet) error {
	results, _ := json.Marshal(set.Results)
	_, err := s.Q.CreateGrowthSearchEvidence(ctx, db.CreateGrowthSearchEvidenceParams{
		ProjectID: set.ProjectID, NormalizedQuery: set.NormalizedQuery, RequestHash: set.RequestHash,
		ResultSetHash: set.ResultSetHash, Provider: set.Provider, Results: results, Synthetic: set.Synthetic,
		TriggerKind: set.Trigger, RequestCostUsd: pgutil.Numeric(set.CostUSD),
		FetchedAt: pgtype.Timestamptz{Time: set.FetchedAt, Valid: true}, ExpiresAt: pgtype.Timestamptz{Time: set.ExpiresAt, Valid: true},
	})
	return err
}
