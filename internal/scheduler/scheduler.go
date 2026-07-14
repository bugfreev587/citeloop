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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/admin"
	"github.com/citeloop/citeloop/internal/agents"
	"github.com/citeloop/citeloop/internal/articleassets"
	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/contextmeta"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/citeloop/citeloop/internal/geo"
	"github.com/citeloop/citeloop/internal/githubapp"
	"github.com/citeloop/citeloop/internal/growthradar"
	"github.com/citeloop/citeloop/internal/growthwork"
	"github.com/citeloop/citeloop/internal/learning"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/measurement"
	"github.com/citeloop/citeloop/internal/notification"
	"github.com/citeloop/citeloop/internal/opportunityfinding"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/platformcontract"
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

var errDirectContentAction = errors.New("direct content action does not need topic planning")

type seoRunner interface {
	Sync(context.Context, uuid.UUID, string) (seopkg.SyncResult, error)
	Analyze(context.Context, uuid.UUID) (seopkg.SyncResult, error)
	Brief(context.Context, uuid.UUID) (seopkg.Brief, error)
	EnsureGrowthOpportunityReservations(context.Context, uuid.UUID) error
	StartDoctorRun(context.Context, seopkg.DoctorRunRequest) (db.SeoDoctorRun, bool, error)
	RunDoctor(context.Context, uuid.UUID, uuid.UUID) (seopkg.DoctorReport, error)
}

type Scheduler struct {
	Pool                     *pgxpool.Pool
	LLM                      llm.Provider
	Search                   search.Provider
	Blog                     *publisher.BlogPublisher
	SEOData                  seopkg.GoogleDataProvider
	GEOAnswerProvider        geo.AnswerProvider
	GEOProviderRunBudgetUSD  float64
	BlogBaseURL              string
	Log                      *slog.Logger
	now                      func() time.Time
	alert                    func(projectID uuid.UUID, msg string)
	httpClient               *http.Client
	siteFixVerifier          canonicalSiteFixPageVerifier
	seoRunnerFactory         func(q *db.Queries) seoRunner
	NotificationSecret       string
	ResendAPIKey             string
	NotificationEmailFrom    string
	NotificationEmailReplyTo string
	UniPostDeployHookURL     string
	GitHubApp                *githubapp.Service
	ArticleAssets            *articleassets.Service
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
		Store: db.New(s.Pool),
		Sender: notification.HTTPSender{
			Client:       s.httpClient,
			ResendAPIKey: s.ResendAPIKey,
			EmailFrom:    s.NotificationEmailFrom,
			EmailReplyTo: s.NotificationEmailReplyTo,
		},
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
	case workflow.EventOpportunityFindingRequested:
		return s.handleOpportunityFindingRequested(ctx, event)
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

func (s *Scheduler) handleOpportunityFindingRequested(ctx context.Context, event db.WorkflowEvent) error {
	q := db.New(s.Pool)
	project, err := q.GetProject(ctx, event.ProjectID)
	if err != nil {
		return err
	}
	trigger, _, err := opportunityFindingTrigger(event)
	if err != nil {
		return workflow.Permanent(err)
	}
	cfg, err := config.Parse(project.Config)
	if err != nil {
		return workflow.Permanent(err)
	}
	stages := cfg.OpportunityFindingStagesForTrigger(trigger)
	observeRequest := s.geoObserveRequest()
	inputs := map[string]any{
		"version": "opportunity-finding/v1", "trigger": trigger,
		"signal_scan": stages.SignalScan, "ai_discovery": stages.AIDiscovery,
		"blog_base_url": s.BlogBaseURL, "observe_request": observeRequest,
	}
	runner := s.newSEORunner(q, (growthwork.ComparatorAuthority{Provider: s.LLM}).ForConfig(cfg, trigger))
	summary, err := opportunityfinding.RunCheckpointedWorkflow(ctx, q, opportunityfinding.WorkflowRequest{
		ProjectID: project.ID, WorkflowEventID: event.ID, Inputs: inputs, Now: s.currentTime,
	}, func(stageCtx context.Context, stage opportunityfinding.Stage, progress []opportunityfinding.StageProgress) opportunityfinding.StageOutcome {
		return s.executeOpportunityFindingStage(stageCtx, q, runner, project, cfg, trigger, observeRequest, stage, progress)
	})
	if err != nil {
		return err
	}
	s.logger().Info("opportunity finding workflow complete", "project", project.ID, "workflow_event", event.ID, "status", summary.Status, "stage_errors", summary.ErrorCount)
	return nil
}

func opportunityFindingTrigger(event db.WorkflowEvent) (config.GrowthAITrigger, bool, error) {
	payload := struct {
		Trigger config.GrowthAITrigger `json:"trigger"`
	}{}
	if len(event.Payload) > 0 {
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return "", false, fmt.Errorf("decode Opportunity Finding trigger: %w", err)
		}
	}
	if payload.Trigger == "" {
		payload.Trigger = config.GrowthAITriggerManual
	}
	switch payload.Trigger {
	case config.GrowthAITriggerManual:
		return payload.Trigger, false, nil
	case config.GrowthAITriggerScheduled:
		return payload.Trigger, true, nil
	case config.GrowthAITriggerEvent:
		return payload.Trigger, false, nil
	default:
		return "", false, fmt.Errorf("unsupported Opportunity Finding trigger %q", payload.Trigger)
	}
}

func (s *Scheduler) RecomputeMeasurements(ctx context.Context, projectID uuid.UUID) error {
	return s.handleMeasurementWindowDue(ctx, projectID)
}

// TickMeasurements guarantees that every measuring Growth Action advances even
// when no workflow event was enqueued. Per-project advisory locks and immutable
// checkpoint inserts make retries safe.
func (s *Scheduler) TickMeasurements(ctx context.Context) {
	if err := s.TickSiteFixMeasurements(ctx); err != nil {
		s.Log.Error("Site Fix measurement tick failed", "err", err)
	}
	projects, err := db.New(s.Pool).ListProjects(ctx)
	if err != nil {
		s.Log.Error("measurement tick list projects failed", "err", err)
		return
	}
	for _, project := range projects {
		if err := s.RecomputeMeasurements(ctx, project.ID); err != nil {
			s.Log.Error("measurement tick failed", "project", project.ID, "err", err)
		}
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
		if !action.MeasuringStartedAt.Valid {
			action, err = q.BindLegacyMeasuringContentActionPolicy(ctx, db.BindLegacyMeasuringContentActionPolicyParams{
				ID: action.ID, ProjectID: action.ProjectID,
			})
			if err != nil {
				return err
			}
		}
		window, completed, remaining, terminalReason := completeActionMeasurementCheckpoints(action, now)
		opportunity, err := q.GetSEOOpportunity(ctx, db.GetSEOOpportunityParams{ID: action.OpportunityID, ProjectID: action.ProjectID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		window, remaining, evaluatedTerminalReason, err := evaluateMeasurementCheckpoints(ctx, q, action, opportunity, window, now)
		if err != nil {
			return err
		}
		if evaluatedTerminalReason != "" {
			terminalReason = &evaluatedTerminalReason
		}
		completed = newlyCompletedCheckpointCount(window, now)
		status := "measuring"
		if remaining == 0 {
			status = "completed"
		}
		outcome := measurementOutcomeSummary(action, status, completed, remaining, now, window)
		for _, measurement := range actionMeasurementsFromWindow(action, window, now) {
			if err := q.InsertActionMeasurementCheckpoint(ctx, measurement); err != nil {
				return err
			}
		}
		updatedAction, err := q.UpdateContentActionOutcomeSummary(ctx, db.UpdateContentActionOutcomeSummaryParams{
			ID:                        action.ID,
			ProjectID:                 projectID,
			Status:                    status,
			OutcomeSummary:            outcome,
			MeasurementWindow:         window,
			MeasurementTerminalReason: terminalReason,
		})
		if err != nil {
			return err
		}
		if status == "completed" {
			reason := ""
			if terminalReason != nil {
				reason = *terminalReason
			}
			if err := learning.RecordTerminalOutcome(ctx, q, updatedAction, opportunity, window, outcome, reason); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

func newlyCompletedCheckpointCount(raw json.RawMessage, now time.Time) int {
	var window map[string]any
	if json.Unmarshal(raw, &window) != nil {
		return 0
	}
	checkpoints, _ := window["checkpoints"].([]any)
	completedAt := now.UTC().Format(time.RFC3339)
	count := 0
	for _, item := range checkpoints {
		checkpoint, ok := item.(map[string]any)
		if ok && strings.TrimSpace(stringFromAny(checkpoint["completed_at"])) == completedAt {
			count++
		}
	}
	return count
}

func completeActionMeasurementCheckpoints(action db.ContentAction, now time.Time) (json.RawMessage, int, int, *string) {
	policy, err := measurement.Parse(action.MeasurementPolicy)
	if err != nil {
		policy = measurement.LegacyPolicy()
	}
	start := action.MeasuringStartedAt
	if !start.Valid {
		start = action.PublishedAt
	}
	if !start.Valid {
		start = pgtype.Timestamptz{Time: now.UTC(), Valid: true}
	}
	deadline := action.AbsoluteTerminalAt
	if !deadline.Valid {
		deadline = pgtype.Timestamptz{Time: policy.AbsoluteTerminalAt(start.Time), Valid: true}
	}
	window, completed, remaining, terminalReason := completeDueMeasurementCheckpointsWithPolicy(
		action.MeasurementWindow, start, deadline, policy, now,
	)
	if terminalReason == "" {
		return window, completed, remaining, nil
	}
	return window, completed, remaining, &terminalReason
}

func completeDueMeasurementCheckpoints(raw json.RawMessage, publishedAt pgtype.Timestamptz, now time.Time) (json.RawMessage, int, int) {
	policy := measurement.LegacyPolicy()
	deadline := pgtype.Timestamptz{}
	if publishedAt.Valid {
		deadline = pgtype.Timestamptz{Time: policy.AbsoluteTerminalAt(publishedAt.Time), Valid: true}
	}
	window, completed, remaining, _ := completeDueMeasurementCheckpointsWithPolicy(raw, publishedAt, deadline, policy, now)
	return window, completed, remaining
}

func completeDueMeasurementCheckpointsWithPolicy(raw json.RawMessage, startedAt, absoluteTerminalAt pgtype.Timestamptz, policy measurement.Policy, now time.Time) (json.RawMessage, int, int, string) {
	window := map[string]any{}
	if len(raw) > 0 && json.Valid(raw) {
		_ = json.Unmarshal(raw, &window)
	}
	window["last_checked_at"] = now.Format(time.RFC3339)
	window["policy_version"] = policy.PolicyVersion
	checkpoints := normalizeMeasurementCheckpoints(window["checkpoints"], policy)
	window["checkpoints"] = checkpoints
	deadlineReached := absoluteTerminalAt.Valid && !absoluteTerminalAt.Time.UTC().After(now.UTC())

	completedNow := 0
	remainingScheduled := 0
	for _, item := range checkpoints {
		checkpoint := item
		status := strings.TrimSpace(fmt.Sprint(checkpoint["status"]))
		if status == "" {
			status = "scheduled"
		}
		if status != "scheduled" {
			continue
		}
		day := measurementCheckpointDay(checkpoint["day"])
		if deadlineReached || measurementCheckpointDue(startedAt, day, now) {
			if deadlineReached && !measurementCheckpointDue(startedAt, day, now) {
				checkpoint["status"] = "expired"
			} else {
				checkpoint["status"] = "completed"
			}
			checkpoint["completed_at"] = now.Format(time.RFC3339)
			checkpoint["outcome"] = "insufficient_data"
			checkpoint["outcome_label"] = "insufficient_data"
			checkpoint["outcome_reason"] = measurementInsufficientDataReason()
			checkpoint["attribution_confidence"] = "low"
			checkpoint["data_quality_state"] = "insufficient"
			checkpoint["source_freshness"] = map[string]any{}
			checkpoint["confounders"] = measurementDefaultConfounders()
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
	window["latest_outcome"] = "insufficient_data"
	window["outcome_label"] = "insufficient_data"
	window["outcome_reason"] = measurementInsufficientDataReason()
	terminalReason := ""
	if deadlineReached {
		terminalReason = "absolute_deadline_reached"
		window["state"] = "completed"
		window["terminal_reason"] = terminalReason
		remainingScheduled = 0
	} else if remainingScheduled == 0 {
		terminalReason = "measurement_checkpoints_completed"
		window["terminal_reason"] = terminalReason
	}
	return mustJSON(window), completedNow, remainingScheduled, terminalReason
}

func normalizeMeasurementCheckpoints(raw any, policy measurement.Policy) []map[string]any {
	existing := map[int]map[string]any{}
	if values, ok := raw.([]any); ok {
		for _, value := range values {
			if checkpoint, ok := value.(map[string]any); ok {
				existing[measurementCheckpointDay(checkpoint["day"])] = checkpoint
			}
		}
	}
	checkpoints := make([]map[string]any, 0, len(policy.Checkpoints()))
	for _, definition := range policy.Checkpoints() {
		checkpoint := existing[definition.Day]
		if checkpoint == nil {
			checkpoint = map[string]any{"day": definition.Day, "status": "scheduled"}
		}
		checkpoint["day"] = definition.Day
		checkpoint["role"] = string(definition.Role)
		checkpoint["attempt"] = definition.Attempt
		checkpoints = append(checkpoints, checkpoint)
	}
	return checkpoints
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

func actionMeasurementsFromWindow(action db.ContentAction, window json.RawMessage, now time.Time) []db.InsertActionMeasurementCheckpointParams {
	var data map[string]any
	if len(window) == 0 || !json.Valid(window) || json.Unmarshal(window, &data) != nil {
		return nil
	}
	checkpoints, _ := data["checkpoints"].([]any)
	if len(checkpoints) == 0 {
		return nil
	}
	measurements := make([]db.InsertActionMeasurementCheckpointParams, 0, len(checkpoints))
	for _, item := range checkpoints {
		checkpoint, ok := item.(map[string]any)
		if !ok {
			continue
		}
		status := strings.TrimSpace(fmt.Sprint(checkpoint["status"]))
		if status != "completed" && status != "expired" {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(checkpoint["completed_at"])) != now.UTC().Format(time.RFC3339) {
			continue
		}
		day := measurementCheckpointDay(checkpoint["day"])
		measurementStart := action.MeasuringStartedAt
		if !measurementStart.Valid {
			measurementStart = action.PublishedAt
		}
		windowStart, windowEnd := measurementWindowDates(measurementStart, day, now)
		articleID := action.DraftArticleID
		if !articleID.Valid {
			articleID = action.TargetArticleID
		}
		outcomeLabel := firstNonEmptyString(fmt.Sprint(checkpoint["outcome_label"]), fmt.Sprint(checkpoint["outcome"]), "insufficient_data")
		if outcomeLabel == "<nil>" {
			outcomeLabel = "insufficient_data"
		}
		outcomeReason := strings.TrimSpace(fmt.Sprint(checkpoint["outcome_reason"]))
		if outcomeReason == "" || outcomeReason == "<nil>" {
			outcomeReason = measurementInsufficientDataReason()
		}
		confidence := strings.TrimSpace(fmt.Sprint(checkpoint["attribution_confidence"]))
		if confidence == "" || confidence == "<nil>" {
			confidence = "low"
		}
		confounders := checkpoint["confounders"]
		if confounders == nil {
			confounders = measurementDefaultConfounders()
		}
		measurements = append(measurements, db.InsertActionMeasurementCheckpointParams{
			ProjectID:                action.ProjectID,
			ContentActionID:          action.ID,
			ArticleID:                articleID,
			CheckpointDay:            int32(day),
			WindowStart:              windowStart,
			WindowEnd:                windowEnd,
			SeoMetrics:               mustJSON(firstNonNil(checkpoint["seo_metrics"], map[string]any{})),
			Ga4Metrics:               mustJSON(firstNonNil(checkpoint["ga4_metrics"], map[string]any{})),
			GeoMetrics:               mustJSON(firstNonNil(checkpoint["geo_metrics"], map[string]any{})),
			ExecutionMetrics:         measurementExecutionMetrics(action, data, checkpoint),
			OutcomeLabel:             outcomeLabel,
			OutcomeReason:            outcomeReason,
			AttributionConfidence:    confidence,
			Confounders:              mustJSON(confounders),
			ComputedAt:               pgtype.Timestamptz{Time: now, Valid: true},
			CheckpointRole:           firstNonEmptyString(fmt.Sprint(checkpoint["role"]), string(measurement.RolePrimary)),
			MeasurementPolicyVersion: firstNonEmptyString(action.MeasurementPolicyVersion, measurement.LegacyPolicy().PolicyVersion),
			CheckpointAttempt:        int32(max(1, measurementCheckpointDay(checkpoint["attempt"]))),
			DataQualityState:         firstNonEmptyString(fmt.Sprint(checkpoint["data_quality_state"]), "insufficient"),
			SourceFreshness:          mustJSON(firstNonNil(checkpoint["source_freshness"], map[string]any{})),
		})
	}
	return measurements
}

func measurementWindowDates(publishedAt pgtype.Timestamptz, day int, now time.Time) (pgtype.Date, pgtype.Date) {
	start := now.UTC()
	if publishedAt.Valid {
		start = publishedAt.Time.UTC()
	}
	end := start.AddDate(0, 0, max(0, day-1))
	return pgtype.Date{Time: dateOnly(start), Valid: true}, pgtype.Date{Time: dateOnly(end), Valid: true}
}

func dateOnly(t time.Time) time.Time {
	year, month, day := t.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func measurementOutcomeSummary(action db.ContentAction, status string, completed, remaining int, now time.Time, window json.RawMessage) json.RawMessage {
	primaryMetric := ""
	windowData := map[string]any{}
	if len(window) > 0 && json.Valid(window) && json.Unmarshal(window, &windowData) == nil {
		primaryMetric, _ = windowData["primary_metric"].(string)
	}
	outcomeLabel := firstNonEmptyString(stringFromAny(windowData["outcome_label"]), stringFromAny(windowData["latest_outcome"]), measurement.OutcomeInsufficientData)
	outcomeReason := firstNonEmptyString(stringFromAny(windowData["outcome_reason"]), measurementInsufficientDataReason())
	confidence := firstNonEmptyString(stringFromAny(windowData["attribution_confidence"]), "low")
	confounders := firstNonNil(windowData["confounders"], measurementDefaultConfounders())
	terminalReason := strings.TrimSpace(fmt.Sprint(windowData["terminal_reason"]))
	if terminalReason == "<nil>" {
		terminalReason = ""
	}
	if status == "completed" && outcomeLabel == measurement.OutcomeInsufficientData {
		outcomeReason = "Measurement window closed, but comparative search or engagement data is still insufficient for reliable attribution."
		if terminalReason == "absolute_deadline_reached" {
			outcomeReason = "The immutable measurement deadline was reached; remaining checkpoints terminalized as insufficient data."
		}
	}
	return mustJSON(map[string]any{
		"action_id":                   action.ID.String(),
		"attribution_confidence":      confidence,
		"computed_at":                 now.Format(time.RFC3339),
		"completed_checkpoints":       completed,
		"confounders":                 confounders,
		"outcome_label":               outcomeLabel,
		"outcome_reason":              outcomeReason,
		"outcome_summary":             outcomeLabel,
		"primary_metric":              primaryMetric,
		"remaining_checkpoints":       remaining,
		"result":                      outcomeLabel,
		"state":                       outcomeLabel,
		"status":                      status,
		"summary":                     outcomeReason,
		"legacy_outcome_fallback":     "inconclusive",
		"measurement_terminal_reason": terminalReason,
	})
}

func measurementExecutionMetrics(action db.ContentAction, window map[string]any, checkpoint map[string]any) json.RawMessage {
	payload := map[string]any{
		"action_status":  action.Status,
		"checkpoint_day": checkpoint["day"],
		"primary_metric": window["primary_metric"],
		"source":         "measurement_window_due",
	}
	if action.VerifiedAt.Valid {
		payload["verified_at"] = action.VerifiedAt.Time.UTC().Format(time.RFC3339)
	}
	if action.PublishedAt.Valid {
		payload["published_at"] = action.PublishedAt.Time.UTC().Format(time.RFC3339)
	}
	return mustJSON(payload)
}

func measurementInsufficientDataReason() string {
	return "No comparable before/after search, engagement, or citation data was available for this checkpoint."
}

func measurementDefaultConfounders() []string {
	return []string{
		"Search Console or analytics data is missing for the checkpoint window.",
		"Ranking, traffic, and AI citation movement can be influenced by seasonality, competitor changes, or crawl latency.",
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
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
		if errors.Is(err, errDirectContentAction) {
			s.Log.Info("direct content action skipped from topic planning", "project", projectID, "action", action.ID)
			if _, releaseErr := tx.Exec(ctx, releaseSQL); releaseErr != nil {
				return releaseErr
			}
			continue
		}
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
	if !contentActionCreatesContent(action) {
		return db.Topic{}, fmt.Errorf("%w: %s", errDirectContentAction, action.ID)
	}
	opp, err := q.GetSEOOpportunity(ctx, db.GetSEOOpportunityParams{ID: action.OpportunityID, ProjectID: projectID})
	if err != nil {
		return db.Topic{}, err
	}
	topicParams := topicFromContentAction(projectID, action, opp)
	contracts, err := q.ListActivePlatformContentContracts(ctx)
	if err != nil {
		return db.Topic{}, err
	}
	planInput, err := platformcontract.LegacyPlanInput(platformcontract.PlanInput{
		ProjectID: projectID, OpportunityID: opp.ID, ContentActionID: action.ID,
		AssetType: firstNonEmpty(contentActionAssetType(action), "blog_post"),
	}, topicParams.Channel, contracts)
	if err != nil {
		return db.Topic{}, err
	}
	contexts, err := q.ListPlatformTargetContexts(ctx, db.ListPlatformTargetContextsParams{ProjectID: projectID, Platform: ""})
	if err != nil {
		return db.Topic{}, err
	}
	if err := platformcontract.ValidatePlanSelection(planInput, contracts, contexts, s.currentTime()); err != nil {
		return db.Topic{}, err
	}
	targetPlan, err := platformcontract.CreatePlan(ctx, q, planInput)
	if err != nil {
		return db.Topic{}, err
	}
	topicParams.Channel = platformcontract.DeriveChannel(targetPlan)
	topicParams.AssetType = ptr(targetPlan.AssetType)
	topicParams.TargetPlanID = pgtype.UUID{Bytes: targetPlan.ID, Valid: true}
	topic, err := q.CreateTopic(ctx, topicParams)
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

func contentActionCreatesContent(action db.ContentAction) bool {
	if action.WorkType != nil && strings.TrimSpace(*action.WorkType) == "improve_page" {
		return false
	}
	if strings.TrimSpace(contentActionAssetType(action)) == "page_update" {
		return false
	}
	if action.WorkType != nil && strings.TrimSpace(*action.WorkType) == "fix_site_issue" {
		return false
	}
	return contentActionNeedsTopic(contentActionAssetType(action), action.ActionType)
}

func contentActionNeedsTopic(assetType string, actionType string) bool {
	text := strings.ToLower(strings.TrimSpace(assetType + " " + actionType))
	switch {
	case strings.Contains(text, "page_update"):
		return false
	case strings.Contains(text, "internal_link_patch") || strings.Contains(text, "internal link"):
		return false
	case strings.Contains(text, "schema_patch") || strings.Contains(text, "schema patch"):
		return false
	case strings.Contains(text, "sitemap_update") || strings.Contains(text, "technical_fix") || strings.Contains(text, "technical seo"):
		return false
	case strings.Contains(text, "robots") || strings.Contains(text, "canonical") || strings.Contains(text, "crawler"):
		return false
	default:
		return true
	}
}

func contentActionAssetType(action db.ContentAction) string {
	if action.AssetType == nil {
		return ""
	}
	return strings.TrimSpace(*action.AssetType)
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
		Channel:               publishStrategyForContentAction(action, opp),
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

func publishStrategyForContentAction(action db.ContentAction, opp db.SeoOpportunity) string {
	for _, raw := range []json.RawMessage{action.InputSnapshot, action.OutputSnapshot, action.EvidenceSnapshot} {
		if strategy := snapshotPublishStrategy(raw); strategy != "" {
			return strategy
		}
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		contentActionAssetType(action),
		action.ActionType,
		stringPtrValue(action.WorkType),
		opp.Type,
		stringPtrValue(opp.RecommendedAction),
		stringPtrValue(opp.ExpectedImpact),
		stringPtrValue(opp.Query),
	}, " ")))
	switch {
	case strings.Contains(text, "community") || strings.Contains(text, "reddit") || strings.Contains(text, "dev.to") || strings.Contains(text, "hashnode") || strings.Contains(text, "distribution") || strings.Contains(text, "syndication"):
		if strings.Contains(text, "page") || strings.Contains(text, "guide") || strings.Contains(text, "comparison") || strings.Contains(text, "article") {
			return "both"
		}
		return "syndication"
	case action.WorkType != nil && strings.TrimSpace(*action.WorkType) == "improve_page":
		return "blog"
	case strings.Contains(text, "page_update") || strings.Contains(text, "metadata_rewrite") || strings.Contains(text, "refresh"):
		return "blog"
	case action.WorkType != nil && strings.TrimSpace(*action.WorkType) == "create_content":
		return "both"
	case strings.Contains(text, "comparison") || strings.Contains(text, "alternative") || strings.Contains(text, "guide") || strings.Contains(text, "glossary") || strings.Contains(text, "supporting section"):
		return "both"
	default:
		return "blog"
	}
}

func snapshotPublishStrategy(raw json.RawMessage) string {
	if len(raw) == 0 || !json.Valid(raw) {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return ""
	}
	for _, key := range []string{"publish_strategy", "publish_to", "content_destination_strategy", "channel"} {
		if strategy := normalizePublishStrategy(fmt.Sprint(data[key])); strategy != "" {
			return strategy
		}
	}
	return ""
}

func normalizePublishStrategy(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "_", " ")
	normalized = strings.ReplaceAll(normalized, "-", " ")
	switch normalized {
	case "blog", "source", "source article", "canonical", "owned site", "owned site article":
		return "blog"
	case "syndication", "syndicate", "distribution", "distribution draft":
		return "syndication"
	case "both", "blog and syndication", "blog + syndication", "source and distribution":
		return "both"
	default:
		return ""
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
		if err := s.enqueueScheduledOpportunityFinding(ctx, p); err != nil {
			s.logger().Error("enqueue scheduled opportunity finding failed", "project", p.ID, "err", err)
		}
	}
}

func (s *Scheduler) executeOpportunityFindingStage(
	ctx context.Context,
	q *db.Queries,
	runner seoRunner,
	p db.Project,
	cfg config.ProjectConfig,
	trigger config.GrowthAITrigger,
	observeRequest geo.ObserveAnswerProviderRequest,
	stage opportunityfinding.Stage,
	progress []opportunityfinding.StageProgress,
) opportunityfinding.StageOutcome {
	stages := cfg.OpportunityFindingStagesForTrigger(trigger)
	if !stages.SignalScan && !stages.AIDiscovery && stage != opportunityfinding.StageSummary {
		return opportunityfinding.StageOutcome{Status: "skipped", Summary: map[string]any{"reason": "disabled_by_project_authority"}}
	}
	switch stage {
	case opportunityfinding.StageEvidenceRefresh:
		if cfg.GrowthRadarMode == config.GrowthRadarOff {
			return opportunityfinding.StageOutcome{Status: "skipped", Summary: map[string]any{"reason": "growth_radar_off"}}
		}
		summary := map[string]any{}
		runErrors := make([]error, 0, 2)
		if stages.SignalScan {
			result, err := runner.Sync(ctx, p.ID, s.BlogBaseURL)
			summary["signal_scan"] = result
			if err != nil {
				runErrors = append(runErrors, fmt.Errorf("signal evidence refresh: %w", err))
			}
		} else {
			summary["signal_scan"] = map[string]any{"status": "skipped"}
		}
		if stages.AIDiscovery {
			comparator := (growthwork.ComparatorAuthority{Provider: s.LLM}).ForConfig(cfg, trigger)
			geoService := s.geoService(ctx, q, comparator)
			searchCollector := growthradar.SearchCollector{Provider: s.Search, Store: growthradar.DBSearchEvidenceStore{Q: q}}
			result, err := opportunityfinding.RefreshAIDiscoveryEvidence(ctx, p.ID, q, geoService, opportunityfinding.AIDiscoveryOptions{ObserveRequest: observeRequest, SearchCollector: &searchCollector, GrowthRadarMode: cfg.GrowthRadarMode})
			summary["ai_discovery"] = result
			if err == nil {
				err = opportunityFindingStepErrors(result.Errors)
			}
			if err != nil {
				runErrors = append(runErrors, fmt.Errorf("AI evidence refresh: %w", err))
			}
		} else {
			summary["ai_discovery"] = map[string]any{"status": "skipped"}
		}
		return opportunityfinding.StageOutcome{Summary: summary, Err: errors.Join(runErrors...)}
	case opportunityfinding.StageDeterministicSignals:
		if !stages.SignalScan {
			return opportunityfinding.StageOutcome{Status: "skipped", Summary: map[string]any{"reason": "growth_signal_disabled"}}
		}
		result, err := runner.Analyze(ctx, p.ID)
		return opportunityfinding.StageOutcome{Summary: map[string]any{"signal_scan": result}, Err: err}
	case opportunityfinding.StageAIHypotheses:
		if !stages.AIDiscovery {
			return opportunityfinding.StageOutcome{Status: "skipped", Summary: map[string]any{"reason": "growth_ai_not_authorized"}}
		}
		comparator := (growthwork.ComparatorAuthority{Provider: s.LLM}).ForConfig(cfg, trigger)
		geoService := s.geoService(ctx, q, comparator)
		result, err := opportunityfinding.MaterializeAIDiscoveryHypothesesWithMode(ctx, p.ID, geoService, cfg.GrowthRadarMode, q)
		if err == nil {
			err = opportunityFindingStepErrors(result.Errors)
		}
		return opportunityfinding.StageOutcome{Summary: map[string]any{"ai_discovery": result}, Err: err}
	case opportunityfinding.StageArbitration:
		err := runner.EnsureGrowthOpportunityReservations(ctx, p.ID)
		return opportunityfinding.StageOutcome{Summary: map[string]any{"canonical_reservations_checked": err == nil}, Err: err}
	case opportunityfinding.StageMaterialization:
		brief, err := runner.Brief(ctx, p.ID)
		return opportunityfinding.StageOutcome{Summary: map[string]any{
			"mode": brief.Mode, "actions": len(brief.Actions), "blockers": len(brief.Blockers), "geo_opportunities": len(brief.GEOOpportunities),
		}, Err: err}
	case opportunityfinding.StageSummary:
		counts := map[string]int{"succeeded": 0, "partial": 0, "failed": 0, "skipped": 0}
		for _, item := range progress {
			counts[item.Status]++
		}
		return opportunityfinding.StageOutcome{Summary: map[string]any{"stage_counts": counts, "trigger": trigger}}
	default:
		return opportunityfinding.StageOutcome{Err: fmt.Errorf("unknown Opportunity Finding stage %q", stage)}
	}
}

func (s *Scheduler) enqueueScheduledOpportunityFinding(ctx context.Context, project db.Project) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	lockName := workflowEventDedupeKey(workflow.EventOpportunityFindingRequested, project.ID)
	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock(hashtextextended($1, 0))", lockName); err != nil {
		return err
	}
	q := db.New(tx)
	if _, err := q.ActiveOpportunityFindingWorkflowEvent(ctx, project.ID); err == nil {
		return tx.Commit(ctx)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	requestID := uuid.New()
	payload, err := json.Marshal(map[string]any{"request_id": requestID, "trigger": config.GrowthAITriggerScheduled})
	if err != nil {
		return err
	}
	entityType := "project"
	now := s.currentTime()
	dateKey := now.UTC().Format("2006-01-02")
	if _, err := q.EnqueueWorkflowEvent(ctx, db.EnqueueWorkflowEventParams{
		ProjectID: project.ID, EventType: workflow.EventOpportunityFindingRequested,
		EntityType: &entityType, EntityID: pgtype.UUID{Bytes: project.ID, Valid: true},
		DedupeKey: workflowEventDedupeKey(workflow.EventOpportunityFindingRequested, project.ID, "scheduled", dateKey),
		Payload:   payload, RunAfter: pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func opportunityFindingStepErrors(stepErrors map[string]string) error {
	if len(stepErrors) == 0 {
		return nil
	}
	names := make([]string, 0, len(stepErrors))
	for name := range stepErrors {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+": "+stepErrors[name])
	}
	return errors.New(strings.Join(parts, "; "))
}

func (s *Scheduler) runSEOForProject(ctx context.Context, q *db.Queries, p db.Project, cfg config.ProjectConfig) error {
	return s.runSEOForProjectWithTrigger(ctx, q, p, cfg, config.GrowthAITriggerScheduled)
}

func (s *Scheduler) runSEOForProjectWithTrigger(ctx context.Context, q *db.Queries, p db.Project, cfg config.ProjectConfig, trigger config.GrowthAITrigger) error {
	comparator := (growthwork.ComparatorAuthority{Provider: s.LLM}).ForConfig(cfg, trigger)
	runner := s.newSEORunner(q, comparator)
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

// TickSEODoctor runs the user-facing technical SEO Doctor weekly for projects
// that do not already have a fresh onboarding/manual/weekly/post-publish report.
func (s *Scheduler) TickSEODoctor(ctx context.Context) {
	q := db.New(s.Pool)
	projects, err := q.ListSEODoctorRunsDueWeekly(ctx)
	if err != nil {
		s.logger().Error("list projects for seo doctor tick", "err", err)
		return
	}
	for _, p := range projects {
		if err := s.runSEODoctorForProject(ctx, q, p); err != nil {
			s.logger().Error("seo doctor tick failed", "project", p.ID, "err", err)
		}
	}
}

func (s *Scheduler) runSEODoctorForProject(ctx context.Context, q *db.Queries, p db.Project) error {
	runner := s.newSEORunner(q, nil)
	siteURL := s.BlogBaseURL
	if cfg, err := config.Parse(p.Config); err == nil && strings.TrimSpace(cfg.SiteURL) != "" {
		siteURL = cfg.SiteURL
	}
	run, created, err := runner.StartDoctorRun(ctx, seopkg.DoctorRunRequest{
		ProjectID: p.ID,
		Trigger:   seopkg.DoctorTriggerWeekly,
		SiteURL:   siteURL,
	})
	if err != nil {
		return fmt.Errorf("seo doctor start: %w", err)
	}
	if !created {
		s.logger().Info("seo doctor tick deduped active run", "project", p.ID, "run", run.ID)
		return nil
	}
	report, err := runner.RunDoctor(ctx, p.ID, run.ID)
	if err != nil {
		return fmt.Errorf("seo doctor run: %w", err)
	}
	s.logger().Info(
		"seo doctor tick complete",
		"project", p.ID,
		"run", run.ID,
		"health_score", report.Human.HealthScore,
		"findings", len(report.Findings),
	)
	return nil
}

func (s *Scheduler) newSEORunner(q *db.Queries, growthComparator discovery.SemanticComparator) seoRunner {
	if s.seoRunnerFactory != nil {
		return s.seoRunnerFactory(q)
	}
	return seopkg.Service{
		Q:                q,
		Pool:             s.Pool,
		HTTPClient:       s.httpClient,
		BlogBaseURL:      s.BlogBaseURL,
		GoogleData:       s.SEOData,
		LLM:              s.LLM,
		GrowthComparator: growthComparator,
		Now:              s.now,
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
	ag := agents.NewInsight(agents.Deps{Q: q, LLM: s.LLM, Search: s.Search, AICalls: q}, s.Log)
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
	writer := agents.NewWriter(agents.Deps{Q: q, LLM: s.LLM, Search: s.Search, AICalls: q, ArticleAssets: s.ArticleAssets}, s.Log)
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
	writer := agents.NewWriter(agents.Deps{Q: q, LLM: s.LLM, Search: s.Search, AICalls: q, ArticleAssets: s.ArticleAssets}, s.Log)
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

func (s *Scheduler) geoService(ctx context.Context, q *db.Queries, comparator discovery.SemanticComparator) geo.Service {
	return geo.Service{
		Q:              q,
		EvidenceStore:  q,
		AICallStore:    q,
		GrowthWriter:   growthwork.NewService(s.Pool, q, comparator),
		HTTPClient:     s.httpClient,
		AnswerProvider: s.geoAnswerProvider(ctx),
		Now:            s.currentTime,
	}
}

func (s *Scheduler) geoAnswerProvider(ctx context.Context) geo.AnswerProvider {
	if provider := s.adminGEOAnswerProvider(ctx); provider != nil {
		return provider
	}
	return s.GEOAnswerProvider
}

func (s *Scheduler) adminGEOAnswerProvider(ctx context.Context) geo.AnswerProvider {
	if s.Pool == nil {
		return nil
	}
	credentials, err := admin.LoadRuntimeGEOCredentials(ctx, s.Pool)
	if err != nil {
		s.logger().Warn("admin GEO credential unavailable", "err", err)
		return nil
	}
	if credentials == nil {
		llmCredentials, err := admin.LoadCredentials(ctx, s.Pool)
		if err != nil {
			s.logger().Warn("admin LLM credential unavailable for GEO fallback", "err", err)
			return nil
		}
		if llmCredentials == nil || strings.TrimSpace(llmCredentials.APIKey) == "" {
			return nil
		}
		env := config.FromEnv()
		baseURL := strings.TrimSpace(llmCredentials.BaseURL)
		if baseURL == "" {
			baseURL = env.TokenGateBaseURL
		}
		model := strings.TrimSpace(llmCredentials.Model)
		if model == "" {
			model = env.TokenGateModel
		}
		return geo.NewTokenGateAnswerProvider(geo.TokenGateAnswerProviderConfig{
			Scope:   string(admin.GEOProviderOpenAI),
			APIKey:  llmCredentials.APIKey,
			BaseURL: baseURL,
			Model:   model,
			Engine:  admin.GEOEngineForScope(admin.GEOProviderOpenAI),
		}, nil)
	}
	return geo.NewTokenGateAnswerProvider(geo.TokenGateAnswerProviderConfig{
		Scope:   string(credentials.Scope),
		APIKey:  credentials.APIKey,
		BaseURL: credentials.BaseURL,
		Model:   credentials.Model,
		Engine:  admin.GEOEngineForScope(credentials.Scope),
	}, nil)
}

func (s *Scheduler) geoObserveRequest() geo.ObserveAnswerProviderRequest {
	budgetUSD := s.GEOProviderRunBudgetUSD
	if budgetUSD <= 0 {
		budgetUSD = 1
	}
	return geo.ObserveAnswerProviderRequest{
		Engine:     "OpenAI",
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

// TickSiteFixReconcile polls open source-backed PRs and advances the apply
// ledger when GitHub reports a merge (or a close without merge), so an operator
// no longer has to tell CiteLoop that the PR landed. This is the merge-detection
// half of the site-fix auto-verification loop; production verification runs
// separately once the change is merged.
func (s *Scheduler) TickSiteFixReconcile(ctx context.Context) {
	q := db.New(s.Pool)
	projects, err := q.ListProjects(ctx)
	if err != nil {
		s.Log.Error("list projects", "err", err)
		return
	}
	for _, p := range projects {
		if err := s.reconcileSiteChangePRsForProject(ctx, q, p); err != nil {
			s.Log.Error("site fix PR reconcile failed", "project", p.ID, "err", err)
		}
		if err := s.reconcileSiteChangeVerificationForProject(ctx, q, p); err != nil {
			s.Log.Error("site fix verification reconcile failed", "project", p.ID, "err", err)
		}
	}
}

func (s *Scheduler) reconcileSiteChangePRsForProject(ctx context.Context, q *db.Queries, p db.Project) error {
	canonicalApps, err := q.ListCanonicalSiteFixPRsForReconciliation(ctx, p.ID)
	if err != nil {
		return err
	}
	legacyApps, err := q.ListOpenSiteChangePRApplications(ctx, p.ID)
	if err != nil {
		return err
	}
	apps := append(canonicalApps, legacyApps...)
	if len(apps) == 0 {
		return nil
	}
	token, err := s.githubTokenForProject(ctx, q, p)
	if err != nil {
		return err
	}
	if strings.TrimSpace(token) == "" {
		// No usable GitHub auth (connection removed or App unconfigured); leave the
		// open PRs for a later tick instead of failing the whole project.
		return nil
	}
	now := s.currentTime()
	for _, app := range apps {
		if app.GithubPrNumber == nil {
			continue
		}
		repo := strings.TrimSpace(stringPtrValue(app.RepoFullName))
		if repo == "" {
			s.Log.Warn("site fix application missing repo; cannot poll PR", "project", p.ID, "application", app.ID)
			continue
		}
		created := siteFixPRCreatedAt(app)
		elapsed := now.Sub(created)

		// Give up after the horizon: flag for the operator and stop polling/nagging.
		if elapsed >= siteFixPRGiveUp {
			if err := s.markSiteChangePRNeedsFollowUp(ctx, q, p, app, "pull request not merged within 14 days"); err != nil {
				s.Log.Error("mark site fix PR needs follow-up", "project", p.ID, "application", app.ID, "err", err)
			}
			continue
		}

		// Merge check is gated by the Fibonacci-backoff schedule so an unmerged PR
		// is polled densely right after creation and sparsely later.
		if siteFixTimeDue(app.NextPollAt, now) {
			base := strings.TrimSpace(stringPtrValue(app.BaseBranch))
			pr, err := publisher.NewGitHubPRClient(token, repo, base, s.Log).GetPullRequest(ctx, int(*app.GithubPrNumber))
			if err != nil {
				s.Log.Warn("read site fix PR state", "project", p.ID, "application", app.ID, "err", err)
			} else if pr.Merged {
				if err := s.markSiteChangePRMerged(ctx, q, p, app, pr); err != nil {
					s.Log.Error("mark site fix PR merged", "project", p.ID, "application", app.ID, "err", err)
				}
				continue
			} else if strings.EqualFold(pr.State, "closed") {
				if err := s.markSiteChangePRClosed(ctx, q, p, app); err != nil {
					s.Log.Error("mark site fix PR closed", "project", p.ID, "application", app.ID, "err", err)
				}
				continue
			}
			// Still open (or a transient read error): schedule the next poll so we
			// back off instead of hammering GitHub every tick.
			if next, ok := nextSiteFixPollAt(created, now); ok {
				if err := q.SetSiteChangePRNextPollAt(ctx, db.SetSiteChangePRNextPollAtParams{
					ID:         app.ID,
					ProjectID:  p.ID,
					NextPollAt: pgutil.TS(next),
				}); err != nil {
					s.Log.Error("schedule next site fix PR poll", "project", p.ID, "application", app.ID, "err", err)
				}
			}
		}

		// Nag the operator on its own cadence (every 12h, then daily after 3 days).
		if siteFixTimeDue(app.NextNotifyAt, now) {
			s.dispatchSiteFixPRAwaitingMerge(ctx, q, p, app, elapsed)
			if next, ok := nextSiteFixNotifyAt(created, now); ok {
				if err := q.SetSiteChangePRNextNotifyAt(ctx, db.SetSiteChangePRNextNotifyAtParams{
					ID:           app.ID,
					ProjectID:    p.ID,
					NextNotifyAt: pgutil.TS(next),
				}); err != nil {
					s.Log.Error("schedule next site fix PR nag", "project", p.ID, "application", app.ID, "err", err)
				}
			}
		}
	}
	return nil
}

// siteFixPRPollCheckpoints are cumulative delays from PR creation at which the
// merge poll runs — a Fibonacci-like backoff (5,10,15,25,40,65,… minutes) that
// stays dense right after creation, when a merge is most likely, and thins out
// over the first ~3 days. After the last checkpoint the poll falls back to daily.
var siteFixPRPollCheckpoints = []time.Duration{
	5 * time.Minute, 10 * time.Minute, 15 * time.Minute, 25 * time.Minute, 40 * time.Minute,
	65 * time.Minute, 105 * time.Minute, 170 * time.Minute, 275 * time.Minute, 445 * time.Minute,
	720 * time.Minute, 1165 * time.Minute, 1885 * time.Minute, 3050 * time.Minute,
}

const (
	// siteFixPRGiveUp is when an unmerged PR is flagged needs_follow_up and both
	// polling and nagging stop.
	siteFixPRGiveUp = 14 * 24 * time.Hour
	// siteFixPRFibonacciWindow bounds the dense-nag phase; after it the nag drops
	// from every 12h to daily.
	siteFixPRFibonacciWindow = 3 * 24 * time.Hour
	siteFixPRDailyInterval   = 24 * time.Hour
	siteFixPRNagInterval     = 12 * time.Hour
)

// nextSiteFixPollAt returns the next merge-poll time for a PR created at
// prCreatedAt, or ok=false once the give-up horizon has passed.
func nextSiteFixPollAt(prCreatedAt, now time.Time) (time.Time, bool) {
	elapsed := now.Sub(prCreatedAt)
	if elapsed >= siteFixPRGiveUp {
		return time.Time{}, false
	}
	for _, cp := range siteFixPRPollCheckpoints {
		if cp > elapsed {
			return prCreatedAt.Add(cp), true
		}
	}
	return now.Add(siteFixPRDailyInterval), true
}

// nextSiteFixNotifyAt returns the next nag time: every 12h for the first 3 days,
// daily after, and ok=false once the give-up horizon has passed.
func nextSiteFixNotifyAt(prCreatedAt, now time.Time) (time.Time, bool) {
	elapsed := now.Sub(prCreatedAt)
	if elapsed >= siteFixPRGiveUp {
		return time.Time{}, false
	}
	if elapsed < siteFixPRFibonacciWindow {
		return now.Add(siteFixPRNagInterval), true
	}
	return now.Add(siteFixPRDailyInterval), true
}

func siteFixTimeDue(at pgtype.Timestamptz, now time.Time) bool {
	return !at.Valid || !now.Before(at.Time)
}

func siteFixPRCreatedAt(app db.SiteChangeApplication) time.Time {
	if app.PrCreatedAt.Valid {
		return app.PrCreatedAt.Time
	}
	if app.CreatedAt.Valid {
		return app.CreatedAt.Time
	}
	return app.UpdatedAt.Time
}

func (s *Scheduler) markSiteChangePRNeedsFollowUp(ctx context.Context, q *db.Queries, p db.Project, app db.SiteChangeApplication, reason string) error {
	if app.SiteFixID.Valid && !app.ContentActionID.Valid {
		_, err := q.MarkCanonicalSiteFixApplyFailure(ctx, db.MarkCanonicalSiteFixApplyFailureParams{
			ProjectID: p.ID, SiteFixID: app.SiteFixID, ApplicationID: app.ID,
			FailureReason: &reason,
		})
		return err
	}
	contentActionID, err := legacyApplicationContentActionID(app)
	if err != nil {
		return err
	}
	if _, err := q.MarkSiteChangeApplicationStatus(ctx, db.MarkSiteChangeApplicationStatusParams{
		ID:                   app.ID,
		ProjectID:            p.ID,
		Status:               "needs_follow_up",
		DeploymentSnapshot:   json.RawMessage(`{}`),
		VerificationSnapshot: json.RawMessage(`{}`),
		FailureReason:        &reason,
	}); err != nil {
		return err
	}
	result := json.RawMessage(mustJSON(map[string]any{
		"mode":                       "github_pr",
		"status":                     "needs_follow_up",
		"site_change_application_id": app.ID,
		"github_pr_number":           app.GithubPrNumber,
		"github_pr_url":              app.GithubPrUrl,
		"github_pr_state":            stringPtrValue(app.GithubPrState),
		"repo":                       app.RepoFullName,
		"base_branch":                app.BaseBranch,
		"target_url":                 app.TargetUrl,
		"follow_up_reason":           reason,
	}))
	if _, err := q.MarkContentActionSiteFixPRResult(ctx, db.MarkContentActionSiteFixPRResultParams{
		ID:              contentActionID,
		ProjectID:       p.ID,
		PublisherResult: result,
	}); err != nil {
		return err
	}
	s.Log.Info("site fix PR flagged needs follow-up (unmerged 14d)", "project", p.ID, "application", app.ID, "pr_url", stringPtrValue(app.GithubPrUrl))
	return nil
}

func (s *Scheduler) dispatchSiteFixPRAwaitingMerge(ctx context.Context, store notification.DispatchStore, p db.Project, app db.SiteChangeApplication, elapsed time.Duration) {
	prNumber := 0
	if app.GithubPrNumber != nil {
		prNumber = int(*app.GithubPrNumber)
	}
	ageHours := int(elapsed / time.Hour)
	if ageHours < 0 {
		ageHours = 0
	}
	event := notification.NewSiteFixPRAwaitingMergeEvent(
		p.ID,
		app.ID,
		stringPtrValue(app.GithubPrUrl),
		prNumber,
		ageHours,
		s.currentTime(),
		"/projects/"+p.ID.String()+"/seo",
	)
	s.dispatchNotification(ctx, store, event)
}

// githubTokenForProject resolves a usable GitHub token for the project's enabled
// publisher connection, preferring a GitHub App installation token and falling
// back to a stored PAT. It returns an empty token (no error) when the project
// has no enabled connection, so callers can skip silently.
func (s *Scheduler) githubTokenForProject(ctx context.Context, q publisherConnectionQuerier, p db.Project) (string, error) {
	conn, err := q.GetEnabledPublisherConnectionForProject(ctx, db.GetEnabledPublisherConnectionForProjectParams{
		ProjectID: p.ID,
		Kind:      publisher.ConnectionKindGitHubNextJS,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	token, err := s.githubInstallationToken(ctx, conn)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(token) == "" {
		token, err = s.publisherCredentialToken(ctx, q, conn)
		if err != nil {
			return "", err
		}
	}
	return token, nil
}

func (s *Scheduler) markSiteChangePRMerged(ctx context.Context, q *db.Queries, p db.Project, app db.SiteChangeApplication, pr publisher.GitHubPRState) error {
	if app.SiteFixID.Valid && !app.ContentActionID.Valid {
		mergedAt := s.currentTime()
		if pr.MergedAt != nil {
			mergedAt = pr.MergedAt.UTC()
		}
		if _, err := q.MarkCanonicalSiteFixPRMerged(ctx, db.MarkCanonicalSiteFixPRMergedParams{
			SiteFixID: uuid.UUID(app.SiteFixID.Bytes), ProjectID: p.ID,
			ApplicationID: app.ID, ObservedMergedAt: pgutil.TS(mergedAt),
		}); err != nil {
			return err
		}
		s.Log.Info("canonical Doctor Site Fix PR merged; awaiting deployment", "project", p.ID, "site_fix", uuid.UUID(app.SiteFixID.Bytes), "application", app.ID)
		return nil
	}
	contentActionID, err := legacyApplicationContentActionID(app)
	if err != nil {
		return err
	}
	merged := "merged"
	if _, err := q.MarkSiteChangeApplicationStatus(ctx, db.MarkSiteChangeApplicationStatusParams{
		ID:                   app.ID,
		ProjectID:            p.ID,
		Status:               "github_pr_merged",
		GithubPrState:        &merged,
		DeploymentSnapshot:   json.RawMessage(`{}`),
		VerificationSnapshot: json.RawMessage(`{}`),
	}); err != nil {
		return err
	}
	result := json.RawMessage(mustJSON(map[string]any{
		"mode":                       "github_pr",
		"status":                     "github_pr_merged",
		"site_change_application_id": app.ID,
		"github_pr_number":           app.GithubPrNumber,
		"github_pr_url":              app.GithubPrUrl,
		"github_pr_state":            "merged",
		"merged_at":                  pr.MergedAt,
		"repo":                       app.RepoFullName,
		"base_branch":                app.BaseBranch,
		"target_url":                 app.TargetUrl,
	}))
	if _, err := q.MarkContentActionSiteFixPRResult(ctx, db.MarkContentActionSiteFixPRResultParams{
		ID:              contentActionID,
		ProjectID:       p.ID,
		PublisherResult: result,
	}); err != nil {
		return err
	}
	// Reuse next_poll_at for the post-merge verification cadence: wait out a short
	// deploy grace before the first production check.
	if err := q.SetSiteChangePRNextPollAt(ctx, db.SetSiteChangePRNextPollAtParams{
		ID:         app.ID,
		ProjectID:  p.ID,
		NextPollAt: pgutil.TS(s.currentTime().Add(siteFixDeployGrace)),
	}); err != nil {
		s.Log.Error("schedule post-merge verification", "project", p.ID, "application", app.ID, "err", err)
	}
	s.Log.Info("site fix PR merged", "project", p.ID, "application", app.ID, "pr_url", stringPtrValue(app.GithubPrUrl))
	return nil
}

func (s *Scheduler) markSiteChangePRClosed(ctx context.Context, q *db.Queries, p db.Project, app db.SiteChangeApplication) error {
	if app.SiteFixID.Valid && !app.ContentActionID.Valid {
		reason := "pull request was closed without merging"
		closedState := "closed"
		_, err := q.MarkCanonicalSiteFixApplyFailure(ctx, db.MarkCanonicalSiteFixApplyFailureParams{
			ProjectID: p.ID, SiteFixID: app.SiteFixID, ApplicationID: app.ID,
			ObservedGithubPrState: &closedState, FailureReason: &reason,
		})
		return err
	}
	contentActionID, err := legacyApplicationContentActionID(app)
	if err != nil {
		return err
	}
	closedState := "closed"
	reason := "pull request was closed without merging"
	if _, err := q.MarkSiteChangeApplicationStatus(ctx, db.MarkSiteChangeApplicationStatusParams{
		ID:                   app.ID,
		ProjectID:            p.ID,
		Status:               "github_pr_closed",
		GithubPrState:        &closedState,
		DeploymentSnapshot:   json.RawMessage(`{}`),
		VerificationSnapshot: json.RawMessage(`{}`),
		FailureReason:        &reason,
	}); err != nil {
		return err
	}
	result := json.RawMessage(mustJSON(map[string]any{
		"mode":                       "github_pr",
		"status":                     "github_pr_closed",
		"site_change_application_id": app.ID,
		"github_pr_number":           app.GithubPrNumber,
		"github_pr_url":              app.GithubPrUrl,
		"github_pr_state":            "closed",
		"repo":                       app.RepoFullName,
		"base_branch":                app.BaseBranch,
		"target_url":                 app.TargetUrl,
	}))
	if _, err := q.MarkContentActionSiteFixPRResult(ctx, db.MarkContentActionSiteFixPRResultParams{
		ID:              contentActionID,
		ProjectID:       p.ID,
		PublisherResult: result,
	}); err != nil {
		return err
	}
	s.Log.Info("site fix PR closed without merge", "project", p.ID, "application", app.ID, "pr_url", stringPtrValue(app.GithubPrUrl))
	return nil
}

func legacyApplicationContentActionID(app db.SiteChangeApplication) (uuid.UUID, error) {
	if !app.ContentActionID.Valid {
		return uuid.Nil, fmt.Errorf("legacy Site Change application %s has no Content Action source", app.ID)
	}
	return uuid.UUID(app.ContentActionID.Bytes), nil
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
		assets, assetErr := q.ListArticleAssetsForArticle(ctx, db.ListArticleAssetsForArticleParams{ProjectID: p.ID, ArticleID: a.ID})
		if assetErr != nil {
			s.Log.Warn("article assets unavailable; publishing text only", "article", a.ID, "err", assetErr)
			assets = nil
		}
		res, err := blog.PublishWithAssets(ctx, &a, assets)
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
		newPlatformMetadata := []byte(publisher.RewriteForDistribution(string(v.PlatformMetadata), realURL))
		if _, err := q.UnlockVariant(ctx, db.UnlockVariantParams{
			ID: v.ID, CanonicalUrl: &realURL, ContentMd: newContent, SeoMeta: newSEO, PlatformMetadata: newPlatformMetadata,
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
