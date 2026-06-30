package admin

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type GEOProviderScope string

const (
	GEOProviderPerplexity GEOProviderScope = "perplexity"
	GEOProviderOpenAI     GEOProviderScope = "openai"
	GEOProviderAnthropic  GEOProviderScope = "anthropic"
	GEOProviderGemini     GEOProviderScope = "gemini"
)

type GEOCredentials struct {
	Scope     GEOProviderScope
	Provider  Provider
	APIKey    string
	BaseURL   string
	Model     string
	Enabled   bool
	UpdatedAt time.Time
}

type GEOUpdateInput struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
	Enabled  *bool  `json:"enabled"`
}

type GEOStatus struct {
	Scope      string     `json:"scope"`
	Provider   string     `json:"provider"`
	Configured bool       `json:"configured"`
	Enabled    bool       `json:"enabled"`
	KeyTail    string     `json:"key_tail,omitempty"`
	BaseURL    string     `json:"base_url,omitempty"`
	Model      string     `json:"model,omitempty"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

func GEOCredentialScopes() []GEOProviderScope {
	return []GEOProviderScope{
		GEOProviderPerplexity,
		GEOProviderOpenAI,
		GEOProviderAnthropic,
		GEOProviderGemini,
	}
}

func RuntimeGEOCredentialScopes() []GEOProviderScope {
	return []GEOProviderScope{
		GEOProviderOpenAI,
		GEOProviderAnthropic,
		GEOProviderPerplexity,
	}
}

func ValidGEOCredentialScope(value string) bool {
	_, err := ParseGEOCredentialScope(value)
	return err == nil
}

func ParseGEOCredentialScope(value string) (GEOProviderScope, error) {
	scope := GEOProviderScope(strings.ToLower(strings.TrimSpace(value)))
	for _, allowed := range GEOCredentialScopes() {
		if scope == allowed {
			return scope, nil
		}
	}
	return "", errors.New("unsupported GEO provider scope")
}

func DefaultGEOModel(scope GEOProviderScope) string {
	switch scope {
	case GEOProviderPerplexity:
		return "sonar-pro"
	case GEOProviderOpenAI:
		return "gpt-5.1"
	case GEOProviderAnthropic:
		return "claude-sonnet-4-6"
	case GEOProviderGemini:
		return "gemini-2.5-pro"
	default:
		return ""
	}
}

func GEOStatusForScope(scope GEOProviderScope, c *GEOCredentials) GEOStatus {
	if c == nil {
		return GEOStatus{
			Scope:      string(scope),
			Provider:   string(ProviderTokenGate),
			Configured: false,
			Enabled:    false,
			BaseURL:    DefaultTokenGateBaseURL,
			Model:      DefaultGEOModel(scope),
		}
	}
	return GEOStatusFromCredential(c)
}

func GEOStatusFromCredential(c *GEOCredentials) GEOStatus {
	if c == nil {
		return GEOStatus{Provider: string(ProviderTokenGate), BaseURL: DefaultTokenGateBaseURL}
	}
	updatedAt := c.UpdatedAt
	return GEOStatus{
		Scope:      string(c.Scope),
		Provider:   string(ProviderTokenGate),
		Configured: strings.TrimSpace(c.APIKey) != "",
		Enabled:    c.Enabled,
		KeyTail:    keyTail(c.APIKey),
		BaseURL:    resolveCredentialBaseURL(c.BaseURL),
		Model:      firstNonBlank(c.Model, DefaultGEOModel(c.Scope)),
		UpdatedAt:  &updatedAt,
	}
}

func ApplyGEOUpdate(existing *GEOCredentials, in GEOUpdateInput) (GEOCredentials, error) {
	scope := GEOProviderScope("")
	if existing != nil {
		scope = existing.Scope
	}
	next := GEOCredentials{Scope: scope, Provider: ProviderTokenGate, APIKey: strings.TrimSpace(in.APIKey)}
	if next.APIKey == "" {
		if existing == nil || strings.TrimSpace(existing.APIKey) == "" {
			return GEOCredentials{}, errors.New("api_key required")
		}
		next.APIKey = existing.APIKey
	}
	baseURL, err := normalizeBaseURL(in.BaseURL)
	if err != nil {
		return GEOCredentials{}, err
	}
	if baseURL == "" && existing != nil {
		baseURL = strings.TrimSpace(existing.BaseURL)
	}
	next.BaseURL = resolveCredentialBaseURL(baseURL)
	next.Model = nextModelValue(in.Model, existingGEOModel(existing))
	if next.Model == "" {
		next.Model = DefaultGEOModel(next.Scope)
	}
	next.Enabled = true
	if existing != nil {
		next.Enabled = existing.Enabled
	}
	if in.Enabled != nil {
		next.Enabled = *in.Enabled
	}
	return next, nil
}

func LoadGEOCredentials(ctx context.Context, pool *pgxpool.Pool, scope GEOProviderScope) (*GEOCredentials, error) {
	var c GEOCredentials
	err := pool.QueryRow(ctx, `
		select scope, provider, api_key, base_url, model, enabled, updated_at
		from admin_geo_provider_credentials
		where scope = $1
	`, scope).Scan(&c.Scope, &c.Provider, &c.APIKey, &c.BaseURL, &c.Model, &c.Enabled, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func ListGEOStatuses(ctx context.Context, pool *pgxpool.Pool) ([]GEOStatus, error) {
	rows, err := pool.Query(ctx, `
		select scope, provider, api_key, base_url, model, enabled, updated_at
		from admin_geo_provider_credentials
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byScope := map[GEOProviderScope]*GEOCredentials{}
	for rows.Next() {
		var c GEOCredentials
		if err := rows.Scan(&c.Scope, &c.Provider, &c.APIKey, &c.BaseURL, &c.Model, &c.Enabled, &c.UpdatedAt); err != nil {
			return nil, err
		}
		byScope[c.Scope] = &c
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]GEOStatus, 0, len(GEOCredentialScopes()))
	for _, scope := range GEOCredentialScopes() {
		out = append(out, GEOStatusForScope(scope, byScope[scope]))
	}
	return out, nil
}

func SaveGEOCredentials(ctx context.Context, pool *pgxpool.Pool, scope GEOProviderScope, in GEOUpdateInput) (*GEOCredentials, error) {
	existing, err := LoadGEOCredentials(ctx, pool, scope)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		existing = &GEOCredentials{Scope: scope, Provider: ProviderTokenGate, Enabled: true}
	}
	next, err := ApplyGEOUpdate(existing, in)
	if err != nil {
		return nil, err
	}
	next.Scope = scope

	var saved GEOCredentials
	err = pool.QueryRow(ctx, `
		insert into admin_geo_provider_credentials (scope, provider, api_key, base_url, model, enabled)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (scope) do update
		set provider = excluded.provider,
		    api_key = excluded.api_key,
		    base_url = excluded.base_url,
		    model = excluded.model,
		    enabled = excluded.enabled,
		    updated_at = now()
		returning scope, provider, api_key, base_url, model, enabled, updated_at
	`, next.Scope, next.Provider, next.APIKey, next.BaseURL, next.Model, next.Enabled).
		Scan(&saved.Scope, &saved.Provider, &saved.APIKey, &saved.BaseURL, &saved.Model, &saved.Enabled, &saved.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func DeleteGEOCredentials(ctx context.Context, pool *pgxpool.Pool, scope GEOProviderScope) error {
	_, err := pool.Exec(ctx, `delete from admin_geo_provider_credentials where scope = $1`, scope)
	return err
}

func LoadRuntimeGEOCredentials(ctx context.Context, pool *pgxpool.Pool) (*GEOCredentials, error) {
	candidates := make([]*GEOCredentials, 0, len(RuntimeGEOCredentialScopes()))
	for _, scope := range RuntimeGEOCredentialScopes() {
		credentials, err := LoadGEOCredentials(ctx, pool, scope)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, credentials)
	}
	return SelectRuntimeGEOCredentials(candidates), nil
}

func SelectRuntimeGEOCredentials(candidates []*GEOCredentials) *GEOCredentials {
	for _, credentials := range candidates {
		if credentials == nil || !credentials.Enabled {
			continue
		}
		if strings.TrimSpace(credentials.APIKey) == "" || strings.TrimSpace(credentials.Model) == "" {
			continue
		}
		return credentials
	}
	return nil
}

func GEOEngineForScope(scope GEOProviderScope) string {
	switch scope {
	case GEOProviderOpenAI:
		return "OpenAI"
	case GEOProviderAnthropic:
		return "Anthropic"
	case GEOProviderPerplexity:
		return "Perplexity"
	case GEOProviderGemini:
		return "Gemini"
	default:
		return strings.Title(string(scope))
	}
}

func existingGEOModel(existing *GEOCredentials) string {
	if existing == nil {
		return ""
	}
	return strings.TrimSpace(existing.Model)
}
