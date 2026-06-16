package scheduler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/citeloop/citeloop/internal/agents"
	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/topicstate"
	"github.com/citeloop/citeloop/internal/workflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	// reviewRecoveryLimit bounds how many blocked drafts we work per project per tick.
	reviewRecoveryLimit = 25
	// maxReviewRecoveryAttempts is the total automated-recovery budget per draft
	// (re-QA + repair). After this it escalates to a genuine human decision.
	maxReviewRecoveryAttempts = 4
	// maxRequalifyBeforeRegen is how many bare QA re-runs we try on an
	// un-evaluated ("no claim map") draft before regenerating it from scratch.
	maxRequalifyBeforeRegen = 2
	// maxTopicRegenerations bounds fresh-draft regenerations per topic so a
	// chronically un-draftable topic eventually reaches a human instead of looping.
	maxTopicRegenerations = 2
	// draftRepairBudget mirrors the Writer's per-draft AI repair cap.
	draftRepairBudget = 2
	autoReviewer      = "citeloop-auto"
)

// TickReviewRecovery drains the review queue without a human: for every
// auto-advance project it pushes blocked-but-not-human drafts through re-QA →
// AI repair → regeneration, escalating to a genuine human decision only when QA
// actually finds an unresolvable evidence/positioning issue or the automated
// budget is spent. It then auto-approves any draft QA has cleared so a hands-off
// project never parks clean content in front of the operator (§5.5).
func (s *Scheduler) TickReviewRecovery(ctx context.Context) {
	q := db.New(s.Pool)
	projects, err := q.ListProjects(ctx)
	if err != nil {
		s.Log.Error("review recovery: list projects", "err", err)
		return
	}
	for _, p := range projects {
		if err := s.recoverReviewForProject(ctx, p); err != nil {
			s.Log.Error("review recovery tick failed", "project", p.ID, "err", err)
		}
	}
}

func (s *Scheduler) recoverReviewForProject(ctx context.Context, p db.Project) error {
	cfg, err := config.Parse(p.Config)
	if err != nil {
		return err
	}
	// Hands-off recovery + auto-approval only run when the project opted into
	// auto-advance; otherwise the human stays the gate.
	if !cfg.AutoAdvanceEnabled {
		return nil
	}
	q := db.New(s.Pool)

	// Cost breaker (§5.4): recovery makes LLM calls, so stop when the month's
	// spend has reached the budget.
	if spent, err := q.MonthlySpend(ctx, p.ID); err == nil && pgutil.Float(spent) >= cfg.MonthlyBudgetUSD {
		return nil
	}

	recoverable, err := q.ListRecoverableArticlesForProject(ctx, db.ListRecoverableArticlesForProjectParams{
		ProjectID: p.ID,
		Limit:     reviewRecoveryLimit,
	})
	if err != nil {
		return err
	}
	for _, art := range recoverable {
		if err := s.recoverArticle(ctx, q, p.ID, art); err != nil {
			s.Log.Warn("review recovery: article step failed", "project", p.ID, "article", art.ID, "err", err)
		}
	}

	s.autoApproveReadyForProject(ctx, q, p.ID, cfg)
	return nil
}

// recoverArticle advances one blocked draft by exactly one automated step so the
// work is bounded and observable: each tick either re-runs QA, repairs, or
// regenerates, then lets the next tick re-evaluate the fresh state.
func (s *Scheduler) recoverArticle(ctx context.Context, q *db.Queries, projectID uuid.UUID, art db.Article) error {
	// QA found a real decision a human must make — hand it over.
	if articleNeedsHuman(art) {
		return s.escalateArticle(ctx, q, projectID, art, "QA found a product claim that needs a human evidence or positioning decision.")
	}
	if art.RecoveryAttempts >= maxReviewRecoveryAttempts {
		return s.escalateArticle(ctx, q, projectID, art, "CiteLoop could not automatically produce a publishable draft after several attempts.")
	}

	claimed, err := q.IncrementArticleRecoveryAttempt(ctx, db.IncrementArticleRecoveryAttemptParams{ID: art.ID, ProjectID: projectID})
	if err != nil {
		return err
	}

	switch {
	case articleCanAutoFix(claimed) && claimed.RepairAttempts < draftRepairBudget:
		// QA evaluated the draft and flagged a safe, editor-fixable issue.
		writer := agents.NewWriter(agents.Deps{Q: q, LLM: s.LLM, Search: s.Search}, s.Log)
		_, err := writer.RepairArticle(ctx, projectID, claimed.ID)
		return err
	case !articleHasClaimMap(claimed) && claimed.RecoveryAttempts <= maxRequalifyBeforeRegen:
		// QA never returned a claim map (infrastructure/parse failure, not a
		// content problem) — re-run QA; transient failures clear here.
		qa := agents.NewQA(agents.Deps{Q: q, LLM: s.LLM, Search: s.Search}, s.Log)
		_, err := qa.Requalify(ctx, projectID, claimed.ID)
		return err
	case articleHasClaimMap(claimed):
		// QA evaluated but the draft is still blocking and not auto-fixable; one
		// more QA pass before we give the topic a fresh draft.
		qa := agents.NewQA(agents.Deps{Q: q, LLM: s.LLM, Search: s.Search}, s.Log)
		_, err := qa.Requalify(ctx, projectID, claimed.ID)
		return err
	default:
		return s.regenerateOrEscalate(ctx, q, projectID, claimed)
	}
}

// regenerateOrEscalate discards a draft CiteLoop could not evaluate and asks the
// Writer for a fresh one, bounded per topic; once that budget is spent it
// becomes a human decision.
func (s *Scheduler) regenerateOrEscalate(ctx context.Context, q *db.Queries, projectID uuid.UUID, art db.Article) error {
	topic, err := q.GetTopicForProject(ctx, db.GetTopicForProjectParams{ID: art.TopicID, ProjectID: projectID})
	if err != nil {
		return err
	}
	if topic.RecoveryAttempts >= maxTopicRegenerations {
		return s.escalateArticle(ctx, q, projectID, art, "CiteLoop regenerated this topic several times but QA still could not evaluate the draft.")
	}
	// Regeneration needs an active profile; without one CiteLoop cannot write a
	// fresh draft, so hand it to a human instead of deleting content it can't replace.
	if _, err := q.GetActiveProfile(ctx, projectID); err != nil {
		return s.escalateArticle(ctx, q, projectID, art, "CiteLoop needs a confirmed Context profile before it can regenerate this draft.")
	}

	// Clear the topic's non-terminal drafts first: the (topic, kind, platform)
	// unique index counts rejected rows, so a reject-then-create would collide.
	if err := q.DeleteRecoverableArticlesForTopic(ctx, db.DeleteRecoverableArticlesForTopicParams{TopicID: topic.ID, ProjectID: projectID}); err != nil {
		return err
	}
	if _, err := q.IncrementTopicRecoveryAttempt(ctx, db.IncrementTopicRecoveryAttemptParams{ID: topic.ID, ProjectID: projectID}); err != nil {
		return err
	}

	// Move the topic back through backlog → generating so the Writer can produce
	// a clean draft, mirroring the daily generation pass.
	backlog, err := topicstate.Transition(topicstate.Status(topic.Status), topicstate.EventRejectDraft)
	if err != nil {
		return err
	}
	if _, err := q.UpdateTopicStatus(ctx, db.UpdateTopicStatusParams{ID: topic.ID, Status: string(backlog)}); err != nil {
		return err
	}
	generating, err := topicstate.Transition(backlog, topicstate.EventStartGeneration)
	if err != nil {
		return err
	}
	genTopic, err := q.UpdateTopicStatus(ctx, db.UpdateTopicStatusParams{ID: topic.ID, Status: string(generating)})
	if err != nil {
		return err
	}
	writer := agents.NewWriter(agents.Deps{Q: q, LLM: s.LLM, Search: s.Search}, s.Log)
	if _, err := writer.Generate(ctx, projectID, genTopic); err != nil {
		return err
	}
	s.Log.Info("review recovery regenerated draft", "project", projectID, "topic", topic.ID)
	return nil
}

func (s *Scheduler) escalateArticle(ctx context.Context, q *db.Queries, projectID uuid.UUID, art db.Article, reason string) error {
	options := []map[string]string{
		{"label": "Add or fix evidence in Context", "description": "Supply the source/evidence the claim needs, then CiteLoop re-checks automatically."},
		{"label": "Edit the draft", "description": "Adjust the wording yourself; saving re-runs QA."},
		{"label": "Reject", "description": "Discard the draft and return the topic to the backlog."},
	}
	_, err := q.EscalateArticleToHumanForProject(ctx, db.EscalateArticleToHumanForProjectParams{
		ID:                   art.ID,
		ProjectID:            projectID,
		RepairFailureReason:  ptr(reason),
		HumanDecisionOptions: mustJSON(options),
	})
	return err
}

// autoApproveReadyForProject approves drafts QA has cleared and schedules
// canonicals, so a hands-off project publishes without a manual click.
func (s *Scheduler) autoApproveReadyForProject(ctx context.Context, q *db.Queries, projectID uuid.UUID, cfg config.ProjectConfig) {
	ready, err := q.ListApprovableForProject(ctx, db.ListApprovableForProjectParams{ProjectID: projectID, Limit: reviewRecoveryLimit})
	if err != nil {
		s.Log.Warn("review recovery: list approvable failed", "project", projectID, "err", err)
		return
	}
	for _, art := range ready {
		schedAt := pgtype.Timestamptz{}
		if art.Kind == "canonical" {
			topic, err := q.GetTopicForProject(ctx, db.GetTopicForProjectParams{ID: art.TopicID, ProjectID: projectID})
			if err != nil {
				s.Log.Warn("review recovery: topic lookup for auto-approve failed", "project", projectID, "article", art.ID, "err", err)
				continue
			}
			schedAt = autoApprovalScheduleAt(topic.ScheduledAt, s.now())
		}
		approved, err := q.ApproveArticleForProject(ctx, db.ApproveArticleForProjectParams{
			ID:          art.ID,
			Status:      "approved",
			ScheduledAt: schedAt,
			ReviewedBy:  ptr(autoReviewer),
			ProjectID:   projectID,
		})
		if err != nil {
			s.Log.Warn("review recovery: auto-approve failed", "project", projectID, "article", art.ID, "err", err)
			continue
		}
		if _, err := q.EnqueueWorkflowEvent(ctx, db.EnqueueWorkflowEventParams{
			ProjectID:  projectID,
			EventType:  workflow.EventDraftApproved,
			DedupeKey:  workflowEventDedupeKey(workflow.EventDraftApproved, projectID, approved.ID.String()),
			Payload:    mustJSON(map[string]any{"article_id": approved.ID, "kind": approved.Kind, "auto": true}),
			EntityType: ptr("article"),
			EntityID:   pgtype.UUID{Bytes: approved.ID, Valid: true},
		}); err != nil {
			s.Log.Warn("review recovery: enqueue draft.approved failed", "project", projectID, "article", approved.ID, "err", err)
			continue
		}
		s.Log.Info("review recovery auto-approved draft", "project", projectID, "article", approved.ID, "kind", approved.Kind)
	}
}

// autoApprovalScheduleAt publishes immediately when the topic has no explicit
// date (auto-advance is already confirmed by the caller), otherwise honors the
// topic's scheduled date.
func autoApprovalScheduleAt(topicSchedule pgtype.Timestamptz, now time.Time) pgtype.Timestamptz {
	if topicSchedule.Valid {
		return topicSchedule
	}
	return pgutil.TS(now)
}

func maxDraftRepairAttempts() int32 { return 2 }

type recoveryQAView struct {
	Claims []struct {
		Mapped bool `json:"mapped"`
	} `json:"claims"`
	CanAutoFix           bool              `json:"can_auto_fix"`
	HumanDecisionOptions []json.RawMessage `json:"human_decision_options"`
}

func parseRecoveryQA(raw json.RawMessage) recoveryQAView {
	var v recoveryQAView
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &v)
	}
	return v
}

func articleHasClaimMap(art db.Article) bool {
	return len(parseRecoveryQA(art.QaFeedback).Claims) > 0
}

func articleCanAutoFix(art db.Article) bool {
	return art.QaBlocking && parseRecoveryQA(art.QaFeedback).CanAutoFix
}

// articleNeedsHuman is true only when QA actually evaluated the draft and found a
// real, non-auto-fixable decision (an unmapped product claim or model-provided
// options). Infra failures with no claim map are recoverable, not human work.
func articleNeedsHuman(art db.Article) bool {
	if !art.QaBlocking {
		return false
	}
	view := parseRecoveryQA(art.QaFeedback)
	if view.CanAutoFix {
		return false
	}
	for _, c := range view.Claims {
		if !c.Mapped {
			return true
		}
	}
	return len(view.HumanDecisionOptions) > 0
}
