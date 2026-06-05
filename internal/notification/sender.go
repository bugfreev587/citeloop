package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPSender struct {
	Client *http.Client
}

func (s HTTPSender) Send(ctx context.Context, kind, webhookURL string, payload json.RawMessage) error {
	body, err := webhookBody(kind, payload)
	if err != nil {
		return err
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
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
