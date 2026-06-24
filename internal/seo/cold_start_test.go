package seo

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
)

func TestColdStartOpportunityCandidatesUseConfirmedContextEvidence(t *testing.T) {
	title := "Product page"
	profile := json.RawMessage(`{
		"positioning":"AI citation workflow for technical teams",
		"icp":"B2B SaaS marketers",
		"value_props":["Source-backed briefs","Reviewable evidence"],
		"differentiators":["Human confirmation gate"],
		"competitors":["Generic SEO tools"],
		"key_terms":["AI citations","content operations"],
		"context_confirmed_at":"2026-06-12T00:00:00Z"
	}`)
	inventory := []db.ContentInventory{
		{
			Url:              "https://example.com/product",
			Title:            &title,
			Topics:           json.RawMessage(`["product","workflow"]`),
			EvidenceSnippets: json.RawMessage(`["fact one","fact two","fact three"]`),
		},
	}

	candidates := coldStartOpportunityCandidates(profile, inventory, "https://example.com")

	if len(candidates) != 3 {
		t.Fatalf("candidate count = %d, want 3", len(candidates))
	}
	if candidates[0].Query != "cold-start:context-backed-use-case-pages" {
		t.Fatalf("first query = %q", candidates[0].Query)
	}
	if candidates[1].Type != "cold_start_competitive_gap" {
		t.Fatalf("second type = %q, want competitive gap", candidates[1].Type)
	}
	if candidates[2].PageURL != "https://example.com/product" {
		t.Fatalf("evidence page URL = %q", candidates[2].PageURL)
	}
	if got := candidates[0].Evidence["evidence_count"]; got != 3 {
		t.Fatalf("evidence count = %v, want 3", got)
	}
	if strings.Contains(candidates[0].ExpectedImpact, "before Search Console data is available") {
		t.Fatal("cold-start impact copy must not claim Search Console data is unavailable")
	}
	if !strings.Contains(candidates[0].ExpectedImpact, "missing or still too thin") {
		t.Fatalf("cold-start impact copy should explain low-data mode, got %q", candidates[0].ExpectedImpact)
	}
}

func TestProfileHasContextConfirmationRequiresTimestamp(t *testing.T) {
	if profileHasContextConfirmation(json.RawMessage(`{"positioning":"done"}`)) {
		t.Fatal("profile without confirmation timestamp should not be confirmed")
	}
	if !profileHasContextConfirmation(json.RawMessage(`{"context_confirmed_at":"2026-06-12T00:00:00Z"}`)) {
		t.Fatal("profile with context_confirmed_at should be confirmed")
	}
}
