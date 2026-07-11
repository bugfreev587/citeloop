package discovery

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestProjectEquivalentDoctorAndOpportunitySchemaWork(t *testing.T) {
	projectID := uuid.New()
	target := "https://example.com/pricing"
	doctor := ProjectDoctorFinding(db.SeoDoctorFinding{
		ID:             uuid.New(),
		ProjectID:      projectID,
		IssueType:      "structured_data_missing",
		Category:       "structured_data",
		NormalizedUrls: json.RawMessage(`["https://example.com/pricing"]`),
		Evidence:       json.RawMessage(`{"source":"crawl"}`),
	})
	opportunity := ProjectSEOOpportunity(db.SeoOpportunity{
		ID:                uuid.New(),
		ProjectID:         projectID,
		Type:              "schema_gap",
		NormalizedPageUrl: target,
		Evidence:          json.RawMessage(`{"source":"technical_checks"}`),
	})

	for name, candidate := range map[string]Candidate{"doctor": doctor, "opportunity": opportunity} {
		if candidate.SuggestedOwner != OwnerDoctor || candidate.VerificationMode != VerificationImmediate {
			t.Fatalf("%s routing = %s/%s, want doctor/immediate", name, candidate.SuggestedOwner, candidate.VerificationMode)
		}
		if candidate.Status != StatusIdentityReady {
			t.Fatalf("%s status = %s, want identity_ready", name, candidate.Status)
		}
	}
	doctorIdentity, err := BuildIdentity(doctor)
	if err != nil {
		t.Fatal(err)
	}
	opportunityIdentity, err := BuildIdentity(opportunity)
	if err != nil {
		t.Fatal(err)
	}
	if doctorIdentity.ExactSignatureHash != opportunityIdentity.ExactSignatureHash {
		t.Fatalf("equivalent schema work did not collide: %s != %s", doctorIdentity.ExactSignatureHash, opportunityIdentity.ExactSignatureHash)
	}
}

func TestProjectOpportunityNormalizesLegacyConfidencePercentage(t *testing.T) {
	candidate := ProjectSEOOpportunity(db.SeoOpportunity{
		ID:                uuid.New(),
		ProjectID:         uuid.New(),
		Type:              "gsc_low_ctr_query",
		NormalizedPageUrl: "https://example.com/pricing",
		Confidence:        testNumeric(t, "82"),
	})
	if math.Abs(candidate.Confidence-0.82) > 0.00001 {
		t.Fatalf("confidence = %f, want 0.82", candidate.Confidence)
	}
}

func TestProjectTechnicalQueryDoesNotChangeExactIdentity(t *testing.T) {
	projectID := uuid.New()
	doctor := ProjectDoctorFinding(db.SeoDoctorFinding{
		ID:             uuid.New(),
		ProjectID:      projectID,
		IssueType:      "structured_data_missing",
		NormalizedUrls: json.RawMessage(`["https://example.com/pricing"]`),
	})
	query := "pricing software"
	opportunity := ProjectSEOOpportunity(db.SeoOpportunity{
		ID:                uuid.New(),
		ProjectID:         projectID,
		Type:              "schema_gap",
		Query:             &query,
		NormalizedPageUrl: "https://example.com/pricing",
	})
	left, err := BuildIdentity(doctor)
	if err != nil {
		t.Fatal(err)
	}
	right, err := BuildIdentity(opportunity)
	if err != nil {
		t.Fatal(err)
	}
	if left.ExactSignatureHash != right.ExactSignatureHash {
		t.Fatal("detector query changed the exact identity of equivalent technical work")
	}
}

func TestProjectContentQueryMateriallyScopesIdentity(t *testing.T) {
	projectID := uuid.New()
	firstQuery := "pricing software"
	secondQuery := "enterprise pricing software"
	base := db.SeoOpportunity{
		ID:                uuid.New(),
		ProjectID:         projectID,
		Type:              "gsc_query_gap",
		NormalizedPageUrl: "https://example.com/pricing",
		Query:             &firstQuery,
	}
	first := ProjectSEOOpportunity(base)
	base.ID = uuid.New()
	base.Query = &secondQuery
	second := ProjectSEOOpportunity(base)
	left, _ := BuildIdentity(first)
	right, _ := BuildIdentity(second)
	if left.ExactSignatureHash == right.ExactSignatureHash {
		t.Fatal("distinct growth queries must remain distinct content work")
	}
}

func TestProjectLowCTRQueryDoesNotDuplicateSameTitleMutation(t *testing.T) {
	projectID := uuid.New()
	firstQuery := "pricing software"
	secondQuery := "enterprise pricing software"
	base := db.SeoOpportunity{
		ID:                uuid.New(),
		ProjectID:         projectID,
		Type:              "gsc_low_ctr_query",
		NormalizedPageUrl: "https://example.com/pricing",
		Query:             &firstQuery,
	}
	first := ProjectSEOOpportunity(base)
	base.ID = uuid.New()
	base.Query = &secondQuery
	second := ProjectSEOOpportunity(base)
	left, err := BuildIdentity(first)
	if err != nil {
		t.Fatal(err)
	}
	right, err := BuildIdentity(second)
	if err != nil {
		t.Fatal(err)
	}
	if left.ExactSignatureHash != right.ExactSignatureHash {
		t.Fatal("GSC query split the same page title mutation into duplicate exact identities")
	}
}

func TestProjectTechnicalVisibilityUsesStructuredSubtype(t *testing.T) {
	projectID := uuid.New()
	target := "https://example.com/pricing"
	tests := []struct {
		name       string
		issue      string
		doctorType string
	}{
		{name: "http", issue: "http_status", doctorType: "broken_url"},
		{name: "robots", issue: "robots_noindex", doctorType: "robots_blocked"},
		{name: "canonical", issue: "canonical_missing", doctorType: "canonical_missing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doctor := ProjectDoctorFinding(db.SeoDoctorFinding{
				ID:             uuid.New(),
				ProjectID:      projectID,
				IssueType:      tt.doctorType,
				NormalizedUrls: json.RawMessage(`["https://example.com/pricing"]`),
			})
			opportunity := ProjectSEOOpportunity(db.SeoOpportunity{
				ID:                uuid.New(),
				ProjectID:         projectID,
				Type:              "technical_visibility_issue",
				NormalizedPageUrl: target,
				Evidence:          json.RawMessage(`{"issue":"` + tt.issue + `"}`),
			})
			left, err := BuildIdentity(doctor)
			if err != nil {
				t.Fatal(err)
			}
			right, err := BuildIdentity(opportunity)
			if err != nil {
				t.Fatal(err)
			}
			if left.ExactSignatureHash != right.ExactSignatureHash {
				t.Fatalf("equivalent %s work did not collide", tt.name)
			}
		})
	}
}

func TestProjectAmbiguousTechnicalVisibilityNeedsSpecification(t *testing.T) {
	candidate := ProjectSEOOpportunity(db.SeoOpportunity{
		ID:                uuid.New(),
		ProjectID:         uuid.New(),
		Type:              "technical_visibility_issue",
		NormalizedPageUrl: "https://example.com/pricing",
		Evidence:          json.RawMessage(`{"issue":"future_unknown_blocker"}`),
	})
	if candidate.Status != StatusNeedsSpecification {
		t.Fatalf("status = %s, want needs_specification", candidate.Status)
	}
}

func TestProjectCurrentDeterministicOpportunityTypes(t *testing.T) {
	query := "pricing software"
	tests := []struct {
		typeName string
		evidence json.RawMessage
	}{
		{typeName: "internal_link_gap"},
		{typeName: "schema_gap"},
		{typeName: "thin_evidence_page"},
		{typeName: "gsc_low_ctr_query"},
		{typeName: "gsc_query_gap"},
		{typeName: "gsc_striking_distance_query"},
		{typeName: "gsc_content_decay"},
		{typeName: "gsc_query_cannibalization", evidence: json.RawMessage(`{"competing_pages":[{"normalized_page_url":"https://example.com/compare"}]}`)},
	}
	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			candidate := ProjectSEOOpportunity(db.SeoOpportunity{
				ID:                uuid.New(),
				ProjectID:         uuid.New(),
				Type:              tt.typeName,
				NormalizedPageUrl: "https://example.com/pricing",
				Query:             &query,
				Evidence:          tt.evidence,
			})
			if candidate.Status != StatusIdentityReady {
				t.Fatalf("status = %s (%s), want identity_ready", candidate.Status, candidate.HoldReason)
			}
		})
	}
}

func TestEveryEmittedDoctorFindingHasImmediateRepairSpecification(t *testing.T) {
	broken := []string{
		"broken_url", "soft_404", "noindex", "robots_blocked", "canonical_missing", "structured_data_missing",
		"title_missing", "meta_description_missing", "h1_missing", "important_page_missing_from_sitemap",
		"internal_link_gap", "unsafe_mdx_detected", "geo_crawler_access_blocked",
	}
	optimizations := []string{
		"metadata_readability", "duplicate_metadata_template", "supported_fact_extractability",
		"source_association", "entity_naming_consistency",
	}
	for _, issueType := range append(broken, optimizations...) {
		t.Run(issueType, func(t *testing.T) {
			candidate := ProjectDoctorFinding(db.SeoDoctorFinding{
				ID: uuid.New(), ProjectID: uuid.New(), RunID: uuid.New(), IssueType: issueType,
				FindingKind: "broken", NormalizedUrls: json.RawMessage(`["https://example.com/page"]`),
				Evidence: json.RawMessage(`{"source":"doctor"}`),
			})
			if candidate.Status != StatusIdentityReady || candidate.SuggestedOwner != OwnerDoctor || candidate.VerificationMode != VerificationImmediate || candidate.ArtifactIntent != ArtifactRepairExistingSurface {
				t.Fatalf("candidate = %+v", candidate)
			}
			if len(candidate.ProposedMutations) != 1 || candidate.ChangeFamily == "" || candidate.ProposedMutations[0].Field == "" {
				t.Fatalf("mutation spec = %+v", candidate)
			}
		})
	}
}

func TestCitationFactExpansionProjectsAsDelayedExistingPageGrowth(t *testing.T) {
	candidate := ProjectSEOOpportunity(db.SeoOpportunity{
		ID: uuid.New(), ProjectID: uuid.New(), Type: "citation_fact_expansion",
		NormalizedPageUrl: "https://example.com/page", Evidence: json.RawMessage(`{"added_propositions":["new fact"]}`),
	})
	if candidate.Status != StatusIdentityReady || candidate.SuggestedOwner != OwnerOpportunities || candidate.VerificationMode != VerificationDelayed || candidate.ArtifactIntent != ArtifactUpdateExistingContent {
		t.Fatalf("candidate = %+v", candidate)
	}
	if candidate.PrimarySuccessMetric == "" || len(candidate.ProposedMutations) != 1 {
		t.Fatalf("growth spec = %+v", candidate)
	}
}

func TestProjectAllCurrentlyEmittedOpportunityTypesHaveExplicitOutcome(t *testing.T) {
	query := "pricing software"
	tests := []struct {
		typeName string
		evidence json.RawMessage
		want     CandidateStatus
	}{
		{typeName: "internal_link_gap", want: StatusIdentityReady},
		{typeName: "schema_gap", want: StatusIdentityReady},
		{typeName: "technical_visibility_issue", evidence: json.RawMessage(`{"issue":"http_status"}`), want: StatusIdentityReady},
		{typeName: "thin_evidence_page", want: StatusIdentityReady},
		{typeName: "gsc_query_cannibalization", evidence: json.RawMessage(`{"competing_pages":[{"normalized_page_url":"https://example.com/compare"}]}`), want: StatusIdentityReady},
		{typeName: "gsc_low_ctr_query", want: StatusIdentityReady},
		{typeName: "gsc_query_gap", want: StatusIdentityReady},
		{typeName: "gsc_striking_distance_query", want: StatusIdentityReady},
		{typeName: "gsc_content_decay", want: StatusIdentityReady},
		// These current producers do not provide an exact executable mutation or
		// canonical slug. Phase 1A records them without inventing a work identity.
		{typeName: "indexing_anomaly", want: StatusNeedsSpecification},
		{typeName: "cold_start_context_plan", want: StatusNeedsSpecification},
		{typeName: "cold_start_competitive_gap", want: StatusNeedsSpecification},
		{typeName: "cold_start_evidence_page", want: StatusNeedsSpecification},
	}
	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			candidate := ProjectSEOOpportunity(db.SeoOpportunity{
				ID:                uuid.New(),
				ProjectID:         uuid.New(),
				Type:              tt.typeName,
				NormalizedPageUrl: "https://example.com/pricing",
				Query:             &query,
				Evidence:          tt.evidence,
			})
			if candidate.Status != tt.want {
				t.Fatalf("status = %s (%s), want %s", candidate.Status, candidate.HoldReason, tt.want)
			}
		})
	}
}

func TestProjectThinEvidenceIsGrowthAddition(t *testing.T) {
	candidate := ProjectSEOOpportunity(db.SeoOpportunity{
		ID:                uuid.New(),
		ProjectID:         uuid.New(),
		Type:              "thin_evidence_page",
		NormalizedPageUrl: "https://example.com/pricing",
	})
	if candidate.SuggestedOwner != OwnerOpportunities || candidate.VerificationMode != VerificationDelayed {
		t.Fatalf("routing = %s/%s, want opportunities/delayed", candidate.SuggestedOwner, candidate.VerificationMode)
	}
	if candidate.ProposedMutations[0].Operation != "add" || candidate.ProposedMutations[0].Field != "evidence_block" {
		t.Fatalf("mutation = %+v, want add:evidence_block", candidate.ProposedMutations[0])
	}
}

func TestProjectMissingTitleAndLowCTRTitleDiffer(t *testing.T) {
	projectID := uuid.New()
	target := "https://example.com/pricing"
	doctor := ProjectDoctorFinding(db.SeoDoctorFinding{
		ID:             uuid.New(),
		ProjectID:      projectID,
		IssueType:      "title_missing",
		NormalizedUrls: json.RawMessage(`["https://example.com/pricing"]`),
	})
	opportunity := ProjectSEOOpportunity(db.SeoOpportunity{
		ID:                uuid.New(),
		ProjectID:         projectID,
		Type:              "low_ctr",
		NormalizedPageUrl: target,
		RecommendedAction: stringPointer("Rewrite the title for higher CTR"),
	})
	if doctor.ProposedMutations[0].Operation != "add" || opportunity.ProposedMutations[0].Operation != "update" {
		t.Fatalf("title operations = %s/%s, want add/update", doctor.ProposedMutations[0].Operation, opportunity.ProposedMutations[0].Operation)
	}
	if opportunity.SuggestedOwner != OwnerOpportunities || opportunity.VerificationMode != VerificationDelayed {
		t.Fatalf("low CTR routing = %s/%s", opportunity.SuggestedOwner, opportunity.VerificationMode)
	}
	doctorIdentity, _ := BuildIdentity(doctor)
	opportunityIdentity, _ := BuildIdentity(opportunity)
	if doctorIdentity.ExactSignatureHash == opportunityIdentity.ExactSignatureHash {
		t.Fatal("missing title repair and CTR title experiment must not collide exactly")
	}
}

func TestProjectAICitationEvidenceGapAsGrowthWork(t *testing.T) {
	candidate := ProjectSEOOpportunity(db.SeoOpportunity{
		ID:                uuid.New(),
		ProjectID:         uuid.New(),
		Type:              "geo_project_mentioned_without_citation",
		NormalizedPageUrl: "https://example.com/product",
		Evidence:          json.RawMessage(`{"engine":"perplexity"}`),
	})
	if candidate.SuggestedOwner != OwnerOpportunities || candidate.VerificationMode != VerificationDelayed {
		t.Fatalf("citation gap routing = %s/%s", candidate.SuggestedOwner, candidate.VerificationMode)
	}
	if candidate.ChangeFamily != "content.evidence" || candidate.ArtifactIntent != ArtifactUpdateExistingContent {
		t.Fatalf("citation gap change = %s/%s", candidate.ChangeFamily, candidate.ArtifactIntent)
	}
}

func TestProjectUnknownWorkWithoutTargetNeedsSpecification(t *testing.T) {
	candidate := ProjectSEOOpportunity(db.SeoOpportunity{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
		Type:      "unknown_future_detector",
	})
	if candidate.Status != StatusNeedsSpecification {
		t.Fatalf("status = %s, want needs_specification", candidate.Status)
	}
	if candidate.HoldReason == "" {
		t.Fatal("needs_specification candidate must explain the hold")
	}
}

func stringPointer(value string) *string { return &value }

func testNumeric(t *testing.T, value string) pgtype.Numeric {
	t.Helper()
	var numeric pgtype.Numeric
	if err := numeric.Scan(value); err != nil {
		t.Fatal(err)
	}
	return numeric
}
