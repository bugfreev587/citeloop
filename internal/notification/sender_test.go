package notification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPSenderPostsSlackPayload(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := HTTPSender{Client: server.Client()}.Send(context.Background(), KindSlackWebhook, server.URL, json.RawMessage(`{"message":"Publish failed"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got["text"] != "Publish failed" {
		t.Fatalf("slack body = %#v", got)
	}
}

func TestHTTPSenderPostsDiscordPayload(t *testing.T) {
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	err := HTTPSender{Client: server.Client()}.Send(context.Background(), KindDiscordWebhook, server.URL, json.RawMessage(`{"title":"Budget stopped","error":"limit reached"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got["content"] != "Budget stopped\nlimit reached" || got["username"] != "CiteLoop" {
		t.Fatalf("discord body = %#v", got)
	}
}

func TestHTTPSenderReturnsNon2xxError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	err := HTTPSender{Client: server.Client()}.Send(context.Background(), KindSlackWebhook, server.URL, json.RawMessage(`{"message":"hello"}`))
	if err == nil {
		t.Fatal("expected non-2xx send to fail")
	}
}
