package api

import (
	"net/http"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/go-chi/chi/v5"
)

type adminUserResponse struct {
	OwnerID      string `json:"owner_id"`
	OwnerEmail   string `json:"owner_email"`
	ProjectCount int64  `json:"project_count"`
	CreatedAt    any    `json:"created_at"`
	UpdatedAt    any    `json:"updated_at"`
}

type adminUserDeleteResponse struct {
	OwnerID         string `json:"owner_id"`
	OwnerEmail      string `json:"owner_email"`
	DeletedProjects int    `json:"deleted_projects"`
}

func (s *Server) listAdminUsers(w http.ResponseWriter, r *http.Request) {
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	users, err := s.Q.ListAdminUsers(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, emptySlice(s.adminUserResponses(r, users)))
}

func (s *Server) deleteAdminUser(w http.ResponseWriter, r *http.Request) {
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database unavailable")
		return
	}
	ownerID := strings.TrimSpace(chi.URLParam(r, "ownerID"))
	if ownerID == "" {
		writeErr(w, http.StatusBadRequest, "bad owner id")
		return
	}
	ownerEmail := s.userEmail(r.Context(), ownerID)
	projects, err := s.Q.DeleteProjectsByOwner(r.Context(), ownerID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(projects) == 0 {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, adminUserDeleteResponse{
		OwnerID:         ownerID,
		OwnerEmail:      ownerEmail,
		DeletedProjects: len(projects),
	})
}

func (s *Server) adminUserResponses(r *http.Request, users []db.ListAdminUsersRow) []adminUserResponse {
	out := make([]adminUserResponse, 0, len(users))
	for _, user := range users {
		ownerID := strings.TrimSpace(user.OwnerID)
		out = append(out, adminUserResponseFor(user, s.userEmail(r.Context(), ownerID)))
	}
	return out
}

func adminUserResponseFor(user db.ListAdminUsersRow, ownerEmail string) adminUserResponse {
	return adminUserResponse{
		OwnerID:      user.OwnerID,
		OwnerEmail:   ownerEmail,
		ProjectCount: user.ProjectCount,
		CreatedAt:    user.CreatedAt,
		UpdatedAt:    user.UpdatedAt,
	}
}
