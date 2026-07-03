package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/autopilot"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/publisher"
	seopkg "github.com/citeloop/citeloop/internal/seo"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type AutopilotReadinessGate struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
	NextAction string `json:"next_action"`
	Blocking   bool   `json:"blocking"`
}

type AutopilotReadiness struct {
	ReadyForLevel2        bool                     `json:"ready_for_level_2"`
	AutopilotLevel        int32                    `json:"autopilot_level"`
	DerivedMode           string                   `json:"derived_mode"`
	AutomationPaused      bool                     `json:"automation_paused"`
	SafeModeActive        bool                     `json:"safe_mode_active"`
	KillSwitchEnabled     bool                     `json:"kill_switch_enabled"`
	FailedGates           []string                 `json:"failed_gates"`
	Gates                 []AutopilotReadinessGate `json:"gates"`
	PublisherCapabilities map[string]bool          `json:"publisher_capabilities"`
	LowRiskActionTypes    []string                 `json:"low_risk_action_types"`
	GeneratedAt           time.Time                `json:"generated_at"`
}

type AutopilotPlanAction struct {
	OpportunityID       string         `json:"opportunity_id"`
	Type                string         `json:"type"`
	RecommendedAction   *string        `json:"recommended_action"`
	ActionBucket        string         `json:"action_bucket"`
	AssetType           *string        `json:"asset_type"`
	RiskLevel           string         `json:"risk_level"`
	RiskReasons         []string       `json:"risk_reasons"`
	ClassifierVersion   string         `json:"classifier_version"`
	AutoPublishAllowed  bool           `json:"auto_publish_allowed"`
	ReviewRequired      bool           `json:"review_required"`
	MeasurementSchedule map[string]any `json:"measurement_schedule"`
}

type AutopilotExecuteResult struct {
	Plan            db.SeoActionPlan   `json:"plan"`
	ExecutedActions []db.ContentAction `json:"executed_actions"`
	DeferredActions []map[string]any   `json:"deferred_actions"`
	Readiness       AutopilotReadiness `json:"readiness"`
	GuardrailResult []map[string]any   `json:"guardrail_results"`
	RecoveryPlans   []map[string]any   `json:"recovery_plans"`
	GeneratedAt     time.Time          `json:"generated_at"`
}

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
		AutomationPaused              bool            `json:"automation_paused"`
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
		RecoveryPlanAcknowledged      bool            `json:"recovery_plan_acknowledged"`
		RecoveryPlanAcknowledgedBy    string          `json:"recovery_plan_acknowledged_by"`
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
		AutomationPaused:                  in.AutomationPaused,
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
	if in.RecoveryPlanAcknowledged {
		ackBy := strings.TrimSpace(in.RecoveryPlanAcknowledgedBy)
		if ackBy == "" {
			ackBy = "human"
		}
		policy, err = s.Q.AcknowledgeSEOPolicyRecoveryPlan(r.Context(), db.AcknowledgeSEOPolicyRecoveryPlanParams{
			ProjectID:                  projectID,
			RecoveryPlanAcknowledgedBy: ackBy,
		})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) getAutopilotReadiness(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	readiness, err := s.autopilotReadiness(r, projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, readiness)
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
		autoPublishAllowed := policy.AutopilotLevel >= 2 && result.Level == autopilot.RiskLow && !policy.AutomationPaused && !policy.KillSwitchEnabled && !policy.SafeModeEnabled
		actionBucket := actionBucketFor(valueOr(opp.RecommendedAction, opp.Type), opp.Type)
		measurementSchedule := measurementScheduleForAction(actionBucket)
		selected = append(selected, map[string]any{
			"opportunity_id":       opp.ID,
			"type":                 opp.Type,
			"recommended_action":   opp.RecommendedAction,
			"action_bucket":        actionBucket,
			"risk_level":           result.Level,
			"risk_reasons":         result.Reasons,
			"classifier_version":   result.ClassifierVersion,
			"auto_publish_allowed": autoPublishAllowed,
			"review_required":      !autoPublishAllowed,
			"measurement_schedule": measurementSchedule,
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
	portfolio := actionPortfolioDocument(selected, []map[string]any{}, policy, now)
	plan, err := s.Q.CreateSEOActionPlan(r.Context(), db.CreateSEOActionPlanParams{
		ProjectID:             projectID,
		AutopilotRunID:        uuidToPGLocal(run.ID),
		PlanWindowStart:       pgtype.Date{Time: now, Valid: true},
		PlanWindowEnd:         pgtype.Date{Time: now.AddDate(0, 0, 7), Valid: true},
		Status:                "ready_for_review",
		Actions:               mustJSONLocal(portfolio),
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

func (s *Server) autopilotReadiness(r *http.Request, projectID uuid.UUID) (AutopilotReadiness, error) {
	policy, err := s.ensureSEOPolicy(r, projectID)
	if err != nil {
		return AutopilotReadiness{}, err
	}
	overview, err := s.seoService().Overview(r.Context(), projectID)
	if err != nil {
		return AutopilotReadiness{}, err
	}
	publisherConnections, err := s.Q.ListPublisherConnections(r.Context(), projectID)
	if err != nil {
		return AutopilotReadiness{}, err
	}
	notificationChannels, err := s.Q.ListNotificationChannels(r.Context(), projectID)
	if err != nil {
		return AutopilotReadiness{}, err
	}
	safeModeActive := policy.SafeModeEnabled
	if _, err := s.Q.GetOpenSafeModeEvent(r.Context(), projectID); err == nil {
		safeModeActive = true
	} else if err != pgx.ErrNoRows {
		return AutopilotReadiness{}, err
	}

	searchReady := checklistStatus(overview.SetupChecklist, "search_data") == "connected"
	publisherReady := checklistStatus(overview.SetupChecklist, "publisher_write") == "connected"
	notificationReady, notificationStatus := verifiedNotificationChannelStatus(notificationChannels)
	policyConfirmed := policy.AutopilotLevel >= 2
	budgetConfigured := pgutil.Float(policy.MonthlyBudgetLimit) > 0
	capabilities := activePublisherCapabilities(publisherConnections)
	recoveryAcknowledged := policy.RecoveryPlanAcknowledgedAt.Valid
	rollbackOrRecoveryReady := publisherReady && (capabilities[publisher.CapabilityRollback] || recoveryAcknowledged)

	gates := []AutopilotReadinessGate{
		readinessGate("search_read", "Search data", searchReady, "First-party Search Console data is required before Level 2 can execute SEO changes.", "Connect Search Console or keep Autopilot below Level 2."),
		readinessGate("publisher_write", "Publisher write", publisherReady, "A connected, enabled publisher with scoped credentials is required before Autopilot can create or update content.", "Connect and test the GitHub/Next.js publisher."),
		readinessGateWithStatus("notification_write", "Notifications", notificationReady, notificationStatus, "Verified notifications are required so failures and approval needs reach the operator.", "Create and test a Slack or Discord notification channel."),
		readinessGate("autopilot_policy_confirmed", "Policy confirmed", policyConfirmed, "Level 2 requires an explicit policy level so CiteLoop knows which low-risk actions may run.", "Set Autopilot level to 2 after reviewing limits and risk thresholds."),
		readinessGate("automation_pause_clear", "Automation paused", !policy.AutomationPaused, "Automation is paused and blocks scheduled automation and guarded execution.", "Resume automation in Settings when you want CiteLoop to run eligible work again."),
		readinessGate("monthly_budget_configured", "Monthly budget", budgetConfigured, "A project-level budget cap is required before automated execution can spend variable cost.", "Set a monthly Autopilot budget greater than 0."),
		readinessGate("safe_mode_clear", "Safe mode clear", !safeModeActive, "Open safe mode blocks all automatic execution.", "Resolve the safe mode reason, then exit safe mode."),
		readinessGate("kill_switch_clear", "Kill switch clear", !policy.KillSwitchEnabled, "The kill switch is enabled and blocks Level 2 execution.", "Turn off the kill switch only when you are ready to resume automation."),
		readinessGate("rollback_or_recovery_ready", "Rollback or recovery ready", rollbackOrRecoveryReady, "Every Level 2 action must have publisher rollback support or an acknowledged manual recovery plan.", "Open Settings Automation and acknowledge the manual recovery plan."),
	}
	failed := make([]string, 0)
	for _, gate := range gates {
		if gate.Blocking {
			failed = append(failed, gate.Key)
		}
	}
	return AutopilotReadiness{
		ReadyForLevel2:        len(failed) == 0,
		AutopilotLevel:        policy.AutopilotLevel,
		DerivedMode:           derivedMode(policy.AutopilotLevel),
		AutomationPaused:      policy.AutomationPaused,
		SafeModeActive:        safeModeActive,
		KillSwitchEnabled:     policy.KillSwitchEnabled,
		FailedGates:           failed,
		Gates:                 gates,
		PublisherCapabilities: capabilities,
		LowRiskActionTypes:    lowRiskActionTypes(),
		GeneratedAt:           time.Now().UTC(),
	}, nil
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

func (s *Server) executeAutopilotPlan(w http.ResponseWriter, r *http.Request) {
	projectID, planID, ok := s.seoIDs(w, r, "planID")
	if !ok {
		return
	}
	plan, err := s.Q.GetSEOActionPlanForProject(r.Context(), db.GetSEOActionPlanForProjectParams{ID: planID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "autopilot plan not found")
		return
	}
	readiness, err := s.autopilotReadiness(r, projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !readiness.ReadyForLevel2 {
		blocked, _ := s.Q.UpdateSEOActionPlanStatus(r.Context(), db.UpdateSEOActionPlanStatusParams{ID: plan.ID, ProjectID: projectID, Status: "blocked"})
		if blocked.ID != uuid.Nil {
			plan = blocked
		}
		// records autopilot_audit_events: policy_not_ready blocks guarded execution before any write.
		_, _ = s.Q.InsertAutopilotAuditEvent(r.Context(), db.InsertAutopilotAuditEventParams{
			ProjectID:      projectID,
			Actor:          "autopilot",
			EventType:      "autopilot_execute_blocked",
			EntityType:     "seo_action_plan",
			EntityID:       uuidToPGLocal(plan.ID),
			BeforeSnapshot: mustJSONLocal(map[string]any{"status": plan.Status}),
			AfterSnapshot:  mustJSONLocal(map[string]any{"reason": "policy_not_ready", "failed_gates": readiness.FailedGates}),
		})
		writeJSON(w, http.StatusConflict, map[string]any{"error": "policy_not_ready", "plan": plan, "readiness": readiness})
		return
	}

	actions := selectedAutopilotPlanActions(plan.Actions)
	policy, err := s.ensureSEOPolicy(r, projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now().UTC()
	executed := make([]db.ContentAction, 0)
	deferred := make([]map[string]any, 0)
	guardrailResults := make([]map[string]any, 0, len(actions))
	recoveryPlans := make([]map[string]any, 0)

	for _, candidate := range actions {
		allowed, reason := candidateAllowedForExecution(candidate, policy, readiness.PublisherCapabilities)
		guardrail := map[string]any{
			"opportunity_id":       candidate.OpportunityID,
			"auto_publish_allowed": candidate.AutoPublishAllowed,
			"risk_level":           candidate.RiskLevel,
			"action_bucket":        candidate.ActionBucket,
			"publisher_capability": requiredPublisherCapability(candidate.ActionBucket),
			"status":               "passed",
		}
		if !allowed {
			guardrail["status"] = "blocked"
			guardrail["reason"] = reason
			deferred = append(deferred, map[string]any{
				"opportunity_id": candidate.OpportunityID,
				"reason":         reason,
				"action_bucket":  candidate.ActionBucket,
			})
			guardrailResults = append(guardrailResults, guardrail)
			s.auditAutopilotAction(r, projectID, plan.ID, "autopilot_action_deferred", map[string]any{"candidate": candidate, "reason": reason})
			continue
		}
		action, err := s.executeAutopilotCandidate(r, projectID, plan.ID, candidate, guardrail, now)
		if err != nil {
			guardrail["status"] = "blocked"
			guardrail["reason"] = err.Error()
			deferred = append(deferred, map[string]any{
				"opportunity_id": candidate.OpportunityID,
				"reason":         err.Error(),
				"action_bucket":  candidate.ActionBucket,
			})
			guardrailResults = append(guardrailResults, guardrail)
			s.auditAutopilotAction(r, projectID, plan.ID, "autopilot_action_deferred", map[string]any{"candidate": candidate, "reason": err.Error()})
			continue
		}
		executed = append(executed, action)
		guardrailResults = append(guardrailResults, guardrail)
		recoveryPlans = append(recoveryPlans, map[string]any{
			"action_id":                action.ID,
			"manual_rollback_required": !readiness.PublisherCapabilities[publisher.CapabilityRollback],
			"recovery_plan":            manualRecoveryPlan(candidate),
		})
		s.auditAutopilotAction(r, projectID, action.ID, "autopilot_action_executed", map[string]any{"candidate": candidate, "action_id": action.ID})
	}

	status := "blocked"
	if len(executed) > 0 {
		status = "executing"
	}
	updated, err := s.Q.UpdateSEOActionPlanStatus(r.Context(), db.UpdateSEOActionPlanStatusParams{ID: plan.ID, ProjectID: projectID, Status: status})
	if err == nil {
		plan = updated
	}
	writeJSON(w, http.StatusOK, AutopilotExecuteResult{
		Plan:            plan,
		ExecutedActions: executed,
		DeferredActions: deferred,
		Readiness:       readiness,
		GuardrailResult: guardrailResults,
		RecoveryPlans:   recoveryPlans,
		GeneratedAt:     now,
	})
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

func readinessGate(key, label string, passed bool, reason, nextAction string) AutopilotReadinessGate {
	status := "connected"
	if !passed {
		status = "blocked"
	}
	return readinessGateWithStatus(key, label, passed, status, reason, nextAction)
}

func readinessGateWithStatus(key, label string, passed bool, status, reason, nextAction string) AutopilotReadinessGate {
	return AutopilotReadinessGate{
		Key:        key,
		Label:      label,
		Status:     status,
		Reason:     reason,
		NextAction: nextAction,
		Blocking:   !passed,
	}
}

func checklistStatus(items []seopkg.SetupChecklistItem, key string) string {
	for _, item := range items {
		if item.Key == key {
			return item.Status
		}
	}
	return ""
}

func verifiedNotificationChannelStatus(channels []db.NotificationChannel) (bool, string) {
	if len(channels) == 0 {
		return false, "blocked"
	}
	for _, channel := range channels {
		if channel.VerifiedAt.Valid {
			return true, "connected"
		}
	}
	return false, "in_progress"
}

func activePublisherCapabilities(connections []db.PublisherConnection) map[string]bool {
	capabilities := map[string]bool{}
	for _, connection := range connections {
		if connection.Kind != publisher.ConnectionKindGitHubNextJS || connection.Status != "connected" || !connection.Enabled {
			continue
		}
		var parsed map[string]bool
		if err := json.Unmarshal(connection.Capabilities, &parsed); err != nil {
			continue
		}
		for key, value := range parsed {
			capabilities[key] = capabilities[key] || value
		}
	}
	return capabilities
}

func manualRecoverySupported(connections []db.PublisherConnection) bool {
	for _, connection := range connections {
		if connection.Kind == publisher.ConnectionKindGitHubNextJS && connection.Status == "connected" && connection.Enabled {
			return true
		}
	}
	return false
}

func lowRiskActionTypes() []string {
	return []string{
		"metadata rewrite",
		"add internal links",
		"refresh short paragraph with existing evidence",
		"create supporting article draft",
		"submit sitemap",
		"rerun GEO observation",
	}
}

func selectedAutopilotPlanActions(raw json.RawMessage) []AutopilotPlanAction {
	var doc struct {
		SelectedActions []AutopilotPlanAction `json:"selected_actions"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &doc) != nil {
		return []AutopilotPlanAction{}
	}
	return doc.SelectedActions
}

func candidateAllowedForExecution(candidate AutopilotPlanAction, policy db.SeoPolicy, capabilities map[string]bool) (bool, string) {
	if !candidate.AutoPublishAllowed {
		return false, "auto_publish_not_allowed"
	}
	if strings.ToLower(strings.TrimSpace(candidate.RiskLevel)) != string(autopilot.RiskLow) {
		return false, "risk_requires_review"
	}
	if !policyAllowsAction(policy.AllowedActionTypes, candidate) {
		return false, "policy_action_type_not_allowed"
	}
	required := requiredPublisherCapability(candidate.ActionBucket)
	if required != "" && !capabilities[required] {
		return false, "publisher_capability_missing"
	}
	return true, "allowed"
}

func policyAllowsAction(raw json.RawMessage, candidate AutopilotPlanAction) bool {
	allowed := stringListFromRaw(raw)
	if len(allowed) == 0 {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(candidate.ActionBucket + " " + valueOr(candidate.RecommendedAction, candidate.Type)))
	for _, item := range allowed {
		normalized := strings.ToLower(strings.TrimSpace(item))
		if normalized == "" {
			continue
		}
		if strings.Contains(text, normalized) || actionSynonymAllowed(normalized, text) {
			return true
		}
	}
	return false
}

func actionSynonymAllowed(allowed, text string) bool {
	switch allowed {
	case "metadata rewrite":
		return strings.Contains(text, "title") || strings.Contains(text, "meta")
	case "technical seo fix task":
		return strings.Contains(text, "sitemap") || strings.Contains(text, "structured") || strings.Contains(text, "schema")
	case "submit sitemap":
		return strings.Contains(text, "sitemap")
	default:
		return false
	}
}

func requiredPublisherCapability(actionBucket string) string {
	switch strings.ToLower(strings.TrimSpace(actionBucket)) {
	case "rewrite title/meta":
		return publisher.CapabilityMetadataUpdate
	case "add internal links", "refresh existing page":
		return publisher.CapabilityUpdateArticle
	case "create new asset":
		return publisher.CapabilityCreateArticle
	case "submit/update sitemap":
		return publisher.CapabilityPublishMode
	default:
		return publisher.CapabilityCreateArticle
	}
}

func stringListFromRaw(raw json.RawMessage) []string {
	var values []string
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return nil
	}
	return values
}

func (s *Server) executeAutopilotCandidate(r *http.Request, projectID, planID uuid.UUID, candidate AutopilotPlanAction, guardrail map[string]any, now time.Time) (db.ContentAction, error) {
	opportunityID, err := uuid.Parse(strings.TrimSpace(candidate.OpportunityID))
	if err != nil {
		return db.ContentAction{}, err
	}
	opp, err := s.Q.GetSEOOpportunity(r.Context(), db.GetSEOOpportunityParams{ID: opportunityID, ProjectID: projectID})
	if err != nil {
		return db.ContentAction{}, err
	}
	actionType := strings.TrimSpace(valueOr(candidate.RecommendedAction, candidate.Type))
	if actionType == "" {
		actionType = candidate.ActionBucket
	}
	assetTypeValue := ""
	if candidate.AssetType != nil {
		assetTypeValue = strings.TrimSpace(*candidate.AssetType)
	}
	targetHash := (*string)(nil)
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
		OpportunityID:           opp.ID,
		ActionType:              actionType,
		Status:                  "approved",
		TargetArticleID:         opp.ArticleID,
		TargetUrl:               opp.PageUrl,
		NormalizedTargetUrl:     strPtrFrom(opp.NormalizedPageUrl),
		TargetContentHashBefore: targetHash,
		BaselineWindow:          json.RawMessage(`{"days":28}`),
		MeasurementWindow:       measurementWindowForAction(assetTypeValue, actionType),
	})
	if err != nil {
		return db.ContentAction{}, err
	}
	approvedBy := "autopilot"
	action, err = s.Q.UpdateContentActionExecutionMetadata(r.Context(), db.UpdateContentActionExecutionMetadataParams{
		ID:               action.ID,
		ProjectID:        projectID,
		AssetType:        candidate.AssetType,
		RiskReasons:      mustJSONLocal(candidate.RiskReasons),
		EvidenceSnapshot: contentActionEvidenceSnapshot(nil, opp),
		InputSnapshot: mustJSONLocal(map[string]any{
			"source":         "guarded_autopilot",
			"plan_id":        planID,
			"opportunity_id": opp.ID,
			"action_bucket":  candidate.ActionBucket,
		}),
		OutputSnapshot: mustJSONLocal(map[string]any{
			"guardrail_results": []map[string]any{guardrail},
			"publisher_capability": map[string]any{
				"required": requiredPublisherCapability(candidate.ActionBucket),
			},
		}),
		DiffSnapshot: mustJSONLocal(map[string]any{
			"manual_rollback_required": true,
			"recovery_plan":            manualRecoveryPlan(candidate),
			"diff_source":              "guarded_autopilot_plan",
		}),
		ReviewRequired:       false,
		ApprovedBy:           &approvedBy,
		ApprovedAt:           pgutil.TS(now),
		VerificationSnapshot: json.RawMessage(`{}`),
	})
	if err != nil {
		return db.ContentAction{}, err
	}
	_, _ = s.Q.UpdateSEOOpportunityStatus(r.Context(), db.UpdateSEOOpportunityStatusParams{ID: opp.ID, ProjectID: projectID, Status: "converted"})
	return action, nil
}

func manualRecoveryPlan(candidate AutopilotPlanAction) []string {
	action := strings.TrimSpace(valueOr(candidate.RecommendedAction, candidate.Type))
	if action == "" {
		action = candidate.ActionBucket
	}
	return []string{
		"Open the publisher repository or CMS change created for this action.",
		"Revert the metadata or content diff tied to: " + action + ".",
		"Re-run CiteLoop publish verification after rollback.",
	}
}

func (s *Server) auditAutopilotAction(r *http.Request, projectID, entityID uuid.UUID, eventType string, snapshot map[string]any) {
	_, _ = s.Q.InsertAutopilotAuditEvent(r.Context(), db.InsertAutopilotAuditEventParams{
		ProjectID:      projectID,
		Actor:          "autopilot",
		EventType:      eventType,
		EntityType:     "content_action",
		EntityID:       uuidToPGLocal(entityID),
		BeforeSnapshot: json.RawMessage(`{}`),
		AfterSnapshot:  mustJSONLocal(snapshot),
	})
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
		AutomationPaused:                  false,
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
		risk := riskLevelString(action["risk_level"])
		if risk == string(autopilot.RiskHigh) {
			return "high"
		}
		if risk == string(autopilot.RiskMedium) {
			out = "medium"
		}
	}
	return out
}

func actionPortfolioDocument(selected, rejected []map[string]any, policy db.SeoPolicy, now time.Time) map[string]any {
	riskSummary := map[string]int{"low": 0, "medium": 0, "high": 0}
	requiredApprovals := []map[string]any{}
	measurementSchedule := []map[string]any{}
	for _, action := range selected {
		risk := riskLevelString(action["risk_level"])
		if _, ok := riskSummary[risk]; ok {
			riskSummary[risk]++
		}
		if required, _ := action["review_required"].(bool); required {
			requiredApprovals = append(requiredApprovals, map[string]any{
				"opportunity_id": action["opportunity_id"],
				"risk_level":     risk,
				"action_bucket":  action["action_bucket"],
			})
		}
		if schedule, ok := action["measurement_schedule"].(map[string]any); ok {
			measurementSchedule = append(measurementSchedule, schedule)
		}
	}
	return map[string]any{
		"selected_actions":     selected,
		"deferred_actions":     []map[string]any{},
		"rejected_actions":     rejected,
		"reason_codes":         map[string]any{"selection": "open_opportunity_priority"},
		"policy_snapshot":      map[string]any{"autopilot_level": policy.AutopilotLevel, "automation_paused": policy.AutomationPaused, "weekly_action_limit": policy.WeeklyActionLimit, "safe_mode_enabled": policy.SafeModeEnabled, "kill_switch_enabled": policy.KillSwitchEnabled},
		"budget_snapshot":      map[string]any{"expected_effort": len(selected)},
		"risk_summary":         riskSummary,
		"required_approvals":   requiredApprovals,
		"measurement_schedule": measurementSchedule,
		"generated_at":         now,
	}
}

func riskLevelString(value any) string {
	switch risk := value.(type) {
	case autopilot.RiskLevel:
		return string(risk)
	case string:
		return risk
	default:
		return "low"
	}
}

func actionBucketFor(actionType, opportunityType string) string {
	text := strings.ToLower(strings.TrimSpace(actionType + " " + opportunityType))
	switch {
	case strings.Contains(text, "metadata") || strings.Contains(text, "title") || strings.Contains(text, "meta"):
		return "rewrite title/meta"
	case strings.Contains(text, "internal link"):
		return "add internal links"
	case strings.Contains(text, "schema") || strings.Contains(text, "structured"):
		return "add structured data"
	case strings.Contains(text, "sitemap"):
		return "submit/update sitemap"
	case strings.Contains(text, "distribution") || strings.Contains(text, "syndication") || strings.Contains(text, "external"):
		return "distribute canonical variant"
	case strings.Contains(text, "mention") || strings.Contains(text, "monitor"):
		return "monitor external mention"
	case strings.Contains(text, "refresh") || strings.Contains(text, "decay"):
		return "refresh existing page"
	default:
		return "create new asset"
	}
}

func measurementScheduleForAction(actionBucket string) map[string]any {
	switch actionBucket {
	case "rewrite title/meta", "distribute canonical variant":
		return map[string]any{"bucket": actionBucket, "checkpoints": []int{7, 14, 28}, "primary_metric": "clicks"}
	case "add internal links":
		return map[string]any{"bucket": actionBucket, "checkpoints": []int{14, 28, 56}, "primary_metric": "clicks"}
	case "submit/update sitemap", "add structured data":
		return map[string]any{"bucket": actionBucket, "checkpoints": []int{1, 7, 14, 28}, "primary_metric": "indexability"}
	case "monitor external mention":
		return map[string]any{"bucket": actionBucket, "checkpoints": []int{7, 14, 28}, "primary_metric": "mentions"}
	default:
		return map[string]any{"bucket": actionBucket, "checkpoints": []int{14, 28, 56, 90}, "primary_metric": "clicks"}
	}
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
