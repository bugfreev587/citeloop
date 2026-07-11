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

func TestCitationFactExpansionRoutesToImprovePage(t *testing.T) {
	opportunity := db.SeoOpportunity{Type: "citation_fact_expansion"}
	if got := workTypeForOpportunity(opportunity); got != WorkTypeImprovePage {
		t.Fatalf("work type = %q, want %q", got, WorkTypeImprovePage)
	}
}

func TestCanonicalAuthorityRejectsEveryEmittedDoctorTechnicalType(t *testing.T) {
	canonical := db.ProductWriterAuthority{Product: "doctor", WriterAuthority: "canonical"}
	for _, opportunityType := range []string{
		"geo_crawler_access_blocked", "meta_description_missing", "h1_missing", "important_page_missing_from_sitemap",
		"unsafe_mdx_detected", "metadata_readability", "duplicate_metadata_template", "supported_fact_extractability",
		"source_association", "entity_naming_consistency",
		"title_duplicate", "duplicate_title", "title_too_long", "title_invalid",
	} {
		if legacyTechnicalAutopilotAllowed(canonical, db.SeoOpportunity{Type: opportunityType}, AutopilotPlanAction{}) {
			t.Fatalf("canonical authority allowed legacy technical type %q", opportunityType)
		}
	}
}
