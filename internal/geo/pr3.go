package geo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/topicstate"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const AgentAnalyzer = "geo_analyzer"

type AnalyzeObservationsRequest struct {
	Limit int32 `json:"limit,omitempty"`
}

type AnalyzeObservationsResult struct {
	Run             db.GeoRun                               `json:"run"`
	Opportunities   []db.UpsertGEOObservationOpportunityRow `json:"opportunities"`
	AssetBriefs     []db.GeoAssetBrief                      `json:"asset_briefs"`
	DataSourceNotes []string                                `json:"data_source_notes"`
}

type AcceptGEOAssetBriefResult struct {
	Brief db.GeoAssetBrief `json:"brief"`
	Topic db.Topic         `json:"topic"`
}

type assetBriefTopicMetadata struct {
	AssetBriefID       string   `json:"asset_brief_id,omitempty"`
	Links              []string `json:"links,omitempty"`
	SourceEvidence     []string `json:"source_evidence,omitempty"`
	RecommendedOutline []string `json:"recommended_outline,omitempty"`
}

type geoGap struct {
	Type        string
	AssetType   string
	Action      string
	Impact      string
	Risk        string
	Evidence    map[string]any
	PromptText  string
	TargetTopic string
	Priority    float64
	Confidence  float64
}

func (s Service) AnalyzeObservations(ctx context.Context, projectID uuid.UUID, req AnalyzeObservationsRequest) (AnalyzeObservationsResult, error) {
	now := s.now()
	run, err := s.Q.StartGEORun(ctx, db.StartGEORunParams{
		ProjectID: projectID,
		Agent:     AgentAnalyzer,
		Provider:  ProviderDeterministic,
		StartedAt: pgutil.TS(now),
		Input:     jsonBytes(req),
	})
	if err != nil {
		return AnalyzeObservationsResult{}, err
	}
	result := AnalyzeObservationsResult{Run: run, DataSourceNotes: []string{"geo_observation_analyzer"}}
	finish := func(status string, output any, runErr error) (AnalyzeObservationsResult, error) {
		var errText *string
		if runErr != nil {
			message := runErr.Error()
			errText = &message
		}
		finished, finishErr := s.Q.FinishGEORun(ctx, db.FinishGEORunParams{
			ID:         run.ID,
			ProjectID:  projectID,
			Status:     status,
			FinishedAt: pgutil.TS(s.now()),
			Output:     jsonBytes(output),
			Error:      errText,
			CostUsd:    pgtype.Numeric{},
		})
		if finishErr == nil {
			result.Run = finished
		}
		if runErr != nil {
			return result, runErr
		}
		return result, finishErr
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	observations, err := s.Q.ListGEOObservations(ctx, db.ListGEOObservationsParams{ProjectID: projectID, LimitRows: limit})
	if err != nil {
		return finish("error", result, err)
	}
	prompts, err := s.Q.ListGEOPrompts(ctx, db.ListGEOPromptsParams{ProjectID: projectID})
	if err != nil {
		return finish("error", result, err)
	}
	promptByID := map[uuid.UUID]db.GeoPrompt{}
	for _, prompt := range prompts {
		promptByID[prompt.ID] = prompt
	}

	seen := map[string]struct{}{}
	for _, observation := range observations {
		for _, gap := range gapsForObservation(observation, promptByID) {
			key := gap.Type + "\x00" + gap.PromptText
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			opp, err := s.upsertObservationOpportunity(ctx, projectID, gap)
			if err != nil {
				return finish("error", result, err)
			}
			if opp.ID == uuid.Nil {
				continue
			}
			result.Opportunities = append(result.Opportunities, opp)
			if s.GrowthWriter != nil {
				canExecute, err := s.GrowthWriter.CanExecuteOpportunity(ctx, projectID, opp.ID)
				if err != nil {
					return finish("error", result, err)
				}
				if !canExecute {
					continue
				}
			}
			brief, err := s.createAssetBrief(ctx, projectID, run.ID, opp.ID, gap)
			if err != nil {
				return finish("error", result, err)
			}
			result.AssetBriefs = append(result.AssetBriefs, brief)
		}
	}
	return finish("ok", result, nil)
}

func (s Service) AcceptGEOAssetBrief(ctx context.Context, projectID, briefID uuid.UUID) (AcceptGEOAssetBriefResult, error) {
	brief, err := s.Q.GetGEOAssetBriefForProject(ctx, db.GetGEOAssetBriefForProjectParams{ID: briefID, ProjectID: projectID})
	if err != nil {
		return AcceptGEOAssetBriefResult{}, err
	}
	if s.GrowthWriter != nil {
		if err := s.GrowthWriter.EnsureOpportunityReserved(ctx, projectID, brief.OpportunityID); err != nil {
			return AcceptGEOAssetBriefResult{}, err
		}
		canExecute, err := s.GrowthWriter.CanExecuteOpportunity(ctx, projectID, brief.OpportunityID)
		if err != nil {
			return AcceptGEOAssetBriefResult{}, err
		}
		if !canExecute {
			return AcceptGEOAssetBriefResult{}, errors.New("Growth opportunity is blocked by unresolved Doctor work")
		}
	}
	accepted, err := s.Q.UpdateGEOAssetBriefStatus(ctx, db.UpdateGEOAssetBriefStatusParams{ID: briefID, ProjectID: projectID, Status: "accepted"})
	if err != nil {
		return AcceptGEOAssetBriefResult{}, err
	}
	topic, err := s.Q.CreateTopic(ctx, db.CreateTopicParams{
		ProjectID:     projectID,
		Channel:       publicationChannel(accepted.PublicationSurface),
		Title:         topicTitleForBrief(accepted),
		TargetKeyword: nil,
		TargetPrompt:  stringPtr(firstJSONText(accepted.TargetPrompts)),
		Angle:         stringPtr(accepted.AssetType),
		Format:        stringPtr("geo_asset_brief"),
		Priority:      8,
		InternalLinks: assetBriefTopicMetadataJSON(accepted),
		Status:        string(topicstate.StatusBacklog),
		ScheduledAt:   pgtype.Timestamptz{},
	})
	if err != nil {
		return AcceptGEOAssetBriefResult{}, err
	}
	converted, err := s.Q.UpdateGEOAssetBriefStatus(ctx, db.UpdateGEOAssetBriefStatusParams{ID: briefID, ProjectID: projectID, Status: "converted"})
	if err != nil {
		return AcceptGEOAssetBriefResult{}, err
	}
	return AcceptGEOAssetBriefResult{Brief: converted, Topic: topic}, nil
}

func gapsForObservation(observation db.GeoObservation, prompts map[uuid.UUID]db.GeoPrompt) []geoGap {
	prompt := prompts[uuidFromPG(observation.PromptID)]
	promptText := prompt.PromptText
	if promptText == "" {
		promptText = observation.Engine + " observation"
	}
	targetTopic := prompt.TargetTopic
	if targetTopic == "" {
		targetTopic = promptText
	}
	gaps := []geoGap{}
	if jsonArrayLen(observation.CompetitorCitations) > 0 && observation.ProjectCitationCount == 0 {
		gaps = append(gaps, geoGap{
			Type:        "geo_competitor_cited_project_absent",
			AssetType:   assetTypeForIntent(prompt.IntentType, true),
			Action:      "create comparison page",
			Impact:      "Give answer engines a project-owned, evidence-backed source for prompts where competitors are cited.",
			Risk:        "medium",
			Evidence:    observationEvidence(observation),
			PromptText:  promptText,
			TargetTopic: targetTopic,
			Priority:    88,
			Confidence:  confidenceScore(observation.Confidence),
		})
	}
	if observation.BrandMentioned && observation.ProjectCitationCount == 0 {
		gaps = append(gaps, geoGap{
			Type:        "geo_project_mentioned_without_citation",
			AssetType:   assetTypeForIntent(prompt.IntentType, false),
			Action:      "refresh canonical with evidence block",
			Impact:      "Turn brand mentions into citable project-owned sources with extractable evidence.",
			Risk:        "low",
			Evidence:    observationEvidence(observation),
			PromptText:  promptText,
			TargetTopic: targetTopic,
			Priority:    78,
			Confidence:  confidenceScore(observation.Confidence),
		})
	}
	return gaps
}

func (s Service) upsertObservationOpportunity(ctx context.Context, projectID uuid.UUID, gap geoGap) (db.UpsertGEOObservationOpportunityRow, error) {
	action := gap.Action
	impact := gap.Impact
	query := gap.PromptText
	if s.GrowthWriter == nil {
		return s.Q.UpsertGEOObservationOpportunity(ctx, db.UpsertGEOObservationOpportunityParams{
			ProjectID: projectID, Type: gap.Type, Status: "open",
			PriorityScore: pgutil.Numeric(gap.Priority), Confidence: pgutil.Numeric(gap.Confidence),
			PageUrl: nil, NormalizedPageUrl: "", Query: &query, Evidence: jsonBytes(gap.Evidence),
			RecommendedAction: &action, ExpectedImpact: &impact, Effort: 3, RiskLevel: gap.Risk,
		})
	}
	opportunity, err := s.GrowthWriter.CreateOpportunity(ctx, db.CreateCanonicalGrowthOpportunityParams{
		ID:                uuid.New(),
		ProjectID:         projectID,
		Type:              gap.Type,
		PriorityScore:     pgutil.Numeric(gap.Priority),
		Confidence:        pgutil.Numeric(gap.Confidence),
		PageUrl:           nil,
		NormalizedPageUrl: "",
		Query:             &query,
		Evidence:          jsonBytes(gap.Evidence),
		RecommendedAction: &action,
		ExpectedImpact:    &impact,
		Effort:            3,
		RiskLevel:         gap.Risk,
	})
	if err != nil {
		return db.UpsertGEOObservationOpportunityRow{}, err
	}
	return db.UpsertGEOObservationOpportunityRow{
		ID: opportunity.ID, ProjectID: opportunity.ProjectID, Type: opportunity.Type,
		Status: opportunity.Status, PriorityScore: opportunity.PriorityScore, Confidence: opportunity.Confidence,
		PageUrl: opportunity.PageUrl, NormalizedPageUrl: opportunity.NormalizedPageUrl,
		Query: opportunity.Query, Evidence: opportunity.Evidence, RecommendedAction: opportunity.RecommendedAction,
		ExpectedImpact: opportunity.ExpectedImpact, Effort: opportunity.Effort, RiskLevel: opportunity.RiskLevel,
		OpportunityIdentityKey: opportunity.OpportunityIdentityKey, EvidenceFingerprint: opportunity.EvidenceFingerprint,
		CanonicalGrowth: opportunity.CanonicalGrowth,
	}, nil
}

func (s Service) createAssetBrief(ctx context.Context, projectID, runID, opportunityID uuid.UUID, gap geoGap) (db.GeoAssetBrief, error) {
	return s.Q.CreateGEOAssetBrief(ctx, db.CreateGEOAssetBriefParams{
		ProjectID:          projectID,
		OpportunityID:      opportunityID,
		AssetType:          gap.AssetType,
		Status:             "ready_for_review",
		TargetPrompts:      jsonBytes([]string{gap.PromptText}),
		RequiredEvidence:   jsonBytes(requiredEvidenceForGap(gap)),
		RecommendedOutline: jsonBytes(outlineForGap(gap)),
		InternalLinkPlan:   jsonBytes([]string{}),
		PublicationSurface: "blog",
		CreatedByRunID:     uuidToPG(runID),
	})
}

func observationEvidence(observation db.GeoObservation) map[string]any {
	evidence := map[string]any{
		"source":                 "geo_observations",
		"reason":                 "geo_citation_source_gap",
		"why_now":                "A recorded answer-engine observation shows citation coverage missing or weaker than competitor/source evidence.",
		"scoring_method":         "geo_gap = competitor citations or brand mention observed while project citation count is zero",
		"scoring_version":        "geo_observation_v1",
		"expected_impact_range":  "medium",
		"data_source_notes":      []string{"geo_observations", "answer_engine_observation"},
		"idempotency_key":        strings.Join([]string{"geo_observations", observation.Engine, observation.ID.String()}, "|"),
		"observation_id":         observation.ID,
		"run_id":                 observation.RunID,
		"engine":                 observation.Engine,
		"source_type":            observation.SourceType,
		"observation_state":      observation.ObservationState,
		"cited_urls":             rawJSONList(observation.CitedUrls),
		"competitor_citations":   rawJSONList(observation.CompetitorCitations),
		"project_citation_count": observation.ProjectCitationCount,
		"brand_mentioned":        observation.BrandMentioned,
	}
	if observation.PromptID.Valid {
		evidence["prompt_id"] = observation.PromptID.Bytes
	}
	if observation.ObservedAt.Valid {
		evidence["observed_at"] = observation.ObservedAt.Time.UTC().Format(time.RFC3339)
	}
	return evidence
}

func assetTypeForIntent(intent string, competitorGap bool) string {
	if competitorGap || intent == "comparison" {
		return "comparison_page"
	}
	if intent == "alternative" {
		return "alternative_page"
	}
	if intent == "definition_entity" {
		return "glossary_definition"
	}
	return "source_backed_evidence_page"
}

func requiredEvidenceForGap(gap geoGap) []string {
	extras := gapSourceEvidence(gap.Evidence)
	if gap.Type == "geo_competitor_cited_project_absent" {
		return append([]string{"first-party comparison criteria", "supported product claims", "competitor citation evidence"}, extras...)
	}
	return append([]string{"self-contained definition or evidence block", "supported product claims", "extractable citation snippet"}, extras...)
}

func outlineForGap(gap geoGap) []string {
	return []string{
		fmt.Sprintf("Answer the prompt: %s", gap.PromptText),
		fmt.Sprintf("Explain %s with evidence", gap.TargetTopic),
		"Show cited sources and supported product claims",
		"Add internal links from related canonical pages",
	}
}

func topicTitleForBrief(brief db.GeoAssetBrief) string {
	prompt := firstJSONText(brief.TargetPrompts)
	if prompt == "" {
		return strings.ReplaceAll(brief.AssetType, "_", " ")
	}
	return prompt
}

func publicationChannel(surface string) string {
	if strings.TrimSpace(surface) == "external" {
		return "syndication"
	}
	return "blog"
}

func firstJSONText(raw json.RawMessage) string {
	var values []string
	_ = json.Unmarshal(raw, &values)
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}

func rawJSONList(raw json.RawMessage) []string {
	var values []string
	_ = json.Unmarshal(raw, &values)
	return values
}

func assetBriefTopicMetadataJSON(brief db.GeoAssetBrief) json.RawMessage {
	return jsonBytes(assetBriefTopicMetadata{
		AssetBriefID:       brief.ID.String(),
		Links:              rawJSONList(brief.InternalLinkPlan),
		SourceEvidence:     rawJSONList(brief.RequiredEvidence),
		RecommendedOutline: rawJSONList(brief.RecommendedOutline),
	})
}

func gapSourceEvidence(evidence map[string]any) []string {
	if len(evidence) == 0 {
		return nil
	}
	var out []string
	if values := stringValues(evidence["competitor_citations"]); len(values) > 0 {
		out = append(out, "competitor citations observed: "+strings.Join(values, ", "))
	}
	if values := stringValues(evidence["cited_urls"]); len(values) > 0 {
		out = append(out, "cited URLs: "+strings.Join(values, ", "))
	}
	if count, ok := evidence["project_citation_count"].(int32); ok {
		out = append(out, fmt.Sprintf("project citation count observed: %d", count))
	}
	return out
}

func stringValues(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func jsonArrayLen(raw json.RawMessage) int {
	return len(rawJSONList(raw))
}

func confidenceScore(confidence string) float64 {
	switch confidence {
	case ConfidenceHigh:
		return 90
	case ConfidenceLow:
		return 55
	default:
		return 70
	}
}

func uuidFromPG(value pgtype.UUID) uuid.UUID {
	if !value.Valid {
		return uuid.Nil
	}
	return uuid.UUID(value.Bytes)
}
