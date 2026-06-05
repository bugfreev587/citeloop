// Package admin contains global administrator settings. These settings are not
// project-scoped because they affect the platform runtime itself.
package admin

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Provider string

const (
	ProviderTokenGate Provider = "tokengate"
	ProviderOpenAI    Provider = "openai"
	ProviderClaude    Provider = "claude"

	DefaultTokenGateBaseURL = "https://tokengate-production.up.railway.app/v1"
	DefaultOpenAIBaseURL    = "https://api.openai.com/v1"
)

type Credentials struct {
	Provider  Provider
	APIKey    string
	BaseURL   string
	UpdatedAt time.Time
}

type UpdateInput struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url"`
}

type Status struct {
	Provider   string     `json:"provider"`
	Configured bool       `json:"configured"`
	KeyTail    string     `json:"key_tail,omitempty"`
	BaseURL    string     `json:"base_url,omitempty"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}

func StatusFromCredentials(c *Credentials) Status {
	if c == nil {
		return Status{Provider: string(ProviderTokenGate), Configured: false, BaseURL: DefaultTokenGateBaseURL}
	}
	updatedAt := c.UpdatedAt
	return Status{
		Provider:   string(c.Provider),
		Configured: c.APIKey != "",
		KeyTail:    keyTail(c.APIKey),
		BaseURL:    resolveCredentialBaseURL(c.Provider, c.BaseURL),
		UpdatedAt:  &updatedAt,
	}
}

func ApplyUpdate(existing *Credentials, in UpdateInput) (Credentials, error) {
	provider, err := normalizeProvider(in.Provider)
	if err != nil {
		return Credentials{}, err
	}

	next := Credentials{Provider: provider, APIKey: strings.TrimSpace(in.APIKey)}
	if next.APIKey == "" {
		if existing == nil {
			return Credentials{}, errors.New("api_key required")
		}
		if existing.Provider != provider {
			return Credentials{}, errors.New("api_key required when changing provider")
		}
		next.APIKey = existing.APIKey
	}
	baseURL, err := normalizeBaseURL(provider, in.BaseURL)
	if err != nil {
		return Credentials{}, err
	}
	if baseURL == "" && existing != nil && existing.Provider == provider {
		baseURL = strings.TrimSpace(existing.BaseURL)
	}
	next.BaseURL = resolveCredentialBaseURL(provider, baseURL)
	return next, nil
}

func LoadCredentials(ctx context.Context, pool *pgxpool.Pool) (*Credentials, error) {
	var c Credentials
	err := pool.QueryRow(ctx, `
		select provider, api_key, base_url, updated_at
		from admin_llm_credentials
		where singleton = true
	`).Scan(&c.Provider, &c.APIKey, &c.BaseURL, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func SaveCredentials(ctx context.Context, pool *pgxpool.Pool, in UpdateInput) (*Credentials, error) {
	existing, err := LoadCredentials(ctx, pool)
	if err != nil {
		return nil, err
	}
	next, err := ApplyUpdate(existing, in)
	if err != nil {
		return nil, err
	}

	var saved Credentials
	err = pool.QueryRow(ctx, `
		insert into admin_llm_credentials (singleton, provider, api_key, base_url)
		values (true, $1, $2, $3)
		on conflict (singleton) do update
		set provider = excluded.provider,
		    api_key = excluded.api_key,
		    base_url = excluded.base_url,
		    updated_at = now()
		returning provider, api_key, base_url, updated_at
	`, next.Provider, next.APIKey, next.BaseURL).Scan(&saved.Provider, &saved.APIKey, &saved.BaseURL, &saved.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

type RuntimeProvider struct {
	Pool     *pgxpool.Pool
	Env      config.Env
	Fallback llm.Provider
}

func NewRuntimeProvider(pool *pgxpool.Pool, env config.Env, fallback llm.Provider) *RuntimeProvider {
	if fallback == nil {
		fallback = llm.NewMock()
	}
	return &RuntimeProvider{Pool: pool, Env: env, Fallback: fallback}
}

func (p *RuntimeProvider) Complete(ctx context.Context, req llm.CompletionReq) (llm.CompletionResp, error) {
	cred, err := LoadCredentials(ctx, p.Pool)
	if err != nil {
		return llm.CompletionResp{}, err
	}
	if cred == nil || cred.APIKey == "" {
		return p.Fallback.Complete(ctx, req)
	}
	return ProviderFromCredentials(*cred, p.Env).Complete(ctx, req)
}

func ProviderFromCredentials(c Credentials, env config.Env) llm.Provider {
	switch c.Provider {
	case ProviderClaude:
		return llm.NewClaude(c.APIKey, env.AnthropicModel)
	case ProviderOpenAI:
		return llm.NewOpenAIChat(c.APIKey, resolveCredentialBaseURL(ProviderOpenAI, c.BaseURL), env.TokenGateModel)
	default:
		baseURL := strings.TrimSpace(c.BaseURL)
		if baseURL == "" {
			baseURL = env.TokenGateBaseURL
		}
		return llm.NewOpenAIChat(c.APIKey, resolveCredentialBaseURL(ProviderTokenGate, baseURL), env.TokenGateModel)
	}
}

func normalizeProvider(value string) (Provider, error) {
	switch Provider(strings.ToLower(strings.TrimSpace(value))) {
	case "", ProviderTokenGate:
		return ProviderTokenGate, nil
	case ProviderOpenAI:
		return ProviderOpenAI, nil
	case ProviderClaude:
		return ProviderClaude, nil
	default:
		return "", errors.New("provider must be tokengate, openai, or claude")
	}
}

func normalizeBaseURL(provider Provider, value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if provider == ProviderClaude || value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("base_url must be a valid URL")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("base_url must use http or https")
	}
	return value, nil
}

func resolveCredentialBaseURL(provider Provider, value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	switch provider {
	case ProviderClaude:
		return ""
	case ProviderOpenAI:
		if value != "" {
			return value
		}
		return DefaultOpenAIBaseURL
	default:
		if value != "" {
			return value
		}
		return DefaultTokenGateBaseURL
	}
}

func keyTail(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4 {
		return value
	}
	return value[len(value)-4:]
}
