package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

func TestNotificationChannelRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/projects/not-a-uuid/notifications/channels"},
		{http.MethodPost, "/api/projects/not-a-uuid/notifications/channels"},
		{http.MethodPost, "/api/projects/not-a-uuid/notifications/channels/not-a-channel/test"},
		{http.MethodDelete, "/api/projects/not-a-uuid/notifications/channels/not-a-channel"},
		{http.MethodGet, "/api/projects/not-a-uuid/notifications/events"},
		{http.MethodGet, "/api/projects/not-a-uuid/notifications/subscriptions"},
		{http.MethodPut, "/api/projects/not-a-uuid/notifications/subscriptions"},
		{http.MethodGet, "/api/projects/not-a-uuid/notifications/deliveries"},
		{http.MethodPost, "/api/projects/not-a-uuid/notifications/deliveries/not-a-delivery/retry"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != http.StatusBadRequest {
			t.Fatalf("%s %s status = %d, want %d", tc.method, tc.path, res.Code, http.StatusBadRequest)
		}
	}
}

func TestNotificationChannelTestRequiresSecretBeforeSending(t *testing.T) {
	router := (&Server{}).Router()
	projectID := uuid.New()
	channelID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/notifications/channels/"+channelID.String()+"/test", nil)
	res := httptest.NewRecorder()

	router.ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusInternalServerError)
	}
	if !strings.Contains(res.Body.String(), "NOTIFICATION_SECRET_KEY") {
		t.Fatalf("response should explain missing notification secret: %s", res.Body.String())
	}
}

func TestNotificationChannelResponseOmitsEncryptedURL(t *testing.T) {
	channel := db.NotificationChannel{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
		Kind:      "slack_webhook",
		Config:    json.RawMessage(`{"encrypted_url":"ciphertext","redacted_url":"https://hooks.slack.com/services/T/B/****"}`),
		Label:     "alerts",
	}

	body, err := json.Marshal(notificationChannelResponse(channel))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "encrypted_url") || strings.Contains(string(body), "ciphertext") {
		t.Fatalf("response leaked encrypted url config: %s", string(body))
	}
	if !strings.Contains(string(body), "redacted_url") {
		t.Fatalf("response missing redacted url: %s", string(body))
	}
}

func TestNotificationSupportedEventsResponse(t *testing.T) {
	body, err := json.Marshal(supportedNotificationEvents())
	if err != nil {
		t.Fatal(err)
	}
	for _, eventType := range []string{"generation.failed", "publish.failed", "budget.stopped", "review.overdue", "webhook.delivery.dead"} {
		if !strings.Contains(string(body), eventType) {
			t.Fatalf("events response missing %s: %s", eventType, string(body))
		}
	}
}
