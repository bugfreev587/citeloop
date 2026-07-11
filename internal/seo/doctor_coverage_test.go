package seo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestDoctorReportsBrokenOptimizationHealthy(t *testing.T) {
	projectID := uuid.New()
	present := "present"
	missing := "missing"
	statusOK := int32(200)
	oneLink := int32(1)
	readableTitle := "CiteLoop Doctor"
	duplicateTitle := "Shared Product Template"
	checks := []db.TechnicalCheck{
		fullyCheckedDoctorPage("https://example.com/healthy", &statusOK, &present, &oneLink, map[string]any{"page_title": readableTitle}),
		fullyCheckedDoctorPage("https://example.com/broken", &statusOK, &present, &oneLink, map[string]any{"page_title": "Broken canonical page"}),
		fullyCheckedDoctorPage("https://example.com/duplicate-a", &statusOK, &present, &oneLink, map[string]any{
			"page_title": duplicateTitle, "primary_intent": "describe product A", "existing_propositions": []string{"Product A has an existing feature."},
		}),
		fullyCheckedDoctorPage("https://example.com/duplicate-b", &statusOK, &present, &oneLink, map[string]any{
			"page_title": duplicateTitle, "primary_intent": "describe product B", "existing_propositions": []string{"Product B has an existing feature."},
		}),
		fullyCheckedDoctorPage("https://example.com/citation", &statusOK, &present, &oneLink, map[string]any{
			"page_title":     strings.Repeat("Readable metadata needs a deterministic correction ", 3),
			"primary_intent": "explain the existing Doctor product",
			"citation_readiness": map[string]any{
				"preserved_propositions":        []string{"Doctor checks the live site."},
				"added_propositions":            []string{},
				"removed_propositions":          []string{},
				"source_association_changes":    []map[string]any{{"proposition": "Doctor checks the live site.", "source": "https://example.com/source"}},
				"supported_fact_extractability": "needs_optimization",
				"source_association_status":     "missing_visible_association",
				"entity_name":                   "Cite Loop",
				"canonical_entity_name":         "CiteLoop",
			},
		}),
		fullyCheckedDoctorPage("https://example.com/partial", &statusOK, nil, &oneLink, map[string]any{"crawl_status": "partial"}),
	}
	checks[1].CanonicalStatus = &missing

	findings, rerouted := doctorFindingCandidatesAndGrowthFromChecks(projectID, checks)
	if len(rerouted) != 0 {
		t.Fatalf("preservation-safe examples should remain Doctor optimizations, rerouted=%#v", rerouted)
	}
	byIssue := map[string]doctorFindingCandidate{}
	for _, finding := range findings {
		byIssue[finding.IssueType] = finding
		if finding.IssueType == "healthy" || finding.Evidence["finding_kind"] == "healthy" {
			t.Fatalf("healthy coverage must never be an actionable finding: %#v", finding)
		}
	}
	if got := byIssue["canonical_missing"].Evidence["finding_kind"]; got != "broken" {
		t.Fatalf("broken finding kind = %#v, want broken", got)
	}
	for _, issue := range []string{"metadata_readability", "duplicate_metadata_template", "supported_fact_extractability", "source_association", "entity_naming_consistency"} {
		finding, ok := byIssue[issue]
		if !ok {
			t.Fatalf("missing deterministic optimization %q in %#v", issue, findings)
		}
		if finding.Evidence["finding_kind"] != "optimization" {
			t.Fatalf("%s finding kind = %#v, want optimization", issue, finding.Evidence["finding_kind"])
		}
		if added := stringSliceEvidence(finding.Evidence["added_propositions"]); len(added) != 0 {
			t.Fatalf("%s added propositions = %#v, want none", issue, added)
		}
		if finding.Evidence["primary_intent_before"] == "" || finding.Evidence["primary_intent_before"] != finding.Evidence["primary_intent_after"] {
			t.Fatalf("%s must preserve a non-empty primary intent: %#v", issue, finding.Evidence)
		}
		if preserved := stringSliceEvidence(finding.Evidence["preserved_propositions"]); len(preserved) == 0 {
			t.Fatalf("%s must carry the existing proposition set it preserves: %#v", issue, finding.Evidence)
		}
		for _, field := range []string{"preserved_propositions", "added_propositions", "removed_propositions", "source_association_changes"} {
			if _, ok := finding.Evidence[field]; !ok {
				t.Fatalf("%s missing proposition-preservation field %q: %#v", issue, field, finding.Evidence)
			}
		}
	}

	coverage := buildDoctorHealthyCoverage(checks, nil)
	canonical := coverageByCheck(coverage)["canonical"]
	assertURLInCoverage(t, canonical.CheckedURLs, "https://example.com/healthy", "checked")
	assertURLInCoverage(t, canonical.PassedURLs, "https://example.com/healthy", "passed")
	assertURLInCoverage(t, canonical.FailedURLs, "https://example.com/broken", "failed")
	assertURLInCoverage(t, canonical.SkippedURLs, "https://example.com/partial", "skipped")
	if containsCoverageURL(canonical.PassedURLs, "https://example.com/partial") {
		t.Fatal("partial-crawl URL must never be healthy")
	}
	if doctorCoverageComplete(coverage) {
		t.Fatal("coverage containing failed or skipped URLs must not display healthy")
	}
}

func TestDoctorCitationOptimizationFailsClosedToGrowthWhenAddingFacts(t *testing.T) {
	present := "present"
	statusOK := int32(200)
	oneLink := int32(1)
	check := fullyCheckedDoctorPage("https://example.com/citation", &statusOK, &present, &oneLink, map[string]any{
		"page_title":     strings.Repeat("Citation metadata readability correction ", 3),
		"primary_intent": "explain the citation product",
		"citation_readiness": map[string]any{
			"preserved_propositions":        []string{"Existing supported fact."},
			"added_propositions":            []string{"New fact from a source that is not on the live page."},
			"removed_propositions":          []string{},
			"source_association_changes":    []map[string]any{},
			"supported_fact_extractability": "needs_optimization",
		},
	})

	findings, rerouted := doctorFindingCandidatesAndGrowthFromChecks(uuid.New(), []db.TechnicalCheck{check})
	for _, finding := range findings {
		if finding.FindingKind == "optimization" {
			t.Fatalf("fact-adding proposal must fail closed out of every Doctor optimization: %#v", finding)
		}
	}
	if len(rerouted) != 1 || rerouted[0].Type != "citation_fact_expansion" {
		t.Fatalf("rerouted growth candidates = %#v, want citation_fact_expansion", rerouted)
	}
}

func TestDoctorConsumesAuthoritativeCrawlerAccessEvidence(t *testing.T) {
	snapshot := db.AiCrawlerAccessSnapshot{
		PageUrl:           "https://example.com/blocked",
		NormalizedPageUrl: "/blocked",
		TargetUserAgent:   "OAI-SearchBot",
		EvidenceType:      "robots_static",
		RobotsState:       "disallowed",
		AccessState:       "blocked",
		Confidence:        "high",
	}
	findings := doctorFindingCandidatesFromCrawlerAccess(uuid.New(), []db.AiCrawlerAccessSnapshot{snapshot})
	if len(findings) != 1 || findings[0].IssueType != "geo_crawler_access_blocked" || findings[0].FindingKind != "broken" {
		t.Fatalf("crawler-access Doctor findings = %#v", findings)
	}
	coverage := buildDoctorHealthyCoverage(nil, []db.AiCrawlerAccessSnapshot{snapshot})
	geoCoverage := coverageByCheck(coverage)["geo_crawler_access"]
	assertURLInCoverage(t, geoCoverage.CheckedURLs, snapshot.PageUrl, "checked")
	assertURLInCoverage(t, geoCoverage.FailedURLs, snapshot.PageUrl, "failed")

	inferred := snapshot
	inferred.PageUrl = "https://example.com/inferred"
	inferred.NormalizedPageUrl = "/inferred"
	inferred.EvidenceType = "honest_probe"
	inferred.AccessState = "challenge"
	inferred.Confidence = "medium"
	inferred.Inferred = true
	if got := doctorFindingCandidatesFromCrawlerAccess(uuid.New(), []db.AiCrawlerAccessSnapshot{inferred}); len(got) != 0 {
		t.Fatalf("inferred crawler warning must remain non-actionable evidence: %#v", got)
	}
	inferredCoverage := coverageByCheck(buildDoctorHealthyCoverage(nil, []db.AiCrawlerAccessSnapshot{inferred}))["geo_crawler_access"]
	assertURLInCoverage(t, inferredCoverage.SkippedURLs, inferred.PageUrl, "skipped")
}

func TestDoctorPersistenceContractsWriteFindingKindAndHealthyCoverage(t *testing.T) {
	candidate := doctorFindingCandidate{IssueType: "canonical_missing", Evidence: map[string]any{}}.withDefaults()
	raw, err := json.Marshal(candidate.upsertParams(uuid.New(), uuid.New(), pgtype.Timestamptz{}))
	if err != nil {
		t.Fatalf("marshal finding params: %v", err)
	}
	if !strings.Contains(string(raw), `"finding_kind":"broken"`) {
		t.Fatalf("upsert params do not persist finding_kind: %s", raw)
	}

	sql, err := os.ReadFile("../db/queries/seo.sql")
	if err != nil {
		t.Fatalf("read seo queries: %v", err)
	}
	text := strings.ToLower(string(sql))
	if !strings.Contains(text, "finding_kind = excluded.finding_kind") {
		t.Fatal("UpsertSEODoctorFinding must persist finding_kind on insert and update")
	}
	if !strings.Contains(text, "healthy_coverage = sqlc.arg(healthy_coverage)::jsonb") {
		t.Fatal("CompleteSEODoctorRun must persist structured healthy coverage")
	}
}

func TestProductionTechnicalAndGEOEvidenceCanProduceCompleteHealthyCoverage(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/robots.txt":
			_, _ = w.Write([]byte("User-agent: OAI-SearchBot\nAllow: /\n"))
		case "/sitemap.xml":
			_, _ = w.Write([]byte(`<urlset><url><loc>` + server.URL + `/healthy</loc></url></urlset>`))
		default:
			_, _ = w.Write([]byte(`<!doctype html><html><head><title>Healthy page</title><meta name="description" content="Healthy description"><link rel="canonical" href="` + server.URL + `/healthy"><script type="application/ld+json">{"name":"Healthy"}</script></head><body><h1>Healthy page</h1><a href="/about">About</a></body></html>`))
		}
	}))
	defer server.Close()

	pageURL := server.URL + "/healthy"
	result := (Service{HTTPClient: server.Client()}).checkURL(context.Background(), pageURL, server.URL)
	params := technicalCheckParams(uuid.New(), uuid.New(), pageURL, pageURL, pgtype.UUID{}, nil, false, result)
	if params.RobotsStatus == nil || *params.RobotsStatus != "index" || params.SitemapStatus == nil || *params.SitemapStatus != "present" {
		t.Fatalf("production mapping robots=%v sitemap=%v result=%+v", params.RobotsStatus, params.SitemapStatus, result)
	}
	check := technicalCheckFromParams(params)

	snapshots := []db.AiCrawlerAccessSnapshot{{PageUrl: pageURL, NormalizedPageUrl: pageURL, EvidenceType: "robots_static", RobotsState: "allowed", AccessState: "allowed", Confidence: "high", Inferred: true}}
	coverage := buildDoctorHealthyCoverage([]db.TechnicalCheck{check}, snapshots)
	if !doctorCoverageComplete(coverage) {
		t.Fatalf("coverage should be complete healthy: %#v", coverage)
	}
	if findings := doctorFindingCandidatesFromChecks(uuid.New(), []db.TechnicalCheck{check}); len(findings) != 0 {
		t.Fatalf("healthy production check emitted findings: %#v", findings)
	}
}

func TestLatestTechnicalCheckCrawlCompletenessControlsResolutionAndHealth(t *testing.T) {
	serviceSource, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serviceSource), "articles = articles[:1000]") {
		t.Fatal("SEO Sync must not silently truncate the technical crawl at 1,000 articles")
	}
	if !latestTechnicalCheckCrawlComplete("ok", 1001, 1001, 0, false) {
		t.Fatal("a successful 1,001-page crawl must be complete")
	}
	for _, tc := range []struct {
		name     string
		status   string
		expected int64
		loaded   int
		failed   int64
		bounded  bool
	}{
		{name: "partial page load", status: "ok", expected: 1001, loaded: 1000},
		{name: "failed latest run", status: "error", expected: 1001, loaded: 1001},
		{name: "failed page fetch", status: "ok", expected: 1001, loaded: 1001, failed: 1},
		{name: "bounded latest run", status: "ok", expected: 10001, loaded: 10000, bounded: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if latestTechnicalCheckCrawlComplete(tc.status, tc.expected, tc.loaded, tc.failed, tc.bounded) {
				t.Fatal("incomplete latest crawl reported complete")
			}
			coverage := appendCrawlCompletenessCoverage(nil, false)
			if doctorCoverageComplete(coverage) {
				t.Fatal("incomplete latest crawl reported healthy coverage")
			}
		})
	}
}

func fullyCheckedDoctorPage(url string, status *int32, present *string, links *int32, details map[string]any) db.TechnicalCheck {
	raw, _ := json.Marshal(details)
	return db.TechnicalCheck{PageUrl: url, NormalizedPageUrl: url, HttpStatus: status, CanonicalStatus: present, RobotsStatus: present, TitleStatus: present, MetaDescriptionStatus: present, H1Status: present, StructuredDataStatus: present, SitemapStatus: present, InternalLinkCount: links, RawDetails: raw}
}

func technicalCheckFromParams(params db.UpsertTechnicalCheckParams) db.TechnicalCheck {
	return db.TechnicalCheck{PageUrl: params.PageUrl, NormalizedPageUrl: params.NormalizedPageUrl, HttpStatus: params.HttpStatus, CanonicalStatus: params.CanonicalStatus, RobotsStatus: params.RobotsStatus, TitleStatus: params.TitleStatus, MetaDescriptionStatus: params.MetaDescriptionStatus, H1Status: params.H1Status, StructuredDataStatus: params.StructuredDataStatus, SitemapStatus: params.SitemapStatus, InternalLinkCount: params.InternalLinkCount, OutboundLinkCount: params.OutboundLinkCount, RawDetails: params.RawDetails}
}

func coverageByCheck(values []doctorCheckCoverage) map[string]doctorCheckCoverage {
	out := map[string]doctorCheckCoverage{}
	for _, value := range values {
		out[value.Check] = value
	}
	return out
}

func assertURLInCoverage(t *testing.T, values []string, want, label string) {
	t.Helper()
	if !containsCoverageURL(values, want) {
		t.Fatalf("%s URLs = %#v, missing %s", label, values, want)
	}
}

func containsCoverageURL(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stringSliceEvidence(value any) []string {
	if values, ok := value.([]string); ok {
		return values
	}
	values := []string{}
	for _, item := range value.([]any) {
		values = append(values, item.(string))
	}
	return values
}
