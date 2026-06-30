package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/citeloop/citeloop/internal/admin"
	geopkg "github.com/citeloop/citeloop/internal/geo"
)

func (s *Server) geoService(ctx context.Context) geopkg.Service {
	service := geopkg.Service{Q: s.Q}
	if provider := s.geoAnswerProvider(ctx); provider != nil {
		service.AnswerProvider = provider
	}
	return service
}

func (s *Server) geoAnswerProvider(ctx context.Context) geopkg.AnswerProvider {
	if s.Pool != nil {
		credentials, err := admin.LoadGEOCredentials(ctx, s.Pool, admin.GEOProviderPerplexity)
		if err == nil && credentials != nil && credentials.Enabled && strings.TrimSpace(credentials.APIKey) != "" {
			return tokenGateProviderFromGEOCredentials(*credentials)
		}
		if err != nil && s.Log != nil {
			s.Log.Warn("admin GEO credential unavailable", "err", err)
		}
	}
	if s.Env.PerplexityAPIKey != "" {
		return geopkg.NewPerplexityProvider(s.Env.PerplexityAPIKey, s.Env.PerplexityBaseURL, s.Env.PerplexityModel, nil)
	}
	return nil
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
