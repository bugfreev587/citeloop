package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/notification"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type notificationChannelDTO struct {
	ID                       uuid.UUID       `json:"id"`
	ProjectID                *uuid.UUID      `json:"project_id"`
	OwnerID                  string          `json:"owner_id"`
	Kind                     string          `json:"kind"`
	Config                   json.RawMessage `json:"config"`
	Label                    string          `json:"label"`
	VerifiedAt               any             `json:"verified_at"`
	CreatedAt                any             `json:"created_at"`
	DeletedAt                any             `json:"deleted_at,omitempty"`
	ProjectSubscriptionCount int32           `json:"project_subscription_count"`
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
		out = append(out, notificationChannelResponse(notificationChannelFromListRow(channel), channel.ProjectSubscriptionCount))
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
		Kind        string `json:"kind"`
		URL         string `json:"url"`
		Destination string `json:"destination"`
		Label       string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	destination := strings.TrimSpace(in.Destination)
	if destination == "" {
		destination = strings.TrimSpace(in.URL)
	}
	var cfg json.RawMessage
	switch in.Kind {
	case notification.KindSlackWebhook, notification.KindDiscordWebhook:
		prepared, err := notification.PrepareWebhookConfig(in.Kind, destination, s.Env.NotificationSecretKey)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		cfg = prepared.JSON()
	case notification.KindEmail:
		project, err := s.Q.GetProject(r.Context(), projectID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "project not found")
			return
		}
		prepared, err := notification.PrepareEmailConfig(project.OwnerID, destination, s.Env.NotificationSecretKey)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		cfg = prepared.JSON()
	default:
		writeErr(w, http.StatusBadRequest, "notification channel kind must be slack_webhook, discord_webhook, or email")
		return
	}
	channel, err := s.Q.CreateNotificationChannel(r.Context(), db.CreateNotificationChannelParams{
		ProjectID: projectID,
		Kind:      in.Kind,
		Config:    cfg,
		Label:     in.Label,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, notificationChannelResponse(channel, 0))
}

func (s *Server) updateNotificationChannel(w http.ResponseWriter, r *http.Request) {
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
	var in struct {
		Label       *string `json:"label"`
		URL         *string `json:"url"`
		Destination *string `json:"destination"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.URL != nil || in.Destination != nil {
		writeErr(w, http.StatusBadRequest, "notification destination updates are not supported in V1; create a new channel")
		return
	}
	if in.Label == nil {
		channel, err := s.Q.GetNotificationChannel(r.Context(), db.GetNotificationChannelParams{ID: channelID, ProjectID: projectID})
		if err != nil {
			writeErr(w, http.StatusNotFound, "channel not found")
			return
		}
		usage, err := s.Q.CountNotificationChannelProjectSubscriptions(r.Context(), db.CountNotificationChannelProjectSubscriptionsParams{ProjectID: projectID, ChannelID: channelID})
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, notificationChannelResponse(channel, usage))
		return
	}
	label := strings.TrimSpace(*in.Label)
	channel, err := s.Q.UpdateNotificationChannelLabel(r.Context(), db.UpdateNotificationChannelLabelParams{ID: channelID, ProjectID: projectID, Label: label})
	if err != nil {
		writeErr(w, http.StatusNotFound, "channel not found")
		return
	}
	usage, err := s.Q.CountNotificationChannelProjectSubscriptions(r.Context(), db.CountNotificationChannelProjectSubscriptionsParams{ProjectID: projectID, ChannelID: channelID})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, notificationChannelResponse(channel, usage))
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
	usage, err := s.Q.CountNotificationChannelProjectSubscriptions(r.Context(), db.CountNotificationChannelProjectSubscriptionsParams{ProjectID: projectID, ChannelID: channelID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "channel not found")
		return
	}
	channel, err := s.Q.SoftDeleteNotificationChannel(r.Context(), db.SoftDeleteNotificationChannelParams{ID: channelID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "channel not found")
		return
	}
	writeJSON(w, http.StatusOK, notificationChannelResponse(channel, usage))
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
	label := strings.TrimSpace(channel.Label)
	if label == "" {
		label = "Notification channel"
	}
	payload, _ := json.Marshal(map[string]string{
		"title":   "Test notification",
		"message": "CiteLoop test notification: " + label,
	})
	target, err := notificationDeliveryTarget(channel, s.Env.NotificationSecretKey, payload)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "channel config cannot be decrypted")
		return
	}
	target.DeliveryID = channel.ID
	sender := notification.HTTPSender{
		ResendAPIKey: s.Env.ResendAPIKey,
		EmailFrom:    s.Env.NotificationEmailFrom,
		EmailReplyTo: s.Env.NotificationEmailReplyTo,
	}
	if err := sender.Send(r.Context(), target); err != nil {
		writeErr(w, http.StatusBadGateway, "notification test failed: "+err.Error())
		return
	}
	channel, err = s.Q.MarkNotificationChannelVerified(r.Context(), db.MarkNotificationChannelVerifiedParams{ID: channelID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	usage, err := s.Q.CountNotificationChannelProjectSubscriptions(r.Context(), db.CountNotificationChannelProjectSubscriptionsParams{ProjectID: projectID, ChannelID: channelID})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, notificationChannelResponse(channel, usage))
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
		if err == pgx.ErrNoRows {
			writeErr(w, http.StatusBadRequest, "channel is unavailable for this project or requires a successful test first")
			return
		}
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

func notificationChannelResponse(channel db.NotificationChannel, projectSubscriptionCount int32) notificationChannelDTO {
	return notificationChannelDTO{
		ID:                       channel.ID,
		ProjectID:                nullableUUID(channel.ProjectID),
		OwnerID:                  channel.OwnerID,
		Kind:                     channel.Kind,
		Config:                   redactedNotificationConfig(channel),
		Label:                    channel.Label,
		VerifiedAt:               channel.VerifiedAt,
		CreatedAt:                channel.CreatedAt,
		DeletedAt:                channel.DeletedAt,
		ProjectSubscriptionCount: projectSubscriptionCount,
	}
}

func notificationChannelFromListRow(row db.ListNotificationChannelsRow) db.NotificationChannel {
	return db.NotificationChannel{
		ID:         row.ID,
		ProjectID:  row.ProjectID,
		OwnerID:    row.OwnerID,
		Kind:       row.Kind,
		Config:     row.Config,
		Label:      row.Label,
		VerifiedAt: row.VerifiedAt,
		CreatedAt:  row.CreatedAt,
		DeletedAt:  row.DeletedAt,
	}
}

func nullableUUID(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := uuid.UUID(value.Bytes)
	return &id
}

func redactedNotificationConfig(channel db.NotificationChannel) json.RawMessage {
	switch channel.Kind {
	case notification.KindEmail:
		var cfg notification.EmailConfig
		_ = json.Unmarshal(channel.Config, &cfg)
		redacted, _ := json.Marshal(map[string]string{"redacted_to": cfg.RedactedTo})
		return redacted
	default:
		var cfg notification.WebhookConfig
		_ = json.Unmarshal(channel.Config, &cfg)
		redacted, _ := json.Marshal(map[string]string{"redacted_url": cfg.RedactedURL})
		return redacted
	}
}

func notificationDeliveryTarget(channel db.NotificationChannel, secret string, payload json.RawMessage) (notification.DeliveryTarget, error) {
	target := notification.DeliveryTarget{Kind: channel.Kind, Payload: payload}
	switch channel.Kind {
	case notification.KindSlackWebhook, notification.KindDiscordWebhook:
		var cfg notification.WebhookConfig
		if err := json.Unmarshal(channel.Config, &cfg); err != nil {
			return notification.DeliveryTarget{}, err
		}
		destination, err := notification.DecryptWebhookURL(cfg, secret)
		if err != nil {
			return notification.DeliveryTarget{}, err
		}
		target.Destination = destination
		return target, nil
	case notification.KindEmail:
		var cfg notification.EmailConfig
		if err := json.Unmarshal(channel.Config, &cfg); err != nil {
			return notification.DeliveryTarget{}, err
		}
		destination, err := notification.DecryptEmailTo(cfg, secret)
		if err != nil {
			return notification.DeliveryTarget{}, err
		}
		target.Destination = destination
		return target, nil
	default:
		return notification.DeliveryTarget{}, fmt.Errorf("unsupported notification channel kind %q", channel.Kind)
	}
}

func supportedNotificationEvents() []notificationEventDTO {
	types := []string{
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
