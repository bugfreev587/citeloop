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

func TestPrepareEmailConfigEncryptsRedactsAndHashesPerOwner(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"

	cfg, err := PrepareEmailConfig("owner-1", " Ops@Example.COM ", secret)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EncryptedTo == "" {
		t.Fatal("encrypted email is empty")
	}
	if strings.Contains(strings.ToLower(cfg.EncryptedTo), "ops@example.com") {
		t.Fatalf("encrypted email leaks plaintext: %s", cfg.EncryptedTo)
	}
	if cfg.RedactedTo != "o***@example.com" {
		t.Fatalf("redacted email = %q", cfg.RedactedTo)
	}
	decrypted, err := DecryptEmailTo(cfg, secret)
	if err != nil {
		t.Fatal(err)
	}
	if decrypted != "ops@example.com" {
		t.Fatalf("decrypted email = %q", decrypted)
	}

	sameOwner, err := PrepareEmailConfig("owner-1", "ops@example.com", secret)
	if err != nil {
		t.Fatal(err)
	}
	otherOwner, err := PrepareEmailConfig("owner-2", "ops@example.com", secret)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AddressHash != sameOwner.AddressHash {
		t.Fatal("same owner and normalized address must produce a stable hash")
	}
	if cfg.AddressHash == otherOwner.AddressHash {
		t.Fatal("same address under another owner must not produce the same hash")
	}
}

func TestPrepareEmailConfigRejectsInvalidEmail(t *testing.T) {
	_, err := PrepareEmailConfig("owner-1", "not-an-email", "0123456789abcdef0123456789abcdef")
	if err == nil {
		t.Fatal("expected invalid email to be rejected")
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
