package discovery

import (
	"encoding/json"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
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
