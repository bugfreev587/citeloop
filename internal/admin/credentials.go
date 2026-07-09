// Package admin contains global administrator settings. These settings are not
// project-scoped because they affect the platform runtime itself.
package admin

import (
	"context"
	"encoding/json"
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

type ModelRole string

const (
	RolePlanning ModelRole = "planning"
	RoleWriter   ModelRole = "writer"
	RoleQA       ModelRole = "qa"
	RoleSiteFix  ModelRole = "site_fix"
)

type ModelProvider string

const (
	ModelProviderOpenAI    ModelProvider = "openai"
	ModelProviderAnthropic ModelProvider = "anthropic"
)

type ModelRoute struct {
	PrimaryProvider     ModelProvider `json:"primary_provider"`
	OpenAIModelAlias    string        `json:"openai_model_alias"`
	AnthropicModelAlias string        `json:"anthropic_model_alias"`
	FallbackEnabled     bool          `json:"fallback_enabled"`
}

type ModelRoutes map[string]ModelRoute

type RuntimeProbeTarget struct {
	Role            ModelRole `json:"role"`
	Label           string    `json:"label"`
	Purpose         llm.CompletionPurpose
	Provider        ModelProvider `json:"primary_provider"`
	ModelAlias      string        `json:"model_alias"`
	FallbackEnabled bool          `json:"fallback_enabled"`
}

type Credentials struct {
	Provider    Provider
	APIKey      string
	BaseURL     string
	Model       string
	WriterModel string
	QAModel     string
	Routes      ModelRoutes
	UpdatedAt   time.Time
}

type UpdateInput struct {
	Provider    string      `json:"provider"`
	APIKey      string      `json:"api_key"`
	BaseURL     string      `json:"base_url"`
	Model       string      `json:"model"`
	WriterModel string      `json:"writer_model"`
	QAModel     string      `json:"qa_model"`
	Routes      ModelRoutes `json:"routes"`
}

type Status struct {
	Provider    string      `json:"provider"`
	Configured  bool        `json:"configured"`
	KeyTail     string      `json:"key_tail,omitempty"`
	BaseURL     string      `json:"base_url,omitempty"`
	Model       string      `json:"model,omitempty"`
	WriterModel string      `json:"writer_model,omitempty"`
	QAModel     string      `json:"qa_model,omitempty"`
	Routes      ModelRoutes `json:"routes,omitempty"`
	UpdatedAt   *time.Time  `json:"updated_at,omitempty"`
}

func StatusFromCredentials(c *Credentials) Status {
	if c == nil {
		routes := defaultModelRoutes("", "", "")
		return Status{
			Provider:    string(ProviderTokenGate),
			Configured:  false,
			BaseURL:     DefaultTokenGateBaseURL,
			Model:       selectedModelForRole(routes, RolePlanning, ""),
			WriterModel: selectedModelForRole(routes, RoleWriter, ""),
			QAModel:     selectedModelForRole(routes, RoleQA, ""),
			Routes:      routes,
		}
	}
	updatedAt := c.UpdatedAt
	routes := normalizedRoutesForCredential(*c, config.Env{})
	return Status{
		Provider:    string(ProviderTokenGate),
		Configured:  c.APIKey != "",
		KeyTail:     keyTail(c.APIKey),
		BaseURL:     resolveCredentialBaseURL(c.BaseURL),
		Model:       selectedModelForRole(routes, RolePlanning, c.Model),
		WriterModel: selectedModelForRole(routes, RoleWriter, c.WriterModel),
		QAModel:     selectedModelForRole(routes, RoleQA, c.QAModel),
		Routes:      routes,
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
	legacyModel := nextModelValue(in.Model, existingModel(existing, func(c *Credentials) string { return c.Model }))
	legacyWriterModel := nextModelValue(in.WriterModel, existingModel(existing, func(c *Credentials) string { return c.WriterModel }))
	legacyQAModel := nextModelValue(in.QAModel, existingModel(existing, func(c *Credentials) string { return c.QAModel }))
	next.Routes = normalizeModelRoutes(in.Routes, existingModelRoutes(existing), legacyModel, legacyWriterModel, legacyQAModel)
	applyLegacyModelOverrides(next.Routes, in)
	next.Model = selectedModelForRole(next.Routes, RolePlanning, legacyModel)
	next.WriterModel = selectedModelForRole(next.Routes, RoleWriter, legacyWriterModel)
	next.QAModel = selectedModelForRole(next.Routes, RoleQA, legacyQAModel)
	return next, nil
}

func LoadCredentials(ctx context.Context, pool *pgxpool.Pool) (*Credentials, error) {
	var c Credentials
	var rawRoutes []byte
	err := pool.QueryRow(ctx, `
		select provider, api_key, base_url, model, writer_model, qa_model, model_routes, updated_at
		from admin_llm_credentials
		where singleton = true
	`).Scan(&c.Provider, &c.APIKey, &c.BaseURL, &c.Model, &c.WriterModel, &c.QAModel, &rawRoutes, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.Routes = modelRoutesFromRaw(rawRoutes, c.Model, c.WriterModel, c.QAModel)
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

	routesRaw, err := json.Marshal(next.Routes)
	if err != nil {
		return nil, err
	}

	var saved Credentials
	var savedRoutes []byte
	err = pool.QueryRow(ctx, `
		insert into admin_llm_credentials (singleton, provider, api_key, base_url, model, writer_model, qa_model, model_routes)
		values (true, $1, $2, $3, $4, $5, $6, $7::jsonb)
		on conflict (singleton) do update
		set provider = excluded.provider,
		    api_key = excluded.api_key,
		    base_url = excluded.base_url,
		    model = excluded.model,
		    writer_model = excluded.writer_model,
		    qa_model = excluded.qa_model,
		    model_routes = excluded.model_routes,
		    updated_at = now()
		returning provider, api_key, base_url, model, writer_model, qa_model, model_routes, updated_at
	`, next.Provider, next.APIKey, next.BaseURL, next.Model, next.WriterModel, next.QAModel, routesRaw).
		Scan(&saved.Provider, &saved.APIKey, &saved.BaseURL, &saved.Model, &saved.WriterModel, &saved.QAModel, &savedRoutes, &saved.UpdatedAt)
	if err != nil {
		return nil, err
	}
	saved.Routes = modelRoutesFromRaw(savedRoutes, saved.Model, saved.WriterModel, saved.QAModel)
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
	target := runtimeRouteForRequest(*cred, p.Env, req)
	req.Model = target.ModelAlias
	req.DisableProviderFallback = req.DisableProviderFallback || !target.FallbackEnabled
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
	return runtimeRouteForRequest(c, env, req).ModelAlias
}

func runtimeRouteForRequest(c Credentials, env config.Env, req llm.CompletionReq) RuntimeProbeTarget {
	role := roleForPurpose(req.Purpose)
	routes := normalizedRoutesForCredential(c, env)
	route := routeForRole(routes, role)
	model := selectedModel(route, envFallbackModel(env))
	return RuntimeProbeTarget{
		Role:            role,
		Label:           labelForRole(role),
		Purpose:         purposeForRole(role),
		Provider:        route.PrimaryProvider,
		ModelAlias:      model,
		FallbackEnabled: route.FallbackEnabled,
	}
}

// CredentialsWithRouteOverrides returns a copy of c with the given routes
// merged over the saved ones, so a connection test can probe route selections
// the admin has edited in the UI but not persisted yet.
func CredentialsWithRouteOverrides(c Credentials, routes ModelRoutes) Credentials {
	if len(routes) == 0 {
		return c
	}
	c.Routes = normalizeModelRoutes(routes, c.Routes, c.Model, c.WriterModel, c.QAModel)
	return c
}

func RuntimeProbeTargets(c Credentials, env config.Env) []RuntimeProbeTarget {
	targets := make([]RuntimeProbeTarget, 0, len(runtimeRoleOrder()))
	for _, role := range runtimeRoleOrder() {
		targets = append(targets, runtimeRouteForRequest(c, env, llm.CompletionReq{Purpose: purposeForRole(role)}))
	}
	return targets
}

func roleForPurpose(purpose llm.CompletionPurpose) ModelRole {
	switch purpose {
	case llm.PurposeWriter:
		return RoleWriter
	case llm.PurposeQA:
		return RoleQA
	case llm.PurposeSiteFix:
		return RoleSiteFix
	default:
		return RolePlanning
	}
}

func purposeForRole(role ModelRole) llm.CompletionPurpose {
	switch role {
	case RoleWriter:
		return llm.PurposeWriter
	case RoleQA:
		return llm.PurposeQA
	case RoleSiteFix:
		return llm.PurposeSiteFix
	default:
		return llm.PurposeDefault
	}
}

func labelForRole(role ModelRole) string {
	switch role {
	case RoleWriter:
		return "AI writer"
	case RoleQA:
		return "QA"
	case RoleSiteFix:
		return "Site Fix"
	default:
		return "Planning"
	}
}

func runtimeRoleOrder() []ModelRole {
	return []ModelRole{RolePlanning, RoleWriter, RoleQA, RoleSiteFix}
}

func normalizedRoutesForCredential(c Credentials, env config.Env) ModelRoutes {
	return normalizeModelRoutes(c.Routes, nil, firstNonBlank(c.Model, env.TokenGateModel), c.WriterModel, c.QAModel)
}

func normalizeModelRoutes(input ModelRoutes, existing ModelRoutes, model, writerModel, qaModel string) ModelRoutes {
	defaults := defaultModelRoutes(model, writerModel, qaModel)
	out := ModelRoutes{}
	for _, role := range runtimeRoleOrder() {
		key := string(role)
		route := defaults[key]
		if existingRoute, ok := existing[key]; ok {
			route = mergeModelRoute(route, existingRoute)
		}
		if inputRoute, ok := input[key]; ok {
			route = mergeModelRoute(route, inputRoute)
		}
		out[key] = normalizeModelRoute(role, route)
	}
	return out
}

func defaultModelRoutes(model, writerModel, qaModel string) ModelRoutes {
	return ModelRoutes{
		string(RolePlanning): {
			PrimaryProvider:     ModelProviderOpenAI,
			OpenAIModelAlias:    firstNonBlank(model, llm.DefaultOpenAIModel),
			AnthropicModelAlias: llm.DefaultTokenGateModel,
			FallbackEnabled:     true,
		},
		string(RoleWriter): {
			PrimaryProvider:     ModelProviderOpenAI,
			OpenAIModelAlias:    firstNonBlank(writerModel, model, llm.DefaultOpenAIWriterModel),
			AnthropicModelAlias: llm.DefaultTokenGateModel,
			FallbackEnabled:     true,
		},
		string(RoleQA): {
			PrimaryProvider:     ModelProviderOpenAI,
			OpenAIModelAlias:    firstNonBlank(qaModel, model, llm.DefaultOpenAIQAModel),
			AnthropicModelAlias: "claude-opus-4-8",
			FallbackEnabled:     true,
		},
		string(RoleSiteFix): {
			PrimaryProvider:     ModelProviderAnthropic,
			OpenAIModelAlias:    firstNonBlank(model, llm.DefaultOpenAIModel),
			AnthropicModelAlias: "claude-opus-4-8",
			FallbackEnabled:     false,
		},
	}
}

func normalizeModelRoute(role ModelRole, route ModelRoute) ModelRoute {
	defaultRoute := defaultModelRoutes("", "", "")[string(role)]
	route.PrimaryProvider = normalizeModelProvider(route.PrimaryProvider, defaultRoute.PrimaryProvider)
	route.OpenAIModelAlias = firstNonBlank(route.OpenAIModelAlias, defaultRoute.OpenAIModelAlias)
	route.AnthropicModelAlias = firstNonBlank(route.AnthropicModelAlias, defaultRoute.AnthropicModelAlias)
	return route
}

func normalizeModelProvider(provider ModelProvider, fallback ModelProvider) ModelProvider {
	switch provider {
	case ModelProviderOpenAI, ModelProviderAnthropic:
		return provider
	default:
		return fallback
	}
}

func mergeModelRoute(base, override ModelRoute) ModelRoute {
	if override.PrimaryProvider != "" {
		base.PrimaryProvider = override.PrimaryProvider
	}
	if strings.TrimSpace(override.OpenAIModelAlias) != "" {
		base.OpenAIModelAlias = strings.TrimSpace(override.OpenAIModelAlias)
	}
	if strings.TrimSpace(override.AnthropicModelAlias) != "" {
		base.AnthropicModelAlias = strings.TrimSpace(override.AnthropicModelAlias)
	}
	base.FallbackEnabled = override.FallbackEnabled
	return base
}

func applyLegacyModelOverrides(routes ModelRoutes, in UpdateInput) {
	applyLegacyModelOverride(routes, RolePlanning, in.Model)
	applyLegacyModelOverride(routes, RoleWriter, in.WriterModel)
	applyLegacyModelOverride(routes, RoleQA, in.QAModel)
}

func applyLegacyModelOverride(routes ModelRoutes, role ModelRole, value string) {
	model := strings.TrimSpace(value)
	if model == "" {
		return
	}
	key := string(role)
	route := routeForRole(routes, role)
	if route.PrimaryProvider == ModelProviderAnthropic {
		route.AnthropicModelAlias = model
	} else {
		route.OpenAIModelAlias = model
	}
	routes[key] = route
}

func routeForRole(routes ModelRoutes, role ModelRole) ModelRoute {
	if route, ok := routes[string(role)]; ok {
		return normalizeModelRoute(role, route)
	}
	return defaultModelRoutes("", "", "")[string(role)]
}

func selectedModelForRole(routes ModelRoutes, role ModelRole, fallback string) string {
	return selectedModel(routeForRole(routes, role), fallback)
}

func selectedModel(route ModelRoute, fallback string) string {
	if route.PrimaryProvider == ModelProviderAnthropic {
		return firstNonBlank(route.AnthropicModelAlias, route.OpenAIModelAlias, fallback)
	}
	return firstNonBlank(route.OpenAIModelAlias, route.AnthropicModelAlias, fallback)
}

func envFallbackModel(env config.Env) string {
	return firstNonBlank(env.TokenGateModel, llm.DefaultTokenGateModel)
}

func modelRoutesFromRaw(raw []byte, model, writerModel, qaModel string) ModelRoutes {
	if len(raw) == 0 {
		return defaultModelRoutes(model, writerModel, qaModel)
	}
	var routes ModelRoutes
	if err := json.Unmarshal(raw, &routes); err != nil {
		return defaultModelRoutes(model, writerModel, qaModel)
	}
	return normalizeModelRoutes(routes, nil, model, writerModel, qaModel)
}

func existingModelRoutes(existing *Credentials) ModelRoutes {
	if existing == nil {
		return nil
	}
	return existing.Routes
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
