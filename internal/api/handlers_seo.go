package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/aicalls"
	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/citeloop/citeloop/internal/growthwork"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/opportunityfinding"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/platformcontract"
	"github.com/citeloop/citeloop/internal/publisher"
	seopkg "github.com/citeloop/citeloop/internal/seo"
	"github.com/citeloop/citeloop/internal/topicstate"
	"github.com/citeloop/citeloop/internal/workflow"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) seoService() seopkg.Service {
	return seopkg.Service{Q: s.Q, Pool: s.Pool, BlogBaseURL: s.Env.BlogBaseURL, GoogleData: s.SEOData, LLM: s.LLM, DoctorAIModel: s.Env.TokenGateModel}
}

func (s *Server) seoServiceForProject(ctx context.Context, projectID uuid.UUID) seopkg.Service {
	svc := s.seoService()
	if provider, override := s.googleDataProviderForProject(ctx, projectID); override {
		svc.GoogleData = provider
	}
	return svc
}

func (s *Server) growthComparatorForProject(ctx context.Context, projectID uuid.UUID, trigger config.GrowthAITrigger) (discovery.SemanticComparator, error) {
	project, err := s.Q.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	projectConfig, err := config.Parse(project.Config)
	if err != nil {
		return nil, err
	}
	return (growthwork.ComparatorAuthority{Provider: s.LLM, Model: s.Env.TokenGateModel}).ForConfig(projectConfig, trigger), nil
}

func (s *Server) seoServiceWithGrowthAuthority(ctx context.Context, projectID uuid.UUID, trigger config.GrowthAITrigger) (seopkg.Service, error) {
	svc := s.seoServiceForProject(ctx, projectID)
	comparator, err := s.growthComparatorForProject(ctx, projectID, trigger)
	if err != nil {
		return seopkg.Service{}, err
	}
	svc.GrowthComparator = comparator
	return svc, nil
}

func (s *Server) getSEOOverview(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	overview, err := s.seoService().Overview(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, overview)
}

type VisibilityLifecycleStage string

const (
	VisibilityStageDetected           VisibilityLifecycleStage = "detected"
	VisibilityStageAddedToPlan        VisibilityLifecycleStage = "added_to_plan"
	VisibilityStagePlanned            VisibilityLifecycleStage = "planned"
	VisibilityStageDrafting           VisibilityLifecycleStage = "drafting"
	VisibilityStageReadyForReview     VisibilityLifecycleStage = "ready_for_review"
	VisibilityStageApproved           VisibilityLifecycleStage = "approved"
	VisibilityStagePublishedOrApplied VisibilityLifecycleStage = "published_or_applied"
	VisibilityStageMeasuring          VisibilityLifecycleStage = "measuring"
	VisibilityStageLearned            VisibilityLifecycleStage = "learned"
	VisibilityStageBlocked            VisibilityLifecycleStage = "blocked"
)

var visibilityLifecycleStages = []VisibilityLifecycleStage{
	VisibilityStageDetected,
	VisibilityStageAddedToPlan,
	VisibilityStagePlanned,
	VisibilityStageDrafting,
	VisibilityStageReadyForReview,
	VisibilityStageApproved,
	VisibilityStagePublishedOrApplied,
	VisibilityStageMeasuring,
	VisibilityStageLearned,
	VisibilityStageBlocked,
}

type VisibilitySummary struct {
	CapabilityMode        string                      `json:"capability_mode"`
	PrimaryStatus         string                      `json:"primary_status"`
	SetupBlockers         []seopkg.SetupChecklistItem `json:"setup_blockers"`
	OpenOpportunities     []db.SeoOpportunity         `json:"open_opportunities"`
	ActionsInLoop         []VisibilityActionInLoop    `json:"actions_in_loop"`
	LifecycleCounts       map[string]int              `json:"lifecycle_counts"`
	TopMeasurementUpdates []VisibilityMeasurement     `json:"top_measurement_updates"`
	DiagnosticsHealth     map[string]any              `json:"diagnostics_health"`
}

type VisibilityActionInLoop struct {
	ID                        uuid.UUID                `json:"id"`
	OpportunityID             uuid.UUID                `json:"opportunity_id"`
	ActionType                string                   `json:"action_type"`
	Status                    string                   `json:"status"`
	LifecycleStage            VisibilityLifecycleStage `json:"lifecycle_stage"`
	AssetType                 *string                  `json:"asset_type,omitempty"`
	TargetURL                 *string                  `json:"target_url,omitempty"`
	NormalizedTargetURL       *string                  `json:"normalized_target_url,omitempty"`
	OpportunityStatus         string                   `json:"opportunity_status"`
	OpportunityType           string                   `json:"opportunity_type"`
	OpportunityPageURL        *string                  `json:"opportunity_page_url,omitempty"`
	OpportunityNormalizedURL  *string                  `json:"opportunity_normalized_page_url,omitempty"`
	OpportunityQuery          *string                  `json:"opportunity_query,omitempty"`
	OpportunityRecommended    *string                  `json:"opportunity_recommended_action,omitempty"`
	OpportunityExpectedImpact *string                  `json:"opportunity_expected_impact,omitempty"`
	OpportunityRiskLevel      string                   `json:"opportunity_risk_level"`
	TopicID                   *uuid.UUID               `json:"topic_id,omitempty"`
	TopicStatus               *string                  `json:"topic_status,omitempty"`
	TopicTitle                *string                  `json:"topic_title,omitempty"`
	DraftArticleID            *uuid.UUID               `json:"draft_article_id,omitempty"`
	DraftArticleStatus        *string                  `json:"draft_article_status,omitempty"`
	DraftArticleCanonicalURL  *string                  `json:"draft_article_canonical_url,omitempty"`
	ReviewRequired            bool                     `json:"review_required"`
	ApprovalSource            string                   `json:"approval_source"`
	RoutingSource             string                   `json:"routing_source"`
	WorkType                  *string                  `json:"work_type,omitempty"`
	PublishedAt               *time.Time               `json:"published_at,omitempty"`
	VerifiedAt                *time.Time               `json:"verified_at,omitempty"`
	MeasurementWindow         json.RawMessage          `json:"measurement_window"`
	MeasurementPolicyVersion  string                   `json:"measurement_policy_version"`
	MeasurementPolicy         json.RawMessage          `json:"measurement_policy"`
	MeasuringStartedAt        *time.Time               `json:"measuring_started_at,omitempty"`
	AbsoluteTerminalAt        *time.Time               `json:"absolute_terminal_at,omitempty"`
	MeasurementTerminalReason *string                  `json:"measurement_terminal_reason,omitempty"`
	OutcomeSummary            json.RawMessage          `json:"outcome_summary"`
	VerificationSnapshot      json.RawMessage          `json:"verification_snapshot"`
	CreatedAt                 *time.Time               `json:"created_at,omitempty"`
	UpdatedAt                 *time.Time               `json:"updated_at,omitempty"`
}

type VisibilityMeasurement struct {
	ActionID uuid.UUID `json:"action_id"`
	Status   string    `json:"status"`
	Summary  string    `json:"summary"`
}

// ActionMeasurement fields expose outcome_label, outcome_reason,
// attribution_confidence, and confounders for Results attribution.
type ActionMeasurement = db.ActionMeasurement

type ResultsAction struct {
	db.ContentAction
	OpportunityType              string              `json:"opportunity_type"`
	OpportunityQuery             *string             `json:"opportunity_query,omitempty"`
	OpportunityPageURL           *string             `json:"opportunity_page_url,omitempty"`
	OpportunityNormalizedURL     *string             `json:"opportunity_normalized_page_url,omitempty"`
	OpportunityRecommendedAction *string             `json:"opportunity_recommended_action,omitempty"`
	OpportunityExpectedImpact    *string             `json:"opportunity_expected_impact,omitempty"`
	TopicTitle                   *string             `json:"topic_title,omitempty"`
	DraftArticleStatus           *string             `json:"draft_article_status,omitempty"`
	DraftArticleCanonicalURL     *string             `json:"draft_article_canonical_url,omitempty"`
	LatestMeasurement            *ActionMeasurement  `json:"latest_measurement,omitempty"`
	Measurements                 []ActionMeasurement `json:"measurements"`
}

type GrowthLearningFeedItem struct {
	ID                       uuid.UUID       `json:"id"`
	ProjectID                uuid.UUID       `json:"project_id"`
	OpportunityID            uuid.UUID       `json:"opportunity_id"`
	ContentActionID          uuid.UUID       `json:"content_action_id"`
	ArticleID                *uuid.UUID      `json:"article_id,omitempty"`
	ArtifactURL              string          `json:"artifact_url,omitempty"`
	RecordKind               string          `json:"record_kind"`
	LearningSummary          string          `json:"learning_summary"`
	Applicability            json.RawMessage `json:"applicability"`
	ScoringEligible          bool            `json:"scoring_eligible"`
	LearningVersion          string          `json:"learning_version"`
	ActionFamily             string          `json:"action_family"`
	TargetIdentity           json.RawMessage `json:"target_identity"`
	Audience                 json.RawMessage `json:"audience"`
	PrimaryMetric            string          `json:"primary_metric"`
	OutcomeLabel             string          `json:"outcome_label"`
	TerminalReason           string          `json:"terminal_reason"`
	MeasurementPolicyVersion string          `json:"measurement_policy_version"`
	BaselineSnapshot         json.RawMessage `json:"baseline_snapshot"`
	CheckpointSnapshot       json.RawMessage `json:"checkpoint_snapshot"`
	OutcomeSnapshot          json.RawMessage `json:"outcome_snapshot"`
	DataQualityState         string          `json:"data_quality_state,omitempty"`
	QualityGaps              json.RawMessage `json:"quality_gaps,omitempty"`
	Recommendation           string          `json:"recommendation,omitempty"`
	CreatedAt                *time.Time      `json:"created_at,omitempty"`
}

type OpportunityFindingStatus struct {
	GrowthSignalEnabled bool                            `json:"growth_signal_enabled"`
	GrowthAIEnabled     bool                            `json:"growth_ai_enabled"`
	GrowthAIRunPolicy   string                          `json:"growth_ai_run_policy"`
	ManualMode          bool                            `json:"manual_mode"`
	LastRun             *OpportunityFindingRun          `json:"last_run,omitempty"`
	NextFindingAt       *time.Time                      `json:"next_finding_at,omitempty"`
	Summary             []OpportunityFindingSummaryItem `json:"summary"`
	Counts              OpportunityFindingCounts        `json:"counts"`
}

type OpportunityFindingRun struct {
	ID              uuid.UUID                         `json:"id"`
	Status          string                            `json:"status"`
	StartedAt       *time.Time                        `json:"started_at,omitempty"`
	FinishedAt      *time.Time                        `json:"finished_at,omitempty"`
	DurationMs      int64                             `json:"duration_ms"`
	Error           *string                           `json:"error,omitempty"`
	StageProgress   []OpportunityFindingStageProgress `json:"stage_progress,omitempty"`
	ProgressPercent int                               `json:"progress_percent"`
	CurrentStage    string                            `json:"current_stage,omitempty"`
}

type OpportunityFindingStageProgress struct {
	Stage              string         `json:"stage"`
	Order              int16          `json:"order"`
	Status             string         `json:"status"`
	AttemptNumber      int32          `json:"attempt_number"`
	RequestFingerprint string         `json:"request_fingerprint"`
	Summary            map[string]any `json:"summary"`
	Error              *string        `json:"error,omitempty"`
}

type OpportunityFindingSummaryItem struct {
	Label  string `json:"label"`
	Detail string `json:"detail"`
	Tone   string `json:"tone"`
}

type OpportunityFindingCounts struct {
	Open      int            `json:"open"`
	Processed int            `json:"processed"`
	InLoop    int            `json:"in_loop"`
	Total     int            `json:"total"`
	ByStatus  map[string]int `json:"by_status"`
}

type opportunityFindingRunOutput struct {
	GeneratedAnomalies int      `json:"generated_anomalies"`
	DataSourceNotes    []string `json:"data_source_notes"`
	CheckedURLs        int      `json:"checked_urls"`
	ConnectedGSC       bool     `json:"connected_gsc"`
	ColdStart          bool     `json:"cold_start"`
}

func (s *Server) getVisibilitySummary(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	overview, err := s.seoService().Overview(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	openOpps, err := s.Q.ListSEOOpportunities(r.Context(), db.ListSEOOpportunitiesParams{
		ProjectID: projectID,
		Status:    "open",
		LimitRows: 50,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	rows, err := s.Q.ListVisibilityActionRows(r.Context(), db.ListVisibilityActionRowsParams{
		ProjectID: projectID,
		Limit:     50,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	counts := emptyVisibilityLifecycleCounts()
	counts[string(VisibilityStageDetected)] = len(openOpps)
	actions := make([]VisibilityActionInLoop, 0, len(rows))
	measurements := []VisibilityMeasurement{}
	for _, row := range rows {
		stage := deriveVisibilityLifecycleStage(row)
		counts[string(stage)]++
		item := visibilityActionInLoop(row, stage)
		actions = append(actions, item)
		if measurement := visibilityMeasurementUpdate(item); measurement != nil && len(measurements) < 5 {
			measurements = append(measurements, *measurement)
		}
	}

	setupBlockers := make([]seopkg.SetupChecklistItem, 0)
	for _, item := range overview.SetupChecklist {
		if item.Status == "connected" || item.Status == "optional" {
			continue
		}
		setupBlockers = append(setupBlockers, item)
	}
	summary := VisibilitySummary{
		CapabilityMode:        overview.CapabilityMode,
		PrimaryStatus:         visibilityPrimaryStatus(len(openOpps), len(actions), setupBlockers, counts),
		SetupBlockers:         setupBlockers,
		OpenOpportunities:     emptySlice(openOpps),
		ActionsInLoop:         actions,
		LifecycleCounts:       counts,
		TopMeasurementUpdates: measurements,
		DiagnosticsHealth: map[string]any{
			"capability_mode":      overview.CapabilityMode,
			"cold_start":           overview.ColdStart,
			"data_source_warnings": overview.DataSourceWarnings,
			"technical":            overview.Technical,
		},
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) getOpportunityFindingStatus(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	status, err := s.opportunityFindingStatus(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) runOpportunityFinding(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	_, err = s.Q.GetProject(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.enqueueOpportunityFindingWorkflowEvent(r.Context(), projectID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	status, err := s.opportunityFindingStatus(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"status": status})
}

func (s *Server) enqueueOpportunityFindingWorkflowEvent(ctx context.Context, projectID uuid.UUID) error {
	if s.Pool == nil {
		return errors.New("database unavailable")
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock(hashtextextended($1, 0))", workflowDedupeKey(workflow.EventOpportunityFindingRequested, projectID)); err != nil {
		return err
	}
	q := db.New(tx)
	if _, err := q.ActiveOpportunityFindingWorkflowEvent(ctx, projectID); err == nil {
		return tx.Commit(ctx)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	requestID := uuid.New()
	payload, err := json.Marshal(map[string]any{"request_id": requestID, "trigger": string(config.GrowthAITriggerManual)})
	if err != nil {
		return err
	}
	entityType := "project"
	if _, err := q.EnqueueWorkflowEvent(ctx, db.EnqueueWorkflowEventParams{
		ProjectID: projectID, EventType: workflow.EventOpportunityFindingRequested,
		EntityType: &entityType, EntityID: pgtype.UUID{Bytes: projectID, Valid: true},
		DedupeKey: workflowDedupeKey(workflow.EventOpportunityFindingRequested, projectID, requestID),
		Payload:   payload, RunAfter: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Server) opportunityFindingStatus(ctx context.Context, projectID uuid.UUID) (OpportunityFindingStatus, error) {
	project, err := s.Q.GetProject(ctx, projectID)
	if err != nil {
		return OpportunityFindingStatus{}, err
	}
	cfg, err := config.Parse(project.Config)
	if err != nil {
		return OpportunityFindingStatus{}, err
	}
	latestRun, latestWorkflow, err := s.latestOpportunityFindingRun(ctx, projectID)
	if err != nil {
		return OpportunityFindingStatus{}, err
	}
	countRows, err := s.Q.SEOOpportunityCounts(ctx, projectID)
	if err != nil {
		return OpportunityFindingStatus{}, err
	}
	counts := opportunityFindingCounts(countRows)
	lastRun := opportunityFindingRunView(latestRun, latestWorkflow)
	if opportunityFindingWorkflowOwnsRun(lastRun, latestWorkflow) {
		stageRows, err := s.Q.ListOpportunityFindingStages(ctx, db.ListOpportunityFindingStagesParams{
			ProjectID: projectID, WorkflowEventID: latestWorkflow.ID,
		})
		if err != nil {
			return OpportunityFindingStatus{}, err
		}
		attachOpportunityFindingStageProgress(lastRun, stageRows)
	}
	status := OpportunityFindingStatus{
		GrowthSignalEnabled: cfg.GrowthSignalEnabled,
		GrowthAIEnabled:     cfg.GrowthAIEnabled,
		GrowthAIRunPolicy:   cfg.GrowthAIRunPolicy,
		ManualMode:          opportunityFindingManualMode(cfg),
		LastRun:             lastRun,
		NextFindingAt:       nextOpportunityFindingAt(time.Now().UTC(), cfg),
		Summary:             opportunityFindingSummary(latestRun, cfg, counts),
		Counts:              counts,
	}
	return status, nil
}

func opportunityFindingWorkflowOwnsRun(run *OpportunityFindingRun, event *db.WorkflowEvent) bool {
	return run != nil && event != nil && run.ID == event.ID
}

func opportunityFindingStageProgress(rows []db.OpportunityFindingStageCheckpoint) ([]OpportunityFindingStageProgress, int, string) {
	progress := make([]OpportunityFindingStageProgress, 0, len(rows))
	completed := 0
	current := ""
	for _, row := range rows {
		summary := map[string]any{}
		_ = json.Unmarshal(row.OutputSummary, &summary)
		progress = append(progress, OpportunityFindingStageProgress{
			Stage: row.Stage, Order: row.StageOrder, Status: row.Status,
			AttemptNumber: row.AttemptNumber, RequestFingerprint: row.RequestFingerprint,
			Summary: summary, Error: row.Error,
		})
		if row.Status == "running" && current == "" {
			current = row.Stage
		}
		if row.Status != "running" {
			completed++
		}
	}
	percent := completed * 100 / len(opportunityfinding.OrderedStages)
	return progress, percent, current
}

func attachOpportunityFindingStageProgress(run *OpportunityFindingRun, rows []db.OpportunityFindingStageCheckpoint) {
	if run == nil {
		return
	}
	run.StageProgress, run.ProgressPercent, run.CurrentStage = opportunityFindingStageProgress(rows)
	if run.Status != "completed" {
		return
	}
	for _, stage := range run.StageProgress {
		if stage.Status == "partial" || stage.Status == "failed" {
			run.Status = "partial"
			return
		}
	}
}

func (s *Server) latestOpportunityFindingRun(ctx context.Context, projectID uuid.UUID) (*db.SeoRun, *db.WorkflowEvent, error) {
	runs, err := s.Q.ListSEORuns(ctx, db.ListSEORunsParams{
		ProjectID: projectID,
		Agent:     "seo_analyzer",
		LimitRows: 1,
	})
	if err != nil {
		return nil, nil, err
	}
	var analyzer *db.SeoRun
	if len(runs) > 0 {
		analyzer = &runs[0]
	}
	workflowEvent, workflowErr := s.Q.LatestOpportunityFindingWorkflowEvent(ctx, projectID)
	if workflowErr != nil && !errors.Is(workflowErr, pgx.ErrNoRows) {
		return nil, nil, workflowErr
	}
	if workflowErr != nil {
		return analyzer, nil, nil
	}
	return analyzer, &workflowEvent, nil
}

func opportunityFindingRunView(run *db.SeoRun, workflowEvent *db.WorkflowEvent) *OpportunityFindingRun {
	workflowIsLatest := workflowEvent != nil && (workflowEvent.Status == "pending" || workflowEvent.Status == "running")
	if workflowEvent != nil && !workflowIsLatest {
		analyzerTime := pgTimePtr(runTime(run))
		workflowTime := pgTimePtr(workflowEvent.UpdatedAt)
		workflowIsLatest = run == nil || analyzerTime == nil || (workflowTime != nil && workflowTime.After(*analyzerTime))
	}
	if workflowIsLatest {
		status := workflowEvent.Status
		switch status {
		case "pending":
			status = "queued"
		case "succeeded":
			status = "completed"
		case "dead":
			status = "failed"
		}
		started := pgTimePtr(workflowEvent.LockedAt)
		if started == nil {
			started = pgTimePtr(workflowEvent.CreatedAt)
		}
		finished := pgTimePtr(workflowEvent.ProcessedAt)
		if finished == nil && status == "failed" {
			finished = pgTimePtr(workflowEvent.UpdatedAt)
		}
		var durationMs int64
		if started != nil && finished != nil && finished.After(*started) {
			durationMs = finished.Sub(*started).Milliseconds()
		}
		return &OpportunityFindingRun{ID: workflowEvent.ID, Status: status, StartedAt: started, FinishedAt: finished, DurationMs: durationMs, Error: workflowEvent.Error}
	}
	if run == nil {
		return nil
	}
	started := pgTimePtr(run.StartedAt)
	finished := pgTimePtr(run.FinishedAt)
	var durationMs int64
	if started != nil && finished != nil && finished.After(*started) {
		durationMs = finished.Sub(*started).Milliseconds()
	}
	return &OpportunityFindingRun{
		ID:         run.ID,
		Status:     run.Status,
		StartedAt:  started,
		FinishedAt: finished,
		DurationMs: durationMs,
		Error:      run.Error,
	}
}

func runTime(run *db.SeoRun) pgtype.Timestamptz {
	if run == nil {
		return pgtype.Timestamptz{}
	}
	if run.FinishedAt.Valid {
		return run.FinishedAt
	}
	return run.StartedAt
}

func opportunityFindingCounts(rows []db.SEOOpportunityCountsRow) OpportunityFindingCounts {
	counts := OpportunityFindingCounts{ByStatus: map[string]int{}}
	for _, row := range rows {
		n := int(row.Count)
		counts.Total += n
		counts.ByStatus[row.Status] += n
		switch row.Status {
		case "open":
			counts.Open += n
		case "converted", "watching", "due_for_review":
			counts.InLoop += n
			counts.Processed += n
		default:
			counts.Processed += n
		}
	}
	return counts
}

func opportunityFindingSummary(run *db.SeoRun, cfg config.ProjectConfig, counts OpportunityFindingCounts) []OpportunityFindingSummaryItem {
	items := make([]OpportunityFindingSummaryItem, 0, 6)
	output := opportunityFindingRunOutput{}
	if run != nil && len(run.Output) > 0 {
		_ = json.Unmarshal(run.Output, &output)
	}
	if cfg.GrowthSignalEnabled {
		detail := "No evidence matching run recorded yet"
		tone := "amber"
		if run != nil {
			tone = "green"
			if output.GeneratedAnomalies > 0 {
				detail = fmt.Sprintf("%d signals matched or updated", output.GeneratedAnomalies)
			} else {
				detail = "No new decision-ready findings"
			}
		}
		items = append(items, OpportunityFindingSummaryItem{Label: "Evidence matching", Detail: detail, Tone: tone})
	}
	for _, note := range output.DataSourceNotes {
		if item, ok := opportunityFindingSummaryFromNote(note); ok {
			items = append(items, item)
		}
	}
	if output.CheckedURLs > 0 {
		items = append(items, OpportunityFindingSummaryItem{
			Label:  "Crawl inventory",
			Detail: fmt.Sprintf("%d URLs checked for technical and content signals", output.CheckedURLs),
			Tone:   "neutral",
		})
	}
	if counts.Processed > 0 {
		items = append(items, OpportunityFindingSummaryItem{
			Label:  "Deduplication",
			Detail: fmt.Sprintf("%d processed opportunities kept out of the decision queue", counts.Processed),
			Tone:   "neutral",
		})
	}
	items = append(items, opportunityFindingAISummary(cfg))
	return items
}

func opportunityFindingSummaryFromNote(note string) (OpportunityFindingSummaryItem, bool) {
	key, value, ok := strings.Cut(note, ":")
	if !ok {
		return OpportunityFindingSummaryItem{}, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return OpportunityFindingSummaryItem{}, false
	}
	switch strings.TrimSpace(key) {
	case "gsc_metric_opportunities":
		return OpportunityFindingSummaryItem{Label: "GSC metrics", Detail: fmt.Sprintf("%d query/page signals matched", n), Tone: "green"}, true
	case "actionable_seo_opportunities":
		return OpportunityFindingSummaryItem{Label: "Site inventory", Detail: fmt.Sprintf("%d technical or page-work signals matched", n), Tone: "green"}, true
	case "cold_start_opportunities":
		return OpportunityFindingSummaryItem{Label: "Cold start", Detail: fmt.Sprintf("%d profile-based opportunities matched", n), Tone: "green"}, true
	default:
		return OpportunityFindingSummaryItem{}, false
	}
}

func opportunityFindingAISummary(cfg config.ProjectConfig) OpportunityFindingSummaryItem {
	if !cfg.GrowthAIEnabled {
		return OpportunityFindingSummaryItem{Label: "AI assistance", Detail: "Off for Opportunities", Tone: "neutral"}
	}
	switch cfg.GrowthAIRunPolicy {
	case config.GrowthAIRunPolicyScheduledAndEvent:
		return OpportunityFindingSummaryItem{Label: "AI assistance", Detail: "Scheduled and approved event runs", Tone: "green"}
	case config.GrowthAIRunPolicyScheduledOnly:
		return OpportunityFindingSummaryItem{Label: "AI assistance", Detail: "Scheduled runs only", Tone: "green"}
	case config.GrowthAIRunPolicyManualOnly:
		return OpportunityFindingSummaryItem{Label: "AI assistance", Detail: "Manual only", Tone: "amber"}
	default:
		return OpportunityFindingSummaryItem{Label: "AI assistance", Detail: "On demand; review before provider use", Tone: "neutral"}
	}
}

func opportunityFindingManualMode(cfg config.ProjectConfig) bool {
	return cfg.AllowsGrowthAI(config.GrowthAITriggerManual) && !cfg.AllowsGrowthAI(config.GrowthAITriggerScheduled)
}

func nextOpportunityFindingAt(now time.Time, cfg config.ProjectConfig) *time.Time {
	hasScheduledSignalScan := cfg.GrowthSignalEnabled
	hasScheduledAI := cfg.AllowsGrowthAI(config.GrowthAITriggerScheduled)
	if !hasScheduledSignalScan && !hasScheduledAI {
		return nil
	}
	utc := now.UTC()
	next := time.Date(utc.Year(), utc.Month(), utc.Day(), 3, 0, 0, 0, time.UTC)
	if !next.After(utc) {
		next = next.Add(24 * time.Hour)
	}
	return &next
}

func (s *Server) syncSEO(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	var in struct {
		SiteURL string `json:"site_url"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	svc, err := s.seoServiceWithGrowthAuthority(r.Context(), projectID, config.GrowthAITriggerManual)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	syncResult, err := svc.Sync(r.Context(), projectID, in.SiteURL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	analyzeResult, err := svc.Analyze(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sync": syncResult, "analyze": analyzeResult})
}

func (s *Server) analyzeSEO(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	svc, err := s.seoServiceWithGrowthAuthority(r.Context(), projectID, config.GrowthAITriggerManual)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	result, err := svc.Analyze(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listSEORuns(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	limit, err := parseLimit(r, 50, 100)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad limit")
		return
	}
	cursor, err := parseCursor(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad cursor")
		return
	}
	runs, err := s.Q.ListSEORuns(r.Context(), db.ListSEORunsParams{
		ProjectID:       projectID,
		Agent:           r.URL.Query().Get("agent"),
		Status:          r.URL.Query().Get("status"),
		CursorStartedAt: pgutil.TS(cursor),
		LimitRows:       int32(limit),
	})
	if cursor.IsZero() {
		runs, err = s.Q.ListSEORuns(r.Context(), db.ListSEORunsParams{
			ProjectID: projectID,
			Agent:     r.URL.Query().Get("agent"),
			Status:    r.URL.Query().Get("status"),
			LimitRows: int32(limit),
		})
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(runs))
}

func (s *Server) listSEOOpportunities(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	limit, err := parseLimit(r, 50, 100)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad limit")
		return
	}
	cursor, err := parseCursor(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad cursor")
		return
	}
	// Snoozed opportunities return to Needs decision once their snooze window
	// elapses (PRD §11.2); promote them before listing.
	if _, err := s.Q.WakeDueSnoozedSEOOpportunities(r.Context(), projectID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	params := db.ListSEOOpportunitiesParams{
		ProjectID: projectID,
		Type:      r.URL.Query().Get("type"),
		Status:    r.URL.Query().Get("status"),
		LimitRows: int32(limit),
	}
	if !cursor.IsZero() {
		params.CursorCreatedAt = pgutil.TS(cursor)
	}
	opps, err := s.Q.ListSEOOpportunities(r.Context(), params)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(opps))
}

func (s *Server) getSEOOpportunity(w http.ResponseWriter, r *http.Request) {
	projectID, oppID, ok := s.seoIDs(w, r, "opportunityID")
	if !ok {
		return
	}
	opp, err := s.Q.GetSEOOpportunity(r.Context(), db.GetSEOOpportunityParams{ID: oppID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "opportunity not found")
		return
	}
	writeJSON(w, http.StatusOK, opp)
}

func (s *Server) acceptSEOOpportunity(w http.ResponseWriter, r *http.Request) {
	s.createSEOContentActionFromOpportunity(w, r, http.StatusOK)
}

func (s *Server) dismissSEOOpportunity(w http.ResponseWriter, r *http.Request) {
	s.updateSEOOpportunityStatus(w, r, "dismissed")
}

func (s *Server) recordSEOOpportunityReviewState(ctx context.Context, q *db.Queries, projectID uuid.UUID, oppID uuid.UUID, contentActionID pgtype.UUID, reviewStatus string, snoozedUntil pgtype.Timestamptz) error {
	if q == nil {
		return nil
	}
	_, err := q.CreateOrUpdateSEOOpportunityReviewState(ctx, db.CreateOrUpdateSEOOpportunityReviewStateParams{
		ProjectID:           projectID,
		SourceOpportunityID: oppID,
		ContentActionID:     contentActionID,
		ReviewStatus:        reviewStatus,
		SnoozedUntil:        snoozedUntil,
	})
	return err
}

func (s *Server) updateSEOOpportunityStatus(w http.ResponseWriter, r *http.Request, status string) {
	projectID, oppID, ok := s.seoIDs(w, r, "opportunityID")
	if !ok {
		return
	}
	opp, err := s.Q.UpdateSEOOpportunityStatus(r.Context(), db.UpdateSEOOpportunityStatusParams{
		ID:        oppID,
		ProjectID: projectID,
		Status:    status,
	})
	if err != nil {
		writeErr(w, http.StatusNotFound, "opportunity not found")
		return
	}
	if status == "dismissed" {
		if err := s.recordSEOOpportunityReviewState(r.Context(), s.Q, projectID, oppID, pgtype.UUID{}, "dismissed", pgtype.Timestamptz{}); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := s.enqueueWorkflowEvent(r.Context(), projectID, workflow.EventOpportunityReviewed, "seo_opportunity", oppID, workflowDedupeKey(workflow.EventOpportunityReviewed, projectID, oppID, status), map[string]any{
		"opportunity_id": oppID,
		"status":         status,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, opp)
}

func (s *Server) createSEOContentAction(w http.ResponseWriter, r *http.Request) {
	s.createSEOContentActionFromOpportunity(w, r, http.StatusCreated)
}

// contentActionOverrides carries the optional caller-supplied fields used when
// creating a content action from an opportunity. All fields are optional; empty
// values fall back to opportunity-derived defaults.
type contentActionOverrides struct {
	ActionType       string          `json:"action_type"`
	AssetType        string          `json:"asset_type"`
	WorkType         string          `json:"work_type"`
	TargetSurfaceID  *uuid.UUID      `json:"target_surface_id"`
	RiskReasons      json.RawMessage `json:"risk_reasons"`
	EvidenceSnapshot json.RawMessage `json:"evidence_snapshot"`
	InputSnapshot    json.RawMessage `json:"input_snapshot"`
	OutputSnapshot   json.RawMessage `json:"output_snapshot"`
	DiffSnapshot     json.RawMessage `json:"diff_snapshot"`
	ReviewRequired   *bool           `json:"review_required"`
}

// persistContentActionFromOpportunity creates a content action for an existing
// opportunity, marks the opportunity converted, and enqueues the reviewed event.
// It is shared by the opportunity-accept endpoint and the Doctor
// finding→Site Fix conversion, so the lifecycle stays identical for both origins.
func (s *Server) persistContentActionFromOpportunity(ctx context.Context, projectID uuid.UUID, opp db.SeoOpportunity, workType, routingSource string, in contentActionOverrides) (db.ContentAction, error) {
	if err := s.requireGrowthOpportunityExecutable(ctx, projectID, opp.ID); err != nil {
		return db.ContentAction{}, err
	}
	actionType := strings.TrimSpace(in.ActionType)
	if actionType == "" && opp.RecommendedAction != nil {
		actionType = *opp.RecommendedAction
	}
	if actionType == "" {
		actionType = "technical SEO fix task"
	}
	assetTypeValue := inferContentActionAssetType(opp, actionType, in.AssetType)
	var assetType *string
	if assetTypeValue != "" {
		assetType = &assetTypeValue
	}
	var targetHash *string
	if opp.ArticleID.Valid {
		article, err := s.Q.GetArticleForProject(ctx, db.GetArticleForProjectParams{
			ID:        uuid.UUID(opp.ArticleID.Bytes),
			ProjectID: projectID,
		})
		if err == nil {
			targetHash = article.ContentHash
		}
	}
	action, err := s.Q.CreateContentAction(ctx, db.CreateContentActionParams{
		ProjectID:               projectID,
		OpportunityID:           opp.ID,
		ActionType:              actionType,
		Status:                  "ready_for_review",
		TargetArticleID:         opp.ArticleID,
		TargetUrl:               opp.PageUrl,
		NormalizedTargetUrl:     strPtrFrom(opp.NormalizedPageUrl),
		TargetContentHashBefore: targetHash,
		BaselineWindow:          json.RawMessage(`{"days":28}`),
		MeasurementWindow:       measurementWindowForAction(assetTypeValue, actionType),
		ApprovalSource:          ApprovalSourceHumanReview,
		RoutingSource:           routingSource,
		WorkType:                &workType,
	})
	if err != nil {
		return db.ContentAction{}, err
	}
	reviewRequired := defaultReviewRequiredForAssetType(assetTypeValue, opp.RiskLevel)
	if in.ReviewRequired != nil {
		reviewRequired = *in.ReviewRequired
	}
	targetSurfaceID := pgtype.UUID{}
	if in.TargetSurfaceID != nil {
		targetSurfaceID = pgtype.UUID{Bytes: *in.TargetSurfaceID, Valid: true}
	}
	action, err = s.Q.UpdateContentActionExecutionMetadata(ctx, db.UpdateContentActionExecutionMetadataParams{
		ID:                   action.ID,
		ProjectID:            projectID,
		AssetType:            assetType,
		TargetSurfaceID:      targetSurfaceID,
		RiskReasons:          rawOrDefault(in.RiskReasons, `[]`),
		EvidenceSnapshot:     contentActionEvidenceSnapshot(in.EvidenceSnapshot, opp),
		InputSnapshot:        contentActionInputSnapshot(in.InputSnapshot, opp, actionType),
		OutputSnapshot:       defaultOutputSnapshotForAction(in.OutputSnapshot, assetTypeValue, actionType, opp),
		DiffSnapshot:         defaultDiffSnapshotForAction(in.DiffSnapshot, assetTypeValue, actionType, opp),
		ReviewRequired:       reviewRequired,
		VerificationSnapshot: json.RawMessage(`{}`),
	})
	if err != nil {
		return db.ContentAction{}, err
	}
	_, _ = s.Q.UpdateSEOOpportunityStatus(ctx, db.UpdateSEOOpportunityStatusParams{ID: opp.ID, ProjectID: projectID, Status: "converted"})
	if err := s.enqueueWorkflowEvent(ctx, projectID, workflow.EventOpportunityReviewed, "seo_opportunity", opp.ID, workflowDedupeKey(workflow.EventOpportunityReviewed, projectID, opp.ID, "converted"), map[string]any{
		"opportunity_id": opp.ID,
		"action_id":      action.ID,
		"status":         "converted",
	}); err != nil {
		return db.ContentAction{}, err
	}
	return action, nil
}

var errGrowthOpportunityHardBlocked = errors.New("Growth opportunity is blocked by unresolved Doctor work")

func (s *Server) requireGrowthOpportunityExecutable(ctx context.Context, projectID, opportunityID uuid.UUID) error {
	if s.Pool != nil {
		svc, err := s.seoServiceWithGrowthAuthority(ctx, projectID, config.GrowthAITriggerManual)
		if err != nil {
			return fmt.Errorf("load Growth AI authority: %w", err)
		}
		if err := svc.EnsureGrowthOpportunityReserved(ctx, projectID, opportunityID); err != nil {
			return fmt.Errorf("canonicalize Growth opportunity reservations: %w", err)
		}
	}
	canExecute, err := s.Q.GrowthOpportunityExecutionGuard(ctx, db.GrowthOpportunityExecutionGuardParams{
		ProjectID: projectID, OpportunityID: pgtype.UUID{Bytes: opportunityID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("check Growth work reservation: %w", err)
	}
	if !canExecute {
		return errGrowthOpportunityHardBlocked
	}
	return nil
}

func (s *Server) createSEOContentActionFromOpportunity(w http.ResponseWriter, r *http.Request, successStatus int) {
	projectID, oppID, ok := s.seoIDs(w, r, "opportunityID")
	if !ok {
		return
	}
	var in contentActionOverrides
	_ = json.NewDecoder(r.Body).Decode(&in)
	opp, err := s.Q.GetSEOOpportunity(r.Context(), db.GetSEOOpportunityParams{ID: oppID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "opportunity not found")
		return
	}
	recommendedWorkType := workTypeForOpportunity(opp)
	workType := recommendedWorkType
	routingSource := RoutingSourceSystem
	if requested := strings.TrimSpace(strings.ToLower(in.WorkType)); requested != "" && requested != recommendedWorkType {
		if !workTypeAllowed(opp, requested) {
			writeErr(w, http.StatusBadRequest, "work_type not allowed for this opportunity")
			return
		}
		workType = requested
		routingSource = RoutingSourceUserOverride
	}
	action, err := s.persistContentActionFromOpportunity(r.Context(), projectID, opp, workType, routingSource, in)
	if err != nil {
		if errors.Is(err, errGrowthOpportunityHardBlocked) {
			writeErr(w, http.StatusConflict, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, successStatus, action)
}

func (s *Server) listSEOContentActions(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	limit, err := parseLimit(r, 50, 100)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad limit")
		return
	}
	cursor, err := parseCursor(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad cursor")
		return
	}
	params := db.ListContentActionsParams{
		ProjectID: projectID,
		Status:    r.URL.Query().Get("status"),
		LimitRows: int32(limit),
	}
	if !cursor.IsZero() {
		params.CursorCreatedAt = pgutil.TS(cursor)
	}
	actions, err := s.Q.ListContentActions(r.Context(), params)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(actions))
}

func (s *Server) getSEOContentAction(w http.ResponseWriter, r *http.Request) {
	projectID, actionID, ok := s.seoIDs(w, r, "actionID")
	if !ok {
		return
	}
	action, err := s.Q.GetContentAction(r.Context(), db.GetContentActionParams{ID: actionID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "action not found")
		return
	}
	writeJSON(w, http.StatusOK, action)
}

func (s *Server) planSEOContentAction(w http.ResponseWriter, r *http.Request) {
	projectID, actionID, ok := s.seoIDs(w, r, "actionID")
	if !ok {
		return
	}
	var in struct {
		PublishStrategy string                    `json:"publish_strategy"`
		PublishTo       string                    `json:"publish_to"`
		AssetType       string                    `json:"asset_type"`
		CanonicalTarget platformcontract.Target   `json:"canonical_target"`
		TargetPlatforms []platformcontract.Target `json:"target_platforms"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	requestedPublishStrategy := firstNonEmpty(in.PublishStrategy, in.PublishTo)
	action, err := s.Q.GetContentAction(r.Context(), db.GetContentActionParams{ID: actionID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "action not found")
		return
	}
	if !contentActionCreatesContent(action) {
		writeErr(w, http.StatusBadRequest, "content action does not create content")
		return
	}
	if topic, ok, err := s.existingTopicForContentAction(r.Context(), projectID, actionID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	} else if ok {
		if publishStrategy := normalizePublishStrategy(requestedPublishStrategy); publishStrategy != "" && topic.Channel != publishStrategy {
			topic, err = s.Q.UpdateTopic(r.Context(), db.UpdateTopicParams{
				ID:                    topic.ID,
				ProjectID:             projectID,
				Channel:               publishStrategy,
				Title:                 topic.Title,
				TargetKeyword:         topic.TargetKeyword,
				TargetPrompt:          topic.TargetPrompt,
				Angle:                 topic.Angle,
				Format:                topic.Format,
				Priority:              topic.Priority,
				InternalLinks:         topic.InternalLinks,
				Status:                topic.Status,
				ScheduledAt:           topic.ScheduledAt,
				SourceContentActionID: topic.SourceContentActionID,
			})
			if err != nil {
				writeErr(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		writeJSON(w, http.StatusOK, topic)
		return
	}
	opp, err := s.Q.GetSEOOpportunity(r.Context(), db.GetSEOOpportunityParams{ID: action.OpportunityID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "opportunity not found")
		return
	}
	assetType := firstNonEmpty(in.AssetType, contentActionAssetType(action), "blog_post")
	planInput := platformcontract.PlanInput{
		ProjectID: projectID, OpportunityID: opp.ID, ContentActionID: action.ID, AssetType: assetType,
		CanonicalTarget: in.CanonicalTarget, Targets: in.TargetPlatforms, SelectionMode: "contract_matrix",
	}
	if len(in.TargetPlatforms) == 0 {
		planInput, err = s.legacyTargetPlan(r.Context(), projectID, opp.ID, action.ID, assetType, requestedPublishStrategy)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	contracts, err := s.Q.ListActivePlatformContentContracts(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	contexts, err := s.Q.ListPlatformTargetContexts(r.Context(), db.ListPlatformTargetContextsParams{ProjectID: projectID, Platform: ""})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := platformcontract.ValidatePlanSelection(planInput, contracts, contexts, time.Now().UTC()); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.Pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "database transaction is unavailable")
		return
	}
	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(context.WithoutCancel(r.Context()))
	q := db.New(tx)
	targetPlan, err := platformcontract.CreatePlan(r.Context(), q, planInput)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	topicParams := topicFromContentAction(projectID, action, opp, requestedPublishStrategy)
	topicParams.Channel = platformcontract.DeriveChannel(targetPlan)
	topicParams.AssetType = strPtr(targetPlan.AssetType)
	topicParams.TargetPlanID = pgtype.UUID{Bytes: targetPlan.ID, Valid: true}
	topic, err := q.CreateTopic(r.Context(), topicParams)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := q.UpdateContentActionStatus(r.Context(), db.UpdateContentActionStatusParams{
		ID:        action.ID,
		ProjectID: projectID,
		Status:    "approved",
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := q.EnqueueWorkflowEvent(r.Context(), db.EnqueueWorkflowEventParams{
		ProjectID:  projectID,
		EventType:  workflow.EventContentPlanCreated,
		DedupeKey:  workflowEventDedupeKey(workflow.EventContentPlanCreated, projectID, action.ID.String()),
		Payload:    mustJSONLocal(map[string]any{"action_id": action.ID, "topic_id": topic.ID}),
		EntityType: strPtr("topic"),
		EntityID:   pgtype.UUID{Bytes: topic.ID, Valid: true},
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, topic)
}

func (s *Server) legacyTargetPlan(ctx context.Context, projectID, opportunityID, actionID uuid.UUID, assetType, requestedStrategy string) (platformcontract.PlanInput, error) {
	contracts, err := s.Q.ListActivePlatformContentContracts(ctx)
	if err != nil {
		return platformcontract.PlanInput{}, err
	}
	strategy := normalizePublishStrategy(requestedStrategy)
	if strategy == "" {
		strategy = "blog"
	}
	return platformcontract.LegacyPlanInput(platformcontract.PlanInput{
		ProjectID: projectID, OpportunityID: opportunityID, ContentActionID: actionID, AssetType: assetType,
	}, strategy, contracts)
}

func (s *Server) createPageUpdateDraftForAction(w http.ResponseWriter, r *http.Request) {
	projectID, actionID, ok := s.seoIDs(w, r, "actionID")
	if !ok {
		return
	}
	action, err := s.Q.GetContentAction(r.Context(), db.GetContentActionParams{ID: actionID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "action not found")
		return
	}
	if !contentActionIsPageUpdate(action) {
		writeErr(w, http.StatusBadRequest, "content action is not an existing page update")
		return
	}
	opp, err := s.Q.GetSEOOpportunity(r.Context(), db.GetSEOOpportunityParams{ID: action.OpportunityID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "opportunity not found")
		return
	}
	targetURL := firstNonEmpty(
		stringPtrValueAPI(action.TargetUrl),
		stringPtrValueAPI(opp.PageUrl),
		stringPtrValueAPI(action.NormalizedTargetUrl),
		opp.NormalizedPageUrl,
	)
	normalizedTargetURL := firstNonEmpty(
		stringPtrValueAPI(action.NormalizedTargetUrl),
		opp.NormalizedPageUrl,
		targetURL,
	)
	if targetURL == "" || normalizedTargetURL == "" {
		writeErr(w, http.StatusBadRequest, "page update target URL is missing")
		return
	}
	targetArticleID := action.TargetArticleID
	if !targetArticleID.Valid {
		targetArticleID = opp.ArticleID
	}
	draft, err := s.Q.CreateOrReusePageUpdateDraft(r.Context(), db.CreateOrReusePageUpdateDraftParams{
		ProjectID:              projectID,
		ContentActionID:        action.ID,
		TargetUrl:              targetURL,
		NormalizedTargetUrl:    normalizedTargetURL,
		OpportunityKey:         opp.OpportunityKey,
		TargetArticleID:        targetArticleID,
		BaseContentHash:        action.TargetContentHashBefore,
		ResolutionCriteria:     pageUpdateResolutionCriteria(action, opp, targetURL),
		OriginalSourceSnapshot: pageUpdateOriginalSourceSnapshot(action, opp, targetURL, normalizedTargetURL),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, draft)
}

func (s *Server) getPageUpdateDraft(w http.ResponseWriter, r *http.Request) {
	projectID, draftID, ok := s.seoIDs(w, r, "draftID")
	if !ok {
		return
	}
	draft, err := s.Q.GetPageUpdateDraftForProject(r.Context(), db.GetPageUpdateDraftForProjectParams{ID: draftID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "page update draft not found")
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

func (s *Server) generatePageUpdateDraft(w http.ResponseWriter, r *http.Request) {
	projectID, draftID, ok := s.seoIDs(w, r, "draftID")
	if !ok {
		return
	}
	var in struct {
		ProposedContentMD string          `json:"proposed_content_md"`
		Patch             json.RawMessage `json:"patch"`
		DiffSnapshot      json.RawMessage `json:"diff_snapshot"`
		QAFeedback        json.RawMessage `json:"qa_feedback"`
	}
	if err := decodeOptionalJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	draft, action, err := s.pageUpdateDraftWithAction(r.Context(), projectID, draftID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "page update draft not found")
		return
	}
	proposed := strings.TrimSpace(in.ProposedContentMD)
	if proposed == "" {
		proposed = defaultPageUpdateProposedContent(action, draft)
	}
	patch := rawOrDefault(in.Patch, "")
	if len(patch) == 0 || !json.Valid(patch) {
		patch = defaultPageUpdatePatch(action, draft, proposed)
	}
	diff := rawOrDefault(in.DiffSnapshot, "")
	if len(diff) == 0 || !json.Valid(diff) {
		diff = defaultPageUpdateDiff(action, draft, proposed)
	}
	qa := rawOrDefault(in.QAFeedback, "")
	if len(qa) == 0 || !json.Valid(qa) {
		qa = defaultPageUpdateQA(action, draft)
	}
	draft, err = s.Q.UpdatePageUpdateDraftContent(r.Context(), db.UpdatePageUpdateDraftContentParams{
		ID:                draft.ID,
		ProjectID:         projectID,
		ProposedContentMd: proposed,
		Patch:             patch,
		DiffSnapshot:      diff,
		QaFeedback:        qa,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.Q.UpdateContentActionExecutionMetadata(r.Context(), db.UpdateContentActionExecutionMetadataParams{
		ID:                   action.ID,
		ProjectID:            projectID,
		AssetType:            action.AssetType,
		TargetSurfaceID:      action.TargetSurfaceID,
		RiskReasons:          rawOrDefault(action.RiskReasons, `[]`),
		EvidenceSnapshot:     rawOrDefault(action.EvidenceSnapshot, `{}`),
		InputSnapshot:        rawOrDefault(action.InputSnapshot, `{}`),
		OutputSnapshot:       mustJSONLocal(map[string]any{"page_update_draft_id": draft.ID, "status": draft.Status, "target_url": draft.TargetUrl}),
		DiffSnapshot:         diff,
		ReviewRequired:       true,
		ApprovedBy:           action.ApprovedBy,
		ApprovedAt:           action.ApprovedAt,
		VerifiedAt:           action.VerifiedAt,
		VerificationSnapshot: rawOrDefault(action.VerificationSnapshot, `{}`),
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.Q.UpdateContentActionStatus(r.Context(), db.UpdateContentActionStatusParams{ID: action.ID, ProjectID: projectID, Status: "ready_for_review"}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

func (s *Server) approvePageUpdateDraft(w http.ResponseWriter, r *http.Request) {
	projectID, draftID, ok := s.seoIDs(w, r, "draftID")
	if !ok {
		return
	}
	draft, action, err := s.pageUpdateDraftWithAction(r.Context(), projectID, draftID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "page update draft not found")
		return
	}
	if draft.Status != "ready_for_review" && draft.Status != "approved" {
		writeErr(w, http.StatusBadRequest, "page update draft is not ready for approval")
		return
	}
	draft, err = s.Q.UpdatePageUpdateDraftStatus(r.Context(), db.UpdatePageUpdateDraftStatusParams{ID: draft.ID, ProjectID: projectID, Status: "approved"})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.Q.UpdateContentActionStatus(r.Context(), db.UpdateContentActionStatusParams{ID: action.ID, ProjectID: projectID, Status: "approved"}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

func (s *Server) applyPageUpdateDraft(w http.ResponseWriter, r *http.Request) {
	projectID, draftID, ok := s.seoIDs(w, r, "draftID")
	if !ok {
		return
	}
	ctx := r.Context()
	claimed, err := s.Q.ClaimPageUpdateDraftForApply(ctx, db.ClaimPageUpdateDraftForApplyParams{ID: draftID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "page update draft is not approved or is already being applied")
		return
	}
	action, err := s.Q.GetContentAction(ctx, db.GetContentActionParams{ID: claimed.ContentActionID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "content action not found")
		return
	}
	fallbackReason := "No exact CiteLoop-published MDX/Markdown source mapping and connected GitHub publisher were available for this V1 page update."
	if draft, applied, applyErr := s.applyPageUpdateDraftViaGitHubPR(ctx, projectID, claimed, action); applied {
		writeJSON(w, http.StatusOK, draft)
		return
	} else if applyErr != nil {
		fallbackReason = applyErr.Error()
	}
	draft, err := s.markPageUpdateDraftManualApply(ctx, projectID, claimed, action, fallbackReason)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

func (s *Server) applyPageUpdateDraftViaGitHubPR(ctx context.Context, projectID uuid.UUID, claimed db.PageUpdateDraft, action db.ContentAction) (db.PageUpdateDraft, bool, error) {
	if !claimed.TargetArticleID.Valid {
		return db.PageUpdateDraft{}, false, fmt.Errorf("page update draft has no target article")
	}
	article, err := s.Q.GetArticleForProject(ctx, db.GetArticleForProjectParams{ID: uuid.UUID(claimed.TargetArticleID.Bytes), ProjectID: projectID})
	if err != nil {
		return db.PageUpdateDraft{}, false, err
	}
	mapping, ok := pageUpdateExactSourceMapping(article)
	if !ok {
		return db.PageUpdateDraft{}, false, fmt.Errorf("target article is not an exact CiteLoop-published Markdown source")
	}
	conn, err := s.Q.GetEnabledPublisherConnectionForProject(ctx, db.GetEnabledPublisherConnectionForProjectParams{
		ProjectID: projectID,
		Kind:      publisher.ConnectionKindGitHubNextJS,
	})
	if err != nil {
		return db.PageUpdateDraft{}, false, err
	}
	cfg, err := publisher.ParseGitHubNextJSConfig(conn.Config)
	if err != nil {
		return db.PageUpdateDraft{}, false, err
	}
	if target, ok := publisher.GitHubNextJSTargetForSiteURL(claimed.TargetUrl); ok {
		cfg.Branch = target.Branch
		cfg.BaseURL = target.BaseURL
	}
	token, err := s.publisherConnectionToken(ctx, projectID, conn)
	if err != nil || strings.TrimSpace(token) == "" {
		if err == nil {
			err = fmt.Errorf("github publisher token is empty")
		}
		return db.PageUpdateDraft{}, false, err
	}
	workingBranch := pageUpdateWorkingBranch(claimed)
	repo := cfg.Repo
	baseBranch := cfg.Branch
	sourceFilePath := mapping.SourceFilePath
	sourceFilePaths := mustJSONLocal([]string{sourceFilePath})
	baseCommitSHA := strPtrFrom(mapping.BaseCommitSHA)
	proposedHash := pageUpdateContentHash(claimed.ProposedContentMd)
	app, err := s.Q.CreateOrReuseSiteChangeApplication(ctx, db.CreateOrReuseSiteChangeApplicationParams{
		ProjectID:               projectID,
		SourceOpportunityID:     pgtype.UUID{Bytes: action.OpportunityID, Valid: true},
		ContentActionID:         pgtype.UUID{Bytes: action.ID, Valid: true},
		PageUpdateDraftID:       pgtype.UUID{Bytes: claimed.ID, Valid: true},
		ApplicationKind:         "page_update",
		TargetUrl:               claimed.TargetUrl,
		NormalizedTargetUrl:     claimed.NormalizedTargetUrl,
		OpportunityKey:          claimed.OpportunityKey,
		PublisherConnectionID:   pgtype.UUID{Bytes: conn.ID, Valid: true},
		RepoFullName:            &repo,
		BaseBranch:              &baseBranch,
		WorkingBranch:           &workingBranch,
		BaseCommitSha:           baseCommitSHA,
		HeadCommitSha:           nil,
		SourceFilePath:          &sourceFilePath,
		SourceFilePaths:         sourceFilePaths,
		SourceMappingConfidence: mapping.Confidence,
		SourceMappingReason:     mapping.Reason,
		BaseFileSha:             nil,
		BaseContentHash:         claimed.BaseContentHash,
		ProposedContentHash:     &proposedHash,
		PatchSnapshot:           jsonOrEmptyObject(claimed.Patch),
		DiffSnapshot:            jsonOrEmptyObject(claimed.DiffSnapshot),
		ResolutionCriteria:      jsonOrEmptyObject(claimed.ResolutionCriteria),
		Status:                  "ready_for_pr",
	})
	if err != nil {
		return db.PageUpdateDraft{}, false, err
	}
	if app.Status == "github_pr_open" && strings.TrimSpace(stringPtrValueAPI(app.GithubPrUrl)) != "" {
		draft, err := s.markPageUpdateDraftGitHubPRResult(ctx, projectID, claimed, action, app)
		return draft, err == nil, err
	}
	pr, err := publisher.NewGitHubPRClient(token, cfg.Repo, cfg.Branch, s.Log).CreatePageUpdatePR(ctx, publisher.GitHubPRInput{
		SourcePath:        mapping.SourceFilePath,
		WorkingBranch:     workingBranch,
		BaseFileSHA:       "",
		ProposedContentMD: claimed.ProposedContentMd,
		CommitMessage:     pageUpdatePRCommitMessage(action),
		Title:             pageUpdatePRTitle(action),
		Body:              pageUpdatePRBody(claimed, action, mapping),
	})
	if err != nil {
		_, _ = s.Q.MarkSiteChangeApplicationStatus(ctx, db.MarkSiteChangeApplicationStatusParams{
			ID:                   app.ID,
			ProjectID:            projectID,
			Status:               "manual_apply_required",
			GithubPrState:        nil,
			DeploymentSnapshot:   json.RawMessage(`{}`),
			VerificationSnapshot: json.RawMessage(`{}`),
			FailureReason:        strPtrFrom(err.Error()),
		})
		return db.PageUpdateDraft{}, false, err
	}
	prNumber := int32(pr.Number)
	app, err = s.Q.MarkSiteChangeApplicationGitHubPR(ctx, db.MarkSiteChangeApplicationGitHubPRParams{
		ID:             app.ID,
		ProjectID:      projectID,
		WorkingBranch:  &pr.WorkingBranch,
		HeadCommitSha:  &pr.HeadCommitSHA,
		BaseFileSha:    &pr.BaseFileSHA,
		GithubPrNumber: &prNumber,
		GithubPrUrl:    &pr.URL,
		GithubPrState:  &pr.State,
	})
	if err != nil {
		return db.PageUpdateDraft{}, false, err
	}
	draft, err := s.markPageUpdateDraftGitHubPRResult(ctx, projectID, claimed, action, app)
	if err != nil {
		return db.PageUpdateDraft{}, false, err
	}
	return draft, true, nil
}

func (s *Server) markPageUpdateDraftGitHubPRResult(ctx context.Context, projectID uuid.UUID, claimed db.PageUpdateDraft, action db.ContentAction, app db.SiteChangeApplication) (db.PageUpdateDraft, error) {
	result := mustJSONLocal(map[string]any{
		"mode":                       "github_pr",
		"status":                     "github_pr_open",
		"site_change_application_id": app.ID,
		"github_pr_number":           app.GithubPrNumber,
		"github_pr_url":              app.GithubPrUrl,
		"github_pr_state":            app.GithubPrState,
		"repo":                       app.RepoFullName,
		"base_branch":                app.BaseBranch,
		"working_branch":             app.WorkingBranch,
		"head_commit_sha":            app.HeadCommitSha,
		"base_file_sha":              app.BaseFileSha,
		"source_file_path":           app.SourceFilePath,
		"target_url":                 claimed.TargetUrl,
	})
	draft, err := s.Q.MarkPageUpdateDraftApplyResult(ctx, db.MarkPageUpdateDraftApplyResultParams{
		ID:              claimed.ID,
		ProjectID:       projectID,
		Status:          "verification_pending",
		PublisherResult: result,
	})
	if err != nil {
		return db.PageUpdateDraft{}, err
	}
	if _, err := s.Q.UpdateContentActionStatus(ctx, db.UpdateContentActionStatusParams{ID: action.ID, ProjectID: projectID, Status: "verification_pending"}); err != nil {
		return db.PageUpdateDraft{}, err
	}
	return draft, nil
}

func (s *Server) markPageUpdateDraftManualApply(ctx context.Context, projectID uuid.UUID, claimed db.PageUpdateDraft, action db.ContentAction, reason string) (db.PageUpdateDraft, error) {
	result := mustJSONLocal(map[string]any{
		"mode":       "manual_patch",
		"status":     "manual_apply_required",
		"target_url": claimed.TargetUrl,
		"reason":     reason,
	})
	draft, err := s.Q.MarkPageUpdateDraftApplyResult(ctx, db.MarkPageUpdateDraftApplyResultParams{
		ID:              claimed.ID,
		ProjectID:       projectID,
		Status:          "manual_apply_required",
		PublisherResult: result,
	})
	if err != nil {
		return db.PageUpdateDraft{}, err
	}
	if _, err := s.Q.UpdateContentActionStatus(ctx, db.UpdateContentActionStatusParams{ID: action.ID, ProjectID: projectID, Status: "manual_apply_required"}); err != nil {
		return db.PageUpdateDraft{}, err
	}
	return draft, nil
}

func (s *Server) createSiteFixGitHubPR(w http.ResponseWriter, r *http.Request) {
	projectID, actionID, ok := s.seoIDs(w, r, "actionID")
	if !ok {
		return
	}
	action, err := s.Q.GetContentAction(r.Context(), db.GetContentActionParams{ID: actionID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "content action not found")
		return
	}
	if !contentActionIsSiteFix(action) {
		writeErr(w, http.StatusBadRequest, "content action is not a site fix")
		return
	}
	updated, err := s.createSiteFixGitHubPRForAction(r.Context(), projectID, action)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) createSiteFixGitHubPRForAction(ctx context.Context, projectID uuid.UUID, action db.ContentAction) (db.ContentAction, error) {
	if siteFixAssetTypeForAction(action) != "metadata_rewrite" {
		return db.ContentAction{}, fmt.Errorf("GitHub PR apply currently supports metadata rewrite site fixes")
	}
	conn, err := s.Q.GetEnabledPublisherConnectionForProject(ctx, db.GetEnabledPublisherConnectionForProjectParams{
		ProjectID: projectID,
		Kind:      publisher.ConnectionKindGitHubNextJS,
	})
	if err != nil {
		return db.ContentAction{}, err
	}
	cfg, err := publisher.ParseGitHubNextJSConfig(conn.Config)
	if err != nil {
		return db.ContentAction{}, err
	}
	targetURL := contentActionTargetURL(action)
	if targetURL == "" {
		return db.ContentAction{}, fmt.Errorf("site fix target URL is missing")
	}
	if title, description := siteFixProposedMetadata(action); title == "" && description == "" {
		action, err = s.generateSiteFixAIProposal(ctx, projectID, action)
		if err != nil {
			return db.ContentAction{}, err
		}
	}
	if target, ok := publisher.GitHubNextJSTargetForSiteURL(targetURL); ok {
		cfg.Branch = target.Branch
		cfg.BaseURL = target.BaseURL
	}
	token, err := s.publisherConnectionToken(ctx, projectID, conn)
	if err != nil || strings.TrimSpace(token) == "" {
		if err == nil {
			err = fmt.Errorf("github publisher token is empty")
		}
		return db.ContentAction{}, err
	}
	client := publisher.NewGitHubPRClient(token, cfg.Repo, cfg.Branch, s.Log)
	resolved, err := s.resolveSiteFixGitHubPRSource(ctx, projectID, action, cfg, client)
	if err != nil {
		return db.ContentAction{}, err
	}
	mapping := resolved.Mapping
	baseContent := resolved.BaseContent
	baseFileSHA := resolved.BaseFileSHA
	proposedContent := resolved.ProposedContent

	workingBranch := siteFixWorkingBranch(action)
	repo := cfg.Repo
	baseBranch := cfg.Branch
	sourceFilePath := mapping.SourceFilePath
	sourceFilePaths := mustJSONLocal([]string{sourceFilePath})
	baseCommitSHA := strPtrFrom(mapping.BaseCommitSHA)
	proposedHash := pageUpdateContentHash(proposedContent)
	app, err := s.Q.CreateOrReuseSiteChangeApplication(ctx, db.CreateOrReuseSiteChangeApplicationParams{
		ProjectID:               projectID,
		SourceOpportunityID:     pgtype.UUID{Bytes: action.OpportunityID, Valid: true},
		ContentActionID:         pgtype.UUID{Bytes: action.ID, Valid: true},
		PageUpdateDraftID:       pgtype.UUID{},
		ApplicationKind:         "site_fix",
		TargetUrl:               targetURL,
		NormalizedTargetUrl:     contentActionNormalizedTargetURL(action),
		OpportunityKey:          siteFixOpportunityKey(action),
		PublisherConnectionID:   pgtype.UUID{Bytes: conn.ID, Valid: true},
		RepoFullName:            &repo,
		BaseBranch:              &baseBranch,
		WorkingBranch:           &workingBranch,
		BaseCommitSha:           baseCommitSHA,
		HeadCommitSha:           nil,
		SourceFilePath:          &sourceFilePath,
		SourceFilePaths:         sourceFilePaths,
		SourceMappingConfidence: mapping.Confidence,
		SourceMappingReason:     mapping.Reason,
		BaseFileSha:             &baseFileSHA,
		BaseContentHash:         action.TargetContentHashBefore,
		ProposedContentHash:     &proposedHash,
		PatchSnapshot:           rawOrDefault(action.OutputSnapshot, `{}`),
		DiffSnapshot:            rawOrDefault(action.DiffSnapshot, `{}`),
		ResolutionCriteria:      siteFixResolutionCriteria(action),
		Status:                  "ready_for_pr",
	})
	if err != nil {
		return db.ContentAction{}, err
	}
	if app.Status == "github_pr_open" && strings.TrimSpace(stringPtrValueAPI(app.GithubPrUrl)) != "" {
		return s.markSiteFixGitHubPRResult(ctx, projectID, action, app)
	}
	if app.Status == "verification_pending" && strings.TrimSpace(stringPtrValueAPI(app.GithubPrUrl)) == "" {
		return s.markSiteFixAlreadyAppliedResult(ctx, projectID, action, app)
	}
	if proposedContent == baseContent {
		app, err = s.Q.MarkSiteChangeApplicationStatus(ctx, db.MarkSiteChangeApplicationStatusParams{
			ID:                 app.ID,
			ProjectID:          projectID,
			Status:             "verification_pending",
			GithubPrState:      nil,
			DeploymentSnapshot: siteFixAlreadyAppliedDeploymentSnapshot(action, mapping),
			VerificationSnapshot: mustJSONLocal(map[string]any{
				"status":           "pending_production_verification",
				"source_file_path": mapping.SourceFilePath,
				"target_url":       contentActionTargetURL(action),
			}),
			FailureReason: nil,
		})
		if err != nil {
			return db.ContentAction{}, err
		}
		return s.markSiteFixAlreadyAppliedResult(ctx, projectID, action, app)
	}
	pr, err := client.CreatePageUpdatePR(ctx, publisher.GitHubPRInput{
		SourcePath:        mapping.SourceFilePath,
		WorkingBranch:     workingBranch,
		BaseFileSHA:       baseFileSHA,
		ProposedContentMD: proposedContent,
		CommitMessage:     siteFixPRCommitMessage(action),
		Title:             siteFixPRTitle(action),
		Body:              siteFixPRBody(action, mapping),
	})
	if err != nil {
		_, _ = s.Q.MarkSiteChangeApplicationStatus(ctx, db.MarkSiteChangeApplicationStatusParams{
			ID:                   app.ID,
			ProjectID:            projectID,
			Status:               "manual_apply_required",
			GithubPrState:        nil,
			DeploymentSnapshot:   json.RawMessage(`{}`),
			VerificationSnapshot: json.RawMessage(`{}`),
			FailureReason:        strPtrFrom(err.Error()),
		})
		return db.ContentAction{}, err
	}
	prNumber := int32(pr.Number)
	app, err = s.Q.MarkSiteChangeApplicationGitHubPR(ctx, db.MarkSiteChangeApplicationGitHubPRParams{
		ID:             app.ID,
		ProjectID:      projectID,
		WorkingBranch:  &pr.WorkingBranch,
		HeadCommitSha:  &pr.HeadCommitSHA,
		BaseFileSha:    &pr.BaseFileSHA,
		GithubPrNumber: &prNumber,
		GithubPrUrl:    &pr.URL,
		GithubPrState:  &pr.State,
	})
	if err != nil {
		return db.ContentAction{}, err
	}
	return s.markSiteFixGitHubPRResult(ctx, projectID, action, app)
}

type siteFixSourceReader interface {
	ReadFile(ctx context.Context, sourcePath, ref string) (string, string, error)
}

type siteFixResolvedSource struct {
	Mapping         pageUpdateSourceMapping
	BaseContent     string
	BaseFileSHA     string
	ProposedContent string
}

func (s *Server) resolveSiteFixGitHubPRSource(ctx context.Context, projectID uuid.UUID, action db.ContentAction, cfg publisher.GitHubNextJSConfig, reader siteFixSourceReader) (siteFixResolvedSource, error) {
	mappings := []pageUpdateSourceMapping{}
	if action.TargetArticleID.Valid {
		article, err := s.Q.GetArticleForProject(ctx, db.GetArticleForProjectParams{ID: uuid.UUID(action.TargetArticleID.Bytes), ProjectID: projectID})
		if err != nil {
			return siteFixResolvedSource{}, err
		}
		if mapping, ok := pageUpdateExactSourceMapping(article); ok {
			mappings = append(mappings, mapping)
		}
	}
	mappings = append(mappings, siteFixMetadataRewriteSourceCandidates(action, cfg)...)
	mappings = uniquePageUpdateSourceMappings(mappings)
	if len(mappings) == 0 {
		return siteFixResolvedSource{}, fmt.Errorf("site fix could not infer a source file for %s; copy the fix JSON for manual application", contentActionTargetURL(action))
	}

	var lastErr error
	for _, mapping := range mappings {
		baseContent, baseFileSHA, err := reader.ReadFile(ctx, mapping.SourceFilePath, cfg.Branch)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", mapping.SourceFilePath, err)
			continue
		}
		proposedContent, err := siteFixMetadataRewriteContent(baseContent, action)
		if err != nil {
			if isSiteFixMissingProposedMetadataError(err) {
				return siteFixResolvedSource{}, fmt.Errorf("%s: %w", mapping.SourceFilePath, err)
			}
			lastErr = fmt.Errorf("%s: %w", mapping.SourceFilePath, err)
			continue
		}
		return siteFixResolvedSource{
			Mapping:         mapping,
			BaseContent:     baseContent,
			BaseFileSHA:     baseFileSHA,
			ProposedContent: proposedContent,
		}, nil
	}
	if lastErr != nil {
		return siteFixResolvedSource{}, fmt.Errorf("site fix could not locate a supported metadata source file for %s: %w", contentActionTargetURL(action), lastErr)
	}
	return siteFixResolvedSource{}, fmt.Errorf("site fix could not locate a supported metadata source file for %s", contentActionTargetURL(action))
}

type siteFixAIContract struct {
	Hash            string         `json:"hash"`
	Issue           map[string]any `json:"issue"`
	TargetURL       string         `json:"target_url"`
	AssetType       string         `json:"asset_type"`
	ActionType      string         `json:"action_type"`
	Observed        map[string]any `json:"observed"`
	Constraints     []string       `json:"constraints"`
	AcceptanceTests []string       `json:"acceptance_tests"`
}

type siteFixAIProposal struct {
	ProposedChange struct {
		Title           string `json:"title"`
		MetaDescription string `json:"meta_description"`
	} `json:"proposed_change"`
	EvidenceAlignment []string `json:"evidence_alignment,omitempty"`
	RiskNotes         []string `json:"risk_notes,omitempty"`
	Rationale         string   `json:"rationale,omitempty"`
}

func (s *Server) generateSiteFixAIProposal(ctx context.Context, projectID uuid.UUID, action db.ContentAction) (db.ContentAction, error) {
	contract := buildSiteFixAIContract(action)
	promptRaw, err := json.MarshalIndent(contract, "", "  ")
	if err != nil {
		return db.ContentAction{}, err
	}
	req := llm.CompletionReq{
		System:    "You are CiteLoop Site Fix. Generate narrow crawler-facing metadata changes for an existing production page. Return only reviewed JSON.",
		Prompt:    siteFixAIProposalPrompt(string(promptRaw)),
		Purpose:   llm.PurposeSiteFix,
		JSON:      true,
		MaxTokens: 900,
	}
	var resp llm.CompletionResp
	callID := uuid.Nil
	if s.AICalls == nil {
		if s.LLM == nil {
			return db.ContentAction{}, fmt.Errorf("site fix AI model is not configured")
		}
		resp, err = s.LLM.Complete(ctx, req)
	} else {
		completion, completionErr := aicalls.New(s.AICalls).Complete(ctx, aicalls.Spec{
			ProjectID: projectID, Stage: "fix_generation", LinkedObjectType: "content_action", LinkedObjectID: action.ID,
			Provider: "runtime_route", Model: "runtime_route", PromptVersion: "legacy-site-fix-proposal-v2",
			RequestFingerprint: aicalls.Fingerprint(req),
		}, s.LLM, req)
		resp, callID, err = completion.Response, completion.Call.ID, completionErr
	}
	if err != nil {
		return db.ContentAction{}, fmt.Errorf("site fix AI proposal failed: %w", err)
	}
	proposal, err := parseSiteFixAIProposal(resp.Text)
	if err != nil {
		if callID != uuid.Nil {
			if _, ledgerErr := aicalls.New(s.AICalls).FailOutput(context.WithoutCancel(ctx), callID, projectID, "invalid_response"); ledgerErr != nil {
				return db.ContentAction{}, errors.Join(err, ledgerErr)
			}
		}
		return db.ContentAction{}, fmt.Errorf("site fix AI proposal invalid: %w", err)
	}
	if err := validateSiteFixAIProposal(action, proposal); err != nil {
		if callID != uuid.Nil {
			if _, ledgerErr := aicalls.New(s.AICalls).FailOutput(context.WithoutCancel(ctx), callID, projectID, "invalid_output"); ledgerErr != nil {
				return db.ContentAction{}, errors.Join(err, ledgerErr)
			}
		}
		return db.ContentAction{}, err
	}
	output, diff := siteFixAIProposalSnapshots(action, contract, proposal, resp)
	return s.Q.UpdateContentActionExecutionMetadata(ctx, db.UpdateContentActionExecutionMetadataParams{
		AssetType:            action.AssetType,
		TargetSurfaceID:      action.TargetSurfaceID,
		RiskReasons:          rawOrDefault(action.RiskReasons, `[]`),
		EvidenceSnapshot:     rawOrDefault(action.EvidenceSnapshot, `{}`),
		InputSnapshot:        rawOrDefault(action.InputSnapshot, `{}`),
		OutputSnapshot:       output,
		DiffSnapshot:         diff,
		ReviewRequired:       action.ReviewRequired,
		ApprovedBy:           action.ApprovedBy,
		ApprovedAt:           action.ApprovedAt,
		VerifiedAt:           action.VerifiedAt,
		VerificationSnapshot: rawOrDefault(action.VerificationSnapshot, `{}`),
		ID:                   action.ID,
		ProjectID:            projectID,
	})
}

func buildSiteFixAIContract(action db.ContentAction) siteFixAIContract {
	assetType := siteFixAssetTypeForAction(action)
	targetURL := contentActionTargetURL(action)
	title, description := siteFixProposedMetadata(action)
	contract := siteFixAIContract{
		Issue: compactActionPayload(map[string]any{
			"category":      "site_fix",
			"issue_type":    assetType,
			"affected_urls": nonEmptyStrings(targetURL),
			"problem":       action.ActionType,
		}),
		TargetURL:  targetURL,
		AssetType:  assetType,
		ActionType: strings.TrimSpace(action.ActionType),
		Observed: map[string]any{
			"target_url":                 targetURL,
			"current_title":              firstActionSnapshotMetadataString(action, "title", "page_title", "current_title", "observed_title"),
			"current_meta_description":   firstActionSnapshotMetadataString(action, "meta_description", "description", "current_meta_description", "observed_meta_description"),
			"canonical":                  firstActionSnapshotMetadataString(action, "canonical", "canonical_url"),
			"query":                      firstActionSnapshotMetadataString(action, "query", "target_query"),
			"proposed_title":             title,
			"proposed_meta_description":  description,
			"has_explicit_proposed_copy": title != "" || description != "",
		},
		Constraints: []string{
			"Return concrete title and meta_description values.",
			"Do not create a new page or blog post.",
			"Do not use the opportunity title as the page title unless it is a natural production SEO title.",
			"Preserve canonical URL, indexability, production host, and existing route.",
			"Use production brand and page context only; do not invent unsupported product capabilities.",
		},
		AcceptanceTests: siteFixAcceptanceTests(action),
	}
	contract.Hash = siteFixAIContractHash(contract)
	return contract
}

func siteFixAIContractHash(contract siteFixAIContract) string {
	contract.Hash = ""
	raw, _ := json.Marshal(contract)
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("sha256:%x", sum[:])
}

func firstActionSnapshotMetadataString(action db.ContentAction, keys ...string) string {
	for _, raw := range []json.RawMessage{action.OutputSnapshot, action.DiffSnapshot, action.EvidenceSnapshot, action.InputSnapshot} {
		if !rawHasMeaningfulJSON(raw) {
			continue
		}
		var parsed any
		if err := json.Unmarshal(raw, &parsed); err != nil {
			continue
		}
		if value := firstEvidenceString(parsed, keys); value != "" {
			return value
		}
	}
	return ""
}

func siteFixAIProposalPrompt(contractJSON string) string {
	return `Use this CiteLoop site-fix contract to propose crawler-facing metadata for the existing target page.

Return exactly one JSON object:
{
  "proposed_change": {
    "title": "production page title",
    "meta_description": "production meta description"
  },
  "evidence_alignment": ["why this resolves the finding"],
  "risk_notes": ["anything a human should review"]
}

Rules:
- Do not create or suggest a new article, page, route, or visible content section.
- Keep the target URL and canonical intent unchanged.
- Use concise production copy that fits normal search snippets.
- If current proposed copy is missing, infer the smallest safe title/meta description from the contract.
- Never return placeholders, localhost, staging URLs, or process instructions as copy.

Contract:
` + contractJSON
}

func parseSiteFixAIProposal(text string) (siteFixAIProposal, error) {
	var proposal siteFixAIProposal
	raw, err := firstJSONObject(text)
	if err != nil {
		return proposal, err
	}
	if err := json.Unmarshal([]byte(raw), &proposal); err != nil {
		return proposal, err
	}
	proposal.ProposedChange.Title = strings.TrimSpace(proposal.ProposedChange.Title)
	proposal.ProposedChange.MetaDescription = strings.TrimSpace(proposal.ProposedChange.MetaDescription)
	return proposal, nil
}

func firstJSONObject(text string) (string, error) {
	start := strings.Index(text, "{")
	if start < 0 {
		return "", fmt.Errorf("response did not contain a JSON object")
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(text); i++ {
		ch := text[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("response JSON object was not balanced")
}

func validateSiteFixAIProposal(action db.ContentAction, proposal siteFixAIProposal) error {
	title := strings.TrimSpace(proposal.ProposedChange.Title)
	description := strings.TrimSpace(proposal.ProposedChange.MetaDescription)
	if title == "" && description == "" {
		return fmt.Errorf("site fix AI proposal has no title or meta description")
	}
	if strings.EqualFold(title, strings.TrimSpace(action.ActionType)) {
		return fmt.Errorf("site fix AI proposal used the opportunity title as page metadata")
	}
	for _, value := range []string{title, description} {
		normalized := strings.ToLower(value)
		if strings.Contains(normalized, "localhost") || strings.Contains(normalized, "staging") || strings.Contains(normalized, "placeholder") {
			return fmt.Errorf("site fix AI proposal contains non-production placeholder copy")
		}
	}
	return nil
}

func siteFixAIProposalSnapshots(action db.ContentAction, contract siteFixAIContract, proposal siteFixAIProposal, resp llm.CompletionResp) (json.RawMessage, json.RawMessage) {
	proposedChange := compactActionPayload(map[string]any{
		"title":            proposal.ProposedChange.Title,
		"meta_description": proposal.ProposedChange.MetaDescription,
	})
	publisherResult := map[string]any{
		"status":               "ai_preview_ready",
		"mode":                 "site_fix_ai_pr",
		"ai_fix_contract_hash": contract.Hash,
		"ai_fix_contract":      contract,
		"ai_proposal":          proposal,
		"model":                resp.Model,
		"tokens":               resp.Tokens,
		"cost_usd":             resp.CostUSD,
	}
	output := mergeRawJSONObject(action.OutputSnapshot, map[string]any{
		"publisher_result": publisherResult,
		"proposed_change":  proposedChange,
	})
	diff := mergeRawJSONObject(action.DiffSnapshot, map[string]any{
		"ai_site_fix": map[string]any{
			"contract_hash":   contract.Hash,
			"proposed_change": proposedChange,
		},
		"proposed_metadata": proposedChange,
	})
	return output, diff
}

func mergeRawJSONObject(raw json.RawMessage, values map[string]any) json.RawMessage {
	merged := map[string]any{}
	if rawHasMeaningfulJSON(raw) {
		_ = json.Unmarshal(raw, &merged)
	}
	for key, value := range values {
		merged[key] = value
	}
	return mustJSONLocal(merged)
}

func uniquePageUpdateSourceMappings(mappings []pageUpdateSourceMapping) []pageUpdateSourceMapping {
	seen := map[string]bool{}
	out := []pageUpdateSourceMapping{}
	for _, mapping := range mappings {
		path := strings.TrimSpace(mapping.SourceFilePath)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		mapping.SourceFilePath = path
		out = append(out, mapping)
	}
	return out
}

func (s *Server) markSiteFixGitHubPRResult(ctx context.Context, projectID uuid.UUID, action db.ContentAction, app db.SiteChangeApplication) (db.ContentAction, error) {
	result := siteFixGitHubPRResult(action, app)
	return s.Q.MarkContentActionSiteFixPRResult(ctx, db.MarkContentActionSiteFixPRResultParams{
		ID:              action.ID,
		ProjectID:       projectID,
		PublisherResult: result,
	})
}

func (s *Server) markSiteFixAlreadyAppliedResult(ctx context.Context, projectID uuid.UUID, action db.ContentAction, app db.SiteChangeApplication) (db.ContentAction, error) {
	result := siteFixAlreadyAppliedResult(action, app)
	return s.Q.MarkContentActionSiteFixPRResult(ctx, db.MarkContentActionSiteFixPRResultParams{
		ID:              action.ID,
		ProjectID:       projectID,
		PublisherResult: result,
	})
}

func siteFixGitHubPRResult(action db.ContentAction, app db.SiteChangeApplication) json.RawMessage {
	return mustJSONLocal(map[string]any{
		"mode":                       "github_pr",
		"status":                     "github_pr_open",
		"site_change_application_id": app.ID,
		"github_pr_number":           app.GithubPrNumber,
		"github_pr_url":              app.GithubPrUrl,
		"github_pr_state":            app.GithubPrState,
		"repo":                       app.RepoFullName,
		"base_branch":                app.BaseBranch,
		"working_branch":             app.WorkingBranch,
		"head_commit_sha":            app.HeadCommitSha,
		"base_file_sha":              app.BaseFileSha,
		"source_file_path":           app.SourceFilePath,
		"target_url":                 contentActionTargetURL(action),
	})
}

func siteFixAlreadyAppliedResult(action db.ContentAction, app db.SiteChangeApplication) json.RawMessage {
	return mustJSONLocal(map[string]any{
		"mode":                       "github_pr",
		"status":                     "already_applied",
		"site_change_application_id": app.ID,
		"repo":                       app.RepoFullName,
		"base_branch":                app.BaseBranch,
		"working_branch":             app.WorkingBranch,
		"base_file_sha":              app.BaseFileSha,
		"source_file_path":           app.SourceFilePath,
		"target_url":                 contentActionTargetURL(action),
		"message":                    "The mapped source file already contains the proposed metadata. Verify production before marking the site fix applied.",
	})
}

func siteFixAlreadyAppliedDeploymentSnapshot(action db.ContentAction, mapping pageUpdateSourceMapping) json.RawMessage {
	return mustJSONLocal(map[string]any{
		"mode":                  "github_pr",
		"status":                "already_applied",
		"source_file_path":      mapping.SourceFilePath,
		"source_mapping_reason": mapping.Reason,
		"target_url":            contentActionTargetURL(action),
	})
}

func (s *Server) verifyPageUpdateDraft(w http.ResponseWriter, r *http.Request) {
	projectID, draftID, ok := s.seoIDs(w, r, "draftID")
	if !ok {
		return
	}
	var in struct {
		Status               string          `json:"status"`
		VerificationSnapshot json.RawMessage `json:"verification_snapshot"`
	}
	if err := decodeOptionalJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	draft, action, err := s.pageUpdateDraftWithAction(r.Context(), projectID, draftID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "page update draft not found")
		return
	}
	requested := strings.ToLower(strings.TrimSpace(in.Status))
	if requested == "" {
		requested = "verified"
	}
	draftStatus := "verified"
	parentStatus := "measuring"
	verifiedAt := pgutil.TS(time.Now().UTC())
	switch requested {
	case "verified", "ok", "passed":
		draftStatus = "verified"
		parentStatus = "measuring"
	case "needs_follow_up", "follow_up":
		draftStatus = "needs_follow_up"
		parentStatus = "needs_follow_up"
		verifiedAt = pgtype.Timestamptz{}
	case "failed", "verification_failed":
		draftStatus = "verification_failed"
		parentStatus = "verification_failed"
		verifiedAt = pgtype.Timestamptz{}
	default:
		writeErr(w, http.StatusBadRequest, "bad verification status")
		return
	}
	snapshot := in.VerificationSnapshot
	if len(snapshot) == 0 || !json.Valid(snapshot) {
		snapshot = mustJSONLocal(map[string]any{"source": "manual_page_update_verify", "status": draftStatus, "target_url": draft.TargetUrl})
	}
	draft, err = s.Q.MarkPageUpdateDraftVerification(r.Context(), db.MarkPageUpdateDraftVerificationParams{
		ID:                   draft.ID,
		ProjectID:            projectID,
		Status:               draftStatus,
		VerificationSnapshot: snapshot,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.Q.MarkContentActionVerification(r.Context(), db.MarkContentActionVerificationParams{
		ID:                   action.ID,
		ProjectID:            projectID,
		Status:               parentStatus,
		VerifiedAt:           verifiedAt,
		VerificationSnapshot: snapshot,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, draft)
}

func (s *Server) pageUpdateDraftWithAction(ctx context.Context, projectID uuid.UUID, draftID uuid.UUID) (db.PageUpdateDraft, db.ContentAction, error) {
	draft, err := s.Q.GetPageUpdateDraftForProject(ctx, db.GetPageUpdateDraftForProjectParams{ID: draftID, ProjectID: projectID})
	if err != nil {
		return db.PageUpdateDraft{}, db.ContentAction{}, err
	}
	action, err := s.Q.GetContentAction(ctx, db.GetContentActionParams{ID: draft.ContentActionID, ProjectID: projectID})
	if err != nil {
		return db.PageUpdateDraft{}, db.ContentAction{}, err
	}
	return draft, action, nil
}

type pageUpdateSourceMapping struct {
	SourceFilePath string
	BaseCommitSHA  string
	Confidence     string
	Reason         string
}

func pageUpdateExactSourceMapping(article db.Article) (pageUpdateSourceMapping, bool) {
	publishPath := strings.TrimSpace(stringPtrValueAPI(article.PublishPath))
	var result struct {
		Path      string `json:"path"`
		CommitSHA string `json:"commit_sha"`
	}
	if len(article.PublishResult) > 0 && json.Valid(article.PublishResult) {
		_ = json.Unmarshal(article.PublishResult, &result)
		if publishPath == "" {
			publishPath = strings.TrimSpace(result.Path)
		}
	}
	if publishPath == "" {
		return pageUpdateSourceMapping{}, false
	}
	lower := strings.ToLower(publishPath)
	if !strings.HasSuffix(lower, ".mdx") && !strings.HasSuffix(lower, ".md") {
		return pageUpdateSourceMapping{}, false
	}
	return pageUpdateSourceMapping{
		SourceFilePath: publishPath,
		BaseCommitSHA:  strings.TrimSpace(result.CommitSHA),
		Confidence:     "exact",
		Reason:         "Matched CiteLoop-published article publish path.",
	}, true
}

func siteFixMetadataRewriteSourceCandidates(action db.ContentAction, cfg publisher.GitHubNextJSConfig) []pageUpdateSourceMapping {
	if siteFixAssetTypeForAction(action) != "metadata_rewrite" {
		return nil
	}
	targetURL := contentActionTargetURL(action)
	if strings.TrimSpace(targetURL) == "" {
		return nil
	}
	candidates := []pageUpdateSourceMapping{}
	add := func(sourcePath, confidence, reason string) {
		sourcePath = cleanSiteFixSourcePath(sourcePath)
		if sourcePath == "" {
			return
		}
		candidates = append(candidates, pageUpdateSourceMapping{
			SourceFilePath: sourcePath,
			Confidence:     confidence,
			Reason:         reason,
		})
	}

	if slug, ok := siteFixRelativeSlugForBaseURL(targetURL, cfg.BaseURL); ok && slug != "" {
		contentDir := cleanSiteFixSourcePath(cfg.ContentDir)
		if contentDir != "" {
			add(contentDir+"/"+slug+".mdx", "derived", "Derived Markdown source path from publisher content directory and target URL.")
			add(contentDir+"/"+slug+".md", "derived", "Derived Markdown source path from publisher content directory and target URL.")
		}
	}

	_, routePath, ok := siteFixURLHostPath(targetURL)
	if !ok {
		return uniquePageUpdateSourceMappings(candidates)
	}
	for _, root := range []string{"dashboard/src/app", "src/app", "app"} {
		if routePath == "" {
			add(root+"/marketing/page.tsx", "heuristic", "Matched homepage to a common Next.js marketing metadata route.")
			add(root+"/page.tsx", "heuristic", "Matched homepage to a Next.js App Router metadata source candidate.")
			add(root+"/layout.tsx", "heuristic", "Matched homepage to a Next.js App Router metadata source candidate.")
			continue
		}
		add(root+"/"+routePath+"/page.tsx", "heuristic", "Matched target URL path to a Next.js App Router metadata source candidate.")
		add(root+"/"+routePath+"/layout.tsx", "heuristic", "Matched target URL path to a Next.js App Router metadata source candidate.")
	}
	return uniquePageUpdateSourceMappings(candidates)
}

func siteFixRelativeSlugForBaseURL(targetRaw, baseRaw string) (string, bool) {
	targetHost, targetPath, ok := siteFixURLHostPath(targetRaw)
	if !ok {
		return "", false
	}
	baseHost, basePath, ok := siteFixURLHostPath(baseRaw)
	if !ok || targetHost != baseHost {
		return "", false
	}
	if basePath == "" {
		return targetPath, true
	}
	if targetPath == basePath {
		return "", true
	}
	prefix := basePath + "/"
	if strings.HasPrefix(targetPath, prefix) {
		return strings.TrimPrefix(targetPath, prefix), true
	}
	return "", false
}

func siteFixURLHostPath(raw string) (string, string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", false
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Hostname() == "" {
		return "", "", false
	}
	return strings.ToLower(parsed.Hostname()), cleanSiteFixSourcePath(parsed.Path), true
}

func cleanSiteFixSourcePath(raw string) string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(raw), "/"), "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		cleaned = append(cleaned, part)
	}
	return strings.Join(cleaned, "/")
}

func pageUpdateWorkingBranch(draft db.PageUpdateDraft) string {
	suffix := strings.ReplaceAll(draft.ID.String(), "-", "")
	if len(suffix) > 12 {
		suffix = suffix[:12]
	}
	return "citeloop/page-update-" + suffix
}

func pageUpdateContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("sha256:%x", sum)
}

func pageUpdatePRTitle(action db.ContentAction) string {
	title := compactPageUpdateText(action.ActionType)
	if title == "" {
		title = "Apply page update"
	}
	if len(title) > 88 {
		title = strings.TrimSpace(title[:88])
	}
	return "CiteLoop: " + title
}

func pageUpdatePRCommitMessage(action db.ContentAction) string {
	title := compactPageUpdateText(action.ActionType)
	if title == "" {
		title = "Apply CiteLoop page update"
	}
	return title
}

func pageUpdatePRBody(draft db.PageUpdateDraft, action db.ContentAction, mapping pageUpdateSourceMapping) string {
	criteria := compactJSONSummary(draft.ResolutionCriteria)
	if criteria == "" {
		criteria = "No structured criteria were recorded."
	}
	return fmt.Sprintf(`## CiteLoop Page Update

Target URL: %s
Opportunity key: %s
Source file: %s
Source mapping: %s

## Requested Change

%s

## Resolution Criteria

%s

After this PR merges and deploys, CiteLoop should verify the target page before closing the opportunity.
`,
		draft.TargetUrl,
		draft.OpportunityKey,
		mapping.SourceFilePath,
		mapping.Reason,
		compactPageUpdateText(action.ActionType),
		criteria,
	)
}

func jsonOrEmptyObject(raw json.RawMessage) json.RawMessage {
	if len(raw) > 0 && json.Valid(raw) {
		return raw
	}
	return json.RawMessage(`{}`)
}

func compactPageUpdateText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func (s *Server) existingTopicForContentAction(ctx context.Context, projectID uuid.UUID, actionID uuid.UUID) (db.Topic, bool, error) {
	topics, err := s.Q.ListTopics(ctx, projectID)
	if err != nil {
		return db.Topic{}, false, err
	}
	for _, topic := range topics {
		if topic.SourceContentActionID.Valid && uuid.UUID(topic.SourceContentActionID.Bytes) == actionID {
			return topic, true, nil
		}
	}
	return db.Topic{}, false, nil
}

func (s *Server) listResultsActions(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	limit, err := parseLimit(r, 50, 100)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad limit")
		return
	}
	cursor, err := parseCursor(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad cursor")
		return
	}
	actions, err := s.resultsActionsForProject(r.Context(), projectID, r.URL.Query().Get("status"), limit, cursor)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(actions))
}

func (s *Server) getResultsAction(w http.ResponseWriter, r *http.Request) {
	projectID, actionID, ok := s.seoIDs(w, r, "actionID")
	if !ok {
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	row, err := s.Q.GetResultsActionRow(r.Context(), db.GetResultsActionRowParams{ID: actionID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "action not found")
		return
	}
	measurements, err := s.Q.ListActionMeasurementsForAction(r.Context(), db.ListActionMeasurementsForActionParams{
		ProjectID:       projectID,
		ContentActionID: actionID,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	action := resultsActionFromGetRow(row)
	attachResultsMeasurements(&action, measurements)
	writeJSON(w, http.StatusOK, action)
}

func (s *Server) listGrowthLearnings(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	limit, err := parseLimit(r, 50, 100)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad limit")
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	learnings, err := s.Q.ListGrowthLearnings(r.Context(), db.ListGrowthLearningsParams{
		ProjectID: projectID,
		LimitRows: int32(limit),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	qualityRecords, err := s.Q.ListMeasurementQualityRecords(r.Context(), db.ListMeasurementQualityRecordsParams{
		ProjectID: projectID,
		LimitRows: int32(limit),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	feed := growthLearningFeed(learnings, qualityRecords)
	if len(feed) > limit {
		feed = feed[:limit]
	}
	writeJSON(w, http.StatusOK, emptySlice(feed))
}

func growthLearningFeed(learnings []db.ListGrowthLearningsRow, quality []db.ListMeasurementQualityRecordsRow) []GrowthLearningFeedItem {
	feed := make([]GrowthLearningFeedItem, 0, len(learnings)+len(quality))
	for _, learning := range learnings {
		feed = append(feed, GrowthLearningFeedItem{
			ID: learning.ID, ProjectID: learning.ProjectID, OpportunityID: learning.OpportunityID,
			ContentActionID: learning.ContentActionID, ArticleID: optionalUUID(learning.ArticleID),
			ArtifactURL: learning.ArtifactUrl, RecordKind: "directional_learning",
			LearningSummary: learning.LearningSummary, Applicability: learning.Applicability,
			ScoringEligible: learning.ScoringEligible, LearningVersion: learning.LearningVersion,
			ActionFamily: learning.ActionFamily, TargetIdentity: learning.TargetIdentity, Audience: learning.Audience,
			PrimaryMetric: learning.PrimaryMetric, OutcomeLabel: learning.OutcomeLabel,
			TerminalReason: learning.TerminalReason, MeasurementPolicyVersion: learning.MeasurementPolicyVersion,
			BaselineSnapshot: learning.BaselineSnapshot, CheckpointSnapshot: learning.CheckpointSnapshot,
			OutcomeSnapshot: learning.OutcomeSnapshot, CreatedAt: optionalTime(learning.CreatedAt),
		})
	}
	for _, record := range quality {
		feed = append(feed, GrowthLearningFeedItem{
			ID: record.ID, ProjectID: record.ProjectID, OpportunityID: record.OpportunityID,
			ContentActionID: record.ContentActionID, ArticleID: optionalUUID(record.ArticleID),
			ArtifactURL: record.ArtifactUrl, RecordKind: "measurement_quality",
			LearningSummary: record.Recommendation, Applicability: json.RawMessage(`{}`),
			ScoringEligible: record.ScoringEligible, LearningVersion: record.QualityVersion,
			ActionFamily: record.ActionFamily, TargetIdentity: record.TargetIdentity, Audience: record.Audience,
			PrimaryMetric: record.PrimaryMetric, OutcomeLabel: record.OutcomeLabel,
			TerminalReason: record.TerminalReason, MeasurementPolicyVersion: record.MeasurementPolicyVersion,
			BaselineSnapshot: record.BaselineSnapshot, CheckpointSnapshot: record.CheckpointSnapshot,
			OutcomeSnapshot: record.OutcomeSnapshot, DataQualityState: record.DataQualityState,
			QualityGaps: record.QualityGaps, Recommendation: record.Recommendation,
			CreatedAt: optionalTime(record.CreatedAt),
		})
	}
	sort.SliceStable(feed, func(i, j int) bool {
		if feed[i].CreatedAt == nil {
			return false
		}
		if feed[j].CreatedAt == nil {
			return true
		}
		return feed[i].CreatedAt.After(*feed[j].CreatedAt)
	})
	return feed
}

func optionalUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := uuid.UUID(value.Bytes)
	return &id
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func (s *Server) recomputeResults(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	status := "scheduler_unavailable"
	if s.Sched != nil {
		if err := s.Sched.RecomputeMeasurements(r.Context(), projectID); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		status = "recomputed"
	}
	actions, err := s.resultsActionsForProject(r.Context(), projectID, "", 50, time.Time{})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": status, "actions": emptySlice(actions)})
}

func (s *Server) resultsActionsForProject(ctx context.Context, projectID uuid.UUID, status string, limit int, cursor time.Time) ([]ResultsAction, error) {
	if s.Q == nil {
		return nil, fmt.Errorf("database unavailable")
	}
	params := db.ListResultsActionRowsParams{
		ProjectID: projectID,
		Status:    status,
		LimitRows: int32(limit),
	}
	if !cursor.IsZero() {
		params.CursorUpdatedAt = pgutil.TS(cursor)
	}
	rows, err := s.Q.ListResultsActionRows(ctx, params)
	if err != nil {
		return nil, err
	}
	measurementLimit := limit * 8
	if measurementLimit < 100 {
		measurementLimit = 100
	}
	if measurementLimit > 500 {
		measurementLimit = 500
	}
	measurements, err := s.Q.ListActionMeasurementsForProject(ctx, db.ListActionMeasurementsForProjectParams{
		ProjectID: projectID,
		LimitRows: int32(measurementLimit),
	})
	if err != nil {
		return nil, err
	}
	grouped := map[uuid.UUID][]ActionMeasurement{}
	for _, measurement := range measurements {
		grouped[measurement.ContentActionID] = append(grouped[measurement.ContentActionID], measurement)
	}
	actions := make([]ResultsAction, 0, len(rows))
	for _, row := range rows {
		action := resultsActionFromListRow(row)
		attachResultsMeasurements(&action, grouped[row.ID])
		actions = append(actions, action)
	}
	return actions, nil
}

func (s *Server) updateSEOContentActionStatus(w http.ResponseWriter, r *http.Request, status string) {
	projectID, actionID, ok := s.seoIDs(w, r, "actionID")
	if !ok {
		return
	}
	action, err := s.Q.UpdateContentActionStatus(r.Context(), db.UpdateContentActionStatusParams{
		ID:        actionID,
		ProjectID: projectID,
		Status:    status,
	})
	if err != nil {
		writeErr(w, http.StatusNotFound, "action not found")
		return
	}
	writeJSON(w, http.StatusOK, action)
}

func writeContentActionMutationError(w http.ResponseWriter, err error, notFoundMessage, internalMessage string) {
	if errors.Is(err, pgx.ErrNoRows) {
		writeErr(w, http.StatusNotFound, notFoundMessage)
		return
	}
	writeErr(w, http.StatusInternalServerError, internalMessage)
}

func (s *Server) returnSEOContentActionToOpportunity(w http.ResponseWriter, r *http.Request) {
	projectID, actionID, ok := s.seoIDs(w, r, "actionID")
	if !ok {
		return
	}
	if s.Pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	q := db.New(tx)
	action, err := q.MarkContentActionReturnedToOpportunity(r.Context(), db.MarkContentActionReturnedToOpportunityParams{
		ActionID:  actionID,
		ProjectID: projectID,
	})
	if err != nil {
		writeContentActionMutationError(w, err, "action not found or no longer reversible", "could not move action back to Opportunities")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.enqueueWorkflowEvent(r.Context(), projectID, workflow.EventOpportunityReviewed, "seo_opportunity", action.OpportunityID, workflowDedupeKey(workflow.EventOpportunityReviewed, projectID, action.OpportunityID, "returned_to_opportunities"), map[string]any{
		"opportunity_id": action.OpportunityID,
		"action_id":      action.ID,
		"status":         "open",
		"action_status":  "returned",
		"reason":         "returned_to_opportunities",
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, action)
}

func (s *Server) dismissSEOContentActionAndOpportunity(w http.ResponseWriter, r *http.Request) {
	projectID, actionID, ok := s.seoIDs(w, r, "actionID")
	if !ok {
		return
	}
	if s.Pool == nil {
		writeErr(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	q := db.New(tx)
	action, err := q.DismissSEOContentActionAndOpportunity(r.Context(), db.DismissSEOContentActionAndOpportunityParams{
		ActionID:  actionID,
		ProjectID: projectID,
	})
	if err != nil {
		writeErr(w, http.StatusNotFound, "action not found or no longer dismissible")
		return
	}
	if err := s.recordSEOOpportunityReviewState(r.Context(), q, projectID, action.OpportunityID, pgtype.UUID{Bytes: action.ID, Valid: true}, "dismissed", pgtype.Timestamptz{}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.enqueueWorkflowEvent(r.Context(), projectID, workflow.EventOpportunityReviewed, "seo_opportunity", action.OpportunityID, workflowDedupeKey(workflow.EventOpportunityReviewed, projectID, action.OpportunityID, action.ID, "opportunity dismissed"), map[string]any{
		"opportunity_id": action.OpportunityID,
		"action_id":      action.ID,
		"status":         "dismissed",
		"action_status":  "dismissed",
		"reason":         "opportunity dismissed",
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, action)
}

func (s *Server) verifySEOContentAction(w http.ResponseWriter, r *http.Request) {
	projectID, actionID, ok := s.seoIDs(w, r, "actionID")
	if !ok {
		return
	}
	var in struct {
		Status               string          `json:"status"`
		VerificationSnapshot json.RawMessage `json:"verification_snapshot"`
	}
	if err := decodeOptionalJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	status := strings.ToLower(strings.TrimSpace(in.Status))
	if status == "" {
		status = "verified"
	}
	nextStatus := "measuring"
	verifiedAt := pgutil.TS(time.Now().UTC())
	switch status {
	case "verified", "ok", "passed":
		nextStatus = "measuring"
	case "failed", "verification_failed":
		nextStatus = "verification_failed"
		verifiedAt = pgtype.Timestamptz{}
	case "recovery_required":
		nextStatus = "recovery_required"
		verifiedAt = pgtype.Timestamptz{}
	default:
		writeErr(w, http.StatusBadRequest, "bad verification status")
		return
	}
	snapshot := in.VerificationSnapshot
	if len(snapshot) == 0 || !json.Valid(snapshot) {
		snapshot = mustJSONLocal(map[string]any{"source": "manual", "status": status})
	}
	action, err := s.Q.MarkContentActionVerification(r.Context(), db.MarkContentActionVerificationParams{
		ID:                   actionID,
		ProjectID:            projectID,
		Status:               nextStatus,
		VerifiedAt:           verifiedAt,
		VerificationSnapshot: snapshot,
	})
	if err != nil {
		writeErr(w, http.StatusNotFound, "action not found")
		return
	}
	writeJSON(w, http.StatusOK, action)
}

func (s *Server) getSEOBrief(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	brief, err := s.seoService().Brief(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, brief)
}

func (s *Server) getSEOSettings(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	var prop *db.SeoProperty
	if p, err := s.Q.GetSEOPropertyForProject(r.Context(), projectID); err == nil {
		prop = &p
	}
	integrations, err := s.Q.ListSEOIntegrations(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"property": prop, "integrations": emptySlice(integrations)})
}

func (s *Server) updateSEOSettings(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	var in struct {
		SiteURL        string          `json:"site_url"`
		GSCSiteURL     string          `json:"gsc_site_url"`
		GA4PropertyID  string          `json:"ga4_property_id"`
		Normalize      json.RawMessage `json:"url_normalization_config"`
		DefaultCountry string          `json:"default_country"`
		DefaultLang    string          `json:"default_language"`
		CredentialRef  string          `json:"gsc_credential_ref"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(in.SiteURL) == "" {
		writeErr(w, http.StatusBadRequest, "site_url required")
		return
	}
	if len(in.Normalize) == 0 {
		in.Normalize = json.RawMessage(`{}`)
	}
	prop, err := s.Q.UpsertSEOProperty(r.Context(), db.UpsertSEOPropertyParams{
		ProjectID:              projectID,
		SiteUrl:                strings.TrimSpace(in.SiteURL),
		GscSiteUrl:             strPtrFrom(in.GSCSiteURL),
		Ga4PropertyID:          strPtrFrom(in.GA4PropertyID),
		UrlNormalizationConfig: in.Normalize,
		DefaultCountry:         strPtrFrom(in.DefaultCountry),
		DefaultLanguage:        strPtrFrom(in.DefaultLang),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	existingIntegrations, err := s.Q.ListSEOIntegrations(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now().UTC()
	gscIntegration, err := s.Q.UpsertSEOIntegration(
		r.Context(),
		seoSettingsIntegrationParams(projectID, seopkg.ProviderGSC, in.CredentialRef, existingIntegrations, now, nil),
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var ga4Integration *db.SeoIntegration
	if strings.TrimSpace(in.GA4PropertyID) != "" || strings.TrimSpace(in.CredentialRef) != "" {
		row, err := s.Q.UpsertSEOIntegration(
			r.Context(),
			seoSettingsIntegrationParams(projectID, seopkg.ProviderGA4, in.CredentialRef, existingIntegrations, now, &gscIntegration),
		)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ga4Integration = &row
	}
	writeJSON(w, http.StatusOK, map[string]any{"property": prop, "integration": gscIntegration, "ga4_integration": ga4Integration})
}

func seoSettingsIntegrationParams(projectID uuid.UUID, provider string, credentialRef string, existing []db.SeoIntegration, now time.Time, fallback *db.SeoIntegration) db.UpsertSEOIntegrationParams {
	if ref := strings.TrimSpace(credentialRef); ref != "" {
		return db.UpsertSEOIntegrationParams{
			ProjectID:      projectID,
			Provider:       provider,
			Status:         "connected",
			CredentialRef:  &ref,
			LastVerifiedAt: pgutil.TS(now),
		}
	}
	for _, integration := range existing {
		if integration.Provider != provider {
			continue
		}
		if integration.Status == "missing" && fallback != nil && fallback.Status == "connected" && fallback.CredentialRef != nil {
			break
		}
		return db.UpsertSEOIntegrationParams{
			ProjectID:      projectID,
			Provider:       provider,
			Status:         integration.Status,
			CredentialRef:  integration.CredentialRef,
			LastVerifiedAt: integration.LastVerifiedAt,
			LastError:      integration.LastError,
		}
	}
	if fallback != nil && fallback.Status == "connected" && fallback.CredentialRef != nil {
		return db.UpsertSEOIntegrationParams{
			ProjectID:      projectID,
			Provider:       provider,
			Status:         "connected",
			CredentialRef:  fallback.CredentialRef,
			LastVerifiedAt: fallback.LastVerifiedAt,
		}
	}
	return db.UpsertSEOIntegrationParams{
		ProjectID: projectID,
		Provider:  provider,
		Status:    "missing",
	}
}

func (s *Server) seoIDs(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, uuid.UUID, bool) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return uuid.Nil, uuid.Nil, false
	}
	entityID, err := uuid.Parse(chi.URLParam(r, param))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad "+param)
		return uuid.Nil, uuid.Nil, false
	}
	return projectID, entityID, true
}

func parseLimit(r *http.Request, def, max int) (int, error) {
	limit := def
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			if err == nil {
				err = fmt.Errorf("limit must be positive")
			}
			return 0, err
		}
		limit = n
	}
	if limit > max {
		limit = max
	}
	return limit, nil
}

func parseCursor(r *http.Request) (time.Time, error) {
	raw := r.URL.Query().Get("cursor")
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func strPtrFrom(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func contentActionEvidenceSnapshot(raw json.RawMessage, opp db.SeoOpportunity) json.RawMessage {
	if len(raw) > 0 && json.Valid(raw) {
		return raw
	}
	return rawOrDefault(opp.Evidence, `{}`)
}

func contentActionInputSnapshot(raw json.RawMessage, opp db.SeoOpportunity, actionType string) json.RawMessage {
	if len(raw) > 0 && json.Valid(raw) {
		return raw
	}
	payload := map[string]any{
		"opportunity_id":   opp.ID.String(),
		"opportunity_type": opp.Type,
		"action_type":      actionType,
	}
	if opp.Query != nil && strings.TrimSpace(*opp.Query) != "" {
		payload["query"] = strings.TrimSpace(*opp.Query)
	}
	if opp.PageUrl != nil && strings.TrimSpace(*opp.PageUrl) != "" {
		payload["page_url"] = strings.TrimSpace(*opp.PageUrl)
	}
	if strings.TrimSpace(opp.NormalizedPageUrl) != "" {
		payload["normalized_page_url"] = strings.TrimSpace(opp.NormalizedPageUrl)
	}
	if opp.RecommendedAction != nil && strings.TrimSpace(*opp.RecommendedAction) != "" {
		payload["recommended_action"] = strings.TrimSpace(*opp.RecommendedAction)
	}
	if opp.ExpectedImpact != nil && strings.TrimSpace(*opp.ExpectedImpact) != "" {
		payload["expected_impact"] = strings.TrimSpace(*opp.ExpectedImpact)
	}
	return mustJSONLocal(payload)
}

func inferContentActionAssetType(opp db.SeoOpportunity, actionType string, explicit string) string {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed
	}
	text := strings.ToLower(strings.Join([]string{
		opp.Type,
		actionType,
		stringPtrValueAPI(opp.RecommendedAction),
		stringPtrValueAPI(opp.ExpectedImpact),
		stringPtrValueAPI(opp.Query),
	}, " "))
	switch {
	case strings.Contains(text, "schema") || strings.Contains(text, "json-ld") || strings.Contains(text, "structured data"):
		return "schema_patch"
	case strings.Contains(text, "internal link") || strings.Contains(text, "internal-link"):
		return "internal_link_patch"
	case strings.Contains(text, "metadata") || strings.Contains(text, "meta description") || strings.Contains(text, "title") || strings.Contains(text, "ctr"):
		return "metadata_rewrite"
	case strings.Contains(text, "sitemap"):
		return "sitemap_update"
	case strings.Contains(text, "robots") || strings.Contains(text, "canonical") || strings.Contains(text, "index") || strings.Contains(text, "crawl") || strings.Contains(text, "technical"):
		return "technical_fix"
	case strings.Contains(text, "geo") || strings.Contains(text, "citation") || strings.Contains(text, "answer engine"):
		return "glossary_definition"
	case strings.Contains(text, "comparison"):
		return "comparison_page"
	case strings.Contains(text, "alternative"):
		return "alternative_page"
	case strings.Contains(text, "template") || strings.Contains(text, "checklist"):
		return "template_or_checklist"
	default:
		return "blog_post"
	}
}

func defaultReviewRequiredForAssetType(assetType string, riskLevel string) bool {
	if riskLevel == "high" || riskLevel == "medium" {
		return true
	}
	switch assetType {
	case "metadata_rewrite", "internal_link_patch", "sitemap_update":
		return false
	default:
		return true
	}
}

func defaultOutputSnapshotForAction(raw json.RawMessage, assetType string, actionType string, opp db.SeoOpportunity) json.RawMessage {
	if rawHasMeaningfulJSON(raw) {
		return raw
	}
	path := actionGenerationPath(assetType, actionType)
	if path == "topic_article" {
		return rawOrDefault(raw, `{}`)
	}
	return mustJSONLocal(map[string]any{
		"output_type":          path,
		"asset_type":           assetType,
		"title":                actionType,
		"target_url":           opportunityTargetURL(opp),
		"review_state":         "ready_for_review",
		"seo_geo_contribution": seoGeoContributionForAsset(assetType),
		"deliverable":          directActionDeliverable(assetType),
	})
}

func defaultDiffSnapshotForAction(raw json.RawMessage, assetType string, actionType string, opp db.SeoOpportunity) json.RawMessage {
	if rawHasMeaningfulJSON(raw) {
		return raw
	}
	path := actionGenerationPath(assetType, actionType)
	if path == "topic_article" {
		return rawOrDefault(raw, `{}`)
	}
	targetURL := opportunityTargetURL(opp)
	acceptanceTests := directActionAcceptanceTests(assetType, actionType, targetURL)
	aiRepair := directActionAIRepairPayload(assetType, actionType, opp)
	if path == "technical_task" {
		return mustJSONLocal(map[string]any{
			"output_type": "technical_task",
			"target_url":  targetURL,
			"checklist": []map[string]any{
				{
					"task":                 actionType,
					"seo_geo_contribution": seoGeoContributionForAsset(assetType),
					"likely_surfaces":      directActionLikelySurfaces(assetType, targetURL),
					"implementation_steps": directActionImplementationSteps(assetType, actionType, targetURL),
					"verification_steps":   acceptanceTests,
				},
			},
			"ai_repair":           aiRepair,
			"acceptance_tests":    acceptanceTests,
			"requires_apply_step": true,
		})
	}
	return mustJSONLocal(map[string]any{
		"output_type":         "direct_patch",
		"target_url":          targetURL,
		"proposed_changes":    []map[string]any{directActionProposedChange(assetType, actionType, targetURL)},
		"ai_repair":           aiRepair,
		"acceptance_tests":    acceptanceTests,
		"requires_apply_step": true,
	})
}

func directActionProposedChange(assetType string, actionType string, targetURL string) map[string]any {
	return compactActionPayload(map[string]any{
		"asset_type":            assetType,
		"target_url":            targetURL,
		"instruction":           actionType,
		"seo_geo_contribution":  seoGeoContributionForAsset(assetType),
		"likely_surfaces":       directActionLikelySurfaces(assetType, targetURL),
		"implementation_steps":  directActionImplementationSteps(assetType, actionType, targetURL),
		"verification_steps":    directActionAcceptanceTests(assetType, actionType, targetURL),
		"patch_contract":        directActionPatchContract(assetType, targetURL),
		"human_review":          directActionHumanReview(assetType),
		"human_review_required": true,
	})
}

func directActionAIRepairPayload(assetType string, actionType string, opp db.SeoOpportunity) map[string]any {
	targetURL := opportunityTargetURL(opp)
	evidence := directActionRepairEvidence(opp, assetType, actionType)
	payload := map[string]any{
		"issue": compactActionPayload(map[string]any{
			"category":        "site_fix",
			"issue_type":      assetType,
			"affected_urls":   nonEmptyStrings(targetURL),
			"normalized_urls": nonEmptyStrings(opp.NormalizedPageUrl),
			"problem":         actionType,
			"why_it_matters":  firstNonEmptyString(stringPtrValueAPI(opp.ExpectedImpact), seoGeoContributionForAsset(assetType)),
		}),
		"evidence": evidence,
		"fix": compactActionPayload(map[string]any{
			"goal":               actionType,
			"instructions":       directActionImplementationSteps(assetType, actionType, targetURL),
			"likely_surfaces":    directActionLikelySurfaces(assetType, targetURL),
			"seo_contract":       directActionPatchContract(assetType, targetURL),
			"deduplication_rule": directActionDeduplicationRule(assetType),
			"do_not":             directActionDoNot(assetType),
			"risk_level":         opp.RiskLevel,
		}),
		"acceptance_tests": directActionAIRepairAcceptanceTests(assetType, actionType, targetURL, opp),
		"human_review":     directActionHumanReview(assetType),
	}
	if assetType == "metadata_rewrite" {
		payload["observed"] = metadataRewriteObservedSnapshot(opp)
		payload["opportunity"] = metadataRewriteOpportunityContext(opp, actionType)
		payload["proposed_change"] = metadataRewriteProposedChange(opp)
	}
	return payload
}

func directActionAIRepairAcceptanceTests(assetType string, actionType string, targetURL string, opp db.SeoOpportunity) []string {
	if assetType != "metadata_rewrite" {
		return directActionAcceptanceTests(assetType, actionType, targetURL)
	}
	proposed := metadataRewriteProposedChange(opp)
	observed := metadataRewriteObservedSnapshot(opp)
	tests := []string{}
	if title, ok := proposed["title"].(string); ok && strings.TrimSpace(title) != "" {
		tests = append(tests, "Fetch "+targetURL+" and confirm the initial HTML <title> equals "+strconv.Quote(title)+".")
	}
	if description, ok := proposed["meta_description"].(string); ok && strings.TrimSpace(description) != "" {
		tests = append(tests, "Fetch "+targetURL+" and confirm meta[name=\"description\"] equals "+strconv.Quote(description)+".")
	}
	if canonical, ok := observed["canonical"].(string); ok && strings.TrimSpace(canonical) != "" {
		tests = append(tests, "Confirm canonical URL remains "+strconv.Quote(canonical)+".")
	} else {
		tests = append(tests, "Confirm canonical URL remains the production canonical URL for "+targetURL+".")
	}
	tests = append(tests,
		"Confirm the page remains indexable: no noindex robots meta, no blocking X-Robots-Tag, and robots.txt does not disallow the URL.",
		"Check OpenGraph and Twitter card title/description values for duplicate or conflicting metadata signals.",
		"Run the relevant SEO/technical check again and confirm the active finding no longer appears for the target URL.",
	)
	return tests
}

func directActionRepairEvidence(opp db.SeoOpportunity, assetType string, actionType string) map[string]any {
	recommendedAction := stringPtrValueAPI(opp.RecommendedAction)
	if assetType == "metadata_rewrite" && strings.TrimSpace(actionType) != "" {
		recommendedAction = strings.TrimSpace(actionType)
	}
	payload := map[string]any{
		"page_url":            opportunityTargetURL(opp),
		"normalized_page_url": opp.NormalizedPageUrl,
		"opportunity_type":    opp.Type,
		"query":               stringPtrValueAPI(opp.Query),
		"recommended_action":  recommendedAction,
		"expected_impact":     stringPtrValueAPI(opp.ExpectedImpact),
	}
	evidence := compactActionPayload(payload)
	if rawHasMeaningfulJSON(opp.Evidence) {
		var parsed any
		if err := json.Unmarshal(opp.Evidence, &parsed); err == nil {
			if observedMetadata := directActionObservedMetadata(parsed); len(observedMetadata) > 0 {
				evidence["observed_metadata"] = observedMetadata
			}
			sourceEvidence := parsed
			if assetType == "metadata_rewrite" {
				sourceEvidence = sanitizeMetadataRewriteSourceEvidence(parsed)
			}
			evidence["source_evidence"] = sourceEvidence
		}
	}
	return evidence
}

func sanitizeMetadataRewriteSourceEvidence(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, entry := range typed {
			if isMetadataRewriteScopeKey(key) {
				continue
			}
			out[key] = sanitizeMetadataRewriteSourceEvidence(entry)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, entry := range typed {
			out = append(out, sanitizeMetadataRewriteSourceEvidence(entry))
		}
		return out
	default:
		return value
	}
}

func isMetadataRewriteScopeKey(key string) bool {
	switch normalizeMetadataKey(key) {
	case "recommendedaction", "recommendedactions", "suggestedaction", "suggestedactions", "actionrecommendation", "actionrecommendations":
		return true
	default:
		return false
	}
}

func metadataRewriteObservedSnapshot(opp db.SeoOpportunity) map[string]any {
	parsed := parsedActionEvidence(opp.Evidence)
	observedContainers := []string{"observed", "observed_metadata", "observedMetadata", "metadata", "page_metadata", "pageMetadata", "seo_metadata", "seoMetadata", "technical", "raw_details", "rawDetails"}
	return compactActionPayload(map[string]any{
		"status":           firstEvidenceValueInContainers(parsed, observedContainers, []string{"status", "http_status", "httpStatus", "status_code", "statusCode"}),
		"title":            firstEvidenceStringInContainers(parsed, observedContainers, []string{"title", "page_title", "pageTitle", "current_title", "currentTitle", "observed_title", "observedTitle"}),
		"meta_description": firstEvidenceStringInContainers(parsed, observedContainers, []string{"meta_description", "metaDescription", "description", "current_meta_description", "currentMetaDescription", "observed_meta_description", "observedMetaDescription"}),
		"canonical":        firstEvidenceStringInContainers(parsed, observedContainers, []string{"canonical", "canonical_url", "canonicalUrl", "canonical_href", "canonicalHref"}),
		"robots":           firstEvidenceStringInContainers(parsed, observedContainers, []string{"robots", "robots_status", "robotsStatus", "robots_state", "robotsState", "meta_robots", "metaRobots", "indexability"}),
		"observed_at":      firstEvidenceStringInContainers(parsed, observedContainers, []string{"observed_at", "observedAt", "checked_at", "checkedAt", "crawled_at", "crawledAt", "fetched_at", "fetchedAt"}),
	})
}

func metadataRewriteOpportunityContext(opp db.SeoOpportunity, actionType string) map[string]any {
	parsed := parsedActionEvidence(opp.Evidence)
	return compactActionPayload(map[string]any{
		"query":              stringPtrValueAPI(opp.Query),
		"intent":             firstEvidenceString(parsed, []string{"query_intent", "queryIntent", "intent", "intent_type", "intentType"}),
		"problem_detail":     firstEvidenceString(parsed, []string{"problem_detail", "problemDetail", "snippet_issue", "snippetIssue", "current_snippet_issue", "currentSnippetIssue", "issue_detail", "issueDetail"}),
		"confidence":         firstEvidenceValue(parsed, []string{"confidence", "confidence_score", "confidenceScore"}),
		"priority":           firstEvidenceValue(parsed, []string{"priority", "priority_score", "priorityScore"}),
		"recommended_action": strings.TrimSpace(actionType),
	})
}

func metadataRewriteProposedChange(opp db.SeoOpportunity) map[string]any {
	parsed := parsedActionEvidence(opp.Evidence)
	proposedContainers := []string{"proposed_change", "proposedChange", "proposed_metadata", "proposedMetadata", "metadata_rewrite", "metadataRewrite", "recommended_metadata", "recommendedMetadata", "recommendation"}
	title := firstEvidenceStringInContainers(parsed, proposedContainers, []string{"title", "proposed_title", "proposedTitle", "recommended_title", "recommendedTitle", "new_title", "newTitle"})
	if title == "" {
		title = firstEvidenceString(parsed, []string{"proposed_title", "proposedTitle", "recommended_title", "recommendedTitle", "new_title", "newTitle"})
	}
	description := firstEvidenceStringInContainers(parsed, proposedContainers, []string{"meta_description", "metaDescription", "description", "proposed_meta_description", "proposedMetaDescription", "recommended_meta_description", "recommendedMetaDescription", "new_meta_description", "newMetaDescription"})
	if description == "" {
		description = firstEvidenceString(parsed, []string{"proposed_meta_description", "proposedMetaDescription", "recommended_meta_description", "recommendedMetaDescription", "new_meta_description", "newMetaDescription"})
	}
	return compactActionPayload(map[string]any{
		"title":                    title,
		"meta_description":         description,
		"seo_impact":               firstEvidenceStringInContainers(parsed, proposedContainers, []string{"seo_impact", "seoImpact", "seo_contribution", "seoContribution"}),
		"geo_impact":               firstEvidenceStringInContainers(parsed, proposedContainers, []string{"geo_impact", "geoImpact", "geo_contribution", "geoContribution"}),
		"content_support_required": firstEvidenceValueInContainers(parsed, proposedContainers, []string{"content_support_required", "contentSupportRequired", "requires_content_support", "requiresContentSupport"}),
		"preserve":                 []string{"canonical", "indexability", "production URL"},
	})
}

func parsedActionEvidence(raw json.RawMessage) any {
	if !rawHasMeaningfulJSON(raw) {
		return nil
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	return parsed
}

func firstEvidenceString(value any, aliases []string) string {
	found := firstEvidenceValue(value, aliases)
	if text, ok := found.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func firstEvidenceStringInContainers(value any, containers []string, aliases []string) string {
	found := firstEvidenceValueInContainers(value, containers, aliases)
	if text, ok := found.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func firstEvidenceValue(value any, aliases []string) any {
	wanted := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		wanted[normalizeMetadataKey(alias)] = struct{}{}
	}
	return firstEvidenceValueIn(value, wanted)
}

func firstEvidenceValueInContainers(value any, containers []string, aliases []string) any {
	wantedContainers := make(map[string]struct{}, len(containers))
	for _, container := range containers {
		wantedContainers[normalizeMetadataKey(container)] = struct{}{}
	}
	wantedAliases := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		wantedAliases[normalizeMetadataKey(alias)] = struct{}{}
	}
	return firstEvidenceValueInNamedContainer(value, wantedContainers, wantedAliases)
}

func firstEvidenceValueInNamedContainer(value any, wantedContainers map[string]struct{}, wantedAliases map[string]struct{}) any {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			entry := typed[key]
			if _, ok := wantedContainers[normalizeMetadataKey(key)]; ok {
				if found := firstEvidenceValueIn(entry, wantedAliases); found != nil {
					return found
				}
			}
		}
		for _, key := range keys {
			if found := firstEvidenceValueInNamedContainer(typed[key], wantedContainers, wantedAliases); found != nil {
				return found
			}
		}
	case []any:
		for _, entry := range typed {
			if found := firstEvidenceValueInNamedContainer(entry, wantedContainers, wantedAliases); found != nil {
				return found
			}
		}
	}
	return nil
}

func firstEvidenceValueIn(value any, wanted map[string]struct{}) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, entry := range typed {
			if _, ok := wanted[normalizeMetadataKey(key)]; !ok {
				continue
			}
			if found := normalizeEvidenceScalar(entry); found != nil {
				return found
			}
		}

		preferredContainers := []string{"observed", "observed_metadata", "metadata", "page_metadata", "seo_metadata", "technical", "raw_details", "opportunity", "proposed_change"}
		for _, preferred := range preferredContainers {
			normalizedPreferred := normalizeMetadataKey(preferred)
			for key, entry := range typed {
				if normalizeMetadataKey(key) != normalizedPreferred {
					continue
				}
				if found := firstEvidenceValueIn(entry, wanted); found != nil {
					return found
				}
			}
		}

		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if found := firstEvidenceValueIn(typed[key], wanted); found != nil {
				return found
			}
		}
	case []any:
		for _, entry := range typed {
			if found := firstEvidenceValueIn(entry, wanted); found != nil {
				return found
			}
		}
	}
	return nil
}

func normalizeEvidenceScalar(value any) any {
	switch typed := value.(type) {
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			return trimmed
		}
	case float64, bool:
		return typed
	case int:
		return typed
	case int64:
		return typed
	case json.Number:
		return typed.String()
	}
	return nil
}

func directActionObservedMetadata(parsed any) map[string]any {
	fields := []struct {
		name    string
		aliases []string
	}{
		{name: "canonical_url", aliases: []string{"canonical_url", "canonical", "canonicalUrl", "canonical_href", "canonicalHref"}},
		{name: "title", aliases: []string{"title", "page_title", "pageTitle"}},
		{name: "description", aliases: []string{"description", "meta_description", "metaDescription"}},
		{name: "og_title", aliases: []string{"og_title", "ogTitle"}},
		{name: "og_description", aliases: []string{"og_description", "ogDescription"}},
		{name: "og_image", aliases: []string{"og_image", "ogImage"}},
		{name: "brand_name", aliases: []string{"brand_name", "brandName", "site_name", "siteName", "og_site_name", "ogSiteName", "application_name", "applicationName"}},
	}
	out := map[string]any{}
	for _, field := range fields {
		if value := firstObservedMetadataString(parsed, field.aliases); value != "" {
			out[field.name] = value
		}
	}
	return out
}

func firstObservedMetadataString(value any, aliases []string) string {
	wanted := make(map[string]struct{}, len(aliases))
	for _, alias := range aliases {
		wanted[normalizeMetadataKey(alias)] = struct{}{}
	}
	return firstObservedMetadataStringIn(value, wanted)
}

func firstObservedMetadataStringIn(value any, wanted map[string]struct{}) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, entry := range typed {
			if _, ok := wanted[normalizeMetadataKey(key)]; !ok {
				continue
			}
			if text, ok := entry.(string); ok {
				if trimmed := strings.TrimSpace(text); trimmed != "" {
					return trimmed
				}
			}
		}

		preferredContainers := []string{"observed_metadata", "metadata", "page_metadata", "seo_metadata", "open_graph", "opengraph"}
		for _, preferred := range preferredContainers {
			normalizedPreferred := normalizeMetadataKey(preferred)
			for key, entry := range typed {
				if normalizeMetadataKey(key) != normalizedPreferred {
					continue
				}
				if found := firstObservedMetadataStringIn(entry, wanted); found != "" {
					return found
				}
			}
		}

		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if found := firstObservedMetadataStringIn(typed[key], wanted); found != "" {
				return found
			}
		}
	case []any:
		for _, entry := range typed {
			if found := firstObservedMetadataStringIn(entry, wanted); found != "" {
				return found
			}
		}
	}
	return ""
}

func normalizeMetadataKey(value string) string {
	replacer := strings.NewReplacer("_", "", "-", "", " ", "", ".", "")
	return replacer.Replace(strings.ToLower(strings.TrimSpace(value)))
}

func directActionLikelySurfaces(assetType string, targetURL string) []string {
	switch assetType {
	case "schema_patch":
		return []string{
			"Page route or template that renders " + targetURL,
			"Shared SEO metadata or structured-data component used by that route",
			"Server-rendered head/layout file where JSON-LD can be emitted in initial HTML",
		}
	case "internal_link_patch":
		return []string{
			"Target page content for " + targetURL,
			"Relevant source pages in the same topic cluster",
			"Navigation, related-content, or body-copy components that own internal links",
		}
	case "sitemap_update":
		return []string{
			"Production sitemap generator or sitemap.xml route",
			"Robots.txt sitemap declaration",
			"Canonical URL config for " + targetURL,
		}
	default:
		return []string{
			"Page route, metadata config, or crawler-facing component for " + targetURL,
			"Robots, canonical, redirect, sitemap, or server response configuration that controls discoverability",
		}
	}
}

func directActionImplementationSteps(assetType string, actionType string, targetURL string) []string {
	switch assetType {
	case "metadata_rewrite":
		return []string{
			"Locate the page route, layout metadata, or SEO config that emits the production <title> and meta description for " + targetURL + ".",
			"Replace the existing title and meta description with the exact proposed_change values in this JSON.",
			"Preserve the canonical URL, indexability, and production host while checking OpenGraph and Twitter card metadata for intentional consistency.",
		}
	case "schema_patch":
		return []string{
			"Locate the route/template that renders the target URL and confirm whether JSON-LD already exists.",
			"Add or update server-rendered JSON-LD in a script[type=\"application/ld+json\"] block using real production page metadata.",
			"Preserve the canonical target URL, omit placeholder fields, and keep all URL fields absolute production URLs.",
		}
	case "internal_link_patch":
		return []string{
			"Identify source pages with topical relevance and enough body context to link naturally to the target URL.",
			"Add descriptive anchor text that matches the destination intent without keyword stuffing.",
			"Confirm the new links are crawlable HTML links and do not point through redirects or non-canonical URL variants.",
		}
	case "sitemap_update":
		return []string{
			"Locate the production sitemap generator and confirm the target URL inclusion or exclusion rule.",
			"Update sitemap and robots declarations so the canonical target URL is discoverable by crawlers.",
			"Keep generated sitemap URLs canonical, absolute, indexable, and free of staging or preview hosts.",
		}
	default:
		return []string{
			"Locate the code or configuration that controls the crawler-facing behavior for the target URL.",
			"Apply the requested site fix: " + actionType + ".",
			"Preserve canonical URLs, indexability, and production-only hosts while making the smallest safe change.",
		}
	}
}

func directActionAcceptanceTests(assetType string, actionType string, targetURL string) []string {
	switch assetType {
	case "schema_patch":
		return []string{
			"Inspect the initial HTML for " + targetURL + " and verify it includes server-rendered JSON-LD in a script[type=\"application/ld+json\"] element.",
			"Parse every JSON-LD block as valid JSON and verify it has @context set to https://schema.org, a relevant @type, and no placeholders.",
			"Validate the JSON-LD with Schema Markup Validator for " + targetURL + " and resolve every parser error.",
			"Use Google Rich Results Test only to confirm the page is readable and parser-error free; WebSite, Organization, and WebPage schema does not require rich result eligibility.",
		}
	case "internal_link_patch":
		return []string{
			"Fetch the updated source pages and confirm they contain crawlable HTML links to " + targetURL + ".",
			"Verify anchor text is descriptive, unique enough to explain the destination, and does not duplicate existing boilerplate links.",
			"Confirm linked URLs resolve to canonical production URLs without redirect chains.",
		}
	case "sitemap_update":
		return []string{
			"Fetch the production sitemap and confirm it contains the canonical target URL when the page should be indexed.",
			"Fetch robots.txt and confirm it advertises the correct sitemap and does not block the target URL.",
			"Confirm the sitemap URL returns 200, valid XML, production hosts only, and no non-canonical variants.",
		}
	default:
		return []string{
			"Fetch " + targetURL + " and confirm the crawler-facing behavior now matches the requested site fix: " + actionType + ".",
			"Run the relevant SEO/technical check again and confirm the active finding no longer appears for the target URL.",
			"Confirm production pages still return the expected status, canonical URL, and indexability signals.",
		}
	}
}

func directActionPatchContract(assetType string, targetURL string) map[string]any {
	switch assetType {
	case "schema_patch":
		pageRole := "web_page"
		schemaTypes := []string{"WebPage"}
		if isHomepageURL(targetURL) {
			pageRole = "homepage"
			schemaTypes = []string{"WebSite", "Organization", "WebPage"}
		}
		return map[string]any{
			"change_type":        "json_ld_schema_patch",
			"target_url":         targetURL,
			"page_role":          pageRole,
			"schema_types":       schemaTypes,
			"render_requirement": "JSON-LD must be present in the initial server-rendered HTML.",
			"deduplication_rule": directActionDeduplicationRule(assetType),
			"graph_guidance":     directActionSchemaGraphGuidance(targetURL),
			"do_not":             directActionDoNot(assetType),
			"constraints": []string{
				"Use real production brand, page, and canonical metadata.",
				"Use absolute production URLs only.",
				"Omit fields that cannot be verified instead of shipping blank or placeholder values.",
				directActionDeduplicationRule(assetType),
			},
		}
	case "internal_link_patch":
		return map[string]any{
			"change_type": "internal_link_patch",
			"target_url":  targetURL,
			"constraints": []string{
				"Links must be crawlable HTML anchors.",
				"Anchor copy must describe the destination intent.",
				"Use canonical production URLs and avoid redirect chains.",
			},
		}
	case "metadata_rewrite":
		return map[string]any{
			"change_type": "metadata_rewrite",
			"target_url":  targetURL,
			"constraints": []string{
				"Update the existing title and meta description signal instead of creating new page content.",
				"Use only reviewed production copy values from proposed_change.",
				"Do not use staging, preview, localhost, or placeholder URLs.",
				"Verify the exact crawler-facing values in production after deployment.",
			},
			"preserve": []string{"canonical", "indexability", "production URL"},
		}
	default:
		return map[string]any{
			"change_type": assetType,
			"target_url":  targetURL,
			"constraints": []string{
				"Make the smallest production-safe change that resolves the crawler-facing issue.",
				"Do not use staging, preview, localhost, or placeholder URLs.",
				"Verify the signal in production after deployment.",
			},
		}
	}
}

func directActionDeduplicationRule(assetType string) string {
	switch assetType {
	case "metadata_rewrite":
		return "Update the existing title/meta description source of truth; check OpenGraph and Twitter card metadata for duplicates or conflicting values instead of adding parallel SEO signals."
	case "schema_patch":
		return "If JSON-LD already exists, update or extend the existing graph instead of adding duplicate Organization, WebSite, or WebPage nodes."
	case "internal_link_patch":
		return "If a crawlable canonical link to the target already exists on a source page, update anchor/context only when it improves clarity instead of adding duplicate boilerplate links."
	case "sitemap_update":
		return "Update the canonical sitemap entry or generation rule instead of adding duplicate URL variants."
	default:
		return "Update the existing crawler-facing signal when present instead of adding duplicate or conflicting signals."
	}
}

func directActionDoNot(assetType string) []string {
	switch assetType {
	case "schema_patch":
		return []string{
			"Do not add unverified sameAs links.",
			"Do not add placeholder logo, address, founder, phone, or social profile fields.",
			"Do not inject JSON-LD only on the client after hydration.",
			"Do not change visible page content unless required.",
		}
	case "internal_link_patch":
		return []string{
			"Do not add links with generic anchor text such as click here.",
			"Do not point links at staging, preview, redirecting, or non-canonical URLs.",
			"Do not add duplicate navigation or footer links when contextual body links are the intended fix.",
		}
	default:
		return []string{
			"Do not add placeholder values or staging URLs.",
			"Do not change unrelated visible page content unless required.",
			"Do not create duplicate or conflicting SEO signals.",
		}
	}
}

func directActionHumanReview(assetType string) map[string]any {
	switch assetType {
	case "schema_patch":
		return map[string]any{
			"required": true,
			"reason":   "Structured data affects public search and entity interpretation and should use verified brand metadata only.",
			"review_focus": []string{
				"brand name",
				"description",
				"canonical URL",
				"organization identity",
			},
		}
	default:
		return map[string]any{
			"required": true,
			"reason":   "This fix changes crawler-facing production signals and should be reviewed before applying.",
			"review_focus": []string{
				"target URL",
				"canonical URL",
				"production-only values",
			},
		}
	}
}

func directActionSchemaGraphGuidance(targetURL string) map[string]any {
	pageID := schemaFragmentID(targetURL, "webpage")
	guidance := map[string]any{
		"recommended_shape": "Use one JSON-LD object with @context set to https://schema.org and an @graph array.",
		"stable_ids": map[string]any{
			"WebPage": pageID,
		},
		"relationships": []string{
			"Use stable @id values so entities can reference each other without duplicating nodes.",
		},
		"example": map[string]any{
			"@context": "https://schema.org",
			"@graph": []map[string]any{
				{"@type": "WebPage", "@id": pageID},
			},
		},
	}
	if isHomepageURL(targetURL) {
		orgID := schemaFragmentID(targetURL, "organization")
		websiteID := schemaFragmentID(targetURL, "website")
		guidance["stable_ids"] = map[string]any{
			"Organization": orgID,
			"WebSite":      websiteID,
			"WebPage":      pageID,
		}
		guidance["relationships"] = []string{
			"WebSite.publisher should reference the Organization @id.",
			"WebPage.isPartOf should reference the WebSite @id.",
			"WebPage.about or WebPage.publisher should reference the Organization @id when verified.",
		}
		guidance["example"] = map[string]any{
			"@context": "https://schema.org",
			"@graph": []map[string]any{
				{"@type": "Organization", "@id": orgID},
				{"@type": "WebSite", "@id": websiteID},
				{"@type": "WebPage", "@id": pageID},
			},
		}
	}
	return guidance
}

func schemaFragmentID(targetURL string, fragment string) string {
	trimmed := strings.TrimSpace(targetURL)
	if trimmed == "" {
		return "#" + fragment
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.RawQuery = ""
		parsed.Fragment = fragment
		if parsed.Path == "" {
			parsed.Path = "/"
		}
		return parsed.String()
	}
	return strings.TrimRight(trimmed, "/") + "/#" + fragment
}

func compactActionPayload(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, entry := range value {
		switch typed := entry.(type) {
		case nil:
			continue
		case string:
			if strings.TrimSpace(typed) == "" {
				continue
			}
			out[key] = typed
		case []string:
			if len(typed) == 0 {
				continue
			}
			out[key] = typed
		case []map[string]any:
			if len(typed) == 0 {
				continue
			}
			out[key] = typed
		case map[string]any:
			if len(typed) == 0 {
				continue
			}
			out[key] = typed
		default:
			out[key] = entry
		}
	}
	return out
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func isHomepageURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	path := strings.TrimSpace(parsed.Path)
	return path == "" || path == "/"
}

func actionGenerationPath(assetType string, actionType string) string {
	text := strings.ToLower(strings.TrimSpace(assetType + " " + actionType))
	switch {
	case strings.Contains(text, "metadata_rewrite") || strings.Contains(text, "internal_link_patch") || strings.Contains(text, "schema_patch"):
		return "direct_patch"
	case strings.Contains(text, "sitemap_update") || strings.Contains(text, "technical_fix") || strings.Contains(text, "technical seo") || strings.Contains(text, "robots") || strings.Contains(text, "canonical"):
		return "technical_task"
	default:
		return "topic_article"
	}
}

func rawHasMeaningfulJSON(raw json.RawMessage) bool {
	if len(raw) == 0 || !json.Valid(raw) {
		return false
	}
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "{}" && trimmed != "[]"
}

func opportunityTargetURL(opp db.SeoOpportunity) string {
	if opp.PageUrl != nil && strings.TrimSpace(*opp.PageUrl) != "" {
		return strings.TrimSpace(*opp.PageUrl)
	}
	return strings.TrimSpace(opp.NormalizedPageUrl)
}

func seoGeoContributionForAsset(assetType string) string {
	switch assetType {
	case "metadata_rewrite":
		return "Improve search appearance and click-through for an existing page."
	case "internal_link_patch":
		return "Strengthen crawl paths and topical authority between existing pages."
	case "schema_patch":
		return "Make page entities and answers easier for search and answer engines to parse."
	case "sitemap_update", "technical_fix":
		return "Remove indexing or crawl blockers so visibility signals can be collected reliably."
	default:
		return "Create or refresh an owned asset that can earn search demand and AI citations."
	}
}

func directActionDeliverable(assetType string) string {
	switch assetType {
	case "metadata_rewrite":
		return "Title and meta description patch for an existing page."
	case "internal_link_patch":
		return "Internal link additions or edits for existing pages."
	case "schema_patch":
		return "Structured data patch for human review before applying."
	case "sitemap_update", "technical_fix":
		return "Technical task with verification steps before marking applied."
	default:
		return "Reviewable SEO/GEO action output."
	}
}

func stringPtrValueAPI(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func contentActionIsPageUpdate(action db.ContentAction) bool {
	if action.WorkType != nil && strings.TrimSpace(*action.WorkType) == WorkTypeImprovePage {
		return true
	}
	return strings.TrimSpace(contentActionAssetType(action)) == "page_update"
}

func contentActionCreatesContent(action db.ContentAction) bool {
	if contentActionIsPageUpdate(action) {
		return false
	}
	if action.WorkType != nil && strings.TrimSpace(*action.WorkType) == WorkTypeFixSiteIssue {
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

func contentActionIsSiteFix(action db.ContentAction) bool {
	if action.WorkType != nil && strings.TrimSpace(*action.WorkType) == WorkTypeFixSiteIssue {
		return true
	}
	switch contentActionAssetType(action) {
	case "metadata_rewrite", "internal_link_patch", "schema_patch", "sitemap_update", "technical_fix":
		return true
	default:
		return false
	}
}

func siteFixAssetTypeForAction(action db.ContentAction) string {
	return strings.TrimSpace(contentActionAssetType(action))
}

func contentActionTargetURL(action db.ContentAction) string {
	return firstNonEmpty(
		stringPtrValueAPI(action.TargetUrl),
		stringPtrValueAPI(action.NormalizedTargetUrl),
	)
}

func contentActionNormalizedTargetURL(action db.ContentAction) string {
	return firstNonEmpty(
		stringPtrValueAPI(action.NormalizedTargetUrl),
		stringPtrValueAPI(action.TargetUrl),
	)
}

func siteFixOpportunityKey(action db.ContentAction) string {
	return "site_fix:" + action.ID.String()
}

func siteFixWorkingBranch(action db.ContentAction) string {
	suffix := strings.ReplaceAll(action.ID.String(), "-", "")
	if len(suffix) > 12 {
		suffix = suffix[:12]
	}
	return "citeloop/site-fix-" + suffix
}

func siteFixPRTitle(action db.ContentAction) string {
	title := compactPageUpdateText(action.ActionType)
	if title == "" {
		title = "Apply CiteLoop site fix"
	}
	if len(title) > 88 {
		title = strings.TrimSpace(title[:88])
	}
	return "CiteLoop: " + title
}

func siteFixPRCommitMessage(action db.ContentAction) string {
	title := compactPageUpdateText(action.ActionType)
	if title == "" {
		return "Apply CiteLoop site fix"
	}
	return title
}

func siteFixResolutionCriteria(action db.ContentAction) json.RawMessage {
	return mustJSONLocal(map[string]any{
		"source":                    "site_fix",
		"content_action_id":         action.ID,
		"opportunity_id":            action.OpportunityID,
		"asset_type":                siteFixAssetTypeForAction(action),
		"target_url":                contentActionTargetURL(action),
		"target_url_locked":         true,
		"creates_new_article":       false,
		"requires_post_merge_check": true,
		"acceptance_tests":          siteFixAcceptanceTests(action),
	})
}

func siteFixAcceptanceTests(action db.ContentAction) []string {
	tests := []string{}
	var parsed any
	if rawHasMeaningfulJSON(action.DiffSnapshot) {
		_ = json.Unmarshal(action.DiffSnapshot, &parsed)
		tests = appendStringValues(tests, nestedStringSlice(parsed, "acceptance_tests")...)
	}
	if len(tests) > 0 {
		return tests
	}
	targetURL := contentActionTargetURL(action)
	switch siteFixAssetTypeForAction(action) {
	case "metadata_rewrite":
		return []string{
			"Fetch " + targetURL + " and confirm the page emits the proposed title and meta description.",
			"Confirm canonical URL and indexability remain unchanged.",
			"Run the original SEO/GEO check again and confirm the finding no longer appears for the target URL.",
		}
	default:
		return []string{
			"Apply the site fix to the existing target page.",
			"Run the original SEO/GEO check again and confirm the finding no longer appears for the target URL.",
		}
	}
}

func siteFixPRBody(action db.ContentAction, mapping pageUpdateSourceMapping) string {
	criteria := compactJSONSummary(siteFixResolutionCriteria(action))
	if criteria == "" {
		criteria = "No structured criteria were recorded."
	}
	return fmt.Sprintf(`## CiteLoop Site Fix

Target URL: %s
Source file: %s
Source mapping: %s

## Requested Change

%s

## Resolution Criteria

%s

After this PR merges and deploys, CiteLoop should re-run the original finding before marking the site fix applied.
`,
		contentActionTargetURL(action),
		mapping.SourceFilePath,
		mapping.Reason,
		compactPageUpdateText(action.ActionType),
		criteria,
	)
}

func siteFixMetadataRewriteContent(source string, action db.ContentAction) (string, error) {
	title, description := siteFixProposedMetadata(action)
	if title == "" && description == "" {
		return "", fmt.Errorf(siteFixMissingProposedMetadataMessage)
	}
	frontmatter, body, newline, ok := splitMDFrontmatter(source)
	if ok {
		updates := map[string]string{}
		if title != "" {
			updates["title"] = title
			updates["seo_title"] = title
		}
		if description != "" {
			updates["description"] = description
			updates["excerpt"] = description
		}
		nextFrontmatter := rewriteYAMLStringFields(frontmatter, updates, newline)
		return "---" + newline + nextFrontmatter + "---" + body, nil
	}
	updated, matched, err := rewriteNextMetadataSource(source, title, description)
	if err != nil {
		return "", err
	}
	if matched {
		return updated, nil
	}
	return "", fmt.Errorf("metadata rewrite site fix requires Markdown frontmatter or supported Next.js metadata source")
}

const siteFixMissingProposedMetadataMessage = "metadata rewrite site fix has no proposed title or meta description"

func isSiteFixMissingProposedMetadataError(err error) bool {
	return err != nil && strings.Contains(err.Error(), siteFixMissingProposedMetadataMessage)
}

func rewriteNextMetadataSource(source, title, description string) (string, bool, error) {
	updated := source
	matchedTitle := false
	matchedDescription := false
	if title != "" {
		var matched bool
		updated, matched = rewriteTypeScriptConstStringAssignment(updated, `(?:[A-Za-z0-9]+_)*TITLE(?:_[A-Za-z0-9]+)*`, title)
		matchedTitle = matchedTitle || matched
		if !matchedTitle {
			updated, matched = rewriteTypeScriptObjectStringField(updated, "title", title)
			matchedTitle = matchedTitle || matched
		}
	}
	if description != "" {
		var matched bool
		updated, matched = rewriteTypeScriptConstStringAssignment(updated, `(?:[A-Za-z0-9]+_)*(?:META_)?DESCRIPTION(?:_[A-Za-z0-9]+)*`, description)
		matchedDescription = matchedDescription || matched
		if !matchedDescription {
			updated, matched = rewriteTypeScriptObjectStringField(updated, "description", description)
			matchedDescription = matchedDescription || matched
		}
	}
	missing := []string{}
	if title != "" && !matchedTitle {
		missing = append(missing, "title")
	}
	if description != "" && !matchedDescription {
		missing = append(missing, "description")
	}
	if len(missing) > 0 {
		return "", false, fmt.Errorf("supported Next.js metadata source is missing rewritable %s", strings.Join(missing, " and "))
	}
	return updated, matchedTitle || matchedDescription, nil
}

func rewriteTypeScriptConstStringAssignment(source, identifierPattern, value string) (string, bool) {
	re := regexp.MustCompile("(?ms)(const\\s+" + identifierPattern + "\\s*=\\s*)([\"'`])([^\"'`]*)([\"'`])(\\s*;)")
	matched := false
	updated := re.ReplaceAllStringFunc(source, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) != 6 {
			return match
		}
		matched = true
		return parts[1] + quoteTypeScriptString(value) + parts[5]
	})
	return updated, matched
}

func rewriteTypeScriptObjectStringField(source, field, value string) (string, bool) {
	re := regexp.MustCompile("(?m)(\\b" + regexp.QuoteMeta(field) + "\\s*:\\s*)([\"'`])([^\"'`]*)([\"'`])")
	matched := false
	updated := re.ReplaceAllStringFunc(source, func(match string) string {
		parts := re.FindStringSubmatch(match)
		if len(parts) != 5 {
			return match
		}
		matched = true
		return parts[1] + quoteTypeScriptString(value)
	})
	return updated, matched
}

func quoteTypeScriptString(value string) string {
	return strconv.Quote(value)
}

func splitMDFrontmatter(source string) (string, string, string, bool) {
	newline := "\n"
	if strings.HasPrefix(source, "---\r\n") {
		newline = "\r\n"
	} else if !strings.HasPrefix(source, "---\n") {
		return "", "", "", false
	}
	start := len("---" + newline)
	closing := strings.Index(source[start:], newline+"---")
	if closing < 0 {
		return "", "", "", false
	}
	frontmatterEnd := start + closing
	bodyStart := frontmatterEnd + len(newline+"---")
	return source[start:frontmatterEnd], source[bodyStart:], newline, true
}

func rewriteYAMLStringFields(frontmatter string, updates map[string]string, newline string) string {
	lines := strings.Split(frontmatter, newline)
	seen := map[string]bool{}
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		for key, value := range updates {
			if strings.HasPrefix(trimmed, key+":") {
				prefix := line[:strings.Index(line, key+":")]
				lines[i] = prefix + key + ": " + strconv.Quote(value)
				seen[key] = true
			}
		}
	}
	for _, key := range []string{"title", "seo_title", "description", "excerpt"} {
		if value, ok := updates[key]; ok && !seen[key] {
			lines = append(lines, key+": "+strconv.Quote(value))
		}
	}
	if len(lines) == 1 && lines[0] == "" {
		lines = []string{}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, newline) + newline
}

func siteFixProposedMetadata(action db.ContentAction) (string, string) {
	for _, raw := range []json.RawMessage{action.DiffSnapshot, action.OutputSnapshot, action.EvidenceSnapshot, action.InputSnapshot} {
		title, description := proposedMetadataFromRaw(raw)
		if title != "" || description != "" {
			return title, description
		}
	}
	return "", ""
}

func proposedMetadataFromRaw(raw json.RawMessage) (string, string) {
	if !rawHasMeaningfulJSON(raw) {
		return "", ""
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", ""
	}
	for _, path := range [][]string{
		{"publisher_result", "ai_proposal", "proposed_change"},
		{"ai_site_fix", "proposed_change"},
		{"ai_proposal", "proposed_change"},
		{"ai_repair", "proposed_change"},
		{"proposed_change"},
		{"proposed_metadata"},
		{"metadata_rewrite"},
		{"recommended_metadata"},
	} {
		if title, description := metadataValuesAtPath(parsed, path...); title != "" || description != "" {
			return title, description
		}
	}
	if changes := nestedAny(parsed, "proposed_changes"); changes != nil {
		if list, ok := changes.([]any); ok {
			for _, item := range list {
				if title, description := metadataValuesAtPath(item, "proposed_change"); title != "" || description != "" {
					return title, description
				}
			}
		}
	}
	return "", ""
}

func metadataValuesAtPath(parsed any, path ...string) (string, string) {
	node := nestedAny(parsed, path...)
	if node == nil {
		return "", ""
	}
	title := firstNestedString(node, "title", "proposed_title", "recommended_title", "new_title")
	description := firstNestedString(node, "meta_description", "metaDescription", "description", "proposed_meta_description", "recommended_meta_description", "new_meta_description")
	return title, description
}

func nestedAny(value any, path ...string) any {
	current := value
	for _, key := range path {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = obj[key]
	}
	return current
}

func firstNestedString(value any, keys ...string) string {
	obj, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range keys {
		if text, ok := obj[key].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func nestedStringSlice(value any, path ...string) []string {
	node := nestedAny(value, path...)
	items, ok := node.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, strings.TrimSpace(text))
		}
	}
	return out
}

func appendStringValues(values []string, next ...string) []string {
	for _, value := range next {
		if strings.TrimSpace(value) != "" {
			values = append(values, strings.TrimSpace(value))
		}
	}
	return values
}

func pageUpdateResolutionCriteria(action db.ContentAction, opp db.SeoOpportunity, targetURL string) json.RawMessage {
	checks := []string{
		"Target URL remains unchanged.",
		"Updated page includes source-backed support for the accepted opportunity.",
		"Post-update verification rechecks the original opportunity type.",
	}
	if opp.Type == "thin_evidence_page" {
		checks = []string{
			"Fresh crawl extracts at least 3 source-backed evidence snippets.",
			"Evidence block includes claim, source, supporting detail, and caveat.",
			"Target URL remains unchanged.",
		}
	}
	return mustJSONLocal(map[string]any{
		"target_url":          targetURL,
		"opportunity_type":    opp.Type,
		"opportunity_key":     opp.OpportunityKey,
		"action_type":         action.ActionType,
		"expected_impact":     stringPtrValueAPI(opp.ExpectedImpact),
		"checks":              checks,
		"target_url_locked":   true,
		"creates_new_article": false,
	})
}

func pageUpdateOriginalSourceSnapshot(action db.ContentAction, opp db.SeoOpportunity, targetURL string, normalizedTargetURL string) json.RawMessage {
	return mustJSONLocal(map[string]any{
		"source":                     "content_action",
		"content_action_id":          action.ID,
		"opportunity_id":             opp.ID,
		"opportunity_key":            opp.OpportunityKey,
		"target_url":                 targetURL,
		"normalized_target_url":      normalizedTargetURL,
		"target_content_hash_before": stringPtrValueAPI(action.TargetContentHashBefore),
		"evidence":                   rawOrDefault(opp.Evidence, `{}`),
	})
}

func defaultPageUpdateProposedContent(action db.ContentAction, draft db.PageUpdateDraft) string {
	title := firstNonEmpty(action.ActionType, "Update existing page")
	return fmt.Sprintf("## Page update: %s\n\nTarget URL: %s\n\nAdd or revise the existing page section so it directly resolves the accepted opportunity. Keep the slug, canonical URL, and page intent unchanged.\n\n### Evidence update\n\n- Add source-backed claims that can be extracted from the page.\n- Pair each claim with a source, supporting detail, and caveat when relevant.\n- Keep this as an edit to the existing page, not a new article.\n", title, draft.TargetUrl)
}

func defaultPageUpdatePatch(action db.ContentAction, draft db.PageUpdateDraft, proposed string) json.RawMessage {
	return mustJSONLocal(map[string]any{
		"type":       "manual_patch",
		"target_url": draft.TargetUrl,
		"operations": []map[string]any{
			{
				"op":      "insert_or_update_section",
				"section": "evidence",
				"summary": firstNonEmpty(action.ActionType, "Update existing page evidence"),
				"content": proposed,
			},
		},
		"guards": []string{
			"Do not change the target URL or slug.",
			"Do not create a new canonical article.",
			"Recheck the original opportunity after applying.",
		},
	})
}

func defaultPageUpdateDiff(action db.ContentAction, draft db.PageUpdateDraft, proposed string) json.RawMessage {
	return mustJSONLocal(map[string]any{
		"output_type":           "page_update_diff",
		"target_url":            draft.TargetUrl,
		"normalized_target_url": draft.NormalizedTargetUrl,
		"change_summary":        firstNonEmpty(action.ActionType, "Update existing page"),
		"sections_added":        []string{"Evidence update"},
		"sections_edited":       []string{},
		"metadata_changes":      []string{},
		"target_url_locked":     true,
		"creates_new_article":   false,
		"before":                "Existing target page content.",
		"after":                 proposed,
	})
}

func defaultPageUpdateQA(action db.ContentAction, draft db.PageUpdateDraft) json.RawMessage {
	return mustJSONLocal(map[string]any{
		"status":                                 "passed",
		"claim_safety":                           "requires_human_review",
		"page_update_scope":                      "passed",
		"target_url_locked":                      true,
		"resolution_criteria_likely_satisfied":   true,
		"no_new_article_or_distribution_created": true,
		"manual_review_required_before_applying": true,
		"target_url":                             draft.TargetUrl,
		"content_action_id":                      action.ID,
	})
}

func topicFromContentAction(projectID uuid.UUID, action db.ContentAction, opp db.SeoOpportunity, requestedPublishStrategy string) db.CreateTopicParams {
	title := firstNonEmpty(
		action.ActionType,
		stringPtrValueAPI(opp.RecommendedAction),
		"Improve search visibility",
	)
	if opp.Query != nil && strings.TrimSpace(*opp.Query) != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(strings.TrimSpace(*opp.Query))) {
		title = title + ": " + strings.TrimSpace(*opp.Query)
	}
	targetPrompt := firstNonEmpty(stringPtrValueAPI(opp.Query), stringPtrValueAPI(opp.ExpectedImpact), action.ActionType)
	angle := firstNonEmpty(stringPtrValueAPI(opp.ExpectedImpact), action.ActionType)
	internalLinks := []map[string]string{}
	if action.TargetUrl != nil && strings.TrimSpace(*action.TargetUrl) != "" {
		internalLinks = append(internalLinks, map[string]string{"url": strings.TrimSpace(*action.TargetUrl)})
	}
	return db.CreateTopicParams{
		ProjectID:             projectID,
		Channel:               publishStrategyForContentAction(action, opp, requestedPublishStrategy),
		Title:                 title,
		TargetKeyword:         opp.Query,
		TargetPrompt:          strPtr(targetPrompt),
		Angle:                 strPtr(angle),
		Format:                strPtr("article"),
		Priority:              priorityFromOpportunityScore(pgutil.Float(opp.PriorityScore)),
		InternalLinks:         mustJSONLocal(internalLinks),
		Status:                string(topicstate.StatusBacklog),
		SourceContentActionID: pgtype.UUID{Bytes: action.ID, Valid: true},
	}
}

func publishStrategyForContentAction(action db.ContentAction, opp db.SeoOpportunity, requested string) string {
	if strategy := normalizePublishStrategy(requested); strategy != "" {
		return strategy
	}
	for _, raw := range []json.RawMessage{action.InputSnapshot, action.OutputSnapshot, action.EvidenceSnapshot} {
		if strategy := snapshotPublishStrategy(raw); strategy != "" {
			return strategy
		}
	}
	text := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		contentActionAssetType(action),
		action.ActionType,
		stringPtrValueAPI(action.WorkType),
		opp.Type,
		stringPtrValueAPI(opp.RecommendedAction),
		stringPtrValueAPI(opp.ExpectedImpact),
		stringPtrValueAPI(opp.Query),
	}, " ")))
	switch {
	case strings.Contains(text, "community") || strings.Contains(text, "reddit") || strings.Contains(text, "dev.to") || strings.Contains(text, "hashnode") || strings.Contains(text, "distribution") || strings.Contains(text, "syndication"):
		if strings.Contains(text, "page") || strings.Contains(text, "guide") || strings.Contains(text, "comparison") || strings.Contains(text, "article") {
			return "both"
		}
		return "syndication"
	case action.WorkType != nil && strings.TrimSpace(*action.WorkType) == WorkTypeImprovePage:
		return "blog"
	case strings.Contains(text, "page_update") || strings.Contains(text, "metadata_rewrite") || strings.Contains(text, "refresh"):
		return "blog"
	case action.WorkType != nil && strings.TrimSpace(*action.WorkType) == WorkTypeCreateContent:
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

func measurementWindowForAction(assetType, actionType string) json.RawMessage {
	days, primary, secondary := measurementPlanFor(assetType, actionType)
	return mustJSONLocal(map[string]any{
		"baseline":          map[string]any{"days": 28},
		"checkpoints":       checkpointObjects(days),
		"primary_metric":    primary,
		"secondary_metrics": secondary,
	})
}

func measurementPlanFor(assetType, actionType string) ([]int, string, []string) {
	text := strings.ToLower(strings.TrimSpace(assetType + " " + actionType))
	switch {
	case strings.Contains(text, "metadata_rewrite") || strings.Contains(text, "metadata") || strings.Contains(text, "title") || strings.Contains(text, "meta"):
		return []int{7, 14, 28}, "ctr", []string{"impressions", "clicks", "position"}
	case strings.Contains(text, "internal_link_patch") || strings.Contains(text, "internal link"):
		return []int{14, 28, 56}, "clicks", []string{"impressions", "position"}
	case strings.Contains(text, "external_distribution") || strings.Contains(text, "distribution") || strings.Contains(text, "syndication"):
		return []int{7, 14, 28}, "referral_sessions", []string{"brand_mentions", "backlinks"}
	case strings.Contains(text, "geo") || strings.Contains(text, "citation"):
		// GEO citation-ready asset checks run weekly for the first eight weeks.
		return []int{7, 14, 21, 28, 35, 42, 49, 56}, "project_owned_citations", []string{"brand_mentions", "competitor_citations"}
	case strings.Contains(text, "sitemap") || strings.Contains(text, "technical"):
		return []int{1, 7, 14, 28}, "indexed_status", []string{"http_status", "technical_issue_count"}
	default:
		return []int{14, 28, 56, 90}, "clicks", []string{"impressions", "ctr", "position"}
	}
}

func checkpointObjects(days []int) []map[string]any {
	out := make([]map[string]any, 0, len(days))
	for _, day := range days {
		out = append(out, map[string]any{"day": day, "status": "scheduled"})
	}
	return out
}

func emptyVisibilityLifecycleCounts() map[string]int {
	counts := make(map[string]int, len(visibilityLifecycleStages))
	for _, stage := range visibilityLifecycleStages {
		counts[string(stage)] = 0
	}
	return counts
}

func deriveVisibilityLifecycleStage(row db.ListVisibilityActionRowsRow) VisibilityLifecycleStage {
	status := strings.ToLower(strings.TrimSpace(row.Status))
	draftStatus := ""
	if row.DraftArticleStatus != nil {
		draftStatus = strings.ToLower(strings.TrimSpace(*row.DraftArticleStatus))
	}

	if status == "failed" || status == "verification_failed" || status == "recovery_required" {
		return VisibilityStageBlocked
	}
	if status == "completed" {
		return VisibilityStageLearned
	}
	if status == "measuring" {
		return VisibilityStageMeasuring
	}
	if status == "published" || row.PublishedAt.Valid || row.VerifiedAt.Valid || draftStatus == "published" {
		return VisibilityStagePublishedOrApplied
	}
	if status == "drafting" {
		return VisibilityStageDrafting
	}
	if draftStatus == "pending_review" && (row.DraftArticleID.Valid || row.DraftArticleJoinedID.Valid) {
		return VisibilityStageReadyForReview
	}
	if status == "approved" {
		if row.TopicID.Valid {
			return VisibilityStagePlanned
		}
		return VisibilityStageApproved
	}
	if status == "ready_for_review" {
		if rawJSONHasData(row.DiffSnapshot) || rawJSONHasData(row.OutputSnapshot) {
			return VisibilityStageReadyForReview
		}
		if row.TopicID.Valid {
			return VisibilityStagePlanned
		}
		return VisibilityStageAddedToPlan
	}
	return VisibilityStageAddedToPlan
}

func visibilityActionInLoop(row db.ListVisibilityActionRowsRow, stage VisibilityLifecycleStage) VisibilityActionInLoop {
	draftArticleID := pgUUIDPtr(row.DraftArticleID)
	if draftArticleID == nil {
		draftArticleID = pgUUIDPtr(row.DraftArticleJoinedID)
	}
	return VisibilityActionInLoop{
		ID:                        row.ID,
		OpportunityID:             row.OpportunityID,
		ActionType:                row.ActionType,
		Status:                    row.Status,
		LifecycleStage:            stage,
		AssetType:                 row.AssetType,
		TargetURL:                 row.TargetUrl,
		NormalizedTargetURL:       row.NormalizedTargetUrl,
		OpportunityStatus:         row.OpportunityStatus,
		OpportunityType:           row.OpportunityType,
		OpportunityPageURL:        row.OpportunityPageUrl,
		OpportunityNormalizedURL:  row.OpportunityNormalizedPageUrl,
		OpportunityQuery:          row.OpportunityQuery,
		OpportunityRecommended:    row.OpportunityRecommendedAction,
		OpportunityExpectedImpact: row.OpportunityExpectedImpact,
		OpportunityRiskLevel:      row.OpportunityRiskLevel,
		TopicID:                   pgUUIDPtr(row.TopicID),
		TopicStatus:               row.TopicStatus,
		TopicTitle:                row.TopicTitle,
		DraftArticleID:            draftArticleID,
		DraftArticleStatus:        row.DraftArticleStatus,
		DraftArticleCanonicalURL:  row.DraftArticleCanonicalUrl,
		ReviewRequired:            row.ReviewRequired,
		ApprovalSource:            row.ApprovalSource,
		RoutingSource:             row.RoutingSource,
		WorkType:                  row.WorkType,
		PublishedAt:               pgTimePtr(row.PublishedAt),
		VerifiedAt:                pgTimePtr(row.VerifiedAt),
		MeasurementWindow:         rawOrDefault(row.MeasurementWindow, `{}`),
		MeasurementPolicyVersion:  row.MeasurementPolicyVersion,
		MeasurementPolicy:         rawOrDefault(row.MeasurementPolicy, `{}`),
		MeasuringStartedAt:        pgTimePtr(row.MeasuringStartedAt),
		AbsoluteTerminalAt:        pgTimePtr(row.AbsoluteTerminalAt),
		MeasurementTerminalReason: row.MeasurementTerminalReason,
		OutcomeSummary:            rawOrDefault(row.OutcomeSummary, `{}`),
		VerificationSnapshot:      rawOrDefault(row.VerificationSnapshot, `{}`),
		CreatedAt:                 pgTimePtr(row.CreatedAt),
		UpdatedAt:                 pgTimePtr(row.UpdatedAt),
	}
}

func resultsActionFromListRow(row db.ListResultsActionRowsRow) ResultsAction {
	return ResultsAction{
		ContentAction: db.ContentAction{
			ID:                        row.ID,
			ProjectID:                 row.ProjectID,
			OpportunityID:             row.OpportunityID,
			ActionType:                row.ActionType,
			Status:                    row.Status,
			TargetArticleID:           row.TargetArticleID,
			TargetUrl:                 row.TargetUrl,
			NormalizedTargetUrl:       row.NormalizedTargetUrl,
			TargetContentHashBefore:   row.TargetContentHashBefore,
			TargetContentHashAfter:    row.TargetContentHashAfter,
			DraftArticleID:            row.DraftArticleID,
			BaselineWindow:            rawOrDefault(row.BaselineWindow, `{}`),
			MeasurementWindow:         rawOrDefault(row.MeasurementWindow, `{}`),
			MeasurementPolicyVersion:  row.MeasurementPolicyVersion,
			MeasurementPolicy:         rawOrDefault(row.MeasurementPolicy, `{}`),
			MeasuringStartedAt:        row.MeasuringStartedAt,
			AbsoluteTerminalAt:        row.AbsoluteTerminalAt,
			MeasurementTerminalReason: row.MeasurementTerminalReason,
			PublishedAt:               row.PublishedAt,
			OutcomeSummary:            rawOrDefault(row.OutcomeSummary, `{}`),
			CreatedAt:                 row.CreatedAt,
			UpdatedAt:                 row.UpdatedAt,
			AssetType:                 row.AssetType,
			TargetSurfaceID:           row.TargetSurfaceID,
			RiskReasons:               rawOrDefault(row.RiskReasons, `[]`),
			EvidenceSnapshot:          rawOrDefault(row.EvidenceSnapshot, `{}`),
			InputSnapshot:             rawOrDefault(row.InputSnapshot, `{}`),
			OutputSnapshot:            rawOrDefault(row.OutputSnapshot, `{}`),
			DiffSnapshot:              rawOrDefault(row.DiffSnapshot, `{}`),
			ReviewRequired:            row.ReviewRequired,
			ApprovedBy:                row.ApprovedBy,
			ApprovedAt:                row.ApprovedAt,
			VerifiedAt:                row.VerifiedAt,
			VerificationSnapshot:      rawOrDefault(row.VerificationSnapshot, `{}`),
			ApprovalSource:            row.ApprovalSource,
			RoutingSource:             row.RoutingSource,
			WorkType:                  row.WorkType,
		},
		OpportunityType:              row.OpportunityType,
		OpportunityQuery:             row.OpportunityQuery,
		OpportunityPageURL:           row.OpportunityPageUrl,
		OpportunityNormalizedURL:     row.OpportunityNormalizedPageUrl,
		OpportunityRecommendedAction: row.OpportunityRecommendedAction,
		OpportunityExpectedImpact:    row.OpportunityExpectedImpact,
		TopicTitle:                   row.TopicTitle,
		DraftArticleStatus:           row.DraftArticleStatus,
		DraftArticleCanonicalURL:     row.DraftArticleCanonicalUrl,
		Measurements:                 []ActionMeasurement{},
	}
}

func resultsActionFromGetRow(row db.GetResultsActionRowRow) ResultsAction {
	return ResultsAction{
		ContentAction: db.ContentAction{
			ID:                        row.ID,
			ProjectID:                 row.ProjectID,
			OpportunityID:             row.OpportunityID,
			ActionType:                row.ActionType,
			Status:                    row.Status,
			TargetArticleID:           row.TargetArticleID,
			TargetUrl:                 row.TargetUrl,
			NormalizedTargetUrl:       row.NormalizedTargetUrl,
			TargetContentHashBefore:   row.TargetContentHashBefore,
			TargetContentHashAfter:    row.TargetContentHashAfter,
			DraftArticleID:            row.DraftArticleID,
			BaselineWindow:            rawOrDefault(row.BaselineWindow, `{}`),
			MeasurementWindow:         rawOrDefault(row.MeasurementWindow, `{}`),
			MeasurementPolicyVersion:  row.MeasurementPolicyVersion,
			MeasurementPolicy:         rawOrDefault(row.MeasurementPolicy, `{}`),
			MeasuringStartedAt:        row.MeasuringStartedAt,
			AbsoluteTerminalAt:        row.AbsoluteTerminalAt,
			MeasurementTerminalReason: row.MeasurementTerminalReason,
			PublishedAt:               row.PublishedAt,
			OutcomeSummary:            rawOrDefault(row.OutcomeSummary, `{}`),
			CreatedAt:                 row.CreatedAt,
			UpdatedAt:                 row.UpdatedAt,
			AssetType:                 row.AssetType,
			TargetSurfaceID:           row.TargetSurfaceID,
			RiskReasons:               rawOrDefault(row.RiskReasons, `[]`),
			EvidenceSnapshot:          rawOrDefault(row.EvidenceSnapshot, `{}`),
			InputSnapshot:             rawOrDefault(row.InputSnapshot, `{}`),
			OutputSnapshot:            rawOrDefault(row.OutputSnapshot, `{}`),
			DiffSnapshot:              rawOrDefault(row.DiffSnapshot, `{}`),
			ReviewRequired:            row.ReviewRequired,
			ApprovedBy:                row.ApprovedBy,
			ApprovedAt:                row.ApprovedAt,
			VerifiedAt:                row.VerifiedAt,
			VerificationSnapshot:      rawOrDefault(row.VerificationSnapshot, `{}`),
			ApprovalSource:            row.ApprovalSource,
			RoutingSource:             row.RoutingSource,
			WorkType:                  row.WorkType,
		},
		OpportunityType:              row.OpportunityType,
		OpportunityQuery:             row.OpportunityQuery,
		OpportunityPageURL:           row.OpportunityPageUrl,
		OpportunityNormalizedURL:     row.OpportunityNormalizedPageUrl,
		OpportunityRecommendedAction: row.OpportunityRecommendedAction,
		OpportunityExpectedImpact:    row.OpportunityExpectedImpact,
		TopicTitle:                   row.TopicTitle,
		DraftArticleStatus:           row.DraftArticleStatus,
		DraftArticleCanonicalURL:     row.DraftArticleCanonicalUrl,
		Measurements:                 []ActionMeasurement{},
	}
}

func attachResultsMeasurements(action *ResultsAction, measurements []ActionMeasurement) {
	action.Measurements = emptySlice(measurements)
	if len(action.Measurements) == 0 {
		action.LatestMeasurement = nil
		return
	}
	latest := action.Measurements[0]
	for _, measurement := range action.Measurements[1:] {
		if measurement.ComputedAt.Valid && (!latest.ComputedAt.Valid || measurement.ComputedAt.Time.After(latest.ComputedAt.Time)) {
			latest = measurement
			continue
		}
		if !measurement.ComputedAt.Valid && !latest.ComputedAt.Valid && measurement.CheckpointDay > latest.CheckpointDay {
			latest = measurement
		}
	}
	action.LatestMeasurement = &latest
}

func visibilityPrimaryStatus(openCount, actionCount int, setupBlockers []seopkg.SetupChecklistItem, counts map[string]int) string {
	if counts[string(VisibilityStageBlocked)] > 0 {
		return "blocked"
	}
	if openCount > 0 {
		return "review_needed"
	}
	if actionCount > 0 {
		return "loop_in_motion"
	}
	if len(setupBlockers) > 0 {
		return "limited_setup"
	}
	return "steady"
}

func visibilityMeasurementUpdate(item VisibilityActionInLoop) *VisibilityMeasurement {
	switch item.LifecycleStage {
	case VisibilityStageMeasuring, VisibilityStageLearned, VisibilityStagePublishedOrApplied:
	default:
		return nil
	}
	summary := compactJSONSummary(item.OutcomeSummary)
	if summary == "" {
		summary = measurementWindowSummary(item.MeasurementWindow)
	}
	if summary == "" {
		summary = "Measurement window is waiting for enough data."
	}
	return &VisibilityMeasurement{ActionID: item.ID, Status: string(item.LifecycleStage), Summary: summary}
}

func measurementWindowSummary(raw json.RawMessage) string {
	var data map[string]any
	if len(raw) == 0 || !json.Valid(raw) || json.Unmarshal(raw, &data) != nil {
		return ""
	}
	primary, _ := data["primary_metric"].(string)
	checkpoints, _ := data["checkpoints"].([]any)
	if primary == "" && len(checkpoints) == 0 {
		return ""
	}
	if primary == "" {
		return fmt.Sprintf("%d measurement checkpoints scheduled.", len(checkpoints))
	}
	return fmt.Sprintf("%s measurement scheduled across %d checkpoints.", primary, len(checkpoints))
}

func compactJSONSummary(raw json.RawMessage) string {
	if !rawJSONHasData(raw) {
		return ""
	}
	var data map[string]any
	if json.Unmarshal(raw, &data) != nil {
		return string(raw)
	}
	for _, key := range []string{"summary", "result", "state", "status"} {
		if value, ok := data[key]; ok {
			text := strings.TrimSpace(fmt.Sprint(value))
			if text != "" {
				return text
			}
		}
	}
	return "Outcome summary is available."
}

func rawJSONHasData(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null" && trimmed != "{}" && trimmed != "[]"
}

func pgUUIDPtr(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := uuid.UUID(value.Bytes)
	return &id
}

func pgTimePtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}
