package api

import (
	"encoding/json"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
)

func finding(category, issueType, fixIntent string) db.SeoDoctorFinding {
	return db.SeoDoctorFinding{
		Category:  category,
		IssueType: issueType,
		FixIntent: fixIntent,
	}
}

// directActionAssetTypes mirrors the frontend predicate (lib/site-fix.ts):
// an action is a Site Fix when its asset type is one of these.
var directActionAssetTypes = map[string]bool{
	"internal_link_patch": true,
	"schema_patch":        true,
	"sitemap_update":      true,
	"technical_fix":       true,
	"metadata_rewrite":    true, // routed via technical_task output_type
}

func TestDoctorFindingOpportunityTypeAlwaysRoutesToSiteFix(t *testing.T) {
	cases := []db.SeoDoctorFinding{
		finding("structured_data", "structured_data_missing", "Add JSON-LD"),
		finding("links", "internal_link_gap", "Add internal links"),
		finding("technical", "canonical_conflict", "Fix canonical"),
		finding("crawl", "robots_blocked", "Unblock crawler"),
		finding("geo", "geo_crawler_access_blocked", "Allow answer engine crawlers"),
		finding("other", "mystery_issue", ""),
	}
	for _, f := range cases {
		oppType := doctorFindingOpportunityType(f)
		if !fixSiteIssueOpportunityTypes[oppType] {
			t.Fatalf("doctorFindingOpportunityType(%q/%q) = %q, which is not a fix-site-issue type", f.Category, f.IssueType, oppType)
		}
		// The synthesized opportunity must route to Site Fixes.
		if got := workTypeForOpportunity(db.SeoOpportunity{Type: oppType}); got != WorkTypeFixSiteIssue {
			t.Fatalf("opportunity type %q routed to %q, want %q", oppType, got, WorkTypeFixSiteIssue)
		}
	}
}

func TestDoctorFindingAssetTypeIsDirectAction(t *testing.T) {
	cases := map[string]db.SeoDoctorFinding{
		"schema_patch":        finding("structured_data", "structured_data_missing", "Add JSON-LD schema"),
		"internal_link_patch": finding("links", "internal_link_gap", "Add internal links"),
		"sitemap_update":      finding("crawl", "sitemap_missing_url", "Add URL to sitemap"),
		"metadata_rewrite":    finding("metadata", "title_tag_missing", "Rewrite the meta description"),
		"technical_fix":       finding("technical", "canonical_conflict", "Fix canonical tag"),
	}
	for want, f := range cases {
		got := doctorFindingAssetType(f)
		if got != want {
			t.Fatalf("doctorFindingAssetType(%q) = %q, want %q", f.IssueType, got, want)
		}
		if !directActionAssetTypes[got] {
			t.Fatalf("asset type %q is not a direct-action (Site Fix) asset type", got)
		}
	}
}

func TestFirstDoctorFindingURL(t *testing.T) {
	if got := firstDoctorFindingURL(json.RawMessage(`["", "  ", "https://example.com/page"]`)); got != "https://example.com/page" {
		t.Fatalf("firstDoctorFindingURL skipped blanks incorrectly: %q", got)
	}
	if got := firstDoctorFindingURL(json.RawMessage(`[]`)); got != "" {
		t.Fatalf("firstDoctorFindingURL(empty) = %q, want empty", got)
	}
	if got := firstDoctorFindingURL(json.RawMessage(`not json`)); got != "" {
		t.Fatalf("firstDoctorFindingURL(invalid) = %q, want empty", got)
	}
}

func TestDoctorFindingDiffSnapshotCarriesFixContent(t *testing.T) {
	f := finding("structured_data", "structured_data_missing", "Add JSON-LD")
	f.DeveloperInstructions = "Add server-rendered JSON-LD to the homepage."
	f.LikelyFilesOrSurfaces = json.RawMessage(`["app/layout.tsx"]`)
	f.AcceptanceTests = json.RawMessage(`["Validate with Rich Results Test"]`)

	var snapshot map[string]any
	if err := json.Unmarshal(doctorFindingDiffSnapshot(f), &snapshot); err != nil {
		t.Fatalf("diff snapshot is not valid JSON: %v", err)
	}
	if snapshot["output_type"] != "technical_task" {
		t.Fatalf("diff snapshot output_type = %v, want technical_task (so isDirectAction is true)", snapshot["output_type"])
	}
	changes, ok := snapshot["proposed_changes"].([]any)
	if !ok || len(changes) != 1 {
		t.Fatalf("diff snapshot missing proposed_changes: %v", snapshot["proposed_changes"])
	}
	if _, ok := snapshot["acceptance_tests"]; !ok {
		t.Fatalf("diff snapshot dropped acceptance_tests")
	}
}
