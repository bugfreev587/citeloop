package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/clerk/clerk-sdk-go/v2"
)

func requestWithClaims(subject string, role string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	claims := &clerk.SessionClaims{}
	claims.Subject = subject
	claims.ActiveOrganizationRole = role
	return req.WithContext(clerk.ContextWithSessionClaims(context.Background(), claims))
}

func TestOwnerIDUsesClerkSubjectWhenConfigured(t *testing.T) {
	srv := &Server{Env: config.Env{ClerkSecretKey: "sk_test"}}
	req := requestWithClaims("user_123", "")

	if got := srv.ownerID(req); got != "user_123" {
		t.Fatalf("ownerID = %q, want Clerk subject", got)
	}
}

func TestOwnerIDFallsBackToDefaultForLocalDev(t *testing.T) {
	srv := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)

	if got := srv.ownerID(req); got != "default" {
		t.Fatalf("ownerID = %q, want default", got)
	}
}

func TestAdminAccessRequiresAllowlistOrAdminRoleWhenClerkIsConfigured(t *testing.T) {
	srv := &Server{Env: config.Env{ClerkSecretKey: "sk_test", AdminUserIDs: "user_admin"}}

	if srv.isAdmin(requestWithClaims("user_member", "")) {
		t.Fatal("ordinary Clerk user must not be an admin")
	}
	if !srv.isAdmin(requestWithClaims("user_admin", "")) {
		t.Fatal("allowlisted Clerk user must be an admin")
	}
	if !srv.isAdmin(requestWithClaims("user_org_admin", "org:admin")) {
		t.Fatal("Clerk org admin role must be accepted")
	}
}
