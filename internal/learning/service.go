package learning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	RecordKindDirectional = "directional_learning"
	RecordKindQuality     = "measurement_quality"
)

type TerminalStore interface {
	RecordGrowthTerminalOutcome(context.Context, db.RecordGrowthTerminalOutcomeParams) error
}

func RecordTerminalOutcome(ctx context.Context, store TerminalStore, action db.ContentAction, opportunity db.SeoOpportunity, window, outcome json.RawMessage, terminalReason string) error {
	if action.Status != "completed" {
		return fmt.Errorf("Growth action %s is not terminal", action.ID)
	}
	outcomeData := object(outcome)
	label := normalizedString(outcomeData["outcome_label"])
	recordKind := ""
	switch label {
	case "positive", "negative", "mixed", "inconclusive":
		recordKind = RecordKindDirectional
	case "insufficient_data":
		recordKind = RecordKindQuality
	default:
		return fmt.Errorf("unsupported terminal Growth outcome %q", label)
	}
	windowData := object(window)
	spec := object(opportunity.GrowthSpec)
	baselineSnapshot := mustJSON(map[string]any{
		"growth_spec_baseline":   spec["baseline"],
		"action_baseline_window": object(action.BaselineWindow),
	})
	audience := arrayOrEmpty(spec["audience"])
	primaryMetric := firstString(spec["primary_metric"], windowData["primary_metric"], outcomeData["primary_metric"])
	targetIdentity := mustJSON(map[string]any{
		"target_url": action.TargetUrl, "normalized_target_url": action.NormalizedTargetUrl,
		"article_id": pgUUIDString(articleID(action)), "query": opportunity.Query,
	})
	applicability := mustJSON(map[string]any{
		"action_family": opportunity.Type, "target_identity": object(targetIdentity),
		"audience": audience, "primary_metric": primaryMetric,
	})
	qualityState, qualityGaps := qualityDetails(windowData, outcomeData)
	if terminalReason = strings.TrimSpace(terminalReason); terminalReason == "" {
		terminalReason = firstString(windowData["terminal_reason"], outcomeData["measurement_terminal_reason"], "measurement_checkpoints_completed")
	}
	article := articleID(action)
	return store.RecordGrowthTerminalOutcome(ctx, db.RecordGrowthTerminalOutcomeParams{
		ProjectID: action.ProjectID, OpportunityID: action.OpportunityID, ContentActionID: action.ID,
		ArticleID: article, ArtifactUrl: firstString(action.NormalizedTargetUrl, action.TargetUrl),
		ActionFamily: opportunity.Type, TargetIdentity: targetIdentity, Audience: mustJSON(audience),
		PrimaryMetric: primaryMetric, OutcomeLabel: label, RecordKind: recordKind,
		TerminalReason: terminalReason, MeasurementPolicyVersion: action.MeasurementPolicyVersion,
		BaselineSnapshot: baselineSnapshot, CheckpointSnapshot: mustJSON(windowData), OutcomeSnapshot: mustJSON(outcomeData),
		LearningSummary: firstString(outcomeData["outcome_reason"], outcomeData["summary"], label),
		Applicability:   applicability, DataQualityState: qualityState, QualityGaps: mustJSON(qualityGaps),
		QualityRecommendation: qualityRecommendation(qualityState),
	})
}

func qualityDetails(window, outcome map[string]any) (string, []string) {
	state := firstString(outcome["data_quality_state"], window["data_quality_state"], "insufficient")
	gaps := []string{}
	for _, source := range []any{outcome["confounders"], window["confounders"]} {
		if values, ok := source.([]any); ok {
			for _, value := range values {
				if text := normalizedString(value); text != "" {
					gaps = append(gaps, text)
				}
			}
		}
	}
	if len(gaps) == 0 {
		gaps = append(gaps, "No reliable comparable source window was available.")
	}
	return state, unique(gaps)
}

func qualityRecommendation(state string) string {
	switch state {
	case "provider_unavailable":
		return "Reconnect the unavailable provider, collect a fresh complete window, and open a new audited Growth Action if measurement should continue."
	case "stale":
		return "Refresh source observations and use a new audited measurement window; do not extend the closed action."
	default:
		return "Improve source coverage or instrumentation before opening a new measured Growth Action."
	}
}

func articleID(action db.ContentAction) pgtype.UUID {
	if action.DraftArticleID.Valid {
		return action.DraftArticleID
	}
	return action.TargetArticleID
}

func object(raw any) map[string]any {
	switch value := raw.(type) {
	case json.RawMessage:
		out := map[string]any{}
		if len(value) > 0 && json.Valid(value) {
			_ = json.Unmarshal(value, &out)
		}
		return out
	case []byte:
		return object(json.RawMessage(value))
	case map[string]any:
		return value
	default:
		return map[string]any{}
	}
}

func arrayOrEmpty(value any) []any {
	values, _ := value.([]any)
	if values == nil {
		return []any{}
	}
	return values
}

func firstString(values ...any) string {
	for _, value := range values {
		if text := normalizedString(value); text != "" {
			return text
		}
	}
	return ""
}

func normalizedString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func pgUUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", value.Bytes[0:4], value.Bytes[4:6], value.Bytes[6:8], value.Bytes[8:10], value.Bytes[10:16])
}

func mustJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}

func unique(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
