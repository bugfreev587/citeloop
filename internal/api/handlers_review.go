package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/agents"
	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/platformcontract"
	"github.com/citeloop/citeloop/internal/topicstate"
	"github.com/citeloop/citeloop/internal/workflow"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const autoFixReviewer = "citeloop-auto"

// listReview returns pending_review articles grouped by topic so a canonical and
// its variants appear together (§5.5).
func (s *Server) listReview(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	arts, err := s.Q.ListPendingReview(r.Context(), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// group by topic
	groups := map[uuid.UUID][]db.Article{}
	order := []uuid.UUID{}
	for _, a := range arts {
		if _, ok := groups[a.TopicID]; !ok {
			order = append(order, a.TopicID)
		}
		groups[a.TopicID] = append(groups[a.TopicID], a)
	}
	out := make([]map[string]any, 0, len(order))
	for _, tid := range order {
		out = append(out, map[string]any{"topic_id": tid, "articles": groups[tid]})
	}
	writeJSON(w, 200, out)
}

func (s *Server) articleID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "articleID"))
}

func (s *Server) getProjectArticle(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	aid, err := s.articleID(r)
	if err != nil {
		writeErr(w, 400, "bad article id")
		return
	}
	a, err := s.Q.GetArticleForProject(r.Context(), db.GetArticleForProjectParams{ID: aid, ProjectID: projectID})
	if err != nil {
		writeErr(w, 404, "article not found")
		return
	}
	writeJSON(w, 200, a)
}

// repinProjectArticleTargetContext is the explicit recovery path for an
// expired or missing immutable Hashnode/Reddit context. It never mutates a
// published artifact and revalidates the body against the selected revision.
func (s *Server) repinProjectArticleTargetContext(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	articleID, err := s.articleID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad article id")
		return
	}
	var input struct {
		TargetContextID uuid.UUID `json:"target_context_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.TargetContextID == uuid.Nil {
		writeErr(w, http.StatusBadRequest, "target_context_id is required")
		return
	}
	article, err := s.Q.GetArticleForProject(r.Context(), db.GetArticleForProjectParams{ID: articleID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "article not found")
		return
	}
	if article.Status == "published" {
		writeErr(w, http.StatusConflict, "published artifacts cannot be re-pinned")
		return
	}
	if !article.PlatformContractID.Valid || article.PlatformContractVersion == nil {
		writeErr(w, http.StatusConflict, "article has no pinned platform contract")
		return
	}
	contract, err := s.Q.GetPlatformContentContractByID(r.Context(), db.GetPlatformContentContractByIDParams{ID: article.PlatformContractID.Bytes, Version: *article.PlatformContractVersion})
	if err != nil {
		writeErr(w, http.StatusConflict, "pinned platform contract is unavailable")
		return
	}
	contextRow, err := s.Q.GetPlatformTargetContextForProject(r.Context(), db.GetPlatformTargetContextForProjectParams{ID: input.TargetContextID, ProjectID: projectID})
	if err != nil || contextRow.Platform != contract.Platform || !contextRow.ExpiresAt.Valid || !platformcontract.TargetContextCurrent(contextRow.Status, contextRow.ExpiresAt.Time, time.Now().UTC()) {
		writeErr(w, http.StatusConflict, "a current target context for the article platform is required")
		return
	}
	metadata := map[string]any{}
	if len(article.PlatformMetadata) > 0 {
		_ = json.Unmarshal(article.PlatformMetadata, &metadata)
	}
	switch contract.Platform {
	case "hashnode":
		if existing, _ := metadata["publication"].(string); strings.TrimSpace(existing) != "" && strings.TrimSpace(existing) != contextRow.TargetKey {
			writeErr(w, http.StatusConflict, "changing the exact Hashnode publication requires regeneration")
			return
		}
		metadata["publication"] = contextRow.TargetKey
	case "reddit":
		if existing, _ := metadata["subreddit"].(string); strings.TrimSpace(existing) != "" && strings.TrimSpace(existing) != contextRow.TargetKey {
			writeErr(w, http.StatusConflict, "changing the exact subreddit requires regeneration")
			return
		}
		metadata["subreddit"] = contextRow.TargetKey
		if _, ok := metadata["post_type"]; !ok {
			metadata["post_type"] = article.OutputType
		}
		if contextRow.RequiredFlair != nil && strings.TrimSpace(*contextRow.RequiredFlair) != "" {
			metadata["flair"] = strings.TrimSpace(*contextRow.RequiredFlair)
		}
	default:
		writeErr(w, http.StatusConflict, "article platform does not use a target context")
		return
	}
	encoded, _ := json.Marshal(metadata)
	updated, err := s.Q.UpdateArticleTargetContextForProject(r.Context(), db.UpdateArticleTargetContextForProjectParams{
		TargetContextID: pgtype.UUID{Bytes: input.TargetContextID, Valid: true}, PlatformMetadata: encoded, ID: article.ID, ProjectID: projectID,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	updated, report, err := platformcontract.RevalidateArticle(r.Context(), s.Q, updated, time.Now().UTC())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"article": updated, "contract_validation": report})
}

func (s *Server) editProjectArticle(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	s.editArticleScoped(w, r, projectID)
}

// editArticle updates content/seo. When the content changes it re-runs QA so
// qa_blocking is recomputed from the actual (edited) text — the ONLY way to
// clear the blocking gate. There is no flag-flip shortcut: a reviewer must edit
// the unsupported claim out (or map it to evidence) for QA to unblock it (§5.5).
func (s *Server) editArticle(w http.ResponseWriter, r *http.Request) {
	s.editArticleScoped(w, r, uuid.Nil)
}

func (s *Server) editArticleScoped(w http.ResponseWriter, r *http.Request, projectID uuid.UUID) {
	aid, err := s.articleID(r)
	if err != nil {
		writeErr(w, 400, "bad article id")
		return
	}
	var in struct {
		ContentMd string          `json:"content_md"`
		SeoMeta   json.RawMessage `json:"seo_meta"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	var cur db.Article
	if projectID == uuid.Nil {
		cur, err = s.Q.GetArticle(r.Context(), aid)
	} else {
		cur, err = s.Q.GetArticleForProject(r.Context(), db.GetArticleForProjectParams{ID: aid, ProjectID: projectID})
	}
	if err != nil {
		writeErr(w, 404, "article not found")
		return
	}
	contentChanged := in.ContentMd != "" && in.ContentMd != cur.ContentMd
	if in.SeoMeta == nil {
		in.SeoMeta = cur.SeoMeta
	}
	if in.ContentMd == "" {
		in.ContentMd = cur.ContentMd
	}
	var updated db.Article
	if projectID == uuid.Nil {
		updated, err = s.Q.UpdateArticleContent(r.Context(), db.UpdateArticleContentParams{
			ID: aid, ContentMd: in.ContentMd, SeoMeta: in.SeoMeta,
		})
	} else {
		updated, err = s.Q.UpdateArticleContentForProject(r.Context(), db.UpdateArticleContentForProjectParams{
			ID: aid, ContentMd: in.ContentMd, SeoMeta: in.SeoMeta, ProjectID: projectID,
		})
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// Re-qualify on content change so the gate reflects the edited text. Without
	// a content change QA cannot have changed, so we skip the LLM call.
	if contentChanged {
		qaAgent := agents.NewQA(agents.Deps{Q: s.Q, LLM: s.LLM, Search: s.Search, AICalls: s.AICalls}, s.Log)
		requalified, qerr := qaAgent.Requalify(r.Context(), cur.ProjectID, aid)
		if qerr != nil {
			writeErr(w, 500, "re-qa failed: "+qerr.Error())
			return
		}
		updated = requalified
	}
	updated, _, err = platformcontract.RevalidateArticle(r.Context(), s.Q, updated, time.Now().UTC())
	if err != nil {
		writeErr(w, 500, "platform contract validation failed: "+err.Error())
		return
	}
	writeJSON(w, 200, updated)
}

func (s *Server) fixProjectArticle(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	aid, err := s.articleID(r)
	if err != nil {
		writeErr(w, 400, "bad article id")
		return
	}
	writer := agents.NewWriter(agents.Deps{Q: s.Q, LLM: s.LLM, Search: s.Search, AICalls: s.AICalls, ArticleAssets: s.ArticleAssets}, s.Log)
	updated, err := writer.RepairArticle(r.Context(), projectID, aid)
	if err != nil {
		writeErr(w, 500, "ai fix failed: "+err.Error())
		return
	}
	writeJSON(w, 200, updated)
}

// recheckProjectArticle re-runs QA on the draft's current content. It's the
// operator's recovery for a draft that was blocked by a QA infrastructure
// failure (e.g. a truncated model response) rather than a real content problem.
func (s *Server) recheckProjectArticle(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	aid, err := s.articleID(r)
	if err != nil {
		writeErr(w, 400, "bad article id")
		return
	}
	qa := agents.NewQA(agents.Deps{Q: s.Q, LLM: s.LLM, Search: s.Search, AICalls: s.AICalls}, s.Log)
	updated, err := qa.Requalify(r.Context(), projectID, aid)
	if err != nil {
		writeErr(w, 500, "qa re-check failed: "+err.Error())
		return
	}
	writeJSON(w, 200, updated)
}

// applyFixProjectArticle applies a specific human-chosen resolution from the
// Review decision panel — the AI rewrites the draft per that instruction and QA
// re-runs, so the operator resolves a block with one click instead of editing.
func (s *Server) applyFixProjectArticle(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	aid, err := s.articleID(r)
	if err != nil {
		writeErr(w, 400, "bad article id")
		return
	}
	var in struct {
		Instruction string `json:"instruction"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if strings.TrimSpace(in.Instruction) == "" {
		writeErr(w, 400, "instruction required")
		return
	}
	writer := agents.NewWriter(agents.Deps{Q: s.Q, LLM: s.LLM, Search: s.Search, AICalls: s.AICalls, ArticleAssets: s.ArticleAssets}, s.Log)
	updated, err := writer.RepairArticleWithInstruction(r.Context(), projectID, aid, in.Instruction)
	if err != nil {
		writeErr(w, 500, "apply fix failed: "+err.Error())
		return
	}
	if updated.QaBlocking {
		writeErr(w, 500, "apply fix did not clear QA")
		return
	}
	approved, err := s.approveArticleRecord(r.Context(), updated, projectID, autoFixReviewer)
	if err != nil {
		writeErr(w, 500, "apply fix approve failed: "+err.Error())
		return
	}
	writeJSON(w, 200, approved)
}

func (s *Server) approveProjectArticle(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	s.approveArticleScoped(w, r, projectID)
}

// approveArticle is the human gate. canonical → approved + scheduled_at written
// (inherit topic.scheduled_at or follow the project's auto-advance scheduling).
// variant → approved (waits for canonical publish). Blocking articles cannot be
// approved (§5.5).
func (s *Server) approveArticle(w http.ResponseWriter, r *http.Request) {
	s.approveArticleScoped(w, r, uuid.Nil)
}

func (s *Server) approveArticleScoped(w http.ResponseWriter, r *http.Request, projectID uuid.UUID) {
	aid, err := s.articleID(r)
	if err != nil {
		writeErr(w, 400, "bad article id")
		return
	}
	var in struct {
		ReviewedBy string `json:"reviewed_by"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.ReviewedBy == "" {
		in.ReviewedBy = "reviewer"
	}
	var a db.Article
	if projectID == uuid.Nil {
		a, err = s.Q.GetArticle(r.Context(), aid)
	} else {
		a, err = s.Q.GetArticleForProject(r.Context(), db.GetArticleForProjectParams{ID: aid, ProjectID: projectID})
	}
	if err != nil {
		writeErr(w, 404, "article not found")
		return
	}
	if a.QaBlocking {
		writeErr(w, 409, "article has unresolved qa_blocking issues; resolve before approving")
		return
	}
	a, _, err = platformcontract.RevalidateArticle(r.Context(), s.Q, a, time.Now().UTC())
	if err != nil {
		writeErr(w, 500, "platform contract validation failed: "+err.Error())
		return
	}
	if contractValidationBlocks(a.ContractValidation) {
		writeErr(w, http.StatusConflict, "article has unresolved platform contract violations; resolve before approving")
		return
	}

	updated, err := s.approveArticleRecord(r.Context(), a, projectID, in.ReviewedBy)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, updated)
}

func contractValidationBlocks(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var report struct {
		Passed *bool `json:"passed"`
	}
	if json.Unmarshal(raw, &report) != nil || report.Passed == nil {
		return true
	}
	return !*report.Passed
}

func (s *Server) approveArticleRecord(ctx context.Context, a db.Article, projectID uuid.UUID, reviewedBy string) (db.Article, error) {
	validated, report, err := platformcontract.RevalidateArticle(ctx, s.Q, a, time.Now().UTC())
	if err != nil {
		return db.Article{}, err
	}
	if !report.Passed {
		return db.Article{}, errors.New("article has unresolved platform contract violations")
	}
	a = validated
	schedAt := pgtype.Timestamptz{}
	if a.Kind == "canonical" {
		// scheduled_at single source of truth (§3): inherit topic or follow
		// the project's auto-advance scheduling policy.
		var topic db.Topic
		var err error
		if projectID == uuid.Nil {
			topic, err = s.Q.GetTopic(ctx, a.TopicID)
		} else {
			topic, err = s.Q.GetTopicForProject(ctx, db.GetTopicForProjectParams{ID: a.TopicID, ProjectID: projectID})
		}
		if err != nil {
			return db.Article{}, err
		}
		cfg, err := s.projectConfigByID(ctx, a.ProjectID)
		if err != nil {
			return db.Article{}, err
		}
		latest, _ := s.Q.LatestCanonicalPublishSlotForProject(ctx, a.ProjectID)
		schedAt = canonicalApprovalScheduleAt(topic.ScheduledAt, latest, cfg, time.Now())
	}

	var updated db.Article
	if projectID == uuid.Nil {
		updated, err = s.Q.ApproveArticle(ctx, db.ApproveArticleParams{
			ID:          a.ID,
			Status:      "approved",
			ScheduledAt: schedAt,
			ReviewedBy:  &reviewedBy,
		})
	} else {
		updated, err = s.Q.ApproveArticleForProject(ctx, db.ApproveArticleForProjectParams{
			ID:          a.ID,
			Status:      "approved",
			ScheduledAt: schedAt,
			ReviewedBy:  &reviewedBy,
			ProjectID:   projectID,
		})
	}
	if err != nil {
		return db.Article{}, err
	}
	if err := s.enqueueWorkflowEvent(ctx, updated.ProjectID, workflow.EventDraftApproved, "article", updated.ID, workflowDedupeKey(workflow.EventDraftApproved, updated.ProjectID, updated.ID), map[string]any{
		"article_id": updated.ID,
		"kind":       updated.Kind,
		"status":     updated.Status,
	}); err != nil {
		return db.Article{}, err
	}
	return updated, nil
}

// canonicalApprovalScheduleAt decides a freshly approved canonical's publish
// slot. An operator-set topic date always wins; otherwise the project's publish
// cadence staggers it after the latest slot already taken (manual mode leaves it
// unscheduled to wait on the Publish page).
func canonicalApprovalScheduleAt(topicSchedule, latest pgtype.Timestamptz, cfg config.ProjectConfig, now time.Time) pgtype.Timestamptz {
	if topicSchedule.Valid {
		return topicSchedule
	}
	latestTime := time.Time{}
	if latest.Valid {
		latestTime = latest.Time
	}
	if when, ok := cfg.NextPublishSlot(latestTime, now); ok {
		return pgutil.TS(when)
	}
	return pgtype.Timestamptz{}
}

func (s *Server) rejectProjectArticle(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	s.rejectArticleScoped(w, r, projectID)
}

func (s *Server) rejectArticle(w http.ResponseWriter, r *http.Request) {
	s.rejectArticleScoped(w, r, uuid.Nil)
}

func (s *Server) rejectArticleScoped(w http.ResponseWriter, r *http.Request, projectID uuid.UUID) {
	aid, err := s.articleID(r)
	if err != nil {
		writeErr(w, 400, "bad article id")
		return
	}
	var in struct {
		ReviewedBy string `json:"reviewed_by"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.ReviewedBy == "" {
		in.ReviewedBy = "reviewer"
	}
	var a db.Article
	if projectID == uuid.Nil {
		a, err = s.Q.RejectArticle(r.Context(), db.RejectArticleParams{ID: aid, ReviewedBy: &in.ReviewedBy})
	} else {
		a, err = s.Q.RejectArticleForProject(r.Context(), db.RejectArticleForProjectParams{
			ID: aid, ReviewedBy: &in.ReviewedBy, ProjectID: projectID,
		})
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// topic back to backlog so it can be re-picked (§5.5).
	if topic, err := s.Q.GetTopic(r.Context(), a.TopicID); err == nil {
		if nextStatus, err := topicstate.Transition(topicstate.Status(topic.Status), topicstate.EventRejectDraft); err == nil {
			_, _ = s.Q.UpdateTopicStatus(r.Context(), db.UpdateTopicStatusParams{ID: a.TopicID, Status: string(nextStatus)})
		}
	}
	writeJSON(w, 200, a)
}

// publishProjectArticleNow is the operator's "Publish now" override on the
// Publish page: it brings an approved canonical's slot to now so the next
// publish tick sends it out (also how manual-mode drafts get published).
func (s *Server) publishProjectArticleNow(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	aid, err := s.articleID(r)
	if err != nil {
		writeErr(w, 400, "bad article id")
		return
	}
	a, err := s.Q.PublishArticleNowForProject(r.Context(), db.PublishArticleNowForProjectParams{ID: aid, ProjectID: projectID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, 409, "only an approved canonical draft can be published now")
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	if s.Sched != nil {
		s.Sched.TickPublish(r.Context())
	}
	writeJSON(w, 200, a)
}

func (s *Server) markProjectDistributed(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	s.markDistributedScoped(w, r, projectID)
}

func (s *Server) markDistributed(w http.ResponseWriter, r *http.Request) {
	s.markDistributedScoped(w, r, uuid.Nil)
}

func (s *Server) markDistributedScoped(w http.ResponseWriter, r *http.Request, projectID uuid.UUID) {
	aid, err := s.articleID(r)
	if err != nil {
		writeErr(w, 400, "bad article id")
		return
	}
	var a db.Article
	if projectID == uuid.Nil {
		a, err = s.Q.MarkDistributed(r.Context(), aid)
	} else {
		a, err = s.Q.MarkDistributedForProject(r.Context(), db.MarkDistributedForProjectParams{ID: aid, ProjectID: projectID})
	}
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, a)
}

func (s *Server) retryProjectPublish(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	aid, err := s.articleID(r)
	if err != nil {
		writeErr(w, 400, "bad article id")
		return
	}
	a, err := s.Q.RetryPublishArticle(r.Context(), db.RetryPublishArticleParams{ID: aid, ProjectID: projectID})
	if err != nil {
		writeErr(w, 404, "publish_failed article not found")
		return
	}
	if s.Sched != nil {
		s.Sched.TickPublish(r.Context())
	}
	writeJSON(w, 200, a)
}
