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

func TestEmailIsAdminMatchesAllowlistCaseInsensitively(t *testing.T) {
	const allow = "owner@unipost.dev, Admin@Example.com"
	if !emailIsAdmin("admin@example.com", allow) {
		t.Fatal("listed email (any case) must be admin")
	}
	if !emailIsAdmin("owner@unipost.dev", allow) {
		t.Fatal("listed email must be admin")
	}
	if emailIsAdmin("stranger@example.com", allow) {
		t.Fatal("unlisted email must not be admin")
	}
	if emailIsAdmin("", allow) || emailIsAdmin("admin@example.com", "") {
		t.Fatal("empty email or empty allowlist must not be admin")
	}
}

func TestAdminAccessUsesEmailAllowlistOrAdminRoleWhenClerkIsConfigured(t *testing.T) {
	srv := &Server{
		Env: config.Env{ClerkSecretKey: "sk_test", AdminEmails: "admin@example.com"},
		emailResolver: func(_ context.Context, subject string) string {
			switch subject {
			case "user_admin":
				return "admin@example.com"
			case "user_member":
				return "member@example.com"
			}
			return ""
		},
	}

	if srv.isAdmin(requestWithClaims("user_member", "")) {
		t.Fatal("ordinary Clerk user must not be an admin")
	}
	if !srv.isAdmin(requestWithClaims("user_admin", "")) {
		t.Fatal("admin-email user must be an admin")
	}
	if !srv.isAdmin(requestWithClaims("user_org_admin", "org:admin")) {
		t.Fatal("Clerk org admin role must be accepted")
	}
}

func TestAdminAccessOpenInLocalDev(t *testing.T) {
	srv := &Server{}
	if !srv.isAdmin(requestWithClaims("anyone", "")) {
		t.Fatal("local dev (no Clerk key) should allow admin access")
	}
}
