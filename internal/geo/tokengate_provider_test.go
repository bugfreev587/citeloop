package geo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

func TestTokenGateAnswerProviderUsesChatCompletionsAndCitations(t *testing.T) {
	promptID := uuid.New()
	var gotPath string
	var gotAuth string
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		gotModel = body.Model
		if len(body.Messages) == 0 || body.Messages[len(body.Messages)-1].Content != "Which tools mention CiteLoop?" {
			t.Fatalf("messages = %#v", body.Messages)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"sonar-pro",
			"choices":[{"message":{"content":"CiteLoop is mentioned with supporting sources."}}],
			"citations":["https://citeloop.app/docs","https://competitor.example/guide"],
			"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30,"cost":{"total_cost":0.0042}}
		}`))
	}))
	defer server.Close()

	provider := NewTokenGateAnswerProvider(TokenGateAnswerProviderConfig{
		Scope:   "perplexity",
		APIKey:  "tg-test-key",
		BaseURL: server.URL,
		Model:   "sonar-pro",
		Engine:  "Perplexity",
	}, server.Client())

	rows, cost, err := provider.Observe(context.Background(), []db.GeoPrompt{{
		ID:         promptID,
		PromptText: "Which tools mention CiteLoop?",
		Locale:     "en-US",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q, want /chat/completions", gotPath)
	}
	if gotAuth != "Bearer tg-test-key" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotModel != "sonar-pro" {
		t.Fatalf("model = %q", gotModel)
	}
	if cost != 0.0042 {
		t.Fatalf("cost = %v", cost)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.PromptID != promptID || row.Engine != "Perplexity" {
		t.Fatalf("row identity = %+v", row)
	}
	if row.AnswerSummary == "" || len(row.CitedURLs) != 2 || row.CitedURLs[0] != "https://citeloop.app/docs" {
		t.Fatalf("row = %+v", row)
	}
}

func TestTokenGateAnswerProviderRejectsEmptyAnswerButPreservesAccounting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"sonar-pro","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":1,"total_tokens":13,"cost":{"total_cost":0.004}}}`))
	}))
	defer server.Close()
	provider := NewTokenGateAnswerProvider(TokenGateAnswerProviderConfig{APIKey: "key", BaseURL: server.URL, Model: "sonar-pro", Engine: "Perplexity"}, server.Client())
	row, cost, err := provider.ObservePrompt(context.Background(), db.GeoPrompt{ID: uuid.New(), PromptText: "question"})
	if !errors.Is(err, ErrInvalidAnswerProviderResponse) || row.AnswerSummary != "" || row.TotalTokens != 13 || cost != 0.004 {
		t.Fatalf("row=%+v cost=%v err=%v", row, cost, err)
	}
}

func TestTokenGateAnswerProviderPreservesAccountingOnHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"usage":{"prompt_tokens":9,"completion_tokens":2,"total_tokens":11,"total_cost":0.003},"error":{"message":"rate limited"}}`))
	}))
	defer server.Close()
	provider := NewTokenGateAnswerProvider(TokenGateAnswerProviderConfig{APIKey: "key", BaseURL: server.URL, Model: "sonar-pro"}, server.Client())
	row, cost, err := provider.ObservePrompt(context.Background(), db.GeoPrompt{ID: uuid.New(), PromptText: "question"})
	if err == nil || errors.Is(err, ErrInvalidAnswerProviderResponse) || row.TotalTokens != 11 || cost != 0.003 {
		t.Fatalf("row=%+v cost=%v err=%v", row, cost, err)
	}
}

func TestPerplexityProviderRejectsEmptyAnswerAndPreservesErrorAccounting(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body       string
		invalid    bool
		wantTokens int
		wantCost   float64
	}{
		{name: "empty success", status: http.StatusOK, body: `{"choices":[],"usage":{"prompt_tokens":8,"completion_tokens":1,"total_tokens":9,"cost":{"total_cost":0.002}}}`, invalid: true, wantTokens: 9, wantCost: 0.002},
		{name: "http failure", status: http.StatusTooManyRequests, body: `{"usage":{"prompt_tokens":6,"completion_tokens":2,"total_tokens":8,"total_cost":0.0015}}`, wantTokens: 8, wantCost: 0.0015},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			provider := NewPerplexityProvider("key", server.URL, "sonar-pro", server.Client())
			row, cost, err := provider.ObservePrompt(context.Background(), db.GeoPrompt{ID: uuid.New(), PromptText: "question"})
			if err == nil || errors.Is(err, ErrInvalidAnswerProviderResponse) != tc.invalid || row.TotalTokens != tc.wantTokens || cost != tc.wantCost {
				t.Fatalf("row=%+v cost=%v err=%v", row, cost, err)
			}
		})
	}
}

func TestAnswerProviderEngineUsesTokenGateEngine(t *testing.T) {
	provider := NewTokenGateAnswerProvider(TokenGateAnswerProviderConfig{
		Scope:  "anthropic",
		APIKey: "tg-test-key",
		Model:  "claude-sonnet-4-6",
		Engine: "Anthropic",
	}, nil)

	if got := AnswerProviderEngine(provider, "OpenAI"); got != "Anthropic" {
		t.Fatalf("engine = %q, want Anthropic", got)
	}
	if got := AnswerProviderEngine(NewPerplexityProvider("pplx", "", "", nil), "OpenAI"); got != "Perplexity" {
		t.Fatalf("perplexity engine = %q, want Perplexity", got)
	}
}
