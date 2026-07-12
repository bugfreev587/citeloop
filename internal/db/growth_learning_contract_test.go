package db

import (
	"os"
	"strings"
	"testing"
)

func TestGrowthLearningMigrationSeparatesDirectionalAndQualityRecords(t *testing.T) {
	raw, err := os.ReadFile("../migrations/0076_growth_learnings.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"create table if not exists growth_terminal_outcomes",
		"create table if not exists growth_learnings",
		"create table if not exists measurement_quality_records",
		"unique (project_id, content_action_id)",
		"positive",
		"negative",
		"mixed",
		"inconclusive",
		"insufficient_data",
		"directional_learning",
		"measurement_quality",
		"scoring_eligible boolean not null default false",
		"check (scoring_eligible = false)",
		"growth terminal records are immutable",
		"deferrable initially deferred",
		"candidate_id uuid references discovery_candidates(id) on delete no action deferrable initially deferred",
		"opportunity_id uuid not null references seo_opportunities(id) on delete no action deferrable initially deferred",
		"content_action_id uuid not null references content_actions(id) on delete no action deferrable initially deferred",
		"article_id uuid references articles(id) on delete no action deferrable initially deferred",
		"enforce_growth_terminal_project_scope",
		"opportunity.project_id = new.project_id",
		"action.project_id = new.project_id",
		"action.opportunity_id = new.opportunity_id",
		"candidate.project_id = new.project_id",
		"article.project_id = new.project_id",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("Growth learning migration missing %q", want)
		}
	}
}

func TestGrowthLearningQueryIsIdempotentAndLinksCanonicalObjects(t *testing.T) {
	raw, err := os.ReadFile("queries/growth_learning.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"-- name: recordgrowthterminaloutcome :exec",
		"on conflict (project_id, content_action_id) do nothing",
		"from work_signature_registry",
		"reserved_work_id",
		"insert into growth_learnings",
		"insert into measurement_quality_records",
		"-- name: listgrowthlearnings :many",
		"-- name: listapplicablegrowthlearnings :many",
		"jsonb_each_text(sqlc.arg(target_identity)::jsonb)",
		"jsonb_array_elements_text(sqlc.arg(audience)::jsonb)",
		"learning.scoring_eligible = true",
		"-- name: listmeasurementqualityrecords :many",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("Growth learning query missing %q", want)
		}
	}
}

func TestCanonicalGrowthMergeKeepsLearningScoreAndProvenanceAligned(t *testing.T) {
	raw, err := os.ReadFile("queries/seo.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(raw))
	for _, want := range []string{
		"when sqlc.arg(evidence)::jsonb ? 'learning_scoring'",
		"then sqlc.arg(incoming_priority_score)::numeric",
		"jsonb_build_object('learning_scoring', sqlc.arg(evidence)::jsonb->'learning_scoring')",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("canonical Growth merge missing %q", want)
		}
	}
}
