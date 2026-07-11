package discovery

import (
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
