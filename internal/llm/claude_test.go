package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestClaudeCompleteHonorsRequestModelOverride(t *testing.T) {
	var gotBody map[string]any

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
				"content": [{"type": "text", "text": "pong"}],
				"model": "claude-opus-4-8",
				"usage": {"input_tokens": 10, "output_tokens": 5}
			}`)),
		}, nil
	})

	p := NewClaude("anthropic-test-key", "claude-sonnet-4-6")
	p.client = &http.Client{Transport: transport}
	resp, err := p.Complete(context.Background(), CompletionReq{
		Model:     "claude-opus-4-8",
		Prompt:    "ping",
		MaxTokens: 16,
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
