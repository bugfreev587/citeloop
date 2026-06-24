package googledata

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/oauth2/google"
)

const (
	ScopeSearchConsoleReadonly = "https://www.googleapis.com/auth/webmasters.readonly"
	scopeAnalyticsReadonly     = "https://www.googleapis.com/auth/analytics.readonly"
)

func NewServiceAccountClient(ctx context.Context, credentialsJSON string) (Client, error) {
	trimmed := strings.TrimSpace(credentialsJSON)
	if trimmed == "" {
		return Client{}, errors.New("google service account credentials are empty")
	}
	cfg, err := google.JWTConfigFromJSON([]byte(trimmed), ScopeSearchConsoleReadonly, scopeAnalyticsReadonly)
	if err != nil {
		return Client{}, err
	}
	return Client{HTTPClient: cfg.Client(ctx)}, nil
}
