package db

import (
	"os"
	"strings"
	"testing"
)

func TestAdminDeleteForeignKeyIndexesMigration(t *testing.T) {
	data, err := os.ReadFile("../migrations/0027_admin_delete_fk_indexes.sql")
	if err != nil {
		t.Fatalf("admin delete FK index migration missing: %v", err)
	}
	sql := strings.ToLower(string(data))

	for _, want := range []string{
		"idx_ai_crawler_access_snapshots_run_id",
		"on ai_crawler_access_snapshots (run_id)",
		"idx_autopilot_runs_objective_id",
		"on autopilot_runs (objective_id)",
		"idx_content_actions_opportunity_id",
		"on content_actions (opportunity_id)",
		"idx_content_actions_target_article_id",
		"on content_actions (target_article_id)",
		"idx_content_actions_draft_article_id",
		"on content_actions (draft_article_id)",
		"idx_geo_asset_briefs_created_by_run_id",
		"on geo_asset_briefs (created_by_run_id)",
		"idx_geo_prompt_sets_created_by_run_id",
		"on geo_prompt_sets (created_by_run_id)",
		"idx_geo_prompts_prompt_set_id",
		"on geo_prompts (prompt_set_id)",
		"idx_geo_observations_run_id",
		"on geo_observations (run_id)",
		"idx_geo_observations_prompt_id",
		"on geo_observations (prompt_id)",
		"idx_geo_visibility_scores_run_id",
		"on geo_visibility_scores (run_id)",
		"idx_guardrail_checks_action_id",
		"on guardrail_checks (action_id)",
		"idx_internal_link_edges_source_article_id",
		"on internal_link_edges (source_article_id)",
		"idx_internal_link_edges_target_article_id",
		"on internal_link_edges (target_article_id)",
		"idx_page_performance_daily_article_id",
		"on page_performance_daily (article_id)",
		"idx_page_performance_daily_property_id",
		"on page_performance_daily (property_id)",
		"idx_page_performance_daily_topic_id",
		"on page_performance_daily (topic_id)",
		"idx_publisher_credentials_connection_id",
		"on publisher_credentials (connection_id)",
		"idx_rollback_records_action_id",
		"on rollback_records (action_id)",
		"idx_search_appearance_daily_property_id",
		"on search_appearance_daily (property_id)",
		"idx_search_performance_daily_property_id",
		"on search_performance_daily (property_id)",
		"idx_seo_action_plans_autopilot_run_id",
		"on seo_action_plans (autopilot_run_id)",
		"idx_seo_action_plans_objective_id",
		"on seo_action_plans (objective_id)",
		"idx_seo_experiments_action_id",
		"on seo_experiments (action_id)",
		"idx_seo_opportunities_article_id",
		"on seo_opportunities (article_id)",
		"idx_seo_opportunities_topic_id",
		"on seo_opportunities (topic_id)",
		"idx_seo_opportunities_created_by_run_id",
		"on seo_opportunities (created_by_run_id)",
		"idx_technical_checks_article_id",
		"on technical_checks (article_id)",
		"idx_technical_checks_run_id",
		"on technical_checks (run_id)",
		"idx_topics_source_content_action_id",
		"on topics (source_content_action_id)",
		"idx_url_index_snapshots_article_id",
		"on url_index_snapshots (article_id)",
		"idx_url_index_snapshots_run_id",
		"on url_index_snapshots (run_id)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration should contain %q", want)
		}
	}
}
