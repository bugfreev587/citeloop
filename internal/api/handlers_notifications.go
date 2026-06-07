package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/notification"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type notificationChannelDTO struct {
	ID         uuid.UUID       `json:"id"`
	ProjectID  uuid.UUID       `json:"project_id"`
	Kind       string          `json:"kind"`
	Config     json.RawMessage `json:"config"`
	Label      string          `json:"label"`
	VerifiedAt any             `json:"verified_at"`
	CreatedAt  any             `json:"created_at"`
	DeletedAt  any             `json:"deleted_at,omitempty"`
}

type notificationEventDTO struct {
	Type string `json:"type"`
}

type notificationSubscriptionDTO struct {
	ID        uuid.UUID       `json:"id"`
	ProjectID uuid.UUID       `json:"project_id"`
	EventType string          `json:"event_type"`
	ChannelID uuid.UUID       `json:"channel_id"`
	Enabled   bool            `json:"enabled"`
	Filter    json.RawMessage `json:"filter"`
	CreatedAt any             `json:"created_at"`
}

type notificationDeliveryDTO struct {
	ID             uuid.UUID       `json:"id"`
	ProjectID      uuid.UUID       `json:"project_id"`
	SubscriptionID any             `json:"subscription_id"`
	ChannelID      uuid.UUID       `json:"channel_id"`
	EventType      string          `json:"event_type"`
	EventID        string          `json:"event_id"`
	Payload        json.RawMessage `json:"payload"`
	Status         string          `json:"status"`
	Attempts       int32           `json:"attempts"`
	NextRetryAt    any             `json:"next_retry_at"`
	LastError      *string         `json:"last_error"`
	DeliveredAt    any             `json:"delivered_at"`
	CreatedAt      any             `json:"created_at"`
}

func (s *Server) listNotificationChannels(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	channels, err := s.Q.ListNotificationChannels(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]notificationChannelDTO, 0, len(channels))
	for _, channel := range channels {
		out = append(out, notificationChannelResponse(channel))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createNotificationChannel(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	if s.Env.NotificationSecretKey == "" {
		writeErr(w, http.StatusInternalServerError, "NOTIFICATION_SECRET_KEY is required")
		return
	}
	var in struct {
		Kind  string `json:"kind"`
		URL   string `json:"url"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg, err := notification.PrepareWebhookConfig(in.Kind, in.URL, s.Env.NotificationSecretKey)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	channel, err := s.Q.CreateNotificationChannel(r.Context(), db.CreateNotificationChannelParams{
		ProjectID: projectID,
		Kind:      in.Kind,
		Config:    cfg.JSON(),
		Label:     in.Label,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, notificationChannelResponse(channel))
}

func (s *Server) deleteNotificationChannel(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	channelID, err := uuid.Parse(chi.URLParam(r, "channelID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad channel id")
		return
	}
	channel, err := s.Q.SoftDeleteNotificationChannel(r.Context(), db.SoftDeleteNotificationChannelParams{ID: channelID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "channel not found")
		return
	}
	writeJSON(w, http.StatusOK, notificationChannelResponse(channel))
}

func (s *Server) testNotificationChannel(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	channelID, err := uuid.Parse(chi.URLParam(r, "channelID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad channel id")
		return
	}
	if s.Env.NotificationSecretKey == "" {
		writeErr(w, http.StatusInternalServerError, "NOTIFICATION_SECRET_KEY is required")
		return
	}
	channel, err := s.Q.GetNotificationChannel(r.Context(), db.GetNotificationChannelParams{ID: channelID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "channel not found")
		return
	}
	var cfg notification.WebhookConfig
	if err := json.Unmarshal(channel.Config, &cfg); err != nil {
		writeErr(w, http.StatusInternalServerError, "channel config is invalid")
		return
	}
	webhookURL, err := notification.DecryptWebhookURL(cfg, s.Env.NotificationSecretKey)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "channel config cannot be decrypted")
		return
	}
	label := channel.Label
	if label == "" {
		label = "Webhook"
	}
	payload, _ := json.Marshal(map[string]string{"message": "CiteLoop test notification: " + label})
	if err := (notification.HTTPSender{}).Send(r.Context(), channel.Kind, webhookURL, payload); err != nil {
		writeErr(w, http.StatusBadGateway, "webhook test failed: "+err.Error())
		return
	}
	channel, err = s.Q.MarkNotificationChannelVerified(r.Context(), db.MarkNotificationChannelVerifiedParams{ID: channelID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, notificationChannelResponse(channel))
}

func (s *Server) listNotificationEvents(w http.ResponseWriter, r *http.Request) {
	if _, err := s.projectID(r); err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	writeJSON(w, http.StatusOK, supportedNotificationEvents())
}

func (s *Server) listNotificationSubscriptions(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	subs, err := s.Q.ListNotificationSubscriptions(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]notificationSubscriptionDTO, 0, len(subs))
	for _, sub := range subs {
		out = append(out, notificationSubscriptionResponse(sub))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) upsertNotificationSubscription(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	var in struct {
		EventType string          `json:"event_type"`
		ChannelID string          `json:"channel_id"`
		Enabled   *bool           `json:"enabled"`
		Filter    json.RawMessage `json:"filter"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	channelID, err := uuid.Parse(in.ChannelID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad channel id")
		return
	}
	if !isSupportedNotificationEvent(in.EventType) {
		writeErr(w, http.StatusBadRequest, "unsupported event type")
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	sub, err := s.Q.UpsertNotificationSubscription(r.Context(), db.UpsertNotificationSubscriptionParams{
		ProjectID: projectID,
		EventType: in.EventType,
		ChannelID: channelID,
		Enabled:   enabled,
		Filter:    in.Filter,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, notificationSubscriptionResponse(sub))
}

func (s *Server) listNotificationDeliveries(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	limit := int32(50)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeErr(w, http.StatusBadRequest, "bad limit")
			return
		}
		if n > 100 {
			n = 100
		}
		limit = int32(n)
	}
	deliveries, err := s.Q.ListNotificationDeliveries(r.Context(), db.ListNotificationDeliveriesParams{
		ProjectID: projectID,
		Status:    r.URL.Query().Get("status"),
		LimitRows: limit,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]notificationDeliveryDTO, 0, len(deliveries))
	for _, delivery := range deliveries {
		out = append(out, notificationDeliveryResponse(delivery))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) retryNotificationDelivery(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	deliveryID, err := uuid.Parse(chi.URLParam(r, "deliveryID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad delivery id")
		return
	}
	delivery, err := s.Q.RetryNotificationDelivery(r.Context(), db.RetryNotificationDeliveryParams{ID: deliveryID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "delivery not found")
		return
	}
	writeJSON(w, http.StatusOK, notificationDeliveryResponse(delivery))
}

func notificationChannelResponse(channel db.NotificationChannel) notificationChannelDTO {
	var cfg notification.WebhookConfig
	_ = json.Unmarshal(channel.Config, &cfg)
	redacted, _ := json.Marshal(map[string]string{"redacted_url": cfg.RedactedURL})
	return notificationChannelDTO{
		ID:         channel.ID,
		ProjectID:  channel.ProjectID,
		Kind:       channel.Kind,
		Config:     redacted,
		Label:      channel.Label,
		VerifiedAt: channel.VerifiedAt,
		CreatedAt:  channel.CreatedAt,
		DeletedAt:  channel.DeletedAt,
	}
}

func supportedNotificationEvents() []notificationEventDTO {
	types := []string{
		"generation.failed",
		"publish.failed",
		"budget.stopped",
		"review.overdue",
		"webhook.delivery.dead",
		"seo.sync.failed",
		"seo.auth.expired",
		"seo.opportunity.ready",
		"seo.brief.ready",
		"seo.action.measurement_ready",
		"seo.indexing.anomaly",
	}
	out := make([]notificationEventDTO, 0, len(types))
	for _, eventType := range types {
		out = append(out, notificationEventDTO{Type: eventType})
	}
	return out
}

func isSupportedNotificationEvent(eventType string) bool {
	for _, event := range supportedNotificationEvents() {
		if event.Type == eventType {
			return true
		}
	}
	return false
}

func notificationSubscriptionResponse(sub db.NotificationSubscription) notificationSubscriptionDTO {
	filter := json.RawMessage("null")
	if len(sub.Filter) > 0 {
		filter = json.RawMessage(sub.Filter)
	}
	return notificationSubscriptionDTO{
		ID:        sub.ID,
		ProjectID: sub.ProjectID,
		EventType: sub.EventType,
		ChannelID: sub.ChannelID,
		Enabled:   sub.Enabled,
		Filter:    filter,
		CreatedAt: sub.CreatedAt,
	}
}

func notificationDeliveryResponse(delivery db.NotificationDelivery) notificationDeliveryDTO {
	return notificationDeliveryDTO{
		ID:             delivery.ID,
		ProjectID:      delivery.ProjectID,
		SubscriptionID: delivery.SubscriptionID,
		ChannelID:      delivery.ChannelID,
		EventType:      delivery.EventType,
		EventID:        delivery.EventID,
		Payload:        delivery.Payload,
		Status:         delivery.Status,
		Attempts:       delivery.Attempts,
		NextRetryAt:    delivery.NextRetryAt,
		LastError:      delivery.LastError,
		DeliveredAt:    delivery.DeliveredAt,
		CreatedAt:      delivery.CreatedAt,
	}
}
