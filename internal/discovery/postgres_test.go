package discovery

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestNumericFromConfidenceRejectsLegacyPercentageAtRepositoryBoundary(t *testing.T) {
	if _, err := numericFromConfidence(82); err == nil {
		t.Fatal("repository boundary accepted unnormalized 0-100 confidence")
	}
	if _, err := numericFromConfidence(0.82); err != nil {
		t.Fatalf("repository boundary rejected normalized confidence: %v", err)
	}
}

func TestCreateDecisionParamsStoresNilUUIDSlicesAsJSONArrays(t *testing.T) {
	params, err := createDecisionParams(PreparedDecision{
		ProjectID: uuid.New(), CandidateID: uuid.New(), CandidateVersion: 1,
		Disposition: DispositionDeterministicSafe, Decision: DecisionCreate,
		Owner: OwnerDoctor, Confidence: 1, ExpectedBucketVersions: map[string]int64{},
		Status: ArbitrationStatusPrepared,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(params.OverlapWorkIds) != "[]" {
		t.Fatalf("nil overlap ids encoded as %s, want []", params.OverlapWorkIds)
	}
	if string(params.ComparedWorkIds) != "[]" {
		t.Fatalf("nil compared ids encoded as %s, want []", params.ComparedWorkIds)
	}
}

func TestMarshalMutationsForStorageUsesEmptyJSONArrayForNil(t *testing.T) {
	raw, err := marshalMutationsForStorage(nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]" {
		t.Fatalf("nil mutations encoded as %s, want []", raw)
	}
	var decoded []Mutation
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("stored mutations are not an array: %v", err)
	}
}
