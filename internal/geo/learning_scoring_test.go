package geo

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/learning"
	"github.com/google/uuid"
)

func TestGEOGapPersistsLearningScoreProvenance(t *testing.T) {
	scorer := learning.NewScorer([]db.ListGrowthLearningsRow{{
		ID: uuid.New(), ScoringEligible: true, ActionFamily: "geo_project_mentioned_without_citation",
		PrimaryMetric: "ai_citation_count", OutcomeLabel: "positive",
		TargetIdentity: json.RawMessage(`{"query":"best growth tools"}`),
		Audience:       json.RawMessage(`["people searching for best growth tools"]`),
	}})
	gap, err := applyGEOLearningScore(context.Background(), geoGap{
		Type: "geo_project_mentioned_without_citation", PromptText: "best growth tools",
		Priority: 78, Evidence: map[string]any{},
	}, scorer)
	if err != nil {
		t.Fatal(err)
	}
	if gap.Priority != 81 || gap.Evidence["learning_scoring"] == nil {
		t.Fatalf("GEO learning score = %#v", gap)
	}
}
