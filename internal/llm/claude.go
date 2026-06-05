package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Claude is a real Anthropic Messages API implementation of Provider.
// Wired per the "use real providers" decision; reads the key from env at
// construction. If the key is empty, callers should fall back to Mock.
type Claude struct {
	APIKey string
	Model  string
	client *http.Client
}

func NewClaude(apiKey, model string) *Claude {
	if model == "" {
		model = "claude-opus-4-8"
	}
	return &Claude{APIKey: apiKey, Model: model, client: &http.Client{Timeout: 120 * time.Second}}
}

// Per-million-token prices (USD) for cost accounting (§5.4). Update on model change.
var priceTable = map[string]struct{ in, out float64 }{
	"claude-opus-4-8":   {15, 75},
	"claude-sonnet-4-6": {3, 15},
	"claude-haiku-4-5":  {1, 5},
}

type anthReq struct {
	Model       string        `json:"model"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`
	System      string        `json:"system,omitempty"`
	Messages    []anthMessage `json:"messages"`
}

type anthMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthResp struct {
	Content []struct {
		Text string `json:"text"`
	} `json:"content"`
	Model string `json:"model"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Claude) Complete(ctx context.Context, req CompletionReq) (CompletionResp, error) {
	if c.APIKey == "" {
		return CompletionResp{}, fmt.Errorf("anthropic api key not set")
	}
	maxTok := req.MaxTokens
	if maxTok == 0 {
		maxTok = 4096
	}
	prompt := req.Prompt
	if req.JSON {
		prompt += "\n\nRespond with a single valid JSON object and nothing else."
	}
	body, _ := json.Marshal(anthReq{
		Model:       c.Model,
		MaxTokens:   maxTok,
		Temperature: req.Temperature,
		System:      req.System,
		Messages:    []anthMessage{{Role: "user", Content: prompt}},
	})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return CompletionResp{}, err
	}
	httpReq.Header.Set("x-api-key", c.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return CompletionResp{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return CompletionResp{}, fmt.Errorf("anthropic %d: %s", resp.StatusCode, string(raw))
	}
	var ar anthResp
	if err := json.Unmarshal(raw, &ar); err != nil {
		return CompletionResp{}, err
	}
	if ar.Error != nil {
		return CompletionResp{}, fmt.Errorf("anthropic: %s", ar.Error.Message)
	}
	var text string
	for _, b := range ar.Content {
		text += b.Text
	}
	return CompletionResp{
		Text:    text,
		Model:   ar.Model,
		Tokens:  ar.Usage.InputTokens + ar.Usage.OutputTokens,
		CostUSD: cost(c.Model, ar.Usage.InputTokens, ar.Usage.OutputTokens),
	}, nil
}

func cost(model string, in, out int) float64 {
	p, ok := priceTable[model]
	if !ok {
		p = priceTable["claude-opus-4-8"]
	}
	return float64(in)/1e6*p.in + float64(out)/1e6*p.out
}
