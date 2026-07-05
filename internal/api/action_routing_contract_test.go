package api

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
)

func TestCreateSEOContentActionInfersMultiSurfaceAssetAndReviewOutput(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"inferContentActionAssetType",
		"defaultReviewRequiredForAssetType",
		"defaultOutputSnapshotForAction",
		"defaultDiffSnapshotForAction",
		"metadata_rewrite",
		"internal_link_patch",
		"schema_patch",
		"technical_fix",
		"direct_patch",
		"technical_task",
		"seo_geo_contribution",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("createSEOContentAction routing contract missing %q", want)
		}
	}
}

func TestDefaultDiffSnapshotForSiteFixIncludesAIRepairPayload(t *testing.T) {
	pageURL := "https://unipost.dev/"
	action := "Add structured data to the existing page"
	impact := "Make page entities and answers easier for search and answer engines to parse."
	opp := db.SeoOpportunity{
		Type:              "schema_gap",
		PageUrl:           &pageURL,
		NormalizedPageUrl: pageURL,
		Evidence: json.RawMessage(`{
			"observed_metadata": {
				"canonical_url": "https://unipost.dev/",
				"title": "UniPost - Social publishing for teams",
				"description": "Publish and repurpose social content from one workspace.",
				"og_title": "UniPost",
				"og_description": "Plan social posts with your team.",
				"og_image": "https://unipost.dev/og.png",
				"brand_name": "UniPost"
			}
		}`),
		RecommendedAction: &action,
		ExpectedImpact:    &impact,
		RiskLevel:         "medium",
	}

	raw := defaultDiffSnapshotForAction(nil, "schema_patch", action, opp)
	var payload struct {
		OutputType        string           `json:"output_type"`
		TargetURL         string           `json:"target_url"`
		ProposedChanges   []map[string]any `json:"proposed_changes"`
		AIRepair          map[string]any   `json:"ai_repair"`
		AcceptanceTests   []string         `json:"acceptance_tests"`
		RequiresApplyStep bool             `json:"requires_apply_step"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal default diff snapshot: %v\n%s", err, raw)
	}

	if payload.OutputType != "direct_patch" || payload.TargetURL != pageURL || !payload.RequiresApplyStep {
		t.Fatalf("diff snapshot has wrong envelope: %#v", payload)
	}
	if len(payload.ProposedChanges) == 0 {
		t.Fatal("site fix diff must include structured proposed changes")
	}
	firstChange := payload.ProposedChanges[0]
	for _, key := range []string{"implementation_steps", "verification_steps", "likely_surfaces", "patch_contract", "human_review"} {
		if _, ok := firstChange[key]; !ok {
			t.Fatalf("site fix proposed change missing %s: %#v", key, firstChange)
		}
	}
	patchContract, ok := firstChange["patch_contract"].(map[string]any)
	if !ok {
		t.Fatalf("site fix proposed change patch_contract invalid: %#v", firstChange["patch_contract"])
	}
	for _, key := range []string{"deduplication_rule", "graph_guidance", "do_not"} {
		if _, ok := patchContract[key]; !ok {
			t.Fatalf("schema patch contract missing %s: %#v", key, patchContract)
		}
	}
	if payload.AIRepair == nil {
		t.Fatal("site fix diff must include ai_repair payload for Codex/Claude Code")
	}
	evidence, ok := payload.AIRepair["evidence"].(map[string]any)
	if !ok {
		t.Fatalf("ai_repair.evidence missing or invalid: %#v", payload.AIRepair)
	}
	metadata, ok := evidence["observed_metadata"].(map[string]any)
	if !ok {
		t.Fatalf("ai_repair evidence must include observed metadata when evidence has it: %#v", evidence)
	}
	if metadata["brand_name"] != "UniPost" || metadata["canonical_url"] != pageURL || metadata["og_image"] != "https://unipost.dev/og.png" {
		t.Fatalf("observed metadata should preserve only extracted real fields, got %#v", metadata)
	}
	fix, ok := payload.AIRepair["fix"].(map[string]any)
	if !ok {
		t.Fatalf("ai_repair.fix missing or invalid: %#v", payload.AIRepair)
	}
	for _, key := range []string{"instructions", "likely_surfaces", "seo_contract", "risk_level", "deduplication_rule", "do_not"} {
		if _, ok := fix[key]; !ok {
			t.Fatalf("ai_repair.fix missing %s: %#v", key, fix)
		}
	}
	seoContract, ok := fix["seo_contract"].(map[string]any)
	if !ok {
		t.Fatalf("ai_repair.fix.seo_contract missing or invalid: %#v", fix)
	}
	graphGuidance, ok := seoContract["graph_guidance"].(map[string]any)
	if !ok {
		t.Fatalf("schema patch seo_contract must include graph guidance: %#v", seoContract)
	}
	graphJSON, _ := json.Marshal(graphGuidance)
	graphText := string(graphJSON)
	if !strings.Contains(graphText, "@graph") || !strings.Contains(graphText, "#organization") || !strings.Contains(graphText, "#website") || !strings.Contains(graphText, "#webpage") {
		t.Fatalf("graph guidance should name @graph and stable @id fragments, got %s", graphText)
	}
	humanReview, ok := payload.AIRepair["human_review"].(map[string]any)
	if !ok || humanReview["required"] != true {
		t.Fatalf("ai_repair must explain human review requirement: %#v", payload.AIRepair["human_review"])
	}
	if len(payload.AcceptanceTests) == 0 {
		t.Fatal("site fix diff must include acceptance tests")
	}
	joinedTests := strings.Join(payload.AcceptanceTests, "\n")
	if !strings.Contains(joinedTests, `script[type="application/ld+json"]`) || !strings.Contains(joinedTests, pageURL) {
		t.Fatalf("schema patch acceptance tests should name JSON-LD and target URL, got %q", joinedTests)
	}
	if !strings.Contains(joinedTests, "Schema Markup Validator") || !strings.Contains(joinedTests, "does not require rich result eligibility") {
		t.Fatalf("schema patch acceptance tests should avoid overclaiming rich result eligibility, got %q", joinedTests)
	}
}

func TestPlanSEOContentActionCreatesTopicForManualDrafting(t *testing.T) {
	serverRaw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	if !strings.Contains(string(serverRaw), `r.Post("/actions/{actionID}/plan", s.planSEOContentAction)`) {
		t.Fatal("manual Content Plan drafting must expose a content action planning endpoint")
	}

	handlerRaw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(handlerRaw)
	for _, want := range []string{
		"func (s *Server) planSEOContentAction",
		"GetContentAction",
		"contentActionNeedsTopic",
		"CreateTopic",
		"SourceContentActionID",
		`Status:                string(topicstate.StatusBacklog)`,
		`Status:    "approved"`,
		"EnqueueWorkflowEvent",
		"workflow.EventContentPlanCreated",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("manual content action planning missing %q", want)
		}
	}
}
