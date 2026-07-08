package googledata

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/oauth2/google"
)

const (
	ScopeSearchConsoleReadonly = "https://www.googleapis.com/auth/webmasters.readonly"
	ScopeAnalyticsReadonly     = "https://www.googleapis.com/auth/analytics.readonly"
)

var ErrAnalyticsScopeMissing = errors.New("google analytics permission is missing; reconnect Google to grant Analytics read access")

func HasOAuthScope(rawScope string, required string) bool {
	required = strings.TrimSpace(required)
	if required == "" {
		return false
	}
	for _, scope := range strings.FieldsFunc(rawScope, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == ','
	}) {
		if scope == required {
			return true
		}
	}
	return false
}

func IsInsufficientAuthenticationScopes(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrAnalyticsScopeMissing) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "access_token_scope_insufficient") ||
		strings.Contains(msg, "insufficient authentication scopes")
}

func IsAnalyticsPropertyAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "user does not have sufficient permissions for this property")
}

func NewServiceAccountClient(ctx context.Context, credentialsJSON string) (Client, error) {
	trimmed := strings.TrimSpace(credentialsJSON)
	if trimmed == "" {
		return Client{}, errors.New("google service account credentials are empty")
	}
	cfg, err := google.JWTConfigFromJSON([]byte(trimmed), ScopeSearchConsoleReadonly, ScopeAnalyticsReadonly)
	if err != nil {
		return Client{}, err
	}
	return Client{HTTPClient: cfg.Client(ctx)}, nil
}
