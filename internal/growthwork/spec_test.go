package growthwork

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/citeloop/citeloop/internal/growthspec"
	"github.com/citeloop/citeloop/internal/platformcontract"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestWithGrowthSpecificationUsesEmbeddedOpportunitySpecV2(t *testing.T) {
	contractID := uuid.New()
	input := growthspec.V2Input{Intent: "comparison", JourneyStage: "decision", Audience: []string{"growth leaders"}, TopicClusterID: "cluster-1", NormalizedTopic: "ai visibility", AssetType: "comparison_page", RecommendedAction: "Create comparison", ExpectedUserValue: "Choose using evidence", Target: growthspec.TargetSpec{CanonicalTarget: platformcontract.Target{Platform: "blog", OutputType: "canonical_article", ContractID: contractID, ContractVersion: "v1"}, TargetPlatforms: []platformcontract.Target{{Platform: "blog", OutputType: "canonical_article", ContractID: contractID, ContractVersion: "v1"}}, SelectionMode: "contract_matrix"}, Evidence: json.RawMessage(`{"records":["e1"]}`), SuccessMetric: growthspec.SuccessMetric{Name: "gsc_clicks", WindowDays: 56}, DedupeIdentity: "d1", Score: json.RawMessage(`{"final":80}`), SourceVersions: map[string]string{"search": "brave-v1"}}
	encoded, err := json.Marshal(map[string]any{"opportunity_spec_v2": input})
	if err != nil {
		t.Fatal(err)
	}
	params, result, err := withGrowthSpecification(db.CreateCanonicalGrowthOpportunityParams{ProjectID: uuid.New(), Type: "comparison_page", Evidence: encoded}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != growthspec.VersionV2 || params.GrowthSpecVersion != growthspec.VersionV2 || params.GrowthSpecState != growthspec.StateDecisionReady {
		t.Fatalf("v2 spec not selected: %#v %#v", result, params)
	}
}

func TestWithGrowthSpecificationProducesDecisionReadyWriterParams(t *testing.T) {
	action := "Rewrite the search snippet"
	impact := "Capture more existing impressions"
	query := "best ai visibility platform"
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	params, result, err := withGrowthSpecification(db.CreateCanonicalGrowthOpportunityParams{
		ID: uuid.New(), ProjectID: uuid.New(), Type: "gsc_low_ctr_query", Query: &query,
		RecommendedAction: &action, ExpectedImpact: &impact,
		NormalizedPageUrl: "https://example.com/ai-visibility",
		Evidence: json.RawMessage(`{
          "source":"gsc_search_analytics","clicks_28d":12,"impressions_28d":1200,
          "ctr_28d":0.01,"window_start":"2026-06-01","window_end":"2026-06-28"
        }`),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != growthspec.StateDecisionReady || params.GrowthSpecState != growthspec.StateDecisionReady {
		t.Fatalf("result=%#v params=%#v", result, params)
	}
	if params.GrowthSpecVersion != growthspec.VersionV1 || !params.DecisionReadyAt.Valid || !params.DecisionReadyAt.Time.Equal(now) {
		t.Fatalf("version/readiness = %q %#v", params.GrowthSpecVersion, params.DecisionReadyAt)
	}
	var spec growthspec.Spec
	if err := json.Unmarshal(params.GrowthSpec, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.PrimaryMetric != "gsc_ctr" {
		t.Fatalf("primary metric = %q", spec.PrimaryMetric)
	}
	if string(params.GrowthSpecMissing) != "[]" {
		t.Fatalf("missing = %s", params.GrowthSpecMissing)
	}
	candidate, identity, _, err := projectGrowthCandidate(params)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Status != discovery.StatusIdentityReady || candidate.PrimarySuccessMetric != "gsc_ctr" || len(candidate.AudienceIdentity) == 0 {
		t.Fatalf("candidate = %#v", candidate)
	}
	if identity.ExactSignatureHash == "" {
		t.Fatal("decision-ready candidate must have an enforceable identity")
	}
}

func TestProjectGrowthCandidateKeepsIncompleteWorkInInternalHold(t *testing.T) {
	action := "Create a comparison page"
	query := "alpha vs beta"
	params, result, err := withGrowthSpecification(db.CreateCanonicalGrowthOpportunityParams{
		ID: uuid.New(), ProjectID: uuid.New(), Type: "comparison_page", Query: &query,
		RecommendedAction: &action, Evidence: json.RawMessage(`{"source":"context"}`),
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if result.State != growthspec.StateNeedsSpecification {
		t.Fatalf("state = %q", result.State)
	}
	candidate, identity, _, err := projectGrowthCandidate(params)
	if err != nil {
		t.Fatalf("incomplete candidate must persist as a hold, got %v", err)
	}
	if candidate.Status != discovery.StatusNeedsSpecification || candidate.HoldReason == "" {
		t.Fatalf("candidate = %#v", candidate)
	}
	if identity.ExactSignatureHash != "" {
		t.Fatalf("held candidate unexpectedly has an enforced identity: %#v", identity)
	}
}

func TestIncompleteForwardCandidateHoldIsNonFatalToTheFindingRun(t *testing.T) {
	for _, status := range []discovery.CandidateStatus{discovery.StatusNeedsSpecification, discovery.StatusNeedsEvidence} {
		if !isInternalGrowthHold(discovery.Candidate{Status: status}, discovery.PreparedDecision{Decision: discovery.DecisionHold}) {
			t.Fatalf("status %q was not recognized as an internal hold", status)
		}
	}
	if isInternalGrowthHold(discovery.Candidate{Status: discovery.StatusNeedsArbitration}, discovery.PreparedDecision{Decision: discovery.DecisionHold}) {
		t.Fatal("arbitration review must remain an explicit failure/hold")
	}
}

func TestSemanticReviewHoldIsCandidateScoped(t *testing.T) {
	prepared := discovery.PreparedDecision{
		Decision:    discovery.DecisionHold,
		Disposition: discovery.DispositionSemanticComparison,
		Reason:      "semantic suppression is disabled until the launch evaluation gate passes",
	}
	err := growthArbitrationOutcomeError(prepared)
	if !errors.Is(err, ErrGrowthHeld) {
		t.Fatalf("error must retain Growth hold identity: %v", err)
	}
	if !errors.Is(err, discovery.ErrCandidateReviewRequired) {
		t.Fatalf("semantic review hold must be candidate-scoped: %v", err)
	}

	providerFailure := growthArbitrationOutcomeError(discovery.PreparedDecision{
		Decision:    discovery.DecisionHold,
		Disposition: discovery.DispositionProviderFailure,
		Reason:      "provider unavailable",
	})
	if errors.Is(providerFailure, discovery.ErrCandidateReviewRequired) {
		t.Fatalf("provider failure must remain fatal to the Finding stage: %v", providerFailure)
	}
}

func TestResolveLegacyGrowthIntendedTargetUsesExecutedArtifact(t *testing.T) {
	target := "https://unipost.dev/"
	row := db.GetLegacyGrowthIntendedTargetRow{
		ActionTargetUrl: &target,
		SeoCanonicalUrl: "/unified-social-media-posting-api-comparison-alternatives",
		SeoSlug:         "fallback-slug",
	}
	if got := resolveLegacyGrowthIntendedTarget(row); got != "https://unipost.dev/unified-social-media-posting-api-comparison-alternatives" {
		t.Fatalf("target = %q", got)
	}

	row.SeoCanonicalUrl = ""
	row.SeoSlug = "context-backed-use-case-pages"
	if got := resolveLegacyGrowthIntendedTarget(row); got != "https://unipost.dev/context-backed-use-case-pages" {
		t.Fatalf("slug target = %q", got)
	}
}

func TestEnrichLegacyGrowthEvidencePreservesOriginalAndAddsRecoveredTarget(t *testing.T) {
	raw, err := enrichLegacyGrowthEvidence(json.RawMessage(`{"source":"context_confirmation"}`), "https://unipost.dev/use-cases")
	if err != nil {
		t.Fatal(err)
	}
	var evidence map[string]any
	if err := json.Unmarshal(raw, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence["source"] != "context_confirmation" || evidence["intended_slug_or_canonical"] != "https://unipost.dev/use-cases" {
		t.Fatalf("evidence = %#v", evidence)
	}
	if evidence["intended_target_provenance"] != "legacy_execution_artifact" {
		t.Fatalf("missing provenance: %#v", evidence)
	}
}

func TestLegacyGrowthTargetSnapshotDetectsExecutionChainChange(t *testing.T) {
	row := db.GetLegacyGrowthIntendedTargetRow{
		ActionID: uuid.New(), ArticleID: uuid.New(),
		ActionUpdatedAt: pgtype.Timestamptz{Time: time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC), Valid: true},
		SeoSlug:         "comparison-page",
	}
	before := getLegacyGrowthTargetSnapshot(row)
	row.ActionUpdatedAt.Time = row.ActionUpdatedAt.Time.Add(time.Minute)
	if after := getLegacyGrowthTargetSnapshot(row); after == before {
		t.Fatal("execution-chain change must invalidate the prepared legacy target snapshot")
	}
}
