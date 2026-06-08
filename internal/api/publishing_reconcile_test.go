package api

import (
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestPublishingReconcileCandidateClassification(t *testing.T) {
	canonicalURL := "https://example.com/a"
	if !isReconcileCandidate(db.Article{Kind: "canonical", Status: "approved", PublishResult: []byte(`{"url":"https://example.com/a"}`)}) {
		t.Fatal("approved canonical with publish_result should be a reconcile candidate")
	}
	if isReconcileCandidate(db.Article{Kind: "syndication_variant", Status: "approved", PublishResult: []byte(`{"url":"https://example.com/a"}`)}) {
		t.Fatal("syndication variants should not be reconcile candidates")
	}
	if !isReconcileCandidate(db.Article{Kind: "canonical", Status: "pending_url_verification"}) {
		t.Fatal("pending URL verification canonical should be a reconcile candidate")
	}
	if !isReconcileCandidate(db.Article{Kind: "canonical", Status: "published", PublishResult: []byte(`{"url":"https://example.com/a"}`)}) {
		t.Fatal("published canonical missing canonical URL should be a reconcile candidate")
	}
	if isReconcileCandidate(db.Article{
		Kind:                   "canonical",
		Status:                 "published",
		PublishResult:          []byte(`{"url":"https://example.com/a"}`),
		CanonicalUrl:           &canonicalURL,
		CanonicalUrlVerifiedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}) {
		t.Fatal("fully verified published canonical should not be a reconcile candidate")
	}
}

func TestPublishingSkippedReasonsExplainPendingReview(t *testing.T) {
	reasons := publishingSkippedReasons(
		publishingLaneCountsDTO{PendingReview: 2, ApprovedVariantsWaitingCanonical: 1},
		publishingHealthDTO{Ready: false, Reasons: []string{"publisher_missing"}, NextAction: "Connect publisher."},
	)
	seen := map[string]int{}
	for _, reason := range reasons {
		seen[reason.Reason] = reason.Count
	}
	if seen["publisher_blocked"] != 1 {
		t.Fatalf("publisher blocker missing: %#v", seen)
	}
	if seen["drafts_waiting_review"] != 2 {
		t.Fatalf("pending review explanation missing: %#v", seen)
	}
	if seen["variants_waiting_canonical"] != 1 {
		t.Fatalf("variant waiting explanation missing: %#v", seen)
	}
	if _, ok := seen["no_reconcile_candidates"]; !ok {
		t.Fatalf("no reconcile candidate reason missing: %#v", seen)
	}
}
