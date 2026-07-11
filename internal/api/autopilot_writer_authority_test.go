package api

import (
	"testing"

	"github.com/citeloop/citeloop/internal/db"
)

func TestCanonicalDoctorAuthorityRejectsGuardedAutopilotTechnicalAction(t *testing.T) {
	articleAsset := "article"
	candidate := AutopilotPlanAction{AssetType: &articleAsset}
	technical := db.SeoOpportunity{Type: "schema_gap"}

	canonical := db.ProductWriterAuthority{Product: "doctor", WriterAuthority: "canonical"}
	if legacyTechnicalAutopilotAllowed(canonical, technical, candidate) {
		t.Fatal("canonical Doctor authority must reject legacy technical action even when asset type looks like growth content")
	}
	metadataRepair := db.SeoOpportunity{Type: "title_missing"}
	if legacyTechnicalAutopilotAllowed(canonical, metadataRepair, candidate) {
		t.Fatal("canonical Doctor authority must reject missing-metadata repair even when legacy work-type heuristics call it page improvement")
	}

	legacy := db.ProductWriterAuthority{Product: "doctor", WriterAuthority: "legacy"}
	if !legacyTechnicalAutopilotAllowed(legacy, technical, candidate) {
		t.Fatal("legacy authority should remain readable/executable during the migration window")
	}
}
