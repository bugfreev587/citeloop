package notification

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"net/url"
	"strings"
)

const (
	KindSlackWebhook   = "slack_webhook"
	KindDiscordWebhook = "discord_webhook"
	KindEmail          = "email"
)

type WebhookConfig struct {
	EncryptedURL string `json:"encrypted_url"`
	RedactedURL  string `json:"redacted_url"`
}

type EmailConfig struct {
	EncryptedTo string `json:"encrypted_to"`
	RedactedTo  string `json:"redacted_to"`
	AddressHash string `json:"address_hash"`
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

func PrepareEmailConfig(ownerID, rawEmail, secret string) (EmailConfig, error) {
	normalized, err := normalizeEmail(rawEmail)
	if err != nil {
		return EmailConfig{}, err
	}
	encrypted, err := encryptString(normalized, secret)
	if err != nil {
		return EmailConfig{}, err
	}
	return EmailConfig{
		EncryptedTo: encrypted,
		RedactedTo:  redactEmail(normalized),
		AddressHash: emailAddressHash(ownerID, normalized, secret),
	}, nil
}

func DecryptEmailTo(cfg EmailConfig, secret string) (string, error) {
	return decryptString(cfg.EncryptedTo, secret)
}

func (c WebhookConfig) JSON() json.RawMessage {
	b, _ := json.Marshal(c)
	return b
}

func (c EmailConfig) JSON() json.RawMessage {
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
		return errors.New("notification channel kind must be slack_webhook, discord_webhook, or email")
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

func normalizeEmail(rawEmail string) (string, error) {
	parsed, err := mail.ParseAddress(strings.TrimSpace(rawEmail))
	if err != nil {
		return "", errors.New("email address must be valid")
	}
	address := strings.ToLower(strings.TrimSpace(parsed.Address))
	local, domain, ok := strings.Cut(address, "@")
	if !ok || local == "" || domain == "" || strings.Contains(domain, " ") {
		return "", errors.New("email address must be valid")
	}
	return address, nil
}

func redactEmail(email string) string {
	local, domain, ok := strings.Cut(email, "@")
	if !ok || local == "" || domain == "" {
		return "****"
	}
	return local[:1] + "***@" + domain
}

func emailAddressHash(ownerID, email, secret string) string {
	mac := hmac.New(sha256.New, []byte(strings.TrimSpace(secret)))
	_, _ = mac.Write([]byte(strings.TrimSpace(ownerID)))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(mac.Sum(nil))
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
		return "", errors.New("encrypted notification destination is malformed")
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
