package db

import (
	"strings"
	"testing"
)

func TestRunsQueriesExposeListAndDetailContracts(t *testing.T) {
	if !strings.Contains(listGenerationRuns, "where project_id = $1") {
		t.Fatal("ListGenerationRuns must be project-scoped")
	}
	for _, want := range []string{
		"($2::text = '' or agent = $2)",
		"($3::text = '' or status = $3)",
		"($4::timestamptz is null or created_at < $4)",
		"order by created_at desc",
	} {
		if !strings.Contains(listGenerationRuns, want) {
			t.Fatalf("ListGenerationRuns missing %q", want)
		}
	}
	if !strings.Contains(getGenerationRun, "where id = $1") || !strings.Contains(getGenerationRun, "and project_id = $2") {
		t.Fatal("GetGenerationRun must be scoped by run id and project id")
	}
}
