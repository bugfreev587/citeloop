package api

import (
	"os"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestVisibilitySummaryRouteAndLifecycleContract(t *testing.T) {
	serverRaw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	handlerRaw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	server := string(serverRaw)
	handler := string(handlerRaw)

	for _, want := range []string{
		`r.Get("/visibility/summary", s.getVisibilitySummary)`,
	} {
		if !strings.Contains(server, want) {
			t.Fatalf("visibility summary route missing %q", want)
		}
	}

	for _, want := range []string{
		"func (s *Server) getVisibilitySummary",
		"type VisibilitySummary",
		"capability_mode",
		"primary_status",
		"setup_blockers",
		"open_opportunities",
		"actions_in_loop",
		"lifecycle_counts",
		"top_measurement_updates",
		"diagnostics_health",
		"deriveVisibilityLifecycleStage",
	} {
		if !strings.Contains(handler, want) {
			t.Fatalf("visibility summary handler missing %q", want)
		}
	}

	for _, stage := range []string{
		"detected",
		"added_to_plan",
		"planned",
		"drafting",
		"ready_for_review",
		"approved",
		"published_or_applied",
		"measuring",
		"learned",
		"blocked",
	} {
		if !strings.Contains(handler, `"`+stage+`"`) {
			t.Fatalf("visibility lifecycle stage %q missing from handler", stage)
		}
	}
}

func TestAcceptSEOOpportunityAliasesActionCreation(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(raw)
	start := strings.Index(source, "func (s *Server) acceptSEOOpportunity")
	end := strings.Index(source, "func (s *Server) dismissSEOOpportunity")
	if start < 0 || end < 0 || end <= start {
		t.Fatal("could not locate acceptSEOOpportunity body")
	}
	body := source[start:end]
	if !strings.Contains(body, "createSEOContentActionFromOpportunity") {
		t.Fatal("acceptSEOOpportunity must call the shared content action creation helper")
	}
	if strings.Contains(body, `"accepted"`) || strings.Contains(body, "updateSEOOpportunityStatus") {
		t.Fatal("acceptSEOOpportunity must not leave an accepted-without-action state")
	}
}

func TestVisibilityLifecycleOnlyTreatsPendingReviewDraftAsReviewHandoff(t *testing.T) {
	actionID := uuid.New()
	topicID := uuid.New()
	pending := "pending_review"
	approved := "approved"
	published := "published"

	stage := deriveVisibilityLifecycleStage(db.ListVisibilityActionRowsRow{
		Status:             "approved",
		TopicID:            pgtype.UUID{Bytes: topicID, Valid: true},
		DraftArticleID:     pgtype.UUID{Bytes: actionID, Valid: true},
		DraftArticleStatus: nil,
	})
	if stage != VisibilityStagePlanned {
		t.Fatalf("stale draft id stage = %q, want %q", stage, VisibilityStagePlanned)
	}

	stage = deriveVisibilityLifecycleStage(db.ListVisibilityActionRowsRow{
		Status:             "approved",
		TopicID:            pgtype.UUID{Bytes: topicID, Valid: true},
		DraftArticleID:     pgtype.UUID{Bytes: actionID, Valid: true},
		DraftArticleStatus: &pending,
	})
	if stage != VisibilityStageReadyForReview {
		t.Fatalf("pending review draft stage = %q, want %q", stage, VisibilityStageReadyForReview)
	}

	stage = deriveVisibilityLifecycleStage(db.ListVisibilityActionRowsRow{
		Status:             "completed",
		DraftArticleID:     pgtype.UUID{Bytes: actionID, Valid: true},
		DraftArticleStatus: &approved,
		VerifiedAt:         pgtype.Timestamptz{Valid: true},
	})
	if stage != VisibilityStageApproved {
		t.Fatalf("stale completed parent with unpublished approved draft stage = %q, want %q", stage, VisibilityStageApproved)
	}

	stage = deriveVisibilityLifecycleStage(db.ListVisibilityActionRowsRow{
		Status:             "measuring",
		DraftArticleID:     pgtype.UUID{Bytes: actionID, Valid: true},
		DraftArticleStatus: &pending,
		VerifiedAt:         pgtype.Timestamptz{Valid: true},
	})
	if stage != VisibilityStageReadyForReview {
		t.Fatalf("stale measuring parent with pending review draft stage = %q, want %q", stage, VisibilityStageReadyForReview)
	}

	stage = deriveVisibilityLifecycleStage(db.ListVisibilityActionRowsRow{
		Status:             "completed",
		DraftArticleID:     pgtype.UUID{Bytes: actionID, Valid: true},
		DraftArticleStatus: &published,
		PublishedAt:        pgtype.Timestamptz{Valid: true},
	})
	if stage != VisibilityStageLearned {
		t.Fatalf("completed parent with published draft stage = %q, want %q", stage, VisibilityStageLearned)
	}
}
