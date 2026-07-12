package seo

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/learning"
	"github.com/google/uuid"
)

func TestSearchAndActionableCandidatesPersistLearningScoreProvenance(t *testing.T) {
	rows := []db.ListGrowthLearningsRow{{
		ID: uuid.New(), ScoringEligible: true, ActionFamily: "gsc_low_ctr_query",
		PrimaryMetric: "gsc_ctr", OutcomeLabel: "positive",
		TargetIdentity: json.RawMessage(`{"normalized_target_url":"https://example.com/a","query":"growth loop"}`),
		Audience:       json.RawMessage(`["people searching for growth loop"]`),
	}, {
		ID: uuid.New(), ScoringEligible: true, ActionFamily: "thin_evidence_page",
		PrimaryMetric: "ai_citation_count", OutcomeLabel: "negative",
		TargetIdentity: json.RawMessage(`{"normalized_target_url":"https://example.com/b"}`),
		Audience:       json.RawMessage(`["organic and answer-engine visitors to https://example.com/b"]`),
	}}
	scorer := learning.NewScorer(rows)

	search, err := applySearchLearningScores(context.Background(), []searchMetricOpportunityCandidate{{
		Type: "gsc_low_ctr_query", Query: "growth loop", NormalizedPageURL: "https://example.com/a",
		PriorityScore: 70, Evidence: map[string]any{},
	}}, scorer)
	if err != nil {
		t.Fatal(err)
	}
	if search[0].PriorityScore != 73 || search[0].Evidence["learning_scoring"] == nil {
		t.Fatalf("search learning score = %#v", search[0])
	}

	actionable, err := applyActionableLearningScores(context.Background(), []actionableSEOOpportunityCandidate{{
		Type: "thin_evidence_page", NormalizedPageURL: "https://example.com/b",
		PriorityScore: 64, Evidence: map[string]any{},
	}}, scorer)
	if err != nil {
		t.Fatal(err)
	}
	if actionable[0].PriorityScore != 61 || actionable[0].Evidence["learning_scoring"] == nil {
		t.Fatalf("actionable learning score = %#v", actionable[0])
	}
}
