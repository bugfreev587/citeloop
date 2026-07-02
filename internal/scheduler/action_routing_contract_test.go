package scheduler

import (
	"os"
	"strings"
	"testing"
)

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
		{"", "rewrite homepage title and meta description"},
	}
	for _, tc := range direct {
		if contentActionNeedsTopic(tc.assetType, tc.actionType) {
			t.Fatalf("%s/%s should not create a topic by default", tc.assetType, tc.actionType)
		}
	}
}

func TestDirectActionPlanningIsSkippedWithoutMarkingFailed(t *testing.T) {
	raw, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{
		"errDirectContentAction",
		"errors.Is(err, errDirectContentAction)",
		"direct content action skipped from topic planning",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("scheduler direct action skip contract missing %q", want)
		}
	}
	if strings.Contains(body, `fmt.Errorf("content action %s is routed as a direct action`) {
		t.Fatal("direct action planning must return a typed sentinel error so the caller can skip without marking failed")
	}
}
