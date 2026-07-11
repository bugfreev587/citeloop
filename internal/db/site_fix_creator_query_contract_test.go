package db

import (
	"os"
	"strings"
	"testing"
)

func TestCanonicalSiteFixCreatorQueries(t *testing.T) {
	discoverySQL, err := os.ReadFile("queries/discovery.sql")
	if err != nil {
		t.Fatal(err)
	}
	siteFixSQL, err := os.ReadFile("queries/site_fixes.sql")
	if err != nil {
		t.Fatal(err)
	}

	for _, required := range []string{
		"-- name: EnsureCanonicalDiscoveryRun :one",
		"-- name: CompleteCanonicalDiscoveryRun :one",
		"-- name: GetDiscoveryCandidateForArbitration :one",
		"-- name: GetSEODoctorFindingForSiteFixForUpdate :one",
		"for update",
		"sqlc.arg(id)",
	} {
		if !strings.Contains(strings.ToLower(string(discoverySQL)), strings.ToLower(required)) {
			t.Fatalf("discovery queries missing %q", required)
		}
	}
	if !strings.Contains(string(siteFixSQL), "-- name: GetLatestCanonicalSiteFixForFindingForUpdate :one") {
		t.Fatal("site-fix queries must lock the latest predecessor")
	}
	if !strings.Contains(string(siteFixSQL), "-- name: GetActiveCanonicalSiteFixForFindingForUpdate :one") {
		t.Fatal("site-fix queries must reject any active predecessor")
	}
	if !strings.Contains(strings.ToLower(string(discoverySQL)), "run.status = 'completed'") {
		t.Fatal("arbitration must not load a partially materialized canonical candidate")
	}
	signatureInsert := queryContractSection(t, string(discoverySQL), "InsertEnforcedWorkSignature")
	if !strings.Contains(signatureInsert, "(id, project_id") || !strings.Contains(signatureInsert, "(sqlc.arg(id), sqlc.arg(project_id)") {
		t.Fatal("enforced signature insertion must use the caller's preallocated id")
	}
}

func queryContractSection(t *testing.T, sql, name string) string {
	t.Helper()
	start := strings.Index(sql, "-- name: "+name+" ")
	if start < 0 {
		t.Fatalf("query %s not found", name)
	}
	rest := sql[start+1:]
	if end := strings.Index(rest, "-- name: "); end >= 0 {
		return sql[start : start+1+end]
	}
	return sql[start:]
}
