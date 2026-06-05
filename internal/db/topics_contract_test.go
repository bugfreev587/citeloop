package db

import (
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
