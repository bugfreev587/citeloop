package articleassets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIImageProviderUsesApprovedBriefAndReturnsDecodedImage(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/generations" || r.Header.Get("Authorization") != "Bearer image-secret" {
			t.Fatalf("request = %s %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString([]byte("png-bytes"))}}, "size": "1536x1024"})
	}))
	defer server.Close()
	provider := OpenAIProvider{APIKey: "image-secret", BaseURL: server.URL, Model: "gpt-image-1", Client: server.Client()}
	result, err := provider.Generate(context.Background(), GenerateRequest{Role: RoleHero, Prompt: "Approved outline: evidence-led workflow"})
	if err != nil || string(result.Bytes) != "png-bytes" || result.Provider != "openai" {
		t.Fatalf("result = %#v %v", result, err)
	}
	if received["model"] != "gpt-image-1" || !strings.Contains(received["prompt"].(string), "evidence-led workflow") {
		t.Fatalf("payload = %#v", received)
	}
}

func TestOpenAIImageProviderRejectsMissingPromptAndBoundedError(t *testing.T) {
	provider := OpenAIProvider{APIKey: "key", BaseURL: "https://api.openai.test", Model: "gpt-image-1"}
	if _, err := provider.Generate(context.Background(), GenerateRequest{}); err == nil {
		t.Fatal("empty approved prompt accepted")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, strings.Repeat("sensitive ", 2000), http.StatusBadRequest)
	}))
	defer server.Close()
	provider.BaseURL, provider.Client = server.URL, server.Client()
	if _, err := provider.Generate(context.Background(), GenerateRequest{Prompt: "diagram"}); err == nil || len(err.Error()) > 700 {
		t.Fatalf("unbounded provider error: %v", err)
	}
}
