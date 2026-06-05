package notification

import (
	"strings"
	"testing"
)

func TestPrepareWebhookConfigEncryptsAndRedactsSlackURL(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"
	rawURL := "https://hooks.slack.com/services/T000/B000/secret-token"

	cfg, err := PrepareWebhookConfig("slack_webhook", rawURL, secret)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EncryptedURL == "" {
		t.Fatal("encrypted URL is empty")
	}
	if strings.Contains(cfg.EncryptedURL, "secret-token") {
		t.Fatalf("encrypted URL leaks plaintext: %s", cfg.EncryptedURL)
	}
	if cfg.RedactedURL != "https://hooks.slack.com/services/T000/B000/****" {
		t.Fatalf("redacted URL = %q", cfg.RedactedURL)
	}

	decrypted, err := DecryptWebhookURL(cfg, secret)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != rawURL {
		t.Fatalf("decrypted URL = %q", decrypted)
	}
}

func TestPrepareWebhookConfigRejectsMismatchedKindURL(t *testing.T) {
	_, err := PrepareWebhookConfig("slack_webhook", "https://discord.com/api/webhooks/1/token", "0123456789abcdef0123456789abcdef")
	if err == nil {
		t.Fatal("expected mismatched Slack URL to be rejected")
	}
}

func TestBudgetStoppedEventIDIncludesBudgetHash(t *testing.T) {
	a := BudgetStoppedEventID("project-1", "2026-06", "50")
	b := BudgetStoppedEventID("project-1", "2026-06", "75")
	c := BudgetStoppedEventID("project-1", "2026-06", "50")

	if a == b {
		t.Fatal("budget change must change event id")
	}
	if a != c {
		t.Fatal("same budget inputs must produce stable event id")
	}
	if !strings.HasPrefix(a, "budget.stopped:project-1:2026-06:") {
		t.Fatalf("unexpected event id prefix: %s", a)
	}
}
