package db

import (
	"strings"
	"testing"
)

func TestDiscoveryReservationQueriesEnforceShortTransaction(t *testing.T) {
	queries := map[string]string{
		"GetArbitrationDecision":            getArbitrationDecision,
		"LockArbitrationDecisionForReserve": lockArbitrationDecisionForReserve,
		"LockConflictBucketsForReserve":     lockConflictBucketsForReserve,
		"LockDiscoveryCandidateForReserve":  lockDiscoveryCandidateForReserve,
		"InsertEnforcedWorkSignature":       insertEnforcedWorkSignature,
		"IncrementConflictBucketVersions":   incrementConflictBucketVersions,
		"MarkArbitrationDecisionReserved":   markArbitrationDecisionReserved,
	}
	candidateLock := strings.ToLower(lockDiscoveryCandidateForReserve)
	if !strings.Contains(candidateLock, "for update") || !strings.Contains(candidateLock, "project_id") {
		t.Fatalf("candidate reservation lock must be project scoped and FOR UPDATE: %s", lockDiscoveryCandidateForReserve)
	}
	for name, query := range queries {
		if strings.TrimSpace(query) == "" {
			t.Fatalf("%s query should exist", name)
		}
	}
	locks := strings.ToLower(lockConflictBucketsForReserve)
	for _, want := range []string{"order by bucket_key asc", "for update", "project_id"} {
		if !strings.Contains(locks, want) {
			t.Fatalf("bucket lock query missing %q: %s", want, lockConflictBucketsForReserve)
		}
	}
	insert := strings.ToLower(insertEnforcedWorkSignature)
	for _, want := range []string{"'enforced'", "'reserved'", "true", "arbitration_decision_id", "reserved_work_type", "reserved_work_id"} {
		if !strings.Contains(insert, want) {
			t.Fatalf("enforced signature insert missing %q: %s", want, insertEnforcedWorkSignature)
		}
	}
	increment := strings.ToLower(incrementConflictBucketVersions)
	if !strings.Contains(increment, "bucket_version = bucket_version + 1") {
		t.Fatalf("reservation must increment every locked bucket version: %s", incrementConflictBucketVersions)
	}
}
