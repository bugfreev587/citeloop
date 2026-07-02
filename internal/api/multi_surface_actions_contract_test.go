package api

import (
	"os"
	"strings"
	"testing"
)

func TestCreateSEOContentActionAcceptsMultiSurfaceMetadata(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"AssetType",
		"`json:\"asset_type\"`",
		"TargetSurfaceID",
		"`json:\"target_surface_id\"`",
		"RiskReasons",
		"`json:\"risk_reasons\"`",
		"EvidenceSnapshot",
		"`json:\"evidence_snapshot\"`",
		"DiffSnapshot",
		"`json:\"diff_snapshot\"`",
		"ReviewRequired",
		"`json:\"review_required\"`",
		"UpdateContentActionExecutionMetadata",
		"defaultReviewRequiredForAssetType",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("createSEOContentAction missing %q", want)
		}
	}
}
