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
	for _, key := range []string{"implementation_steps", "verification_steps", "likely_surfaces", "patch_contract"} {
		if _, ok := firstChange[key]; !ok {
			t.Fatalf("site fix proposed change missing %s: %#v", key, firstChange)
		}
	}
	if payload.AIRepair == nil {
		t.Fatal("site fix diff must include ai_repair payload for Codex/Claude Code")
	}
	fix, ok := payload.AIRepair["fix"].(map[string]any)
	if !ok {
		t.Fatalf("ai_repair.fix missing or invalid: %#v", payload.AIRepair)
	}
	for _, key := range []string{"instructions", "likely_surfaces", "seo_contract", "risk_level"} {
		if _, ok := fix[key]; !ok {
			t.Fatalf("ai_repair.fix missing %s: %#v", key, fix)
		}
	}
	if len(payload.AcceptanceTests) == 0 {
		t.Fatal("site fix diff must include acceptance tests")
	}
	joinedTests := strings.Join(payload.AcceptanceTests, "\n")
	if !strings.Contains(joinedTests, `script[type="application/ld+json"]`) || !strings.Contains(joinedTests, pageURL) {
		t.Fatalf("schema patch acceptance tests should name JSON-LD and target URL, got %q", joinedTests)
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
