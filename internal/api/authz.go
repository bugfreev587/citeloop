package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/clerk/clerk-sdk-go/v2"
	clerkuser "github.com/clerk/clerk-sdk-go/v2/user"
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

// isAdmin grants platform-admin access by the signed-in user's email matching the
// ADMINS allowlist (UniPost-style), or a Clerk org:admin role. Local dev (no Clerk
// key) is open. The email is resolved from the Clerk Backend API by subject.
func (s *Server) isAdmin(r *http.Request) bool {
	if s.Env.ClerkSecretKey == "" {
		return true
	}
	claims, ok := clerk.SessionClaimsFromContext(r.Context())
	if !ok || claims == nil {
		return false
	}
	if claims.HasRole("org:admin") || claims.HasRole("admin") || claims.HasPermission("org:admin") {
		return true
	}
	email := s.userEmail(r.Context(), strings.TrimSpace(claims.Subject))
	return emailIsAdmin(email, s.Env.AdminEmails)
}

// userEmail resolves the signed-in user's primary email. emailResolver is an
// injection seam for tests; in production it falls back to the Clerk Backend API.
func (s *Server) userEmail(ctx context.Context, subject string) string {
	if subject == "" {
		return ""
	}
	if s.emailResolver != nil {
		return s.emailResolver(ctx, subject)
	}
	u, err := clerkuser.Get(ctx, subject)
	if err != nil || u == nil {
		return ""
	}
	return primaryEmail(u)
}

func primaryEmail(u *clerk.User) string {
	if u.PrimaryEmailAddressID != nil {
		for _, e := range u.EmailAddresses {
			if e != nil && e.ID == *u.PrimaryEmailAddressID {
				return e.EmailAddress
			}
		}
	}
	for _, e := range u.EmailAddresses {
		if e != nil && e.EmailAddress != "" {
			return e.EmailAddress
		}
	}
	return ""
}

// emailIsAdmin matches an email (case-insensitive) against the comma-separated
// ADMINS allowlist.
func emailIsAdmin(email, adminEmails string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	for _, a := range strings.Split(adminEmails, ",") {
		if a = strings.ToLower(strings.TrimSpace(a)); a != "" && a == email {
			return true
		}
	}
	return false
}

// me reports the current user's identity and admin status, for gating UI like the
// Admin entry in docs. Authenticated but not admin-gated.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	email := ""
	if claims, ok := clerk.SessionClaimsFromContext(r.Context()); ok && claims != nil {
		email = s.userEmail(r.Context(), strings.TrimSpace(claims.Subject))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":  s.ownerID(r),
		"email":    email,
		"is_admin": s.isAdmin(r),
	})
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
