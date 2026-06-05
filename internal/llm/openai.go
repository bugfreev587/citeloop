package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIChat calls an OpenAI-compatible Chat Completions endpoint. TokenGate's
// gateway uses this shape at {baseURL}/chat/completions.
type OpenAIChat struct {
	APIKey  string
	BaseURL string
	Model   string
	client  *http.Client
}

func NewOpenAIChat(apiKey, baseURL, model string) *OpenAIChat {
	if baseURL == "" {
		baseURL = "https://tokengate-production.up.railway.app/v1"
	}
	if model == "" {
		model = "claude-haiku-4-5-20251001"
	}
	return &OpenAIChat{
		APIKey:  apiKey,
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

type openAIChatReq struct {
	Model          string          `json:"model"`
	Messages       []openAIMessage `json:"messages"`
	Temperature    float64         `json:"temperature,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type openAIChatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *OpenAIChat) Complete(ctx context.Context, req CompletionReq) (CompletionResp, error) {
	if c.APIKey == "" {
		return CompletionResp{}, fmt.Errorf("tokengate api key not set")
	}

	prompt := req.Prompt
	var format *responseFormat
	if req.JSON {
		prompt += "\n\nRespond with a single valid JSON object and nothing else."
		format = &responseFormat{Type: "json_object"}
	}

	messages := make([]openAIMessage, 0, 2)
	if strings.TrimSpace(req.System) != "" {
		messages = append(messages, openAIMessage{Role: "system", Content: req.System})
	}
	messages = append(messages, openAIMessage{Role: "user", Content: prompt})

	body, _ := json.Marshal(openAIChatReq{
		Model:          c.Model,
		Messages:       messages,
		Temperature:    req.Temperature,
		MaxTokens:      req.MaxTokens,
		ResponseFormat: format,
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return CompletionResp{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return CompletionResp{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return CompletionResp{}, fmt.Errorf("tokengate chat completions %d: %s", resp.StatusCode, string(raw))
	}
	var cr openAIChatResp
	if err := json.Unmarshal(raw, &cr); err != nil {
		return CompletionResp{}, err
	}
	if cr.Error != nil {
		return CompletionResp{}, fmt.Errorf("tokengate chat completions: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return CompletionResp{}, fmt.Errorf("tokengate chat completions returned no choices")
	}
	tokens := cr.Usage.TotalTokens
	if tokens == 0 {
		tokens = cr.Usage.PromptTokens + cr.Usage.CompletionTokens
	}
	model := cr.Model
	if model == "" {
		model = c.Model
	}
	return CompletionResp{
		Text:    cr.Choices[0].Message.Content,
		Model:   model,
		Tokens:  tokens,
		CostUSD: estimateOpenAIChatCost(model, cr.Usage.PromptTokens, cr.Usage.CompletionTokens),
	}, nil
}

func estimateOpenAIChatCost(model string, in, out int) float64 {
	type pricing struct{ in, out float64 }
	table := []struct {
		prefix string
		price  pricing
	}{
		{"claude-haiku-4-5", pricing{1, 5}},
		{"claude-sonnet-4-6", pricing{3, 15}},
		{"claude-opus-4", pricing{15, 75}},
		{"gpt-5.2", pricing{1.25, 10}},
		{"gpt-5.1", pricing{1.25, 10}},
		{"gpt-5", pricing{1.25, 10}},
		{"gpt-4.1", pricing{2, 8}},
		{"gpt-4o", pricing{5, 15}},
	}
	price := pricing{5, 20}
	for _, row := range table {
		if strings.HasPrefix(model, row.prefix) {
			price = row.price
			break
		}
	}
	return float64(in)/1e6*price.in + float64(out)/1e6*price.out
}
