package googledata

import (
	"context"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func SearchConsoleOAuthScopes() []string {
	return []string{ScopeSearchConsoleReadonly, ScopeAnalyticsReadonly}
}

func SearchConsoleOAuthScopeString() string {
	return strings.Join(SearchConsoleOAuthScopes(), " ")
}

func SearchConsoleOAuthConfig(clientID, clientSecret, redirectURI string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURI,
		Scopes:       SearchConsoleOAuthScopes(),
		Endpoint:     google.Endpoint,
	}
}

func NewSearchConsoleOAuthClient(ctx context.Context, clientID, clientSecret, redirectURI string, token *oauth2.Token) Client {
	cfg := SearchConsoleOAuthConfig(clientID, clientSecret, redirectURI)
	return Client{HTTPClient: cfg.Client(ctx, token)}
}
