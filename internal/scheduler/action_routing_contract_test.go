package scheduler

import "testing"

func TestContentActionRoutingSeparatesTopicBackedAndDirectAssets(t *testing.T) {
	topicBacked := []string{
		"blog_post",
		"comparison_page",
		"alternative_page",
		"use_case_page",
		"integration_page",
		"template_or_checklist",
		"glossary_definition",
		"benchmark_report",
	}
	for _, assetType := range topicBacked {
		if !contentActionNeedsTopic(assetType, "create content task") {
			t.Fatalf("%s should create or link a topic", assetType)
		}
	}

	direct := []struct {
		assetType  string
		actionType string
	}{
		{"metadata_rewrite", "rewrite title and meta description"},
		{"internal_link_patch", "add internal links"},
		{"schema_patch", "add software application schema"},
		{"sitemap_update", "submit sitemap"},
		{"technical_fix", "fix robots indexing issue"},
		{"", "technical SEO fix task"},
	}
	for _, tc := range direct {
		if contentActionNeedsTopic(tc.assetType, tc.actionType) {
			t.Fatalf("%s/%s should not create a topic by default", tc.assetType, tc.actionType)
		}
	}
}
