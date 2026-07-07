package googledata

import (
	"context"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func SearchConsoleOAuthConfig(clientID, clientSecret, redirectURI string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURI,
		Scopes:       []string{ScopeSearchConsoleReadonly, ScopeAnalyticsReadonly},
		Endpoint:     google.Endpoint,
	}
}

func NewSearchConsoleOAuthClient(ctx context.Context, clientID, clientSecret, redirectURI string, token *oauth2.Token) Client {
	cfg := SearchConsoleOAuthConfig(clientID, clientSecret, redirectURI)
	return Client{HTTPClient: cfg.Client(ctx, token)}
}
