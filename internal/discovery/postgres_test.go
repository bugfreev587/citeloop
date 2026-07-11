package discovery

import (
	"encoding/json"
	"testing"
)

func TestNumericFromConfidenceRejectsLegacyPercentageAtRepositoryBoundary(t *testing.T) {
	if _, err := numericFromConfidence(82); err == nil {
		t.Fatal("repository boundary accepted unnormalized 0-100 confidence")
	}
	if _, err := numericFromConfidence(0.82); err != nil {
		t.Fatalf("repository boundary rejected normalized confidence: %v", err)
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
