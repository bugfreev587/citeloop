package growthwork

import (
	"os"
	"strings"
	"testing"
)

func TestCreatorPersistsStructuredCrossLineRelationshipsInsideReservation(t *testing.T) {
	raw, err := os.ReadFile("creator.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, want := range []string{
		"LockSEOOpportunityForGrowthReserve",
		"ListWorkSignaturesForRelationship",
		"ClassifyCrossLineDependency",
		"UpsertWorkRelationship",
		"work.WorkSignatureID",
		"work.OverlapWorkIDs",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("Growth creator missing %q", want)
		}
	}
	if strings.Index(source, "LockSEOOpportunityForGrowthReserve") > strings.Index(source, "UpsertWorkRelationship") {
		t.Fatal("Growth opportunity must be locked before relationships are persisted")
	}
}
