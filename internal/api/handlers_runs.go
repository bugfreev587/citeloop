package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	limit := int32(50)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeErr(w, http.StatusBadRequest, "bad limit")
			return
		}
		if n > 100 {
			n = 100
		}
		limit = int32(n)
	}
	var cursor time.Time
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		cursor, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad cursor")
			return
		}
	}
	runs, err := s.Q.ListGenerationRuns(r.Context(), db.ListGenerationRunsParams{
		ProjectID:       projectID,
		Agent:           r.URL.Query().Get("agent"),
		Status:          r.URL.Query().Get("status"),
		CursorCreatedAt: pgutil.TS(cursor),
		LimitRows:       limit,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad run id")
		return
	}
	run, err := s.Q.GetGenerationRun(r.Context(), db.GetGenerationRunParams{ID: runID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "run not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}
