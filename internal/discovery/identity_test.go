package discovery

import (
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestBuildIdentityIsOwnerNeutralAndOrderStable(t *testing.T) {
	projectID := uuid.New()
	base := Candidate{
		ProjectID:           projectID,
		SourceKind:          SourceDoctor,
		NormalizedTargetSet: []string{"https://example.com/pricing/", "https://example.com/pricing"},
		ChangeFamily:        "metadata.title",
		ProposedMutations: []Mutation{
			{Operation: "update", Field: "title", Target: "https://example.com/pricing"},
			{Operation: "move", Field: "title", Selector: "head"},
		},
		ArtifactIntent:       ArtifactRepairExistingSurface,
		TopicEntityIdentity:  []string{"pricing-software", "CiteLoop"},
		AudienceIdentity:     []string{"smb", "en-US"},
		SuggestedOwner:       OwnerDoctor,
		PrimarySuccessMetric: "rendered title exists",
		Confidence:           0.91,
		SignatureVersion:     SignatureVersionV1,
	}

	first, err := BuildIdentity(base)
	if err != nil {
		t.Fatalf("BuildIdentity: %v", err)
	}
	variant := base
	variant.SourceKind = SourceAIDiscovery
	variant.SuggestedOwner = OwnerOpportunities
	variant.PrimarySuccessMetric = "CTR increases"
	variant.Confidence = 0.55
	variant.NormalizedTargetSet = []string{"https://example.com/pricing", "https://example.com/pricing/"}
	variant.ProposedMutations = []Mutation{base.ProposedMutations[1], base.ProposedMutations[0]}
	variant.TopicEntityIdentity = []string{"citeloop", "PRICING-SOFTWARE"}
	variant.AudienceIdentity = []string{"EN-us", "SMB"}

	second, err := BuildIdentity(variant)
	if err != nil {
		t.Fatalf("BuildIdentity variant: %v", err)
	}
	if first.ExactSignatureHash != second.ExactSignatureHash {
		t.Fatalf("owner/source/order changed owner-neutral hash: %s != %s", first.ExactSignatureHash, second.ExactSignatureHash)
	}
	if !reflect.DeepEqual(first.ConflictBucketKeys, second.ConflictBucketKeys) {
		t.Fatalf("bucket keys changed: %#v != %#v", first.ConflictBucketKeys, second.ConflictBucketKeys)
	}
}

func TestBuildIdentityDistinguishesMutationOperation(t *testing.T) {
	base := Candidate{
		ProjectID:           uuid.New(),
		NormalizedTargetSet: []string{"https://example.com/pricing"},
		ChangeFamily:        "metadata.title",
		ProposedMutations:   []Mutation{{Operation: "add", Field: "title"}},
		ArtifactIntent:      ArtifactRepairExistingSurface,
		SignatureVersion:    SignatureVersionV1,
	}
	addIdentity, err := BuildIdentity(base)
	if err != nil {
		t.Fatal(err)
	}
	base.ProposedMutations = []Mutation{{Operation: "update", Field: "title"}}
	updateIdentity, err := BuildIdentity(base)
	if err != nil {
		t.Fatal(err)
	}
	if addIdentity.ExactSignatureHash == updateIdentity.ExactSignatureHash {
		t.Fatal("add:title and update:title must not share a signature")
	}
}

func TestBuildIdentityReturnsStableConflictBuckets(t *testing.T) {
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000123")
	identity, err := BuildIdentity(Candidate{
		ProjectID:               projectID,
		NormalizedTargetSet:     []string{"https://example.com/pricing"},
		ChangeFamily:            "metadata.title",
		ProposedMutations:       []Mutation{{Operation: "update", Field: "title"}},
		ArtifactIntent:          ArtifactRepairExistingSurface,
		TopicEntityIdentity:     []string{"pricing"},
		IntendedSlugOrCanonical: "https://example.com/pricing",
		SignatureVersion:        SignatureVersionV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	wants := []string{
		"project:00000000-0000-0000-0000-000000000123|change:metadata",
		"project:00000000-0000-0000-0000-000000000123|slug:https://example.com/pricing",
		"project:00000000-0000-0000-0000-000000000123|target:https://example.com/pricing",
		"project:00000000-0000-0000-0000-000000000123|topic:pricing",
	}
	if !reflect.DeepEqual(identity.ConflictBucketKeys, wants) {
		t.Fatalf("ConflictBucketKeys = %#v, want %#v", identity.ConflictBucketKeys, wants)
	}
}

func TestValidateCandidateNeedsSpecification(t *testing.T) {
	tests := []Candidate{
		{
			ProjectID:         uuid.New(),
			ChangeFamily:      "schema",
			ProposedMutations: []Mutation{{Operation: "add", Field: "jsonld"}},
			ArtifactIntent:    ArtifactRepairExistingSurface,
			SignatureVersion:  SignatureVersionV1,
		},
		{
			ProjectID:           uuid.New(),
			NormalizedTargetSet: []string{"https://example.com"},
			ChangeFamily:        "content.comparison",
			ProposedMutations:   []Mutation{{Operation: "create", Field: "page"}},
			ArtifactIntent:      ArtifactCreateNewAsset,
			SignatureVersion:    SignatureVersionV1,
		},
	}
	for _, candidate := range tests {
		_, err := BuildIdentity(candidate)
		if !errors.Is(err, ErrNeedsSpecification) {
			t.Fatalf("BuildIdentity error = %v, want ErrNeedsSpecification", err)
		}
	}
}
