package growthradar

import (
	"encoding/json"
	"testing"

	"github.com/citeloop/citeloop/internal/growthspec"
	"github.com/citeloop/citeloop/internal/platformcontract"
	"github.com/google/uuid"
)

func TestMaterializeOpportunitySpecCreatesV2OnlyForOpportunityDisposition(t *testing.T) {
	contractID := uuid.New()
	candidate := MaterializationCandidate{
		ProjectID: uuid.New(), ClusterID: "cluster-1", Topic: "ai visibility", Intent: "comparison", JourneyStage: "decision", Audience: "growth leaders", AssetType: "comparison_page",
		Action: "Create comparison", ExpectedUserValue: "Compare verifiable capabilities", Evidence: json.RawMessage(`{"records":["search-1"]}`), SuccessMetric: growthspec.SuccessMetric{Name: "gsc_clicks", WindowDays: 56},
		Target: growthspec.TargetSpec{CanonicalTarget: platformcontract.Target{Platform: "blog", OutputType: "canonical_article", ContractID: contractID, ContractVersion: "v1"}, TargetPlatforms: []platformcontract.Target{{Platform: "blog", OutputType: "canonical_article", ContractID: contractID, ContractVersion: "v1"}}, SelectionMode: "contract_matrix"},
		Score:  Score{FormulaVersion: FormulaVersion, Final: 80, Disposition: "opportunity"}, SourceVersions: map[string]string{"search": "brave-v1"},
	}
	result := MaterializeOpportunitySpec(candidate)
	if result.Disposition != "opportunity" || result.Spec.State != growthspec.StateDecisionReady || result.Spec.Version != growthspec.VersionV2 {
		t.Fatalf("result = %#v", result)
	}
	if result.Spec.Spec.DedupeIdentity == "" {
		t.Fatal("dedupe identity missing")
	}
	candidate.Score.Disposition = "watchlist"
	if held := MaterializeOpportunitySpec(candidate); held.Spec.State == growthspec.StateDecisionReady || held.Disposition != "watchlist" {
		t.Fatalf("watchlist materialized: %#v", held)
	}
}

func TestMaterializeOpportunitySpecPreservesLegacyDerivedExactList(t *testing.T) {
	contractID := uuid.New()
	candidate := MaterializationCandidate{ProjectID: uuid.New(), ClusterID: "c", Topic: "api", Intent: "how_to", JourneyStage: "awareness", Audience: "developers", AssetType: "blog_post", Action: "Write guide", ExpectedUserValue: "Learn workflow", Evidence: json.RawMessage(`{"records":["e1"]}`), SuccessMetric: growthspec.SuccessMetric{Name: "gsc_clicks", WindowDays: 28}, Target: growthspec.TargetSpec{CanonicalTarget: platformcontract.Target{Platform: "blog", OutputType: "canonical_article", ContractID: contractID, ContractVersion: "v1"}, TargetPlatforms: []platformcontract.Target{{Platform: "blog", OutputType: "canonical_article", ContractID: contractID, ContractVersion: "v1"}, {Platform: "dev_to", OutputType: "native_article", ContractID: contractID, ContractVersion: "v1"}}, SelectionMode: "legacy_derived"}, Score: Score{Final: 78, Disposition: "opportunity"}, SourceVersions: map[string]string{"targeting": "legacy-v1"}}
	result := MaterializeOpportunitySpec(candidate)
	if result.Spec.Spec.Targets.SelectionMode != "legacy_derived" || len(result.Spec.Spec.Targets.TargetPlatforms) != 2 {
		t.Fatalf("legacy exact list lost: %#v", result)
	}
}
