package learning

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

const (
	ScoringVersionV1       = "growth-learning-score-v1"
	maxSingleAdjustment    = 3.0
	maxAggregateAdjustment = 8.0
	maxApplicableLearnings = 5
)

type ScoringStore interface {
	ListApplicableGrowthLearnings(context.Context, db.ListApplicableGrowthLearningsParams) ([]db.ListApplicableGrowthLearningsRow, error)
}

type CandidateScorer interface {
	ScoreCandidate(context.Context, CandidateScoreInput) (ScoreResult, error)
}

type CandidateScoreInput struct {
	BaseScore      float64
	ActionFamily   string
	TargetIdentity map[string]any
	Audience       []string
	PrimaryMetric  string
}

type LearningApplication struct {
	LearningID        uuid.UUID `json:"learning_id"`
	OutcomeLabel      string    `json:"outcome_label"`
	RawAdjustment     float64   `json:"raw_adjustment"`
	Adjustment        float64   `json:"adjustment"`
	MatchedDimensions []string  `json:"matched_dimensions"`
}

type ScoreResult struct {
	Version       string
	BaseScore     float64
	RawAdjustment float64
	Adjustment    float64
	AdjustedScore float64
	LearningIDs   []uuid.UUID
	Applications  []LearningApplication
}

type Scorer struct {
	learnings []db.ListGrowthLearningsRow
}

type ProjectScorer struct {
	store     ScoringStore
	projectID uuid.UUID
}

func NewProjectScorer(store ScoringStore, projectID uuid.UUID) *ProjectScorer {
	return &ProjectScorer{store: store, projectID: projectID}
}

func NewScorer(rows []db.ListGrowthLearningsRow) Scorer {
	eligible := make([]db.ListGrowthLearningsRow, 0, len(rows))
	for _, row := range rows {
		if row.ScoringEligible {
			eligible = append(eligible, row)
		}
	}
	return Scorer{learnings: eligible}
}

func (s Scorer) ScoreCandidate(_ context.Context, input CandidateScoreInput) (ScoreResult, error) {
	return s.Score(input), nil
}

func (s *ProjectScorer) ScoreCandidate(ctx context.Context, input CandidateScoreInput) (ScoreResult, error) {
	target, err := json.Marshal(input.TargetIdentity)
	if err != nil {
		return ScoreResult{}, err
	}
	audience, err := json.Marshal(input.Audience)
	if err != nil {
		return ScoreResult{}, err
	}
	if len(input.TargetIdentity) == 0 || len(input.Audience) == 0 {
		return NewScorer(nil).Score(input), nil
	}
	rows, err := s.store.ListApplicableGrowthLearnings(ctx, db.ListApplicableGrowthLearningsParams{
		ProjectID: s.projectID, ActionFamily: input.ActionFamily, PrimaryMetric: input.PrimaryMetric,
		TargetIdentity: target, Audience: audience, LimitRows: maxApplicableLearnings,
	})
	if err != nil {
		return ScoreResult{}, err
	}
	learnings := make([]db.ListGrowthLearningsRow, 0, len(rows))
	for _, row := range rows {
		learnings = append(learnings, db.ListGrowthLearningsRow{
			ID: row.ID, ScoringEligible: row.ScoringEligible, ActionFamily: row.ActionFamily,
			TargetIdentity: row.TargetIdentity, Audience: row.Audience,
			PrimaryMetric: row.PrimaryMetric, OutcomeLabel: row.OutcomeLabel,
		})
	}
	return NewScorer(learnings).Score(input), nil
}

func (s Scorer) Score(input CandidateScoreInput) ScoreResult {
	base := roundScore(clamp(input.BaseScore, 0, 100))
	result := ScoreResult{
		Version: ScoringVersionV1, BaseScore: base, AdjustedScore: base,
		LearningIDs: []uuid.UUID{}, Applications: []LearningApplication{},
	}
	if normalizeDimension(input.ActionFamily) == "" || normalizeDimension(input.PrimaryMetric) == "" {
		return result
	}

	for _, row := range s.learnings {
		if len(result.Applications) >= maxApplicableLearnings {
			break
		}
		matched, dimensions := learningApplies(input, row)
		if !matched {
			continue
		}
		raw := boundedOutcomeAdjustment(row.OutcomeLabel)
		result.RawAdjustment += raw
		result.LearningIDs = append(result.LearningIDs, row.ID)
		result.Applications = append(result.Applications, LearningApplication{
			LearningID: row.ID, OutcomeLabel: row.OutcomeLabel,
			RawAdjustment: raw, MatchedDimensions: dimensions,
		})
	}
	result.RawAdjustment = roundScore(result.RawAdjustment)
	result.Adjustment = roundScore(clamp(result.RawAdjustment, -maxAggregateAdjustment, maxAggregateAdjustment))
	assignEffectiveAdjustments(result.Applications, result.RawAdjustment, result.Adjustment)
	result.AdjustedScore = roundScore(clamp(base+result.Adjustment, 0, 100))
	return result
}

func (r ScoreResult) Provenance() map[string]any {
	return map[string]any{
		"version": r.Version, "base_score": r.BaseScore, "raw_adjustment": r.RawAdjustment,
		"adjustment": r.Adjustment, "aggregate_cap": maxAggregateAdjustment,
		"adjusted_score": r.AdjustedScore, "learning_ids": r.LearningIDs,
		"applications": r.Applications,
	}
}

func assignEffectiveAdjustments(applications []LearningApplication, rawTotal, boundedTotal float64) {
	if len(applications) == 0 {
		return
	}
	if rawTotal == 0 {
		for i := range applications {
			applications[i].Adjustment = applications[i].RawAdjustment
		}
		return
	}
	scale := 1.0
	if rawTotal != boundedTotal {
		scale = boundedTotal / rawTotal
	}
	effectiveTotal := 0.0
	for i := range applications {
		applications[i].Adjustment = roundScore(applications[i].RawAdjustment * scale)
		effectiveTotal += applications[i].Adjustment
	}
	residual := roundScore(boundedTotal - effectiveTotal)
	if residual == 0 {
		return
	}
	index := deterministicResidualIndex(applications)
	if index >= 0 {
		applications[index].Adjustment = roundScore(applications[index].Adjustment + residual)
	}
}

func deterministicResidualIndex(applications []LearningApplication) int {
	index := -1
	for i := range applications {
		if applications[i].RawAdjustment == 0 {
			continue
		}
		if index < 0 || applications[i].LearningID.String() < applications[index].LearningID.String() {
			index = i
		}
	}
	return index
}

func CandidateContext(base float64, family, normalizedTargetURL, query string, evidence map[string]any) CandidateScoreInput {
	target := map[string]any{}
	if value := strings.TrimSpace(normalizedTargetURL); value != "" {
		target["normalized_target_url"] = value
	}
	if value := strings.TrimSpace(query); value != "" {
		target["query"] = value
	}
	for _, key := range []string{"entity", "entity_id", "target_entity", "target_topic", "prompt", "prompt_text"} {
		if value := strings.TrimSpace(stringValueForScoring(evidence[key])); value != "" {
			target[key] = value
		}
	}
	audience := audienceFromEvidence(evidence)
	if len(audience) == 0 {
		switch {
		case strings.TrimSpace(query) != "":
			audience = []string{"people searching for " + strings.TrimSpace(query)}
		case strings.TrimSpace(normalizedTargetURL) != "":
			audience = []string{"organic and answer-engine visitors to " + strings.TrimSpace(normalizedTargetURL)}
		}
	}
	metric := strings.TrimSpace(stringValueForScoring(evidence["primary_metric"]))
	if metric == "" {
		metric = primaryMetricForFamily(family)
	}
	return CandidateScoreInput{
		BaseScore: base, ActionFamily: family, TargetIdentity: target,
		Audience: audience, PrimaryMetric: metric,
	}
}

func learningApplies(input CandidateScoreInput, row db.ListGrowthLearningsRow) (bool, []string) {
	if normalizeDimension(input.ActionFamily) != normalizeDimension(row.ActionFamily) {
		return false, nil
	}
	if normalizeDimension(input.PrimaryMetric) != normalizeDimension(row.PrimaryMetric) {
		return false, nil
	}
	historicalTarget := map[string]any{}
	if err := json.Unmarshal(row.TargetIdentity, &historicalTarget); err != nil || !identityOverlaps(input.TargetIdentity, historicalTarget) {
		return false, nil
	}
	historicalAudience := []string{}
	if err := json.Unmarshal(row.Audience, &historicalAudience); err != nil || !stringSetsOverlap(input.Audience, historicalAudience) {
		return false, nil
	}
	return true, []string{"action_family", "primary_metric", "target_identity", "audience"}
}

func identityOverlaps(left, right map[string]any) bool {
	leftValues := identityValues(left)
	rightValues := identityValues(right)
	if len(leftValues) == 0 || len(rightValues) == 0 {
		return false
	}
	for value := range leftValues {
		if _, ok := rightValues[value]; ok {
			return true
		}
	}
	return false
}

func identityValues(values map[string]any) map[string]struct{} {
	out := map[string]struct{}{}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, value := range flattenedStrings(values[key]) {
			if normalized := normalizeDimension(value); normalized != "" {
				out[normalized] = struct{}{}
			}
		}
	}
	return out
}

func stringSetsOverlap(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	seen := map[string]struct{}{}
	for _, value := range left {
		if normalized := normalizeDimension(value); normalized != "" {
			seen[normalized] = struct{}{}
		}
	}
	for _, value := range right {
		if _, ok := seen[normalizeDimension(value)]; ok {
			return true
		}
	}
	return false
}

func boundedOutcomeAdjustment(outcome string) float64 {
	adjustment := 0.0
	switch normalizeDimension(outcome) {
	case "positive":
		adjustment = 3
	case "negative":
		adjustment = -3
	case "mixed":
		adjustment = 0
	case "inconclusive":
		adjustment = 0
	}
	return clamp(adjustment, -maxSingleAdjustment, maxSingleAdjustment)
}

func primaryMetricForFamily(family string) string {
	switch normalizeDimension(family) {
	case "low_ctr", "low_ctr_snippet", "gsc_low_ctr", "gsc_low_ctr_query":
		return "gsc_ctr"
	case "striking_distance", "gsc_striking_distance_query":
		return "gsc_position"
	case "geo_project_mentioned_without_citation", "geo_competitor_cited_project_absent", "ai_citation_gap", "weak_citation_surface", "thin_evidence_page", "citation_fact_expansion", "cold_start_evidence_page":
		return "ai_citation_count"
	case "ga4_low_engagement", "ga4_high_traffic_low_engagement":
		return "ga4_engagement_rate"
	case "ga4_conversion_gap", "ga4_conversion_friction":
		return "ga4_key_events"
	default:
		return "gsc_clicks"
	}
}

func audienceFromEvidence(evidence map[string]any) []string {
	for _, key := range []string{"audience", "target_audience", "target_personas"} {
		values := flattenedStrings(evidence[key])
		if len(values) > 0 {
			return values
		}
	}
	return nil
}

func flattenedStrings(value any) []string {
	switch typed := value.(type) {
	case string:
		if value := strings.TrimSpace(typed); value != "" {
			return []string{value}
		}
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := strings.TrimSpace(stringValueForScoring(item)); value != "" {
				out = append(out, value)
			}
		}
		return out
	}
	return nil
}

func stringValueForScoring(value any) string {
	text, _ := value.(string)
	return text
}

func normalizeDimension(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimSuffix(value, "/")))
}

func clamp(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func roundScore(value float64) float64 {
	return math.Round(value*100) / 100
}
