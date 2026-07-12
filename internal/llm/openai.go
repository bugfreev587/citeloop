package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
		model = DefaultTokenGateModel
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
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = c.Model
	}

	resp, err := c.completeAttempt(ctx, req, model)
	if err == nil {
		return resp, nil
	}
	var accountingErr *providerAttemptAccountingError
	if errors.As(err, &accountingErr) {
		return resp, err
	}
	var httpErr *openAIChatHTTPError
	if errors.As(err, &httpErr) && shouldRetryUnsupportedClaudeModel(httpErr.status, model, httpErr.raw) {
		if req.DisableProviderFallback {
			return resp, fmt.Errorf("%w; provider fallback disabled for %s", err, firstNonBlankLocal(string(req.Purpose), "default"))
		}
		fallback := fallbackOpenAIModel(req.Purpose)
		if fallback != "" && fallback != model {
			return c.completeAttempt(ctx, req, fallback)
		}
	}
	return resp, err
}

func (*OpenAIChat) ObservesProviderAttempts() {}

func (c *OpenAIChat) completeAttempt(ctx context.Context, req CompletionReq, model string) (CompletionResp, error) {
	if req.AttemptObserver == nil {
		return c.completeWithModel(ctx, req, model)
	}
	attemptID, err := req.AttemptObserver.StartAttempt(ctx, model)
	if err != nil {
		return CompletionResp{}, err
	}
	resp, providerErr := c.completeWithModel(ctx, req, model)
	finishErr := req.AttemptObserver.FinishAttempt(context.WithoutCancel(ctx), attemptID, resp, providerErr)
	if finishErr != nil {
		return resp, &providerAttemptAccountingError{providerErr: providerErr, ledgerErr: finishErr}
	}
	return resp, errors.Join(providerErr, finishErr)
}

type providerAttemptAccountingError struct {
	providerErr error
	ledgerErr   error
}

func (e *providerAttemptAccountingError) Error() string {
	return errors.Join(e.providerErr, e.ledgerErr).Error()
}

func (e *providerAttemptAccountingError) Unwrap() []error {
	return []error{e.providerErr, e.ledgerErr}
}

func (c *OpenAIChat) completeWithModel(ctx context.Context, req CompletionReq, model string) (CompletionResp, error) {
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
		Model:          model,
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
		return CompletionResp{Provider: "tokengate", Model: model}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var cr openAIChatResp
	_ = json.Unmarshal(raw, &cr)
	ledger := openAICompletionLedger(cr, model)
	if resp.StatusCode >= 400 {
		return ledger, &openAIChatHTTPError{status: resp.StatusCode, raw: raw}
	}
	if err := json.Unmarshal(raw, &cr); err != nil {
		return CompletionResp{Provider: "tokengate", Model: model}, err
	}
	if cr.Error != nil {
		return ledger, fmt.Errorf("tokengate chat completions: %s", cr.Error.Message)
	}
	if len(cr.Choices) == 0 {
		return ledger, fmt.Errorf("tokengate chat completions returned no choices")
	}
	ledger.Text = cr.Choices[0].Message.Content
	return ledger, nil
}

func openAICompletionLedger(cr openAIChatResp, requestedModel string) CompletionResp {
	tokens := cr.Usage.TotalTokens
	if tokens == 0 {
		tokens = cr.Usage.PromptTokens + cr.Usage.CompletionTokens
	}
	respModel := cr.Model
	if respModel == "" {
		respModel = requestedModel
	}
	return CompletionResp{
		Provider: "tokengate", Model: respModel,
		PromptTokens: cr.Usage.PromptTokens, CompletionTokens: cr.Usage.CompletionTokens,
		Tokens: tokens, CostUSD: estimateOpenAIChatCost(respModel, cr.Usage.PromptTokens, cr.Usage.CompletionTokens),
	}
}

type openAIChatHTTPError struct {
	status int
	raw    []byte
}

func (e *openAIChatHTTPError) Error() string {
	return fmt.Sprintf("tokengate chat completions %d: %s", e.status, string(e.raw))
}

func fallbackOpenAIModel(purpose CompletionPurpose) string {
	switch purpose {
	case PurposeQA:
		return DefaultOpenAIQAModel
	case PurposeWriter:
		return DefaultOpenAIWriterModel
	default:
		return DefaultOpenAIModel
	}
}

func shouldRetryUnsupportedClaudeModel(status int, model string, raw []byte) bool {
	if status != http.StatusBadRequest || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "claude-") {
		return false
	}
	msg := strings.ToLower(string(raw))
	return strings.Contains(msg, "not supported") &&
		(strings.Contains(msg, "chatgpt account") || strings.Contains(msg, "openai") || strings.Contains(msg, "codex"))
}

func firstNonBlankLocal(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func estimateOpenAIChatCost(model string, in, out int) float64 {
	type pricing struct{ in, out float64 }
	table := []struct {
		prefix string
		price  pricing
	}{
		{"claude-haiku-4-5", pricing{1, 5}},
		{"claude-sonnet-4-6", pricing{3, 15}},
		{"claude-opus-4-8", pricing{5, 25}},
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
