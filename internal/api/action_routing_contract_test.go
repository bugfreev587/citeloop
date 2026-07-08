package api

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
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

func TestMetadataRewriteAIRepairPayloadIncludesActionableContract(t *testing.T) {
	pageURL := "https://unipost.dev/"
	query := "unipost"
	action := "Rewrite homepage title and meta description for query relevance"
	broadRecommendation := "Expand the existing page or create a supporting section for the query intent"
	impact := "Improve search appearance and click-through for an existing page."
	opp := db.SeoOpportunity{
		Type:              "low_ctr_page",
		PageUrl:           &pageURL,
		NormalizedPageUrl: pageURL,
		Query:             &query,
		Evidence: json.RawMessage(`{
			"observed": {
				"status": 200,
				"title": "UniPost - Social publishing for teams",
				"meta_description": "Publish and repurpose social content from one workspace.",
				"canonical": "https://unipost.dev/",
				"robots": "indexable",
				"observed_at": "2026-07-08"
			},
			"opportunity": {
				"intent": "branded + product category",
				"problem_detail": "Meta description is long and feature-list-like; rewrite for clearer search snippet and product positioning.",
				"confidence": 0.72,
				"priority": "low_to_medium",
				"recommended_action": "Expand the existing page or create a supporting section for the query intent"
			},
			"proposed_change": {
				"title": "UniPost | Social Media Posting API for Developers",
				"meta_description": "UniPost gives developers one API to connect customer social accounts, upload media, schedule posts, and publish across major social platforms.",
				"seo_impact": "search snippet relevance / CTR",
				"geo_impact": "entity clarity / product category clarity",
				"content_support_required": false
			}
		}`),
		RecommendedAction: &broadRecommendation,
		ExpectedImpact:    &impact,
		RiskLevel:         "low",
	}

	raw := defaultDiffSnapshotForAction(nil, "metadata_rewrite", action, opp)
	var payload struct {
		AIRepair map[string]any `json:"ai_repair"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal default diff snapshot: %v\n%s", err, raw)
	}
	repairJSON, _ := json.Marshal(payload.AIRepair)
	if strings.Contains(string(repairJSON), broadRecommendation) {
		t.Fatalf("metadata repair JSON should not carry conflicting content-expansion scope: %s", repairJSON)
	}

	observed, ok := payload.AIRepair["observed"].(map[string]any)
	if !ok {
		t.Fatalf("metadata repair JSON must include current observed values: %#v", payload.AIRepair)
	}
	if observed["status"] != float64(200) || observed["title"] != "UniPost - Social publishing for teams" || observed["meta_description"] != "Publish and repurpose social content from one workspace." {
		t.Fatalf("observed values should preserve status/title/meta description, got %#v", observed)
	}
	if observed["canonical"] != pageURL || observed["robots"] != "indexable" || observed["observed_at"] != "2026-07-08" {
		t.Fatalf("observed values should preserve canonical, robots, and timestamp, got %#v", observed)
	}

	opportunity, ok := payload.AIRepair["opportunity"].(map[string]any)
	if !ok {
		t.Fatalf("metadata repair JSON must include opportunity context: %#v", payload.AIRepair)
	}
	if opportunity["query"] != query || opportunity["intent"] != "branded + product category" || opportunity["confidence"] != 0.72 || opportunity["priority"] != "low_to_medium" {
		t.Fatalf("opportunity context should preserve query intent, confidence, and priority, got %#v", opportunity)
	}
	if !strings.Contains(opportunity["problem_detail"].(string), "clearer search snippet") {
		t.Fatalf("opportunity context should explain why metadata is weak, got %#v", opportunity)
	}

	proposed, ok := payload.AIRepair["proposed_change"].(map[string]any)
	if !ok {
		t.Fatalf("metadata repair JSON must include proposed title and meta description: %#v", payload.AIRepair)
	}
	if proposed["title"] != "UniPost | Social Media Posting API for Developers" {
		t.Fatalf("proposed title not carried through: %#v", proposed)
	}
	if proposed["meta_description"] != "UniPost gives developers one API to connect customer social accounts, upload media, schedule posts, and publish across major social platforms." {
		t.Fatalf("proposed meta description not carried through: %#v", proposed)
	}
	preserve, ok := proposed["preserve"].([]any)
	if !ok || len(preserve) < 3 {
		t.Fatalf("proposed metadata change must name signals to preserve, got %#v", proposed["preserve"])
	}

	fix, ok := payload.AIRepair["fix"].(map[string]any)
	if !ok {
		t.Fatalf("metadata repair JSON must include fix contract: %#v", payload.AIRepair)
	}
	dedupe, _ := fix["deduplication_rule"].(string)
	if !strings.Contains(dedupe, "OpenGraph") || !strings.Contains(dedupe, "Twitter") {
		t.Fatalf("metadata repair dedupe should require OG/Twitter conflict checks, got %q", dedupe)
	}

	tests, ok := payload.AIRepair["acceptance_tests"].([]any)
	if !ok || len(tests) == 0 {
		t.Fatalf("metadata repair JSON must include acceptance tests: %#v", payload.AIRepair["acceptance_tests"])
	}
	testTexts := make([]string, 0, len(tests))
	for _, entry := range tests {
		testTexts = append(testTexts, entry.(string))
	}
	joined := strings.Join(testTexts, "\n")
	for _, want := range []string{
		`<title> equals "UniPost | Social Media Posting API for Developers"`,
		`meta[name="description"] equals "UniPost gives developers one API to connect customer social accounts, upload media, schedule posts, and publish across major social platforms."`,
		`canonical URL remains "https://unipost.dev/"`,
		"page remains indexable",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("metadata acceptance tests missing %q in %s", want, joined)
		}
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

func TestImprovePageActionsUsePageUpdateDraftsInsteadOfTopics(t *testing.T) {
	improvePage := WorkTypeImprovePage
	pageUpdate := "page_update"
	action := db.ContentAction{
		ActionType: "Strengthen the evidence block on this existing page",
		AssetType:  &pageUpdate,
		WorkType:   &improvePage,
	}
	if contentActionCreatesContent(action) {
		t.Fatal("Improve Page actions must not enter the Topic/Article creation path")
	}
	if contentActionNeedsTopic(pageUpdate, action.ActionType) {
		t.Fatal("page_update actions must create Page Update Drafts instead of Topics")
	}

	serverRaw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	serverSource := string(serverRaw)
	for _, want := range []string{
		`r.Post("/actions/{actionID}/page-update-drafts", s.createPageUpdateDraftForAction)`,
		`r.Get("/page-update-drafts/{draftID}", s.getPageUpdateDraft)`,
		`r.Post("/page-update-drafts/{draftID}/generate", s.generatePageUpdateDraft)`,
		`r.Post("/page-update-drafts/{draftID}/approve", s.approvePageUpdateDraft)`,
		`r.Post("/page-update-drafts/{draftID}/apply", s.applyPageUpdateDraft)`,
		`r.Post("/page-update-drafts/{draftID}/verify", s.verifyPageUpdateDraft)`,
	} {
		if !strings.Contains(serverSource, want) {
			t.Fatalf("Page Update Draft route missing %q", want)
		}
	}

	handlerRaw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	handlerSource := string(handlerRaw)
	for _, want := range []string{
		"func (s *Server) createPageUpdateDraftForAction",
		"func (s *Server) generatePageUpdateDraft",
		"func (s *Server) approvePageUpdateDraft",
		"func (s *Server) applyPageUpdateDraft",
		"func (s *Server) verifyPageUpdateDraft",
		"CreateOrReusePageUpdateDraft",
		"UpdatePageUpdateDraftStatus",
		"MarkContentActionVerification",
	} {
		if !strings.Contains(handlerSource, want) {
			t.Fatalf("Page Update Draft handler contract missing %q", want)
		}
	}
}

func TestPageUpdateExactSourceMappingRequiresPublishedMDXArticle(t *testing.T) {
	path := "content/citeloop/blog/evidence.mdx"
	article := db.Article{
		ID:            uuid.New(),
		ProjectID:     uuid.New(),
		PublishPath:   &path,
		PublishResult: []byte(`{"path":"content/citeloop/blog/evidence.mdx","commit_sha":"base-commit-sha"}`),
	}
	mapping, ok := pageUpdateExactSourceMapping(article)
	if !ok {
		t.Fatal("expected published MDX article to resolve exact source mapping")
	}
	if mapping.SourceFilePath != path || mapping.BaseCommitSHA != "base-commit-sha" || mapping.Confidence != "exact" {
		t.Fatalf("unexpected mapping: %#v", mapping)
	}

	article.PublishPath = nil
	article.PublishResult = []byte(`{"path":"content/citeloop/blog/evidence.mdx","commit_sha":"base-commit-sha"}`)
	mapping, ok = pageUpdateExactSourceMapping(article)
	if !ok || mapping.SourceFilePath != path {
		t.Fatalf("expected publish_result.path fallback, got ok=%v mapping=%#v", ok, mapping)
	}

	tsxPath := "app/blog/evidence/page.tsx"
	article.PublishPath = &tsxPath
	article.PublishResult = []byte(`{"commit_sha":"base-commit-sha"}`)
	if mapping, ok := pageUpdateExactSourceMapping(article); ok {
		t.Fatalf("non-MDX path must not be exact V1 mapping: %#v", mapping)
	}

	article.PublishPath = nil
	article.PublishResult = []byte(`{"commit_sha":"base-commit-sha"}`)
	if mapping, ok := pageUpdateExactSourceMapping(article); ok {
		t.Fatalf("article without publish path must not be exact mapping: %#v", mapping)
	}
}

func TestPageUpdateApplyCreatesSourceBackedGitHubPRApplication(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"GetEnabledPublisherConnectionForProject",
		"publisher.ConnectionKindGitHubNextJS",
		"pageUpdateExactSourceMapping",
		"CreateOrReuseSiteChangeApplication",
		`app.Status == "github_pr_open"`,
		"markPageUpdateDraftGitHubPRResult",
		"publisher.NewGitHubPRClient",
		"CreatePageUpdatePR",
		"MarkSiteChangeApplicationGitHubPR",
		`"github_pr"`,
		`"github_pr_open"`,
		`"manual_patch"`,
		`"manual_apply_required"`,
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("page update apply source-backed PR contract missing %q", want)
		}
	}
}
