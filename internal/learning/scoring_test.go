package learning

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

type scoringStoreStub struct {
	params db.ListApplicableGrowthLearningsParams
	rows   []db.ListApplicableGrowthLearningsRow
	err    error
}

func (s *scoringStoreStub) ListApplicableGrowthLearnings(_ context.Context, params db.ListApplicableGrowthLearningsParams) ([]db.ListApplicableGrowthLearningsRow, error) {
	s.params = params
	return s.rows, s.err
}

func TestScorerAppliesOnlyCompatibleDirectionalLearnings(t *testing.T) {
	target := json.RawMessage(`{"normalized_target_url":"https://example.com/growth","query":"growth loop"}`)
	audience := json.RawMessage(`["growth leaders"]`)
	positiveID := uuid.New()
	negativeID := uuid.New()
	scorer := NewScorer([]db.ListGrowthLearningsRow{
		learningRow(positiveID, "content_refresh", "gsc_clicks", "positive", target, audience, true),
		learningRow(negativeID, "content_refresh", "gsc_clicks", "negative", target, audience, true),
		learningRow(uuid.New(), "comparison_page", "gsc_clicks", "positive", target, audience, true),
		learningRow(uuid.New(), "content_refresh", "ga4_key_events", "positive", target, audience, true),
		learningRow(uuid.New(), "content_refresh", "gsc_clicks", "positive", json.RawMessage(`{"query":"other"}`), audience, true),
		learningRow(uuid.New(), "content_refresh", "gsc_clicks", "positive", target, json.RawMessage(`["developers"]`), true),
		learningRow(uuid.New(), "content_refresh", "gsc_clicks", "positive", target, audience, false),
	})

	result := scorer.Score(CandidateScoreInput{
		BaseScore: 72, ActionFamily: "content_refresh", PrimaryMetric: "gsc_clicks",
		TargetIdentity: map[string]any{"normalized_target_url": "https://example.com/growth", "query": "growth loop"},
		Audience:       []string{"growth leaders"},
	})

	if result.AdjustedScore != 72 || result.Adjustment != 0 {
		t.Fatalf("score = %.2f adjustment %.2f, want 72 and 0", result.AdjustedScore, result.Adjustment)
	}
	if len(result.Applications) != 2 || len(result.LearningIDs) != 2 {
		t.Fatalf("applications = %#v", result.Applications)
	}
	if result.LearningIDs[0] != positiveID || result.LearningIDs[1] != negativeID {
		t.Fatalf("learning IDs = %v", result.LearningIDs)
	}
	if result.Version != ScoringVersionV1 {
		t.Fatalf("version = %q", result.Version)
	}
}

func TestScorerBoundsSingleAndAggregateInfluence(t *testing.T) {
	rows := make([]db.ListGrowthLearningsRow, 0, 5)
	for range 5 {
		rows = append(rows, learningRow(uuid.New(), "content_refresh", "gsc_clicks", "positive",
			json.RawMessage(`{"query":"growth loop"}`), json.RawMessage(`["growth leaders"]`), true))
	}
	result := NewScorer(rows).Score(CandidateScoreInput{
		BaseScore: 95, ActionFamily: "content_refresh", PrimaryMetric: "gsc_clicks",
		TargetIdentity: map[string]any{"query": "growth loop"}, Audience: []string{"growth leaders"},
	})
	if result.Adjustment != 8 || result.AdjustedScore != 100 {
		t.Fatalf("bounded result = %#v", result)
	}
	for _, applied := range result.Applications {
		if applied.Adjustment > 3 || applied.Adjustment < -3 {
			t.Fatalf("single learning adjustment not bounded: %#v", applied)
		}
	}
}

func TestScorerAggregateBoundIsIndependentOfLearningOrder(t *testing.T) {
	ids := []uuid.UUID{uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()}
	rows := []db.ListGrowthLearningsRow{
		learningRow(ids[0], "content_refresh", "gsc_clicks", "positive", json.RawMessage(`{"query":"growth loop"}`), json.RawMessage(`["growth leaders"]`), true),
		learningRow(ids[1], "content_refresh", "gsc_clicks", "positive", json.RawMessage(`{"query":"growth loop"}`), json.RawMessage(`["growth leaders"]`), true),
		learningRow(ids[2], "content_refresh", "gsc_clicks", "positive", json.RawMessage(`{"query":"growth loop"}`), json.RawMessage(`["growth leaders"]`), true),
		learningRow(ids[3], "content_refresh", "gsc_clicks", "positive", json.RawMessage(`{"query":"growth loop"}`), json.RawMessage(`["growth leaders"]`), true),
		learningRow(ids[4], "content_refresh", "gsc_clicks", "negative", json.RawMessage(`{"query":"growth loop"}`), json.RawMessage(`["growth leaders"]`), true),
	}
	input := CandidateScoreInput{BaseScore: 70, ActionFamily: "content_refresh", PrimaryMetric: "gsc_clicks", TargetIdentity: map[string]any{"query": "growth loop"}, Audience: []string{"growth leaders"}}
	forward := NewScorer(rows).Score(input)
	for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
		rows[left], rows[right] = rows[right], rows[left]
	}
	reverse := NewScorer(rows).Score(input)
	if forward.RawAdjustment != 9 || reverse.RawAdjustment != 9 || forward.Adjustment != 8 || reverse.Adjustment != 8 || forward.AdjustedScore != reverse.AdjustedScore {
		t.Fatalf("order-dependent score: forward=%#v reverse=%#v", forward, reverse)
	}
}

func TestScoreResultProvenanceIsPersistableAndExplainable(t *testing.T) {
	id := uuid.New()
	result := NewScorer([]db.ListGrowthLearningsRow{
		learningRow(id, "content_refresh", "gsc_clicks", "mixed",
			json.RawMessage(`{"query":"growth loop"}`), json.RawMessage(`["growth leaders"]`), true),
	}).Score(CandidateScoreInput{
		BaseScore: 70, ActionFamily: "content_refresh", PrimaryMetric: "gsc_clicks",
		TargetIdentity: map[string]any{"query": "growth loop"}, Audience: []string{"growth leaders"},
	})
	provenance := result.Provenance()
	for _, key := range []string{"version", "base_score", "raw_adjustment", "adjustment", "aggregate_cap", "adjusted_score", "learning_ids", "applications"} {
		if _, ok := provenance[key]; !ok {
			t.Fatalf("provenance missing %q: %#v", key, provenance)
		}
	}
	applications, ok := provenance["applications"].([]LearningApplication)
	if !ok || len(applications) != 1 || applications[0].LearningID != id || len(applications[0].MatchedDimensions) != 4 {
		t.Fatalf("provenance applications = %#v", provenance["applications"])
	}
}

func TestProjectScorerQueriesApplicableDimensionsBeforeLimiting(t *testing.T) {
	projectID := uuid.New()
	learningID := uuid.New()
	store := &scoringStoreStub{rows: []db.ListApplicableGrowthLearningsRow{{
		ID: learningID, ScoringEligible: true, ActionFamily: "content_refresh",
		PrimaryMetric: "gsc_clicks", OutcomeLabel: "positive",
		TargetIdentity: json.RawMessage(`{"query":"growth loop"}`), Audience: json.RawMessage(`["growth leaders"]`),
	}}}
	input := CandidateScoreInput{BaseScore: 70, ActionFamily: "content_refresh", PrimaryMetric: "gsc_clicks", TargetIdentity: map[string]any{"query": "growth loop"}, Audience: []string{"growth leaders"}}
	result, err := NewProjectScorer(store, projectID).ScoreCandidate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.AdjustedScore != 73 || len(result.LearningIDs) != 1 || result.LearningIDs[0] != learningID {
		t.Fatalf("project score = %#v", result)
	}
	if store.params.ProjectID != projectID || store.params.ActionFamily != input.ActionFamily || store.params.PrimaryMetric != input.PrimaryMetric || store.params.LimitRows != 5 {
		t.Fatalf("applicability query params = %#v", store.params)
	}
	if string(store.params.TargetIdentity) != `{"query":"growth loop"}` || string(store.params.Audience) != `["growth leaders"]` {
		t.Fatalf("applicability JSON params = target %s audience %s", store.params.TargetIdentity, store.params.Audience)
	}
}

func learningRow(id uuid.UUID, family, metric, outcome string, target, audience json.RawMessage, eligible bool) db.ListGrowthLearningsRow {
	return db.ListGrowthLearningsRow{
		ID: id, ActionFamily: family, PrimaryMetric: metric, OutcomeLabel: outcome,
		TargetIdentity: target, Audience: audience, ScoringEligible: eligible,
	}
}
