package admin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/llm"
)

func TestStatusFromCredentialsMasksAPIKey(t *testing.T) {
	updatedAt := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	status := StatusFromCredentials(&Credentials{
		Provider:    ProviderTokenGate,
		APIKey:      "sk-live-secret-abcdef",
		BaseURL:     "https://tokengate.example/v1",
		Model:       "tg-default",
		WriterModel: "tg-writer",
		QAModel:     "tg-qa",
		UpdatedAt:   updatedAt,
	})

	if !status.Configured {
		t.Fatal("status should be configured")
	}
	if status.Provider != "tokengate" {
		t.Fatalf("provider = %q", status.Provider)
	}
	if status.KeyTail != "cdef" {
		t.Fatalf("key tail = %q", status.KeyTail)
	}
	if status.BaseURL != "https://tokengate.example/v1" {
		t.Fatalf("base url = %q", status.BaseURL)
	}
	if status.Model != "tg-default" || status.WriterModel != "tg-writer" || status.QAModel != "tg-qa" {
		t.Fatalf("models = default:%q writer:%q qa:%q", status.Model, status.WriterModel, status.QAModel)
	}
	raw, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sk-live-secret") {
		t.Fatalf("status leaked the raw api key: %s", raw)
	}
}

func TestApplyUpdatePreservesExistingKeyAndModelsWhenBlank(t *testing.T) {
	next, err := ApplyUpdate(&Credentials{
		Provider:    ProviderTokenGate,
		APIKey:      "tg-existing-key",
		BaseURL:     "https://tokengate.example/v1",
		Model:       "tg-default",
		WriterModel: "tg-writer",
		QAModel:     "tg-qa",
	}, UpdateInput{
		Provider:    "claude",
		APIKey:      "   ",
		Model:       "   ",
		WriterModel: "   ",
		QAModel:     "   ",
	})
	if err != nil {
		t.Fatal(err)
	}

	if next.APIKey != "tg-existing-key" {
		t.Fatalf("api key = %q", next.APIKey)
	}
	if next.Provider != ProviderTokenGate {
		t.Fatalf("provider = %q", next.Provider)
	}
	if next.Model != "tg-default" || next.WriterModel != "tg-writer" || next.QAModel != "tg-qa" {
		t.Fatalf("models = default:%q writer:%q qa:%q", next.Model, next.WriterModel, next.QAModel)
	}
}

func TestApplyUpdateForcesTokenGateProviderAndModels(t *testing.T) {
	next, err := ApplyUpdate(nil, UpdateInput{
		Provider:    "openai",
		APIKey:      "tg-new-key",
		BaseURL:     " https://tokengate-production.up.railway.app/v1/ ",
		Model:       " gpt-5.1 ",
		WriterModel: " gpt-5.1 ",
		QAModel:     " gpt-5.5 ",
	})
	if err != nil {
		t.Fatal(err)
	}

	if next.Provider != ProviderTokenGate {
		t.Fatalf("provider = %q", next.Provider)
	}
	if next.BaseURL != "https://tokengate-production.up.railway.app/v1" {
		t.Fatalf("base url = %q", next.BaseURL)
	}
	if next.Model != "gpt-5.1" {
		t.Fatalf("model = %q", next.Model)
	}
	if next.WriterModel != "gpt-5.1" {
		t.Fatalf("writer model = %q", next.WriterModel)
	}
	if next.QAModel != "gpt-5.5" {
		t.Fatalf("qa model = %q", next.QAModel)
	}
}

func TestApplyUpdatePreservesExistingBaseURLWhenBlank(t *testing.T) {
	next, err := ApplyUpdate(&Credentials{
		Provider: ProviderTokenGate,
		APIKey:   "tg-existing-key",
		BaseURL:  "https://tokengate.example/v1",
	}, UpdateInput{
		Provider: "tokengate",
		APIKey:   "   ",
		BaseURL:  "   ",
	})
	if err != nil {
		t.Fatal(err)
	}

	if next.APIKey != "tg-existing-key" {
		t.Fatalf("api key = %q", next.APIKey)
	}
	if next.BaseURL != "https://tokengate.example/v1" {
		t.Fatalf("base url = %q", next.BaseURL)
	}
}

func TestProviderFromCredentialsUsesTokenGateBaseURL(t *testing.T) {
	provider := ProviderFromCredentials(Credentials{
		Provider: ProviderTokenGate,
		APIKey:   "tg-test-key",
		BaseURL:  "https://tokengate.example/v1",
	}, config.Env{TokenGateModel: "claude-sonnet-4-6"})

	openai, ok := provider.(*llm.OpenAIChat)
	if !ok {
		t.Fatalf("provider = %T, want *llm.OpenAIChat", provider)
	}
	if openai.APIKey != "tg-test-key" {
		t.Fatalf("api key = %q", openai.APIKey)
	}
	if openai.BaseURL != "https://tokengate.example/v1" {
		t.Fatalf("base url = %q", openai.BaseURL)
	}
}

func TestModelForRequestRoutesWriterAndQA(t *testing.T) {
	cred := Credentials{
		Model:       "gpt-5.1",
		WriterModel: "gpt-5.1",
		QAModel:     "gpt-5.5",
	}
	env := config.Env{TokenGateModel: "env-default"}

	if got := modelForRequest(cred, env, llm.CompletionReq{}); got != "gpt-5.1" {
		t.Fatalf("default model = %q", got)
	}
	if got := modelForRequest(cred, env, llm.CompletionReq{Purpose: llm.PurposeWriter}); got != "gpt-5.1" {
		t.Fatalf("writer model = %q", got)
	}
	if got := modelForRequest(cred, env, llm.CompletionReq{Purpose: llm.PurposeQA}); got != "gpt-5.5" {
		t.Fatalf("qa model = %q", got)
	}
}

func TestApplyUpdateStoresFourRoleModelRoutes(t *testing.T) {
	next, err := ApplyUpdate(nil, UpdateInput{
		Provider: "tokengate",
		APIKey:   "tg-new-key",
		BaseURL:  "https://tokengate.example/v1",
		Routes: ModelRoutes{
			string(RolePlanning): {
				PrimaryProvider:     ModelProviderOpenAI,
				OpenAIModelAlias:    "gpt-5.1",
				AnthropicModelAlias: "claude-sonnet-4-6",
				FallbackEnabled:     true,
			},
			string(RoleWriter): {
				PrimaryProvider:     ModelProviderOpenAI,
				OpenAIModelAlias:    "gpt-5.1",
				AnthropicModelAlias: "claude-sonnet-4-6",
				FallbackEnabled:     true,
			},
			string(RoleQA): {
				PrimaryProvider:     ModelProviderOpenAI,
				OpenAIModelAlias:    "gpt-5.5",
				AnthropicModelAlias: "claude-opus-4-8",
				FallbackEnabled:     true,
			},
			string(RoleSiteFix): {
				PrimaryProvider:     ModelProviderAnthropic,
				OpenAIModelAlias:    "gpt-5.1",
				AnthropicModelAlias: "claude-opus-4-8",
				FallbackEnabled:     false,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(next.Routes) != 4 {
		t.Fatalf("routes = %#v", next.Routes)
	}
	if next.Model != "gpt-5.1" || next.WriterModel != "gpt-5.1" || next.QAModel != "gpt-5.5" {
		t.Fatalf("legacy selected models = default:%q writer:%q qa:%q", next.Model, next.WriterModel, next.QAModel)
	}
	siteFix := next.Routes[string(RoleSiteFix)]
	if siteFix.PrimaryProvider != ModelProviderAnthropic || siteFix.AnthropicModelAlias != "claude-opus-4-8" || siteFix.FallbackEnabled {
		t.Fatalf("site fix route = %#v", siteFix)
	}
}

func TestRuntimeRouteForRequestRoutesSiteFixAndFallbackPolicy(t *testing.T) {
	cred := Credentials{
		Model:       "gpt-5.1",
		WriterModel: "gpt-5.1",
		QAModel:     "gpt-5.5",
		Routes: ModelRoutes{
			string(RoleSiteFix): {
				PrimaryProvider:     ModelProviderAnthropic,
				OpenAIModelAlias:    "gpt-5.1",
				AnthropicModelAlias: "claude-opus-4-8",
				FallbackEnabled:     false,
			},
		},
	}
	target := runtimeRouteForRequest(cred, config.Env{TokenGateModel: "env-default"}, llm.CompletionReq{Purpose: llm.PurposeSiteFix})

	if target.Role != RoleSiteFix {
		t.Fatalf("role = %q", target.Role)
	}
	if target.ModelAlias != "claude-opus-4-8" {
		t.Fatalf("model alias = %q", target.ModelAlias)
	}
	if target.FallbackEnabled {
		t.Fatal("site fix fallback should stay disabled when configured off")
	}
}

func TestRuntimeProbeTargetsCoverPlanningWriterQASiteFix(t *testing.T) {
	targets := RuntimeProbeTargets(Credentials{}, config.Env{TokenGateModel: "env-default"})
	roles := map[ModelRole]bool{}
	for _, target := range targets {
		roles[target.Role] = true
		if target.ModelAlias == "" {
			t.Fatalf("target %s missing model alias: %#v", target.Role, target)
		}
	}
	for _, role := range []ModelRole{RolePlanning, RoleWriter, RoleQA, RoleSiteFix} {
		if !roles[role] {
			t.Fatalf("probe targets missing role %s: %#v", role, targets)
		}
	}
}

func TestCredentialsWithRouteOverridesProbesUnsavedSelection(t *testing.T) {
	saved := Credentials{APIKey: "tg-key"}
	saved.Routes = defaultModelRoutes("", "", "")

	overridden := CredentialsWithRouteOverrides(saved, ModelRoutes{
		string(RolePlanning): {
			PrimaryProvider:     ModelProviderAnthropic,
			AnthropicModelAlias: "claude-sonnet-4-6",
			FallbackEnabled:     true,
		},
	})

	targets := RuntimeProbeTargets(overridden, config.Env{})
	var planning RuntimeProbeTarget
	for _, target := range targets {
		if target.Role == RolePlanning {
			planning = target
		}
	}
	if planning.Provider != ModelProviderAnthropic {
		t.Fatalf("planning provider = %q, want anthropic", planning.Provider)
	}
	if planning.ModelAlias != "claude-sonnet-4-6" {
		t.Fatalf("planning model alias = %q", planning.ModelAlias)
	}

	// Saved credentials stay untouched and empty overrides are a no-op.
	if saved.Routes[string(RolePlanning)].PrimaryProvider != ModelProviderOpenAI {
		t.Fatalf("saved planning provider mutated: %#v", saved.Routes[string(RolePlanning)])
	}
	unchanged := CredentialsWithRouteOverrides(saved, nil)
	if unchanged.Routes[string(RolePlanning)].PrimaryProvider != ModelProviderOpenAI {
		t.Fatalf("no-op override changed provider: %#v", unchanged.Routes[string(RolePlanning)])
	}
}

func TestModelForRequestFallsBackToDefaultThenEnv(t *testing.T) {
	env := config.Env{TokenGateModel: "env-default"}

	if got := modelForRequest(Credentials{Model: "gpt-5.1"}, env, llm.CompletionReq{Purpose: llm.PurposeWriter}); got != "gpt-5.1" {
		t.Fatalf("writer fallback model = %q", got)
	}
	if got := modelForRequest(Credentials{}, env, llm.CompletionReq{Purpose: llm.PurposeQA}); got != "env-default" {
		t.Fatalf("env fallback model = %q", got)
	}
}
