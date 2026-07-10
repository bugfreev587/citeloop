package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	htmlpkg "html"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type HTTPSender struct {
	Client       *http.Client
	ResendAPIKey string
	EmailFrom    string
	EmailReplyTo string
	ResendURL    string
}

type DeliveryTarget struct {
	Kind        string
	Destination string
	DeliveryID  uuid.UUID
	Payload     json.RawMessage
}

func (s HTTPSender) Send(ctx context.Context, target DeliveryTarget) error {
	switch target.Kind {
	case KindSlackWebhook, KindDiscordWebhook:
		return s.sendWebhook(ctx, target)
	case KindEmail:
		return s.sendEmail(ctx, target)
	default:
		return fmt.Errorf("unsupported notification channel kind %q", target.Kind)
	}
}

func (s HTTPSender) sendWebhook(ctx context.Context, target DeliveryTarget) error {
	body, err := webhookBody(target.Kind, target.Payload)
	if err != nil {
		return err
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.Destination, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func (s HTTPSender) sendEmail(ctx context.Context, target DeliveryTarget) error {
	if strings.TrimSpace(s.ResendAPIKey) == "" {
		return fmt.Errorf("RESEND_API_KEY is required")
	}
	if strings.TrimSpace(s.EmailFrom) == "" {
		return fmt.Errorf("NOTIFICATION_EMAIL_FROM is required")
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	baseURL := strings.TrimRight(strings.TrimSpace(s.ResendURL), "/")
	if baseURL == "" {
		baseURL = "https://api.resend.com"
	}
	body, err := json.Marshal(emailBody(s.EmailFrom, s.EmailReplyTo, target.Destination, target.Payload))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(s.ResendAPIKey))
	req.Header.Set("Content-Type", "application/json")
	if target.DeliveryID != uuid.Nil {
		req.Header.Set("Idempotency-Key", target.DeliveryID.String())
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("resend returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

func webhookBody(kind string, payload json.RawMessage) ([]byte, error) {
	message := messageFromPayload(payload)
	switch kind {
	case KindSlackWebhook:
		return json.Marshal(map[string]string{"text": message})
	case KindDiscordWebhook:
		return json.Marshal(map[string]string{"content": message, "username": "CiteLoop"})
	default:
		return nil, fmt.Errorf("unsupported notification channel kind %q", kind)
	}
}

func emailBody(from, replyTo, to string, payload json.RawMessage) map[string]any {
	text := strings.TrimSpace(messageFromPayload(payload))
	if dashboardURL := dashboardURLFromPayload(payload); dashboardURL != "" {
		text += "\n\nOpen in CiteLoop: " + dashboardURL
	}
	body := map[string]any{
		"from":    strings.TrimSpace(from),
		"to":      []string{strings.TrimSpace(to)},
		"subject": emailSubject(payload),
		"text":    text,
		"html":    "<p>" + strings.ReplaceAll(htmlpkg.EscapeString(text), "\n", "<br>") + "</p>",
	}
	if strings.TrimSpace(replyTo) != "" {
		body["reply_to"] = strings.TrimSpace(replyTo)
	}
	return body
}

func emailSubject(payload json.RawMessage) string {
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err == nil {
		if title, ok := fields["title"].(string); ok && strings.TrimSpace(title) != "" {
			return "CiteLoop: " + strings.TrimSpace(title)
		}
	}
	return "CiteLoop notification"
}

func dashboardURLFromPayload(payload json.RawMessage) string {
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err == nil {
		if dashboardURL, ok := fields["dashboard_url"].(string); ok && strings.TrimSpace(dashboardURL) != "" {
			return strings.TrimSpace(dashboardURL)
		}
	}
	return ""
}

func messageFromPayload(payload json.RawMessage) string {
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err == nil {
		if message, ok := fields["message"].(string); ok && strings.TrimSpace(message) != "" {
			return message
		}
		parts := make([]string, 0, 2)
		if title, ok := fields["title"].(string); ok && strings.TrimSpace(title) != "" {
			parts = append(parts, title)
		}
		if errText, ok := fields["error"].(string); ok && strings.TrimSpace(errText) != "" {
			parts = append(parts, errText)
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	if len(payload) == 0 {
		return "CiteLoop notification"
	}
	return string(payload)
}
