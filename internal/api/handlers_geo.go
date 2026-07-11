package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/citeloop/citeloop/internal/admin"
	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/discovery"
	geopkg "github.com/citeloop/citeloop/internal/geo"
	"github.com/citeloop/citeloop/internal/growthwork"
)

func (s *Server) geoService(ctx context.Context) geopkg.Service {
	var comparator discovery.SemanticComparator
	if s.LLM != nil {
		comparator = discovery.NewLLMSemanticComparator(s.LLM, "tokengate", s.Env.TokenGateModel)
	}
	service := geopkg.Service{Q: s.Q, GrowthWriter: growthwork.NewService(s.Pool, s.Q, comparator)}
	if provider := s.geoAnswerProvider(ctx); provider != nil {
		service.AnswerProvider = provider
	}
	return service
}

func (s *Server) geoAnswerProvider(ctx context.Context) geopkg.AnswerProvider {
	if s.Pool != nil {
		credentials, err := admin.LoadRuntimeGEOCredentials(ctx, s.Pool)
		if err == nil && credentials != nil {
			return tokenGateProviderFromGEOCredentials(*credentials)
		}
		if err != nil && s.Log != nil {
			s.Log.Warn("admin GEO credential unavailable", "err", err)
		}
		llmCredentials, err := admin.LoadCredentials(ctx, s.Pool)
		if err == nil && llmCredentials != nil && strings.TrimSpace(llmCredentials.APIKey) != "" {
			return tokenGateProviderFromLLMCredentials(*llmCredentials, s.Env)
		}
		if err != nil && s.Log != nil {
			s.Log.Warn("admin LLM credential unavailable for GEO fallback", "err", err)
		}
	}
	if s.Env.PerplexityAPIKey != "" {
		return geopkg.NewPerplexityProvider(s.Env.PerplexityAPIKey, s.Env.PerplexityBaseURL, s.Env.PerplexityModel, nil)
	}
	return nil
}

func tokenGateProviderFromLLMCredentials(credentials admin.Credentials, env config.Env) geopkg.TokenGateAnswerProvider {
	baseURL := strings.TrimSpace(credentials.BaseURL)
	if baseURL == "" {
		baseURL = env.TokenGateBaseURL
	}
	model := strings.TrimSpace(credentials.Model)
	if model == "" {
		model = env.TokenGateModel
	}
	return geopkg.NewTokenGateAnswerProvider(geopkg.TokenGateAnswerProviderConfig{
		Scope:   string(admin.GEOProviderOpenAI),
		APIKey:  credentials.APIKey,
		BaseURL: baseURL,
		Model:   model,
		Engine:  admin.GEOEngineForScope(admin.GEOProviderOpenAI),
	}, nil)
}

func (s *Server) aiDiscoveryObserveRequest() geopkg.ObserveAnswerProviderRequest {
	budgetUSD := s.Env.GEOProviderRunBudgetUSD
	if budgetUSD <= 0 {
		budgetUSD = 1
	}
	return geopkg.ObserveAnswerProviderRequest{
		Engine:     "OpenAI",
		MaxPrompts: 10,
		BudgetUSD:  budgetUSD,
	}
}

func (s *Server) runGEOCrawlerAudit(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database not configured")
		return
	}
	var in geopkg.CrawlerAuditRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	result, err := s.geoService(r.Context()).RunCrawlerAudit(r.Context(), projectID, in)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getLatestGEOCrawlerAudit(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database not configured")
		return
	}
	snapshots, err := s.geoService(r.Context()).LatestCrawlerAudit(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": emptySlice(snapshots)})
}
