package geo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
)

type TokenGateAnswerProviderConfig struct {
	Scope   string
	APIKey  string
	BaseURL string
	Model   string
	Engine  string
}

type TokenGateAnswerProvider struct {
	scope   string
	apiKey  string
	baseURL string
	model   string
	engine  string
	client  *http.Client
}

func NewTokenGateAnswerProvider(cfg TokenGateAnswerProviderConfig, client *http.Client) TokenGateAnswerProvider {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = "https://tokengate-production.up.railway.app/v1"
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return TokenGateAnswerProvider{
		scope:   strings.TrimSpace(cfg.Scope),
		apiKey:  strings.TrimSpace(cfg.APIKey),
		baseURL: strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		model:   strings.TrimSpace(cfg.Model),
		engine:  strings.TrimSpace(cfg.Engine),
		client:  client,
	}
}

func (p TokenGateAnswerProvider) Name() string {
	scope := strings.TrimSpace(p.scope)
	if scope == "" {
		scope = "answer"
	}
	return "tokengate_" + scope
}

func (p TokenGateAnswerProvider) Engine() string {
	return firstNonBlank(p.engine, strings.Title(p.scope))
}

func (p TokenGateAnswerProvider) Available() bool {
	return p.apiKey != "" && p.model != ""
}

func (p TokenGateAnswerProvider) Observe(ctx context.Context, prompts []db.GeoPrompt) ([]ProviderObservation, float64, error) {
	if !p.Available() {
		return nil, 0, fmt.Errorf("tokengate GEO provider is not configured")
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

func (p TokenGateAnswerProvider) observePrompt(ctx context.Context, prompt db.GeoPrompt) (ProviderObservation, float64, error) {
	body := map[string]any{
		"model":       p.model,
		"messages":    []map[string]string{{"role": "user", "content": prompt.PromptText}},
		"temperature": 0.2,
		"max_tokens":  1024,
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return ProviderObservation{}, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := p.client.Do(req)
	if err != nil {
		return ProviderObservation{}, 0, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return ProviderObservation{}, 0, fmt.Errorf("tokengate GEO chat completions %d: %s", res.StatusCode, string(raw))
	}
	var out tokenGateGEOResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return ProviderObservation{}, 0, err
	}
	content := ""
	if len(out.Choices) > 0 {
		content = out.Choices[0].Message.Content
	}
	cost := out.Usage.Cost.TotalCost
	if cost == 0 {
		cost = out.Usage.TotalCost
	}
	return ProviderObservation{
		PromptID:      prompt.ID,
		Engine:        p.Engine(),
		Locale:        providerLocale(prompt.Locale),
		AnswerSummary: strings.TrimSpace(content),
		CitedURLs:     uniqueStrings(out.Citations),
		Confidence:    ConfidenceMedium,
		CostUSD:       cost,
	}, cost, nil
}

type tokenGateGEOResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Citations []string `json:"citations"`
	Usage     struct {
		PromptTokens     int     `json:"prompt_tokens"`
		CompletionTokens int     `json:"completion_tokens"`
		TotalTokens      int     `json:"total_tokens"`
		TotalCost        float64 `json:"total_cost"`
		Cost             struct {
			TotalCost float64 `json:"total_cost"`
		} `json:"cost"`
	} `json:"usage"`
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

var _ AnswerProvider = TokenGateAnswerProvider{}
