package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	seopkg "github.com/citeloop/citeloop/internal/seo"
	"github.com/citeloop/citeloop/internal/topicstate"
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
	ApprovalSource            string                   `json:"approval_source"`
	RoutingSource             string                   `json:"routing_source"`
	WorkType                  *string                  `json:"work_type,omitempty"`
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
		WorkType         string          `json:"work_type"`
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
		ApprovalSource:          ApprovalSourceHumanReview,
		RoutingSource:           routingSource,
		WorkType:                &workType,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	reviewRequired := defaultReviewRequiredForAssetType(assetTypeValue, opp.RiskLevel)
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
		OutputSnapshot:       defaultOutputSnapshotForAction(in.OutputSnapshot, assetTypeValue, actionType, opp),
		DiffSnapshot:         defaultDiffSnapshotForAction(in.DiffSnapshot, assetTypeValue, actionType, opp),
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

func (s *Server) planSEOContentAction(w http.ResponseWriter, r *http.Request) {
	projectID, actionID, ok := s.seoIDs(w, r, "actionID")
	if !ok {
		return
	}
	action, err := s.Q.GetContentAction(r.Context(), db.GetContentActionParams{ID: actionID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "action not found")
		return
	}
	if !contentActionNeedsTopic(contentActionAssetType(action), action.ActionType) {
		writeErr(w, http.StatusBadRequest, "content action does not create content")
		return
	}
	if topic, ok, err := s.existingTopicForContentAction(r.Context(), projectID, actionID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	} else if ok {
		writeJSON(w, http.StatusOK, topic)
		return
	}
	opp, err := s.Q.GetSEOOpportunity(r.Context(), db.GetSEOOpportunityParams{ID: action.OpportunityID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "opportunity not found")
		return
	}
	topic, err := s.Q.CreateTopic(r.Context(), topicFromContentAction(projectID, action, opp))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.Q.UpdateContentActionStatus(r.Context(), db.UpdateContentActionStatusParams{
		ID:        action.ID,
		ProjectID: projectID,
		Status:    "approved",
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.Q.EnqueueWorkflowEvent(r.Context(), db.EnqueueWorkflowEventParams{
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
	writeJSON(w, http.StatusOK, topic)
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
	return map[string]any{
		"issue": compactActionPayload(map[string]any{
			"category":        "site_fix",
			"issue_type":      assetType,
			"affected_urls":   nonEmptyStrings(targetURL),
			"normalized_urls": nonEmptyStrings(opp.NormalizedPageUrl),
			"problem":         actionType,
			"why_it_matters":  firstNonEmptyString(stringPtrValueAPI(opp.ExpectedImpact), seoGeoContributionForAsset(assetType)),
		}),
		"evidence": directActionRepairEvidence(opp),
		"fix": compactActionPayload(map[string]any{
			"goal":               actionType,
			"instructions":       directActionImplementationSteps(assetType, actionType, targetURL),
			"likely_surfaces":    directActionLikelySurfaces(assetType, targetURL),
			"seo_contract":       directActionPatchContract(assetType, targetURL),
			"deduplication_rule": directActionDeduplicationRule(assetType),
			"do_not":             directActionDoNot(assetType),
			"risk_level":         opp.RiskLevel,
		}),
		"acceptance_tests": directActionAcceptanceTests(assetType, actionType, targetURL),
		"human_review":     directActionHumanReview(assetType),
	}
}

func directActionRepairEvidence(opp db.SeoOpportunity) map[string]any {
	evidence := compactActionPayload(map[string]any{
		"page_url":            opportunityTargetURL(opp),
		"normalized_page_url": opp.NormalizedPageUrl,
		"opportunity_type":    opp.Type,
		"query":               stringPtrValueAPI(opp.Query),
		"recommended_action":  stringPtrValueAPI(opp.RecommendedAction),
		"expected_impact":     stringPtrValueAPI(opp.ExpectedImpact),
	})
	if rawHasMeaningfulJSON(opp.Evidence) {
		var parsed any
		if err := json.Unmarshal(opp.Evidence, &parsed); err == nil {
			if observedMetadata := directActionObservedMetadata(parsed); len(observedMetadata) > 0 {
				evidence["observed_metadata"] = observedMetadata
			}
			evidence["source_evidence"] = parsed
		}
	}
	return evidence
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

func contentActionNeedsTopic(assetType string, actionType string) bool {
	text := strings.ToLower(strings.TrimSpace(assetType + " " + actionType))
	switch {
	case strings.Contains(text, "metadata_rewrite") || strings.Contains(text, "metadata patch") || strings.Contains(text, "metadata"):
		return false
	case strings.Contains(text, "title") || strings.Contains(text, "meta description"):
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
		Channel:               "blog",
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
			ApprovalSource:          row.ApprovalSource,
			RoutingSource:           row.RoutingSource,
			WorkType:                row.WorkType,
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
			ApprovalSource:          row.ApprovalSource,
			RoutingSource:           row.RoutingSource,
			WorkType:                row.WorkType,
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
