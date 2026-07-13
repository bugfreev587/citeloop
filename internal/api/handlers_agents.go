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
	"github.com/citeloop/citeloop/internal/platform"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/citeloop/citeloop/internal/topicstate"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) projectConfig(r *http.Request, id uuid.UUID) (config.ProjectConfig, error) {
	p, err := s.Q.GetProject(r.Context(), id)
	if err != nil {
		return config.ProjectConfig{}, err
	}
	return config.Parse(p.Config)
}

// runInsight builds the product profile from the landing page, then continues
// the slower public crawl and inventory pass in the background.
func (s *Server) runInsight(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	var in struct {
		LandingURL string `json:"landing_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.LandingURL == "" {
		writeErr(w, 400, "landing_url required")
		return
	}
	cfg, err := s.projectConfig(r, id)
	if err != nil {
		writeErr(w, 404, "project not found")
		return
	}
	landingURL, err := configuredContextLanding(cfg.SiteURL, in.LandingURL)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "landing_url must match configured domain")
		return
	}
	ag := agents.NewInsight(agents.Deps{Q: s.Q, LLM: s.LLM, Search: s.Search, AICalls: s.AICalls}, s.Log)
	profile, summary, err := ag.RunQuickProfile(r.Context(), id, landingURL, cfg.Crawl)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.startInsightInventoryCrawl(id, landingURL, cfg.Crawl)
	writeJSON(w, 200, map[string]any{
		"profile":          profile,
		"inventory_count":  0,
		"crawl_summary":    summary,
		"background_crawl": true,
	})
}

func (s *Server) listTopics(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	topics, err := s.Q.ListTopics(r.Context(), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, emptySlice(topics))
}

func (s *Server) topicID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "topicID"))
}

func nullableTopicText(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func validTopicChannel(value string) bool {
	return value == "blog" || value == "syndication" || value == "both"
}

// updateTopic allows the single operator to correct Strategist output before
// generation while keeping every mutation scoped to the project route.
func (s *Server) updateTopic(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	topicID, err := s.topicID(r)
	if err != nil {
		writeErr(w, 400, "bad topic id")
		return
	}
	var in struct {
		Channel       *string          `json:"channel"`
		Title         *string          `json:"title"`
		TargetKeyword *string          `json:"target_keyword"`
		TargetPrompt  *string          `json:"target_prompt"`
		Angle         *string          `json:"angle"`
		Format        *string          `json:"format"`
		Priority      *int             `json:"priority"`
		InternalLinks *json.RawMessage `json:"internal_links"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}

	cur, err := s.Q.GetTopicForProject(r.Context(), db.GetTopicForProjectParams{ID: topicID, ProjectID: projectID})
	if err != nil {
		writeErr(w, 404, "topic not found")
		return
	}
	params := db.UpdateTopicParams{
		ID:                    topicID,
		ProjectID:             projectID,
		Channel:               cur.Channel,
		Title:                 cur.Title,
		TargetKeyword:         cur.TargetKeyword,
		TargetPrompt:          cur.TargetPrompt,
		Angle:                 cur.Angle,
		Format:                cur.Format,
		Priority:              cur.Priority,
		InternalLinks:         cur.InternalLinks,
		Status:                cur.Status,
		ScheduledAt:           cur.ScheduledAt,
		SourceContentActionID: cur.SourceContentActionID,
	}
	if in.Channel != nil {
		channel := strings.TrimSpace(*in.Channel)
		if !validTopicChannel(channel) {
			writeErr(w, 400, "invalid channel")
			return
		}
		params.Channel = channel
	}
	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if title == "" {
			writeErr(w, 400, "title required")
			return
		}
		params.Title = title
	}
	if in.TargetKeyword != nil {
		params.TargetKeyword = nullableTopicText(*in.TargetKeyword)
	}
	if in.TargetPrompt != nil {
		params.TargetPrompt = nullableTopicText(*in.TargetPrompt)
	}
	if in.Angle != nil {
		params.Angle = nullableTopicText(*in.Angle)
	}
	if in.Format != nil {
		params.Format = nullableTopicText(*in.Format)
	}
	if in.Priority != nil {
		params.Priority = int32(*in.Priority)
	}
	if in.InternalLinks != nil {
		if len(*in.InternalLinks) == 0 || !json.Valid(*in.InternalLinks) {
			writeErr(w, 400, "invalid internal_links")
			return
		}
		params.InternalLinks = *in.InternalLinks
	}

	updated, err := s.Q.UpdateTopic(r.Context(), params)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, updated)
}

func parseTopicSchedule(value *string) (pgtype.Timestamptz, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return pgtype.Timestamptz{}, nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(*value))
	if err != nil {
		return pgtype.Timestamptz{}, err
	}
	return pgutil.TS(t), nil
}

func (s *Server) scheduleTopic(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	topicID, err := s.topicID(r)
	if err != nil {
		writeErr(w, 400, "bad topic id")
		return
	}
	var in struct {
		ScheduledAt *string `json:"scheduled_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	scheduledAt, err := parseTopicSchedule(in.ScheduledAt)
	if err != nil {
		writeErr(w, 400, "scheduled_at must be RFC3339")
		return
	}
	current, err := s.Q.GetTopicForProject(r.Context(), db.GetTopicForProjectParams{ID: topicID, ProjectID: projectID})
	if err != nil {
		writeErr(w, 404, "topic not found")
		return
	}
	event := topicstate.EventClearSchedule
	if scheduledAt.Valid {
		event = topicstate.EventSchedule
	}
	if _, err := topicstate.Transition(topicstate.Status(current.Status), event); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	updated, err := s.Q.SetTopicScheduledAtForProject(r.Context(), db.SetTopicScheduledAtForProjectParams{
		ID:          topicID,
		ProjectID:   projectID,
		ScheduledAt: scheduledAt,
	})
	if err != nil {
		writeErr(w, 404, "topic not found")
		return
	}
	writeJSON(w, 200, updated)
}

func (s *Server) archiveTopic(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	topicID, err := s.topicID(r)
	if err != nil {
		writeErr(w, 400, "bad topic id")
		return
	}
	current, err := s.Q.GetTopicForProject(r.Context(), db.GetTopicForProjectParams{ID: topicID, ProjectID: projectID})
	if err != nil {
		writeErr(w, 404, "topic not found")
		return
	}
	if _, err := topicstate.Transition(topicstate.Status(current.Status), topicstate.EventArchive); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	updated, err := s.Q.ArchiveTopicForProject(r.Context(), db.ArchiveTopicForProjectParams{ID: topicID, ProjectID: projectID})
	if err != nil {
		writeErr(w, 404, "topic not found")
		return
	}
	writeJSON(w, 200, updated)
}

// generateTopic accepts Writer+QA work for a single topic on demand (§5.3).
func (s *Server) generateTopic(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	topicID, err := s.topicID(r)
	if err != nil {
		writeErr(w, 400, "bad topic id")
		return
	}
	topic, err := s.Q.GetTopicForProject(r.Context(), db.GetTopicForProjectParams{ID: topicID, ProjectID: id})
	if err != nil {
		writeErr(w, 404, "topic not found")
		return
	}
	if topic.Status == string(topicstate.StatusArchived) {
		writeErr(w, http.StatusConflict, "topic is archived")
		return
	}
	existing, err := s.Q.ListArticlesByTopicForProject(r.Context(), db.ListArticlesByTopicForProjectParams{TopicID: topicID, ProjectID: id})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	nonRejected := make([]db.Article, 0, len(existing))
	for _, article := range existing {
		if article.Status != "rejected" {
			nonRejected = append(nonRejected, article)
		}
	}
	if len(nonRejected) > 0 {
		topic, err = s.reconcileDraftedTopicStatus(r.Context(), s.Q, topic)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "topic": topic, "articles": emptySlice(nonRejected)})
		return
	}
	if topic.Status == string(topicstate.StatusGenerating) {
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "generating", "topic": topic, "articles": []db.Article{}})
		return
	}
	if _, err := topicstate.Transition(topicstate.Status(topic.Status), topicstate.EventStartGeneration); err != nil {
		writeErr(w, http.StatusConflict, err.Error())
		return
	}
	if s.Pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "database pool unavailable")
		return
	}
	started, err := s.Q.StartTopicGenerationForProject(r.Context(), db.StartTopicGenerationForProjectParams{ID: topicID, ProjectID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			current, getErr := s.Q.GetTopicForProject(r.Context(), db.GetTopicForProjectParams{ID: topicID, ProjectID: id})
			if getErr != nil {
				writeErr(w, 404, "topic not found")
				return
			}
			if current.Status != string(topicstate.StatusGenerating) {
				writeErr(w, http.StatusConflict, "topic is not ready for generation")
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]any{"status": "generating", "topic": current, "articles": []db.Article{}})
			return
		}
		writeErr(w, 500, err.Error())
		return
	}
	s.startTopicGeneration(id, topicID)
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "generating", "topic": started, "articles": []db.Article{}})
}

func (s *Server) startTopicGeneration(projectID, topicID uuid.UUID) {
	go s.generateTopicInBackground(projectID, topicID)
}

func (s *Server) generateTopicInBackground(projectID, topicID uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	q := db.New(s.Pool)
	topic, err := q.GetTopicForProject(ctx, db.GetTopicForProjectParams{ID: topicID, ProjectID: projectID})
	if err != nil {
		s.Log.Error("manual generation topic lookup failed", "project", projectID, "topic", topicID, "err", err)
		return
	}
	sourceActionID := uuid.UUID{}
	if topic.SourceContentActionID.Valid {
		sourceActionID = uuid.UUID(topic.SourceContentActionID.Bytes)
		if _, err := q.UpdateContentActionStatus(ctx, db.UpdateContentActionStatusParams{
			ID:        sourceActionID,
			ProjectID: projectID,
			Status:    "drafting",
		}); err != nil {
			s.Log.Warn("mark manual content action drafting failed", "project", projectID, "topic", topicID, "action", sourceActionID, "err", err)
		}
	}
	n, err := q.CountNonRejectedArticlesForTopic(ctx, topicID)
	if err != nil {
		s.Log.Error("manual generation article check failed", "project", projectID, "topic", topicID, "err", err)
		s.resetTopicAfterGenerationFailure(ctx, q, topic, sourceActionID)
		return
	}
	if n > 0 {
		s.markSourceContentActionExistingDraft(ctx, q, projectID, topicID, sourceActionID)
		if _, err := s.reconcileDraftedTopicStatus(ctx, q, topic); err != nil {
			s.Log.Warn("manual generation topic draft reconciliation failed", "project", projectID, "topic", topicID, "err", err)
		}
		return
	}

	writer := agents.NewWriter(agents.Deps{Q: q, LLM: s.LLM, Search: s.Search, AICalls: s.AICalls}, s.Log)
	articles, err := writer.Generate(agents.WithAICallRetry(ctx), projectID, topic)
	if err != nil {
		s.Log.Error("manual generation failed", "project", projectID, "topic", topicID, "err", err)
		if n, countErr := q.CountNonRejectedArticlesForTopic(ctx, topicID); countErr == nil {
			if n == 0 {
				s.resetTopicAfterGenerationFailure(ctx, q, topic, sourceActionID)
			} else {
				s.markSourceContentActionExistingDraft(ctx, q, projectID, topicID, sourceActionID)
			}
		}
		return
	}
	s.markSourceContentActionDraftReady(ctx, q, projectID, topicID, sourceActionID, articles)
	s.Log.Info("manual generation accepted into review queue", "project", projectID, "topic", topic.Title)
}

func (s *Server) markSourceContentActionExistingDraft(ctx context.Context, q *db.Queries, projectID, topicID, sourceActionID uuid.UUID) {
	if sourceActionID != uuid.Nil {
		articles, err := q.ListArticlesByTopicForProject(ctx, db.ListArticlesByTopicForProjectParams{TopicID: topicID, ProjectID: projectID})
		if err != nil {
			s.Log.Warn("manual generation existing article lookup failed", "project", projectID, "topic", topicID, "action", sourceActionID, "err", err)
			return
		}
		s.markSourceContentActionDraftReady(ctx, q, projectID, topicID, sourceActionID, articles)
	}
}

func (s *Server) markSourceContentActionDraftReady(ctx context.Context, q *db.Queries, projectID, topicID, sourceActionID uuid.UUID, articles []db.Article) {
	if sourceActionID == uuid.Nil {
		return
	}
	if canonicalID := canonicalArticleID(articles); canonicalID != uuid.Nil {
		if _, err := q.MarkContentActionDraftReady(ctx, db.MarkContentActionDraftReadyParams{
			ID:             sourceActionID,
			ProjectID:      projectID,
			DraftArticleID: pgtype.UUID{Bytes: canonicalID, Valid: true},
		}); err != nil {
			s.Log.Warn("mark manual content action draft ready failed", "project", projectID, "topic", topicID, "action", sourceActionID, "article", canonicalID, "err", err)
		}
	}
}

func canonicalArticleID(articles []db.Article) uuid.UUID {
	for _, article := range articles {
		if article.Kind == "canonical" {
			return article.ID
		}
	}
	if len(articles) > 0 {
		return articles[0].ID
	}
	return uuid.Nil
}

func (s *Server) reconcileDraftedTopicStatus(ctx context.Context, q *db.Queries, topic db.Topic) (db.Topic, error) {
	next, changed, err := topicstate.ReconcileExistingDrafts(topicstate.Status(topic.Status))
	if err != nil {
		return db.Topic{}, err
	}
	if changed {
		return q.UpdateTopicStatusForProject(ctx, db.UpdateTopicStatusForProjectParams{
			ID:        topic.ID,
			ProjectID: topic.ProjectID,
			Status:    string(next),
		})
	}
	return topic, nil
}

func (s *Server) resetTopicAfterGenerationFailure(ctx context.Context, q *db.Queries, topic db.Topic, sourceActionID uuid.UUID) {
	status, err := topicstate.GenerationFailureStatus(topicstate.Status(topic.Status), topic.ScheduledAt.Valid)
	if err != nil {
		s.Log.Warn("generation failure topic state transition rejected", "project", topic.ProjectID, "topic", topic.ID, "status", topic.Status, "err", err)
		return
	}
	if _, err := q.UpdateTopicStatusForProject(ctx, db.UpdateTopicStatusForProjectParams{
		ID:        topic.ID,
		ProjectID: topic.ProjectID,
		Status:    string(status),
	}); err != nil {
		s.Log.Warn("reset topic after generation failure failed", "project", topic.ProjectID, "topic", topic.ID, "err", err)
	}
	if sourceActionID != uuid.Nil {
		if _, err := q.UpdateContentActionStatus(ctx, db.UpdateContentActionStatusParams{
			ID:        sourceActionID,
			ProjectID: topic.ProjectID,
			Status:    "approved",
		}); err != nil {
			s.Log.Warn("reset manual content action after generation failure failed", "project", topic.ProjectID, "topic", topic.ID, "action", sourceActionID, "err", err)
		}
	}
}

func (s *Server) tickGenerate(w http.ResponseWriter, r *http.Request) {
	s.Sched.TickGenerate(r.Context())
	writeJSON(w, 200, map[string]string{"status": "generate tick complete"})
}

func (s *Server) tickPublish(w http.ResponseWriter, r *http.Request) {
	s.Sched.TickPublish(r.Context())
	writeJSON(w, 200, map[string]string{"status": "publish tick complete"})
}

func (s *Server) reconcilePublishing(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	project, err := s.Q.GetProject(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "project not found")
		return
	}
	if s.Sched == nil {
		writeErr(w, 503, "scheduler unavailable")
		return
	}
	if err := s.Sched.ReconcilePublishProject(r.Context(), project); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "reconcile complete"})
}

// listDistribute returns ready_to_distribute variants enriched with the target
// platform's compose URL so the UI can offer "copy variant + open compose page"
// (PRD §5.6).
func (s *Server) listDistribute(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	arts, err := s.Q.ListArticlesByStatus(r.Context(), db.ListArticlesByStatusParams{ProjectID: id, Status: "ready_to_distribute"})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(arts))
	for _, a := range arts {
		plat := ""
		if a.Platform != nil {
			plat = *a.Platform
		}
		out = append(out, map[string]any{
			"article":            a,
			"compose_url":        publisher.ComposeURL(plat),
			"supports_canonical": platform.SupportsCanonical(platform.Platform(plat)),
		})
	}
	writeJSON(w, 200, out)
}

func (s *Server) listArticles(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "published"
	}
	arts, err := s.Q.ListArticlesByStatus(r.Context(), db.ListArticlesByStatusParams{ProjectID: id, Status: status})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, emptySlice(arts))
}
