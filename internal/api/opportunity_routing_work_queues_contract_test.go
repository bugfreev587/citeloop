package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
)

func oppOfType(oppType string, recommended string) db.SeoOpportunity {
	opp := db.SeoOpportunity{Type: oppType}
	if recommended != "" {
		opp.RecommendedAction = &recommended
	}
	return opp
}

func TestWorkTypeForOpportunityMatchesPRDMapping(t *testing.T) {
	// PRD-CiteLoop-Opportunity-Review-and-Work-Queues §6.1.
	cases := map[string]string{
		"gsc_low_ctr_query":                      WorkTypeImprovePage,
		"gsc_striking_distance_query":            WorkTypeImprovePage,
		"gsc_content_decay":                      WorkTypeImprovePage,
		"thin_evidence_page":                     WorkTypeImprovePage,
		"gsc_query_cannibalization":              WorkTypeImprovePage,
		"gsc_query_gap":                          WorkTypeCreateContent,
		"cold_start_context_plan":                WorkTypeCreateContent,
		"cold_start_competitive_gap":             WorkTypeCreateContent,
		"cold_start_evidence_page":               WorkTypeCreateContent,
		"geo_competitor_cited_project_absent":    WorkTypeCreateContent,
		"geo_project_mentioned_without_citation": WorkTypeCreateContent,
		"internal_link_gap":                      WorkTypeFixSiteIssue,
		"schema_gap":                             WorkTypeFixSiteIssue,
		"technical_visibility_issue":             WorkTypeFixSiteIssue,
		"geo_crawler_access_blocked":             WorkTypeFixSiteIssue,
	}
	for oppType, want := range cases {
		if got := workTypeForOpportunity(oppOfType(oppType, "")); got != want {
			t.Fatalf("workTypeForOpportunity(%s) = %s, want %s", oppType, got, want)
		}
	}
}

func TestWorkTypeOverrideAllowancesFollowPRD(t *testing.T) {
	// §6.2: content-route opportunities may switch between Create Content and
	// Improve Page; technically certain site fixes stay locked.
	dual := oppOfType("gsc_query_gap", "")
	allowed := allowedWorkTypesForOpportunity(dual)
	if len(allowed) != 2 || !workTypeAllowed(dual, WorkTypeImprovePage) || !workTypeAllowed(dual, WorkTypeCreateContent) {
		t.Fatalf("gsc_query_gap must allow create_content and improve_page, got %v", allowed)
	}
	if workTypeAllowed(dual, WorkTypeFixSiteIssue) {
		t.Fatal("content opportunities must not route to Site Fixes")
	}
	fix := oppOfType("schema_gap", "")
	if got := allowedWorkTypesForOpportunity(fix); len(got) != 1 || got[0] != WorkTypeFixSiteIssue {
		t.Fatalf("schema_gap must be locked to fix_site_issue, got %v", got)
	}
}

func TestAcceptOpportunityStampsApprovalAndRoutingSource(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"recommendedWorkType := workTypeForOpportunity(opp)",
		"RoutingSourceUserOverride",
		`"work_type not allowed for this opportunity"`,
		"ApprovalSource:          ApprovalSourceHumanReview",
		"RoutingSource:           routingSource",
		"WakeDueSnoozedSEOOpportunities",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("opportunity accept path missing %q", want)
		}
	}
}

func TestContentBriefPlanningUsesSplitMetadataRuleAndPublishStrategy(t *testing.T) {
	if !contentActionCreatesContent(db.ContentAction{AssetType: strPtr("metadata_rewrite"), ActionType: "rewrite title and meta description", WorkType: strPtr(WorkTypeImprovePage)}) {
		t.Fatal("editorial metadata refresh should create a content brief")
	}
	if contentActionCreatesContent(db.ContentAction{AssetType: strPtr("metadata_rewrite"), ActionType: "mechanical metadata patch", WorkType: strPtr(WorkTypeFixSiteIssue)}) {
		t.Fatal("metadata routed as a site fix should not create a content brief")
	}

	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"PublishStrategy string `json:\"publish_strategy\"`",
		"topicFromContentAction(projectID, action, opp, requestedPublishStrategy)",
		"Channel:               publishStrategyForContentAction(action, opp, requestedPublishStrategy)",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("content brief planning missing %q", want)
		}
	}
	if strings.Contains(source, `Channel:               "blog"`) {
		t.Fatal("manual opportunity planning must not hardcode blog")
	}
}

func TestAutopilotActionsCarryPolicyApprovalSource(t *testing.T) {
	raw, err := os.ReadFile("handlers_autopilot.go")
	if err != nil {
		t.Fatalf("read handlers_autopilot.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"ApprovalSource:          ApprovalSourceAutopilotPolicy",
		"RoutingSource:           RoutingSourcePolicy",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("autopilot execution path missing %q", want)
		}
	}
}

func TestOpportunityWorkQueuesSchemaAndRoutes(t *testing.T) {
	migration, err := os.ReadFile(filepath.Join("..", "migrations", "0038_opportunity_review_work_queues.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	migrationSQL := strings.ToLower(string(migration))
	for _, want := range []string{
		"add column if not exists approval_source",
		"add column if not exists routing_source",
		"add column if not exists work_type",
		"'human_review','autopilot_policy','manual','retry_recovery','admin_import'",
		"'system_recommendation','user_override','policy'",
		"add column if not exists snoozed_until",
		"add column if not exists snooze_reason",
		"add column if not exists unsnoozed_at",
		"create table if not exists seo_watchlist_items",
		"observation_window_days int not null default 28",
		"due_at timestamptz not null",
		"'watching','due_for_review','learned','closed'",
	} {
		if !strings.Contains(migrationSQL, want) {
			t.Fatalf("migration 0038 missing %q", want)
		}
	}

	serverRaw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	routes := string(serverRaw)
	for _, want := range []string{
		`r.Post("/topics", s.createTopic)`,
		`r.Post("/opportunities/{opportunityID}/snooze", s.snoozeSEOOpportunity)`,
		`r.Post("/opportunities/{opportunityID}/unsnooze", s.unsnoozeSEOOpportunity)`,
		`r.Post("/opportunities/{opportunityID}/watch", s.watchSEOOpportunity)`,
		`r.Get("/watchlist", s.listSEOWatchlist)`,
		`r.Post("/watchlist/{watchlistItemID}/close", s.closeSEOWatchlistItem)`,
	} {
		if !strings.Contains(routes, want) {
			t.Fatalf("server routes missing %q", want)
		}
	}
}

func TestWatchDecisionCreatesWatchlistItemNotExecutionItem(t *testing.T) {
	raw, err := os.ReadFile("handlers_opportunity_routing.go")
	if err != nil {
		t.Fatalf("read handlers_opportunity_routing.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"CreateSEOWatchlistItem",
		`Status: "watching"`,
		"MarkDueSEOWatchlistItems",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("watch handler missing %q", want)
		}
	}
	if strings.Contains(source, "CreateContentAction") {
		t.Fatal("watch decision must not create an execution item (PRD 14.5.1)")
	}
}
