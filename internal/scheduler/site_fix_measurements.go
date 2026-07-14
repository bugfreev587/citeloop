package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/measurement"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const siteFixMeasurementBatchSize = 50

type siteFixScheduledCheckpoint struct {
	Key, Role              string
	ScheduledAt            time.Time
	WindowStart, WindowEnd time.Time
	Attempt                int
}

type siteFixFrozenPolicy struct {
	MetricThresholds struct {
		Direction string  `json:"direction"`
		Kind      string  `json:"kind"`
		Value     float64 `json:"value"`
	} `json:"metric_thresholds"`
	MinimumSample struct {
		MinimumAfterPeriods int     `json:"minimum_after_periods"`
		MinimumAfterSample  float64 `json:"minimum_after_sample"`
	} `json:"minimum_sample"`
	Guardrails []struct {
		Metric             string  `json:"metric"`
		MaxAdverseRelative float64 `json:"max_adverse_relative"`
	} `json:"guardrails"`
	RequiredDataSources []string `json:"required_data_sources"`
}

func siteFixCheckpointSchedule(row db.SiteFixMeasurement, policy measurement.Policy) ([]siteFixScheduledCheckpoint, error) {
	if !row.StartedAt.Valid {
		return nil, fmt.Errorf("Site Fix measurement %s is not activated", row.ID)
	}
	start := row.StartedAt.Time.UTC()
	baselineStart, baselineEnd, err := siteFixBaselineWindow(row.BaselineWindow)
	if err != nil && !row.ProspectiveObservation {
		return nil, err
	}
	if row.ProspectiveObservation {
		baselineStart, baselineEnd = start, start
	}
	out := []siteFixScheduledCheckpoint{{Key: "baseline", Role: "baseline", ScheduledAt: start, WindowStart: baselineStart, WindowEnd: baselineEnd, Attempt: 1}}
	for _, checkpoint := range policy.Checkpoints() {
		if checkpoint.Role == measurement.RoleBaseline {
			continue
		}
		role := string(checkpoint.Role)
		if checkpoint.Role == measurement.RoleEarly {
			role = "early_signal"
		}
		due := start.AddDate(0, 0, checkpoint.Day)
		out = append(out, siteFixScheduledCheckpoint{
			Key: fmt.Sprintf("%s:%d:%d", role, checkpoint.Day, checkpoint.Attempt), Role: role,
			ScheduledAt: due, WindowStart: start, WindowEnd: due, Attempt: checkpoint.Attempt,
		})
	}
	return out, nil
}

func siteFixBaselineWindow(raw json.RawMessage) (time.Time, time.Time, error) {
	var window struct{ Start, End string }
	if err := json.Unmarshal(raw, &window); err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid frozen baseline window: %w", err)
	}
	start, err := time.Parse(time.RFC3339, window.Start)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid baseline start: %w", err)
	}
	end, err := time.Parse(time.RFC3339, window.End)
	if err != nil || end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid baseline end")
	}
	return start.UTC(), end.UTC(), nil
}

func siteFixMetricName(metric string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "ctr", "gsc_ctr":
		return "gsc_ctr", nil
	case "clicks", "gsc_clicks":
		return "gsc_clicks", nil
	case "impressions", "gsc_impressions":
		return "gsc_impressions", nil
	case "position", "gsc_position":
		return "gsc_position", nil
	case "citations", "ai_citation_count":
		return "ai_citation_count", nil
	case "brand_mentions", "ai_brand_mention_rate":
		return "ai_brand_mention_rate", nil
	case "conversion_rate", "ga4_conversion_rate":
		return "ga4_conversion_rate", nil
	case "qualified_actions", "ga4_key_events":
		return "ga4_key_events", nil
	case "referral_sessions", "ga4_sessions":
		return "ga4_sessions", nil
	default:
		return "", fmt.Errorf("unsupported Site Fix measurement metric %q", metric)
	}
}

func siteFixMetricContract(row db.SiteFixMeasurement) (measurement.MetricContract, error) {
	metric, err := siteFixMetricName(row.PrimaryMetric)
	if err != nil {
		return measurement.MetricContract{}, err
	}
	var policy siteFixFrozenPolicy
	if err := json.Unmarshal(row.MeasurementPolicySnapshot, &policy); err != nil {
		return measurement.MetricContract{}, err
	}
	if policy.MetricThresholds.Direction != "increase" && policy.MetricThresholds.Direction != "decrease" {
		return measurement.MetricContract{}, fmt.Errorf("invalid frozen direction")
	}
	if policy.MetricThresholds.Kind != "absolute" && policy.MetricThresholds.Kind != "relative" {
		return measurement.MetricContract{}, fmt.Errorf("invalid frozen threshold kind")
	}
	contract := measurement.MetricContract{Metric: metric, Direction: policy.MetricThresholds.Direction, ThresholdKind: policy.MetricThresholds.Kind, ThresholdValue: policy.MetricThresholds.Value, MinimumAfterRows: policy.MinimumSample.MinimumAfterPeriods, MinimumAfterSample: policy.MinimumSample.MinimumAfterSample, GuardrailThresholds: map[string]float64{}, UseExplicitGuardrails: true, UseExplicitMinimumSample: true}
	for _, guardrail := range policy.Guardrails {
		name, err := siteFixMetricName(guardrail.Metric)
		if err != nil {
			return measurement.MetricContract{}, fmt.Errorf("unsupported frozen guardrail: %w", err)
		}
		contract.GuardrailThresholds[name] = guardrail.MaxAdverseRelative
	}
	if len(contract.GuardrailThresholds) > 0 {
		allowed := map[string]string{"gsc_ctr": "gsc_impressions", "gsc_clicks": "gsc_impressions", "gsc_position": "gsc_impressions", "ga4_conversion_rate": "ga4_sessions", "ga4_key_events": "ga4_sessions", "ai_citation_count": "ai_brand_mention_rate"}[metric]
		for name := range contract.GuardrailThresholds {
			if allowed == "" || name != allowed {
				return measurement.MetricContract{}, fmt.Errorf("unsupported frozen guardrail %q for %q", name, metric)
			}
		}
	}
	if !row.ProspectiveObservation {
		if value, sample, ok := siteFixBaselineValue(row.BaselineSnapshot, row.PrimaryMetric, metric); ok {
			contract.ImmutableBaselineValue = &value
			contract.ImmutableBaselineSampleSize = sample
		}
		if start, end, err := siteFixBaselineWindow(row.BaselineWindow); err == nil {
			contract.ImmutableBaselineWindowDays = int(end.Sub(start).Hours()/24) + 1
		}
	}
	return contract, nil
}

func siteFixBaselineValue(raw json.RawMessage, names ...string) (float64, float64, bool) {
	var snapshot map[string]any
	if json.Unmarshal(raw, &snapshot) != nil {
		return 0, 0, false
	}
	var value float64
	found := false
	for _, name := range names {
		if candidate, ok := snapshot[name].(float64); ok {
			value, found = candidate, true
			break
		}
		if object, ok := snapshot[name].(map[string]any); ok {
			if candidate, ok := object["value"].(float64); ok {
				value, found = candidate, true
				break
			}
		}
	}
	sample, _ := snapshot["sample_size"].(float64)
	return value, sample, found
}

func siteFixHandoffRetryAt(base time.Time, attempt int32) time.Time {
	if attempt < 1 {
		attempt = 1
	}
	minutes := math.Pow(2, float64(attempt-1))
	if minutes > 60 {
		minutes = 60
	}
	return base.UTC().Add(time.Duration(minutes) * time.Minute)
}

func siteFixHandoffStartedAt(event db.SiteFixMeasurementHandoffOutbox) time.Time {
	start := event.CreatedAt.Time.UTC()
	if event.NextAttemptAt.Valid && (!event.CreatedAt.Valid || !event.NextAttemptAt.Time.After(event.CreatedAt.Time)) {
		start = event.NextAttemptAt.Time.UTC()
	}
	return start
}

func siteFixTerminalDecision(role, outcome string, prospective, exhausted bool) (bool, string) {
	if role == "baseline" || role == "early_signal" {
		return false, ""
	}
	if prospective {
		return true, measurement.OutcomeInsufficientData
	}
	if outcome != measurement.OutcomeInsufficientData {
		return true, outcome
	}
	if exhausted {
		return true, measurement.OutcomeInsufficientData
	}
	return false, ""
}

func siteFixDeadlineEvidenceFailure(deadline bool, err error) (bool, string) {
	if err == nil || !deadline {
		return false, ""
	}
	return true, "The immutable absolute measurement deadline was reached while evidence remained unavailable: " + err.Error()
}

// TickSiteFixMeasurements advances the independent Results-owned aggregate.
// Site Fix lifecycle rows are intentionally never mutated here.
func (s *Scheduler) TickSiteFixMeasurements(ctx context.Context) error {
	now := s.currentTime().UTC()
	q := db.New(s.Pool)
	if _, err := q.TerminalizeExpiredSiteFixMeasurementHandoffs(ctx, pgutil.TS(now)); err != nil {
		return err
	}
	if _, err := q.ReconcileVerifiedSiteFixMeasurementHandoffs(ctx, db.ReconcileVerifiedSiteFixMeasurementHandoffsParams{NowAt: pgutil.TS(now), LimitRows: siteFixMeasurementBatchSize}); err != nil {
		return err
	}
	missing, err := q.ListVerifiedRequiredSiteFixesMissingMeasurement(ctx, siteFixMeasurementBatchSize)
	if err != nil {
		return err
	}
	for _, fix := range missing {
		s.alert(fix.ProjectID, "Verified measurement-required Site Fix has no persisted measurement: "+fix.ID.String())
	}
	for i := 0; i < siteFixMeasurementBatchSize; i++ {
		claimed, err := q.ClaimSiteFixMeasurementHandoff(ctx, db.ClaimSiteFixMeasurementHandoffParams{LockToken: pgtype.UUID{Bytes: uuid.New(), Valid: true}, LockedUntil: pgutil.TS(now.Add(2 * time.Minute)), NowAt: pgutil.TS(now)})
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		if err := s.activateSiteFixMeasurement(ctx, claimed); err != nil {
			classification := "activation_failed"
			message := err.Error()
			_, retryErr := q.RetrySiteFixMeasurementHandoff(ctx, db.RetrySiteFixMeasurementHandoffParams{NextAttemptAt: pgutil.TS(siteFixHandoffRetryAt(now, claimed.AttemptCount)), LastErrorClassification: &classification, LastError: &message, ID: claimed.ID, ProjectID: claimed.ProjectID, LockToken: claimed.LockToken})
			if retryErr != nil {
				return fmt.Errorf("activate: %v; retry: %w", err, retryErr)
			}
		}
	}
	for i := 0; i < siteFixMeasurementBatchSize; i++ {
		processed, err := s.processDueSiteFixMeasurement(ctx, now)
		if err != nil {
			return err
		}
		if !processed {
			break
		}
	}
	return nil
}

func (s *Scheduler) activateSiteFixMeasurement(ctx context.Context, event db.SiteFixMeasurementHandoffOutbox) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock($1)", lockKey(event.ProjectID)); err != nil {
		return err
	}
	q := db.New(tx)
	start := siteFixHandoffStartedAt(event)
	existing, err := q.GetSiteFixMeasurementGeneration(ctx, db.GetSiteFixMeasurementGenerationParams{ProjectID: event.ProjectID, SiteFixID: event.SiteFixID, MeasurementGeneration: event.MeasurementGeneration})
	if err != nil {
		return pgx.ErrNoRows
	}
	deepLink := fmt.Sprintf("/projects/%s/seo?result=site_fix:%s", event.ProjectID, existing.ID)
	row, err := q.ActivateSiteFixMeasurement(ctx, db.ActivateSiteFixMeasurementParams{StartedAt: pgutil.TS(start), ResultsDeepLink: &deepLink, ProjectID: event.ProjectID, SiteFixID: event.SiteFixID, MeasurementGeneration: event.MeasurementGeneration})
	if errors.Is(err, pgx.ErrNoRows) {
		row, err = q.GetSiteFixMeasurementGeneration(ctx, db.GetSiteFixMeasurementGenerationParams{ProjectID: event.ProjectID, SiteFixID: event.SiteFixID, MeasurementGeneration: event.MeasurementGeneration})
		if err == nil && (row.MeasurementGeneration != event.MeasurementGeneration || row.Status == "terminal") {
			err = pgx.ErrNoRows
		}
	}
	if err != nil {
		return err
	}
	policy, err := measurement.Parse(row.MeasurementPolicySnapshot)
	if err != nil {
		return err
	}
	schedule, err := siteFixCheckpointSchedule(row, policy)
	if err != nil {
		return err
	}
	var frozen siteFixFrozenPolicy
	if err := json.Unmarshal(row.MeasurementPolicySnapshot, &frozen); err != nil {
		return err
	}
	required := mustJSON(frozen.RequiredDataSources)
	minimum := mustJSON(frozen.MinimumSample)
	for _, checkpoint := range schedule {
		params := db.GetOrCreateSiteFixMeasurementCheckpointParams{ID: uuid.New(), ProjectID: row.ProjectID, MeasurementID: row.ID, CheckpointKey: checkpoint.Key, CheckpointRole: checkpoint.Role, ScheduledAt: pgutil.TS(checkpoint.ScheduledAt), WindowStart: pgutil.TS(checkpoint.WindowStart), WindowEnd: pgutil.TS(checkpoint.WindowEnd), AttemptNumber: int32(checkpoint.Attempt), RequiredDataSources: required, DataAvailability: json.RawMessage(`{}`), MinimumSample: minimum, SeoMetrics: json.RawMessage(`{}`), Ga4Metrics: json.RawMessage(`{}`), GeoMetrics: json.RawMessage(`{}`), ExecutionMetrics: json.RawMessage(`{}`), GuardrailResults: json.RawMessage(`{}`), AttributionConfidence: "none", RetryClassification: "not_applicable"}
		if checkpoint.Role == "baseline" {
			reason := "Frozen baseline snapshot is informational; directional attribution starts at the primary checkpoint."
			outcome := measurement.OutcomeInsufficientData
			params.OutcomeLabel, params.OutcomeReason = &outcome, &reason
			params.AttributionConfidence = row.AttributionConfidence
			params.ComputedAt = pgutil.TS(start)
			params.ExecutionMetrics = row.BaselineSnapshot
		}
		if _, err := q.GetOrCreateSiteFixMeasurementCheckpoint(ctx, params); err != nil {
			return err
		}
	}
	if _, err := q.CompleteSiteFixMeasurementHandoff(ctx, db.CompleteSiteFixMeasurementHandoffParams{ID: event.ID, ProjectID: event.ProjectID, LockToken: event.LockToken}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Scheduler) processDueSiteFixMeasurement(ctx context.Context, now time.Time) (bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	row, err := q.ClaimDueSiteFixMeasurement(ctx, pgutil.TS(now))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock($1)", lockKey(row.ProjectID)); err != nil {
		return false, err
	}
	checkpoints, err := q.ListSiteFixMeasurementCheckpoints(ctx, db.ListSiteFixMeasurementCheckpointsParams{ProjectID: row.ProjectID, MeasurementID: row.ID})
	if err != nil {
		return false, err
	}
	deadline := row.AbsoluteTerminalAt.Valid && !row.AbsoluteTerminalAt.Time.After(now)
	for index, checkpoint := range checkpoints {
		if checkpoint.ComputedAt.Valid || checkpoint.ScheduledAt.Time.After(now) {
			continue
		}
		evaluation, err := s.evaluateSiteFixCheckpoint(ctx, q, row, checkpoint, now)
		if err != nil {
			terminalFailure, reason := siteFixDeadlineEvidenceFailure(deadline, err)
			if !terminalFailure {
				return false, err
			}
			outcome := measurement.OutcomeInsufficientData
			if _, completeErr := q.CompleteSiteFixMeasurementCheckpoint(ctx, db.CompleteSiteFixMeasurementCheckpointParams{DataAvailability: json.RawMessage(`{"state":"unavailable_at_deadline"}`), SeoMetrics: json.RawMessage(`{}`), Ga4Metrics: json.RawMessage(`{}`), GeoMetrics: json.RawMessage(`{}`), ExecutionMetrics: json.RawMessage(`{}`), GuardrailResults: json.RawMessage(`{}`), OutcomeLabel: &outcome, OutcomeReason: &reason, AttributionConfidence: "low", ComputedAt: pgutil.TS(now), FailureReason: &reason, RetryClassification: "retry_exhausted", ProjectID: row.ProjectID, MeasurementID: row.ID, CheckpointKey: checkpoint.CheckpointKey, AttemptNumber: checkpoint.AttemptNumber}); completeErr != nil {
				return false, completeErr
			}
			if terminalErr := terminalizeSiteFixMeasurement(ctx, q, row, checkpoints, outcome, reason, "low", []string{"evidence_unavailable_at_absolute_deadline"}); terminalErr != nil {
				return false, terminalErr
			}
			break
		}
		outcome, reason := evaluation.OutcomeLabel, evaluation.OutcomeReason
		confidence := evaluation.AttributionConfidence
		if checkpoint.CheckpointRole == "early_signal" {
			confidence = "low"
		}
		if row.ProspectiveObservation {
			confidence = "low"
		}
		if _, err := q.CompleteSiteFixMeasurementCheckpoint(ctx, db.CompleteSiteFixMeasurementCheckpointParams{DataAvailability: evaluation.SourceFreshness, SeoMetrics: evaluation.SEOMetrics, Ga4Metrics: evaluation.GA4Metrics, GeoMetrics: evaluation.GEOMetrics, ExecutionMetrics: json.RawMessage(`{}`), GuardrailResults: mustJSON(evaluation.GuardrailResults), OutcomeLabel: &outcome, OutcomeReason: &reason, AttributionConfidence: confidence, ComputedAt: pgutil.TS(now), RetryClassification: "not_applicable", ProjectID: row.ProjectID, MeasurementID: row.ID, CheckpointKey: checkpoint.CheckpointKey, AttemptNumber: checkpoint.AttemptNumber}); err != nil {
			return false, err
		}
		exhausted := deadline || !hasLaterSiteFixCheckpoint(checkpoints, index)
		terminal, terminalOutcome := siteFixTerminalDecision(checkpoint.CheckpointRole, outcome, row.ProspectiveObservation, exhausted)
		if terminal {
			terminalReason := reason
			if row.ProspectiveObservation {
				terminalReason = "Prospective observation has no pre-change baseline; v1 cannot make directional attribution."
			}
			if err := terminalizeSiteFixMeasurement(ctx, q, row, checkpoints, terminalOutcome, terminalReason, confidence, evaluation.Confounders); err != nil {
				return false, err
			}
			break
		}
	}
	if deadline {
		latest, err := q.GetSiteFixMeasurement(ctx, db.GetSiteFixMeasurementParams{ProjectID: row.ProjectID, ID: row.ID})
		if err != nil {
			return false, err
		}
		if latest.Status != "terminal" {
			if err := terminalizeSiteFixMeasurement(ctx, q, row, checkpoints, measurement.OutcomeInsufficientData, "The immutable absolute measurement deadline was reached without complete comparable evidence.", "low", []string{"absolute_deadline_reached"}); err != nil {
				return false, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func hasLaterSiteFixCheckpoint(checkpoints []db.SiteFixMeasurementCheckpoint, index int) bool {
	for i := index + 1; i < len(checkpoints); i++ {
		if !checkpoints[i].ComputedAt.Valid {
			return true
		}
	}
	return false
}

func (s *Scheduler) evaluateSiteFixCheckpoint(ctx context.Context, q *db.Queries, row db.SiteFixMeasurement, checkpoint db.SiteFixMeasurementCheckpoint, now time.Time) (measurement.EvidenceEvaluation, error) {
	contract, err := siteFixMetricContract(row)
	if err != nil {
		return measurement.EvidenceEvaluation{}, err
	}
	baselineStart, baselineEnd, err := siteFixBaselineWindow(row.BaselineWindow)
	if err != nil && !row.ProspectiveObservation {
		return measurement.EvidenceEvaluation{}, err
	}
	if row.ProspectiveObservation {
		baselineStart, baselineEnd = row.StartedAt.Time.UTC(), row.StartedAt.Time.UTC()
	}
	afterEnd := checkpoint.WindowEnd.Time.UTC()
	if afterEnd.After(now) {
		afterEnd = now
	}
	evidenceIDs, err := siteFixGEOEvidenceIDs(row.TargetIdentity)
	if err != nil {
		return measurement.EvidenceEvaluation{}, err
	}
	query := ""
	if row.TargetQuery != nil {
		query = strings.TrimSpace(*row.TargetQuery)
	}
	evidence, err := q.GetGrowthMeasurementEvidence(ctx, db.GetGrowthMeasurementEvidenceParams{ProjectID: row.ProjectID, TargetUrl: row.NormalizedTargetUrl, Query: query, GeoEvidenceIds: evidenceIDs, BaselineStart: pgtype.Date{Time: baselineStart, Valid: true}, BaselineEnd: pgtype.Date{Time: baselineEnd, Valid: true}, AfterStart: pgtype.Date{Time: row.StartedAt.Time.UTC(), Valid: true}, AfterEnd: pgtype.Date{Time: afterEnd, Valid: true}})
	if err != nil {
		return measurement.EvidenceEvaluation{}, err
	}
	return measurement.EvaluateSourceEvidence(contract, evidence, now)
}

func siteFixGEOEvidenceIDs(raw json.RawMessage) ([]string, error) {
	var identity struct {
		GEOEvidenceIDs []string `json:"geo_evidence_ids"`
		EvidenceIDs    []string `json:"evidence_ids"`
	}
	if len(raw) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(raw, &identity); err != nil {
		return nil, err
	}
	ids := append(identity.GEOEvidenceIDs, identity.EvidenceIDs...)
	seen := map[uuid.UUID]bool{}
	out := []string{}
	for _, value := range ids {
		id, err := uuid.Parse(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid GEO evidence id %q", value)
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id.String())
		}
	}
	return out, nil
}

func terminalizeSiteFixMeasurement(ctx context.Context, q *db.Queries, row db.SiteFixMeasurement, checkpoints []db.SiteFixMeasurementCheckpoint, outcome, reason, confidence string, confounders []string) error {
	if outcome == measurement.OutcomeInsufficientData {
		confidence = "low"
	}
	updated, err := q.TerminalizeSiteFixMeasurement(ctx, db.TerminalizeSiteFixMeasurementParams{TerminalOutcome: &outcome, OutcomeReason: &reason, AttributionConfidence: confidence, Confounders: mustJSON(confounders), ProjectID: row.ProjectID, ID: row.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	recordKind := "directional_learning"
	if outcome == measurement.OutcomeInsufficientData {
		recordKind = "measurement_quality"
	}
	checkpoints, err = q.ListSiteFixMeasurementCheckpoints(ctx, db.ListSiteFixMeasurementCheckpointsParams{ProjectID: row.ProjectID, MeasurementID: row.ID})
	if err != nil {
		return err
	}
	outcomeRecord, err := q.GetOrCreateSiteFixMeasurementTerminalOutcome(ctx, db.GetOrCreateSiteFixMeasurementTerminalOutcomeParams{ID: uuid.New(), ProjectID: row.ProjectID, MeasurementID: row.ID, OutcomeLabel: outcome, RecordKind: recordKind, TerminalReason: reason, MeasurementPolicyVersion: row.MeasurementPolicyVersion, BaselineSnapshot: row.BaselineSnapshot, CheckpointSnapshot: mustJSON(checkpoints), OutcomeSnapshot: mustJSON(updated)})
	if err != nil {
		return err
	}
	if outcome == measurement.OutcomeInsufficientData {
		_, err = q.GetOrCreateSiteFixMeasurementQualityRecord(ctx, db.GetOrCreateSiteFixMeasurementQualityRecordParams{ID: uuid.New(), ProjectID: row.ProjectID, TerminalOutcomeID: outcomeRecord.ID, MeasurementID: row.ID, DataQualityState: "insufficient", QualityGaps: mustJSON(confounders), Recommendation: "Review provider availability and start a new audited generation if another observation is warranted.", QualityVersion: "site-fix-measurement-quality-v1"})
		return err
	}
	_, err = q.GetOrCreateSiteFixMeasurementLearning(ctx, db.GetOrCreateSiteFixMeasurementLearningParams{ID: uuid.New(), ProjectID: row.ProjectID, TerminalOutcomeID: outcomeRecord.ID, MeasurementID: row.ID, LearningSummary: reason, Applicability: mustJSON(map[string]any{"fix_type": row.FixType, "impact_mode": row.ImpactMode, "primary_metric": row.PrimaryMetric}), LearningVersion: "site-fix-learning-v1"})
	return err
}
