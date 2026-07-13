package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func TestSEOActionReturnDismissContracts(t *testing.T) {
	serverRaw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	routes := string(serverRaw)
	for _, want := range []string{
		`r.Post("/actions/{actionID}/return-to-opportunity", s.returnSEOContentActionToOpportunity)`,
		`r.Post("/actions/{actionID}/dismiss", s.dismissSEOContentActionAndOpportunity)`,
	} {
		if !strings.Contains(routes, want) {
			t.Fatalf("server routes missing action lifecycle route %q", want)
		}
	}

	handlerRaw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(handlerRaw)
	for _, want := range []string{
		"func (s *Server) returnSEOContentActionToOpportunity",
		"func (s *Server) dismissSEOContentActionAndOpportunity",
		"MarkContentActionReturnedToOpportunity",
		"DismissSEOContentActionAndOpportunity",
		"CreateOrUpdateSEOOpportunityReviewState",
		"returned_to_opportunities",
		"opportunity dismissed",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("SEO action lifecycle handler contract missing %q", want)
		}
	}
}

func TestWriteContentActionMutationErrorClassifiesNoRowsSeparately(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{"irreversible action", pgx.ErrNoRows, http.StatusNotFound, "action not found or no longer reversible"},
		{"database failure", errors.New("column does not exist"), http.StatusInternalServerError, "could not move action back to Opportunities"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeContentActionMutationError(recorder, tt.err, "action not found or no longer reversible", "could not move action back to Opportunities")
			if recorder.Code != tt.wantStatus || !strings.Contains(recorder.Body.String(), tt.wantBody) {
				t.Fatalf("response status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
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

func TestSiteFixGitHubPRRouteAndHandlerContract(t *testing.T) {
	serverRaw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	if !strings.Contains(string(serverRaw), `r.Post("/actions/{actionID}/site-fix-pr", s.createSiteFixGitHubPR)`) {
		t.Fatal("Site Fixes must expose a GitHub PR apply endpoint")
	}

	handlerRaw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(handlerRaw)
	for _, want := range []string{
		"func (s *Server) createSiteFixGitHubPR",
		"generateSiteFixAIProposal",
		"buildSiteFixAIContract",
		"PurposeSiteFix",
		"content action is not a site fix",
		`"site_fix"`,
		"CreateOrReuseSiteChangeApplication",
		"CreatePageUpdatePR",
		"siteFixWorkingBranch",
		"siteFixPRBody",
		"siteFixGitHubPRResult",
		"verification_pending",
		"github_pr_open",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("site fix GitHub PR handler contract missing %q", want)
		}
	}
}

func TestSiteFixMetadataRewriteUsesAIProposalPublisherResult(t *testing.T) {
	action := db.ContentAction{
		ActionType: "Rewrite homepage title and meta description for query relevance",
		AssetType:  strPtrFrom("metadata_rewrite"),
		OutputSnapshot: json.RawMessage(`{
			"publisher_result": {
				"status": "ai_preview_ready",
				"ai_fix_contract_hash": "sha256-test",
				"ai_proposal": {
					"proposed_change": {
						"title": "UniPost | Social Publishing API for Product Teams",
						"meta_description": "Plan social publishing API work with evidence-backed workflows, review gates, and source-controlled GitHub changes."
					},
					"evidence_alignment": ["Keeps the existing production URL", "Rewrites crawler-facing metadata only"]
				}
			}
		}`),
	}
	source := `export const metadata = {
  title: "Old title",
  description: "Old description",
};
`

	updated, err := siteFixMetadataRewriteContent(source, action)
	if err != nil {
		t.Fatalf("siteFixMetadataRewriteContent returned error: %v", err)
	}
	for _, want := range []string{
		`title: "UniPost | Social Publishing API for Product Teams"`,
		`description: "Plan social publishing API work with evidence-backed workflows, review gates, and source-controlled GitHub changes."`,
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("AI proposal metadata missing %q:\n%s", want, updated)
		}
	}
}

func TestBuildSiteFixAIContractCapturesNoProposedMetadataCase(t *testing.T) {
	action := db.ContentAction{
		ActionType: "Rewrite homepage title and meta description for query relevance",
		AssetType:  strPtrFrom("metadata_rewrite"),
		TargetUrl:  strPtrFrom("https://unipost.dev/"),
		OutputSnapshot: json.RawMessage(`{
			"title": "Rewrite homepage title and meta description for query relevance",
			"asset_type": "metadata_rewrite",
			"deliverable": "Title and meta description patch for an existing page.",
			"proposed_change": {
				"preserve": ["canonical", "indexability", "production URL"]
			}
		}`),
	}
	contract := buildSiteFixAIContract(action)

	if contract.Hash == "" {
		t.Fatal("contract hash should be stable and persisted with the AI proposal")
	}
	if contract.TargetURL != "https://unipost.dev/" {
		t.Fatalf("target url = %q", contract.TargetURL)
	}
	if contract.Observed["proposed_title"] != "" || contract.Observed["proposed_meta_description"] != "" {
		t.Fatalf("generic opportunity copy must not be treated as proposed metadata: %#v", contract.Observed)
	}
	for _, want := range []string{"Return concrete title and meta_description values.", "Do not create a new page or blog post."} {
		if !containsString(contract.Constraints, want) {
			t.Fatalf("contract constraints missing %q: %#v", want, contract.Constraints)
		}
	}
}

func TestContentActionIsSiteFixAcceptsDirectExistingPagePatches(t *testing.T) {
	if !contentActionIsSiteFix(db.ContentAction{AssetType: strPtrFrom("metadata_rewrite"), WorkType: strPtrFrom(WorkTypeImprovePage)}) {
		t.Fatal("metadata rewrites routed as Improve Page should still be applyable as Site Fix PRs")
	}
	if contentActionIsSiteFix(db.ContentAction{AssetType: strPtrFrom("page_update"), WorkType: strPtrFrom(WorkTypeImprovePage)}) {
		t.Fatal("page update drafts must stay on the Page Update flow")
	}
	if contentActionIsSiteFix(db.ContentAction{AssetType: strPtrFrom("blog_post"), WorkType: strPtrFrom(WorkTypeCreateContent)}) {
		t.Fatal("new content actions are not Site Fix PRs")
	}
}

func TestSiteFixMetadataRewriteUpdatesMDXFrontmatter(t *testing.T) {
	action := db.ContentAction{
		ActionType: "Rewrite homepage title and meta description",
		AssetType:  strPtrFrom("metadata_rewrite"),
		DiffSnapshot: json.RawMessage(`{
			"ai_repair": {
				"proposed_change": {
					"title": "UniPost | Social Media Posting API for Developers",
					"meta_description": "UniPost gives developers one API to connect customer social accounts, upload media, schedule posts, and publish across major social platforms."
				}
			}
		}`),
	}
	source := "---\nsource: citeloop\ntitle: \"Old title\"\nseo_title: \"Old SEO title\"\ndescription: \"Old description\"\nexcerpt: \"Old excerpt\"\ncanonical: \"https://unipost.dev/\"\n---\n\n# Body\n"

	updated, err := siteFixMetadataRewriteContent(source, action)
	if err != nil {
		t.Fatalf("siteFixMetadataRewriteContent returned error: %v", err)
	}
	for _, want := range []string{
		`title: "UniPost | Social Media Posting API for Developers"`,
		`seo_title: "UniPost | Social Media Posting API for Developers"`,
		`description: "UniPost gives developers one API to connect customer social accounts, upload media, schedule posts, and publish across major social platforms."`,
		`excerpt: "UniPost gives developers one API to connect customer social accounts, upload media, schedule posts, and publish across major social platforms."`,
		`canonical: "https://unipost.dev/"`,
		"# Body",
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated frontmatter missing %q:\n%s", want, updated)
		}
	}
	if strings.Contains(updated, "Old title") || strings.Contains(updated, "Old description") {
		t.Fatalf("old metadata values should be replaced:\n%s", updated)
	}
}

func TestSiteFixMetadataRewriteUpdatesNextMetadataConstants(t *testing.T) {
	action := db.ContentAction{
		ActionType: "Rewrite homepage title and meta description",
		AssetType:  strPtrFrom("metadata_rewrite"),
		DiffSnapshot: json.RawMessage(`{
			"ai_repair": {
				"proposed_change": {
					"title": "UniPost | Social Media Posting API for Developers",
					"meta_description": "UniPost gives developers one API to connect customer social accounts, upload media, schedule posts, and publish across major social platforms."
				}
			}
		}`),
	}
	source := `import type { Metadata } from "next";

const HOMEPAGE_TITLE = "Old homepage title";
const HOMEPAGE_SUBTITLE = "Keep this subtitle";
const HOMEPAGE_DESCRIPTION =
  "Old homepage description.";

export const metadata: Metadata = {
  title: HOMEPAGE_TITLE,
  description: HOMEPAGE_DESCRIPTION,
  alternates: { canonical: "https://unipost.dev/" },
};
`

	updated, err := siteFixMetadataRewriteContent(source, action)
	if err != nil {
		t.Fatalf("siteFixMetadataRewriteContent returned error: %v", err)
	}
	for _, want := range []string{
		`const HOMEPAGE_TITLE = "UniPost | Social Media Posting API for Developers";`,
		`const HOMEPAGE_SUBTITLE = "Keep this subtitle";`,
		`"UniPost gives developers one API to connect customer social accounts, upload media, schedule posts, and publish across major social platforms.";`,
		`canonical: "https://unipost.dev/"`,
	} {
		if !strings.Contains(updated, want) {
			t.Fatalf("updated Next metadata source missing %q:\n%s", want, updated)
		}
	}
	if strings.Contains(updated, "Old homepage title") || strings.Contains(updated, "Old homepage description") {
		t.Fatalf("old metadata constants should be replaced:\n%s", updated)
	}
}

func TestSiteFixMetadataRewriteUpdatesNextMetadataObjectStrings(t *testing.T) {
	action := db.ContentAction{
		ActionType: "Rewrite metadata",
		AssetType:  strPtrFrom("metadata_rewrite"),
		DiffSnapshot: json.RawMessage(`{
			"proposed_metadata": {
				"title": "New title",
				"description": "New description"
			}
		}`),
	}
	source := `export const metadata = {
  title: "Old title",
  description: "Old description",
  openGraph: {
    title: "Old title",
    description: "Old description",
  },
};
`

	updated, err := siteFixMetadataRewriteContent(source, action)
	if err != nil {
		t.Fatalf("siteFixMetadataRewriteContent returned error: %v", err)
	}
	if strings.Count(updated, `title: "New title"`) != 2 {
		t.Fatalf("expected title strings to be rewritten in metadata and openGraph:\n%s", updated)
	}
	if strings.Count(updated, `description: "New description"`) != 2 {
		t.Fatalf("expected description strings to be rewritten in metadata and openGraph:\n%s", updated)
	}
	if strings.Contains(updated, "Old title") || strings.Contains(updated, "Old description") {
		t.Fatalf("old metadata object strings should be replaced:\n%s", updated)
	}
}

func TestSiteFixMetadataRewriteRejectsOpportunityTitleWithoutExplicitMetadata(t *testing.T) {
	action := db.ContentAction{
		ActionType: "Rewrite homepage title and meta description for query relevance",
		AssetType:  strPtrFrom("metadata_rewrite"),
		OutputSnapshot: json.RawMessage(`{
			"title": "Rewrite homepage title and meta description for query relevance",
			"asset_type": "metadata_rewrite",
			"deliverable": "Title and meta description patch for an existing page.",
			"proposed_change": {
				"preserve": ["canonical", "indexability", "production URL"]
			}
		}`),
	}
	source := `const HOMEPAGE_TITLE = "UniPost | Social Media Posting API for Developers";
const HOMEPAGE_DESCRIPTION =
  "UniPost gives developers one API to connect customer social accounts, upload media, schedule posts, and publish across major social platforms.";
`

	_, err := siteFixMetadataRewriteContent(source, action)
	if err == nil {
		t.Fatal("siteFixMetadataRewriteContent should reject metadata rewrites without explicit proposed copy")
	}
	if !strings.Contains(err.Error(), "no proposed title or meta description") {
		t.Fatalf("expected missing proposed copy error, got %v", err)
	}
}

func TestResolveSiteFixGitHubPRSourceKeepsMissingMetadataError(t *testing.T) {
	action := db.ContentAction{
		ActionType: "Rewrite homepage title and meta description for query relevance",
		AssetType:  strPtrFrom("metadata_rewrite"),
		TargetUrl:  strPtrFrom("https://unipost.dev/"),
		OutputSnapshot: json.RawMessage(`{
			"title": "Rewrite homepage title and meta description for query relevance",
			"asset_type": "metadata_rewrite",
			"proposed_change": {
				"preserve": ["canonical", "indexability", "production URL"]
			}
		}`),
	}
	reader := fakeSiteFixSourceReader{
		files: map[string]string{
			"dashboard/src/app/marketing/page.tsx": `const HOMEPAGE_TITLE = "UniPost | Social Media Posting API for Developers";
const HOMEPAGE_DESCRIPTION = "UniPost gives developers one API.";
`,
		},
	}

	_, err := (&Server{}).resolveSiteFixGitHubPRSource(context.Background(), uuid.New(), action, publisher.GitHubNextJSConfig{
		ContentDir: "content/citeloop/blog",
		BaseURL:    "https://unipost.dev/blog",
		Branch:     "main",
	}, reader)
	if err == nil {
		t.Fatal("resolveSiteFixGitHubPRSource should reject missing proposed metadata copy")
	}
	if !strings.Contains(err.Error(), "no proposed title or meta description") {
		t.Fatalf("expected missing proposed metadata error, got %v", err)
	}
	if strings.Contains(err.Error(), "app/layout.tsx") {
		t.Fatalf("missing proposed metadata error should not be masked by later fallback 404s: %v", err)
	}
}

func TestSiteFixMetadataRewriteSourceCandidatesIncludeHomepageNextMetadata(t *testing.T) {
	action := db.ContentAction{
		ActionType: "Rewrite homepage title and meta description",
		AssetType:  strPtrFrom("metadata_rewrite"),
		TargetUrl:  strPtrFrom("https://unipost.dev/"),
	}
	candidates := siteFixMetadataRewriteSourceCandidates(action, publisher.GitHubNextJSConfig{
		ContentDir: "content/citeloop/blog",
		BaseURL:    "https://unipost.dev/blog",
	})
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		paths = append(paths, candidate.SourceFilePath)
	}
	for _, want := range []string{
		"dashboard/src/app/marketing/page.tsx",
		"dashboard/src/app/page.tsx",
		"dashboard/src/app/layout.tsx",
	} {
		if !containsString(paths, want) {
			t.Fatalf("homepage metadata candidates missing %q: %#v", want, paths)
		}
	}
}

func TestSiteFixMetadataRewriteSourceCandidatesIncludeBlogMarkdownFallback(t *testing.T) {
	action := db.ContentAction{
		ActionType: "Rewrite blog title and meta description",
		AssetType:  strPtrFrom("metadata_rewrite"),
		TargetUrl:  strPtrFrom("https://unipost.dev/blog/evidence-led-social-publishing-api-planning-brief"),
	}
	candidates := siteFixMetadataRewriteSourceCandidates(action, publisher.GitHubNextJSConfig{
		ContentDir: "content/citeloop/blog",
		BaseURL:    "https://unipost.dev/blog",
	})
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		paths = append(paths, candidate.SourceFilePath)
	}
	if !containsString(paths, "content/citeloop/blog/evidence-led-social-publishing-api-planning-brief.mdx") {
		t.Fatalf("blog metadata candidates missing content-dir MDX fallback: %#v", paths)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type fakeSiteFixSourceReader struct {
	files map[string]string
}

func (f fakeSiteFixSourceReader) ReadFile(_ context.Context, sourcePath, _ string) (string, string, error) {
	if content, ok := f.files[sourcePath]; ok {
		return content, "sha-" + sourcePath, nil
	}
	return "", "", errors.New("github content lookup 404: not found")
}
