package scheduler

import (
	"os"
	"strings"
	"testing"
)

func TestCanonicalDoctorSiteFixSchedulerContract(t *testing.T) {
	queryRaw, err := os.ReadFile("../db/queries/site_fixes.sql")
	if err != nil {
		t.Fatal(err)
	}
	queries := strings.ToLower(string(queryRaw))
	for _, name := range []string{
		"-- name: listcanonicalsitefixprsforreconciliation :many",
		"-- name: listcanonicalsitefixesforverification :many",
		"-- name: markcanonicalsitefixprmerged :one",
		"-- name: markcanonicalsitefixmanualapplied :one",
		"-- name: markcanonicalsitefixapplyfailure :one",
	} {
		if !strings.Contains(queries, name) {
			t.Fatalf("canonical scheduler query missing %q", name)
		}
	}
	merged := namedQueryBody(t, queries, "markcanonicalsitefixprmerged")
	if !strings.Contains(merged, "next_poll_at") || !strings.Contains(merged, "interval '3 minutes'") {
		t.Fatal("PR merge must atomically schedule the deploy-grace poll")
	}
	for _, forbidden := range []string{"applied_at =", "deployed_at ="} {
		if strings.Contains(merged, forbidden) {
			t.Fatalf("PR merge is not production evidence; found %q", forbidden)
		}
	}
	verifying := namedQueryBody(t, queries, "markcanonicalsitefixverifying")
	for _, required := range []string{
		"applied_at = coalesce(applied_at, sqlc.arg(deployed_at))",
		"deployed_at = coalesce(deployed_at, sqlc.arg(deployed_at))",
	} {
		if !strings.Contains(verifying, required) {
			t.Fatalf("production evidence transition missing %q", required)
		}
	}
	for _, name := range []string{
		"listcanonicalsitefixprsforreconciliation",
		"listcanonicalsitefixesforverification",
	} {
		body := namedQueryBody(t, queries, name)
		for _, required := range []string{"site_fix_id is not null", "content_action_id is null"} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s must require %q", name, required)
			}
		}
	}

	schedulerRaw, err := os.ReadFile("sitefix_verify.go")
	if err != nil {
		t.Fatal(err)
	}
	scheduler := goFunctionBody(t, string(schedulerRaw), "func (s *Scheduler) reconcileCanonicalSiteFixVerificationForProject")
	for _, forbidden := range []string{
		"MarkSiteChangeApplicationAndContentActionVerified",
		"auto-verify fuzzy",
		"auto_grace_period",
	} {
		if strings.Contains(scheduler, forbidden) {
			t.Fatalf("canonical Doctor scheduler still contains legacy/fuzzy behavior %q", forbidden)
		}
	}
	if strings.Contains(scheduler, `fix.Status == "failed_retryable" || fix.Status == "reopened"`) {
		t.Fatal("reopened Site Fixes must re-enter verifying and execute acceptance tests")
	}
}

func goFunctionBody(t *testing.T, source, signature string) string {
	t.Helper()
	start := strings.Index(source, signature)
	if start < 0 {
		t.Fatalf("function %q not found", signature)
	}
	depth := 0
	started := false
	for i := start; i < len(source); i++ {
		switch source[i] {
		case '{':
			depth++
			started = true
		case '}':
			depth--
			if started && depth == 0 {
				return source[start : i+1]
			}
		}
	}
	t.Fatalf("function %q is not balanced", signature)
	return ""
}

func namedQueryBody(t *testing.T, sql, lowerName string) string {
	t.Helper()
	start := strings.Index(sql, "-- name: "+lowerName+" ")
	if start < 0 {
		t.Fatalf("query %s not found", lowerName)
	}
	rest := sql[start+1:]
	if end := strings.Index(rest, "-- name:"); end >= 0 {
		return sql[start : start+1+end]
	}
	return sql[start:]
}
