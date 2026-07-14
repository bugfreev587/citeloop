package platformcontract

import "strings"

var canonicalAssetTypes = []string{
	"blog_post",
	"comparison_page",
	"alternative_page",
	"use_case_page",
	"integration_page",
	"template_or_checklist",
	"glossary_definition",
	"benchmark_report",
	"source_backed_evidence_page",
	"faq_answer_block",
}

var assetAliases = map[string]string{
	"template_checklist":    "template_or_checklist",
	"integration_docs_page": "integration_page",
}

func CanonicalAssetTypes() []string {
	result := make([]string, len(canonicalAssetTypes))
	copy(result, canonicalAssetTypes)
	return result
}

func CanonicalAssetType(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if alias, ok := assetAliases[value]; ok {
		value = alias
	}
	for _, candidate := range canonicalAssetTypes {
		if value == candidate {
			return candidate, true
		}
	}
	return "", false
}
