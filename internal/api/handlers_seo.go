package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	seopkg "github.com/citeloop/citeloop/internal/seo"
	"github.com/citeloop/citeloop/internal/workflow"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) seoService() seopkg.Service {
	return seopkg.Service{Q: s.Q, BlogBaseURL: s.Env.BlogBaseURL, GoogleData: s.SEOData}
}

func (s *Server) seoServiceForProject(ctx context.Context, projectID uuid.UUID) seopkg.Service {
	svc := s.seoService()
	if provider, override := s.googleDataProviderForProject(ctx, projectID); override {
		svc.GoogleData = provider
	}
	return svc
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
	PublishedAt               *time.Time               `json:"published_at,omitempty"`
	VerifiedAt                *time.Time               `json:"verified_at,omitempty"`
	MeasurementWindow         json.RawMessage          `json:"measurement_window"`
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
	svc := s.seoServiceForProject(r.Context(), projectID)
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
	result, err := s.seoService().Analyze(r.Context(), projectID)
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

func (s *Server) createSEOContentActionFromOpportunity(w http.ResponseWriter, r *http.Request, successStatus int) {
	projectID, oppID, ok := s.seoIDs(w, r, "opportunityID")
	if !ok {
		return
	}
	var in struct {
		ActionType       string          `json:"action_type"`
		AssetType        string          `json:"asset_type"`
		TargetSurfaceID  *uuid.UUID      `json:"target_surface_id"`
		RiskReasons      json.RawMessage `json:"risk_reasons"`
		EvidenceSnapshot json.RawMessage `json:"evidence_snapshot"`
		InputSnapshot    json.RawMessage `json:"input_snapshot"`
		OutputSnapshot   json.RawMessage `json:"output_snapshot"`
		DiffSnapshot     json.RawMessage `json:"diff_snapshot"`
		ReviewRequired   *bool           `json:"review_required"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	opp, err := s.Q.GetSEOOpportunity(r.Context(), db.GetSEOOpportunityParams{ID: oppID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "opportunity not found")
		return
	}
	actionType := strings.TrimSpace(in.ActionType)
	if actionType == "" && opp.RecommendedAction != nil {
		actionType = *opp.RecommendedAction
	}
	if actionType == "" {
		actionType = "technical SEO fix task"
	}
	assetTypeValue := strings.TrimSpace(in.AssetType)
	var assetType *string
	if assetTypeValue != "" {
		assetType = &assetTypeValue
	}
	var targetHash *string
	if opp.ArticleID.Valid {
		article, err := s.Q.GetArticleForProject(r.Context(), db.GetArticleForProjectParams{
			ID:        uuid.UUID(opp.ArticleID.Bytes),
			ProjectID: projectID,
		})
		if err == nil {
			targetHash = article.ContentHash
		}
	}
	action, err := s.Q.CreateContentAction(r.Context(), db.CreateContentActionParams{
		ProjectID:               projectID,
		OpportunityID:           oppID,
		ActionType:              actionType,
		Status:                  "ready_for_review",
		TargetArticleID:         opp.ArticleID,
		TargetUrl:               opp.PageUrl,
		NormalizedTargetUrl:     strPtrFrom(opp.NormalizedPageUrl),
		TargetContentHashBefore: targetHash,
		BaselineWindow:          json.RawMessage(`{"days":28}`),
		MeasurementWindow:       measurementWindowForAction(assetTypeValue, actionType),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	reviewRequired := true
	if in.ReviewRequired != nil {
		reviewRequired = *in.ReviewRequired
	}
	targetSurfaceID := pgtype.UUID{}
	if in.TargetSurfaceID != nil {
		targetSurfaceID = pgtype.UUID{Bytes: *in.TargetSurfaceID, Valid: true}
	}
	action, err = s.Q.UpdateContentActionExecutionMetadata(r.Context(), db.UpdateContentActionExecutionMetadataParams{
		ID:                   action.ID,
		ProjectID:            projectID,
		AssetType:            assetType,
		TargetSurfaceID:      targetSurfaceID,
		RiskReasons:          rawOrDefault(in.RiskReasons, `[]`),
		EvidenceSnapshot:     contentActionEvidenceSnapshot(in.EvidenceSnapshot, opp),
		InputSnapshot:        contentActionInputSnapshot(in.InputSnapshot, opp, actionType),
		OutputSnapshot:       rawOrDefault(in.OutputSnapshot, `{}`),
		DiffSnapshot:         rawOrDefault(in.DiffSnapshot, `{}`),
		ReviewRequired:       reviewRequired,
		VerificationSnapshot: json.RawMessage(`{}`),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _ = s.Q.UpdateSEOOpportunityStatus(r.Context(), db.UpdateSEOOpportunityStatusParams{ID: oppID, ProjectID: projectID, Status: "converted"})
	if err := s.enqueueWorkflowEvent(r.Context(), projectID, workflow.EventOpportunityReviewed, "seo_opportunity", oppID, workflowDedupeKey(workflow.EventOpportunityReviewed, projectID, oppID, "converted"), map[string]any{
		"opportunity_id": oppID,
		"action_id":      action.ID,
		"status":         "converted",
	}); err != nil {
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
	status := "missing"
	var verified pgtype.Timestamptz
	if strings.TrimSpace(in.CredentialRef) != "" {
		status = "connected"
		verified = pgutil.TS(time.Now().UTC())
	}
	gscIntegration, err := s.Q.UpsertSEOIntegration(r.Context(), db.UpsertSEOIntegrationParams{
		ProjectID:      projectID,
		Provider:       seopkg.ProviderGSC,
		Status:         status,
		CredentialRef:  strPtrFrom(in.CredentialRef),
		LastVerifiedAt: verified,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var ga4Integration *db.SeoIntegration
	if strings.TrimSpace(in.GA4PropertyID) != "" || strings.TrimSpace(in.CredentialRef) != "" {
		row, err := s.Q.UpsertSEOIntegration(r.Context(), db.UpsertSEOIntegrationParams{
			ProjectID:      projectID,
			Provider:       seopkg.ProviderGA4,
			Status:         status,
			CredentialRef:  strPtrFrom(in.CredentialRef),
			LastVerifiedAt: verified,
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		ga4Integration = &row
	}
	writeJSON(w, http.StatusOK, map[string]any{"property": prop, "integration": gscIntegration, "ga4_integration": ga4Integration})
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
	if row.DraftArticleID.Valid || row.DraftArticleJoinedID.Valid {
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
		PublishedAt:               pgTimePtr(row.PublishedAt),
		VerifiedAt:                pgTimePtr(row.VerifiedAt),
		MeasurementWindow:         rawOrDefault(row.MeasurementWindow, `{}`),
		OutcomeSummary:            rawOrDefault(row.OutcomeSummary, `{}`),
		VerificationSnapshot:      rawOrDefault(row.VerificationSnapshot, `{}`),
		CreatedAt:                 pgTimePtr(row.CreatedAt),
		UpdatedAt:                 pgTimePtr(row.UpdatedAt),
	}
}

func resultsActionFromListRow(row db.ListResultsActionRowsRow) ResultsAction {
	return ResultsAction{
		ContentAction: db.ContentAction{
			ID:                      row.ID,
			ProjectID:               row.ProjectID,
			OpportunityID:           row.OpportunityID,
			ActionType:              row.ActionType,
			Status:                  row.Status,
			TargetArticleID:         row.TargetArticleID,
			TargetUrl:               row.TargetUrl,
			NormalizedTargetUrl:     row.NormalizedTargetUrl,
			TargetContentHashBefore: row.TargetContentHashBefore,
			TargetContentHashAfter:  row.TargetContentHashAfter,
			DraftArticleID:          row.DraftArticleID,
			BaselineWindow:          rawOrDefault(row.BaselineWindow, `{}`),
			MeasurementWindow:       rawOrDefault(row.MeasurementWindow, `{}`),
			PublishedAt:             row.PublishedAt,
			OutcomeSummary:          rawOrDefault(row.OutcomeSummary, `{}`),
			CreatedAt:               row.CreatedAt,
			UpdatedAt:               row.UpdatedAt,
			AssetType:               row.AssetType,
			TargetSurfaceID:         row.TargetSurfaceID,
			RiskReasons:             rawOrDefault(row.RiskReasons, `[]`),
			EvidenceSnapshot:        rawOrDefault(row.EvidenceSnapshot, `{}`),
			InputSnapshot:           rawOrDefault(row.InputSnapshot, `{}`),
			OutputSnapshot:          rawOrDefault(row.OutputSnapshot, `{}`),
			DiffSnapshot:            rawOrDefault(row.DiffSnapshot, `{}`),
			ReviewRequired:          row.ReviewRequired,
			ApprovedBy:              row.ApprovedBy,
			ApprovedAt:              row.ApprovedAt,
			VerifiedAt:              row.VerifiedAt,
			VerificationSnapshot:    rawOrDefault(row.VerificationSnapshot, `{}`),
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
			ID:                      row.ID,
			ProjectID:               row.ProjectID,
			OpportunityID:           row.OpportunityID,
			ActionType:              row.ActionType,
			Status:                  row.Status,
			TargetArticleID:         row.TargetArticleID,
			TargetUrl:               row.TargetUrl,
			NormalizedTargetUrl:     row.NormalizedTargetUrl,
			TargetContentHashBefore: row.TargetContentHashBefore,
			TargetContentHashAfter:  row.TargetContentHashAfter,
			DraftArticleID:          row.DraftArticleID,
			BaselineWindow:          rawOrDefault(row.BaselineWindow, `{}`),
			MeasurementWindow:       rawOrDefault(row.MeasurementWindow, `{}`),
			PublishedAt:             row.PublishedAt,
			OutcomeSummary:          rawOrDefault(row.OutcomeSummary, `{}`),
			CreatedAt:               row.CreatedAt,
			UpdatedAt:               row.UpdatedAt,
			AssetType:               row.AssetType,
			TargetSurfaceID:         row.TargetSurfaceID,
			RiskReasons:             rawOrDefault(row.RiskReasons, `[]`),
			EvidenceSnapshot:        rawOrDefault(row.EvidenceSnapshot, `{}`),
			InputSnapshot:           rawOrDefault(row.InputSnapshot, `{}`),
			OutputSnapshot:          rawOrDefault(row.OutputSnapshot, `{}`),
			DiffSnapshot:            rawOrDefault(row.DiffSnapshot, `{}`),
			ReviewRequired:          row.ReviewRequired,
			ApprovedBy:              row.ApprovedBy,
			ApprovedAt:              row.ApprovedAt,
			VerifiedAt:              row.VerifiedAt,
			VerificationSnapshot:    rawOrDefault(row.VerificationSnapshot, `{}`),
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
