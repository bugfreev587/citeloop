package db

import (
	"os"
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

func TestArticleRepairStateSchemaIsPersisted(t *testing.T) {
	migration, err := os.ReadFile("../migrations/0009_article_repair_state.sql")
	if err != nil {
		t.Fatalf("read repair migration: %v", err)
	}
	sql := string(migration)
	for _, column := range []string{
		"repair_attempts",
		"last_repair_at",
		"repair_status",
		"repair_failure_reason",
		"requires_human_decision",
		"human_decision_options",
		"qa_feedback",
	} {
		if !strings.Contains(sql, column) {
			t.Fatalf("repair migration must add %s", column)
		}
	}
	if !strings.Contains(sql, "repair_status in") {
		t.Fatal("repair_status must have an explicit check constraint")
	}
}

func TestArticleRepairQueriesAreProjectScopedAndBounded(t *testing.T) {
	for name, query := range map[string]string{
		"StartArticleRepairForProject":  startArticleRepairForProject,
		"FinishArticleRepairForProject": finishArticleRepairForProject,
	} {
		if !strings.Contains(query, "project_id = $2") {
			t.Fatalf("%s must be project scoped", name)
		}
	}
	if !strings.Contains(startArticleRepairForProject, "repair_attempts < $3") {
		t.Fatal("StartArticleRepairForProject must enforce a DB-level repair attempt cap")
	}
	if strings.Contains(startArticleRepairForProject, "and requires_human_decision = false") {
		t.Fatal("StartArticleRepairForProject must let the caller reopen editor-repairable human-decision rows")
	}
	if !strings.Contains(startArticleRepairForProject, "requires_human_decision = false") {
		t.Fatal("StartArticleRepairForProject must clear stale human-decision state when repair starts")
	}
	if !strings.Contains(startArticleRepairForProject, "human_decision_options = '[]'") {
		t.Fatal("StartArticleRepairForProject must clear stale human decision options when repair starts")
	}
}

func TestReviewRecoveryQueryCanReopenStaleHumanDecisions(t *testing.T) {
	if strings.Contains(listRecoverableArticlesForProject, "and requires_human_decision = false") {
		t.Fatal("ListRecoverableArticlesForProject must not exclude stale editor-repairable human-decision rows")
	}
	if !strings.Contains(listRecoverableArticlesForProject, "order by requires_human_decision asc") {
		t.Fatal("ListRecoverableArticlesForProject must prioritize normal recovery rows before stale human-decision rows")
	}
}

func TestCountStockedCanonicalIncludesReservedGeneratingTopics(t *testing.T) {
	if !strings.Contains(countStockedCanonical, "from topics") {
		t.Fatal("CountStockedCanonical must count reserved generating topics before their first article exists")
	}
	if !strings.Contains(countStockedCanonical, "status = 'generating'") {
		t.Fatal("CountStockedCanonical must include topics.status='generating' as in-flight generation")
	}
	if !strings.Contains(countStockedCanonical, "union") {
		t.Fatal("CountStockedCanonical should deduplicate article-backed and topic-backed in-flight work")
	}
}
