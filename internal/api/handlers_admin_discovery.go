package api

import (
	"errors"
	"net/http"

	"github.com/citeloop/citeloop/internal/discovery"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Server) runAdminDiscoveryShadow(w http.ResponseWriter, r *http.Request) {
	projectID, ok := adminDiscoveryProjectID(w, r)
	if !ok {
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	repository := discovery.NewPostgresRepository(s.Q)
	service := discovery.NewService(repository)
	report, err := service.RunProject(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) getAdminDiscoveryShadowReport(w http.ResponseWriter, r *http.Request) {
	projectID, ok := adminDiscoveryProjectID(w, r)
	if !ok {
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	repository := discovery.NewPostgresRepository(s.Q)
	service := discovery.NewService(repository)
	report, err := service.LatestReport(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "discovery shadow report not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func adminDiscoveryProjectID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return uuid.Nil, false
	}
	return projectID, true
}
