package notification

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
)

const (
	KindSlackWebhook   = "slack_webhook"
	KindDiscordWebhook = "discord_webhook"
)

type WebhookConfig struct {
	EncryptedURL string `json:"encrypted_url"`
	RedactedURL  string `json:"redacted_url"`
}

func PrepareWebhookConfig(kind, rawURL, secret string) (WebhookConfig, error) {
	if err := validateWebhookURL(kind, rawURL); err != nil {
		return WebhookConfig{}, err
	}
	encrypted, err := encryptString(rawURL, secret)
	if err != nil {
		return WebhookConfig{}, err
	}
	return WebhookConfig{EncryptedURL: encrypted, RedactedURL: redactWebhookURL(rawURL)}, nil
}

func DecryptWebhookURL(cfg WebhookConfig, secret string) (string, error) {
	return decryptString(cfg.EncryptedURL, secret)
}

func (c WebhookConfig) JSON() json.RawMessage {
	b, _ := json.Marshal(c)
	return b
}

func BudgetStoppedEventID(projectID, period, budgetUSD string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(budgetUSD)))
	return fmt.Sprintf("budget.stopped:%s:%s:%s", projectID, period, hex.EncodeToString(sum[:])[:16])
}

func validateWebhookURL(kind, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return errors.New("webhook url must be a valid https URL")
	}
	switch kind {
	case KindSlackWebhook:
		if parsed.Host != "hooks.slack.com" || !strings.HasPrefix(parsed.Path, "/services/") {
			return errors.New("slack webhook url must start with https://hooks.slack.com/services/")
		}
	case KindDiscordWebhook:
		if (parsed.Host != "discord.com" && parsed.Host != "discordapp.com") || !strings.HasPrefix(parsed.Path, "/api/webhooks/") {
			return errors.New("discord webhook url must start with https://discord.com/api/webhooks/")
		}
	default:
		return errors.New("notification channel kind must be slack_webhook or discord_webhook")
	}
	return nil
}

func redactWebhookURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "****"
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) == 0 {
		parsed.Path = "/****"
		return parsed.String()
	}
	parts[len(parts)-1] = "****"
	return parsed.Scheme + "://" + parsed.Host + "/" + strings.Join(parts, "/")
}

func encryptString(plaintext, secret string) (string, error) {
	key := encryptionKey(secret)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}

func decryptString(ciphertext, secret string) (string, error) {
	raw, err := base64.RawStdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	key := encryptionKey(secret)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("encrypted webhook url is malformed")
	}
	nonce, sealed := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	opened, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", err
	}
	return string(opened), nil
}

func encryptionKey(secret string) []byte {
	secret = strings.TrimSpace(secret)
	if len(secret) == 32 {
		return []byte(secret)
	}
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}
