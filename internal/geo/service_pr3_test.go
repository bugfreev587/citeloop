package geo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/crawl"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/citeloop/citeloop/internal/growthradar"
	"github.com/citeloop/citeloop/internal/growthstage"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCandidateReviewHoldIsNonFatalToFindingBatch(t *testing.T) {
	err := fmt.Errorf("canonical writer: %w", discovery.ErrCandidateReviewRequired)
	if !isCandidateReviewHold(err) {
		t.Fatalf("candidate review hold was not recognized: %v", err)
	}
	if isCandidateReviewHold(errors.New("provider unavailable")) {
		t.Fatal("provider failure must not be treated as a candidate review hold")
	}
}

func TestFoundationStarterCandidateRequiresExplicitCreatePermission(t *testing.T) {
	starter := GrowthRadarCandidate{Disposition: "starter_opportunity"}
	if canCreateGrowthRadarCandidate(starter, AnalyzeObservationsRequest{}) {
		t.Fatalf("starter_opportunity must not create without AllowFoundationStarters")
	}
	if !canCreateGrowthRadarCandidate(starter, AnalyzeObservationsRequest{AllowFoundationStarters: true}) {
		t.Fatalf("starter_opportunity should create when AllowFoundationStarters is true")
	}
	if !canCreateGrowthRadarCandidate(GrowthRadarCandidate{Disposition: "opportunity"}, AnalyzeObservationsRequest{}) {
		t.Fatalf("normal opportunity should remain creatable")
	}
}

func TestPrepareFoundationStarterGapMarksEvidenceAndCopy(t *testing.T) {
	gap := geoGap{
		Action:   "Publish a comparison page",
		Impact:   "Capture early demand.",
		Evidence: map[string]any{},
	}
	candidate := GrowthRadarCandidate{
		Disposition: "starter_opportunity",
		Reason:      "Foundation starter: single-provider demand",
		Score:       growthradar.Score{ReasonCodes: []string{"foundation.starter_single_provider", "demand.single_geo_provider"}},
	}

	prepareFoundationStarterGap(&gap, candidate)

	if gap.Evidence["foundation_starter"] != true {
		t.Fatalf("foundation_starter evidence = %#v, want true", gap.Evidence["foundation_starter"])
	}
	if !strings.Contains(gap.Action, "Foundation starter") {
		t.Fatalf("action = %q, want Foundation starter copy", gap.Action)
	}
	if !strings.Contains(gap.Impact, "first citable evidence base") {
		t.Fatalf("impact = %q, want evidence-building copy", gap.Impact)
	}
}

func TestAnalyzeObservationsCreatesIdempotentGEOOpportunitiesAndBriefs(t *testing.T) {
	projectID := uuid.New()
	runID := uuid.New()
	promptID := uuid.New()
	store := &geoStoreStub{
		runID: runID,
		growthLearnings: []db.ListApplicableGrowthLearningsRow{{
			ID: uuid.New(), ScoringEligible: true, ActionFamily: "geo_project_mentioned_without_citation",
			PrimaryMetric: "ai_citation_count", OutcomeLabel: "positive",
			TargetIdentity: json.RawMessage(`{"query":"best social scheduling tools"}`),
			Audience:       json.RawMessage(`["people searching for best social scheduling tools"]`),
		}},
		prompts: []db.GeoPrompt{{
			ID:          promptID,
			ProjectID:   projectID,
			PromptText:  "best social scheduling tools",
			IntentType:  "category_recommendation",
			TargetTopic: "social scheduling",
			Status:      "active",
			Priority:    8,
		}},
		observations: []db.GeoObservation{
			{
				ID:                   uuid.New(),
				ProjectID:            projectID,
				PromptID:             pgUUID(promptID),
				Engine:               "OpenAI",
				SourceType:           ProviderManualFixture,
				BrandMentioned:       false,
				ProjectCitationCount: 0,
				CompetitorCitations:  json.RawMessage(`["Buffer"]`),
				CitedUrls:            json.RawMessage(`["https://buffer.com/resources"]`),
				ObservationState:     "observed",
				Confidence:           ConfidenceMedium,
			},
			{
				ID:                   uuid.New(),
				ProjectID:            projectID,
				PromptID:             pgUUID(promptID),
				Engine:               "Perplexity",
				SourceType:           ProviderManualFixture,
				BrandMentioned:       true,
				ProjectCitationCount: 0,
				CompetitorCitations:  json.RawMessage(`[]`),
				CitedUrls:            json.RawMessage(`[]`),
				ObservationState:     "observed",
				Confidence:           ConfidenceMedium,
			},
		},
	}

	service := Service{Q: store, Now: func() time.Time { return time.Date(2026, 6, 8, 14, 0, 0, 0, time.UTC) }}
	result, err := service.AnalyzeObservations(context.Background(), projectID, AnalyzeObservationsRequest{Limit: 50})
	if err != nil {
		t.Fatalf("AnalyzeObservations error: %v", err)
	}
	if len(result.Opportunities) != 2 || len(result.AssetBriefs) != 2 {
		t.Fatalf("opportunities=%d briefs=%d, want 2 and 2", len(result.Opportunities), len(result.AssetBriefs))
	}
	again, err := service.AnalyzeObservations(context.Background(), projectID, AnalyzeObservationsRequest{Limit: 50})
	if err != nil {
		t.Fatalf("AnalyzeObservations second run error: %v", err)
	}
	if len(store.opportunities) != 2 || len(store.assetBriefs) != 2 {
		t.Fatalf("after rerun store opportunities=%d briefs=%d, want idempotent 2 and 2; second result %+v", len(store.opportunities), len(store.assetBriefs), again)
	}
	if !hasOpportunityType(result.Opportunities, "geo_competitor_cited_project_absent") {
		t.Fatalf("missing competitor gap opportunity: %+v", result.Opportunities)
	}
	if !hasOpportunityType(result.Opportunities, "geo_project_mentioned_without_citation") {
		t.Fatalf("missing mention without citation opportunity: %+v", result.Opportunities)
	}
	for _, opportunity := range result.Opportunities {
		var evidence map[string]any
		if err := json.Unmarshal(opportunity.Evidence, &evidence); err != nil {
			t.Fatalf("unmarshal GEO opportunity evidence: %v; raw=%s", err, string(opportunity.Evidence))
		}
		for _, key := range []string{"source", "why_now", "scoring_method", "scoring_version", "idempotency_key"} {
			if evidence[key] == nil || evidence[key] == "" {
				t.Fatalf("%s evidence missing %q in %#v", opportunity.Type, key, evidence)
			}
		}
		if opportunity.Type == "geo_project_mentioned_without_citation" {
			if math.Abs(pgutil.Float(opportunity.PriorityScore)-81) > 0.000001 || evidence["learning_scoring"] == nil {
				t.Fatalf("GEO opportunity missing applied learning score: %+v evidence=%#v", opportunity, evidence)
			}
		}
	}
}

func TestGrowthRadarGapUsesDeterministicScoreAndExactContractTarget(t *testing.T) {
	projectID, contractID, devtoContractID := uuid.New(), uuid.New(), uuid.New()
	store := &geoStoreStub{
		profile:        db.ProductProfile{Profile: json.RawMessage(`{"features":["social scheduling"],"icp":["growth leaders"]}`)},
		demandSnapshot: db.GetGrowthRadarDemandSnapshotRow{CurrentImpressions: 1000, PreviousImpressions: 400},
		searchEvidence: 1,
		platformContracts: []db.PlatformContentContract{{
			ID: contractID, Platform: "blog", Version: "v1", Status: "active", GenerationSupported: true,
			AllowedOutputTypes: json.RawMessage(`["long_form_article"]`), CompatibleAssetTypes: json.RawMessage(`["comparison_page"]`), RequiredContextFields: json.RawMessage(`[]`),
		}, {
			ID: devtoContractID, Platform: "dev_to", Version: "v1", Status: "active", GenerationSupported: true,
			AllowedOutputTypes: json.RawMessage(`["devto_markdown"]`), CompatibleAssetTypes: json.RawMessage(`["comparison_page"]`), RequiredContextFields: json.RawMessage(`[]`),
		}},
		publisherConnections: []db.PublisherConnection{{ProjectID: projectID, Kind: "dev_to", Status: "connected", Enabled: true, IsDefault: true}},
	}
	service := Service{Q: store, Now: func() time.Time { return time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC) }}
	candidate, materialized, err := service.scoreGrowthRadarGap(context.Background(), projectID, geoGap{
		Type: "geo_competitor_cited_project_absent", AssetType: "comparison_page", Action: "create comparison page",
		Impact: "Compare verifiable capabilities", Evidence: map[string]any{
			"observation_id": uuid.New(), "source_type": SourceTypeAnswerEngine, "observation_state": "observed", "observed_at": "2026-07-13T12:00:00Z",
			"competitor_citations": []any{"https://competitor.example/guide"},
		}, PromptText: "best social scheduling tools", TargetTopic: "social scheduling",
		Intent: "comparison", Audience: "growth leaders", Recurrence: 5,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Score.Disposition != "opportunity" || candidate.Score.FormulaVersion != growthradar.StageFormulaVersion || candidate.Score.Stage != "foundation" {
		t.Fatalf("candidate score = %+v", candidate.Score)
	}
	if candidate.Score.ReusePotential != 4 || candidate.Snapshot.SelectedExternalTargets != 1 || candidate.Snapshot.CompatibleExternalTargets != 1 || candidate.Snapshot.AdditionalOutputTypes != 1 {
		t.Fatalf("configured Dev.to target must receive exact reuse points: score=%+v snapshot=%+v", candidate.Score, candidate.Snapshot)
	}
	if materialized.Spec.Version != "growth-opportunity-v2" || materialized.Spec.Spec.Targets.CanonicalTarget.ContractID != contractID || len(materialized.Spec.Spec.Targets.TargetPlatforms) != 2 {
		t.Fatalf("materialized spec = %+v", materialized)
	}

	// Scale treats an existing successful canonical asset plus real contract
	// targets as expansion work, not as a duplicate canonical article.
	hashnodeContractID := uuid.New()
	store.platformContracts = append(store.platformContracts, db.PlatformContentContract{
		ID: hashnodeContractID, Platform: "hashnode", Version: "v1", Status: "active", GenerationSupported: true,
		AllowedOutputTypes: json.RawMessage(`["hashnode_markdown"]`), CompatibleAssetTypes: json.RawMessage(`["comparison_page"]`), RequiredContextFields: json.RawMessage(`["publication"]`),
	})
	store.platformTargetContexts = []db.PlatformTargetContext{{
		ID: uuid.New(), ProjectID: projectID, Platform: "hashnode", TargetKey: "publication", Version: 1, Status: "confirmed",
		ExpiresAt: pgtype.Timestamptz{Time: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Valid: true},
	}}
	store.growthStageSetting = db.GrowthStageSetting{ProjectID: projectID, Stage: "scale", StageProfileVersion: growthstage.ProfileVersion, SettingVersion: 2}
	stageGap := geoGap{
		Type: "geo_competitor_cited_project_absent", AssetType: "comparison_page", Action: "create comparison page", Impact: "Compare verifiable capabilities",
		Evidence:   map[string]any{"observation_id": uuid.New(), "source_type": SourceTypeAnswerEngine, "observation_state": "observed", "observed_at": "2026-07-13T12:00:00Z", "competitor_citations": []any{"https://competitor.example/guide"}},
		PromptText: "best social scheduling tools", TargetTopic: "social scheduling", Intent: "comparison", Audience: "growth leaders", Recurrence: 5, IndependentProviders: 2, ObservationDates: 3,
	}
	scaled, scalePlan, err := service.scoreGrowthRadarGap(context.Background(), projectID, stageGap, []db.Topic{{Title: "social scheduling", Status: "published"}})
	if err != nil || scaled.Disposition != "opportunity" || scalePlan.Input.RecommendedAction != "expand existing asset with contract-native variants" {
		t.Fatalf("Scale must produce real target expansion: candidate=%+v plan=%+v err=%v", scaled, scalePlan.Input, err)
	}

	// Optimize reuses the same existing asset but responds to a measured decline
	// with refresh work rather than a redundant new canonical asset.
	store.growthStageSetting = db.GrowthStageSetting{ProjectID: projectID, Stage: "optimize", StageProfileVersion: growthstage.ProfileVersion, SettingVersion: 3}
	store.demandSnapshot = db.GetGrowthRadarDemandSnapshotRow{CurrentImpressions: 1000, PreviousImpressions: 2000}
	optimized, optimizePlan, err := service.scoreGrowthRadarGap(context.Background(), projectID, stageGap, []db.Topic{{Title: "social scheduling", Status: "published"}})
	if err != nil || optimized.Disposition != "opportunity" || optimizePlan.Input.RecommendedAction != "refresh existing asset for measured change" {
		t.Fatalf("Optimize must produce measured refresh work: candidate=%+v plan=%+v err=%v", optimized, optimizePlan.Input, err)
	}

	store.demandSnapshot = db.GetGrowthRadarDemandSnapshotRow{}
	store.searchEvidence = 0
	store.growthStageSetting = db.GrowthStageSetting{}
	held, _, err := service.scoreGrowthRadarGap(context.Background(), projectID, geoGap{Type: "geo_competitor_cited_project_absent", AssetType: "comparison_page", Action: "create", Impact: "value", Evidence: map[string]any{"observation_id": uuid.New()}, PromptText: "unknown", TargetTopic: "unknown", Intent: "comparison", Audience: "unknown audience", Recurrence: 1}, nil)
	if err != nil || held.Disposition != "hold" || held.Snapshot.CapabilityConfirmed || held.Snapshot.AudienceConfirmed {
		t.Fatalf("unconfirmed capability/audience candidate must be held: %+v err=%v", held, err)
	}
	store.demandSnapshot = db.GetGrowthRadarDemandSnapshotRow{CurrentImpressions: 1000, PreviousImpressions: 400}
	store.searchEvidence = 1
	filtered, _, err := service.scoreGrowthRadarGap(context.Background(), projectID, geoGap{Type: "geo_competitor_cited_project_absent", AssetType: "comparison_page", Action: "create", Impact: "value", Evidence: map[string]any{"observation_id": uuid.New()}, PromptText: "DATABASE_PASSWORD=hunter2-production", TargetTopic: "production credential", Intent: "comparison", Audience: "growth leaders", Recurrence: 5}, nil)
	if err != nil || filtered.Disposition != "filtered" {
		t.Fatalf("sensitive candidate must be filtered: %+v err=%v", filtered, err)
	}
}

func TestGrowthRadarGapConfirmsEvidenceBackedPublicTopic(t *testing.T) {
	projectID := uuid.New()
	store := &geoStoreStub{
		profile: db.ProductProfile{
			ProjectID: projectID,
			Profile: json.RawMessage(`{
				"positioning":"UniPost is a social publishing API.",
				"features":["multi-platform scheduling"],
				"icp":["developers"],
				"competitors":["Ayrshare"]
			}`),
		},
		demandSnapshot: db.GetGrowthRadarDemandSnapshotRow{CurrentImpressions: 1000, PreviousImpressions: 400},
		searchEvidence: 1,
	}
	now := time.Date(2026, 7, 14, 13, 0, 0, 0, time.UTC)
	service := Service{Q: store, Now: func() time.Time { return now }}

	candidate, _, err := service.scoreGrowthRadarGap(context.Background(), projectID, geoGap{
		Type:      "geo_competitor_cited_project_absent",
		AssetType: "comparison_page",
		Action:    "create comparison page",
		Impact:    "Compare verifiable capabilities",
		Evidence: map[string]any{
			"observation_id":         uuid.New(),
			"source_type":            SourceTypeAnswerEngine,
			"observation_state":      "observed",
			"engine":                 "Perplexity",
			"observed_at":            now.Format(time.RFC3339),
			"answer_hash":            "answer-1",
			"competitor_citations":   []any{"https://postsyncer.com/tools"},
			"project_citation_count": int32(0),
		},
		PromptText:  "best free social content tools",
		TargetTopic: "free social content tools",
		Intent:      "comparison",
		Audience:    "developers",
		Recurrence:  5,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !candidate.Snapshot.CapabilityConfirmed || candidate.Disposition == "hold" || containsReason(candidate.Score.ReasonCodes, "context.capability_unconfirmed") {
		t.Fatalf("evidence-backed target topic should confirm capability instead of holding: candidate=%+v snapshot=%+v", candidate, candidate.Snapshot)
	}
}

func TestGapsForObservationScoresObservedBrandAbsence(t *testing.T) {
	promptID := uuid.New()
	gaps := gapsForObservation(db.GeoObservation{
		ID:                   uuid.New(),
		PromptID:             pgUUID(promptID),
		SourceType:           SourceTypeAnswerEngine,
		ObservationState:     "observed",
		BrandMentioned:       false,
		ProjectCitationCount: 0,
		CompetitorCitations:  json.RawMessage(`null`),
		CitedUrls:            json.RawMessage(`[]`),
		Confidence:           ConfidenceMedium,
	}, map[uuid.UUID]db.GeoPrompt{promptID: {
		ID: promptID, PromptText: "best audit history tools", TargetTopic: "audit history",
		IntentType: "category_recommendation", TargetPersona: "developers",
	}})
	if len(gaps) != 1 || gaps[0].Type != "geo_project_absent_from_answer" {
		t.Fatalf("gaps = %+v, want one brand-absence candidate", gaps)
	}
	if gaps[0].PromptText != "best audit history tools" || gaps[0].TargetTopic != "audit history" {
		t.Fatalf("gap did not preserve prompt context: %+v", gaps[0])
	}

	// Provider failures are not observations and must never become candidates.
	failed := db.GeoObservation{PromptID: pgUUID(promptID), ObservationState: "provider_unavailable", CompetitorCitations: json.RawMessage(`null`)}
	if got := gapsForObservation(failed, map[uuid.UUID]db.GeoPrompt{promptID: {ID: promptID}}); len(got) != 0 {
		t.Fatalf("provider failure produced candidates: %+v", got)
	}
}

func TestCompetitiveSeedReportCreatesToolsHubGap(t *testing.T) {
	gaps := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{{
		URL: "https://postsyncer.com/tools", CanonicalURL: "https://postsyncer.com/tools",
		Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
		SameArchetypeLinkCount: 120,
		Archetypes:             []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
		Signals:                []string{"sitemap_included", "many_same_archetype_links", "free_tools_language"},
	}})
	if len(gaps) != 1 {
		t.Fatalf("gaps = %+v, want one competitive seed gap", gaps)
	}
	gap := gaps[0]
	if gap.Type != "competitive_tools_hub_gap" || gap.AssetType != "source_backed_evidence_page" || gap.Intent != "category_recommendation" {
		t.Fatalf("gap identity = %+v", gap)
	}
	if gap.TargetTopic != "social publishing tools" || gap.PromptText != "best social publishing tools" {
		t.Fatalf("gap target = %+v", gap)
	}
	if gap.Evidence["source"] != "competitive_seed_url" || gap.Evidence["archetype"] != "tools_hub" || gap.Evidence["seed_url"] != "https://postsyncer.com/tools" || gap.Evidence["competitor_domain"] != "postsyncer.com" {
		t.Fatalf("gap evidence = %#v", gap.Evidence)
	}

	rejected := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{{
		URL: "https://postsyncer.com/tools", Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
		Archetypes: []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "medium"}},
	}})
	if len(rejected) != 0 {
		t.Fatalf("non-high confidence seed should not create gaps: %+v", rejected)
	}
}

func TestCompetitiveSeedReportCreatesResourceHubGap(t *testing.T) {
	gaps := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{{
		URL: "https://buffer.com/resources", CanonicalURL: "https://buffer.com/resources",
		Host: "buffer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
		SameArchetypeLinkCount: 80,
		Archetypes:             []crawl.SeedURLArchetype{{Archetype: "resources_hub", Confidence: "high"}},
		Signals:                []string{"sitemap_included", "resource_hub_language"},
	}})
	if len(gaps) != 1 {
		t.Fatalf("gaps = %+v, want one resource hub competitive seed gap", gaps)
	}
	gap := gaps[0]
	if gap.Type != "competitive_resources_hub_gap" || gap.AssetType != "source_backed_evidence_page" || gap.Intent != "category_recommendation" {
		t.Fatalf("gap identity = %+v", gap)
	}
	if gap.TargetTopic != "social publishing resources" || gap.PromptText != "best social publishing resources" {
		t.Fatalf("gap target = %+v", gap)
	}
	if gap.Evidence["source"] != "competitive_seed_url" || gap.Evidence["archetype"] != "resources_hub" || gap.Evidence["seed_url"] != "https://buffer.com/resources" || gap.Evidence["competitor_domain"] != "buffer.com" {
		t.Fatalf("gap evidence = %#v", gap.Evidence)
	}
}

func TestCompetitiveSeedProbeIntentShapesResourceHubTopic(t *testing.T) {
	gaps := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{{
		URL: "https://buffer.com/templates/social-media-calendar", CanonicalURL: "https://buffer.com/templates/social-media-calendar",
		Host: "buffer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
		DiscoverySource: "topic_path_probe", ProbeIntent: "templates",
		SameArchetypeLinkCount: 35,
		Archetypes:             []crawl.SeedURLArchetype{{Archetype: "resources_hub", Confidence: "high"}},
		Signals:                []string{"sitemap_included", "resource_hub_language"},
	}})
	if len(gaps) != 1 {
		t.Fatalf("gaps = %+v, want one template-shaped resource hub gap", gaps)
	}
	gap := gaps[0]
	if gap.Type != "competitive_resources_hub_gap" || gap.Intent != "template" {
		t.Fatalf("gap identity = %+v, want resource hub gap shaped by template intent", gap)
	}
	if gap.TargetTopic != "social media calendar template" || gap.PromptText != "social media calendar template" {
		t.Fatalf("gap target = %+v, want template-specific topic", gap)
	}
	if gap.Evidence["probe_intent"] != "templates" || gap.Evidence["target_topic_source"] != "seed_url_path" {
		t.Fatalf("gap evidence = %#v, want topic probe intent and URL-derived topic", gap.Evidence)
	}
}

func TestCompetitiveSeedReportDerivesSpecificTopicFromSeedURLPath(t *testing.T) {
	gaps := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{{
		URL: "https://postsyncer.com/tools/social-media-caption-generator", CanonicalURL: "https://postsyncer.com/tools/social-media-caption-generator",
		Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
		SameArchetypeLinkCount: 120,
		Archetypes:             []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
		Signals:                []string{"sitemap_included", "many_same_archetype_links", "free_tools_language"},
	}})
	if len(gaps) != 1 {
		t.Fatalf("gaps = %+v, want one competitive seed gap", gaps)
	}
	if gaps[0].TargetTopic != "social media caption generator" || gaps[0].PromptText != "best social media caption generator tools" {
		t.Fatalf("gap target = %+v, want URL-derived caption generator topic", gaps[0])
	}
	if gaps[0].Evidence["target_topic_source"] != "seed_url_path" {
		t.Fatalf("gap evidence = %#v, want target topic source", gaps[0].Evidence)
	}
}

func TestCompetitiveSeedReportsCollapseDuplicateDerivedTopics(t *testing.T) {
	gaps := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{
		{
			URL: "https://postsyncer.com/tools/social-media-caption-generator", CanonicalURL: "https://postsyncer.com/tools/social-media-caption-generator",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			DiscoverySource:        "site_discovery",
			SameArchetypeLinkCount: 120,
			Archetypes:             []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
			Signals:                []string{"sitemap_included", "many_same_archetype_links", "free_tools_language"},
		},
		{
			URL: "https://postsyncer.com/free-tools", CanonicalURL: "https://postsyncer.com/free-tools",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			Title:                  "Free Social Media Caption Generator | PostSyncer",
			DiscoverySource:        "site_discovery",
			SameArchetypeLinkCount: 80,
			Archetypes:             []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
			Signals:                []string{"free_tools_language"},
		},
	})
	if len(gaps) != 1 {
		t.Fatalf("gaps = %+v, want duplicate derived topic collapsed into one gap", gaps)
	}
	gap := gaps[0]
	if gap.TargetTopic != "social media caption generator" || gap.PromptText != "best social media caption generator tools" {
		t.Fatalf("gap target = %+v, want shared derived caption generator topic", gap)
	}
	if gap.Evidence["competitive_seed_url_count"] != int32(2) {
		t.Fatalf("gap evidence = %#v, want two competitive seed URLs", gap.Evidence)
	}
	samples, ok := gap.Evidence["seed_url_samples"].([]string)
	if !ok || !slices.Contains(samples, "https://postsyncer.com/tools/social-media-caption-generator") || !slices.Contains(samples, "https://postsyncer.com/free-tools") {
		t.Fatalf("seed_url_samples = %#v, want both source URLs", gap.Evidence["seed_url_samples"])
	}
	if gap.Evidence["idempotency_key"] != "competitive_seed_topic|tools_hub|social media caption generator" {
		t.Fatalf("idempotency_key = %#v, want topic-level idempotency", gap.Evidence["idempotency_key"])
	}
	if gap.Priority <= 84 || gap.Confidence <= 0.82 {
		t.Fatalf("gap score priority=%v confidence=%v, want duplicate evidence boost", gap.Priority, gap.Confidence)
	}
}

func TestCompetitiveSeedReportsBoostTopicsSupportedByMultipleCompetitorDomains(t *testing.T) {
	gaps := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{
		{
			URL: "https://postsyncer.com/tools/social-media-caption-generator", CanonicalURL: "https://postsyncer.com/tools/social-media-caption-generator",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			DiscoverySource:        "site_discovery",
			SameArchetypeLinkCount: 120,
			Archetypes:             []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
			Signals:                []string{"sitemap_included", "many_same_archetype_links", "free_tools_language"},
		},
		{
			URL: "https://socialbu.com/tools/social-media-caption-generator", CanonicalURL: "https://socialbu.com/tools/social-media-caption-generator",
			Host: "socialbu.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			DiscoverySource:        "site_discovery",
			SameArchetypeLinkCount: 95,
			Archetypes:             []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
			Signals:                []string{"sitemap_included", "free_tools_language"},
		},
	})
	if len(gaps) != 1 {
		t.Fatalf("gaps = %+v, want cross-domain duplicate topic collapsed", gaps)
	}
	gap := gaps[0]
	if gap.Evidence["competitor_domain_count"] != int32(2) {
		t.Fatalf("gap evidence = %#v, want two competitor domains", gap.Evidence)
	}
	domains, ok := gap.Evidence["competitor_domain_samples"].([]string)
	if !ok || !slices.Contains(domains, "postsyncer.com") || !slices.Contains(domains, "socialbu.com") {
		t.Fatalf("competitor_domain_samples = %#v, want both domains", gap.Evidence["competitor_domain_samples"])
	}
	if gap.Evidence["competitive_domain_diversity"] != true {
		t.Fatalf("gap evidence = %#v, want domain diversity flag", gap.Evidence)
	}
	if gap.Priority <= 88 || gap.Confidence <= 0.87 {
		t.Fatalf("gap score priority=%v confidence=%v, want stronger cross-domain boost", gap.Priority, gap.Confidence)
	}
}

func TestCompetitiveSeedDomainDiversityQualifiesGrowthRadarEvidenceClaim(t *testing.T) {
	source, ageDays, qualified := qualifiedObservationEvidence(map[string]any{
		"source":                       "competitive_seed_url",
		"source_type":                  "competitive_seed_url",
		"seed_url":                     "https://postsyncer.com/tools/social-media-caption-generator",
		"competitor_domain":            "postsyncer.com",
		"archetype":                    "tools_hub",
		"competitive_domain_diversity": true,
		"competitor_domain_count":      int32(2),
		"competitor_domain_samples":    []string{"postsyncer.com", "socialbu.com"},
		"target_topic_source":          "seed_url_path",
		"derived_target_topic":         "social media caption generator",
	}, "absence", time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC))
	if !qualified || ageDays != nil {
		t.Fatalf("competitive seed evidence qualified=%v age=%v, want qualified timeless evidence", qualified, ageDays)
	}
	if source.Class != "competitive_seed_url" || !source.CompleteProvenance || !source.Qualified {
		t.Fatalf("competitive seed source = %+v, want qualified complete competitive seed source", source)
	}
	if source.SupportedClaim != "cross_domain_competitive_topic" {
		t.Fatalf("supported claim = %q, want cross-domain competitive topic", source.SupportedClaim)
	}
}

func TestCompetitiveSeedReportDerivesSpecificTopicFromPageTitleWhenURLPathIsGeneric(t *testing.T) {
	gaps := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{
		{
			URL: "https://postsyncer.com/tools", CanonicalURL: "https://postsyncer.com/tools",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			Title:                  "Free Social Media Caption Generator | PostSyncer",
			SameArchetypeLinkCount: 120,
			Archetypes:             []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
			Signals:                []string{"sitemap_included", "many_same_archetype_links", "free_tools_language"},
		},
		{
			URL: "https://postsyncer.com/alternatives", CanonicalURL: "https://postsyncer.com/alternatives",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			Title:      "Best Buffer Alternatives for Social Media Scheduling | PostSyncer",
			Archetypes: []crawl.SeedURLArchetype{{Archetype: "alternatives_cluster", Confidence: "high"}},
			Signals:    []string{"sitemap_included", "alternatives_language"},
		},
		{
			URL: "https://postsyncer.com/compare", CanonicalURL: "https://postsyncer.com/compare",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			Title:      "Buffer vs Hootsuite: Which Social Media Scheduler Is Better? | PostSyncer",
			Archetypes: []crawl.SeedURLArchetype{{Archetype: "comparison_cluster", Confidence: "high"}},
			Signals:    []string{"sitemap_included", "comparison_language"},
		},
	})
	if len(gaps) != 3 {
		t.Fatalf("gaps = %+v, want title-derived competitive seed gaps", gaps)
	}
	if gaps[0].TargetTopic != "social media caption generator" || gaps[0].PromptText != "best social media caption generator tools" {
		t.Fatalf("tools gap = %+v, want topic from title", gaps[0])
	}
	if gaps[1].TargetTopic != "buffer alternatives" || gaps[1].PromptText != "alternatives to buffer" {
		t.Fatalf("alternative gap = %+v, want subject from title", gaps[1])
	}
	if gaps[2].TargetTopic != "buffer vs hootsuite comparison" || gaps[2].PromptText != "buffer vs hootsuite comparison" {
		t.Fatalf("comparison gap = %+v, want comparison subject from title", gaps[2])
	}
	for _, gap := range gaps {
		if gap.Evidence["target_topic_source"] != "seed_page_title" {
			t.Fatalf("gap evidence = %#v, want title source", gap.Evidence)
		}
	}
}

func TestCompetitiveSeedReportDerivesSpecificTopicFromHeadingOrMetaWhenURLAndTitleAreGeneric(t *testing.T) {
	gaps := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{
		{
			URL: "https://postsyncer.com/tools", CanonicalURL: "https://postsyncer.com/tools",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			Title:                  "PostSyncer Tools",
			PrimaryH1:              "Free Social Media Caption Generator",
			SameArchetypeLinkCount: 120,
			Archetypes:             []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
			Signals:                []string{"sitemap_included", "many_same_archetype_links", "free_tools_language"},
		},
		{
			URL: "https://postsyncer.com/tools", CanonicalURL: "https://postsyncer.com/tools",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			Title:                  "PostSyncer Tools",
			MetaDescription:        "Free LinkedIn Hook Generator.",
			SameArchetypeLinkCount: 120,
			Archetypes:             []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
			Signals:                []string{"sitemap_included", "many_same_archetype_links", "free_tools_language"},
		},
	})
	if len(gaps) != 2 {
		t.Fatalf("gaps = %+v, want heading/meta-derived competitive seed gaps", gaps)
	}
	if gaps[0].TargetTopic != "social media caption generator" || gaps[0].PromptText != "best social media caption generator tools" {
		t.Fatalf("heading gap = %+v, want topic from h1", gaps[0])
	}
	if gaps[0].Evidence["target_topic_source"] != "seed_page_h1" {
		t.Fatalf("heading evidence = %#v, want h1 source", gaps[0].Evidence)
	}
	if gaps[1].TargetTopic != "linkedin hook generator" || gaps[1].PromptText != "best linkedin hook generator tools" {
		t.Fatalf("meta gap = %+v, want topic from meta description", gaps[1])
	}
	if gaps[1].Evidence["target_topic_source"] != "seed_meta_description" {
		t.Fatalf("meta evidence = %#v, want meta source", gaps[1].Evidence)
	}
}

func TestCompetitiveSeedReportGapPreservesAutomaticDiscoveryProvenance(t *testing.T) {
	gaps := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{{
		URL: "https://postsyncer.com/tools/social-media-caption-generator", CanonicalURL: "https://postsyncer.com/tools/social-media-caption-generator",
		Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
		DiscoverySource:        "site_discovery",
		DiscoveredFromURL:      "https://postsyncer.com/",
		SameArchetypeLinkCount: 120,
		Archetypes:             []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
		Signals:                []string{"sitemap_included", "many_same_archetype_links", "free_tools_language", "competitive_urls_discovered"},
	}})
	if len(gaps) != 1 {
		t.Fatalf("gaps = %+v, want one automatically discovered competitive seed gap", gaps)
	}
	evidence := gaps[0].Evidence
	if evidence["discovery_source"] != "site_discovery" || evidence["discovered_from_url"] != "https://postsyncer.com/" {
		t.Fatalf("gap evidence = %#v, want automatic discovery provenance", evidence)
	}
	if notes, ok := evidence["data_source_notes"].([]string); !ok || !slices.Contains(notes, "site_discovery") {
		t.Fatalf("data source notes = %#v, want site_discovery", evidence["data_source_notes"])
	}
}

func TestCompetitiveSeedGapBriefGuidanceNamesSourceURLsAndArchetype(t *testing.T) {
	gaps := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{{
		URL: "https://postsyncer.com/tools/social-media-caption-generator", CanonicalURL: "https://postsyncer.com/tools/social-media-caption-generator",
		Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
		DiscoverySource:        "site_discovery",
		DiscoveredFromURL:      "https://postsyncer.com/",
		SameArchetypeLinkCount: 120,
		Archetypes:             []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
		Signals:                []string{"sitemap_included", "many_same_archetype_links", "free_tools_language", "competitive_urls_discovered"},
	}})
	if len(gaps) != 1 {
		t.Fatalf("gaps = %+v, want one competitive seed gap", gaps)
	}

	required := requiredEvidenceForGap(gaps[0])
	for _, want := range []string{
		"competitor seed URL: https://postsyncer.com/tools/social-media-caption-generator",
		"auto-discovered from: https://postsyncer.com/",
		"competitive archetype: tools_hub",
	} {
		if !slices.Contains(required, want) {
			t.Fatalf("required evidence = %#v, want %q", required, want)
		}
	}

	outline := outlineForGap(gaps[0])
	for _, want := range []string{
		"Use https://postsyncer.com/tools/social-media-caption-generator as the competitor reference, but create a project-specific resource.",
		"Explain why this project should answer the tools_hub opportunity for social media caption generator.",
	} {
		if !slices.Contains(outline, want) {
			t.Fatalf("recommended outline = %#v, want %q", outline, want)
		}
	}
}

func TestCompetitiveSeedGapBriefGuidanceNamesTopicPathProbeProvenance(t *testing.T) {
	gaps := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{{
		URL: "https://postsyncer.com/tools/social-media-caption-generator", CanonicalURL: "https://postsyncer.com/tools/social-media-caption-generator",
		Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
		DiscoverySource:        "topic_path_probe",
		DiscoveredFromURL:      "https://postsyncer.com/",
		ProbeIntent:            "tools",
		SameArchetypeLinkCount: 120,
		Archetypes:             []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
		Signals:                []string{"sitemap_included", "many_same_archetype_links", "free_tools_language"},
	}})
	if len(gaps) != 1 {
		t.Fatalf("gaps = %+v, want one topic path probe competitive seed gap", gaps)
	}

	required := requiredEvidenceForGap(gaps[0])
	for _, want := range []string{
		"automatic discovery method: topic_path_probe",
		"auto-discovered from: https://postsyncer.com/",
		"automatic probe intent: tools",
	} {
		if !slices.Contains(required, want) {
			t.Fatalf("required evidence = %#v, want %q", required, want)
		}
	}

	outline := outlineForGap(gaps[0])
	for _, want := range []string{
		"Explain that CiteLoop inferred this competitor page via topic path probe from https://postsyncer.com/.",
		"Frame the project-specific resource around the automatically inferred tools intent.",
	} {
		if !slices.Contains(outline, want) {
			t.Fatalf("recommended outline = %#v, want %q", outline, want)
		}
	}
}

func TestCompetitiveSeedGapBriefGuidanceNamesDomainDiversity(t *testing.T) {
	gaps := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{
		{
			URL: "https://postsyncer.com/tools/social-media-caption-generator", CanonicalURL: "https://postsyncer.com/tools/social-media-caption-generator",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			DiscoverySource:        "site_discovery",
			SameArchetypeLinkCount: 120,
			Archetypes:             []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
			Signals:                []string{"sitemap_included", "many_same_archetype_links", "free_tools_language"},
		},
		{
			URL: "https://socialbu.com/tools/social-media-caption-generator", CanonicalURL: "https://socialbu.com/tools/social-media-caption-generator",
			Host: "socialbu.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			DiscoverySource:        "site_discovery",
			SameArchetypeLinkCount: 95,
			Archetypes:             []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
			Signals:                []string{"sitemap_included", "free_tools_language"},
		},
	})
	if len(gaps) != 1 {
		t.Fatalf("gaps = %+v, want one cross-domain competitive seed gap", gaps)
	}

	required := requiredEvidenceForGap(gaps[0])
	for _, want := range []string{
		"competitor domains supporting this topic: postsyncer.com, socialbu.com",
		"competitor seed URL samples: https://postsyncer.com/tools/social-media-caption-generator, https://socialbu.com/tools/social-media-caption-generator",
	} {
		if !slices.Contains(required, want) {
			t.Fatalf("required evidence = %#v, want %q", required, want)
		}
	}

	outline := outlineForGap(gaps[0])
	for _, want := range []string{
		"Compare patterns across competitor examples from postsyncer.com, socialbu.com before recommending the project-specific resource.",
		"Use the grouped competitor seed URLs as references, but create a project-specific resource.",
	} {
		if !slices.Contains(outline, want) {
			t.Fatalf("recommended outline = %#v, want %q", outline, want)
		}
	}
}

func TestAnalyzeObservationsCreatesCompetitiveSeedBriefWithSourceGuidance(t *testing.T) {
	projectID := uuid.New()
	store := &geoStoreStub{runID: uuid.New()}
	service := Service{Q: store, Now: func() time.Time { return time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC) }}

	result, err := service.AnalyzeObservations(context.Background(), projectID, AnalyzeObservationsRequest{
		Limit: 50,
		CompetitiveSeedReports: []crawl.SeedURLEnrichment{{
			URL: "https://postsyncer.com/tools/social-media-caption-generator", CanonicalURL: "https://postsyncer.com/tools/social-media-caption-generator",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			DiscoverySource:        "site_discovery",
			DiscoveredFromURL:      "https://postsyncer.com/",
			SameArchetypeLinkCount: 120,
			Archetypes:             []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
			Signals:                []string{"sitemap_included", "many_same_archetype_links", "free_tools_language", "competitive_urls_discovered"},
		}},
	})
	if err != nil {
		t.Fatalf("AnalyzeObservations error: %v", err)
	}
	if len(result.Opportunities) != 1 || len(result.AssetBriefs) != 1 {
		t.Fatalf("opportunities=%d briefs=%d, want competitive seed opportunity and brief", len(result.Opportunities), len(result.AssetBriefs))
	}
	required := rawJSONList(result.AssetBriefs[0].RequiredEvidence)
	for _, want := range []string{
		"competitor seed URL: https://postsyncer.com/tools/social-media-caption-generator",
		"auto-discovered from: https://postsyncer.com/",
		"competitive archetype: tools_hub",
	} {
		if !slices.Contains(required, want) {
			t.Fatalf("brief required evidence = %#v, want %q", required, want)
		}
	}
	outline := rawJSONList(result.AssetBriefs[0].RecommendedOutline)
	if !slices.Contains(outline, "Use https://postsyncer.com/tools/social-media-caption-generator as the competitor reference, but create a project-specific resource.") {
		t.Fatalf("brief recommended outline = %#v, want competitor source guidance", outline)
	}
}

func TestCompetitiveSeedReportCreatesComparisonAndAlternativeGaps(t *testing.T) {
	gaps := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{
		{
			URL: "https://postsyncer.com/alternatives", CanonicalURL: "https://postsyncer.com/alternatives",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			Archetypes: []crawl.SeedURLArchetype{{Archetype: "alternatives_cluster", Confidence: "high"}},
			Signals:    []string{"sitemap_included", "alternatives_language"},
		},
		{
			URL: "https://postsyncer.com/compare/buffer", CanonicalURL: "https://postsyncer.com/compare/buffer",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			Archetypes: []crawl.SeedURLArchetype{{Archetype: "comparison_cluster", Confidence: "high"}},
			Signals:    []string{"sitemap_included", "comparison_language"},
		},
	})
	if len(gaps) != 2 {
		t.Fatalf("gaps = %+v, want alternative and comparison gaps", gaps)
	}
	if gaps[0].Type != "competitive_alternative_gap" || gaps[0].AssetType != "alternative_page" || gaps[0].Intent != "alternative" {
		t.Fatalf("alternative gap = %+v", gaps[0])
	}
	if gaps[0].Evidence["archetype"] != "alternatives_cluster" || gaps[0].Evidence["reason"] != "competitive_alternative_gap" {
		t.Fatalf("alternative evidence = %#v", gaps[0].Evidence)
	}
	if gaps[1].Type != "competitive_comparison_cluster_gap" || gaps[1].AssetType != "comparison_page" || gaps[1].Intent != "comparison" {
		t.Fatalf("comparison gap = %+v", gaps[1])
	}
	if gaps[1].Evidence["archetype"] != "comparison_cluster" || gaps[1].Evidence["reason"] != "competitive_comparison_cluster_gap" {
		t.Fatalf("comparison evidence = %#v", gaps[1].Evidence)
	}
}

func TestCompetitiveSeedProbeIntentShapesGenericArchetypeGaps(t *testing.T) {
	gaps := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{
		{
			URL: "https://postsyncer.com/alternatives/buffer", CanonicalURL: "https://postsyncer.com/alternatives/buffer",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			DiscoverySource: "topic_path_probe", ProbeIntent: "alternatives",
			Archetypes: []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
			Signals:    []string{"sitemap_included", "many_same_archetype_links", "free_tools_language"},
		},
		{
			URL: "https://postsyncer.com/compare/buffer-vs-hootsuite", CanonicalURL: "https://postsyncer.com/compare/buffer-vs-hootsuite",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			DiscoverySource: "topic_path_probe", ProbeIntent: "comparison",
			Archetypes: []crawl.SeedURLArchetype{{Archetype: "tools_hub", Confidence: "high"}},
			Signals:    []string{"sitemap_included", "many_same_archetype_links", "free_tools_language"},
		},
	})
	if len(gaps) != 2 {
		t.Fatalf("gaps = %+v, want intent-shaped alternative and comparison gaps", gaps)
	}
	if gaps[0].Type != "competitive_alternative_gap" || gaps[0].AssetType != "alternative_page" || gaps[0].Intent != "alternative" {
		t.Fatalf("alternative intent-shaped gap = %+v", gaps[0])
	}
	if gaps[0].Evidence["archetype"] != "tools_hub" || gaps[0].Evidence["reason"] != "competitive_alternative_gap" || gaps[0].Evidence["probe_intent"] != "alternatives" {
		t.Fatalf("alternative intent-shaped evidence = %#v", gaps[0].Evidence)
	}
	if gaps[1].Type != "competitive_comparison_cluster_gap" || gaps[1].AssetType != "comparison_page" || gaps[1].Intent != "comparison" {
		t.Fatalf("comparison intent-shaped gap = %+v", gaps[1])
	}
	if gaps[1].Evidence["archetype"] != "tools_hub" || gaps[1].Evidence["reason"] != "competitive_comparison_cluster_gap" || gaps[1].Evidence["probe_intent"] != "comparison" {
		t.Fatalf("comparison intent-shaped evidence = %#v", gaps[1].Evidence)
	}
}

func TestCompetitiveSeedReportDerivesAlternativeAndComparisonSubjectsFromURLPath(t *testing.T) {
	gaps := gapsForCompetitiveSeedReports([]crawl.SeedURLEnrichment{
		{
			URL: "https://postsyncer.com/alternatives/buffer", CanonicalURL: "https://postsyncer.com/alternatives/buffer",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			Archetypes: []crawl.SeedURLArchetype{{Archetype: "alternatives_cluster", Confidence: "high"}},
			Signals:    []string{"sitemap_included", "alternatives_language"},
		},
		{
			URL: "https://postsyncer.com/compare/hootsuite", CanonicalURL: "https://postsyncer.com/compare/hootsuite",
			Host: "postsyncer.com", StatusCode: 200, RobotsAllowed: true, Indexable: true,
			Archetypes: []crawl.SeedURLArchetype{{Archetype: "comparison_cluster", Confidence: "high"}},
			Signals:    []string{"sitemap_included", "comparison_language"},
		},
	})
	if len(gaps) != 2 {
		t.Fatalf("gaps = %+v, want derived alternative and comparison gaps", gaps)
	}
	if gaps[0].TargetTopic != "buffer alternatives" || gaps[0].PromptText != "alternatives to buffer" {
		t.Fatalf("alternative gap = %+v, want subject from URL path", gaps[0])
	}
	if gaps[1].TargetTopic != "hootsuite comparison" || gaps[1].PromptText != "hootsuite comparison" {
		t.Fatalf("comparison gap = %+v, want subject from URL path", gaps[1])
	}
}

func TestGEODemandRejectsSyntheticOrEmptyObservations(t *testing.T) {
	now := time.Date(2026, 7, 14, 0, 0, 0, 0, time.UTC)
	valid := db.GeoObservation{SourceType: SourceTypeAnswerEngine, ObservationState: "observed", Engine: "openai", AnswerSummary: "A useful answer", PromptID: pgUUID(uuid.New()), ObservedAt: pgutil.TS(now)}
	if !qualifiesForGEODemand(valid, now.Add(-30*24*time.Hour)) {
		t.Fatal("real answer material should qualify for provider aggregation")
	}
	for _, observation := range []db.GeoObservation{
		{SourceType: ProviderManualFixture, ObservationState: "observed", Engine: "fixture", AnswerSummary: "synthetic", PromptID: pgUUID(uuid.New()), ObservedAt: pgutil.TS(now)},
		{SourceType: SourceTypeAnswerEngine, ObservationState: "observed", Engine: "openai", PromptID: pgUUID(uuid.New()), ObservedAt: pgutil.TS(now)},
	} {
		if qualifiesForGEODemand(observation, now.Add(-30*24*time.Hour)) {
			t.Fatalf("synthetic or empty answer unexpectedly qualified: %+v", observation)
		}
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

func TestQualifiedObservationEvidenceScopesUncitedAnswersToAbsence(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	evidence := map[string]any{
		"observation_id": uuid.New(), "source_type": SourceTypeAnswerEngine, "observation_state": "observed",
		"engine": "OpenAI", "observed_at": now.Format(time.RFC3339), "answer_hash": "answer-1",
		"cited_urls": []any{}, "competitor_citations": []any{},
	}
	source, _, qualified := qualifiedObservationEvidence(evidence, "absence", now)
	if !qualified || source.SupportedClaim != "absence" || !source.CompleteProvenance {
		t.Fatalf("absence evidence = %+v qualified=%v", source, qualified)
	}
	if _, _, qualified := qualifiedObservationEvidence(evidence, "citation", now); qualified {
		t.Fatal("uncited answer qualified for citation claim")
	}
}

func TestLatestObservedByPromptEngineIgnoresStaleAndFailedStates(t *testing.T) {
	promptID := uuid.New()
	base := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	rows := []db.GeoObservation{
		{ID: uuid.New(), PromptID: pgUUID(promptID), Engine: "OpenAI", ObservationState: "provider_unavailable", ObservedAt: pgutil.TS(base)},
		{ID: uuid.New(), PromptID: pgUUID(promptID), Engine: "OpenAI", ObservationState: "observed", ProjectCitationCount: 1, ObservedAt: pgutil.TS(base.Add(-time.Hour))},
		{ID: uuid.New(), PromptID: pgUUID(promptID), Engine: "OpenAI", ObservationState: "observed", ProjectCitationCount: 0, ObservedAt: pgutil.TS(base.Add(-2 * time.Hour))},
		{ID: uuid.New(), PromptID: pgUUID(promptID), Engine: "Perplexity", ObservationState: "observed", ProjectCitationCount: 0, ObservedAt: pgutil.TS(base.Add(-3 * time.Hour))},
	}
	latest := latestObservedByPromptEngine(rows)
	if len(latest) != 2 {
		t.Fatalf("latest observations = %+v", latest)
	}
	if latest[0].Engine != "OpenAI" || latest[0].ProjectCitationCount != 1 {
		t.Fatalf("newest valid OpenAI observation not selected: %+v", latest[0])
	}
	if latest[1].Engine != "Perplexity" || latest[1].ProjectCitationCount != 0 {
		t.Fatalf("independent engine observation missing: %+v", latest[1])
	}
}

func TestAcceptAssetBriefCreatesTopic(t *testing.T) {
	projectID := uuid.New()
	briefID := uuid.New()
	opportunityID := uuid.New()
	store := &geoStoreStub{
		assetBriefs: []db.GeoAssetBrief{{
			ID:                 briefID,
			ProjectID:          projectID,
			OpportunityID:      opportunityID,
			AssetType:          "comparison_page",
			Status:             "ready_for_review",
			TargetPrompts:      json.RawMessage(`["best social scheduling tools"]`),
			RequiredEvidence:   json.RawMessage(`["first-party comparison proof","competitor citation evidence: Buffer"]`),
			RecommendedOutline: json.RawMessage(`["What to compare","Evidence","When to choose UniPost"]`),
			InternalLinkPlan:   json.RawMessage(`["/blog/social-scheduling"]`),
			PublicationSurface: "blog",
		}},
	}

	result, err := Service{Q: store}.AcceptGEOAssetBrief(context.Background(), projectID, briefID)
	if err != nil {
		t.Fatalf("AcceptGEOAssetBrief error: %v", err)
	}
	if result.Brief.Status != "converted" {
		t.Fatalf("brief status = %q, want converted", result.Brief.Status)
	}
	if result.Topic.ID == uuid.Nil || result.Topic.Status != "backlog" || result.Topic.ProjectID != projectID {
		t.Fatalf("topic = %+v, want backlog topic", result.Topic)
	}
	if len(store.createdTopics) != 1 {
		t.Fatalf("created topics = %d, want 1", len(store.createdTopics))
	}
	if result.Topic.Angle == nil || *result.Topic.Angle != "comparison_page" {
		t.Fatalf("topic angle = %v, want comparison_page asset type", result.Topic.Angle)
	}
	if result.Topic.Format == nil || *result.Topic.Format != "geo_asset_brief" {
		t.Fatalf("topic format = %v, want geo_asset_brief", result.Topic.Format)
	}
	var metadata struct {
		AssetBriefID       string   `json:"asset_brief_id"`
		Links              []string `json:"links"`
		SourceEvidence     []string `json:"source_evidence"`
		RecommendedOutline []string `json:"recommended_outline"`
	}
	if err := json.Unmarshal(result.Topic.InternalLinks, &metadata); err != nil {
		t.Fatalf("topic internal_links should carry asset metadata object: %v; raw=%s", err, string(result.Topic.InternalLinks))
	}
	if metadata.AssetBriefID != briefID.String() {
		t.Fatalf("metadata asset brief id = %q, want %s", metadata.AssetBriefID, briefID)
	}
	if len(metadata.SourceEvidence) != 2 || metadata.SourceEvidence[0] != "first-party comparison proof" {
		t.Fatalf("metadata source evidence = %#v, want brief evidence", metadata.SourceEvidence)
	}
	if len(metadata.RecommendedOutline) != 3 || metadata.RecommendedOutline[0] != "What to compare" {
		t.Fatalf("metadata outline = %#v, want brief outline", metadata.RecommendedOutline)
	}
	if len(metadata.Links) != 1 || metadata.Links[0] != "/blog/social-scheduling" {
		t.Fatalf("metadata links = %#v, want internal link plan", metadata.Links)
	}
}

func hasOpportunityType(rows []db.UpsertGEOObservationOpportunityRow, kind string) bool {
	for _, row := range rows {
		if row.Type == kind {
			return true
		}
	}
	return false
}

func (s *geoStoreStub) UpsertGEOObservationOpportunity(_ context.Context, arg db.UpsertGEOObservationOpportunityParams) (db.UpsertGEOObservationOpportunityRow, error) {
	for _, row := range s.opportunities {
		if row.ProjectID == arg.ProjectID && row.Type == arg.Type && row.NormalizedPageUrl == arg.NormalizedPageUrl && sameQuery(row.Query, arg.Query) {
			return row, nil
		}
	}
	row := db.UpsertGEOObservationOpportunityRow{
		ID:                uuid.New(),
		ProjectID:         arg.ProjectID,
		Type:              arg.Type,
		Status:            arg.Status,
		PriorityScore:     arg.PriorityScore,
		Confidence:        arg.Confidence,
		PageUrl:           arg.PageUrl,
		NormalizedPageUrl: arg.NormalizedPageUrl,
		Query:             arg.Query,
		Evidence:          append(json.RawMessage{}, arg.Evidence...),
		RecommendedAction: arg.RecommendedAction,
		ExpectedImpact:    arg.ExpectedImpact,
		Effort:            arg.Effort,
		RiskLevel:         arg.RiskLevel,
	}
	s.opportunities = append(s.opportunities, row)
	return row, nil
}

func (s *geoStoreStub) CreateGEOAssetBrief(_ context.Context, arg db.CreateGEOAssetBriefParams) (db.GeoAssetBrief, error) {
	for _, row := range s.assetBriefs {
		if row.ProjectID == arg.ProjectID && row.OpportunityID == arg.OpportunityID {
			return row, nil
		}
	}
	id := s.assetBriefID
	if id == uuid.Nil {
		id = uuid.New()
	}
	row := db.GeoAssetBrief{
		ID:                 id,
		ProjectID:          arg.ProjectID,
		OpportunityID:      arg.OpportunityID,
		AssetType:          arg.AssetType,
		Status:             arg.Status,
		TargetPrompts:      append(json.RawMessage{}, arg.TargetPrompts...),
		RequiredEvidence:   append(json.RawMessage{}, arg.RequiredEvidence...),
		RecommendedOutline: append(json.RawMessage{}, arg.RecommendedOutline...),
		InternalLinkPlan:   append(json.RawMessage{}, arg.InternalLinkPlan...),
		PublicationSurface: arg.PublicationSurface,
		CreatedByRunID:     arg.CreatedByRunID,
	}
	s.assetBriefs = append(s.assetBriefs, row)
	return row, nil
}

func (s *geoStoreStub) ListGEOAssetBriefs(context.Context, db.ListGEOAssetBriefsParams) ([]db.GeoAssetBrief, error) {
	return s.assetBriefs, nil
}

func (s *geoStoreStub) GetGEOAssetBriefForProject(_ context.Context, arg db.GetGEOAssetBriefForProjectParams) (db.GeoAssetBrief, error) {
	for _, row := range s.assetBriefs {
		if row.ID == arg.ID && row.ProjectID == arg.ProjectID {
			return row, nil
		}
	}
	return db.GeoAssetBrief{}, nil
}

func (s *geoStoreStub) UpdateGEOAssetBriefStatus(_ context.Context, arg db.UpdateGEOAssetBriefStatusParams) (db.GeoAssetBrief, error) {
	for i := range s.assetBriefs {
		if s.assetBriefs[i].ID == arg.ID && s.assetBriefs[i].ProjectID == arg.ProjectID {
			s.assetBriefs[i].Status = arg.Status
			return s.assetBriefs[i], nil
		}
	}
	row := db.GeoAssetBrief{ID: arg.ID, ProjectID: arg.ProjectID, Status: arg.Status}
	s.assetBriefs = append(s.assetBriefs, row)
	return row, nil
}

func (s *geoStoreStub) CreateTopic(_ context.Context, arg db.CreateTopicParams) (db.Topic, error) {
	row := db.Topic{
		ID:            uuid.New(),
		ProjectID:     arg.ProjectID,
		Channel:       arg.Channel,
		Title:         arg.Title,
		TargetKeyword: arg.TargetKeyword,
		TargetPrompt:  arg.TargetPrompt,
		Angle:         arg.Angle,
		Format:        arg.Format,
		Priority:      arg.Priority,
		InternalLinks: append(json.RawMessage{}, arg.InternalLinks...),
		Status:        arg.Status,
		ScheduledAt:   arg.ScheduledAt,
	}
	s.createdTopics = append(s.createdTopics, row)
	return row, nil
}

func sameQuery(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
