package api

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestGSCOAuthStateRoundTripsAndValidatesOwnerProjectAndExpiry(t *testing.T) {
	projectID := uuid.New()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	state, err := signGSCOAuthState("state-secret", gscOAuthStateClaims{
		ProjectID:   projectID,
		OwnerID:     "user_123",
		RedirectURI: "https://app.citeloop.test/integrations/google/search-console/callback",
		Nonce:       "nonce-1",
		ExpiresAt:   now.Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}

	claims, err := parseGSCOAuthState("state-secret", state, projectID, "user_123", now)
	if err != nil {
		t.Fatal(err)
	}
	if claims.ProjectID != projectID || claims.OwnerID != "user_123" || claims.Nonce != "nonce-1" {
		t.Fatalf("claims = %#v", claims)
	}
	if claims.RedirectURI != "https://app.citeloop.test/integrations/google/search-console/callback" {
		t.Fatalf("redirect uri = %q", claims.RedirectURI)
	}

	if _, err := parseGSCOAuthState("state-secret", state, uuid.New(), "user_123", now); err == nil {
		t.Fatal("expected wrong project to fail")
	}
	if _, err := parseGSCOAuthState("state-secret", state, projectID, "user_456", now); err == nil {
		t.Fatal("expected wrong owner to fail")
	}
	if _, err := parseGSCOAuthState("state-secret", state, projectID, "user_123", now.Add(11*time.Minute)); err == nil {
		t.Fatal("expected expired state to fail")
	}
}

func TestGSCOAuthStateRejectsTamperedPayload(t *testing.T) {
	projectID := uuid.New()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	state, err := signGSCOAuthState("state-secret", gscOAuthStateClaims{
		ProjectID:   projectID,
		OwnerID:     "user_123",
		RedirectURI: "https://app.citeloop.test/integrations/google/search-console/callback",
		Nonce:       "nonce-1",
		ExpiresAt:   now.Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}

	parts := strings.Split(state, ".")
	if len(parts) != 2 {
		t.Fatalf("state = %q, want two signed segments", state)
	}
	tampered := parts[0] + "x." + parts[1]
	if _, err := parseGSCOAuthState("state-secret", tampered, projectID, "user_123", now); err == nil {
		t.Fatal("expected tampered state to fail")
	}
}
