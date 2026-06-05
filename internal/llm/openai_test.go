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
