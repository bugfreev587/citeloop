package geo

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/crawl"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/citeloop/citeloop/internal/growthradar"
	"github.com/citeloop/citeloop/internal/growthspec"
	"github.com/citeloop/citeloop/internal/growthstage"
	"github.com/citeloop/citeloop/internal/learning"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/platformcontract"
	"github.com/citeloop/citeloop/internal/topicstate"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const AgentAnalyzer = "geo_analyzer"

type AnalyzeObservationsRequest struct {
	Limit                  int32                            `json:"limit,omitempty"`
	DryRun                 bool                             `json:"dry_run,omitempty"`
	CompetitiveSeedReports []crawl.SeedURLEnrichment        `json:"competitive_seed_reports,omitempty"`
	BeforeCreate           func(GrowthRadarCandidate) error `json:"-"`
}

type AnalyzeObservationsResult struct {
	Run             db.GeoRun                               `json:"run"`
	Opportunities   []db.UpsertGEOObservationOpportunityRow `json:"opportunities"`
	AssetBriefs     []db.GeoAssetBrief                      `json:"asset_briefs"`
	DataSourceNotes []string                                `json:"data_source_notes"`
	CandidateCount  int                                     `json:"candidate_count"`
	Candidates      []GrowthRadarCandidate                  `json:"candidates"`
	ReviewHoldCount int                                     `json:"review_hold_count"`
}

type GrowthRadarCandidate struct {
	Identity    string               `json:"identity"`
	Disposition string               `json:"disposition"`
	Reason      string               `json:"reason"`
	Score       growthradar.Score    `json:"score"`
	Snapshot    growthradar.Snapshot `json:"scoring_snapshot"`
	Evidence    json.RawMessage      `json:"evidence"`
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
	Type                 string
	AssetType            string
	Action               string
	Impact               string
	Risk                 string
	Evidence             map[string]any
	PromptText           string
	TargetTopic          string
	Priority             float64
	Confidence           float64
	Intent               string
	Audience             string
	Recurrence           int
	IndependentProviders int
	ObservationDates     int
}

type growthRadarDataStore interface {
	ListActivePlatformContentContracts(context.Context) ([]db.PlatformContentContract, error)
	ListPlatformTargetContexts(context.Context, db.ListPlatformTargetContextsParams) ([]db.PlatformTargetContext, error)
	ListPublisherConnections(context.Context, uuid.UUID) ([]db.PublisherConnection, error)
	GetGrowthStageSetting(context.Context, uuid.UUID) (db.GrowthStageSetting, error)
	GetGrowthRadarDemandSnapshot(context.Context, db.GetGrowthRadarDemandSnapshotParams) (db.GetGrowthRadarDemandSnapshotRow, error)
	CountRecentGrowthSearchEvidenceForQuery(context.Context, db.CountRecentGrowthSearchEvidenceForQueryParams) (int64, error)
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
	currentObservations := latestObservedByPromptEngine(observations)
	prompts, err := s.Q.ListGEOPrompts(ctx, db.ListGEOPromptsParams{ProjectID: projectID})
	if err != nil {
		return finish("error", result, err)
	}
	promptByID := map[uuid.UUID]db.GeoPrompt{}
	for _, prompt := range prompts {
		promptByID[prompt.ID] = prompt
	}
	recurrenceByPrompt := map[uuid.UUID]int{}
	providersByPrompt := map[uuid.UUID]map[string]struct{}{}
	datesByPrompt := map[uuid.UUID]map[string]struct{}{}
	cutoff := now.Add(-30 * 24 * time.Hour)
	for _, observation := range observations {
		if !qualifiesForGEODemand(observation, cutoff) {
			continue
		}
		promptID := uuidFromPG(observation.PromptID)
		if providersByPrompt[promptID] == nil {
			providersByPrompt[promptID] = map[string]struct{}{}
			datesByPrompt[promptID] = map[string]struct{}{}
		}
		providersByPrompt[promptID][strings.ToLower(strings.TrimSpace(observation.Engine))] = struct{}{}
		datesByPrompt[promptID][observation.ObservedAt.Time.UTC().Format("2006-01-02")] = struct{}{}
	}
	for _, observation := range currentObservations {
		if observation.PromptID.Valid {
			recurrenceByPrompt[observation.PromptID.Bytes]++
		}
	}
	topics, err := s.Q.ListTopics(ctx, projectID)
	if err != nil {
		return finish("error", result, err)
	}
	scorer := learning.NewProjectScorer(s.Q, projectID)
	pinnedStage := growthstage.DefaultSetting()
	if s.GrowthWriter != nil {
		pinnedStage, err = s.loadGrowthStage(ctx, projectID)
		if err != nil {
			return finish("error", result, err)
		}
	}

	seen := map[string]struct{}{}
	processGap := func(gap geoGap) error {
		var scoreErr error
		gap, scoreErr = applyGEOLearningScore(ctx, gap, scorer)
		if scoreErr != nil {
			return scoreErr
		}
		key := gap.Type + "\x00" + gap.PromptText
		if _, ok := seen[key]; ok {
			return nil
		}
		seen[key] = struct{}{}
		result.CandidateCount++
		candidate := GrowthRadarCandidate{Disposition: "opportunity", Reason: "legacy_store_without_canonical_writer"}
		materialized := growthradar.MaterializationResult{Disposition: "opportunity", Spec: growthspec.Result{State: growthspec.StateDecisionReady}}
		if s.GrowthWriter != nil {
			var materializeErr error
			candidate, materialized, materializeErr = s.scoreGrowthRadarGapWithStage(ctx, projectID, gap, topics, pinnedStage)
			if materializeErr != nil {
				return materializeErr
			}
			result.Candidates = append(result.Candidates, candidate)
		}
		if req.BeforeCreate != nil {
			if auditErr := req.BeforeCreate(candidate); auditErr != nil {
				return fmt.Errorf("persist growth radar candidate before creation: %w", auditErr)
			}
		}
		if req.DryRun {
			return nil
		}
		if candidate.Disposition != "opportunity" || materialized.Spec.State != growthspec.StateDecisionReady {
			return nil
		}
		if s.GrowthWriter != nil {
			gap.Priority = float64(candidate.Score.Final)
			gap.Evidence["opportunity_spec_v2"] = materialized.Input
			gap.Evidence["growth_radar_score"] = candidate.Score
			gap.Evidence["growth_radar_snapshot"] = candidate.Snapshot
		}
		opp, upsertErr := s.upsertObservationOpportunity(ctx, projectID, gap)
		if upsertErr != nil {
			if isCandidateReviewHold(upsertErr) {
				result.ReviewHoldCount++
				return nil
			}
			return upsertErr
		}
		if opp.ID == uuid.Nil {
			return nil
		}
		result.Opportunities = append(result.Opportunities, opp)
		if s.GrowthWriter != nil {
			canExecute, execErr := s.GrowthWriter.CanExecuteOpportunity(ctx, projectID, opp.ID)
			if execErr != nil {
				return execErr
			}
			if !canExecute {
				return nil
			}
		}
		brief, briefErr := s.createAssetBrief(ctx, projectID, run.ID, opp.ID, gap)
		if briefErr != nil {
			return briefErr
		}
		result.AssetBriefs = append(result.AssetBriefs, brief)
		return nil
	}
	for _, observation := range currentObservations {
		for _, gap := range gapsForObservation(observation, promptByID) {
			promptID := uuidFromPG(observation.PromptID)
			gap.Recurrence = recurrenceByPrompt[promptID]
			gap.IndependentProviders = len(providersByPrompt[promptID])
			gap.ObservationDates = len(datesByPrompt[promptID])
			if err := processGap(gap); err != nil {
				return finish("error", result, err)
			}
		}
	}
	for _, gap := range gapsForCompetitiveSeedReports(req.CompetitiveSeedReports) {
		if err := processGap(gap); err != nil {
			return finish("error", result, err)
		}
	}
	return finish("ok", result, nil)
}

func isCandidateReviewHold(err error) bool {
	return errors.Is(err, discovery.ErrCandidateReviewRequired)
}

func hasObservationAnswerMaterial(observation db.GeoObservation) bool {
	return strings.TrimSpace(observation.AnswerSummary) != "" ||
		jsonArrayLen(observation.EvidenceSnippets) > 0 || jsonArrayLen(observation.CitedUrls) > 0 || jsonArrayLen(observation.CompetitorCitations) > 0
}

func qualifiesForGEODemand(observation db.GeoObservation, cutoff time.Time) bool {
	return observation.SourceType == SourceTypeAnswerEngine && observation.ObservationState == "observed" &&
		observation.PromptID.Valid && observation.ObservedAt.Valid && !observation.ObservedAt.Time.Before(cutoff) &&
		strings.TrimSpace(observation.Engine) != "" && hasObservationAnswerMaterial(observation)
}

// ListGEOObservations is newest-first. Candidate generation represents the
// current answer state, so each prompt/engine contributes only its newest
// successful observation. Provider failures are diagnostic events and do not
// erase the last usable answer.
func latestObservedByPromptEngine(observations []db.GeoObservation) []db.GeoObservation {
	latest := make([]db.GeoObservation, 0, len(observations))
	seen := make(map[string]struct{}, len(observations))
	for _, observation := range observations {
		if observation.ObservationState != "observed" {
			continue
		}
		key := "observation:" + observation.ID.String()
		if observation.PromptID.Valid {
			key = uuid.UUID(observation.PromptID.Bytes).String() + "\x00" + strings.ToLower(strings.TrimSpace(observation.Engine))
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		latest = append(latest, observation)
	}
	return latest
}

func applyGEOLearningScore(ctx context.Context, gap geoGap, scorer learning.CandidateScorer) (geoGap, error) {
	result, err := scorer.ScoreCandidate(ctx, learning.CandidateContext(gap.Priority, gap.Type, "", gap.PromptText, gap.Evidence))
	if err != nil {
		return geoGap{}, err
	}
	if len(result.LearningIDs) == 0 {
		return gap, nil
	}
	gap.Priority = result.AdjustedScore
	if gap.Evidence == nil {
		gap.Evidence = map[string]any{}
	}
	gap.Evidence["learning_scoring"] = result.Provenance()
	return gap, nil
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
			Intent:      prompt.IntentType,
			Audience:    prompt.TargetPersona,
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
			Intent:      prompt.IntentType,
			Audience:    prompt.TargetPersona,
		})
	}
	if observation.ObservationState == "observed" && !observation.BrandMentioned && observation.ProjectCitationCount == 0 && jsonArrayLen(observation.CompetitorCitations) == 0 {
		gaps = append(gaps, geoGap{
			Type:        "geo_project_absent_from_answer",
			AssetType:   assetTypeForIntent(prompt.IntentType, false),
			Action:      "create answer-ready canonical",
			Impact:      "Create a relevant, evidence-backed source for an answer where the project is currently absent.",
			Risk:        "low",
			Evidence:    observationEvidence(observation),
			PromptText:  promptText,
			TargetTopic: targetTopic,
			Priority:    70,
			Confidence:  confidenceScore(observation.Confidence),
			Intent:      prompt.IntentType,
			Audience:    prompt.TargetPersona,
		})
	}
	return gaps
}

func gapsForCompetitiveSeedReports(reports []crawl.SeedURLEnrichment) []geoGap {
	gaps := make([]geoGap, 0, len(reports))
	for _, report := range reports {
		top := report.TopArchetype()
		spec, ok := competitiveSeedGapSpec(top.Archetype, report.Host)
		if !ok || report.StatusCode < 200 || report.StatusCode >= 400 || !report.RobotsAllowed || !report.Indexable || strings.ToLower(strings.TrimSpace(top.Confidence)) != "high" {
			continue
		}
		seedURL := strings.TrimSpace(report.CanonicalURL)
		if seedURL == "" {
			seedURL = strings.TrimSpace(report.FinalURL)
		}
		if seedURL == "" {
			seedURL = strings.TrimSpace(report.URL)
		}
		if seedURL == "" || strings.TrimSpace(report.Host) == "" {
			continue
		}
		evidence := map[string]any{
			"source":                     "competitive_seed_url",
			"source_type":                "competitive_seed_url",
			"reason":                     spec.Type,
			"why_now":                    spec.WhyNow,
			"scoring_method":             "competitive_seed = high-confidence " + spec.Archetype + " with crawl/indexability evidence",
			"scoring_version":            "competitive_seed_v1",
			"expected_impact_range":      "medium",
			"data_source_notes":          []string{"competitive_seed_url", "crawler_enrichment"},
			"seed_url":                   seedURL,
			"competitor_domain":          strings.ToLower(strings.TrimSpace(report.Host)),
			"archetype":                  top.Archetype,
			"archetype_confidence":       top.Confidence,
			"signals":                    append([]string(nil), report.Signals...),
			"sitemap_included":           report.SitemapIncluded,
			"same_archetype_link_count":  report.SameArchetypeLinkCount,
			"sitemap_url_sample_count":   len(report.SitemapURLSamples),
			"robots_allowed":             report.RobotsAllowed,
			"indexable":                  report.Indexable,
			"status_code":                report.StatusCode,
			"project_citation_count":     int32(0),
			"competitive_seed_url_count": int32(1),
		}
		if source := strings.TrimSpace(report.DiscoverySource); source != "" {
			evidence["discovery_source"] = source
			notes, _ := evidence["data_source_notes"].([]string)
			evidence["data_source_notes"] = append(notes, source)
		}
		if fromURL := strings.TrimSpace(report.DiscoveredFromURL); fromURL != "" {
			evidence["discovered_from_url"] = fromURL
		}
		if title := strings.TrimSpace(report.Title); title != "" {
			evidence["seed_title"] = title
		}
		if h1 := strings.TrimSpace(report.PrimaryH1); h1 != "" {
			evidence["seed_primary_h1"] = h1
		}
		if description := strings.TrimSpace(report.MetaDescription); description != "" {
			evidence["seed_meta_description"] = description
		}
		promptText, targetTopic, topicSource := competitiveSeedPromptTarget(spec, seedURL, report.Title, report.PrimaryH1, report.MetaDescription)
		if topicSource != "" {
			evidence["target_topic_source"] = topicSource
			evidence["derived_target_topic"] = targetTopic
		}
		evidence["idempotency_key"] = strings.Join([]string{"competitive_seed_topic", spec.Archetype, targetTopic}, "|")
		gaps = append(gaps, geoGap{
			Type:        spec.Type,
			AssetType:   spec.AssetType,
			Action:      spec.Action,
			Impact:      spec.Impact,
			Risk:        "medium",
			Evidence:    evidence,
			PromptText:  promptText,
			TargetTopic: targetTopic,
			Priority:    spec.Priority,
			Confidence:  spec.Confidence,
			Intent:      spec.Intent,
			Audience:    "developers",
			Recurrence:  1,
		})
	}
	return collapseCompetitiveSeedTopicGaps(gaps)
}

type competitiveSeedGapDefinition struct {
	Archetype   string
	Type        string
	AssetType   string
	Action      string
	Impact      string
	WhyNow      string
	PromptText  string
	TargetTopic string
	Intent      string
	Priority    float64
	Confidence  float64
}

func competitiveSeedGapSpec(archetype string, host string) (competitiveSeedGapDefinition, bool) {
	archetype = strings.ToLower(strings.TrimSpace(archetype))
	competitor := competitorNameFromHost(host)
	switch archetype {
	case "tools_hub":
		return competitiveSeedGapDefinition{
			Archetype:   "tools_hub",
			Type:        "competitive_tools_hub_gap",
			AssetType:   "source_backed_evidence_page",
			Action:      "create project-fit tools hub",
			Impact:      "Capture demand exposed by a competitor tools hub with a project-owned, evidence-backed resource.",
			WhyNow:      "A competitor seed URL exposes a crawlable, indexable, high-confidence tools hub archetype.",
			PromptText:  "best social publishing tools",
			TargetTopic: "social publishing tools",
			Intent:      "category_recommendation",
			Priority:    84,
			Confidence:  0.82,
		}, true
	case "alternatives_cluster":
		return competitiveSeedGapDefinition{
			Archetype:   "alternatives_cluster",
			Type:        "competitive_alternative_gap",
			AssetType:   "alternative_page",
			Action:      "create project-fit alternative page",
			Impact:      "Capture alternative-intent demand exposed by a competitor alternatives cluster with a source-backed project page.",
			WhyNow:      "A competitor seed URL exposes a crawlable, indexable, high-confidence alternatives archetype.",
			PromptText:  "alternatives to " + competitor,
			TargetTopic: competitor + " alternatives",
			Intent:      "alternative",
			Priority:    81,
			Confidence:  0.8,
		}, true
	case "comparison_cluster":
		return competitiveSeedGapDefinition{
			Archetype:   "comparison_cluster",
			Type:        "competitive_comparison_cluster_gap",
			AssetType:   "comparison_page",
			Action:      "create project-fit comparison page",
			Impact:      "Capture comparison-intent demand exposed by a competitor comparison cluster with supported differentiators.",
			WhyNow:      "A competitor seed URL exposes a crawlable, indexable, high-confidence comparison archetype.",
			PromptText:  competitor + " comparison",
			TargetTopic: competitor + " comparison",
			Intent:      "comparison",
			Priority:    82,
			Confidence:  0.8,
		}, true
	default:
		return competitiveSeedGapDefinition{}, false
	}
}

func competitiveSeedPromptTarget(spec competitiveSeedGapDefinition, seedURL, title, primaryH1, metaDescription string) (promptText, targetTopic, source string) {
	subject, ok := competitiveSeedSubjectFromURL(seedURL, spec.Archetype)
	if !ok {
		for _, candidate := range []struct {
			value  string
			source string
		}{
			{title, "seed_page_title"},
			{primaryH1, "seed_page_h1"},
			{metaDescription, "seed_meta_description"},
		} {
			subject, ok = competitiveSeedSubjectFromTitle(candidate.value, spec.Archetype)
			if !ok {
				continue
			}
			promptText, targetTopic, ok = competitiveSeedPromptTargetForSubject(spec, subject)
			if !ok {
				continue
			}
			return promptText, targetTopic, candidate.source
		}
		return spec.PromptText, spec.TargetTopic, ""
	}
	promptText, targetTopic, ok = competitiveSeedPromptTargetForSubject(spec, subject)
	if !ok {
		return spec.PromptText, spec.TargetTopic, ""
	}
	return promptText, targetTopic, "seed_url_path"
}

func collapseCompetitiveSeedTopicGaps(gaps []geoGap) []geoGap {
	collapsed := make([]geoGap, 0, len(gaps))
	indexByKey := map[string]int{}
	for _, gap := range gaps {
		key := competitiveSeedTopicGroupKey(gap)
		if key == "" {
			collapsed = append(collapsed, gap)
			continue
		}
		if index, ok := indexByKey[key]; ok {
			mergeCompetitiveSeedTopicGap(&collapsed[index], gap)
			continue
		}
		indexByKey[key] = len(collapsed)
		collapsed = append(collapsed, gap)
	}
	return collapsed
}

func competitiveSeedTopicGroupKey(gap geoGap) string {
	if gap.Evidence == nil || gap.Evidence["source"] != "competitive_seed_url" {
		return ""
	}
	topic := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(gap.TargetTopic)), " "))
	if topic == "" {
		return ""
	}
	return strings.Join([]string{gap.Type, topic}, "|")
}

func mergeCompetitiveSeedTopicGap(base *geoGap, incoming geoGap) {
	if base == nil || base.Evidence == nil {
		return
	}
	count := int32FromEvidence(base.Evidence["competitive_seed_url_count"], 1) + int32FromEvidence(incoming.Evidence["competitive_seed_url_count"], 1)
	base.Evidence["competitive_seed_url_count"] = count
	base.Evidence["competitive_seed_topic_grouped"] = true
	base.Evidence["idempotency_key"] = strings.Join([]string{"competitive_seed_topic", evidenceString(base.Evidence, "archetype"), base.TargetTopic}, "|")
	samples := evidenceStringSlice(base.Evidence["seed_url_samples"])
	if len(samples) == 0 {
		samples = mergeEvidenceStrings(nil, evidenceString(base.Evidence, "seed_url"))
	}
	base.Evidence["seed_url_samples"] = mergeEvidenceStrings(samples, evidenceString(incoming.Evidence, "seed_url"))
	domains := evidenceStringSlice(base.Evidence["competitor_domain_samples"])
	if len(domains) == 0 {
		domains = mergeEvidenceStrings(nil, evidenceString(base.Evidence, "competitor_domain"))
	}
	domains = mergeEvidenceStrings(domains, evidenceString(incoming.Evidence, "competitor_domain"))
	base.Evidence["competitor_domain_samples"] = domains
	base.Evidence["competitor_domain_count"] = int32(len(domains))
	if len(domains) > 1 {
		base.Evidence["competitive_domain_diversity"] = true
	}
	base.Evidence["signals"] = mergeEvidenceStrings(evidenceStringSlice(base.Evidence["signals"]), evidenceStringSlice(incoming.Evidence["signals"])...)
	base.Evidence["data_source_notes"] = mergeEvidenceStrings(evidenceStringSlice(base.Evidence["data_source_notes"]), evidenceStringSlice(incoming.Evidence["data_source_notes"])...)
	base.Evidence["same_archetype_link_count"] = maxInt32Evidence(base.Evidence["same_archetype_link_count"], incoming.Evidence["same_archetype_link_count"])
	base.Evidence["sitemap_url_sample_count"] = maxInt32Evidence(base.Evidence["sitemap_url_sample_count"], incoming.Evidence["sitemap_url_sample_count"])
	base.Recurrence = int(count)
	boost := float64(count-1) * 2
	if len(domains) > 1 {
		boost += float64(len(domains)-1) * 3
	}
	if boost > 6 {
		boost = 6
	}
	base.Priority = minFloat64(90, base.Priority+boost)
	confidenceBoost := float64(count-1) * 0.03
	if len(domains) > 1 {
		confidenceBoost += float64(len(domains)-1) * 0.03
	}
	if confidenceBoost > 0.08 {
		confidenceBoost = 0.08
	}
	base.Confidence = minFloat64(0.9, base.Confidence+confidenceBoost)
}

func evidenceString(evidence map[string]any, key string) string {
	if evidence == nil {
		return ""
	}
	value, _ := evidence[key].(string)
	return strings.TrimSpace(value)
}

func evidenceStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func mergeEvidenceStrings(existing []string, values ...string) []string {
	merged := make([]string, 0, len(existing)+len(values))
	seen := map[string]bool{}
	for _, value := range append(existing, values...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		merged = append(merged, value)
	}
	return merged
}

func int32FromEvidence(value any, fallback int32) int32 {
	switch typed := value.(type) {
	case int32:
		return typed
	case int:
		return int32(typed)
	case float64:
		return int32(typed)
	default:
		return fallback
	}
}

func maxInt32Evidence(left, right any) int32 {
	l := int32FromEvidence(left, 0)
	r := int32FromEvidence(right, 0)
	if r > l {
		return r
	}
	return l
}

func minFloat64(left, right float64) float64 {
	if left < right {
		return left
	}
	return right
}

func competitiveSeedPromptTargetForSubject(spec competitiveSeedGapDefinition, subject string) (promptText, targetTopic string, ok bool) {
	switch spec.Archetype {
	case "tools_hub":
		targetTopic = subject
		promptText = "best " + subject
		if !strings.Contains(subject, "tool") {
			promptText += " tools"
		}
	case "alternatives_cluster":
		base := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(subject, " alternatives"), " alternative"))
		if base == "" {
			return "", "", false
		}
		targetTopic = base + " alternatives"
		promptText = "alternatives to " + base
	case "comparison_cluster":
		base := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(subject, " comparison"), " compare"))
		if base == "" {
			return "", "", false
		}
		targetTopic = base + " comparison"
		promptText = targetTopic
	default:
		return "", "", false
	}
	return promptText, targetTopic, true
}

func competitiveSeedSubjectFromURL(rawURL, archetype string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", false
	}
	segments := pathSegments(parsed.Path)
	markerIndex := -1
	for index, segment := range segments {
		switch archetype {
		case "tools_hub":
			if segment == "tools" || segment == "tool" {
				markerIndex = index
			}
		case "alternatives_cluster":
			if segment == "alternatives" || segment == "alternative" {
				markerIndex = index
			}
		case "comparison_cluster":
			if segment == "compare" || segment == "comparison" {
				markerIndex = index
			}
		}
		if markerIndex >= 0 {
			break
		}
	}
	if markerIndex < 0 || markerIndex+1 >= len(segments) {
		return "", false
	}
	subject := strings.Join(segments[markerIndex+1:], " ")
	subject = strings.Join(strings.Fields(subject), " ")
	if len(strings.Fields(subject)) == 0 {
		return "", false
	}
	return subject, true
}

func competitiveSeedSubjectFromTitle(title, archetype string) (string, bool) {
	normalized := normalizeCompetitiveSeedTitle(title)
	if normalized == "" {
		return "", false
	}
	switch archetype {
	case "tools_hub":
		subject := normalized
		for {
			before := subject
			subject = strings.TrimSpace(strings.TrimPrefix(subject, "best "))
			subject = strings.TrimSpace(strings.TrimPrefix(subject, "free "))
			if subject == before {
				break
			}
		}
		subject = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(subject, " tool"), " tools"))
		fields := strings.Fields(subject)
		if len(fields) < 2 || subject == "tools" || subject == "tool" {
			return "", false
		}
		return subject, true
	case "alternatives_cluster":
		before, _, found := strings.Cut(normalized, " alternatives")
		if !found {
			return "", false
		}
		before = strings.TrimSpace(strings.TrimPrefix(before, "best "))
		if before == "" {
			return "", false
		}
		parts := strings.Fields(before)
		if len(parts) == 0 {
			return "", false
		}
		return parts[len(parts)-1], true
	case "comparison_cluster":
		before, _, _ := strings.Cut(normalized, ":")
		subject := strings.TrimSpace(before)
		if !strings.Contains(subject, " vs ") {
			return "", false
		}
		return subject, true
	default:
		return "", false
	}
}

func normalizeCompetitiveSeedTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	for _, separator := range []string{"|", " — ", " – ", " - "} {
		if before, _, found := strings.Cut(title, separator); found {
			title = before
			break
		}
	}
	title = strings.ToLower(title)
	replacer := strings.NewReplacer("\n", " ", "\t", " ", "_", " ")
	title = replacer.Replace(title)
	title = strings.Join(strings.Fields(title), " ")
	return strings.Trim(title, " .!?…:;")
}

func pathSegments(path string) []string {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
	segments := make([]string, 0, len(parts))
	for _, part := range parts {
		unescaped, err := url.PathUnescape(part)
		if err != nil {
			unescaped = part
		}
		unescaped = strings.TrimSuffix(unescaped, ".html")
		unescaped = strings.TrimSuffix(unescaped, ".htm")
		unescaped = strings.ReplaceAll(unescaped, "-", " ")
		unescaped = strings.ReplaceAll(unescaped, "_", " ")
		unescaped = strings.ToLower(strings.Join(strings.Fields(unescaped), " "))
		if unescaped != "" {
			segments = append(segments, unescaped)
		}
	}
	return segments
}

func competitorNameFromHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(host, "www.")
	if host == "" {
		return "competitor"
	}
	parts := strings.Split(host, ".")
	if strings.TrimSpace(parts[0]) == "" {
		return "competitor"
	}
	return parts[0]
}

func (s Service) scoreGrowthRadarGap(ctx context.Context, projectID uuid.UUID, gap geoGap, topics []db.Topic) (GrowthRadarCandidate, growthradar.MaterializationResult, error) {
	stageSetting, err := s.loadGrowthStage(ctx, projectID)
	if err != nil {
		return GrowthRadarCandidate{}, growthradar.MaterializationResult{}, err
	}
	return s.scoreGrowthRadarGapWithStage(ctx, projectID, gap, topics, stageSetting)
}

func (s Service) loadGrowthStage(ctx context.Context, projectID uuid.UUID) (growthstage.Setting, error) {
	stageSetting := growthstage.DefaultSetting()
	if store, ok := s.Q.(growthRadarDataStore); ok {
		row, stageErr := store.GetGrowthStageSetting(ctx, projectID)
		if stageErr == nil {
			if row.StageProfileVersion != growthstage.ProfileVersion {
				return growthstage.Setting{}, fmt.Errorf("unsupported growth stage profile %q", row.StageProfileVersion)
			}
			stageSetting = growthstage.Setting{Stage: growthstage.Stage(row.Stage), StageProfileVersion: row.StageProfileVersion, SettingVersion: row.SettingVersion, IsDefaultUnconfirmed: row.IsDefaultUnconfirmed}
		} else if !errors.Is(stageErr, pgx.ErrNoRows) {
			return growthstage.Setting{}, stageErr
		}
	}
	return stageSetting, nil
}

func (s Service) scoreGrowthRadarGapWithStage(ctx context.Context, projectID uuid.UUID, gap geoGap, topics []db.Topic, stageSetting growthstage.Setting) (GrowthRadarCandidate, growthradar.MaterializationResult, error) {
	intent, journey, conversion, intentSupported := growthIntentMapping(gap.Intent, gap.Type)
	audience := strings.TrimSpace(gap.Audience)
	now := s.now()
	profile, profileErr := s.Q.GetActiveProfile(ctx, projectID)
	if profileErr != nil && !errors.Is(profileErr, pgx.ErrNoRows) {
		return GrowthRadarCandidate{}, growthradar.MaterializationResult{}, profileErr
	}
	capabilityConfirmed, audienceConfirmed := false, false
	if profileErr == nil {
		capabilityConfirmed, audienceConfirmed = confirmedProfileMappings(profile.Profile, gap.TargetTopic, audience, evidenceIndexForGap(gap, now))
	}
	snapshot := growthradar.Snapshot{
		Stage: string(stageSetting.Stage), StageProfileVersion: stageSetting.StageProfileVersion, StageSettingVersion: stageSetting.SettingVersion,
		PrimaryCoverage: "none", InternalLinkPaths: 0,
		CapabilityConfirmed: capabilityConfirmed, AudienceConfirmed: audienceConfirmed, IntentSupported: intentSupported,
		Intent: intent, JourneyStage: journey, ConversionMapping: conversion,
		MaterialChange: "unchanged",
	}
	if source, ageDays, qualified := qualifiedObservationEvidence(gap.Evidence, claimTypeForGap(gap.Type), now); qualified {
		snapshot.QualifiedRecurrence = 1
		snapshot.EvidenceSources = append(snapshot.EvidenceSources, source)
		snapshot.NewestEvidenceAgeDays = ageDays
	}
	snapshot.IndependentGEOProviders = gap.IndependentProviders
	snapshot.GEOObservationDates = gap.ObservationDates
	snapshot.SensitiveOrUnsupported = growthradar.ContainsInternalSensitiveTerm(gap.PromptText) || growthradar.ContainsInternalSensitiveTerm(gap.TargetTopic)
	matchingExistingAsset := false
	for _, topic := range topics {
		if sameNormalizedText(topic.Title, gap.TargetTopic) || (topic.TargetKeyword != nil && sameNormalizedText(*topic.TargetKeyword, gap.TargetTopic)) {
			snapshot.ExactDuplicate = true
			snapshot.PrimaryCoverage = "covered"
			matchingExistingAsset = true
			break
		}
		if nearNormalizedText(topic.Title, gap.TargetTopic) || (topic.TargetKeyword != nil && nearNormalizedText(*topic.TargetKeyword, gap.TargetTopic)) {
			snapshot.NearDuplicate = true
		}
	}
	var target growthspec.TargetSpec
	if store, ok := s.Q.(growthRadarDataStore); ok {
		aliases := []string{normalizedIdentity(gap.PromptText)}
		if topic := normalizedIdentity(gap.TargetTopic); topic != "" && topic != aliases[0] {
			aliases = append(aliases, topic)
		}
		demand, err := store.GetGrowthRadarDemandSnapshot(ctx, db.GetGrowthRadarDemandSnapshotParams{ProjectID: projectID, Queries: aliases})
		if err != nil {
			return GrowthRadarCandidate{}, growthradar.MaterializationResult{}, err
		}
		snapshot.CurrentImpressions, snapshot.PreviousImpressions = int(demand.CurrentImpressions), int(demand.PreviousImpressions)
		snapshot.MaterialChange = demandMaterialChange(snapshot.CurrentImpressions, snapshot.PreviousImpressions)
		snapshot.HasMaterialChangeEvidence = snapshot.MaterialChange != "unchanged"
		snapshot.HasExistingAsset = matchingExistingAsset
		snapshot.HasSuccessSignal = matchingExistingAsset && snapshot.CurrentImpressions > 0
		if demand.CurrentImpressions > 0 || demand.PreviousImpressions > 0 {
			snapshot.EvidenceSources = append(snapshot.EvidenceSources, growthradar.EvidenceSource{Class: "search_console", Qualified: true, FirstParty: true, CompleteProvenance: true})
		}
		searchCount, err := store.CountRecentGrowthSearchEvidenceForQuery(ctx, db.CountRecentGrowthSearchEvidenceForQueryParams{ProjectID: projectID, Query: gap.PromptText, SinceAt: pgtype.Timestamptz{Time: now.Add(-30 * 24 * time.Hour), Valid: true}})
		if err != nil {
			return GrowthRadarCandidate{}, growthradar.MaterializationResult{}, err
		}
		if searchCount > 0 {
			snapshot.EvidenceSources = append(snapshot.EvidenceSources, growthradar.EvidenceSource{Class: "search_result", Qualified: true, CompleteProvenance: true})
		}
		contracts, err := store.ListActivePlatformContentContracts(ctx)
		if err != nil {
			return GrowthRadarCandidate{}, growthradar.MaterializationResult{}, err
		}
		contexts, err := store.ListPlatformTargetContexts(ctx, db.ListPlatformTargetContextsParams{ProjectID: projectID, Platform: ""})
		if err != nil {
			return GrowthRadarCandidate{}, growthradar.MaterializationResult{}, err
		}
		connections, err := store.ListPublisherConnections(ctx, projectID)
		if err != nil {
			return GrowthRadarCandidate{}, growthradar.MaterializationResult{}, err
		}
		target = growthRadarTarget(gap.AssetType, contracts, contexts, connections, now)
		snapshot.SelectedExternalTargets = maxInt(0, len(target.TargetPlatforms)-1)
		snapshot.CompatibleExternalTargets = snapshot.SelectedExternalTargets
		snapshot.AdditionalOutputTypes = additionalOutputTypes(target)
		snapshot.HasResolvedExpansion = snapshot.CompatibleExternalTargets > 0 || snapshot.AdditionalOutputTypes > 0
		if stageSetting.Stage == growthstage.Scale && !snapshot.HasResolvedExpansion {
			snapshot.MissingStageConfiguration = true
		}
		if stageSetting.Stage == growthstage.Scale && snapshot.ExactDuplicate && snapshot.HasResolvedExpansion {
			// Scale work extends a proven canonical asset to contract-native
			// outputs. It is not a duplicate canonical-creation candidate.
			snapshot.ExactDuplicate = false
			gap.Action = "expand existing asset with contract-native variants"
		}
		if stageSetting.Stage == growthstage.Optimize && snapshot.ExactDuplicate && snapshot.HasMaterialChangeEvidence {
			// Optimize deliberately targets an existing asset. Preserve covered
			// state but route the Opportunity to refresh instead of net-new work.
			snapshot.ExactDuplicate = false
			gap.Action = "refresh existing asset for measured change"
		}
	}
	snapshot.LLMOnlyEvidence = onlyAnswerEngineEvidence(snapshot.EvidenceSources)
	score, err := growthradar.ScoreCandidateForStage(snapshot, stageSetting.Stage)
	if err != nil {
		return GrowthRadarCandidate{}, growthradar.MaterializationResult{}, err
	}
	evidence := jsonBytes(gap.Evidence)
	materialized := growthradar.MaterializeOpportunitySpec(growthradar.MaterializationCandidate{
		ProjectID: projectID, ClusterID: normalizedIdentity(gap.TargetTopic), Topic: gap.TargetTopic,
		Intent: intent, JourneyStage: journey, Audience: audience, AssetType: gap.AssetType,
		Action: gap.Action, ExpectedUserValue: gap.Impact, Evidence: evidence,
		SuccessMetric: growthspec.SuccessMetric{Name: "gsc_clicks", WindowDays: 56}, Target: target, Score: score,
		SourceVersions: map[string]string{"scoring": growthradar.FormulaVersion, "geo": "geo_observation_v1", "targeting": "platform-contract-v1"},
	})
	identity := growthradar.DedupeIdentity(growthradar.TopicIdentityInput{ProjectID: projectID.String(), Cluster: gap.TargetTopic, Intent: intent, Audience: audience, AssetType: gap.AssetType, CanonicalTarget: target.CanonicalTarget.Platform + ":" + target.CanonicalTarget.TargetKey})
	reason := strings.Join(score.ReasonCodes, ",")
	if reason == "" {
		reason = score.Disposition
	}
	return GrowthRadarCandidate{Identity: identity, Disposition: score.Disposition, Reason: reason, Score: score, Snapshot: snapshot, Evidence: evidence}, materialized, nil
}

func growthRadarTarget(assetType string, contracts []db.PlatformContentContract, contexts []db.PlatformTargetContext, connections []db.PublisherConnection, now time.Time) growthspec.TargetSpec {
	var blog platformcontract.Target
	connectionReady := map[string]bool{}
	for _, connection := range connections {
		if !connection.Enabled || connection.Status != "connected" {
			continue
		}
		platform := strings.ToLower(strings.TrimSpace(connection.Kind))
		if platform == "github_nextjs" {
			platform = "blog"
		}
		connectionReady[platform] = true
	}
	currentContext := map[string]db.PlatformTargetContext{}
	for _, targetContext := range contexts {
		if targetContext.Status != "confirmed" || !targetContext.ExpiresAt.Valid || !targetContext.ExpiresAt.Time.After(now) {
			continue
		}
		if existing, ok := currentContext[targetContext.Platform]; !ok || targetContext.Version > existing.Version {
			currentContext[targetContext.Platform] = targetContext
		}
	}
	targets := []platformcontract.Target{}
	for _, contract := range contracts {
		if contract.Status != "active" || !contract.GenerationSupported || !jsonStringListContains(contract.CompatibleAssetTypes, assetType) {
			continue
		}
		outputs := rawStringList(contract.AllowedOutputTypes)
		if len(outputs) == 0 {
			continue
		}
		target := platformcontract.Target{Platform: contract.Platform, OutputType: outputs[0], ContractID: contract.ID, ContractVersion: contract.Version}
		if contract.Platform == "blog" {
			target.IsCanonical = true
			blog = target
			targets = append(targets, target)
			continue
		}
		requiredContext := rawStringList(contract.RequiredContextFields)
		if len(requiredContext) > 0 {
			contextRow, ok := currentContext[contract.Platform]
			if !ok {
				continue
			}
			version := contextRow.Version
			target.TargetKey = contextRow.TargetKey
			target.TargetContextID = contextRow.ID
			target.TargetContextVersion = &version
		} else if !connectionReady[contract.Platform] {
			continue
		}
		targets = append(targets, target)
	}
	if blog.ContractID == uuid.Nil {
		return growthspec.TargetSpec{}
	}
	return growthspec.TargetSpec{CanonicalTarget: blog, TargetPlatforms: targets, SelectionMode: "contract_matrix"}
}

func additionalOutputTypes(target growthspec.TargetSpec) int {
	canonical := target.CanonicalTarget.OutputType
	seen := map[string]struct{}{}
	for _, item := range target.TargetPlatforms {
		if item.IsCanonical || item.OutputType == "" || item.OutputType == canonical {
			continue
		}
		seen[item.OutputType] = struct{}{}
	}
	return len(seen)
}

func growthIntentMapping(raw, gapType string) (intent, journey, conversion string, supported bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "comparison", "category_recommendation":
		return "comparison", "decision", "", true
	case "alternative":
		return "alternative", "decision", "", true
	case "integration":
		return "integration", "consideration", "", true
	case "buyer_intent", "transactional":
		return "transactional", "decision", "", true
	case "problem_solution", "problem_solving", "how_to":
		return "problem_solving", "consideration", "", true
	case "workflow", "use_case":
		return "use_case", "consideration", "", true
	case "definition_entity", "informational", "glossary":
		return "informational", "awareness", "", true
	case "template":
		return "template", "consideration", "", true
	}
	if gapType == "geo_competitor_cited_project_absent" {
		return "comparison", "decision", "", true
	}
	return "", "", "", false
}

func confirmedProfileMappings(profile json.RawMessage, topic, audience string, evidence growthradar.EvidenceIndex) (bool, bool) {
	classification := growthradar.ClassifyContext(profile, evidence)
	capabilityConfirmed, audienceConfirmed := false, false
	for _, term := range classification.Terms {
		if !term.Accepted {
			continue
		}
		switch term.Class {
		case "public_capability", "public_evidence":
			capabilityConfirmed = capabilityConfirmed || deterministicPhraseMatch(topic, term.Value)
		case "audience":
			audienceConfirmed = audienceConfirmed || deterministicPhraseMatch(audience, term.Value)
		}
	}
	return capabilityConfirmed, audienceConfirmed
}

func evidenceIndexForGap(gap geoGap, now time.Time) growthradar.EvidenceIndex {
	if _, _, qualified := qualifiedObservationEvidence(gap.Evidence, claimTypeForGap(gap.Type), now); !qualified {
		return growthradar.EvidenceIndex{}
	}
	topic := strings.TrimSpace(gap.TargetTopic)
	if topic == "" || growthradar.ContainsInternalSensitiveTerm(topic) {
		return growthradar.EvidenceIndex{}
	}
	return growthradar.EvidenceIndex{PublicTerms: []string{topic}}
}

func deterministicPhraseMatch(candidate, confirmed string) bool {
	candidate, confirmed = normalizedIdentity(candidate), normalizedIdentity(confirmed)
	if candidate == "" || confirmed == "" {
		return false
	}
	return candidate == confirmed || strings.Contains(" "+candidate+" ", " "+confirmed+" ") || strings.Contains(" "+confirmed+" ", " "+candidate+" ")
}

func qualifiedObservationEvidence(evidence map[string]any, claimType string, now time.Time) (growthradar.EvidenceSource, *int, bool) {
	if textEvidence(evidence, "source") == "competitive_seed_url" || textEvidence(evidence, "source_type") == "competitive_seed_url" {
		if textEvidence(evidence, "seed_url") == "" || textEvidence(evidence, "competitor_domain") == "" || textEvidence(evidence, "archetype") == "" {
			return growthradar.EvidenceSource{}, nil, false
		}
		supportedClaim := "competitive_archetype"
		if evidence["competitive_domain_diversity"] == true && int32FromEvidence(evidence["competitor_domain_count"], 1) > 1 {
			supportedClaim = "cross_domain_competitive_topic"
		}
		source := growthradar.EvidenceSource{Class: "competitive_seed_url", Qualified: true, CompleteProvenance: true, SupportedClaim: supportedClaim}
		return source, nil, true
	}
	if textEvidence(evidence, "source_type") != SourceTypeAnswerEngine || textEvidence(evidence, "observation_state") != "observed" || textEvidence(evidence, "observation_id") == "" {
		return growthradar.EvidenceSource{}, nil, false
	}
	if claimType == "citation" && !nonEmptyEvidenceList(evidence["cited_urls"]) && !nonEmptyEvidenceList(evidence["competitor_citations"]) {
		return growthradar.EvidenceSource{}, nil, false
	}
	complete := textEvidence(evidence, "engine") != "" && textEvidence(evidence, "observed_at") != "" && textEvidence(evidence, "answer_hash") != ""
	source := growthradar.EvidenceSource{Class: "answer_engine_observation", Qualified: true, CompleteProvenance: complete, SupportedClaim: claimType}
	observedAt, err := time.Parse(time.RFC3339, textEvidence(evidence, "observed_at"))
	if err != nil || observedAt.After(now) {
		return source, nil, true
	}
	age := int(now.Sub(observedAt).Hours() / 24)
	return source, &age, true
}

func claimTypeForGap(gapType string) string {
	if gapType == "geo_competitor_cited_project_absent" || gapType == "geo_project_mentioned_without_citation" {
		return "citation"
	}
	return "absence"
}

func nonEmptyEvidenceList(value any) bool {
	switch list := value.(type) {
	case []string:
		return len(list) > 0
	case []any:
		return len(list) > 0
	case json.RawMessage:
		var decoded []any
		return json.Unmarshal(list, &decoded) == nil && len(decoded) > 0
	default:
		return false
	}
}

func textEvidence(evidence map[string]any, key string) string {
	if value, ok := evidence[key].(string); ok {
		return strings.TrimSpace(value)
	}
	if value, ok := evidence[key].(uuid.UUID); ok {
		return value.String()
	}
	return ""
}

func demandMaterialChange(current, previous int) string {
	if previous < 10 && current >= 10 {
		return "new_query"
	}
	if previous < 10 {
		return "unchanged"
	}
	growth := float64(current-previous) / float64(previous)
	if growth > 1 {
		return "growth_over_100"
	}
	if growth > .25 {
		return "growth_25_100"
	}
	if growth <= -.25 {
		return "decline_over_25"
	}
	return "unchanged"
}

func onlyAnswerEngineEvidence(sources []growthradar.EvidenceSource) bool {
	qualified := 0
	answer := false
	for _, source := range sources {
		if !source.Qualified {
			continue
		}
		qualified++
		answer = answer || source.Class == "answer_engine_observation"
	}
	return qualified == 1 && answer
}

func rawStringList(raw json.RawMessage) []string {
	var values []string
	_ = json.Unmarshal(raw, &values)
	return values
}

func jsonStringListContains(raw json.RawMessage, wanted string) bool {
	canonical, ok := platformcontract.CanonicalAssetType(wanted)
	if ok {
		wanted = canonical
	}
	for _, value := range rawStringList(raw) {
		if value == wanted {
			return true
		}
	}
	return false
}

func sameNormalizedText(left, right string) bool {
	return normalizedIdentity(left) == normalizedIdentity(right)
}
func nearNormalizedText(left, right string) bool {
	leftSet, rightSet := map[string]struct{}{}, map[string]struct{}{}
	for _, token := range strings.Fields(normalizedIdentity(left)) {
		leftSet[token] = struct{}{}
	}
	for _, token := range strings.Fields(normalizedIdentity(right)) {
		rightSet[token] = struct{}{}
	}
	if len(leftSet) < 3 || len(rightSet) < 3 {
		return false
	}
	intersection := 0
	for token := range leftSet {
		if _, ok := rightSet[token]; ok {
			intersection++
		}
	}
	union := len(leftSet) + len(rightSet) - intersection
	return union > 0 && float64(intersection)/float64(union) >= .8
}
func normalizedIdentity(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}
func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
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
	answerMaterial := strings.Join([]string{observation.AnswerSummary, string(observation.EvidenceSnippets), string(observation.CitedUrls), string(observation.CompetitorCitations)}, "\x00")
	answerHash := fmt.Sprintf("%x", sha256.Sum256([]byte(answerMaterial)))
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
		"answer_hash":            answerHash,
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
	if gap.Type == "competitive_tools_hub_gap" {
		return append([]string{"seed URL crawl facts", "tools hub archetype evidence", "supported product claims"}, extras...)
	}
	if gap.Type == "geo_competitor_cited_project_absent" {
		return append([]string{"first-party comparison criteria", "supported product claims", "competitor citation evidence"}, extras...)
	}
	return append([]string{"self-contained definition or evidence block", "supported product claims", "extractable citation snippet"}, extras...)
}

func outlineForGap(gap geoGap) []string {
	outline := []string{
		fmt.Sprintf("Answer the prompt: %s", gap.PromptText),
		fmt.Sprintf("Explain %s with evidence", gap.TargetTopic),
		"Show cited sources and supported product claims",
		"Add internal links from related canonical pages",
	}
	if textEvidence(gap.Evidence, "source_type") == "competitive_seed_url" {
		if seedURLs := stringValues(gap.Evidence["seed_url_samples"]); len(seedURLs) > 1 {
			outline = append([]string{"Use the grouped competitor seed URLs as references, but create a project-specific resource."}, outline...)
		} else if seedURL := textEvidence(gap.Evidence, "seed_url"); seedURL != "" {
			outline = append([]string{"Use " + seedURL + " as the competitor reference, but create a project-specific resource."}, outline...)
		}
		if domains := stringValues(gap.Evidence["competitor_domain_samples"]); len(domains) > 1 {
			outline = append(outline, "Compare patterns across competitor examples from "+strings.Join(domains, ", ")+" before recommending the project-specific resource.")
		}
		if archetype := textEvidence(gap.Evidence, "archetype"); archetype != "" {
			outline = append(outline, fmt.Sprintf("Explain why this project should answer the %s opportunity for %s.", archetype, gap.TargetTopic))
		}
	}
	return outline
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
	if textEvidence(evidence, "source_type") == "competitive_seed_url" {
		if seedURL := textEvidence(evidence, "seed_url"); seedURL != "" {
			out = append(out, "competitor seed URL: "+seedURL)
		}
		if seedURLs := stringValues(evidence["seed_url_samples"]); len(seedURLs) > 1 {
			out = append(out, "competitor seed URL samples: "+strings.Join(seedURLs, ", "))
		}
		if domains := stringValues(evidence["competitor_domain_samples"]); len(domains) > 1 {
			out = append(out, "competitor domains supporting this topic: "+strings.Join(domains, ", "))
		}
		if fromURL := textEvidence(evidence, "discovered_from_url"); fromURL != "" {
			out = append(out, "auto-discovered from: "+fromURL)
		}
		if archetype := textEvidence(evidence, "archetype"); archetype != "" {
			out = append(out, "competitive archetype: "+archetype)
		}
	}
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
