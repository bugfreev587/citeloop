package scheduler

import (
	"context"
	"encoding/json"
	"strings"
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

// TickReviewRecovery drains the review queue without a human: for every project
// it pushes blocked-but-not-human drafts through re-QA → AI repair →
// regeneration, escalating to a genuine human decision only for non-editorial
// positioning choices or when the automated budget is spent. A QA-blocked draft
// is a correctness problem CiteLoop always resolves automatically; the
// auto-advance setting only decides whether the *cleared* draft is auto-approved
// or waits for a human to approve it (§5.5).
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
	q := db.New(s.Pool)

	// Cost breaker (§5.4): recovery makes LLM calls, so stop when the month's
	// spend has reached the budget.
	if spent, err := q.MonthlySpend(ctx, p.ID); err == nil && pgutil.Float(spent) >= cfg.MonthlyBudgetUSD {
		return nil
	}

	// Recovery runs for every project: a QA-blocked draft is a correctness
	// problem CiteLoop should always work through (re-QA → repair → regenerate →
	// escalate), so a draft never parks indefinitely under a "CiteLoop is
	// handling this" label that nothing is acting on.
	recoverable, err := q.ListRecoverableArticlesForProject(ctx, db.ListRecoverableArticlesForProjectParams{
		ProjectID: p.ID,
		Limit:     reviewRecoveryLimit,
	})
	if err != nil {
		return err
	}
	for _, art := range recoverable {
		if err := s.recoverArticle(ctx, q, p.ID, art, cfg); err != nil {
			s.Log.Warn("review recovery: article step failed", "project", p.ID, "article", art.ID, "err", err)
		}
	}

	// Auto-approval is the only step gated by auto-advance: hands-off projects
	// publish QA-cleared drafts without a click, while manual projects keep the
	// human as the approval gate (cleared drafts wait in "Ready to approve").
	if cfg.AutoAdvanceEnabled {
		s.autoApproveReadyForProject(ctx, q, p.ID, cfg)
	}
	return nil
}

// recoverArticle advances one blocked draft by exactly one automated step so the
// work is bounded and observable: each tick either re-runs QA, repairs, or
// regenerates, then lets the next tick re-evaluate the fresh state.
func (s *Scheduler) recoverArticle(ctx context.Context, q *db.Queries, projectID uuid.UUID, art db.Article, cfg config.ProjectConfig) error {
	// QA found a real non-editorial decision a human must make — hand it over.
	if articleNeedsHuman(art) {
		return s.escalateArticle(ctx, q, projectID, art, "QA found a positioning decision that needs human judgment.")
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
		updated, err := writer.RepairArticle(ctx, projectID, claimed.ID)
		if err != nil {
			return err
		}
		return s.approveRecoveredArticle(ctx, q, projectID, updated, cfg)
	case !articleHasClaimMap(claimed) && claimed.RecoveryAttempts <= maxRequalifyBeforeRegen:
		// QA never returned a claim map (infrastructure/parse failure, not a
		// content problem) — re-run QA; transient failures clear here.
		qa := agents.NewQA(agents.Deps{Q: q, LLM: s.LLM, Search: s.Search}, s.Log)
		updated, err := qa.Requalify(ctx, projectID, claimed.ID)
		if err != nil {
			return err
		}
		return s.approveRecoveredArticle(ctx, q, projectID, updated, cfg)
	case articleHasClaimMap(claimed):
		// QA evaluated but the draft is still blocking and not auto-fixable; one
		// more QA pass before we give the topic a fresh draft.
		qa := agents.NewQA(agents.Deps{Q: q, LLM: s.LLM, Search: s.Search}, s.Log)
		updated, err := qa.Requalify(ctx, projectID, claimed.ID)
		if err != nil {
			return err
		}
		return s.approveRecoveredArticle(ctx, q, projectID, updated, cfg)
	default:
		return s.regenerateOrEscalate(ctx, q, projectID, claimed, cfg)
	}
}

// regenerateOrEscalate discards a draft CiteLoop could not evaluate and asks the
// Writer for a fresh one, bounded per topic; once that budget is spent it
// becomes a human decision.
func (s *Scheduler) regenerateOrEscalate(ctx context.Context, q *db.Queries, projectID uuid.UUID, art db.Article, cfg config.ProjectConfig) error {
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
	created, err := writer.Generate(ctx, projectID, genTopic)
	if err != nil {
		return err
	}
	for _, draft := range created {
		if err := s.approveRecoveredArticle(ctx, q, projectID, draft, cfg); err != nil {
			return err
		}
	}
	s.Log.Info("review recovery regenerated draft", "project", projectID, "topic", topic.ID)
	return nil
}

func (s *Scheduler) escalateArticle(ctx context.Context, q *db.Queries, projectID uuid.UUID, art db.Article, reason string) error {
	// Prefer QA's own resolution options (each becomes a one-click fix in the
	// decision panel); fall back to the generic choices only when QA produced none.
	options := escalationOptions(art.QaFeedback)
	_, err := q.EscalateArticleToHumanForProject(ctx, db.EscalateArticleToHumanForProjectParams{
		ID:                   art.ID,
		ProjectID:            projectID,
		RepairFailureReason:  ptr(reason),
		HumanDecisionOptions: mustJSON(options),
	})
	return err
}

func shouldAutoApproveRecoveryResult(art db.Article) bool {
	return art.Status == "pending_review" && !art.QaBlocking && !art.RequiresHumanDecision
}

func (s *Scheduler) approveRecoveredArticle(ctx context.Context, q *db.Queries, projectID uuid.UUID, art db.Article, cfg config.ProjectConfig) error {
	if !shouldAutoApproveRecoveryResult(art) {
		return nil
	}
	schedAt := pgtype.Timestamptz{}
	if art.Kind == "canonical" {
		topic, err := q.GetTopicForProject(ctx, db.GetTopicForProjectParams{ID: art.TopicID, ProjectID: projectID})
		if err != nil {
			return err
		}
		if topic.ScheduledAt.Valid {
			schedAt = topic.ScheduledAt
		} else if latest, err := q.LatestCanonicalPublishSlotForProject(ctx, projectID); err == nil && latest.Valid {
			if when, ok := cfg.NextPublishSlot(latest.Time, s.now()); ok {
				schedAt = pgutil.TS(when)
			}
		} else if when, ok := cfg.NextPublishSlot(time.Time{}, s.now()); ok {
			schedAt = pgutil.TS(when)
		}
	}
	approved, err := q.ApproveArticleForProject(ctx, db.ApproveArticleForProjectParams{
		ID:          art.ID,
		Status:      "approved",
		ScheduledAt: schedAt,
		ReviewedBy:  ptr(autoReviewer),
		ProjectID:   projectID,
	})
	if err != nil {
		return err
	}
	if _, err := q.EnqueueWorkflowEvent(ctx, db.EnqueueWorkflowEventParams{
		ProjectID:  projectID,
		EventType:  workflow.EventDraftApproved,
		DedupeKey:  workflowEventDedupeKey(workflow.EventDraftApproved, projectID, approved.ID.String()),
		Payload:    mustJSON(map[string]any{"article_id": approved.ID, "kind": approved.Kind, "auto": true, "source": "qa_recovery"}),
		EntityType: ptr("article"),
		EntityID:   pgtype.UUID{Bytes: approved.ID, Valid: true},
	}); err != nil {
		return err
	}
	s.Log.Info("review recovery auto-approved recovered draft", "project", projectID, "article", approved.ID, "kind", approved.Kind)
	return nil
}

type decisionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

func escalationOptions(qaFeedback json.RawMessage) []decisionOption {
	var parsed struct {
		HumanDecisionOptions []decisionOption `json:"human_decision_options"`
	}
	if len(qaFeedback) > 0 {
		_ = json.Unmarshal(qaFeedback, &parsed)
	}
	var out []decisionOption
	for _, o := range parsed.HumanDecisionOptions {
		if asksForContextOrEvidence(o.Label) || asksForContextOrEvidence(o.Description) {
			continue
		}
		if strings.TrimSpace(o.Label) != "" || strings.TrimSpace(o.Description) != "" {
			out = append(out, o)
		}
	}
	if len(out) > 0 {
		return out
	}
	return []decisionOption{
		{Label: "Edit the draft", Description: "Adjust the wording yourself; saving re-runs QA."},
		{Label: "Reject", Description: "Discard the draft and return the topic to the backlog."},
	}
}

// autoApproveReadyForProject approves drafts QA has cleared and schedules
// canonicals, so a hands-off project publishes without a manual click.
func (s *Scheduler) autoApproveReadyForProject(ctx context.Context, q *db.Queries, projectID uuid.UUID, cfg config.ProjectConfig) {
	ready, err := q.ListApprovableForProject(ctx, db.ListApprovableForProjectParams{ProjectID: projectID, Limit: reviewRecoveryLimit})
	if err != nil {
		s.Log.Warn("review recovery: list approvable failed", "project", projectID, "err", err)
		return
	}
	// Seed the cadence from the latest slot already taken so a batch of approvals
	// publishes one every publish_interval_days instead of all at once.
	slot := time.Time{}
	if latest, err := q.LatestCanonicalPublishSlotForProject(ctx, projectID); err == nil && latest.Valid {
		slot = latest.Time
	}
	for _, art := range ready {
		schedAt := pgtype.Timestamptz{}
		if art.Kind == "canonical" {
			topic, err := q.GetTopicForProject(ctx, db.GetTopicForProjectParams{ID: art.TopicID, ProjectID: projectID})
			if err != nil {
				s.Log.Warn("review recovery: topic lookup for auto-approve failed", "project", projectID, "article", art.ID, "err", err)
				continue
			}
			if topic.ScheduledAt.Valid {
				schedAt = topic.ScheduledAt // operator-set date always wins
			} else if when, ok := cfg.NextPublishSlot(slot, s.now()); ok {
				schedAt = pgutil.TS(when)
			}
			// In manual mode schedAt stays null: the draft is approved but waits on
			// the Publish page until the operator publishes it.
			if schedAt.Valid && schedAt.Time.After(slot) {
				slot = schedAt.Time
			}
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

func maxDraftRepairAttempts() int32 { return 2 }

type recoveryQAView struct {
	Claims []struct {
		Mapped bool `json:"mapped"`
	} `json:"claims"`
	CanAutoFix           bool                          `json:"can_auto_fix"`
	HumanDecisionOptions []recoveryHumanDecisionOption `json:"human_decision_options"`
}

type recoveryHumanDecisionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
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

// articleNeedsHuman is true only when QA actually evaluated the draft and found
// a non-editorial positioning choice. Unsupported product claims stay in the
// automated editor/regeneration ladder; the user should not rewrite confirmed
// Product Context for one draft.
func articleNeedsHuman(art db.Article) bool {
	if !art.QaBlocking {
		return false
	}
	view := parseRecoveryQA(art.QaFeedback)
	if view.CanAutoFix {
		return false
	}
	hasUnmapped := false
	for _, c := range view.Claims {
		if !c.Mapped {
			hasUnmapped = true
			break
		}
	}
	if hasUnmapped {
		return false
	}
	for _, option := range view.HumanDecisionOptions {
		if !asksForContextOrEvidence(option.Label) && !asksForContextOrEvidence(option.Description) {
			return true
		}
	}
	return false
}

func asksForContextOrEvidence(s string) bool {
	normalized := strings.ToLower(strings.TrimSpace(s))
	if normalized == "" {
		return false
	}
	for _, token := range []string{"context", "evidence", "profile", "source"} {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return false
}
