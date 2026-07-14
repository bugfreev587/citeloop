package platformcontract

import "testing"

func TestCanonicalAssetTypeNormalizesWriterAliases(t *testing.T) {
	tests := map[string]string{
		"template_checklist":          "template_or_checklist",
		"template_or_checklist":       "template_or_checklist",
		"integration_docs_page":       "integration_page",
		"integration_page":            "integration_page",
		"source_backed_evidence_page": "source_backed_evidence_page",
		"faq_answer_block":            "faq_answer_block",
	}
	for input, want := range tests {
		got, ok := CanonicalAssetType(input)
		if !ok || got != want {
			t.Errorf("CanonicalAssetType(%q) = %q, %v; want %q, true", input, got, ok, want)
		}
	}
}

func TestCanonicalAssetTypeRejectsDirectActions(t *testing.T) {
	for _, input := range []string{"metadata_rewrite", "internal_link_patch", "schema_patch", "sitemap_update", "technical_fix", ""} {
		if got, ok := CanonicalAssetType(input); ok {
			t.Errorf("CanonicalAssetType(%q) = %q, true; want rejected", input, got)
		}
	}
}
