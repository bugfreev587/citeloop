package growthradar

import (
	"reflect"
	"testing"

	"github.com/citeloop/citeloop/internal/growthstage"
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

func TestScoreCandidateForStageAppliesProfileAndEvidenceGate(t *testing.T) {
	snapshot := Snapshot{
		CurrentImpressions: 50, QualifiedRecurrence: 1,
		PrimaryCoverage: "none", InternalLinkPaths: 0,
		CapabilityConfirmed: true, AudienceConfirmed: true, IntentSupported: true,
		Intent: "comparison", JourneyStage: "decision",
		NewestEvidenceAgeDays: intPtr(1), MaterialChange: "new_confirmation",
		CompatibleExternalTargets: 3, AdditionalOutputTypes: 1,
		EvidenceSources: []EvidenceSource{
			{Class: "answer_engine_observation", Qualified: true, CompleteProvenance: true},
			{Class: "brave_search", Qualified: true, CompleteProvenance: true},
		},
		IndependentGEOProviders: 2,
	}
	foundation, err := ScoreCandidateForStage(snapshot, growthstage.Foundation)
	if err != nil {
		t.Fatal(err)
	}
	if foundation.Stage != string(growthstage.Foundation) || foundation.StageProfileVersion == "" || foundation.Final < 70 || foundation.Disposition != "opportunity" {
		t.Fatalf("foundation score = %+v", foundation)
	}

	snapshot.IndependentGEOProviders = 1
	snapshot.EvidenceSources = snapshot.EvidenceSources[:1]
	single, err := ScoreCandidateForStage(snapshot, growthstage.Foundation)
	if err != nil {
		t.Fatal(err)
	}
	if single.Disposition == "opportunity" || !containsReason(single.ReasonCodes, "demand.single_geo_provider") {
		t.Fatalf("single-provider score = %+v", single)
	}
}

func TestStageDemandUsesIndependentSEOAndGEOLanes(t *testing.T) {
	tests := []struct {
		providers int
		dates     int
		want      int
	}{
		{providers: 1, dates: 1, want: 2},
		{providers: 2, dates: 1, want: 5},
		{providers: 3, dates: 5, want: 10},
	}
	for _, test := range tests {
		score, err := ScoreCandidateForStage(Snapshot{
			CapabilityConfirmed: true, IndependentGEOProviders: test.providers, GEOObservationDates: test.dates,
		}, growthstage.Traction)
		if err != nil {
			t.Fatal(err)
		}
		if score.RawComponents == nil || score.RawComponents.Demand != test.want {
			t.Errorf("providers=%d dates=%d raw=%+v want=%d", test.providers, test.dates, score.RawComponents, test.want)
		}
	}

	seo, err := ScoreCandidateForStage(Snapshot{CurrentImpressions: 1000, PreviousImpressions: 400, CapabilityConfirmed: true}, growthstage.Traction)
	if err != nil {
		t.Fatal(err)
	}
	if seo.RawComponents == nil || seo.RawComponents.Demand != 15 {
		t.Fatalf("SEO demand = %+v, want 15", seo.RawComponents)
	}
}

func TestOptimizeRequiresMeasuredChangeOnAnExistingAsset(t *testing.T) {
	snapshot := Snapshot{
		CurrentImpressions: 1000, PreviousImpressions: 2000,
		PrimaryCoverage: "covered", InternalLinkPaths: 0,
		CapabilityConfirmed: true, AudienceConfirmed: true, IntentSupported: true,
		Intent: "comparison", JourneyStage: "decision",
		NewestEvidenceAgeDays: intPtr(1), MaterialChange: "decline_over_25",
		SelectedExternalTargets: 2, CompatibleExternalTargets: 2, AdditionalOutputTypes: 2,
		EvidenceSources: []EvidenceSource{
			{Class: "search_console", Qualified: true, FirstParty: true, CompleteProvenance: true},
			{Class: "owned_inventory", Qualified: true, FirstParty: true, CompleteProvenance: true},
			{Class: "search_result", Qualified: true, CompleteProvenance: true},
		},
		HasExistingAsset: true, HasMaterialChangeEvidence: true,
	}
	optimized, err := ScoreCandidateForStage(snapshot, growthstage.Optimize)
	if err != nil || optimized.Disposition != "opportunity" || optimized.Final < 75 {
		t.Fatalf("measured existing-asset decline should optimize: score=%+v err=%v", optimized, err)
	}
	snapshot.HasExistingAsset = false
	withoutAsset, _ := ScoreCandidateForStage(snapshot, growthstage.Optimize)
	if withoutAsset.Disposition == "opportunity" || !containsReason(withoutAsset.ReasonCodes, "stage.optimize_gate") {
		t.Fatalf("net-new asset must not pass Optimize: %+v", withoutAsset)
	}
}

func TestFoundationCreatesWithoutGSCWhenIndependentGEOEvidenceQualifies(t *testing.T) {
	snapshot := Snapshot{
		PrimaryCoverage: "none", InternalLinkPaths: 0, SelectedExternalTargets: 3,
		CapabilityConfirmed: true, AudienceConfirmed: true, IntentSupported: true,
		Intent: "comparison", JourneyStage: "decision", NewestEvidenceAgeDays: intPtr(1), MaterialChange: "new_confirmation",
		CompatibleExternalTargets: 3, AdditionalOutputTypes: 2, IndependentGEOProviders: 2, GEOObservationDates: 2,
		EvidenceSources: []EvidenceSource{
			{Class: "answer_engine_observation", Qualified: true, CompleteProvenance: true, SupportedClaim: "absence"},
			{Class: "answer_engine_observation", Qualified: true, CompleteProvenance: true, SupportedClaim: "absence"},
		},
	}
	score, err := ScoreCandidateForStage(snapshot, growthstage.Foundation)
	if err != nil || score.Disposition != "opportunity" || snapshot.CurrentImpressions != 0 {
		t.Fatalf("Foundation independent-GEO fixture = %+v err=%v", score, err)
	}
}

func TestFoundationCreatesFromCorroboratedAnswerAndSearchEvidence(t *testing.T) {
	age := 0
	snapshot := Snapshot{
		Stage: string(growthstage.Foundation), CurrentImpressions: 0, PreviousImpressions: 0,
		QualifiedRecurrence: 1, PrimaryCoverage: "none", InternalLinkPaths: 0,
		CapabilityConfirmed: true, AudienceConfirmed: true, IntentSupported: true,
		Intent: "use_case", JourneyStage: "consideration", NewestEvidenceAgeDays: &age,
		SelectedExternalTargets: 1, CompatibleExternalTargets: 1,
		IndependentGEOProviders: 1, GEOObservationDates: 1,
		EvidenceSources: []EvidenceSource{
			{Class: "answer_engine_observation", Qualified: true, CompleteProvenance: true, SupportedClaim: "absence"},
			{Class: "search_result", Qualified: true, CompleteProvenance: true},
		},
	}

	score, err := ScoreCandidateForStage(snapshot, growthstage.Foundation)
	if err != nil {
		t.Fatal(err)
	}
	if score.Disposition != "opportunity" || score.Final < 70 {
		t.Fatalf("corroborated Foundation evidence = %+v, want opportunity at threshold", score)
	}
}

func TestTractionCreatesFromObservedDemandAndIndependentEvidence(t *testing.T) {
	snapshot := Snapshot{
		CurrentImpressions: 1000, PreviousImpressions: 400,
		PrimaryCoverage: "none", InternalLinkPaths: 0, SelectedExternalTargets: 2,
		CapabilityConfirmed: true, AudienceConfirmed: true, IntentSupported: true,
		Intent: "comparison", JourneyStage: "decision", NewestEvidenceAgeDays: intPtr(1), MaterialChange: "growth_over_100",
		CompatibleExternalTargets: 2, AdditionalOutputTypes: 2,
		EvidenceSources: []EvidenceSource{
			{Class: "search_console", Qualified: true, FirstParty: true, CompleteProvenance: true},
			{Class: "owned_inventory", Qualified: true, FirstParty: true, CompleteProvenance: true},
		},
	}
	score, err := ScoreCandidateForStage(snapshot, growthstage.Traction)
	if err != nil || score.Disposition != "opportunity" || score.Final < 75 {
		t.Fatalf("Traction demand fixture = %+v err=%v", score, err)
	}
}

func containsReason(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func intPtr(value int) *int { return &value }
