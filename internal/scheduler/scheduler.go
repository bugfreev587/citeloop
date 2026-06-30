// Package scheduler is the automatic operations core (PRD §5.4). A frequent
// generation tick reserves project topics in a short advisory-lock transaction,
// enforces the monthly cost breaker, then generates outside that transaction.
// A separate publish tick auto-publishes due canonicals and unlocks
// distributable variants (§5.6).
package scheduler

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/agents"
	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/contextmeta"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/geo"
	"github.com/citeloop/citeloop/internal/githubapp"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/notification"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/citeloop/citeloop/internal/search"
	"github.com/citeloop/citeloop/internal/secretbox"
	seopkg "github.com/citeloop/citeloop/internal/seo"
	"github.com/citeloop/citeloop/internal/topicstate"
	"github.com/citeloop/citeloop/internal/workflow"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	reviewOverdueHours = 48
	reviewOverdueLimit = 100
)

type seoRunner interface {
	Sync(context.Context, uuid.UUID, string) (seopkg.SyncResult, error)
	Analyze(context.Context, uuid.UUID) (seopkg.SyncResult, error)
	Brief(context.Context, uuid.UUID) (seopkg.Brief, error)
}

type Scheduler struct {
	Pool                    *pgxpool.Pool
	LLM                     llm.Provider
	Search                  search.Provider
	Blog                    *publisher.BlogPublisher
	SEOData                 seopkg.GoogleDataProvider
	GEOAnswerProvider       geo.AnswerProvider
	GEOProviderRunBudgetUSD float64
	BlogBaseURL             string
	Log                     *slog.Logger
	now                     func() time.Time
	alert                   func(projectID uuid.UUID, msg string)
	httpClient              *http.Client
	seoRunnerFactory        func(q *db.Queries) seoRunner
	NotificationSecret      string
	UniPostDeployHookURL    string
	GitHubApp               *githubapp.Service
}

type publisherConnectionQuerier interface {
	GetEnabledPublisherConnectionForProject(context.Context, db.GetEnabledPublisherConnectionForProjectParams) (db.PublisherConnection, error)
	GetActivePublisherCredential(context.Context, db.GetActivePublisherCredentialParams) (db.PublisherCredential, error)
}

func New(pool *pgxpool.Pool, llmP llm.Provider, searchP search.Provider, blog *publisher.BlogPublisher, log *slog.Logger) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{
		Pool: pool, LLM: llmP, Search: searchP, Blog: blog, Log: log,
		now:        time.Now,
		alert:      func(p uuid.UUID, m string) { log.Warn("ALERT", "project", p, "msg", m) },
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *Scheduler) TickNotifications(ctx context.Context) {
	if s.NotificationSecret == "" {
		s.Log.Warn("NOTIFICATION_SECRET_KEY not set; notification worker skipped")
		return
	}
	worker := notification.Worker{
		Store:  db.New(s.Pool),
		Sender: notification.HTTPSender{Client: s.httpClient},
		Secret: s.NotificationSecret,
		Limit:  20,
	}
	processed, err := worker.ProcessOnce(ctx)
	if err != nil {
		s.Log.Error("notification worker failed", "err", err)
		return
	}
	if processed > 0 {
		s.Log.Info("notification deliveries processed", "count", processed)
	}
}

func (s *Scheduler) TickWorkflow(ctx context.Context) {
	worker := workflow.Worker{
		Store:  db.New(s.Pool),
		Handle: s.handleWorkflowEvent,
		Limit:  20,
	}
	processed, err := worker.ProcessOnce(ctx)
	if err != nil {
		s.Log.Error("workflow worker failed", "err", err)
		return
	}
	if processed > 0 {
		s.Log.Info("workflow events processed", "count", processed)
	}
}

func (s *Scheduler) handleWorkflowEvent(ctx context.Context, event db.WorkflowEvent) error {
	switch event.EventType {
	case workflow.EventOpportunityReviewed:
		return s.handleOpportunityReviewed(ctx, event.ProjectID)
	case workflow.EventOpportunityBatchDone:
		return s.handleOpportunityBatchCompleted(ctx, event.ProjectID)
	case workflow.EventContentPlanCreated:
		return s.handleContentPlanCreated(ctx, event.ProjectID)
	case workflow.EventDraftApproved:
		return s.handleDraftApproved(ctx, event.ProjectID)
	case workflow.EventMeasurementWindowDue:
		return s.handleMeasurementWindowDue(ctx, event.ProjectID)
	default:
		s.Log.Info("workflow event ignored", "type", event.EventType, "project", event.ProjectID)
		return nil
	}
}

func (s *Scheduler) handleMeasurementWindowDue(ctx context.Context, projectID uuid.UUID) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock($1)", lockKey(projectID)); err != nil {
		return err
	}
	q := db.New(tx)
	now := s.currentTime().UTC()
	actions, err := q.ListDueMeasuringContentActions(ctx, db.ListDueMeasuringContentActionsParams{
		ProjectID: projectID,
		NowAt:     pgtype.Timestamptz{Time: now, Valid: true},
		LimitRows: 50,
	})
	if err != nil {
		return err
	}
	for _, action := range actions {
		window, completed, remaining := completeDueMeasurementCheckpoints(action.MeasurementWindow, action.PublishedAt, now)
		status := "measuring"
		if remaining == 0 {
			status = "completed"
		}
		outcome := measurementOutcomeSummary(action, status, completed, remaining, now, window)
		if _, err := q.UpdateContentActionOutcomeSummary(ctx, db.UpdateContentActionOutcomeSummaryParams{
			ID:                action.ID,
			ProjectID:         projectID,
			Status:            status,
			OutcomeSummary:    outcome,
			MeasurementWindow: window,
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func completeDueMeasurementCheckpoints(raw json.RawMessage, publishedAt pgtype.Timestamptz, now time.Time) (json.RawMessage, int, int) {
	window := map[string]any{}
	if len(raw) > 0 && json.Valid(raw) {
		_ = json.Unmarshal(raw, &window)
	}
	window["last_checked_at"] = now.Format(time.RFC3339)

	checkpoints, _ := window["checkpoints"].([]any)
	if len(checkpoints) == 0 {
		window["state"] = "completed"
		window["latest_outcome"] = "inconclusive"
		return mustJSON(window), 0, 0
	}

	completedNow := 0
	remainingScheduled := 0
	for _, item := range checkpoints {
		checkpoint, ok := item.(map[string]any)
		if !ok {
			continue
		}
		status := strings.TrimSpace(fmt.Sprint(checkpoint["status"]))
		if status == "" {
			status = "scheduled"
		}
		if status != "scheduled" {
			continue
		}
		day := measurementCheckpointDay(checkpoint["day"])
		if measurementCheckpointDue(publishedAt, day, now) {
			checkpoint["status"] = "completed"
			checkpoint["completed_at"] = now.Format(time.RFC3339)
			checkpoint["outcome"] = "inconclusive"
			completedNow++
			continue
		}
		remainingScheduled++
	}

	state := "measuring"
	if remainingScheduled == 0 {
		state = "completed"
	}
	window["state"] = state
	window["latest_outcome"] = "inconclusive"
	return mustJSON(window), completedNow, remainingScheduled
}

func measurementCheckpointDay(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return i
		}
	}
	return 0
}

func measurementCheckpointDue(publishedAt pgtype.Timestamptz, day int, now time.Time) bool {
	if !publishedAt.Valid {
		return true
	}
	dueAt := publishedAt.Time.UTC().AddDate(0, 0, day)
	return !dueAt.After(now)
}

func measurementOutcomeSummary(action db.ContentAction, status string, completed, remaining int, now time.Time, window json.RawMessage) json.RawMessage {
	primaryMetric := ""
	windowData := map[string]any{}
	if len(window) > 0 && json.Valid(window) && json.Unmarshal(window, &windowData) == nil {
		primaryMetric, _ = windowData["primary_metric"].(string)
	}
	summary := "Measurement checkpoint processed without enough comparative search data to attribute movement."
	if status == "completed" {
		summary = "Measurement window closed; comparative search data remained inconclusive."
	}
	return mustJSON(map[string]any{
		"action_id":             action.ID.String(),
		"computed_at":           now.Format(time.RFC3339),
		"completed_checkpoints": completed,
		"outcome_summary":       "inconclusive",
		"primary_metric":        primaryMetric,
		"remaining_checkpoints": remaining,
		"result":                "inconclusive",
		"state":                 "inconclusive",
		"status":                status,
		"summary":               summary,
	})
}

func (s *Scheduler) handleContentPlanCreated(ctx context.Context, projectID uuid.UUID) error {
	project, err := db.New(s.Pool).GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	cfg, err := config.Parse(project.Config)
	if err != nil {
		return err
	}
	if !cfg.AutoAdvanceEnabled {
		s.Log.Info("workflow auto advance disabled", "project", projectID, "event", workflow.EventContentPlanCreated)
		return nil
	}
	return s.generateForProject(ctx, project)
}

func (s *Scheduler) handleDraftApproved(ctx context.Context, projectID uuid.UUID) error {
	project, err := db.New(s.Pool).GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	cfg, err := config.Parse(project.Config)
	if err != nil {
		return err
	}
	if !cfg.AutoAdvanceEnabled {
		s.Log.Info("workflow auto advance disabled", "project", projectID, "event", workflow.EventDraftApproved)
		return nil
	}
	if err := s.publishForProject(ctx, project); err != nil {
		return err
	}
	if err := s.reconcilePublishForProject(ctx, project); err != nil {
		return err
	}
	s.unlockVariants(ctx)
	return nil
}

func (s *Scheduler) handleOpportunityReviewed(ctx context.Context, projectID uuid.UUID) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock($1)", lockKey(projectID)); err != nil {
		return err
	}
	q := db.New(tx)
	project, err := q.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	cfg, err := config.Parse(project.Config)
	if err != nil {
		return err
	}
	if !cfg.AutoAdvanceEnabled {
		s.Log.Info("workflow auto advance disabled", "project", projectID)
		return tx.Commit(ctx)
	}
	open, err := q.CountOpenSEOOpportunities(ctx, projectID)
	if err != nil {
		return err
	}
	if open > 0 {
		return tx.Commit(ctx)
	}
	actions, err := q.ListUnplannedContentActions(ctx, db.ListUnplannedContentActionsParams{
		ProjectID: projectID,
		Limit:     50,
	})
	if err != nil {
		return err
	}
	if len(actions) == 0 {
		return tx.Commit(ctx)
	}
	batchKey := opportunityBatchKey(projectID, actions)
	_, err = q.EnqueueWorkflowEvent(ctx, db.EnqueueWorkflowEventParams{
		ProjectID:  projectID,
		EventType:  workflow.EventOpportunityBatchDone,
		DedupeKey:  workflowEventDedupeKey(workflow.EventOpportunityBatchDone, projectID, batchKey),
		Payload:    mustJSON(map[string]any{"unplanned_actions": len(actions), "batch_key": batchKey}),
		EntityType: ptr("project"),
		EntityID:   pgtype.UUID{Bytes: projectID, Valid: true},
	})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Scheduler) handleOpportunityBatchCompleted(ctx context.Context, projectID uuid.UUID) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock($1)", lockKey(projectID)); err != nil {
		return err
	}
	q := db.New(tx)
	project, err := q.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	cfg, err := config.Parse(project.Config)
	if err != nil {
		return err
	}
	if !cfg.AutoAdvanceEnabled {
		return tx.Commit(ctx)
	}
	actions, err := q.ListUnplannedContentActions(ctx, db.ListUnplannedContentActionsParams{
		ProjectID: projectID,
		Limit:     50,
	})
	if err != nil {
		return err
	}
	for i, action := range actions {
		savepointSQL := fmt.Sprintf("SAVEPOINT workflow_action_%d", i)
		rollbackSQL := fmt.Sprintf("ROLLBACK TO SAVEPOINT workflow_action_%d", i)
		releaseSQL := fmt.Sprintf("RELEASE SAVEPOINT workflow_action_%d", i)
		if _, err := tx.Exec(ctx, savepointSQL); err != nil {
			return err
		}
		topic, err := s.planOpportunityContentAction(ctx, q, projectID, action)
		if err != nil {
			s.Log.Warn("workflow action planning failed", "project", projectID, "action", action.ID, "err", err)
			if _, rollbackErr := tx.Exec(ctx, rollbackSQL); rollbackErr != nil {
				return rollbackErr
			}
			if _, markErr := q.UpdateContentActionStatus(ctx, db.UpdateContentActionStatusParams{
				ID:        action.ID,
				ProjectID: projectID,
				Status:    "failed",
			}); markErr != nil {
				return markErr
			}
			if _, releaseErr := tx.Exec(ctx, releaseSQL); releaseErr != nil {
				return releaseErr
			}
			continue
		}
		if _, err := tx.Exec(ctx, releaseSQL); err != nil {
			return err
		}
		s.Log.Info("workflow planned topic from opportunity action", "project", projectID, "action", action.ID, "topic", topic.ID)
	}
	remaining, err := q.ListUnplannedContentActions(ctx, db.ListUnplannedContentActionsParams{
		ProjectID: projectID,
		Limit:     1,
	})
	if err != nil {
		return err
	}
	if len(remaining) > 0 {
		batchKey := opportunityBatchKey(projectID, remaining)
		if _, err := q.EnqueueWorkflowEvent(ctx, db.EnqueueWorkflowEventParams{
			ProjectID:  projectID,
			EventType:  workflow.EventOpportunityBatchDone,
			DedupeKey:  workflowEventDedupeKey(workflow.EventOpportunityBatchDone, projectID, batchKey),
			Payload:    mustJSON(map[string]any{"unplanned_actions": len(remaining), "batch_key": batchKey}),
			EntityType: ptr("project"),
			EntityID:   pgtype.UUID{Bytes: projectID, Valid: true},
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Scheduler) planOpportunityContentAction(ctx context.Context, q *db.Queries, projectID uuid.UUID, action db.ContentAction) (db.Topic, error) {
	opp, err := q.GetSEOOpportunity(ctx, db.GetSEOOpportunityParams{ID: action.OpportunityID, ProjectID: projectID})
	if err != nil {
		return db.Topic{}, err
	}
	topic, err := q.CreateTopic(ctx, topicFromContentAction(projectID, action, opp))
	if err != nil {
		return db.Topic{}, err
	}
	if _, err := q.UpdateContentActionStatus(ctx, db.UpdateContentActionStatusParams{
		ID:        action.ID,
		ProjectID: projectID,
		Status:    "approved",
	}); err != nil {
		return db.Topic{}, err
	}
	if _, err := q.EnqueueWorkflowEvent(ctx, db.EnqueueWorkflowEventParams{
		ProjectID:  projectID,
		EventType:  workflow.EventContentPlanCreated,
		DedupeKey:  workflowEventDedupeKey(workflow.EventContentPlanCreated, projectID, action.ID.String()),
		Payload:    mustJSON(map[string]any{"action_id": action.ID, "topic_id": topic.ID}),
		EntityType: ptr("topic"),
		EntityID:   pgtype.UUID{Bytes: topic.ID, Valid: true},
	}); err != nil {
		return db.Topic{}, err
	}
	return topic, nil
}

func topicFromContentAction(projectID uuid.UUID, action db.ContentAction, opp db.SeoOpportunity) db.CreateTopicParams {
	title := firstNonEmpty(
		action.ActionType,
		stringPtrValue(opp.RecommendedAction),
		"Improve search visibility",
	)
	if opp.Query != nil && strings.TrimSpace(*opp.Query) != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(strings.TrimSpace(*opp.Query))) {
		title = title + ": " + strings.TrimSpace(*opp.Query)
	}
	targetPrompt := firstNonEmpty(stringPtrValue(opp.Query), stringPtrValue(opp.ExpectedImpact), action.ActionType)
	angle := firstNonEmpty(stringPtrValue(opp.ExpectedImpact), action.ActionType)
	priority := priorityFromOpportunityScore(pgutil.Float(opp.PriorityScore))
	internalLinks := []map[string]string{}
	if action.TargetUrl != nil && strings.TrimSpace(*action.TargetUrl) != "" {
		internalLinks = append(internalLinks, map[string]string{"url": strings.TrimSpace(*action.TargetUrl)})
	}
	return db.CreateTopicParams{
		ProjectID:             projectID,
		Channel:               "blog",
		Title:                 title,
		TargetKeyword:         opp.Query,
		TargetPrompt:          ptr(targetPrompt),
		Angle:                 ptr(angle),
		Format:                ptr("article"),
		Priority:              priority,
		InternalLinks:         mustJSON(internalLinks),
		Status:                string(topicstate.StatusBacklog),
		SourceContentActionID: pgtype.UUID{Bytes: action.ID, Valid: true},
	}
}

func opportunityBatchKey(projectID uuid.UUID, actions []db.ContentAction) string {
	if len(actions) == 0 {
		return projectID.String()
	}
	return actions[0].ID.String()
}

func workflowEventDedupeKey(eventType string, projectID uuid.UUID, parts ...string) string {
	key := eventType + ":" + projectID.String()
	for _, part := range parts {
		key += ":" + part
	}
	return key
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *Scheduler) TickReviewOverdue(ctx context.Context) {
	q := db.New(s.Pool)
	now := s.currentTime()
	articles, err := q.ListOverdueReviewArticles(ctx, db.ListOverdueReviewArticlesParams{
		CreatedAt: pgtype.Timestamptz{Time: now.Add(-reviewOverdueHours * time.Hour), Valid: true},
		Limit:     reviewOverdueLimit,
	})
	if err != nil {
		s.Log.Error("list overdue review articles", "err", err)
		return
	}
	for _, article := range articles {
		s.dispatchReviewOverdue(ctx, q, article)
	}
	if len(articles) > 0 {
		s.Log.Info("review overdue sweep dispatched", "count", len(articles))
	}
}

// TickSEO runs the SEO Operations Loop across all projects.
func (s *Scheduler) TickSEO(ctx context.Context) {
	q := db.New(s.Pool)
	projects, err := q.ListProjects(ctx)
	if err != nil {
		s.logger().Error("list projects for seo tick", "err", err)
		return
	}
	for _, p := range projects {
		if err := s.runSEOForProject(ctx, q, p); err != nil {
			s.logger().Error("seo tick failed", "project", p.ID, "err", err)
		}
	}
}

func (s *Scheduler) runSEOForProject(ctx context.Context, q *db.Queries, p db.Project) error {
	runner := s.newSEORunner(q)
	syncResult, err := runner.Sync(ctx, p.ID, s.BlogBaseURL)
	if err != nil {
		return fmt.Errorf("seo sync: %w", err)
	}
	analyzeResult, err := runner.Analyze(ctx, p.ID)
	if err != nil {
		return fmt.Errorf("seo analyze: %w", err)
	}
	brief, err := runner.Brief(ctx, p.ID)
	if err != nil {
		return fmt.Errorf("seo brief: %w", err)
	}
	s.logger().Info(
		"seo tick complete",
		"project", p.ID,
		"sync_status", syncResult.Status,
		"analyze_status", analyzeResult.Status,
		"brief_mode", brief.Mode,
		"brief_actions", len(brief.Actions),
	)
	return nil
}

func (s *Scheduler) newSEORunner(q *db.Queries) seoRunner {
	if s.seoRunnerFactory != nil {
		return s.seoRunnerFactory(q)
	}
	return seopkg.Service{
		Q:           q,
		HTTPClient:  s.httpClient,
		BlogBaseURL: s.BlogBaseURL,
		GoogleData:  s.SEOData,
		Now:         s.now,
	}
}

// TickContextRefresh lightly refreshes confirmed project Context from the
// configured project domain. It is intentionally smaller than onboarding.
func (s *Scheduler) TickContextRefresh(ctx context.Context) {
	q := db.New(s.Pool)
	projects, err := q.ListProjects(ctx)
	if err != nil {
		s.logger().Error("list projects for context refresh", "err", err)
		return
	}
	for _, p := range projects {
		if err := s.refreshContextForProject(ctx, q, p); err != nil {
			s.logger().Error("context refresh tick failed", "project", p.ID, "err", err)
		}
	}
}

func (s *Scheduler) refreshContextForProject(ctx context.Context, q *db.Queries, p db.Project) error {
	cfg, err := config.Parse(p.Config)
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.SiteURL) == "" {
		return nil
	}
	active, err := q.GetActiveProfile(ctx, p.ID)
	if err != nil {
		return nil
	}
	now := s.currentTime().UTC()
	if !contextmeta.HasConfirmation(active.Profile) || contextmeta.HasActiveCrawl(active.Profile) || !contextmeta.WeeklyRefreshDue(active.Profile, now) {
		return nil
	}
	startedProfile := contextmeta.StartedProfile(active.Profile, contextmeta.SourceWeekly, now)
	if _, err := q.UpdateProfile(ctx, db.UpdateProfileParams{ID: active.ID, Profile: startedProfile, SourceUrls: active.SourceUrls}); err != nil {
		return err
	}
	ag := agents.NewInsight(agents.Deps{Q: q, LLM: s.LLM, Search: s.Search}, s.Log)
	count, summary, err := ag.RunInventoryFromCrawl(ctx, p.ID, cfg.SiteURL, lightweightContextCrawlConfig(cfg.Crawl))
	if err != nil {
		_, _ = q.UpdateProfile(ctx, db.UpdateProfileParams{
			ID:         active.ID,
			Profile:    contextmeta.ClearStartedProfile(startedProfile),
			SourceUrls: active.SourceUrls,
		})
		return err
	}
	s.logger().Info("context refresh complete", "project", p.ID, "inventory_count", count, "fetched", summary.FetchedCount)
	return nil
}

func lightweightContextCrawlConfig(crawlCfg config.CrawlConfig) config.CrawlConfig {
	if crawlCfg.MaxPages <= 0 || crawlCfg.MaxPages > 5 {
		crawlCfg.MaxPages = 5
	}
	if crawlCfg.SitemapURLCap <= 0 || crawlCfg.SitemapURLCap > 20 {
		crawlCfg.SitemapURLCap = 20
	}
	if crawlCfg.RequestTimeoutMs <= 0 || crawlCfg.RequestTimeoutMs > 4000 {
		crawlCfg.RequestTimeoutMs = 4000
	}
	if crawlCfg.RateLimitRPS < 3 {
		crawlCfg.RateLimitRPS = 3
	}
	return crawlCfg
}

// TickGenerate runs the daily generation pass across all projects (§5.4).
func (s *Scheduler) TickGenerate(ctx context.Context) {
	q := db.New(s.Pool)
	projects, err := q.ListProjects(ctx)
	if err != nil {
		s.Log.Error("list projects", "err", err)
		return
	}
	for _, p := range projects {
		if err := s.generateForProject(ctx, p); err != nil {
			s.Log.Error("generate tick failed", "project", p.ID, "err", err)
		}
	}
}

func (s *Scheduler) generateForProject(ctx context.Context, p db.Project) error {
	cfg, err := config.Parse(p.Config)
	if err != nil {
		return err
	}
	if !cfg.AutoAdvanceEnabled {
		s.Log.Info("generation skipped; workflow auto advance disabled", "project", p.ID)
		return nil
	}
	candidates, err := s.reserveGenerationCandidates(ctx, p, cfg, func(q *db.Queries) ([]db.Topic, error) {
		// Buffer-window stocking (§5.4): keep `buffer_days` worth of content in
		// flight. desired = cadence_per_week * buffer_days / 7 (rounded up).
		// Generate only the deficit vs. what's already stocked.
		desired := ceilDiv(cfg.CadencePerWeek*cfg.BufferDays, 7)
		stocked, err := q.CountStockedCanonical(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		deficit := desired - int(stocked)
		if deficit <= 0 {
			s.Log.Info("buffer already stocked; skipping generation", "project", p.ID, "desired", desired, "stocked", stocked)
			return nil, nil
		}
		return q.SelectGenerationCandidates(ctx, db.SelectGenerationCandidatesParams{
			ProjectID: p.ID,
			Limit:     int32(deficit),
		})
	})
	if err != nil {
		return err
	}
	q := db.New(s.Pool)
	writer := agents.NewWriter(agents.Deps{Q: q, LLM: s.LLM, Search: s.Search}, s.Log)
	for _, t := range candidates {
		s.generateReservedCandidate(ctx, q, p, writer, t)
	}
	return nil
}

func (s *Scheduler) reserveGenerationCandidates(ctx context.Context, p db.Project, cfg config.ProjectConfig, selectCandidates func(*db.Queries) ([]db.Topic, error)) ([]db.Topic, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	locked, err := tryProjectGenerationLock(ctx, tx, p.ID)
	if err != nil {
		return nil, err
	}
	if !locked {
		s.Log.Info("generation skipped; project already reserved", "project", p.ID)
		return nil, nil
	}
	q := db.New(tx)

	// Cost breaker (§5.4): stop before any LLM call if month's spend >= budget.
	spent, err := q.MonthlySpend(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if pgutil.Float(spent) >= cfg.MonthlyBudgetUSD {
		s.alert(p.ID, "monthly budget reached; generation paused")
		s.dispatchBudgetStopped(ctx, q, p.ID, pgutil.Float(spent), cfg.MonthlyBudgetUSD)
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	}

	candidates, err := selectCandidates(q)
	if err != nil {
		return nil, err
	}
	reserved := make([]db.Topic, 0, len(candidates))
	for _, t := range candidates {
		if generating, ok := s.reserveGenerationCandidate(ctx, q, p, t); ok {
			reserved = append(reserved, generating)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return reserved, nil
}

func (s *Scheduler) reserveGenerationCandidate(ctx context.Context, q *db.Queries, p db.Project, t db.Topic) (db.Topic, bool) {
	// Idempotency: skip if a non-rejected article already exists (§5.4).
	n, err := q.CountNonRejectedArticlesForTopic(ctx, t.ID)
	if err != nil {
		return db.Topic{}, false
	}
	if n > 0 {
		if reconciled, changed, err := topicstate.ReconcileExistingDrafts(topicstate.Status(t.Status)); err != nil {
			s.Log.Warn("generation candidate topic draft reconciliation rejected", "topic", t.ID, "status", t.Status, "err", err)
		} else if changed {
			if _, err := q.UpdateTopicStatusForProject(ctx, db.UpdateTopicStatusForProjectParams{
				ID:        t.ID,
				ProjectID: p.ID,
				Status:    string(reconciled),
			}); err != nil {
				s.Log.Warn("generation candidate topic draft reconciliation failed", "topic", t.ID, "err", err)
			}
		}
		return db.Topic{}, false
	}
	nextStatus, err := topicstate.Transition(topicstate.Status(t.Status), topicstate.EventStartGeneration)
	if err != nil {
		s.Log.Warn("topic generation transition rejected", "topic", t.ID, "status", t.Status, "err", err)
		return db.Topic{}, false
	}
	generatingTopic, err := q.UpdateTopicStatusForProject(ctx, db.UpdateTopicStatusForProjectParams{
		ID:        t.ID,
		ProjectID: p.ID,
		Status:    string(nextStatus),
	})
	if err != nil {
		s.Log.Warn("mark generating failed", "topic", t.ID, "err", err)
		return db.Topic{}, false
	}
	if generatingTopic.SourceContentActionID.Valid {
		sourceActionID := uuid.UUID(generatingTopic.SourceContentActionID.Bytes)
		if _, err := q.UpdateContentActionStatus(ctx, db.UpdateContentActionStatusParams{
			ID:        sourceActionID,
			ProjectID: p.ID,
			Status:    "drafting",
		}); err != nil {
			s.Log.Warn("mark content action drafting failed", "topic", t.ID, "action", sourceActionID, "err", err)
		}
	}
	return generatingTopic, true
}

// generateReservedCandidate runs one already-reserved topic through generation
// into the review queue. It deliberately runs outside the advisory-lock
// reservation transaction, so admin deletes do not wait on LLM or QA calls.
func (s *Scheduler) generateReservedCandidate(ctx context.Context, q *db.Queries, p db.Project, writer *agents.Writer, t db.Topic) {
	sourceActionID := uuid.UUID{}
	if t.SourceContentActionID.Valid {
		sourceActionID = uuid.UUID(t.SourceContentActionID.Bytes)
	}
	if err := q.DeleteRecoverableArticlesForTopic(ctx, db.DeleteRecoverableArticlesForTopicParams{
		TopicID:   t.ID,
		ProjectID: p.ID,
	}); err != nil {
		s.Log.Warn("clear stale draft rows before generation failed", "project", p.ID, "topic", t.ID, "err", err)
		s.resetTopicAfterGenerationFailure(ctx, q, p.ID, t, sourceActionID)
		return
	}
	articles, err := writer.Generate(ctx, p.ID, t)
	if err != nil {
		s.alert(p.ID, "generation failed for topic "+t.Title+": "+err.Error())
		s.dispatchGenerationFailed(ctx, q, p.ID, "writer", t.ID.String(), t.Title, err.Error())
		s.resetTopicAfterGenerationFailure(ctx, q, p.ID, t, sourceActionID)
		return
	}
	if sourceActionID != uuid.Nil {
		if canonicalID := canonicalArticleID(articles); canonicalID != uuid.Nil {
			if _, err := q.MarkContentActionDraftReady(ctx, db.MarkContentActionDraftReadyParams{
				ID:             sourceActionID,
				ProjectID:      p.ID,
				DraftArticleID: pgtype.UUID{Bytes: canonicalID, Valid: true},
			}); err != nil {
				s.Log.Warn("mark content action draft ready failed", "topic", t.ID, "action", sourceActionID, "article", canonicalID, "err", err)
			}
		}
	}
	s.Log.Info("generated into review queue", "project", p.ID, "topic", t.Title)
}

func priorityFromOpportunityScore(score float64) int32 {
	if math.IsNaN(score) || math.IsInf(score, 0) || score <= 0 {
		return 5
	}
	if score > 10 {
		priority := int32(math.Ceil((100 - score) / 10))
		if priority < 1 {
			return 1
		}
		if priority > 10 {
			return 10
		}
		return priority
	}
	priority := int32(math.Round(score))
	if priority < 1 {
		return 1
	}
	if priority > 10 {
		return 10
	}
	return priority
}

func (s *Scheduler) resetTopicAfterGenerationFailure(ctx context.Context, q *db.Queries, projectID uuid.UUID, topic db.Topic, sourceActionID uuid.UUID) {
	status, err := topicstate.GenerationFailureStatus(topicstate.Status(topic.Status), topic.ScheduledAt.Valid)
	if err != nil {
		s.Log.Warn("generation failure topic state transition rejected", "project", projectID, "topic", topic.ID, "status", topic.Status, "err", err)
		return
	}
	if _, err := q.UpdateTopicStatusForProject(ctx, db.UpdateTopicStatusForProjectParams{
		ID:        topic.ID,
		ProjectID: projectID,
		Status:    string(status),
	}); err != nil {
		s.Log.Warn("reset topic after generation failure failed", "project", projectID, "topic", topic.ID, "err", err)
		return
	}
	if sourceActionID != uuid.Nil {
		if _, err := q.UpdateContentActionStatus(ctx, db.UpdateContentActionStatusParams{
			ID:        sourceActionID,
			ProjectID: projectID,
			Status:    "approved",
		}); err != nil {
			s.Log.Warn("reset content action after generation failure failed", "project", projectID, "topic", topic.ID, "action", sourceActionID, "err", err)
		}
	}
}

// TickScheduledTopics generates topics whose operator-set scheduled_at slot has
// arrived. Unlike the daily buffer-stocking pass it is time-driven and runs
// frequently, so a scheduled plan item drafts near its slot. It deliberately
// ignores AutoAdvance and buffer stocking — the operator explicitly scheduled
// the slot — but still honors the monthly cost breaker (§5.4).
func (s *Scheduler) TickScheduledTopics(ctx context.Context) {
	q := db.New(s.Pool)
	projects, err := q.ListProjects(ctx)
	if err != nil {
		s.Log.Error("list projects", "err", err)
		return
	}
	for _, p := range projects {
		if err := s.generateDueScheduledForProject(ctx, p); err != nil {
			s.Log.Error("scheduled topic tick failed", "project", p.ID, "err", err)
		}
	}
}

func (s *Scheduler) generateDueScheduledForProject(ctx context.Context, p db.Project) error {
	cfg, err := config.Parse(p.Config)
	if err != nil {
		return err
	}
	due, err := s.reserveGenerationCandidates(ctx, p, cfg, func(q *db.Queries) ([]db.Topic, error) {
		return q.SelectDueScheduledTopics(ctx, p.ID)
	})
	if err != nil {
		return err
	}
	q := db.New(s.Pool)
	writer := agents.NewWriter(agents.Deps{Q: q, LLM: s.LLM, Search: s.Search}, s.Log)
	for _, t := range due {
		s.generateReservedCandidate(ctx, q, p, writer, t)
	}
	return nil
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

// TickGEO runs the weekly GEO observation loop (§12.3).
func (s *Scheduler) TickGEO(ctx context.Context) {
	if s.Pool == nil {
		s.logger().Warn("geo tick skipped; database pool is not configured")
		return
	}
	q := db.New(s.Pool)
	projects, err := q.ListProjects(ctx)
	if err != nil {
		s.logger().Error("list projects for geo tick", "err", err)
		return
	}
	for _, p := range projects {
		if err := s.geoForProject(ctx, q, p); err != nil {
			s.logger().Error("geo tick failed", "project", p.ID, "err", err)
		}
	}
}

func (s *Scheduler) geoForProject(ctx context.Context, q *db.Queries, p db.Project) error {
	svc := s.geoService(q)
	logStep := func(step string, err error) {
		if err != nil {
			s.logger().Warn("geo tick step failed", "project", p.ID, "step", step, "err", err)
		}
	}

	_, auditErr := svc.RunCrawlerAudit(ctx, p.ID, geo.CrawlerAuditRequest{})
	logStep("crawler_audit", auditErr)
	_, observeErr := svc.ObserveAnswerProvider(ctx, p.ID, s.geoObserveRequest())
	logStep("observe_provider", observeErr)
	_, surfaceErr := svc.MonitorExternalSurfaces(ctx, p.ID, geo.MonitorExternalSurfacesRequest{Limit: 25})
	logStep("external_surfaces", surfaceErr)
	_, analyzeErr := svc.AnalyzeObservations(ctx, p.ID, geo.AnalyzeObservationsRequest{Limit: 100})
	logStep("analyze", analyzeErr)
	return nil
}

func (s *Scheduler) geoService(q *db.Queries) geo.Service {
	return geo.Service{
		Q:              q,
		HTTPClient:     s.httpClient,
		AnswerProvider: s.GEOAnswerProvider,
		Now:            s.currentTime,
	}
}

func (s *Scheduler) geoObserveRequest() geo.ObserveAnswerProviderRequest {
	budgetUSD := s.GEOProviderRunBudgetUSD
	if budgetUSD <= 0 {
		budgetUSD = 1
	}
	return geo.ObserveAnswerProviderRequest{
		Engine:     "Perplexity",
		MaxPrompts: 10,
		BudgetUSD:  budgetUSD,
	}
}

// TickPublish auto-publishes due canonicals and unlocks distributable variants.
func (s *Scheduler) TickPublish(ctx context.Context) {
	q := db.New(s.Pool)
	projects, err := q.ListProjects(ctx)
	if err != nil {
		s.Log.Error("list projects", "err", err)
		return
	}
	for _, p := range projects {
		if err := s.publishForProject(ctx, p); err != nil {
			s.Log.Error("publish tick failed", "project", p.ID, "err", err)
		}
		if err := s.reconcilePublishForProject(ctx, p); err != nil {
			s.Log.Error("publish reconcile failed", "project", p.ID, "err", err)
		}
	}
	// variant unlock is project-independent in query; run once.
	s.unlockVariants(ctx)
}

func (s *Scheduler) publishForProject(ctx context.Context, p db.Project) error {
	due, err := s.prepareDueCanonicals(ctx, p)
	if err != nil {
		return err
	}
	if len(due) == 0 {
		return nil
	}
	q := db.New(s.Pool)
	blog, err := s.blogPublisherForProject(ctx, q, p)
	if err != nil {
		return err
	}
	for _, a := range due {
		res, err := blog.Publish(ctx, &a)
		if err != nil {
			s.alert(p.ID, "publish failed for article "+a.ID.String()+": "+err.Error())
			s.markPublishFailed(ctx, q, p, a, "github_write", err.Error(), true)
			continue
		}
		resolvedSlug, publishPath := publishResultRefs(res)
		res.Phase = "pending_url_verification"
		if s.UniPostDeployHookURL != "" {
			if err := s.triggerUniPostDeployHook(ctx); err != nil {
				errText := "deploy hook failed: " + err.Error()
				res.DeployHook = "failed"
				if _, recErr := q.RecordPublishAttemptResult(ctx, db.RecordPublishAttemptResultParams{
					ID:                 a.ID,
					PublishResult:      mustJSON(res),
					ResolvedSlug:       resolvedSlug,
					PublishPath:        publishPath,
					NextPublishRetryAt: nextPublishRetryAt(s.currentTime(), a.PublishAttempts),
				}); recErr != nil {
					s.Log.Error("record publish result failed", "article", a.ID, "err", recErr)
				}
				s.markPublishFailed(ctx, q, p, a, "deploy_hook", errText, true)
				continue
			}
			res.DeployHook = "triggered"
		} else {
			res.DeployHook = "not_configured"
		}
		pubResult := mustJSON(res)
		if _, err := q.RecordPublishAttemptResult(ctx, db.RecordPublishAttemptResultParams{
			ID:                 a.ID,
			PublishResult:      pubResult,
			ResolvedSlug:       resolvedSlug,
			PublishPath:        publishPath,
			NextPublishRetryAt: pgtype.Timestamptz{Time: s.currentTime().Add(time.Minute), Valid: true},
		}); err != nil {
			errText := "record publish result failed: " + err.Error()
			s.Log.Error("record publish result failed", "article", a.ID, "err", err)
			s.markPublishFailed(ctx, q, p, a, "db_backfill", errText, true)
			continue
		}
		verifiedURL, err := s.resolvePublishedURL(ctx, res.URL)
		if err != nil {
			s.Log.Info("published content waiting for URL verification", "article", a.ID, "url", res.URL, "err", err)
			continue
		}
		if verifiedURL != res.URL {
			res.URL = verifiedURL
			pubResult = mustJSON(res)
		}
		published, err := q.MarkPublished(ctx, db.MarkPublishedParams{
			ID:            a.ID,
			PublishResult: pubResult,
			CanonicalUrl:  &res.URL,
			ResolvedSlug:  resolvedSlug,
			PublishPath:   publishPath,
		})
		if err != nil {
			errText := "mark published failed: " + err.Error()
			s.Log.Error("mark published failed", "article", a.ID, "err", err)
			s.markPublishFailed(ctx, q, p, a, "db_backfill", errText, true)
			continue
		}
		// Published canonical feeds back into inventory (§5.6).
		s.feedInventory(ctx, q, published, res.URL)
		s.markContentActionMeasuring(ctx, q, published)
		s.Log.Info("auto-published canonical", "article", a.ID, "url", res.URL)
	}
	return nil
}

func (s *Scheduler) prepareDueCanonicals(ctx context.Context, p db.Project) ([]db.Article, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock($1)", lockKey(p.ID)); err != nil {
		return nil, err
	}
	q := db.New(tx)
	due, err := q.SelectDueCanonical(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if len(due) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	}
	blog, err := s.blogPublisherForProject(ctx, q, p)
	if err != nil {
		return nil, err
	}
	prepared := make([]db.Article, 0, len(due))
	for _, a := range due {
		slug, publishPath, _, err := blog.Resolve(&a)
		if err != nil {
			return nil, err
		}
		phase := "github_write"
		preparedArticle, err := q.PreparePublishAttempt(ctx, db.PreparePublishAttemptParams{
			ID:           a.ID,
			ResolvedSlug: ptr(slug),
			PublishPath:  ptr(publishPath),
			PublishPhase: ptr(phase),
		})
		if err != nil {
			return nil, err
		}
		prepared = append(prepared, preparedArticle)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return prepared, nil
}

func (s *Scheduler) ReconcilePublishProject(ctx context.Context, p db.Project) error {
	return s.reconcilePublishForProject(ctx, p)
}

func (s *Scheduler) reconcilePublishForProject(ctx context.Context, p db.Project) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock($1)", lockKey(p.ID)); err != nil {
		return err
	}
	q := db.New(tx)
	candidates, err := q.SelectPublishReconcileCandidates(ctx, p.ID)
	if err != nil {
		return err
	}
	for _, a := range candidates {
		res, err := publishResultFromArticle(a)
		if err != nil {
			s.markPublishFailed(ctx, q, p, a, "reconcile_invalid_result", "reconcile publish_result invalid: "+err.Error(), true)
			continue
		}
		if res.URL == "" {
			s.markPublishFailed(ctx, q, p, a, "reconcile_missing_url", "reconcile missing canonical url", true)
			continue
		}
		if err := s.verifyPublishedPathForProject(ctx, q, p, res.Path); err != nil {
			s.markPublishFailed(ctx, q, p, a, "reconcile_missing_file", "reconcile content path failed: "+err.Error(), true)
			continue
		}
		verifiedURL, err := s.resolvePublishedURL(ctx, res.URL)
		if err != nil {
			if a.Status == "pending_url_verification" {
				if pendingURLVerificationDeadlineReached(a, s.currentTime()) {
					s.markPublishFailed(ctx, q, p, a, "reconcile_url_unverified", "reconcile url verification failed: "+err.Error(), true)
				} else {
					s.Log.Info("publish still waiting for URL verification", "article", a.ID, "url", res.URL, "err", err)
				}
			} else {
				s.markPublishFailed(ctx, q, p, a, "reconcile_url_unverified", "reconcile url verification failed: "+err.Error(), true)
			}
			continue
		}
		if verifiedURL != res.URL {
			res.URL = verifiedURL
		}
		resolvedSlug, publishPath := publishResultRefs(res)
		published, err := q.MarkPublished(ctx, db.MarkPublishedParams{
			ID:            a.ID,
			PublishResult: mustJSON(res),
			CanonicalUrl:  &res.URL,
			ResolvedSlug:  resolvedSlug,
			PublishPath:   publishPath,
		})
		if err != nil {
			return err
		}
		s.feedInventory(ctx, q, published, res.URL)
		s.markContentActionMeasuring(ctx, q, published)
		s.Log.Info("publish state reconciled", "article", a.ID, "url", res.URL)
	}
	return tx.Commit(ctx)
}

func (s *Scheduler) markContentActionMeasuring(ctx context.Context, q *db.Queries, article db.Article) {
	if q == nil || article.Kind != "canonical" {
		return
	}
	if _, err := q.MarkContentActionMeasuringForDraftArticle(ctx, db.MarkContentActionMeasuringForDraftArticleParams{
		ProjectID:      article.ProjectID,
		DraftArticleID: pgtype.UUID{Bytes: article.ID, Valid: true},
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		s.Log.Warn("mark content action measuring failed", "article", article.ID, "err", err)
	}
}

func (s *Scheduler) unlockVariants(ctx context.Context) {
	q := db.New(s.Pool)
	variants, err := q.SelectUnlockableVariants(ctx)
	if err != nil {
		s.Log.Error("select unlockable variants", "err", err)
		return
	}
	for _, v := range variants {
		// The joined canonical_url is needed; re-read sibling canonical.
		sibs, err := q.ListArticlesByTopic(ctx, v.TopicID)
		if err != nil {
			continue
		}
		var realURL string
		for _, sib := range sibs {
			if sib.Kind == "canonical" && sib.CanonicalUrl != nil {
				realURL = *sib.CanonicalUrl
			}
		}
		if realURL == "" {
			continue // guard: never unlock before canonical URL exists (§5.6)
		}
		newContent := publisher.RewriteForDistribution(v.ContentMd, realURL)
		// Backfill the canonical placeholder in seo_meta too — canonical-capable
		// platforms (Dev.to/Hashnode) carry it in seo_meta.canonical_url (§5.6).
		newSEO := []byte(publisher.RewriteForDistribution(string(v.SeoMeta), realURL))
		if _, err := q.UnlockVariant(ctx, db.UnlockVariantParams{
			ID: v.ID, CanonicalUrl: &realURL, ContentMd: newContent, SeoMeta: newSEO,
		}); err != nil {
			s.Log.Error("unlock variant failed", "article", v.ID, "err", err)
			continue
		}
		s.Log.Info("variant ready to distribute", "article", v.ID, "platform", deref(v.Platform))
	}
}

func (s *Scheduler) verifyPublishedURL(ctx context.Context, url string) error {
	if url == "" {
		return fmt.Errorf("empty published URL")
	}
	client := s.httpClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	if ok, err := request2xx(ctx, client, http.MethodHead, url); err == nil && ok {
		return nil
	}
	ok, err := request2xx(ctx, client, http.MethodGet, url)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("published URL did not return 2xx")
	}
	return nil
}

func (s *Scheduler) resolvePublishedURL(ctx context.Context, publishedURL string) (string, error) {
	if err := s.verifyPublishedURL(ctx, publishedURL); err == nil {
		return publishedURL, nil
	} else {
		normalizedURL, ok := normalizedPublishedURL(publishedURL)
		if !ok {
			return "", err
		}
		if normalizedErr := s.verifyPublishedURL(ctx, normalizedURL); normalizedErr == nil {
			return normalizedURL, nil
		} else {
			return "", fmt.Errorf("published URL %q did not verify: %w; normalized URL %q also did not verify: %v", publishedURL, err, normalizedURL, normalizedErr)
		}
	}
}

func normalizedPublishedURL(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Path == "" {
		return "", false
	}
	base := path.Base(u.Path)
	if base == "." || base == "/" || base == "" {
		return "", false
	}
	normalized := publisher.NormalizeBlogSlug(base)
	if normalized == "" || normalized == base {
		return "", false
	}
	dir := path.Dir(u.Path)
	if dir == "." || dir == "/" {
		u.Path = "/" + normalized
	} else {
		u.Path = strings.TrimRight(dir, "/") + "/" + normalized
	}
	return u.String(), true
}

func (s *Scheduler) triggerUniPostDeployHook(ctx context.Context) error {
	if s.UniPostDeployHookURL == "" {
		return nil
	}
	client := s.httpClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.UniPostDeployHookURL, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("deploy hook returned %d", resp.StatusCode)
	}
	return nil
}

func (s *Scheduler) verifyPublishedPath(ctx context.Context, publishPath string) error {
	if publishPath == "" || s.Blog == nil {
		return nil
	}
	return s.Blog.PublishedPathExists(ctx, publishPath)
}

func (s *Scheduler) verifyPublishedPathForProject(ctx context.Context, q *db.Queries, p db.Project, publishPath string) error {
	if publishPath == "" {
		return nil
	}
	blog, err := s.blogPublisherForProject(ctx, q, p)
	if err != nil {
		return err
	}
	if blog == nil {
		return nil
	}
	return blog.PublishedPathExists(ctx, publishPath)
}

func (s *Scheduler) blogPublisherForProject(ctx context.Context, q publisherConnectionQuerier, p db.Project) (*publisher.BlogPublisher, error) {
	if q == nil {
		return nil, errors.New("publisher connection store unavailable")
	}
	projectTarget := githubNextJSTargetForProject(p)
	conn, err := q.GetEnabledPublisherConnectionForProject(ctx, db.GetEnabledPublisherConnectionForProjectParams{
		ProjectID: p.ID,
		Kind:      publisher.ConnectionKindGitHubNextJS,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("enabled publisher connection is required for project %s", p.ID)
		}
		return nil, err
	}
	// Prefer a GitHub App installation token (no stored PAT) when the connection
	// was set up via Connect GitHub; fall back to the pasted-credential token.
	token, err := s.githubInstallationToken(ctx, conn)
	if err != nil {
		return nil, err
	}
	if token == "" {
		token, err = s.publisherCredentialToken(ctx, q, conn)
		if err != nil {
			return nil, err
		}
	}
	blog, _, err := blogPublisherFromConnection(s.Blog, token, conn, s.Log, projectTarget)
	return blog, err
}

// githubInstallationToken mints a short-lived installation token when the
// connection stores an installation_id and the GitHub App is configured.
func (s *Scheduler) githubInstallationToken(ctx context.Context, conn db.PublisherConnection) (string, error) {
	if s.GitHubApp == nil || !s.GitHubApp.Configured() {
		return "", nil
	}
	var cfg struct {
		InstallationID string `json:"installation_id"`
	}
	if len(conn.Config) > 0 {
		_ = json.Unmarshal(conn.Config, &cfg)
	}
	if strings.TrimSpace(cfg.InstallationID) == "" {
		return "", nil
	}
	return s.GitHubApp.InstallationToken(ctx, cfg.InstallationID)
}

func blogPublisherFromConnection(fallback *publisher.BlogPublisher, token string, conn db.PublisherConnection, log *slog.Logger, target *publisher.GitHubNextJSTarget) (*publisher.BlogPublisher, bool, error) {
	if conn.Kind != publisher.ConnectionKindGitHubNextJS {
		return fallback, false, fmt.Errorf("unsupported publisher connection kind %q", conn.Kind)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, true, errors.New("publisher credential is required for GitHub/Next.js publishing")
	}
	cfg, err := publisher.ParseGitHubNextJSConfig(conn.Config)
	if err != nil {
		return fallback, false, err
	}
	if target != nil {
		cfg.Branch = target.Branch
		cfg.BaseURL = target.BaseURL
	}
	return publisher.NewBlog(token, cfg.Repo, cfg.Branch, cfg.BaseURL, cfg.ContentDir, log), true, nil
}

func githubNextJSTargetForProject(p db.Project) *publisher.GitHubNextJSTarget {
	cfg, err := config.Parse(p.Config)
	if err != nil {
		return nil
	}
	target, ok := publisher.GitHubNextJSTargetForSiteURL(cfg.SiteURL)
	if !ok {
		return nil
	}
	return &target
}

func (s *Scheduler) publisherCredentialToken(ctx context.Context, q publisherConnectionQuerier, conn db.PublisherConnection) (string, error) {
	if conn.CredentialRef == nil {
		return "", nil
	}
	ref := strings.TrimSpace(*conn.CredentialRef)
	if publisher.IsEnvPublisherCredentialRef(ref) {
		return "", errors.New("project-scoped publisher credential is required; env:GITHUB_TOKEN fallback is disabled")
	}
	credentialID, ok := publisher.ParsePublisherCredentialRef(ref)
	if !ok {
		return "", nil
	}
	if q == nil {
		return "", errors.New("publisher credential store unavailable")
	}
	if strings.TrimSpace(s.NotificationSecret) == "" {
		return "", errors.New("NOTIFICATION_SECRET_KEY is required")
	}
	cred, err := q.GetActivePublisherCredential(ctx, db.GetActivePublisherCredentialParams{
		ID:           credentialID,
		ProjectID:    conn.ProjectID,
		ConnectionID: conn.ID,
	})
	if err != nil {
		return "", err
	}
	return secretbox.DecryptString(cred.EncryptedValue, s.NotificationSecret)
}

func nextPublishRetryAt(now time.Time, attempt int32) pgtype.Timestamptz {
	delays := []time.Duration{
		5 * time.Minute,
		15 * time.Minute,
		time.Hour,
		6 * time.Hour,
	}
	if attempt <= 0 || int(attempt) > len(delays) {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: now.Add(delays[attempt-1]), Valid: true}
}

func pendingURLVerificationDeadlineReached(a db.Article, now time.Time) bool {
	return a.Status == "pending_url_verification" &&
		a.NextPublishRetryAt.Valid &&
		!a.NextPublishRetryAt.Time.After(now)
}

func (s *Scheduler) markPublishFailed(ctx context.Context, q *db.Queries, p db.Project, a db.Article, phase, errText string, transition bool) {
	failed, markErr := q.MarkPublishFailed(ctx, db.MarkPublishFailedParams{
		ID:                 a.ID,
		LastPublishError:   ptr(errText),
		NextPublishRetryAt: nextPublishRetryAt(s.currentTime(), a.PublishAttempts),
		PublishPhase:       ptr(phase),
	})
	if markErr != nil {
		s.Log.Error("mark publish failed state failed", "article", a.ID, "err", markErr)
	} else {
		a = failed
	}
	s.dispatchPublishFailed(ctx, q, p, a, phase, errText, transition)
}

func request2xx(ctx context.Context, client *http.Client, method, url string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}

func (s *Scheduler) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Scheduler) logger() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

func (s *Scheduler) dispatchBudgetStopped(ctx context.Context, store notification.DispatchStore, projectID uuid.UUID, spentUSD, budgetUSD float64) {
	event := notification.NewBudgetStoppedEvent(projectID, spentUSD, budgetUSD, s.currentTime(), "/projects/"+projectID.String())
	s.dispatchNotification(ctx, store, event)
}

func (s *Scheduler) dispatchGenerationFailed(ctx context.Context, store notification.DispatchStore, projectID uuid.UUID, agent, scope, title, errText string) {
	event := notification.NewGenerationFailedEvent(
		projectID,
		agent,
		scope,
		title,
		errText,
		s.currentTime(),
		"/projects/"+projectID.String()+"/runs",
	)
	s.dispatchNotification(ctx, store, event)
}

func (s *Scheduler) dispatchPublishFailed(ctx context.Context, store notification.DispatchStore, p db.Project, a db.Article, phase, errText string, transition bool) {
	title, slug := articleTitleSlug(a)
	event := notification.NewPublishFailedEvent(
		p.ID,
		a.ID,
		title,
		slug,
		phase,
		a.PublishAttempts,
		errText,
		s.currentTime(),
		"/projects/"+p.ID.String()+"/publishing",
		transition,
	)
	s.dispatchNotification(ctx, store, event)
}

func (s *Scheduler) dispatchReviewOverdue(ctx context.Context, store notification.DispatchStore, a db.Article) {
	title, _ := articleTitleSlug(a)
	now := s.currentTime()
	ageHours := 0
	if a.CreatedAt.Valid {
		ageHours = int(now.Sub(a.CreatedAt.Time) / time.Hour)
		if ageHours < 0 {
			ageHours = 0
		}
	}
	event := notification.NewReviewOverdueEvent(
		a.ProjectID,
		a.ID,
		title,
		ageHours,
		now,
		"/projects/"+a.ProjectID.String()+"/review",
	)
	s.dispatchNotification(ctx, store, event)
}

func (s *Scheduler) dispatchNotification(ctx context.Context, store notification.DispatchStore, event notification.Event) {
	if err := notification.Dispatch(ctx, store, event); err != nil {
		log := s.Log
		if log == nil {
			log = slog.Default()
		}
		log.Warn("notification dispatch failed", "event_type", event.Type, "event_id", event.ID, "err", err)
	}
}

func articleTitleSlug(a db.Article) (string, string) {
	seo := struct {
		Title string `json:"title"`
		H1    string `json:"h1"`
		Slug  string `json:"slug"`
	}{}
	_ = jsonUnmarshal(a.SeoMeta, &seo)
	title := seo.Title
	if title == "" {
		title = seo.H1
	}
	if title == "" {
		title = a.ID.String()
	}
	return title, seo.Slug
}

func publishResultFromArticle(a db.Article) (publisher.Result, error) {
	var res publisher.Result
	if len(a.PublishResult) > 0 {
		if err := jsonUnmarshal(a.PublishResult, &res); err != nil {
			return res, err
		}
	}
	if res.URL == "" && a.CanonicalUrl != nil {
		res.URL = *a.CanonicalUrl
	}
	if res.Path == "" && a.PublishPath != nil {
		res.Path = *a.PublishPath
	}
	return res, nil
}

func publishResultRefs(res publisher.Result) (*string, *string) {
	slug := ""
	if res.Path != "" {
		base := path.Base(res.Path)
		slug = strings.TrimSuffix(strings.TrimSuffix(base, ".mdx"), ".md")
	}
	if slug == "" && res.URL != "" {
		if parsed, err := url.Parse(res.URL); err == nil {
			slug = path.Base(parsed.Path)
		}
	}
	return ptr(slug), ptr(res.Path)
}

func (s *Scheduler) feedInventory(ctx context.Context, q *db.Queries, a db.Article, url string) {
	seo := struct {
		Title         string `json:"title"`
		TargetKeyword string `json:"target_keyword"`
	}{}
	_ = jsonUnmarshal(a.SeoMeta, &seo)
	_, _ = q.UpsertInventory(ctx, db.UpsertInventoryParams{
		ProjectID:        a.ProjectID,
		Url:              url,
		Title:            ptr(seo.Title),
		TargetKeyword:    ptr(seo.TargetKeyword),
		Topics:           []byte("[]"),
		EvidenceSnippets: []byte("[]"),
		Source:           "generated",
	})
}

// lockKey derives a stable int64 advisory-lock key from a project UUID.
func lockKey(id uuid.UUID) int64 {
	b := id[:]
	return int64(binary.BigEndian.Uint64(b[:8]))
}

func tryProjectGenerationLock(ctx context.Context, tx pgx.Tx, projectID uuid.UUID) (bool, error) {
	var locked bool
	if err := tx.QueryRow(ctx, "select pg_try_advisory_xact_lock($1)", lockKey(projectID)).Scan(&locked); err != nil {
		return false, err
	}
	return locked, nil
}

// LockKey exposes the scheduler's project advisory-lock key for admin cleanup
// paths that must coordinate with background project work.
func LockKey(id uuid.UUID) int64 {
	return lockKey(id)
}

var _ = pgx.ErrNoRows // keep pgx import for callers that switch on it
