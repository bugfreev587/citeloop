package api

import (
	"context"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

type publishingLaneCountsDTO struct {
	ApprovedCanonicalDue             int `json:"approved_canonical_due"`
	ApprovedCanonicalScheduled       int `json:"approved_canonical_scheduled"`
	ApprovedVariantsWaitingCanonical int `json:"approved_variants_waiting_canonical"`
	PendingReview                    int `json:"pending_review"`
	PendingURLVerification           int `json:"pending_url_verification"`
	PublishFailures                  int `json:"publish_failures"`
	RetryableFailures                int `json:"retryable_failures"`
	ReadyToDistribute                int `json:"ready_to_distribute"`
	PublishedCanonical               int `json:"published_canonical"`
	ReconcileCandidates              int `json:"reconcile_candidates"`
}

type publishingSkippedReasonDTO struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
	Detail string `json:"detail"`
}

type publishingReconcileResultDTO struct {
	Status             string                       `json:"status"`
	CheckedArticles    int                          `json:"checked_articles"`
	PublishableCount   int                          `json:"publishable_count"`
	RepairedStateCount int                          `json:"repaired_state_count"`
	SkippedReasons     []publishingSkippedReasonDTO `json:"skipped_reasons"`
	Blockers           []string                     `json:"blockers"`
	Counts             publishingLaneCountsDTO      `json:"counts"`
	Health             publishingHealthDTO          `json:"health"`
}

func (s *Server) publishingReconcileSummary(ctx context.Context, projectID uuid.UUID, health publishingHealthDTO, status string) (publishingReconcileResultDTO, error) {
	counts, err := s.publishingLaneCounts(ctx, projectID, time.Now())
	if err != nil {
		return publishingReconcileResultDTO{}, err
	}
	result := publishingReconcileResultDTO{
		Status:           status,
		CheckedArticles:  counts.ReconcileCandidates,
		PublishableCount: counts.ApprovedCanonicalDue + counts.RetryableFailures,
		SkippedReasons:   publishingSkippedReasons(counts, health),
		Blockers:         append([]string(nil), health.Reasons...),
		Counts:           counts,
		Health:           health,
	}
	if result.Status == "" {
		result.Status = "checked"
	}
	return result, nil
}

func (s *Server) publishingLaneCounts(ctx context.Context, projectID uuid.UUID, now time.Time) (publishingLaneCountsDTO, error) {
	var counts publishingLaneCountsDTO
	approved, err := s.Q.ListArticlesByStatus(ctx, db.ListArticlesByStatusParams{ProjectID: projectID, Status: "approved"})
	if err != nil {
		return counts, err
	}
	pendingReview, err := s.Q.ListArticlesByStatus(ctx, db.ListArticlesByStatusParams{ProjectID: projectID, Status: "pending_review"})
	if err != nil {
		return counts, err
	}
	pendingURL, err := s.Q.ListArticlesByStatus(ctx, db.ListArticlesByStatusParams{ProjectID: projectID, Status: "pending_url_verification"})
	if err != nil {
		return counts, err
	}
	failed, err := s.Q.ListArticlesByStatus(ctx, db.ListArticlesByStatusParams{ProjectID: projectID, Status: "publish_failed"})
	if err != nil {
		return counts, err
	}
	ready, err := s.Q.ListArticlesByStatus(ctx, db.ListArticlesByStatusParams{ProjectID: projectID, Status: "ready_to_distribute"})
	if err != nil {
		return counts, err
	}
	published, err := s.Q.ListArticlesByStatus(ctx, db.ListArticlesByStatusParams{ProjectID: projectID, Status: "published"})
	if err != nil {
		return counts, err
	}

	counts.PendingReview = len(pendingReview)
	counts.PendingURLVerification = len(pendingURL)
	counts.PublishFailures = len(failed)
	counts.ReadyToDistribute = len(ready)

	for _, article := range approved {
		switch article.Kind {
		case "canonical":
			if dueAtOrBefore(article.ScheduledAt.Time, article.ScheduledAt.Valid, now) {
				counts.ApprovedCanonicalDue++
			} else {
				counts.ApprovedCanonicalScheduled++
			}
			if isReconcileCandidate(article) {
				counts.ReconcileCandidates++
			}
		case "syndication_variant":
			counts.ApprovedVariantsWaitingCanonical++
		}
	}
	for _, article := range failed {
		if article.Kind == "canonical" && dueAtOrBefore(article.NextPublishRetryAt.Time, article.NextPublishRetryAt.Valid, now) {
			counts.RetryableFailures++
		}
		if isReconcileCandidate(article) {
			counts.ReconcileCandidates++
		}
	}
	for _, article := range pendingURL {
		if isReconcileCandidate(article) {
			counts.ReconcileCandidates++
		}
	}
	for _, article := range published {
		if article.Kind == "canonical" {
			counts.PublishedCanonical++
		}
		if isReconcileCandidate(article) {
			counts.ReconcileCandidates++
		}
	}
	return counts, nil
}

func dueAtOrBefore(value time.Time, valid bool, now time.Time) bool {
	return valid && !value.After(now)
}

func isReconcileCandidate(article db.Article) bool {
	if article.Kind != "canonical" {
		return false
	}
	switch article.Status {
	case "approved", "publish_failed":
		return hasPublishResult(article)
	case "pending_url_verification":
		return true
	case "published":
		return article.CanonicalUrl == nil || !article.CanonicalUrlVerifiedAt.Valid || !hasPublishResult(article)
	default:
		return false
	}
}

func hasPublishResult(article db.Article) bool {
	trimmed := strings.TrimSpace(string(article.PublishResult))
	return trimmed != "" && trimmed != "null"
}

func publishingSkippedReasons(counts publishingLaneCountsDTO, health publishingHealthDTO) []publishingSkippedReasonDTO {
	reasons := []publishingSkippedReasonDTO{}
	if !health.Ready {
		reasons = append(reasons, publishingSkippedReasonDTO{
			Reason: "publisher_blocked",
			Count:  1,
			Detail: health.NextAction,
		})
	}
	if counts.ReconcileCandidates == 0 {
		reasons = append(reasons, publishingSkippedReasonDTO{
			Reason: "no_reconcile_candidates",
			Count:  0,
			Detail: "No previous publish attempts need URL or file verification right now.",
		})
	}
	if counts.ApprovedCanonicalDue == 0 && counts.RetryableFailures == 0 {
		reasons = append(reasons, publishingSkippedReasonDTO{
			Reason: "no_publishable_canonical_due",
			Count:  0,
			Detail: "No approved canonical articles are due for publishing right now.",
		})
	}
	if counts.PendingReview > 0 {
		reasons = append(reasons, publishingSkippedReasonDTO{
			Reason: "drafts_waiting_review",
			Count:  counts.PendingReview,
			Detail: "Drafts are waiting in Review and are not publishable until approved.",
		})
	}
	if counts.ApprovedVariantsWaitingCanonical > 0 {
		reasons = append(reasons, publishingSkippedReasonDTO{
			Reason: "variants_waiting_canonical",
			Count:  counts.ApprovedVariantsWaitingCanonical,
			Detail: "Syndication variants unlock after their canonical article is published and verified.",
		})
	}
	return reasons
}
