package api

import (
	"net/http"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const localDevOwnerID = "default"

func (s *Server) ownerID(r *http.Request) string {
	claims, ok := clerk.SessionClaimsFromContext(r.Context())
	if ok && claims != nil && strings.TrimSpace(claims.Subject) != "" {
		return strings.TrimSpace(claims.Subject)
	}
	if s.Env.ClerkSecretKey == "" {
		return localDevOwnerID
	}
	return ""
}

func (s *Server) isAdmin(r *http.Request) bool {
	if s.Env.ClerkSecretKey == "" {
		return true
	}
	claims, ok := clerk.SessionClaimsFromContext(r.Context())
	if !ok || claims == nil {
		return false
	}
	subject := strings.TrimSpace(claims.Subject)
	for _, id := range strings.Split(s.Env.AdminUserIDs, ",") {
		if strings.TrimSpace(id) == subject && subject != "" {
			return true
		}
	}
	return claims.HasRole("org:admin") || claims.HasRole("admin") || claims.HasPermission("org:admin")
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdmin(r) {
			writeErr(w, http.StatusForbidden, "admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireProjectOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		projectID, err := uuid.Parse(chi.URLParam(r, "projectID"))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad project id")
			return
		}
		ownerID := s.ownerID(r)
		if ownerID == "" {
			writeErr(w, http.StatusForbidden, "project owner required")
			return
		}
		if s.Q == nil {
			next.ServeHTTP(w, r)
			return
		}
		if _, err := s.Q.GetProjectForOwner(r.Context(), db.GetProjectForOwnerParams{ID: projectID, OwnerID: ownerID}); err != nil {
			writeErr(w, http.StatusNotFound, "project not found")
			return
		}
		next.ServeHTTP(w, r)
	})
}
