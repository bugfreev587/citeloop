package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type adminProjectResponse struct {
	ID         uuid.UUID `json:"id"`
	OwnerID    string    `json:"owner_id"`
	OwnerEmail string    `json:"owner_email"`
	Name       string    `json:"name"`
	Slug       string    `json:"slug"`
	Config     any       `json:"config"`
	CreatedAt  any       `json:"created_at"`
}

func (s *Server) listAdminProjects(w http.ResponseWriter, r *http.Request) {
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	projects, err := s.Q.ListAdminProjects(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(s.adminProjectResponses(r, projects)))
}

func (s *Server) deleteAdminProject(w http.ResponseWriter, r *http.Request) {
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	project, err := s.Q.DeleteProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "project not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.adminProjectResponse(r, project))
}

func (s *Server) adminProjectResponses(r *http.Request, projects []db.Project) []adminProjectResponse {
	emails := make(map[string]string)
	out := make([]adminProjectResponse, 0, len(projects))
	for _, project := range projects {
		ownerID := strings.TrimSpace(project.OwnerID)
		if _, ok := emails[ownerID]; !ok {
			emails[ownerID] = s.userEmail(r.Context(), ownerID)
		}
		out = append(out, adminProjectResponseFor(project, emails[ownerID]))
	}
	return out
}

func (s *Server) adminProjectResponse(r *http.Request, project db.Project) adminProjectResponse {
	return adminProjectResponseFor(project, s.userEmail(r.Context(), strings.TrimSpace(project.OwnerID)))
}

func adminProjectResponseFor(project db.Project, ownerEmail string) adminProjectResponse {
	return adminProjectResponse{
		ID:         project.ID,
		OwnerID:    project.OwnerID,
		OwnerEmail: ownerEmail,
		Name:       project.Name,
		Slug:       project.Slug,
		Config:     project.Config,
		CreatedAt:  project.CreatedAt,
	}
}
