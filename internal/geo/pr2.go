package geo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/growthradar"
	"github.com/citeloop/citeloop/internal/pgutil"
	seopkg "github.com/citeloop/citeloop/internal/seo"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	AgentPromptBuilder     = "geo_prompt_builder"
	AgentObserver          = "geo_observer"
	ProviderDeterministic  = "citeloop_deterministic"
	ProviderManualFixture  = "manual_fixture"
	DefaultPromptSetStatus = "active"
	DefaultLocale          = "en-US"
)

type GeneratePromptSetRequest struct {
	Name          string   `json:"name,omitempty"`
	Locale        string   `json:"locale,omitempty"`
	Status        string   `json:"status,omitempty"`
	TargetEngines []string `json:"target_engines,omitempty"`
}

type GeneratePromptSetResult struct {
	Run              db.GeoRun               `json:"run"`
	PromptSet        db.GeoPromptSet         `json:"prompt_set"`
	Prompts          []db.GeoPrompt          `json:"prompts"`
	Competitors      []db.GeoCompetitor      `json:"competitors"`
	ExternalSurfaces []db.GeoExternalSurface `json:"external_surfaces"`
	DataSourceNotes  []string                `json:"data_source_notes"`
}

type ImportManualFixtureRequest struct {
	Engine       string                          `json:"engine"`
	Locale       string                          `json:"locale,omitempty"`
	Observations []ManualFixtureObservationInput `json:"observations"`
}

type ManualFixtureObservationInput struct {
	PromptID            uuid.UUID `json:"prompt_id"`
	AnswerSummary       string    `json:"answer_summary,omitempty"`
	CitedURLs           []string  `json:"cited_urls,omitempty"`
	BrandMentioned      bool      `json:"brand_mentioned"`
	BrandPosition       *int32    `json:"brand_position,omitempty"`
	CompetitorMentions  []string  `json:"competitor_mentions,omitempty"`
	CompetitorCitations []string  `json:"competitor_citations,omitempty"`
	EvidenceSnippets    []string  `json:"evidence_snippets,omitempty"`
	ProjectCitationRank int32     `json:"project_citation_rank,omitempty"`
	Confidence          string    `json:"confidence,omitempty"`
}

type ImportManualFixtureResult struct {
	Run             db.GeoRun             `json:"run"`
	Observations    []db.GeoObservation   `json:"observations"`
	Score           db.GeoVisibilityScore `json:"score"`
	DataSourceNotes []string              `json:"data_source_notes"`
}

type profileFields struct {
	Positioning     string
	ValueProps      []string
	Features        []string
	ICP             []string
	KeyTerms        []string
	Competitors     []string
	Differentiators []string
}

type promptSpec struct {
	Text          string
	IntentType    string
	TargetPersona string
	TargetTopic   string
	Priority      int32
	Source        string
}

func (s Service) GeneratePromptSet(ctx context.Context, projectID uuid.UUID, req GeneratePromptSetRequest) (GeneratePromptSetResult, error) {
	now := s.now()
	run, err := s.Q.StartGEORun(ctx, db.StartGEORunParams{
		ProjectID: projectID,
		Agent:     AgentPromptBuilder,
		Provider:  ProviderDeterministic,
		StartedAt: pgutil.TS(now),
		Input:     jsonBytes(req),
	})
	if err != nil {
		return GeneratePromptSetResult{}, err
	}
	result := GeneratePromptSetResult{Run: run, DataSourceNotes: []string{"deterministic_profile_prompt_builder"}}
	finish := func(status string, output any, runErr error) (GeneratePromptSetResult, error) {
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

	profile, err := s.Q.GetActiveProfile(ctx, projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		result.DataSourceNotes = append(result.DataSourceNotes, "missing_active_profile")
		return finish("degraded", result, nil)
	}
	if err != nil {
		return finish("error", result, err)
	}
	fields := parseProfileFields(profile.Profile)
	classification := growthradar.ClassifyContext(profile.Profile, growthradar.EvidenceIndex{})
	accepted := make(map[string]struct{}, len(classification.AcceptedVocabulary))
	for _, value := range classification.AcceptedVocabulary {
		accepted[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	fields.ValueProps = acceptedProfileValues(fields.ValueProps, accepted)
	fields.Features = acceptedProfileValues(fields.Features, accepted)
	fields.ICP = acceptedProfileValues(fields.ICP, accepted)
	fields.KeyTerms = acceptedProfileValues(fields.KeyTerms, accepted)
	fields.Competitors = classification.ConfirmedCompetitors
	if _, ok := accepted[strings.ToLower(strings.TrimSpace(fields.Positioning))]; !ok {
		fields.Positioning = ""
	}
	topics, err := s.Q.ListTopics(ctx, projectID)
	if err != nil {
		return finish("error", result, err)
	}
	property, err := s.Q.GetSEOPropertyForProject(ctx, projectID)
	if err == nil && strings.TrimSpace(property.SiteUrl) != "" {
		surface, err := s.upsertProjectSurface(ctx, projectID, property.SiteUrl)
		if err != nil {
			return finish("error", result, err)
		}
		result.ExternalSurfaces = append(result.ExternalSurfaces, surface)
	}

	locale := strings.TrimSpace(req.Locale)
	if locale == "" {
		locale = DefaultLocale
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = DefaultPromptSetStatus
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "GEO prompt set"
	}
	promptSet, err := s.Q.CreateGEOPromptSet(ctx, db.CreateGEOPromptSetParams{
		ProjectID:      projectID,
		Name:           name,
		Status:         status,
		Locale:         locale,
		CreatedByRunID: uuidToPG(run.ID),
	})
	if err != nil {
		return finish("error", result, err)
	}
	result.PromptSet = promptSet

	for _, competitor := range fields.Competitors {
		row, err := s.Q.UpsertGEOCompetitor(ctx, db.UpsertGEOCompetitorParams{
			ProjectID: projectID,
			Name:      competitor,
			Domains:   jsonBytes([]string{}),
			Aliases:   jsonBytes([]string{competitor}),
			Source:    "profile",
			Status:    "active",
		})
		if err != nil {
			return finish("error", result, err)
		}
		result.Competitors = append(result.Competitors, row)
	}

	engines := req.TargetEngines
	if len(engines) == 0 {
		engines = []string{"ChatGPT", "Perplexity", "Google AI Mode"}
	}
	specs := buildPromptSpecs(fields, topics)
	for _, spec := range specs {
		prompt, err := s.Q.CreateGEOPrompt(ctx, db.CreateGEOPromptParams{
			ProjectID:     projectID,
			PromptSetID:   promptSet.ID,
			PromptText:    spec.Text,
			IntentType:    spec.IntentType,
			TargetPersona: spec.TargetPersona,
			TargetTopic:   spec.TargetTopic,
			Locale:        locale,
			TargetEngines: jsonBytes(engines),
			Priority:      spec.Priority,
			Source:        spec.Source,
			Status:        "active",
		})
		if err != nil {
			return finish("error", result, err)
		}
		result.Prompts = append(result.Prompts, prompt)
	}

	return finish("ok", result, nil)
}

func (s Service) ImportManualFixtureObservations(ctx context.Context, projectID uuid.UUID, req ImportManualFixtureRequest) (ImportManualFixtureResult, error) {
	now := s.now()
	run, err := s.Q.StartGEORun(ctx, db.StartGEORunParams{
		ProjectID: projectID,
		Agent:     AgentObserver,
		Provider:  ProviderManualFixture,
		StartedAt: pgutil.TS(now),
		Input:     jsonBytes(req),
	})
	if err != nil {
		return ImportManualFixtureResult{}, err
	}
	result := ImportManualFixtureResult{Run: run, DataSourceNotes: []string{"manual_fixture_observations"}}
	finish := func(status string, output any, runErr error) (ImportManualFixtureResult, error) {
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

	engine := strings.TrimSpace(req.Engine)
	if engine == "" {
		engine = "manual"
	}
	locale := strings.TrimSpace(req.Locale)
	if locale == "" {
		locale = DefaultLocale
	}
	ownedSurfaces, err := s.Q.ListProjectOwnedGEOExternalSurfaces(ctx, projectID)
	if err != nil {
		return finish("error", result, err)
	}
	for _, input := range req.Observations {
		projectURLs, surfaceIDs := projectCitations(input.CitedURLs, ownedSurfaces)
		projectCitationCount := int32(len(projectURLs))
		if err := s.touchCitedSurfaces(ctx, projectID, ownedSurfaces, surfaceIDs, now); err != nil {
			return finish("error", result, err)
		}
		observation, err := s.Q.CreateGEOObservation(ctx, db.CreateGEOObservationParams{
			ProjectID:               projectID,
			RunID:                   run.ID,
			PromptID:                uuidToPG(input.PromptID),
			Engine:                  engine,
			Locale:                  locale,
			SourceType:              ProviderManualFixture,
			BrandMentioned:          input.BrandMentioned,
			BrandPosition:           input.BrandPosition,
			ProjectCitationCount:    projectCitationCount,
			ProjectCitationRankBest: projectCitationRank(input, projectCitationCount),
			ProjectCitedSurfaceIds:  jsonBytes(surfaceIDs),
			CitedUrls:               jsonBytes(input.CitedURLs),
			CompetitorMentions:      jsonBytes(input.CompetitorMentions),
			CompetitorCitations:     jsonBytes(input.CompetitorCitations),
			ObservationState:        "observed",
			AnswerSummary:           strings.TrimSpace(input.AnswerSummary),
			EvidenceSnippets:        jsonBytes(input.EvidenceSnippets),
			Confidence:              observationConfidence(input.Confidence),
			ObservedAt:              pgutil.TS(now),
		})
		if err != nil {
			return finish("error", result, err)
		}
		result.Observations = append(result.Observations, observation)
	}
	score, err := s.scoreObservations(ctx, projectID, run.ID, result.Observations, pgutil.TS(now))
	if err != nil {
		return finish("error", result, err)
	}
	result.Score = score
	status := "ok"
	if len(result.Observations) == 0 {
		status = "degraded"
	}
	return finish(status, result, nil)
}

func (s Service) touchCitedSurfaces(ctx context.Context, projectID uuid.UUID, surfaces []db.GeoExternalSurface, citedSurfaceIDs []string, citedAt time.Time) error {
	if len(citedSurfaceIDs) == 0 {
		return nil
	}
	cited := map[string]struct{}{}
	for _, id := range citedSurfaceIDs {
		cited[id] = struct{}{}
	}
	for _, surface := range surfaces {
		if _, ok := cited[surface.ID.String()]; !ok {
			continue
		}
		backlinkState := surface.BacklinkState
		if strings.TrimSpace(backlinkState) == "" {
			backlinkState = "unknown"
		}
		if _, err := s.Q.UpsertGEOExternalSurface(ctx, db.UpsertGEOExternalSurfaceParams{
			ProjectID:          projectID,
			Url:                surface.Url,
			NormalizedUrl:      surface.NormalizedUrl,
			Platform:           surface.Platform,
			SurfaceType:        surface.SurfaceType,
			OwnerType:          surface.OwnerType,
			CanonicalTargetUrl: surface.CanonicalTargetUrl,
			BacklinkState:      backlinkState,
			LastHttpStatus:     surface.LastHttpStatus,
			LastCitedAt:        pgutil.TS(citedAt),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s Service) scoreObservations(ctx context.Context, projectID, runID uuid.UUID, observations []db.GeoObservation, computedAt pgtype.Timestamptz) (db.GeoVisibilityScore, error) {
	activePrompts, err := s.Q.ListActiveGEOPrompts(ctx, projectID)
	if err != nil {
		return db.GeoVisibilityScore{}, err
	}
	crawlerSnapshots, err := s.Q.ListLatestAICrawlerAccessSnapshots(ctx, projectID)
	if err != nil {
		return db.GeoVisibilityScore{}, err
	}
	projectSurfaces, err := s.Q.ListProjectOwnedGEOExternalSurfaces(ctx, projectID)
	if err != nil {
		return db.GeoVisibilityScore{}, err
	}
	observed := observedOnly(observations)
	totalPrompts := len(activePrompts)
	if totalPrompts == 0 {
		totalPrompts = uniqueObservedPromptCount(observed)
	}
	observedPrompts := uniqueObservedPromptCount(observed)
	engines := map[string]struct{}{}
	brandMentioned := 0
	projectCited := 0
	competitorGap := 0
	for _, observation := range observed {
		engines[observation.Engine] = struct{}{}
		if observation.BrandMentioned {
			brandMentioned++
		}
		if observation.ProjectCitationCount > 0 {
			projectCited++
		}
		if jsonArrayLen(observation.CompetitorCitations) > 0 && observation.ProjectCitationCount == 0 {
			competitorGap++
		}
	}
	coverage := ratio(observedPrompts, totalPrompts)
	brandRate := ratio(brandMentioned, len(observed))
	citationRate := ratio(projectCited, len(observed))
	competitorGapRate := ratio(competitorGap, len(observed))
	crawlerHealth := crawlerAccessHealth(crawlerSnapshots)
	citationRankScore := citationRankScore(observed)
	externalSurfaceCoverage := externalSurfaceCoverage(projectSurfaces)
	scoreValue := 20*crawlerHealth + 20*brandRate + 25*citationRate + 15*citationRankScore + 10*(1-competitorGapRate) + 10*externalSurfaceCoverage
	confidence := scoreConfidence(observedPrompts, coverage)
	breakdown := map[string]any{
		"coverage":                  coverage,
		"crawler_access_health":     crawlerHealth,
		"brand_mention_rate":        brandRate,
		"project_citation_rate":     citationRate,
		"citation_rank_score":       citationRankScore,
		"competitor_gap_rate":       competitorGapRate,
		"external_surface_coverage": externalSurfaceCoverage,
		"observed_count":            len(observed),
		"excluded_count":            len(observations) - len(observed),
		"scoring_version":           "geo_pr6_v2",
		"weights": map[string]float64{
			"crawler_access_health":     20,
			"brand_mention_rate":        20,
			"project_citation_rate":     25,
			"citation_rank_score":       15,
			"competitor_gap_inverse":    10,
			"external_surface_coverage": 10,
		},
		"insufficient_data": confidence == "insufficient_data" || confidence == "low",
	}
	return s.Q.CreateGEOVisibilityScore(ctx, db.CreateGEOVisibilityScoreParams{
		ProjectID:           projectID,
		RunID:               uuidToPG(runID),
		Score:               pgutil.Numeric(scoreValue),
		Coverage:            pgutil.Numeric(coverage),
		Confidence:          confidence,
		Breakdown:           jsonBytes(breakdown),
		PromptCountTotal:    int32(totalPrompts),
		PromptCountObserved: int32(observedPrompts),
		EngineCountObserved: int32(len(engines)),
		ComputedAt:          computedAt,
	})
}

func observedOnly(observations []db.GeoObservation) []db.GeoObservation {
	out := make([]db.GeoObservation, 0, len(observations))
	for _, observation := range observations {
		if observation.ObservationState == ObservationStateObserved || observation.ObservationState == "" {
			out = append(out, observation)
		}
	}
	return out
}

func crawlerAccessHealth(snapshots []db.AiCrawlerAccessSnapshot) float64 {
	if len(snapshots) == 0 {
		return 0.5
	}
	total := 0.0
	for _, snapshot := range snapshots {
		if snapshot.RobotsState == string(RobotsDisallowed) || snapshot.AccessState == AccessBlocked || snapshot.AccessState == AccessError {
			continue
		}
		if snapshot.AccessState == AccessChallenge || snapshot.AccessState == AccessRateLimited || snapshot.AccessState == AccessTimeout {
			total += 0.4
			continue
		}
		total += 1
	}
	return total / float64(len(snapshots))
}

func citationRankScore(observations []db.GeoObservation) float64 {
	total := 0.0
	count := 0
	for _, observation := range observations {
		if observation.ProjectCitationCount == 0 {
			continue
		}
		count++
		if observation.ProjectCitationRankBest == nil || *observation.ProjectCitationRankBest <= 0 {
			total += 0.5
			continue
		}
		rank := *observation.ProjectCitationRankBest
		if rank == 1 {
			total += 1
			continue
		}
		total += 1 / float64(rank)
	}
	return ratioFloat(total, count)
}

func externalSurfaceCoverage(surfaces []db.GeoExternalSurface) float64 {
	if len(surfaces) == 0 {
		return 0
	}
	covered := 0
	for _, surface := range surfaces {
		if surface.LastCitedAt.Valid {
			covered++
			continue
		}
		if surface.LastHttpStatus != nil && *surface.LastHttpStatus >= 200 && *surface.LastHttpStatus < 400 {
			covered++
		}
	}
	return ratio(covered, len(surfaces))
}

func (s Service) upsertProjectSurface(ctx context.Context, projectID uuid.UUID, rawURL string) (db.GeoExternalSurface, error) {
	normalized, err := seopkg.NormalizeURL(rawURL, rawURL, seopkg.URLNormalizationConfig{})
	if err != nil {
		normalized = rawURL
	}
	platform := "site"
	if parsed, err := url.Parse(normalized); err == nil && parsed.Host != "" {
		platform = parsed.Host
	}
	return s.Q.UpsertGEOExternalSurface(ctx, db.UpsertGEOExternalSurfaceParams{
		ProjectID:          projectID,
		Url:                rawURL,
		NormalizedUrl:      normalized,
		Platform:           platform,
		SurfaceType:        "domain",
		OwnerType:          "project",
		CanonicalTargetUrl: nil,
		BacklinkState:      "unknown",
		LastHttpStatus:     nil,
		LastCitedAt:        pgtype.Timestamptz{},
	})
}

func parseProfileFields(raw json.RawMessage) profileFields {
	var data map[string]any
	_ = json.Unmarshal(raw, &data)
	fields := profileFields{
		Positioning:     stringValue(data["positioning"]),
		ValueProps:      stringList(data["value_props"]),
		Features:        stringList(data["features"]),
		ICP:             stringList(data["icp"]),
		KeyTerms:        stringList(data["key_terms"]),
		Competitors:     stringList(data["competitors"]),
		Differentiators: stringList(data["differentiators"]),
	}
	fields.Features = fallbackList(fields.Features, "product workflow")
	fields.ICP = fallbackList(fields.ICP, "buyer")
	fields.KeyTerms = fallbackList(fields.KeyTerms, "product")
	return fields
}

func acceptedProfileValues(values []string, accepted map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := accepted[strings.ToLower(strings.TrimSpace(value))]; ok {
			result = append(result, value)
		}
	}
	return result
}

func buildPromptSpecs(fields profileFields, topics []db.Topic) []promptSpec {
	targets := targetTopics(fields, topics)
	personas := fallbackList(fields.ICP, "buyer")
	competitors := fallbackList(fields.Competitors, "existing tool")
	specs := make([]promptSpec, 0, 40)
	for _, topic := range targets {
		for _, persona := range personas {
			specs = append(specs,
				promptSpec{Text: fmt.Sprintf("best tools for %s for %s", topic, persona), IntentType: "category_recommendation", TargetPersona: persona, TargetTopic: topic, Priority: 8, Source: "profile"},
				promptSpec{Text: fmt.Sprintf("how to solve %s for %s", topic, persona), IntentType: "problem_solution", TargetPersona: persona, TargetTopic: topic, Priority: 7, Source: "profile"},
				promptSpec{Text: fmt.Sprintf("how to automate %s for %s", topic, persona), IntentType: "workflow", TargetPersona: persona, TargetTopic: topic, Priority: 7, Source: "profile"},
				promptSpec{Text: fmt.Sprintf("tools that integrate with %s workflows", topic), IntentType: "integration", TargetPersona: persona, TargetTopic: topic, Priority: 6, Source: "profile"},
				promptSpec{Text: fmt.Sprintf("which product should I use for %s", topic), IntentType: "buyer_intent", TargetPersona: persona, TargetTopic: topic, Priority: 8, Source: "profile"},
				promptSpec{Text: fmt.Sprintf("what is %s", topic), IntentType: "definition_entity", TargetPersona: persona, TargetTopic: topic, Priority: 5, Source: "profile"},
			)
			for _, competitor := range competitors {
				specs = append(specs,
					promptSpec{Text: fmt.Sprintf("%s vs %s for %s", fieldsProductName(fields), competitor, topic), IntentType: "comparison", TargetPersona: persona, TargetTopic: topic, Priority: 8, Source: "competitor"},
					promptSpec{Text: fmt.Sprintf("alternatives to %s for %s", competitor, topic), IntentType: "alternative", TargetPersona: persona, TargetTopic: topic, Priority: 8, Source: "competitor"},
				)
			}
		}
	}
	return uniquePromptSpecs(specs, 30)
}

func targetTopics(fields profileFields, topics []db.Topic) []string {
	values := make([]string, 0, len(fields.KeyTerms)+len(fields.Features)+len(topics))
	values = append(values, fields.KeyTerms...)
	values = append(values, fields.Features...)
	for _, topic := range topics {
		if topic.TargetKeyword != nil {
			values = append(values, *topic.TargetKeyword)
		}
		if topic.TargetPrompt != nil {
			values = append(values, *topic.TargetPrompt)
		}
		if strings.TrimSpace(topic.Title) != "" {
			values = append(values, topic.Title)
		}
	}
	publicValues := make([]string, 0, len(values))
	for _, value := range uniqueStrings(values) {
		if !growthradar.ContainsInternalSensitiveTerm(value) {
			publicValues = append(publicValues, value)
		}
	}
	values = fallbackList(publicValues, "product workflow")
	sort.Strings(values)
	return values
}

func uniquePromptSpecs(specs []promptSpec, min int) []promptSpec {
	seen := map[string]struct{}{}
	out := make([]promptSpec, 0, len(specs))
	for _, spec := range specs {
		key := strings.ToLower(strings.TrimSpace(spec.Text))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, spec)
	}
	for len(out) < min && len(out) > 0 {
		base := out[len(out)%len(out)]
		base.Text = fmt.Sprintf("%s for %s", base.Text, fallbackPersona(len(out)))
		base.TargetPersona = fallbackPersona(len(out))
		out = append(out, base)
	}
	return out
}

func fieldsProductName(fields profileFields) string {
	text := strings.TrimSpace(fields.Positioning)
	if text == "" {
		return "this product"
	}
	first := strings.Fields(text)
	if len(first) == 0 {
		return "this product"
	}
	return strings.Trim(first[0], ".,:;")
}

func fallbackPersona(i int) string {
	personas := []string{"ops teams", "growth teams", "founders", "content teams", "developers"}
	return personas[i%len(personas)]
}

func fallbackList(values []string, fallback string) []string {
	values = uniqueStrings(values)
	if len(values) == 0 {
		return []string{fallback}
	}
	return values
}

func stringValue(value any) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func stringList(value any) []string {
	switch v := value.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := stringValue(item); s != "" {
				out = append(out, s)
			}
		}
		return uniqueStrings(out)
	case []string:
		return uniqueStrings(v)
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{strings.TrimSpace(v)}
	default:
		return nil
	}
}

func projectCitations(citedURLs []string, surfaces []db.GeoExternalSurface) ([]string, []string) {
	projectURLs := []string{}
	surfaceIDs := []string{}
	for _, cited := range uniqueStrings(citedURLs) {
		for _, surface := range surfaces {
			if surface.OwnerType != "project" || !surfaceMatchesURL(surface, cited) {
				continue
			}
			projectURLs = append(projectURLs, cited)
			surfaceIDs = append(surfaceIDs, surface.ID.String())
			break
		}
	}
	return uniqueStrings(projectURLs), uniqueStrings(surfaceIDs)
}

func surfaceMatchesURL(surface db.GeoExternalSurface, citedURL string) bool {
	cited, err := url.Parse(strings.TrimSpace(citedURL))
	if err != nil || cited.Host == "" {
		return false
	}
	surfaceURL, err := url.Parse(strings.TrimSpace(surface.NormalizedUrl))
	if err != nil || surfaceURL.Host == "" {
		return false
	}
	if sameDomainHost(cited.Host, surfaceURL.Host) {
		if surface.SurfaceType == "domain" || surfaceURL.Path == "" || surfaceURL.Path == "/" {
			return true
		}
		return strings.HasPrefix(strings.TrimRight(cited.Path, "/")+"/", strings.TrimRight(surfaceURL.Path, "/")+"/")
	}
	return false
}

func sameDomainHost(a, b string) bool {
	return strings.EqualFold(strings.TrimPrefix(a, "www."), strings.TrimPrefix(b, "www."))
}

func projectCitationRank(input ManualFixtureObservationInput, projectCitationCount int32) *int32 {
	if projectCitationCount == 0 {
		return nil
	}
	if input.ProjectCitationRank > 0 {
		rank := input.ProjectCitationRank
		return &rank
	}
	rank := int32(1)
	return &rank
}

func observationConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ConfidenceHigh:
		return ConfidenceHigh
	case ConfidenceLow:
		return ConfidenceLow
	default:
		return ConfidenceMedium
	}
}

func uniqueObservedPromptCount(observations []db.GeoObservation) int {
	seen := map[uuid.UUID]struct{}{}
	for _, observation := range observations {
		if !observation.PromptID.Valid {
			continue
		}
		seen[uuid.UUID(observation.PromptID.Bytes)] = struct{}{}
	}
	return len(seen)
}

func ratio(numerator, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func ratioFloat(numerator float64, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return numerator / float64(denominator)
}

func scoreConfidence(observedPrompts int, coverage float64) string {
	switch {
	case observedPrompts == 0:
		return "insufficient_data"
	case observedPrompts < 10:
		return ConfidenceLow
	case observedPrompts >= 30 && coverage >= 0.7:
		return ConfidenceHigh
	default:
		return ConfidenceMedium
	}
}

func uuidToPG(value uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: value, Valid: value != uuid.Nil}
}
