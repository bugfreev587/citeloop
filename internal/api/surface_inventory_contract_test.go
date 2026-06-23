package api

import (
	"os"
	"strings"
	"testing"
)

func TestCreateGEOExternalSurfaceAcceptsGeneralizedSurfaceMetadata(t *testing.T) {
	raw, err := os.ReadFile("handlers_geo_pr2.go")
	if err != nil {
		t.Fatalf("read handlers_geo_pr2.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"SourceURL",
		"`json:\"source_url\"`",
		"CanonicalStatus",
		"`json:\"canonical_status\"`",
		"IndexabilityStatus",
		"`json:\"indexability_status\"`",
		"PublicationStatus",
		"`json:\"publication_status\"`",
		"OwnerConfidence",
		"`json:\"owner_confidence\"`",
		"VerificationSnapshot",
		"`json:\"verification_snapshot\"`",
		"RelatedActionIDs",
		"`json:\"related_action_ids\"`",
		"UpdateGEOExternalSurfaceMetadata",
		"rawOrExisting",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("createGEOExternalSurface missing %q", want)
		}
	}
}
