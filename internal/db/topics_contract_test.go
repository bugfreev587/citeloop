package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTopicMutationQueriesAreProjectScoped(t *testing.T) {
	if !strings.Contains(getTopicForProject, "where id = $1") || !strings.Contains(getTopicForProject, "and project_id = $2") {
		t.Fatal("GetTopicForProject must be scoped by topic id and project id")
	}
	if !strings.Contains(updateTopic, "where id = $1") || !strings.Contains(updateTopic, "and project_id = $2") {
		t.Fatal("UpdateTopic must be scoped by topic id and project id")
	}
	if !strings.Contains(setTopicScheduledAtForProject, "where id = $1") || !strings.Contains(setTopicScheduledAtForProject, "and project_id = $2") {
		t.Fatal("SetTopicScheduledAtForProject must be scoped by topic id and project id")
	}
	if !strings.Contains(archiveTopicForProject, "where id = $1") || !strings.Contains(archiveTopicForProject, "and project_id = $2") {
		t.Fatal("ArchiveTopicForProject must be scoped by topic id and project id")
	}
	if !strings.Contains(listArticlesByTopicForProject, "where topic_id = $1") || !strings.Contains(listArticlesByTopicForProject, "and project_id = $2") {
		t.Fatal("ListArticlesByTopicForProject must be scoped by topic id and project id")
	}
}

func TestTopicPriorityBackfillMigrationCoversZeroPriorityBacklog(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "migrations", "0016_backfill_topic_priority.sql"))
	if err != nil {
		t.Fatal(err)
	}
	migration := strings.ToLower(string(body))

	for _, want := range []string{
		"source_content_action_id",
		"seo_opportunities",
		"priority_score",
		"where t.priority <= 0",
		"greatest(1",
		"least(100",
		"row_number() over",
		"status in ('backlog','scheduled','generating')",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("topic priority backfill migration missing %q", want)
		}
	}
}
