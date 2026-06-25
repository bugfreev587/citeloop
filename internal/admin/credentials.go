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

	DefaultTokenGateBaseURL = "https://tokengate-production.up.railway.app/v1"
)

type Credentials struct {
	Provider    Provider
	APIKey      string
	BaseURL     string
	Model       string
	WriterModel string
	QAModel     string
	UpdatedAt   time.Time
}

type UpdateInput struct {
	Provider    string `json:"provider"`
	APIKey      string `json:"api_key"`
	BaseURL     string `json:"base_url"`
	Model       string `json:"model"`
	WriterModel string `json:"writer_model"`
	QAModel     string `json:"qa_model"`
}

type Status struct {
	Provider    string     `json:"provider"`
	Configured  bool       `json:"configured"`
	KeyTail     string     `json:"key_tail,omitempty"`
	BaseURL     string     `json:"base_url,omitempty"`
	Model       string     `json:"model,omitempty"`
	WriterModel string     `json:"writer_model,omitempty"`
	QAModel     string     `json:"qa_model,omitempty"`
	UpdatedAt   *time.Time `json:"updated_at,omitempty"`
}

func StatusFromCredentials(c *Credentials) Status {
	if c == nil {
		return Status{Provider: string(ProviderTokenGate), Configured: false, BaseURL: DefaultTokenGateBaseURL}
	}
	updatedAt := c.UpdatedAt
	return Status{
		Provider:    string(ProviderTokenGate),
		Configured:  c.APIKey != "",
		KeyTail:     keyTail(c.APIKey),
		BaseURL:     resolveCredentialBaseURL(c.BaseURL),
		Model:       strings.TrimSpace(c.Model),
		WriterModel: strings.TrimSpace(c.WriterModel),
		QAModel:     strings.TrimSpace(c.QAModel),
		UpdatedAt:   &updatedAt,
	}
}

func ApplyUpdate(existing *Credentials, in UpdateInput) (Credentials, error) {
	next := Credentials{Provider: ProviderTokenGate, APIKey: strings.TrimSpace(in.APIKey)}
	if next.APIKey == "" {
		if existing == nil {
			return Credentials{}, errors.New("api_key required")
		}
		next.APIKey = existing.APIKey
	}
	baseURL, err := normalizeBaseURL(in.BaseURL)
	if err != nil {
		return Credentials{}, err
	}
	if baseURL == "" && existing != nil {
		baseURL = strings.TrimSpace(existing.BaseURL)
	}
	next.BaseURL = resolveCredentialBaseURL(baseURL)
	next.Model = nextModelValue(in.Model, existingModel(existing, func(c *Credentials) string { return c.Model }))
	next.WriterModel = nextModelValue(in.WriterModel, existingModel(existing, func(c *Credentials) string { return c.WriterModel }))
	next.QAModel = nextModelValue(in.QAModel, existingModel(existing, func(c *Credentials) string { return c.QAModel }))
	return next, nil
}

func LoadCredentials(ctx context.Context, pool *pgxpool.Pool) (*Credentials, error) {
	var c Credentials
	err := pool.QueryRow(ctx, `
		select provider, api_key, base_url, model, writer_model, qa_model, updated_at
		from admin_llm_credentials
		where singleton = true
	`).Scan(&c.Provider, &c.APIKey, &c.BaseURL, &c.Model, &c.WriterModel, &c.QAModel, &c.UpdatedAt)
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
		insert into admin_llm_credentials (singleton, provider, api_key, base_url, model, writer_model, qa_model)
		values (true, $1, $2, $3, $4, $5, $6)
		on conflict (singleton) do update
		set provider = excluded.provider,
		    api_key = excluded.api_key,
		    base_url = excluded.base_url,
		    model = excluded.model,
		    writer_model = excluded.writer_model,
		    qa_model = excluded.qa_model,
		    updated_at = now()
		returning provider, api_key, base_url, model, writer_model, qa_model, updated_at
	`, next.Provider, next.APIKey, next.BaseURL, next.Model, next.WriterModel, next.QAModel).
		Scan(&saved.Provider, &saved.APIKey, &saved.BaseURL, &saved.Model, &saved.WriterModel, &saved.QAModel, &saved.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

// DeleteCredentials removes the saved admin LLM credential so the runtime falls
// back to the server-environment provider (or mock). Idempotent.
func DeleteCredentials(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `delete from admin_llm_credentials where singleton = true`)
	return err
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
	req.Model = modelForRequest(*cred, p.Env, req)
	return ProviderFromCredentials(*cred, p.Env).Complete(ctx, req)
}

func ProviderFromCredentials(c Credentials, env config.Env) llm.Provider {
	baseURL := strings.TrimSpace(c.BaseURL)
	if baseURL == "" {
		baseURL = env.TokenGateBaseURL
	}
	return llm.NewOpenAIChat(c.APIKey, resolveCredentialBaseURL(baseURL), firstNonBlank(c.Model, env.TokenGateModel))
}

func normalizeBaseURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
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

func resolveCredentialBaseURL(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value != "" {
		return value
	}
	return DefaultTokenGateBaseURL
}

func modelForRequest(c Credentials, env config.Env, req llm.CompletionReq) string {
	switch req.Purpose {
	case llm.PurposeWriter:
		return firstNonBlank(c.WriterModel, c.Model, env.TokenGateModel)
	case llm.PurposeQA:
		return firstNonBlank(c.QAModel, c.Model, env.TokenGateModel)
	default:
		return firstNonBlank(c.Model, env.TokenGateModel)
	}
}

func existingModel(existing *Credentials, get func(*Credentials) string) string {
	if existing == nil {
		return ""
	}
	return get(existing)
}

func nextModelValue(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(fallback)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func keyTail(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 4 {
		return value
	}
	return value[len(value)-4:]
}
