package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	geopkg "github.com/citeloop/citeloop/internal/geo"
)

func (s *Server) geoService() geopkg.Service {
	service := geopkg.Service{Q: s.Q}
	if s.Env.PerplexityAPIKey != "" {
		service.AnswerProvider = geopkg.NewPerplexityProvider(s.Env.PerplexityAPIKey, s.Env.PerplexityBaseURL, s.Env.PerplexityModel, nil)
	}
	return service
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
	result, err := s.geoService().RunCrawlerAudit(r.Context(), projectID, in)
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
	snapshots, err := s.geoService().LatestCrawlerAudit(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"snapshots": emptySlice(snapshots)})
}
