package notification

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
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

	err := HTTPSender{Client: server.Client()}.Send(context.Background(), DeliveryTarget{
		Kind:        KindSlackWebhook,
		Destination: server.URL,
		Payload:     json.RawMessage(`{"message":"Publish failed"}`),
	})
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

	err := HTTPSender{Client: server.Client()}.Send(context.Background(), DeliveryTarget{
		Kind:        KindDiscordWebhook,
		Destination: server.URL,
		Payload:     json.RawMessage(`{"title":"Budget stopped","error":"limit reached"}`),
	})
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

	err := HTTPSender{Client: server.Client()}.Send(context.Background(), DeliveryTarget{
		Kind:        KindSlackWebhook,
		Destination: server.URL,
		Payload:     json.RawMessage(`{"message":"hello"}`),
	})
	if err == nil {
		t.Fatal("expected non-2xx send to fail")
	}
}

func TestHTTPSenderPostsResendEmailPayload(t *testing.T) {
	deliveryID := uuid.New()
	var got map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emails" {
			t.Fatalf("path = %s, want /emails", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer resend-key" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Idempotency-Key") != deliveryID.String() {
			t.Fatalf("idempotency key = %q", r.Header.Get("Idempotency-Key"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"email-message-1"}`))
	}))
	defer server.Close()

	err := HTTPSender{
		Client:       server.Client(),
		ResendAPIKey: "resend-key",
		EmailFrom:    "CiteLoop <notifications@citeloop.app>",
		EmailReplyTo: "support@citeloop.app",
		ResendURL:    server.URL,
	}.Send(context.Background(), DeliveryTarget{
		Kind:        KindEmail,
		Destination: "ops@example.com",
		DeliveryID:  deliveryID,
		Payload:     json.RawMessage(`{"title":"Publish failed","message":"CiteLoop publish failed","dashboard_url":"https://citeloop.app/projects/1"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["from"] != "CiteLoop <notifications@citeloop.app>" {
		t.Fatalf("from = %#v", got["from"])
	}
	if got["reply_to"] != "support@citeloop.app" {
		t.Fatalf("reply_to = %#v", got["reply_to"])
	}
	to, ok := got["to"].([]any)
	if !ok || len(to) != 1 || to[0] != "ops@example.com" {
		t.Fatalf("to = %#v", got["to"])
	}
	if got["subject"] != "CiteLoop: Publish failed" {
		t.Fatalf("subject = %#v", got["subject"])
	}
	if got["text"] == "" || got["html"] == "" {
		t.Fatalf("email body missing text/html: %#v", got)
	}
}

func TestHTTPSenderReturnsResendAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"recipient is suppressed"}`))
	}))
	defer server.Close()

	err := HTTPSender{
		Client:       server.Client(),
		ResendAPIKey: "resend-key",
		EmailFrom:    "CiteLoop <notifications@citeloop.app>",
		ResendURL:    server.URL,
	}.Send(context.Background(), DeliveryTarget{
		Kind:        KindEmail,
		Destination: "ops@example.com",
		DeliveryID:  uuid.New(),
		Payload:     json.RawMessage(`{"message":"hello"}`),
	})
	if err == nil {
		t.Fatal("expected Resend non-2xx response to fail")
	}
}
