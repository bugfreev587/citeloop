package growthwork

import (
	"encoding/json"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/growthspec"
	"github.com/jackc/pgx/v5/pgtype"
)

func withGrowthSpecification(params db.CreateCanonicalGrowthOpportunityParams, now time.Time) (db.CreateCanonicalGrowthOpportunityParams, growthspec.Result, error) {
	result := growthspec.Build(growthspec.Input{
		Type:              params.Type,
		Query:             stringPointerValue(params.Query),
		TargetURL:         firstText(params.NormalizedPageUrl, stringPointerValue(params.PageUrl)),
		RecommendedAction: stringPointerValue(params.RecommendedAction),
		ExpectedImpact:    stringPointerValue(params.ExpectedImpact),
		Evidence:          params.Evidence,
		Now:               now,
	})
	specJSON, err := result.JSON()
	if err != nil {
		return params, result, err
	}
	missingJSON, err := json.Marshal(result.Missing)
	if err != nil {
		return params, result, err
	}
	params.GrowthSpecState = result.State
	params.GrowthSpecVersion = result.Version
	params.GrowthSpec = specJSON
	params.GrowthSpecMissing = missingJSON
	params.DecisionReadyAt = pgtype.Timestamptz{}
	if result.State == growthspec.StateDecisionReady {
		params.DecisionReadyAt = pgtype.Timestamptz{Time: now.UTC(), Valid: true}
	}
	return params, result, nil
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func firstText(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
