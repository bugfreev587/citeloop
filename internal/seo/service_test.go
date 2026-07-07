package seo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
)

func TestProviderCanAttemptAfterRecoverableStatus(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{status: "connected", want: true},
		{status: "backfilling", want: true},
		{status: "stale", want: true},
		{status: "error", want: true},
		{status: "expired", want: true},
		{status: "missing", want: false},
		{status: "property_selection_required", want: false},
		{status: "mismatch", want: false},
		{status: "revoked", want: false},
	}
	for _, tc := range cases {
		got := isProviderAttemptable([]db.SeoIntegration{{
			Provider: ProviderGSC,
			Status:   tc.status,
		}}, ProviderGSC)
		if got != tc.want {
			t.Fatalf("status %q attemptable = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestFinishRunContextSurvivesCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	finishCtx, finishCancel := finishRunContext(ctx)
	defer finishCancel()

	if err := finishCtx.Err(); err != nil {
		t.Fatalf("finish context err = %v, want nil", err)
	}
}

func TestCheckURLExtractsRepairMetadataFacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html>
<html lang="en">
  <head>
    <title>UniPost | Unified Social Media Posting API for Developers</title>
    <link rel="canonical" href="/" />
    <link rel="icon" href="/brand/unipost-icon-dark.png" sizes="512x512" type="image/png" />
    <meta name="description" content="UniPost is a unified social media posting API for developers." />
    <meta property="og:site_name" content="UniPost" />
    <meta property="og:description" content="Build social publishing into your product with one API." />
  </head>
  <body><main><h1>Build the social layer once</h1></main></body>
</html>`))
	}))
	defer server.Close()

	result := Service{}.checkURL(context.Background(), server.URL, server.URL)

	if got := result.RawDetails["page_title"]; got != "UniPost | Unified Social Media Posting API for Developers" {
		t.Fatalf("page_title = %#v", got)
	}
	if got := result.RawDetails["meta_description"]; got != "UniPost is a unified social media posting API for developers." {
		t.Fatalf("meta_description = %#v", got)
	}
	if got := result.RawDetails["og_site_name"]; got != "UniPost" {
		t.Fatalf("og_site_name = %#v", got)
	}
	if got := result.RawDetails["html_lang"]; got != "en" {
		t.Fatalf("html_lang = %#v", got)
	}
	if got := result.RawDetails["canonical_url"]; got != server.URL+"/" {
		t.Fatalf("canonical_url = %#v", got)
	}
	logos, ok := result.RawDetails["logo_candidates"].([]string)
	if !ok || len(logos) != 1 || logos[0] != server.URL+"/brand/unipost-icon-dark.png" {
		t.Fatalf("logo_candidates = %#v", result.RawDetails["logo_candidates"])
	}
	if got := result.RawDetails["site_search_observed"]; got != false {
		t.Fatalf("site_search_observed = %#v, want false", got)
	}
}

func TestGA4IntegrationFailureRequiresReconnectForInsufficientScope(t *testing.T) {
	err := errors.New(`google api status 403: { "error": { "code": 403, "message": "Request had insufficient authentication scopes.", "status": "PERMISSION_DENIED", "details": [ { "reason": "ACCESS_TOKEN_SCOPE_INSUFFICIENT" } ] } }`)

	status, message, note := ga4IntegrationFailureForError(err)

	if status != "reconnect_required" {
		t.Fatalf("status = %q, want reconnect_required", status)
	}
	want := "Google Analytics permission is missing. Reconnect Google from Search Console settings so CiteLoop can request Analytics read access, then run SEO sync again."
	if message != want {
		t.Fatalf("message = %q, want %q", message, want)
	}
	if note != "ga4_reconnect_required" {
		t.Fatalf("note = %q, want ga4_reconnect_required", note)
	}
}

func TestGA4IntegrationFailurePreservesOtherErrors(t *testing.T) {
	status, message, note := ga4IntegrationFailureForError(errors.New("google api status 404: property not found"))

	if status != "error" {
		t.Fatalf("status = %q, want error", status)
	}
	if message != "google api status 404: property not found" {
		t.Fatalf("message = %q", message)
	}
	if note != "ga4_error" {
		t.Fatalf("note = %q, want ga4_error", note)
	}
}
