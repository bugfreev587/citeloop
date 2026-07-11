package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOpenAIChatCompleteUsesTokenGateCompatibleChatCompletions(t *testing.T) {
	var gotAuth string
	var gotPath string
	var gotBody map[string]any

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
			"id": "chatcmpl_test",
			"model": "claude-haiku-4-5-20251001",
			"choices": [
				{"message": {"role": "assistant", "content": "{\"ok\":true}"}}
			],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
		}`)),
		}, nil
	})

	p := NewOpenAIChat("tg-test-key", "https://tokengate.example/v1", "claude-haiku-4-5-20251001")
	p.client = &http.Client{Transport: transport}
	resp, err := p.Complete(context.Background(), CompletionReq{
		System:      "system prompt",
		Prompt:      "user prompt",
		MaxTokens:   321,
		Temperature: 0.2,
		JSON:        true,
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if gotAuth != "Bearer tg-test-key" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody["model"] != "claude-haiku-4-5-20251001" {
		t.Fatalf("model = %v", gotBody["model"])
	}
	if gotBody["max_tokens"] != float64(321) {
		t.Fatalf("max_tokens = %v", gotBody["max_tokens"])
	}
	if gotBody["temperature"] != 0.2 {
		t.Fatalf("temperature = %v", gotBody["temperature"])
	}
	responseFormat, ok := gotBody["response_format"].(map[string]any)
	if !ok || responseFormat["type"] != "json_object" {
		t.Fatalf("response_format = %#v", gotBody["response_format"])
	}
	messages, ok := gotBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v", gotBody["messages"])
	}
	if messages[0].(map[string]any)["role"] != "system" {
		t.Fatalf("first message = %#v", messages[0])
	}
	userContent := messages[1].(map[string]any)["content"].(string)
	if userContent != "user prompt\n\nRespond with a single valid JSON object and nothing else." {
		t.Fatalf("user content = %q", userContent)
	}
	if resp.Text != `{"ok":true}` {
		t.Fatalf("text = %q", resp.Text)
	}
	if resp.Model != "claude-haiku-4-5-20251001" {
		t.Fatalf("resp model = %q", resp.Model)
	}
	if resp.Tokens != 15 {
		t.Fatalf("tokens = %d", resp.Tokens)
	}
	if resp.CostUSD <= 0 {
		t.Fatalf("cost should be positive, got %f", resp.CostUSD)
	}
}

func TestOpenAIChatCompleteHonorsRequestModelOverride(t *testing.T) {
	var gotBody map[string]any

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"id": "chatcmpl_test",
				"choices": [
					{"message": {"role": "assistant", "content": "pong"}}
				],
				"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
			}`)),
		}, nil
	})

	p := NewOpenAIChat("tg-test-key", "https://tokengate.example/v1", "claude-sonnet-4-6")
	p.client = &http.Client{Transport: transport}
	resp, err := p.Complete(context.Background(), CompletionReq{
		Model:  "claude-opus-4-8",
		Prompt: "ping",
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	if gotBody["model"] != "claude-opus-4-8" {
		t.Fatalf("model = %v", gotBody["model"])
	}
	if resp.Model != "claude-opus-4-8" {
		t.Fatalf("resp model = %q", resp.Model)
	}
}

func TestOpenAIChatCompleteRetriesUnsupportedClaudeModelWithOpenAIFallback(t *testing.T) {
	tests := []struct {
		name         string
		req          CompletionReq
		wantFallback string
	}{
		{
			name:         "writer uses gpt-5.1",
			req:          CompletionReq{Prompt: "write", Purpose: PurposeWriter, Model: "claude-sonnet-4-6"},
			wantFallback: "gpt-5.1",
		},
		{
			name:         "qa uses gpt-5.5",
			req:          CompletionReq{Prompt: "check", Purpose: PurposeQA, Model: "claude-opus-4-8"},
			wantFallback: "gpt-5.5",
		},
		{
			name:         "default uses gpt-5.1",
			req:          CompletionReq{Prompt: "plan", Model: "claude-sonnet-4-6"},
			wantFallback: "gpt-5.1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotModels []string
			transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				model := body["model"].(string)
				gotModels = append(gotModels, model)
				if len(gotModels) == 1 {
					return &http.Response{
						StatusCode: 400,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"The '` + model + `' model is not supported when using Codex with a ChatGPT account.","type":"invalid_request_error"}}`)),
					}, nil
				}
				return &http.Response{
					StatusCode: 200,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body: io.NopCloser(strings.NewReader(`{
						"id": "chatcmpl_retry",
						"model": "` + tc.wantFallback + `",
						"choices": [{"message": {"role": "assistant", "content": "ok"}}],
						"usage": {"prompt_tokens": 3, "completion_tokens": 2, "total_tokens": 5}
					}`)),
				}, nil
			})

			p := NewOpenAIChat("tg-test-key", "https://tokengate.example/v1", "claude-sonnet-4-6")
			p.client = &http.Client{Transport: transport}
			resp, err := p.Complete(context.Background(), tc.req)
			if err != nil {
				t.Fatalf("complete: %v", err)
			}
			if len(gotModels) != 2 {
				t.Fatalf("models sent = %#v, want initial and retry", gotModels)
			}
			if gotModels[1] != tc.wantFallback {
				t.Fatalf("fallback model = %q, want %q", gotModels[1], tc.wantFallback)
			}
			if resp.Model != tc.wantFallback {
				t.Fatalf("resp model = %q, want %q", resp.Model, tc.wantFallback)
			}
		})
	}
}

func TestOpenAIChatCompleteCanDisableProviderFallbackForSiteFix(t *testing.T) {
	var gotModels []string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		model := body["model"].(string)
		gotModels = append(gotModels, model)
		return &http.Response{
			StatusCode: 400,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"The '` + model + `' model is not supported when using Codex with a ChatGPT account.","type":"invalid_request_error"}}`)),
		}, nil
	})

	p := NewOpenAIChat("tg-test-key", "https://tokengate.example/v1", "claude-opus-4-8")
	p.client = &http.Client{Transport: transport}
	_, err := p.Complete(context.Background(), CompletionReq{
		Prompt:                  "repair site fix",
		Purpose:                 PurposeSiteFix,
		Model:                   "claude-opus-4-8",
		DisableProviderFallback: true,
	})
	if err == nil {
		t.Fatal("expected unsupported Anthropic model error without OpenAI fallback")
	}
	if len(gotModels) != 1 || gotModels[0] != "claude-opus-4-8" {
		t.Fatalf("models sent = %#v, want only the configured site fix model", gotModels)
	}
	if !strings.Contains(err.Error(), "fallback disabled") {
		t.Fatalf("error should make disabled fallback explicit, got %v", err)
	}
}

func TestOpenAIChatCompletePreservesResolvedUsageOnProviderError(t *testing.T) {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTooManyRequests, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"model":"claude-opus-4-8","usage":{"prompt_tokens":11,"completion_tokens":2,"total_tokens":13},"error":{"message":"rate limited"}}`))}, nil
	})
	p := NewOpenAIChat("tg-test-key", "https://tokengate.example/v1", "claude-opus-4-8")
	p.client = &http.Client{Transport: transport}
	resp, err := p.Complete(context.Background(), CompletionReq{Prompt: "repair", Purpose: PurposeSiteFix, Model: "claude-opus-4-8", DisableProviderFallback: true})
	if err == nil {
		t.Fatal("expected provider error")
	}
	if resp.Provider != "tokengate" || resp.Model != "claude-opus-4-8" || resp.PromptTokens != 11 || resp.CompletionTokens != 2 || resp.Tokens != 13 || resp.CostUSD <= 0 {
		t.Fatalf("provider error ledger response = %+v", resp)
	}
}

func TestOpenAIChatCompleteRequiresAPIKey(t *testing.T) {
	p := NewOpenAIChat("", "https://example.test/v1", "claude-haiku-4-5-20251001")
	if _, err := p.Complete(context.Background(), CompletionReq{Prompt: "hi"}); err == nil {
		t.Fatal("expected missing api key error")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
