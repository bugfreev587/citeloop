package geo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	ProviderPerplexitySonar             = "perplexity_sonar"
	SourceTypeAnswerEngine              = "answer_engine"
	ObservationStateObserved            = "observed"
	ObservationStateProviderUnavailable = "provider_unavailable"
	DefaultPerplexityBaseURL            = "https://api.perplexity.ai"
	DefaultPerplexityModel              = "sonar-pro"
)

type AnswerProvider interface {
	Name() string
	Available() bool
	Observe(ctx context.Context, prompts []db.GeoPrompt) ([]ProviderObservation, float64, error)
}

type ProviderObservation struct {
	PromptID            uuid.UUID
	Engine              string
	Locale              string
	AnswerSummary       string
	CitedURLs           []string
	BrandMentioned      bool
	BrandPosition       *int32
	CompetitorMentions  []string
	CompetitorCitations []string
	EvidenceSnippets    []string
	Confidence          string
	CostUSD             float64
}

type ObserveAnswerProviderRequest struct {
	Engine     string  `json:"engine,omitempty"`
	Locale     string  `json:"locale,omitempty"`
	MaxPrompts int32   `json:"max_prompts,omitempty"`
	BudgetUSD  float64 `json:"budget_usd,omitempty"`
}

type ObserveAnswerProviderResult struct {
	Run             db.GeoRun             `json:"run"`
	Observations    []db.GeoObservation   `json:"observations"`
	Score           db.GeoVisibilityScore `json:"score,omitempty"`
	CostUSD         float64               `json:"cost_usd"`
	SkippedPrompts  []SkippedPrompt       `json:"skipped_prompts"`
	SkippedEngines  []string              `json:"skipped_engines"`
	DataSourceNotes []string              `json:"data_source_notes"`
}

type SkippedPrompt struct {
	PromptID   uuid.UUID `json:"prompt_id"`
	PromptText string    `json:"prompt_text"`
	Engine     string    `json:"engine"`
	Reason     string    `json:"reason"`
}

func (s Service) ObserveAnswerProvider(ctx context.Context, projectID uuid.UUID, req ObserveAnswerProviderRequest) (ObserveAnswerProviderResult, error) {
	now := s.now()
	providerName := "provider_unavailable"
	if s.AnswerProvider != nil && s.AnswerProvider.Name() != "" {
		providerName = s.AnswerProvider.Name()
	}
	req.Engine = AnswerProviderEngine(s.AnswerProvider, req.Engine)
	run, err := s.Q.StartGEORun(ctx, db.StartGEORunParams{
		ProjectID: projectID,
		Agent:     AgentObserver,
		Provider:  providerName,
		StartedAt: pgutil.TS(now),
		Input:     jsonBytes(req),
	})
	if err != nil {
		return ObserveAnswerProviderResult{}, err
	}
	result := ObserveAnswerProviderResult{
		Run:             run,
		DataSourceNotes: []string{"answer_provider_observation", "provider_unavailable_not_scored"},
	}
	finish := func(status string, output any, costUSD float64, runErr error) (ObserveAnswerProviderResult, error) {
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
			CostUsd:    pgutil.Numeric(costUSD),
		})
		if finishErr == nil {
			result.Run = finished
		}
		if runErr != nil && len(result.Observations) == 0 {
			return result, runErr
		}
		return result, finishErr
	}

	prompts, err := s.Q.ListActiveGEOPrompts(ctx, projectID)
	if err != nil {
		return finish("error", result, 0, err)
	}
	prompts, result.SkippedPrompts = sampleProviderPrompts(prompts, req.MaxPrompts, req.Engine)
	if len(prompts) == 0 {
		return finish("degraded", result, 0, nil)
	}
	if s.AnswerProvider == nil || !s.AnswerProvider.Available() {
		result.SkippedEngines = append(result.SkippedEngines, req.Engine)
		for _, prompt := range prompts {
			observation, err := s.createProviderUnavailableObservation(ctx, projectID, run.ID, prompt, req)
			if err != nil {
				return finish("error", result, 0, err)
			}
			result.Observations = append(result.Observations, observation)
		}
		return finish("degraded", result, 0, nil)
	}

	providerRows, costUSD, providerErr := s.AnswerProvider.Observe(ctx, prompts)
	if providerErr != nil {
		result.SkippedEngines = appendUniqueString(result.SkippedEngines, req.Engine)
	}
	result.CostUSD = costUSD
	ownedSurfaces, err := s.Q.ListProjectOwnedGEOExternalSurfaces(ctx, projectID)
	if err != nil {
		return finish("error", result, costUSD, err)
	}
	for _, providerRow := range providerRows {
		observation, err := s.createProviderObservation(ctx, projectID, run.ID, providerRow, req, ownedSurfaces, now)
		if err != nil {
			return finish("error", result, costUSD, err)
		}
		result.Observations = append(result.Observations, observation)
	}
	status := "ok"
	if providerErr != nil || len(result.Observations) == 0 || budgetExceeded(req.BudgetUSD, costUSD) {
		status = "degraded"
	}
	if len(result.Observations) > 0 {
		score, err := s.scoreObservations(ctx, projectID, run.ID, result.Observations, pgutil.TS(now))
		if err != nil {
			return finish("error", result, costUSD, err)
		}
		result.Score = score
	}
	return finish(status, result, costUSD, providerErr)
}

func (s Service) createProviderUnavailableObservation(ctx context.Context, projectID, runID uuid.UUID, prompt db.GeoPrompt, req ObserveAnswerProviderRequest) (db.GeoObservation, error) {
	return s.Q.CreateGEOObservation(ctx, db.CreateGEOObservationParams{
		ProjectID:               projectID,
		RunID:                   runID,
		PromptID:                uuidToPG(prompt.ID),
		Engine:                  providerEngine(req.Engine, "Perplexity"),
		Locale:                  providerLocale(req.Locale, prompt.Locale),
		SourceType:              SourceTypeAnswerEngine,
		BrandMentioned:          false,
		BrandPosition:           nil,
		ProjectCitationCount:    0,
		ProjectCitationRankBest: nil,
		ProjectCitedSurfaceIds:  jsonBytes([]string{}),
		CitedUrls:               jsonBytes([]string{}),
		CompetitorMentions:      jsonBytes([]string{}),
		CompetitorCitations:     jsonBytes([]string{}),
		ObservationState:        ObservationStateProviderUnavailable,
		AnswerSummary:           "Answer provider is not configured or unavailable.",
		EvidenceSnippets:        jsonBytes([]string{}),
		Confidence:              ConfidenceLow,
		ObservedAt:              pgutil.TS(s.now()),
	})
}

func (s Service) createProviderObservation(ctx context.Context, projectID, runID uuid.UUID, input ProviderObservation, req ObserveAnswerProviderRequest, ownedSurfaces []db.GeoExternalSurface, observedAt time.Time) (db.GeoObservation, error) {
	projectURLs, surfaceIDs := projectCitations(input.CitedURLs, ownedSurfaces)
	projectCitationCount := int32(len(projectURLs))
	if err := s.touchCitedSurfaces(ctx, projectID, ownedSurfaces, surfaceIDs, observedAt); err != nil {
		return db.GeoObservation{}, err
	}
	return s.Q.CreateGEOObservation(ctx, db.CreateGEOObservationParams{
		ProjectID:               projectID,
		RunID:                   runID,
		PromptID:                uuidToPG(input.PromptID),
		Engine:                  providerEngine(input.Engine, providerEngine(req.Engine, "Perplexity")),
		Locale:                  providerLocale(input.Locale, req.Locale),
		SourceType:              SourceTypeAnswerEngine,
		BrandMentioned:          input.BrandMentioned,
		BrandPosition:           input.BrandPosition,
		ProjectCitationCount:    projectCitationCount,
		ProjectCitationRankBest: providerCitationRank(projectCitationCount),
		ProjectCitedSurfaceIds:  jsonBytes(surfaceIDs),
		CitedUrls:               jsonBytes(input.CitedURLs),
		CompetitorMentions:      jsonBytes(input.CompetitorMentions),
		CompetitorCitations:     jsonBytes(input.CompetitorCitations),
		ObservationState:        ObservationStateObserved,
		AnswerSummary:           strings.TrimSpace(input.AnswerSummary),
		EvidenceSnippets:        jsonBytes(input.EvidenceSnippets),
		Confidence:              observationConfidence(input.Confidence),
		ObservedAt:              pgutil.TS(observedAt),
	})
}

func sampleProviderPrompts(prompts []db.GeoPrompt, maxPrompts int32, engine string) ([]db.GeoPrompt, []SkippedPrompt) {
	if maxPrompts <= 0 || int(maxPrompts) >= len(prompts) {
		return prompts, nil
	}
	skipped := make([]SkippedPrompt, 0, len(prompts)-int(maxPrompts))
	for _, prompt := range prompts[maxPrompts:] {
		skipped = append(skipped, SkippedPrompt{
			PromptID:   prompt.ID,
			PromptText: prompt.PromptText,
			Engine:     engine,
			Reason:     "max_prompts",
		})
	}
	return prompts[:maxPrompts], skipped
}

func providerEngine(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func AnswerProviderEngine(provider AnswerProvider, fallback string) string {
	if provider != nil {
		type engineProvider interface {
			Engine() string
		}
		if withEngine, ok := provider.(engineProvider); ok {
			if engine := strings.TrimSpace(withEngine.Engine()); engine != "" {
				return engine
			}
		}
	}
	return providerEngine(fallback, "Perplexity")
}

func providerLocale(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return DefaultLocale
}

func providerCitationRank(projectCitationCount int32) *int32 {
	if projectCitationCount == 0 {
		return nil
	}
	rank := int32(1)
	return &rank
}

func budgetExceeded(budgetUSD, costUSD float64) bool {
	return budgetUSD > 0 && costUSD > budgetUSD
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

type PerplexityProvider struct {
	APIKey  string
	BaseURL string
	Model   string
	Client  *http.Client
}

func NewPerplexityProvider(apiKey, baseURL, model string, client *http.Client) PerplexityProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultPerplexityBaseURL
	}
	if strings.TrimSpace(model) == "" {
		model = DefaultPerplexityModel
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return PerplexityProvider{
		APIKey:  strings.TrimSpace(apiKey),
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Model:   strings.TrimSpace(model),
		Client:  client,
	}
}

func (p PerplexityProvider) Name() string {
	return ProviderPerplexitySonar
}

func (p PerplexityProvider) Engine() string {
	return "Perplexity"
}

func (p PerplexityProvider) Available() bool {
	return strings.TrimSpace(p.APIKey) != ""
}

func (p PerplexityProvider) Observe(ctx context.Context, prompts []db.GeoPrompt) ([]ProviderObservation, float64, error) {
	if !p.Available() {
		return nil, 0, errors.New("perplexity API key is not configured")
	}
	rows := make([]ProviderObservation, 0, len(prompts))
	totalCost := 0.0
	for _, prompt := range prompts {
		row, cost, err := p.observePrompt(ctx, prompt)
		if err != nil {
			return rows, totalCost, err
		}
		rows = append(rows, row)
		totalCost += cost
	}
	return rows, totalCost, nil
}

func (p PerplexityProvider) observePrompt(ctx context.Context, prompt db.GeoPrompt) (ProviderObservation, float64, error) {
	body := map[string]any{
		"model":       p.Model,
		"messages":    []map[string]string{{"role": "user", "content": prompt.PromptText}},
		"temperature": 0.2,
		"max_tokens":  1024,
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/v1/sonar", bytes.NewReader(payload))
	if err != nil {
		return ProviderObservation{}, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	res, err := p.Client.Do(req)
	if err != nil {
		return ProviderObservation{}, 0, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ProviderObservation{}, 0, fmt.Errorf("perplexity sonar returned HTTP %d", res.StatusCode)
	}
	var out perplexityResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return ProviderObservation{}, 0, err
	}
	content := ""
	if len(out.Choices) > 0 {
		content = out.Choices[0].Message.Content
	}
	cost := out.Usage.Cost.TotalCost
	return ProviderObservation{
		PromptID:      prompt.ID,
		Engine:        "Perplexity",
		Locale:        providerLocale(prompt.Locale),
		AnswerSummary: strings.TrimSpace(content),
		CitedURLs:     uniqueStrings(out.Citations),
		Confidence:    ConfidenceMedium,
		CostUSD:       cost,
	}, cost, nil
}

type perplexityResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Citations []string `json:"citations"`
	Usage     struct {
		Cost struct {
			TotalCost float64 `json:"total_cost"`
		} `json:"cost"`
	} `json:"usage"`
}

var _ AnswerProvider = PerplexityProvider{}

func numericFromCost(costUSD float64) pgtype.Numeric {
	if costUSD <= 0 {
		return pgtype.Numeric{}
	}
	return pgutil.Numeric(costUSD)
}
