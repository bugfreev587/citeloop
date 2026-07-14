package growthradar

import (
	"reflect"
	"testing"
)

func TestScoreCandidateBucketsAndThresholds(t *testing.T) {
	base := Snapshot{
		CurrentImpressions: 1000, PreviousImpressions: 400, QualifiedRecurrence: 5,
		PrimaryCoverage: "none", InternalLinkPaths: 0, SelectedExternalTargets: 3, CoveredExternalTargets: 0,
		CapabilityConfirmed: true, AudienceConfirmed: true, IntentSupported: true,
		Intent: "comparison", JourneyStage: "decision", ConversionMapping: "high",
		NewestEvidenceAgeDays: intPtr(1), MaterialChange: "new_query",
		CompatibleExternalTargets: 3, AdditionalOutputTypes: 2,
		EvidenceSources: []EvidenceSource{
			{Class: "search_console", Qualified: true, FirstParty: true, CompleteProvenance: true},
			{Class: "owned_inventory", Qualified: true, FirstParty: true, CompleteProvenance: true},
			{Class: "brave_search", Qualified: true, CompleteProvenance: true},
		},
	}
	score, err := ScoreCandidate(base)
	if err != nil {
		t.Fatal(err)
	}
	if score.Demand != 25 || score.CoverageGap != 20 || score.Relevance != 15 || score.CommercialValue != 15 || score.Freshness != 10 || score.ReusePotential != 10 || score.EvidenceQuality != 5 || score.Final != 100 || score.Disposition != "opportunity" {
		t.Fatalf("score = %+v", score)
	}
	base.Cannibalization = true
	score, _ = ScoreCandidate(base)
	if score.Final != 70 || score.Disposition != "arbitration" {
		t.Fatalf("cannibalization = %+v", score)
	}
}

func TestScoreDispositionBoundariesAndHardGates(t *testing.T) {
	for _, test := range []struct {
		final int
		want  string
	}{{59, "filtered"}, {60, "watchlist"}, {74, "watchlist"}, {75, "opportunity"}} {
		if got := dispositionForScore(test.final); got != test.want {
			t.Errorf("%d => %s", test.final, got)
		}
	}
	base := Snapshot{CapabilityConfirmed: true, AudienceConfirmed: true, IntentSupported: true}
	checks := []struct {
		mutate func(*Snapshot)
		want   string
	}{
		{func(s *Snapshot) { s.ExactDuplicate = true }, "merged"},
		{func(s *Snapshot) { s.NearDuplicate = true }, "near_duplicate"},
		{func(s *Snapshot) { s.LLMOnlyEvidence = true }, "watchlist"},
		{func(s *Snapshot) { s.SensitiveOrUnsupported = true }, "filtered"},
		{func(s *Snapshot) { s.CapabilityConfirmed = false }, "hold"},
		{func(s *Snapshot) { s.DismissedWithoutChange = true }, "dismissed"},
	}
	for _, check := range checks {
		snapshot := base
		check.mutate(&snapshot)
		score, _ := ScoreCandidate(snapshot)
		if score.Disposition != check.want {
			t.Errorf("got %s want %s", score.Disposition, check.want)
		}
	}
}

func TestScoreReplayIgnoresLLMText(t *testing.T) {
	snapshot := Snapshot{CurrentImpressions: 10, CapabilityConfirmed: true, AudienceConfirmed: true, IntentSupported: true, LLMText: "first"}
	first, _ := ScoreCandidate(snapshot)
	snapshot.LLMText = "entirely different model prose"
	second, _ := ScoreCandidate(snapshot)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("LLM text changed deterministic score: %+v %+v", first, second)
	}
}

func intPtr(value int) *int { return &value }
