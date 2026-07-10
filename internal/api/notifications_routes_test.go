package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestNotificationChannelRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/projects/not-a-uuid/notifications/channels"},
		{http.MethodPost, "/api/projects/not-a-uuid/notifications/channels"},
		{http.MethodPatch, "/api/projects/not-a-uuid/notifications/channels/not-a-channel"},
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
		ID:     uuid.New(),
		Kind:   "slack_webhook",
		Config: json.RawMessage(`{"encrypted_url":"ciphertext","redacted_url":"https://hooks.slack.com/services/T/B/****"}`),
		Label:  "alerts",
	}
	projectID := uuid.New()
	channel.ProjectID = pgtype.UUID{Bytes: projectID, Valid: true}

	body, err := json.Marshal(notificationChannelResponse(channel, 2))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "encrypted_url") || strings.Contains(string(body), "ciphertext") {
		t.Fatalf("response leaked encrypted url config: %s", string(body))
	}
	if !strings.Contains(string(body), "redacted_url") {
		t.Fatalf("response missing redacted url: %s", string(body))
	}
	if !strings.Contains(string(body), `"project_subscription_count":2`) {
		t.Fatalf("response missing usage count: %s", string(body))
	}
}

func TestNotificationChannelResponseOmitsEncryptedEmail(t *testing.T) {
	channel := db.NotificationChannel{
		ID:      uuid.New(),
		OwnerID: "owner-1",
		Kind:    "email",
		Config:  json.RawMessage(`{"encrypted_to":"ciphertext","redacted_to":"o***@example.com","address_hash":"hash"}`),
		Label:   "Ops",
	}

	body, err := json.Marshal(notificationChannelResponse(channel, 1))
	if err != nil {
		t.Fatal(err)
	}
	raw := string(body)
	for _, leak := range []string{"encrypted_to", "ciphertext", "address_hash", "hash"} {
		if strings.Contains(raw, leak) {
			t.Fatalf("response leaked email config %q: %s", leak, raw)
		}
	}
	if !strings.Contains(raw, `"redacted_to":"o***@example.com"`) {
		t.Fatalf("response missing redacted email: %s", raw)
	}
	if !strings.Contains(raw, `"owner_id":"owner-1"`) {
		t.Fatalf("response missing owner id: %s", raw)
	}
}

func TestNotificationSupportedEventsResponse(t *testing.T) {
	body, err := json.Marshal(supportedNotificationEvents())
	if err != nil {
		t.Fatal(err)
	}
	for _, eventType := range []string{
		"generation.failed",
		"publish.failed",
		"budget.stopped",
		"review.overdue",
		"sitefix.pr.awaiting_merge",
		"webhook.delivery.dead",
		"seo.sync.failed",
		"seo.auth.expired",
		"seo.opportunity.ready",
		"seo.brief.ready",
		"seo.action.measurement_ready",
		"seo.indexing.anomaly",
	} {
		if !strings.Contains(string(body), eventType) {
			t.Fatalf("events response missing %s: %s", eventType, string(body))
		}
	}
}
