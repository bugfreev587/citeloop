package db

import (
	"os"
	"strings"
	"testing"
)

func TestPlatformContentContractsMigration(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0085_platform_content_contracts.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, required := range []string{
		"create table if not exists platform_content_contracts",
		"create table if not exists platform_target_contexts",
		"create table if not exists content_target_plans",
		"create table if not exists content_target_plan_items",
		"uniq_active_platform_content_contract",
		"uniq_current_target_context",
		"uniq_target_plan_item",
		"add column if not exists asset_type text",
		"add column if not exists target_plan_id uuid",
		"add column if not exists platform_contract_id uuid",
		"add column if not exists platform_contract_version text",
		"add column if not exists target_context_id uuid",
		"add column if not exists output_type text",
		"source_backed_evidence_page",
		"faq_answer_block",
	} {
		if !strings.Contains(strings.ToLower(body), strings.ToLower(required)) {
			t.Errorf("migration missing %q", required)
		}
	}
	for _, platform := range []string{"blog", "dev_to", "hashnode", "medium", "linkedin", "reddit", "hacker_news"} {
		if !strings.Contains(body, "'"+platform+"'") {
			t.Errorf("migration does not seed platform %q", platform)
		}
	}
}

func TestPlatformContentContractsMigrationBackfillsExistingGEOAssetTypesSafely(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0085_platform_content_contracts.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(raw))
	for _, required := range []string{
		"geo_asset_briefs",
		"source_backed_evidence_page",
		"jsonb_set",
		"seo_meta",
		"asset_type is null",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("safe asset-type backfill missing %q", required)
		}
	}
}

func TestPlatformContentContractQueriesExposeLifecycleAndPlanning(t *testing.T) {
	raw, err := os.ReadFile("queries/platform_contracts.sql")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, required := range []string{
		"-- name: ListActivePlatformContentContracts",
		"-- name: GetActivePlatformContentContract",
		"-- name: ListPlatformTargetContexts",
		"-- name: ConfirmPlatformTargetContext",
		"-- name: CreateContentTargetPlan",
		"-- name: CreateContentTargetPlanItem",
		"-- name: ListContentTargetPlanItems",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("query contract missing %q", required)
		}
	}
}
