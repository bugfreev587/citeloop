package scheduler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
)

func TestPageEmitsProposedMetadata(t *testing.T) {
	page := `<html><head><title>Unipost</title>
	<meta name="description" content="Unipost helps you publish &amp; manage posts across your channels."></head><body>hi</body></html>`

	if !pageEmitsProposedMetadata(page, "Unipost", "Unipost helps you publish & manage posts across your channels.") {
		t.Fatal("expected match when title and (entity-encoded) meta are present")
	}
	// Whitespace/case differences must not break the match.
	if !pageEmitsProposedMetadata(page, "  unipost  ", "") {
		t.Fatal("expected normalized title match")
	}
	// A proposed value not on the page fails.
	if pageEmitsProposedMetadata(page, "Different Title", "") {
		t.Fatal("expected no match for absent title")
	}
	// Both empty is never a trivial pass.
	if pageEmitsProposedMetadata(page, "", "") {
		t.Fatal("empty proposed values must not match")
	}
}

func TestSiteFixURLVerifiableAndProposedMetadata(t *testing.T) {
	app := db.SiteChangeApplication{
		ResolutionCriteria: json.RawMessage(`{"asset_type":"metadata_rewrite","target_url":"https://x.dev/"}`),
		PatchSnapshot:      json.RawMessage(`{"proposed_change":{"title":"New Title","meta_description":"New meta."}}`),
	}
	if !siteFixURLVerifiable(app) {
		t.Fatal("metadata_rewrite should be URL-verifiable")
	}
	title, meta := siteFixProposedMetadata(app)
	if title != "New Title" || meta != "New meta." {
		t.Fatalf("proposed metadata = %q / %q", title, meta)
	}

	fuzzy := db.SiteChangeApplication{ResolutionCriteria: json.RawMessage(`{"asset_type":"content_expansion"}`)}
	if siteFixURLVerifiable(fuzzy) {
		t.Fatal("content_expansion should not be URL-verifiable")
	}

	// Falls back to diff snapshot when patch snapshot lacks proposed_change.
	fallback := db.SiteChangeApplication{
		DiffSnapshot: json.RawMessage(`{"proposed_metadata":{"title":"T","meta_description":"M"}}`),
	}
	title, meta = siteFixProposedMetadata(fallback)
	if title != "T" || meta != "M" {
		t.Fatalf("fallback metadata = %q / %q", title, meta)
	}
}

func TestNextSiteFixVerifyPollAtStopsAtDeadline(t *testing.T) {
	merged := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	if next, ok := nextSiteFixVerifyPollAt(merged, merged); !ok || !next.Equal(merged.Add(5*time.Minute)) {
		t.Fatalf("first verify check = %v ok=%v", next, ok)
	}
	if _, ok := nextSiteFixVerifyPollAt(merged, merged.Add(siteFixVerifyDeadline)); ok {
		t.Fatal("expected verification give-up at the 24h deadline")
	}
}

func TestSiteFixVerifiedPublisherResultRecordsCompletedPRState(t *testing.T) {
	prNumber := int32(175)
	prURL := "https://github.com/bugfreev587/unipost/pull/175"
	repo := "bugfreev587/unipost"
	base := "main"
	verifiedAt := time.Date(2026, 7, 9, 21, 50, 0, 0, time.UTC)
	app := db.SiteChangeApplication{
		ID:             [16]byte{1},
		GithubPrNumber: &prNumber,
		GithubPrUrl:    &prURL,
		RepoFullName:   &repo,
		BaseBranch:     &base,
		TargetUrl:      "https://unipost.dev/",
	}

	var result map[string]any
	if err := json.Unmarshal(siteFixVerifiedPublisherResult(app, "auto_url_check", verifiedAt), &result); err != nil {
		t.Fatalf("decode verified publisher result: %v", err)
	}
	for key, want := range map[string]any{
		"status":              "verified",
		"github_pr_state":     "merged",
		"github_pr_url":       prURL,
		"verification_source": "auto_url_check",
		"verified_at":         "2026-07-09T21:50:00Z",
	} {
		if got := result[key]; got != want {
			t.Fatalf("%s = %#v, want %#v", key, got, want)
		}
	}
}
