package discovery

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestClassifyCrossLineDependencyMissingTitleBlocksLowCTRTitle(t *testing.T) {
	projectID := uuid.New()
	doctor := dependencyCandidate(projectID, OwnerDoctor, VerificationImmediate, "metadata.title", "add", "title")
	growth := dependencyCandidate(projectID, OwnerOpportunities, VerificationDelayed, "metadata.title", "update", "title")
	doctorIdentity, err := BuildIdentity(doctor)
	if err != nil {
		t.Fatal(err)
	}

	relationship, ok, err := ClassifyCrossLineDependency(growth, SnapshotWork{
		ID: uuid.New(), Owner: OwnerDoctor, ExactSignatureHash: doctorIdentity.ExactSignatureHash,
		SignaturePayload: doctorIdentity.SignaturePayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || relationship.Class != DependencyHardBlocker {
		t.Fatalf("relationship = %+v, ok=%t", relationship, ok)
	}
	if len(relationship.OverlappingMutationFields) != 1 || relationship.OverlappingMutationFields[0] != "title" {
		t.Fatalf("overlapping fields = %v", relationship.OverlappingMutationFields)
	}
	if relationship.ReassessmentTrigger != "blocking_work_verified" {
		t.Fatalf("reassessment trigger = %q", relationship.ReassessmentTrigger)
	}
	if !strings.Contains(strings.ToLower(relationship.Reason), "ctr baseline") {
		t.Fatalf("title blocker reason must explain baseline invalidity: %q", relationship.Reason)
	}
}

func TestClassifyCrossLineDependencyNonOverlappingFieldsAreSoft(t *testing.T) {
	projectID := uuid.New()
	doctor := dependencyCandidate(projectID, OwnerDoctor, VerificationImmediate, "metadata.description", "add", "meta_description")
	growth := dependencyCandidate(projectID, OwnerOpportunities, VerificationDelayed, "metadata.title", "update", "title")
	doctorIdentity, err := BuildIdentity(doctor)
	if err != nil {
		t.Fatal(err)
	}

	relationship, ok, err := ClassifyCrossLineDependency(growth, SnapshotWork{
		ID: uuid.New(), Owner: OwnerDoctor, ExactSignatureHash: doctorIdentity.ExactSignatureHash,
		SignaturePayload: doctorIdentity.SignaturePayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || relationship.Class != DependencySoft {
		t.Fatalf("relationship = %+v, ok=%t", relationship, ok)
	}
	if len(relationship.OverlappingMutationFields) != 0 {
		t.Fatalf("overlapping fields = %v", relationship.OverlappingMutationFields)
	}
	if relationship.ReassessmentTrigger != "attribution_reconcile" {
		t.Fatalf("reassessment trigger = %q", relationship.ReassessmentTrigger)
	}
}

func TestClassifyCrossLineDependencyExactWorkUsesSignatureMerge(t *testing.T) {
	projectID := uuid.New()
	candidate := dependencyCandidate(projectID, OwnerOpportunities, VerificationDelayed, "content.evidence", "update", "evidence_block")
	identity, err := BuildIdentity(candidate)
	if err != nil {
		t.Fatal(err)
	}

	_, ok, err := ClassifyCrossLineDependency(candidate, SnapshotWork{
		ID: uuid.New(), Owner: OwnerDoctor, ExactSignatureHash: identity.ExactSignatureHash,
		SignaturePayload: identity.SignaturePayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("exact duplicate must merge through the owner-neutral signature, not create a dependency")
	}
}

func TestClassifyCrossLineDependencyRejectsMalformedActiveSignature(t *testing.T) {
	candidate := dependencyCandidate(uuid.New(), OwnerOpportunities, VerificationDelayed, "metadata.title", "update", "title")
	_, _, err := ClassifyCrossLineDependency(candidate, SnapshotWork{
		ID: uuid.New(), Owner: OwnerDoctor, ExactSignatureHash: "different", SignaturePayload: json.RawMessage(`{"bad":true}`),
	})
	if err == nil {
		t.Fatal("malformed active signature must fail closed")
	}
}

func dependencyCandidate(projectID uuid.UUID, owner Owner, verification VerificationMode, family, operation, field string) Candidate {
	metric := "acceptance_test_pass"
	if verification == VerificationDelayed {
		metric = "ctr"
	}
	return Candidate{
		ProjectID: projectID, NormalizedTargetSet: []string{"https://example.com/pricing"},
		ChangeFamily: family, ProposedMutations: []Mutation{{Operation: operation, Field: field}},
		ArtifactIntent: ArtifactUpdateExistingContent, SuggestedOwner: owner,
		VerificationMode: verification, PrimarySuccessMetric: metric, SignatureVersion: SignatureVersionV1,
	}
}
