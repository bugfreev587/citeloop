package scheduler

import (
	"os"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
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
		"metadata_rewrite",
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

	fixSiteIssue := "fix_site_issue"
	if contentActionCreatesContent(db.ContentAction{AssetType: ptr("metadata_rewrite"), ActionType: "rewrite title", WorkType: &fixSiteIssue}) {
		t.Fatal("metadata routed as a site fix should not create a topic")
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
		"Channel:               publishStrategyForContentAction(action, opp)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("scheduler direct action skip contract missing %q", want)
		}
	}
	if strings.Contains(body, `fmt.Errorf("content action %s is routed as a direct action`) {
		t.Fatal("direct action planning must return a typed sentinel error so the caller can skip without marking failed")
	}
	if strings.Contains(body, `Channel:               "blog"`) {
		t.Fatal("opportunity-generated scheduled topics must not hardcode blog")
	}
}
