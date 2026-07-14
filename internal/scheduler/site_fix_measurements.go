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

type siteFixMeasurementInvariantError struct{ err error }

func (e siteFixMeasurementInvariantError) Error() string { return e.err.Error() }
func siteFixInvariant(err error) error                   { return siteFixMeasurementInvariantError{err: err} }

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
	contract := measurement.MetricContract{Metric: metric, Direction: policy.MetricThresholds.Direction, ThresholdKind: policy.MetricThresholds.Kind, ThresholdValue: policy.MetricThresholds.Value, MinimumAfterRows: policy.MinimumSample.MinimumAfterPeriods, MinimumAfterSample: policy.MinimumSample.MinimumAfterSample, GuardrailThresholds: map[string]float64{}, ImmutableGuardrails: map[string]measurement.ImmutableMetricBaseline{}, UseExplicitGuardrails: true, UseExplicitMinimumSample: true}
	for _, guardrail := range policy.Guardrails {
		name, err := siteFixMetricName(guardrail.Metric)
		if err != nil {
			return measurement.MetricContract{}, fmt.Errorf("unsupported frozen guardrail: %w", err)
		}
		contract.GuardrailThresholds[name] = guardrail.MaxAdverseRelative
		frozen, ok := siteFixBaselineMetric(row.BaselineSnapshot, guardrail.Metric, name)
		if !row.ProspectiveObservation && !ok {
			return measurement.MetricContract{}, fmt.Errorf("frozen guardrail baseline %q is missing", guardrail.Metric)
		}
		if ok {
			contract.ImmutableGuardrails[name] = frozen
		}
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
		frozen, ok := siteFixBaselineMetric(row.BaselineSnapshot, row.PrimaryMetric, metric)
		if !ok {
			return measurement.MetricContract{}, fmt.Errorf("frozen primary baseline %q is incomplete", row.PrimaryMetric)
		}
		contract.ImmutableBaselineValue = &frozen.Value
		contract.ImmutableBaselineSampleSize = frozen.SampleSize
		contract.ImmutableBaselineRows = frozen.Rows
		contract.ImmutableBaselinePartial = frozen.Partial
		if start, end, err := siteFixBaselineWindow(row.BaselineWindow); err == nil {
			contract.ImmutableBaselineWindowDays = int(end.Sub(start).Hours()/24) + 1
		}
	}
	return contract, nil
}

func siteFixBaselineMetric(raw json.RawMessage, names ...string) (measurement.ImmutableMetricBaseline, bool) {
	var snapshot map[string]any
	if json.Unmarshal(raw, &snapshot) != nil {
		return measurement.ImmutableMetricBaseline{}, false
	}
	for _, name := range names {
		if object, ok := snapshot[name].(map[string]any); ok {
			value, valueOK := object["value"].(float64)
			sample, sampleOK := object["sample_size"].(float64)
			rows, rowsOK := object["rows"].(float64)
			partial, partialOK := object["partial"].(bool)
			if valueOK && value >= 0 && sampleOK && sample > 0 && rowsOK && rows >= 1 && rows == math.Trunc(rows) && partialOK {
				return measurement.ImmutableMetricBaseline{Value: value, SampleSize: sample, Rows: int(rows), Partial: partial}, true
			}
		}
	}
	return measurement.ImmutableMetricBaseline{}, false
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
	if event.OccurredAt.Valid {
		return event.OccurredAt.Time.UTC()
	}
	return time.Time{}
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
	return true, "The immutable absolute measurement deadline was reached while evidence remained unavailable."
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
		token := uuid.New()
		handoff, err := q.ClaimFailedTerminalSiteFixMeasurementHandoffAlert(ctx, db.ClaimFailedTerminalSiteFixMeasurementHandoffAlertParams{AlertLockToken: pgtype.UUID{Bytes: token, Valid: true}, AlertLockedUntil: pgutil.TS(now.Add(2 * time.Minute)), NowAt: pgutil.TS(now)})
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		s.alert(handoff.ProjectID, "Site Fix Results handoff exhausted finite retries: "+handoff.ID.String())
		if _, err := q.CompleteFailedTerminalSiteFixMeasurementHandoffAlert(ctx, db.CompleteFailedTerminalSiteFixMeasurementHandoffAlertParams{AlertNotifiedAt: pgutil.TS(now), ID: handoff.ID, ProjectID: handoff.ProjectID, AlertLockToken: pgtype.UUID{Bytes: token, Valid: true}}); err != nil {
			return err
		}
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
			s.logger().Error("Site Fix measurement activation failed", "project", claimed.ProjectID, "site_fix", claimed.SiteFixID, "generation", claimed.MeasurementGeneration, "err", err)
			classification := "activation_failed"
			message := "measurement activation failed; review the audited handoff"
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
	if start.IsZero() {
		return siteFixInvariant(errors.New("handoff occurrence is unavailable"))
	}
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
		params := db.GetOrCreateSiteFixMeasurementCheckpointParams{ID: uuid.New(), ProjectID: row.ProjectID, MeasurementID: row.ID, CheckpointKey: checkpoint.Key, CheckpointRole: checkpoint.Role, ScheduledAt: pgutil.TS(checkpoint.ScheduledAt), WindowStart: pgutil.TS(checkpoint.WindowStart), WindowEnd: pgutil.TS(checkpoint.WindowEnd), AttemptNumber: int32(checkpoint.Attempt), RequiredDataSources: required, DataAvailability: json.RawMessage(`{}`), MinimumSample: minimum, SeoMetrics: json.RawMessage(`{}`), Ga4Metrics: json.RawMessage(`{}`), GeoMetrics: json.RawMessage(`{}`), ExecutionMetrics: json.RawMessage(`{}`), GuardrailResults: json.RawMessage(`{}`), AttributionConfidence: "none", RetryClassification: "not_applicable", EvaluationAttemptCount: 0, NextAttemptAt: pgutil.TS(checkpoint.ScheduledAt)}
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
	checkpoints, err := q.ListSiteFixMeasurementCheckpoints(ctx, db.ListSiteFixMeasurementCheckpointsParams{ProjectID: row.ProjectID, MeasurementID: row.ID})
	if err != nil {
		return false, err
	}
	deadline := row.AbsoluteTerminalAt.Valid && !row.AbsoluteTerminalAt.Time.After(now)
	for index, checkpoint := range checkpoints {
		if checkpoint.ComputedAt.Valid || checkpoint.ScheduledAt.Time.After(now) || (!deadline && checkpoint.NextAttemptAt.Valid && checkpoint.NextAttemptAt.Time.After(now)) {
			continue
		}
		evaluation, err := s.evaluateSiteFixCheckpoint(ctx, q, row, checkpoint, now)
		if err != nil {
			s.logger().Error("Site Fix checkpoint evaluation failed", "project", row.ProjectID, "measurement", row.ID, "checkpoint", checkpoint.CheckpointKey, "err", err)
			_ = tx.Rollback(ctx)
			var invariant siteFixMeasurementInvariantError
			if deadline || errors.As(err, &invariant) {
				return true, s.terminalizeSiteFixEvaluationFailure(ctx, row, now, deadline)
			}
			return true, s.deferSiteFixEvaluation(ctx, checkpoint, now)
		}
		outcome, reason := evaluation.OutcomeLabel, evaluation.OutcomeReason
		confidence := evaluation.AttributionConfidence
		if checkpoint.CheckpointRole == "early_signal" {
			confidence = "low"
		}
		if row.ProspectiveObservation {
			outcome = measurement.OutcomeInsufficientData
			reason = "Prospective observation has no pre-change baseline; v1 records quality only."
			confidence = "low"
		}
		execution := mustJSON(map[string]any{"confounders": evaluation.Confounders, "data_quality_state": evaluation.DataQualityState})
		canonical, err := q.CompleteSiteFixMeasurementCheckpoint(ctx, db.CompleteSiteFixMeasurementCheckpointParams{DataAvailability: evaluation.SourceFreshness, SeoMetrics: evaluation.SEOMetrics, Ga4Metrics: evaluation.GA4Metrics, GeoMetrics: evaluation.GEOMetrics, ExecutionMetrics: execution, GuardrailResults: mustJSON(evaluation.GuardrailResults), OutcomeLabel: &outcome, OutcomeReason: &reason, AttributionConfidence: confidence, ComputedAt: pgutil.TS(now), RetryClassification: "not_applicable", ProjectID: row.ProjectID, MeasurementID: row.ID, CheckpointKey: checkpoint.CheckpointKey, AttemptNumber: checkpoint.AttemptNumber})
		if err != nil {
			return false, err
		}
		outcome, reason, confidence = valueOrEmpty(canonical.OutcomeLabel), valueOrEmpty(canonical.OutcomeReason), canonical.AttributionConfidence
		confounders := siteFixCheckpointConfounders(canonical.ExecutionMetrics)
		exhausted := deadline || !hasLaterSiteFixCheckpoint(checkpoints, index)
		terminal, terminalOutcome := siteFixTerminalDecision(checkpoint.CheckpointRole, outcome, row.ProspectiveObservation, exhausted)
		if terminal {
			terminalReason := reason
			if row.ProspectiveObservation {
				terminalReason = "Prospective observation has no pre-change baseline; v1 cannot make directional attribution."
			}
			if err := terminalizeSiteFixMeasurement(ctx, q, row, now, terminalOutcome, terminalReason, confidence, confounders); err != nil {
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
			if err := terminalizeSiteFixMeasurement(ctx, q, row, now, measurement.OutcomeInsufficientData, "The immutable absolute measurement deadline was reached without complete comparable evidence.", "low", []string{"absolute_deadline_reached"}); err != nil {
				return false, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func siteFixCheckpointConfounders(raw json.RawMessage) []string {
	var payload struct {
		Confounders []string `json:"confounders"`
	}
	_ = json.Unmarshal(raw, &payload)
	return payload.Confounders
}

func (s *Scheduler) deferSiteFixEvaluation(ctx context.Context, checkpoint db.SiteFixMeasurementCheckpoint, now time.Time) error {
	next := siteFixHandoffRetryAt(now, checkpoint.EvaluationAttemptCount+1)
	reason := "measurement_evidence_temporarily_unavailable"
	_, err := db.New(s.Pool).DeferSiteFixMeasurementCheckpoint(ctx, db.DeferSiteFixMeasurementCheckpointParams{NextAttemptAt: pgutil.TS(next), FailureReason: &reason, ProjectID: checkpoint.ProjectID, MeasurementID: checkpoint.MeasurementID, CheckpointKey: checkpoint.CheckpointKey, AttemptNumber: checkpoint.AttemptNumber})
	return err
}

func (s *Scheduler) terminalizeSiteFixEvaluationFailure(ctx context.Context, original db.SiteFixMeasurement, now time.Time, deadline bool) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := db.New(tx)
	row, err := q.GetSiteFixMeasurement(ctx, db.GetSiteFixMeasurementParams{ProjectID: original.ProjectID, ID: original.ID})
	if err != nil {
		return err
	}
	reason, gap := "The frozen measurement contract is invalid; directional attribution is unavailable.", "measurement_contract_invalid"
	if deadline {
		reason, gap = "The immutable measurement deadline was reached before comparable evidence became available.", "evidence_unavailable_at_absolute_deadline"
	}
	if err := terminalizeSiteFixMeasurement(ctx, q, row, now, measurement.OutcomeInsufficientData, reason, "low", []string{gap}); err != nil {
		return err
	}
	return tx.Commit(ctx)
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
	if s.siteFixEvidenceOverride != nil {
		return s.siteFixEvidenceOverride(ctx, q, row, checkpoint, now)
	}
	contract, err := siteFixMetricContract(row)
	if err != nil {
		return measurement.EvidenceEvaluation{}, siteFixInvariant(err)
	}
	baselineStart, baselineEnd, err := siteFixBaselineWindow(row.BaselineWindow)
	if err != nil && !row.ProspectiveObservation {
		return measurement.EvidenceEvaluation{}, siteFixInvariant(err)
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
		return measurement.EvidenceEvaluation{}, siteFixInvariant(err)
	}
	query := ""
	if row.TargetQuery != nil {
		query = strings.TrimSpace(*row.TargetQuery)
	}
	evidence, err := q.GetGrowthMeasurementEvidence(ctx, db.GetGrowthMeasurementEvidenceParams{ProjectID: row.ProjectID, TargetUrl: row.NormalizedTargetUrl, Query: query, GeoEvidenceIds: evidenceIDs, BaselineStart: pgtype.Date{Time: baselineStart, Valid: true}, BaselineEnd: pgtype.Date{Time: baselineEnd, Valid: true}, AfterStart: pgtype.Date{Time: row.StartedAt.Time.UTC(), Valid: true}, AfterEnd: pgtype.Date{Time: afterEnd, Valid: true}})
	if err != nil {
		return measurement.EvidenceEvaluation{}, err
	}
	evaluation, err := measurement.EvaluateSourceEvidence(contract, evidence, now)
	if err != nil {
		return measurement.EvidenceEvaluation{}, siteFixInvariant(err)
	}
	return evaluation, nil
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

func terminalizeSiteFixMeasurement(ctx context.Context, q *db.Queries, row db.SiteFixMeasurement, now time.Time, outcome, reason, confidence string, confounders []string) error {
	if outcome == measurement.OutcomeInsufficientData {
		confidence = "low"
	}
	if _, err := q.CloseSiteFixMeasurementOpenCheckpoints(ctx, db.CloseSiteFixMeasurementOpenCheckpointsParams{ComputedAt: pgutil.TS(now), ProjectID: row.ProjectID, MeasurementID: row.ID}); err != nil {
		return err
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
	checkpoints, err := q.ListSiteFixMeasurementCheckpoints(ctx, db.ListSiteFixMeasurementCheckpointsParams{ProjectID: row.ProjectID, MeasurementID: row.ID})
	if err != nil {
		return err
	}
	outcomeRecord, err := q.GetOrCreateSiteFixMeasurementTerminalOutcome(ctx, db.GetOrCreateSiteFixMeasurementTerminalOutcomeParams{ID: uuid.New(), ProjectID: row.ProjectID, MeasurementID: row.ID, OutcomeLabel: outcome, RecordKind: recordKind, TerminalReason: reason, MeasurementPolicyVersion: row.MeasurementPolicyVersion, BaselineSnapshot: row.BaselineSnapshot, CheckpointSnapshot: mustJSON(map[string]any{"checkpoints": checkpoints}), OutcomeSnapshot: mustJSON(updated)})
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
