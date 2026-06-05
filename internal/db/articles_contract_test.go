package db

import (
	"strings"
	"testing"
)

func TestArticleMutationQueriesAreProjectScoped(t *testing.T) {
	for name, query := range map[string]string{
		"GetArticleForProject":           getArticleForProject,
		"UpdateArticleContentForProject": updateArticleContentForProject,
		"ApproveArticleForProject":       approveArticleForProject,
		"RejectArticleForProject":        rejectArticleForProject,
		"MarkDistributedForProject":      markDistributedForProject,
	} {
		if !strings.Contains(query, "where id = $1") || !strings.Contains(query, "and project_id = $") {
			t.Fatalf("%s must be scoped by article id and project id", name)
		}
	}
}
