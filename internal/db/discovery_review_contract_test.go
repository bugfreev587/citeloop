package db

import (
	"strings"
	"testing"
)

func TestDiscoveryReviewResolutionQueriesAreStaleSafe(t *testing.T) {
	queries := map[string]string{
		"LockDiscoveryReviewItemForResolve": lockDiscoveryReviewItemForResolve,
		"ResolveDiscoveryReviewItem":        resolveDiscoveryReviewItem,
		"DeactivateWorkReviewMemory":        deactivateWorkReviewMemory,
	}
	for name, query := range queries {
		if strings.TrimSpace(query) == "" {
			t.Fatalf("%s query should exist", name)
		}
	}
	lock := strings.ToLower(lockDiscoveryReviewItemForResolve)
	for _, want := range []string{"project_id", "candidate_id", "for update"} {
		if !strings.Contains(lock, want) {
			t.Fatalf("review lock missing %q: %s", want, lockDiscoveryReviewItemForResolve)
		}
	}
	resolve := strings.ToLower(resolveDiscoveryReviewItem)
	for _, want := range []string{"state = 'resolved'", "resolution", "resolved_by", "arbitration_decision_id", "state <> 'resolved'"} {
		if !strings.Contains(resolve, want) {
			t.Fatalf("review resolution missing %q: %s", want, resolveDiscoveryReviewItem)
		}
	}
	if strings.Contains(resolve, "site_fixes") || strings.Contains(resolve, "seo_opportunities") || strings.Contains(resolve, "content_actions") {
		t.Fatal("review resolution must not create user work")
	}
}
