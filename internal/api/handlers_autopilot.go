package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/autopilot"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) listSEOObjectives(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	objectives, err := s.Q.ListSEOObjectives(r.Context(), db.ListSEOObjectivesParams{
		ProjectID: projectID,
		Column2:   r.URL.Query().Get("status"),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(objectives))
}

func (s *Server) createSEOObjective(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	var in struct {
		Name            string          `json:"name"`
		Status          string          `json:"status"`
		PrimaryMetric   string          `json:"primary_metric"`
		Secondary       json.RawMessage `json:"secondary_metrics"`
		TargetPages     json.RawMessage `json:"target_pages"`
		TargetTopics    json.RawMessage `json:"target_topics"`
		TargetQueries   json.RawMessage `json:"target_queries"`
		TimeHorizonDays int32           `json:"time_horizon_days"`
		BudgetUSD       *float64        `json:"budget_usd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if in.Status == "" {
		in.Status = "active"
	}
	if in.PrimaryMetric == "" {
		in.PrimaryMetric = "clicks"
	}
	if in.TimeHorizonDays == 0 {
		in.TimeHorizonDays = 90
	}
	objective, err := s.Q.CreateSEOObjective(r.Context(), db.CreateSEOObjectiveParams{
		ProjectID:        projectID,
		Name:             strings.TrimSpace(in.Name),
		Status:           in.Status,
		PrimaryMetric:    in.PrimaryMetric,
		SecondaryMetrics: rawOrArray(in.Secondary),
		TargetPages:      rawOrArray(in.TargetPages),
		TargetTopics:     rawOrArray(in.TargetTopics),
		TargetQueries:    rawOrArray(in.TargetQueries),
		TimeHorizonDays:  in.TimeHorizonDays,
		BudgetUsd:        pgutil.NumericPtr(in.BudgetUSD),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, objective)
}

func (s *Server) getSEOPolicy(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	policy, err := s.ensureSEOPolicy(r, projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) updateSEOPolicy(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	var in struct {
		AutopilotLevel                int32           `json:"autopilot_level"`
		WeeklyActionLimit             int32           `json:"weekly_action_limit"`
		MonthlyBudgetLimit            float64         `json:"monthly_budget_limit"`
		AllowedActionTypes            json.RawMessage `json:"allowed_action_types"`
		BlockedURLPatterns            json.RawMessage `json:"blocked_url_patterns"`
		RequiresReviewActionTypes     json.RawMessage `json:"requires_review_action_types"`
		MaxAutoChangesPerPagePerMonth int32           `json:"max_auto_changes_per_page_per_month"`
		LowTrafficClicks              int32           `json:"low_traffic_clicks_28d_threshold"`
		LowTrafficImpressions         int32           `json:"low_traffic_impressions_28d_threshold"`
		MinConfidence                 float64         `json:"min_confidence_for_auto_publish"`
		QuietHoursStart               string          `json:"quiet_hours_start"`
		QuietHoursEnd                 string          `json:"quiet_hours_end"`
		QuietHoursTimezone            string          `json:"quiet_hours_timezone"`
		QuietHoursBehavior            string          `json:"quiet_hours_behavior"`
		KillSwitchEnabled             bool            `json:"kill_switch_enabled"`
		SafeModeEnabled               bool            `json:"safe_mode_enabled"`
		RiskClassifierVersion         string          `json:"risk_classifier_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.WeeklyActionLimit == 0 {
		in.WeeklyActionLimit = 5
	}
	if in.MonthlyBudgetLimit == 0 {
		in.MonthlyBudgetLimit = 25
	}
	if in.MaxAutoChangesPerPagePerMonth == 0 {
		in.MaxAutoChangesPerPagePerMonth = 1
	}
	if in.LowTrafficClicks == 0 {
		in.LowTrafficClicks = 10
	}
	if in.LowTrafficImpressions == 0 {
		in.LowTrafficImpressions = 500
	}
	if in.MinConfidence == 0 {
		in.MinConfidence = 80
	}
	if in.QuietHoursTimezone == "" {
		in.QuietHoursTimezone = "America/Los_Angeles"
	}
	if in.QuietHoursBehavior == "" {
		in.QuietHoursBehavior = "defer_to_next_window"
	}
	if in.RiskClassifierVersion == "" {
		in.RiskClassifierVersion = autopilot.DefaultRiskClassifierVersion
	}
	policy, err := s.Q.UpsertSEOPolicy(r.Context(), db.UpsertSEOPolicyParams{
		ProjectID:                         projectID,
		AutopilotLevel:                    in.AutopilotLevel,
		WeeklyActionLimit:                 in.WeeklyActionLimit,
		MonthlyBudgetLimit:                pgutil.Numeric(in.MonthlyBudgetLimit),
		AllowedActionTypes:                rawOrDefault(in.AllowedActionTypes, `["submit sitemap","metadata rewrite","technical SEO fix task"]`),
		BlockedUrlPatterns:                rawOrArray(in.BlockedURLPatterns),
		RequiresReviewActionTypes:         rawOrDefault(in.RequiresReviewActionTypes, `["refresh paragraph","create supporting article","merge pages","noindex/prune/delete","change robots/canonical rules"]`),
		MaxAutoChangesPerPagePerMonth:     in.MaxAutoChangesPerPagePerMonth,
		LowTrafficClicks28dThreshold:      in.LowTrafficClicks,
		LowTrafficImpressions28dThreshold: in.LowTrafficImpressions,
		MinConfidenceForAutoPublish:       pgutil.Numeric(in.MinConfidence),
		QuietHoursStart:                   strPtrFrom(in.QuietHoursStart),
		QuietHoursEnd:                     strPtrFrom(in.QuietHoursEnd),
		QuietHoursTimezone:                in.QuietHoursTimezone,
		QuietHoursBehavior:                in.QuietHoursBehavior,
		KillSwitchEnabled:                 in.KillSwitchEnabled,
		SafeModeEnabled:                   in.SafeModeEnabled,
		RiskClassifierVersion:             in.RiskClassifierVersion,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, _ = s.Q.UpsertRiskClassificationRule(r.Context(), db.UpsertRiskClassificationRuleParams{
		ProjectID: projectID,
		Version:   policy.RiskClassifierVersion,
		Rules:     json.RawMessage(`{"source":"deterministic_default"}`),
		CreatedBy: "system",
	})
	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) generateAutopilotPlan(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	policy, err := s.ensureSEOPolicy(r, projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	opps, err := s.Q.ListSEOOpportunities(r.Context(), db.ListSEOOpportunitiesParams{
		ProjectID: projectID,
		Status:    "open",
		LimitRows: policy.WeeklyActionLimit,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	selected := make([]map[string]any, 0, len(opps))
	for _, opp := range opps {
		result := autopilot.ClassifyRisk(autopilot.RiskInput{
			ActionType:        valueOr(opp.RecommendedAction, opp.Type),
			PageType:          "blog",
			DiffScope:         "metadata-only",
			TrafficPercentile: 0,
			Confidence:        pgutil.Float(opp.Confidence),
		}, riskPolicyFromDB(policy))
		selected = append(selected, map[string]any{
			"opportunity_id":       opp.ID,
			"type":                 opp.Type,
			"recommended_action":   opp.RecommendedAction,
			"risk_level":           result.Level,
			"risk_reasons":         result.Reasons,
			"classifier_version":   result.ClassifierVersion,
			"auto_publish_allowed": policy.AutopilotLevel >= 2 && result.Level == autopilot.RiskLow && !policy.KillSwitchEnabled && !policy.SafeModeEnabled,
		})
	}
	now := time.Now().UTC()
	run, err := s.Q.InsertAutopilotRun(r.Context(), db.InsertAutopilotRunParams{
		ProjectID:              projectID,
		Status:                 "ok",
		AutopilotLevelSnapshot: policy.AutopilotLevel,
		DerivedMode:            derivedMode(policy.AutopilotLevel),
		StartedAt:              pgutil.TS(now),
		FinishedAt:             pgutil.TS(now),
		InputSnapshot:          mustJSONLocal(map[string]any{"policy_id": policy.ID}),
		SelectedActions:        mustJSONLocal(selected),
		RejectedActions:        json.RawMessage(`[]`),
		GuardrailResults:       json.RawMessage(`[]`),
		PublishedChanges:       json.RawMessage(`[]`),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	plan, err := s.Q.CreateSEOActionPlan(r.Context(), db.CreateSEOActionPlanParams{
		ProjectID:             projectID,
		AutopilotRunID:        uuidToPGLocal(run.ID),
		PlanWindowStart:       pgtype.Date{Time: now, Valid: true},
		PlanWindowEnd:         pgtype.Date{Time: now.AddDate(0, 0, 7), Valid: true},
		Status:                "ready_for_review",
		Actions:               mustJSONLocal(selected),
		ExpectedImpact:        mustJSONLocal(map[string]any{"basis": "open_opportunities"}),
		ExpectedEffort:        int32(len(selected)),
		AggregateRisk:         aggregateRisk(selected),
		RiskClassifierVersion: policy.RiskClassifierVersion,
		ApprovalRequired:      true,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "plan": plan})
}

func (s *Server) listAutopilotPlans(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	plans, err := s.Q.ListSEOActionPlans(r.Context(), db.ListSEOActionPlansParams{ProjectID: projectID, Limit: 20})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(plans))
}

func (s *Server) enterSafeMode(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	var in struct {
		Reason        string `json:"reason"`
		TriggerSource string `json:"trigger_source"`
		EnteredBy     string `json:"entered_by"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if strings.TrimSpace(in.Reason) == "" {
		in.Reason = "manual safe mode"
	}
	if in.TriggerSource == "" {
		in.TriggerSource = "manual"
	}
	if in.EnteredBy == "" {
		in.EnteredBy = "human"
	}
	event, err := s.Q.EnterSafeMode(r.Context(), db.EnterSafeModeParams{
		ProjectID:     projectID,
		Reason:        in.Reason,
		TriggerSource: in.TriggerSource,
		EnteredBy:     in.EnteredBy,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, event)
}

func (s *Server) exitSafeMode(w http.ResponseWriter, r *http.Request) {
	projectID, eventID, ok := s.seoIDs(w, r, "safeModeID")
	if !ok {
		return
	}
	var in struct {
		ExitedBy   string `json:"exited_by"`
		ExitReason string `json:"exit_reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.ExitedBy == "" {
		in.ExitedBy = "human"
	}
	if in.ExitReason == "" {
		in.ExitReason = "manual confirmation"
	}
	event, err := s.Q.ExitSafeMode(r.Context(), db.ExitSafeModeParams{
		ID:         eventID,
		ProjectID:  projectID,
		ExitedBy:   &in.ExitedBy,
		ExitReason: &in.ExitReason,
	})
	if err != nil {
		writeErr(w, http.StatusNotFound, "safe mode event not found")
		return
	}
	writeJSON(w, http.StatusOK, event)
}

func (s *Server) listSafeModeEvents(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	events, err := s.Q.ListSafeModeEvents(r.Context(), db.ListSafeModeEventsParams{ProjectID: projectID, Limit: 20})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(events))
}

func (s *Server) ensureSEOPolicy(r *http.Request, projectID uuid.UUID) (db.SeoPolicy, error) {
	policy, err := s.Q.GetSEOPolicy(r.Context(), projectID)
	if err == nil {
		return policy, nil
	}
	if err != pgx.ErrNoRows {
		return policy, err
	}
	return s.Q.UpsertSEOPolicy(r.Context(), db.UpsertSEOPolicyParams{
		ProjectID:                         projectID,
		AutopilotLevel:                    0,
		WeeklyActionLimit:                 5,
		MonthlyBudgetLimit:                pgutil.Numeric(25),
		AllowedActionTypes:                json.RawMessage(`["submit sitemap","metadata rewrite","technical SEO fix task"]`),
		BlockedUrlPatterns:                json.RawMessage(`[]`),
		RequiresReviewActionTypes:         json.RawMessage(`["refresh paragraph","create supporting article","merge pages","noindex/prune/delete","change robots/canonical rules"]`),
		MaxAutoChangesPerPagePerMonth:     1,
		LowTrafficClicks28dThreshold:      10,
		LowTrafficImpressions28dThreshold: 500,
		MinConfidenceForAutoPublish:       pgutil.Numeric(80),
		QuietHoursTimezone:                "America/Los_Angeles",
		QuietHoursBehavior:                "defer_to_next_window",
		RiskClassifierVersion:             autopilot.DefaultRiskClassifierVersion,
	})
}

func riskPolicyFromDB(policy db.SeoPolicy) autopilot.RiskPolicy {
	return autopilot.RiskPolicy{
		LowTrafficClicks28DThreshold:      int(policy.LowTrafficClicks28dThreshold),
		LowTrafficImpressions28DThreshold: int(policy.LowTrafficImpressions28dThreshold),
		MinConfidenceForAutoPublish:       pgutil.Float(policy.MinConfidenceForAutoPublish),
		ClassifierVersion:                 policy.RiskClassifierVersion,
	}
}

func rawOrArray(raw json.RawMessage) json.RawMessage {
	return rawOrDefault(raw, `[]`)
}

func rawOrDefault(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) == 0 || !json.Valid(raw) {
		return json.RawMessage(fallback)
	}
	return raw
}

func valueOr(value *string, fallback string) string {
	if value != nil && strings.TrimSpace(*value) != "" {
		return *value
	}
	return fallback
}

func derivedMode(level int32) string {
	switch level {
	case 1:
		return "draft"
	case 2:
		return "guarded"
	case 3:
		return "portfolio"
	case 4:
		return "expanded"
	default:
		return "observe"
	}
}

func aggregateRisk(actions []map[string]any) string {
	out := "low"
	for _, action := range actions {
		risk, _ := action["risk_level"].(autopilot.RiskLevel)
		if risk == autopilot.RiskHigh {
			return "high"
		}
		if risk == autopilot.RiskMedium {
			out = "medium"
		}
	}
	return out
}

func uuidToPGLocal(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func mustJSONLocal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
