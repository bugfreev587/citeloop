package api

import (
	"errors"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/googledata"
	"github.com/citeloop/citeloop/internal/pgutil"
	seopkg "github.com/citeloop/citeloop/internal/seo"
	"github.com/google/uuid"
)

func TestSEOSettingsIntegrationParamsPreserveExistingWhenCredentialRefOmitted(t *testing.T) {
	projectID := uuid.New()
	verifiedAt := pgutil.TS(time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC))
	credentialRef := "seo_oauth_tokens:google_search_console"
	lastError := "previous transient error"
	existing := []db.SeoIntegration{{
		ProjectID:      projectID,
		Provider:       seopkg.ProviderGSC,
		Status:         "connected",
		CredentialRef:  &credentialRef,
		LastVerifiedAt: verifiedAt,
		LastError:      &lastError,
	}}

	params := seoSettingsIntegrationParams(projectID, seopkg.ProviderGSC, "", existing, time.Now(), nil)

	if params.Status != "connected" {
		t.Fatalf("status = %q, want connected", params.Status)
	}
	if params.CredentialRef == nil || *params.CredentialRef != credentialRef {
		t.Fatalf("credential ref = %v, want existing ref", params.CredentialRef)
	}
	if !params.LastVerifiedAt.Valid || !params.LastVerifiedAt.Time.Equal(verifiedAt.Time) {
		t.Fatalf("last verified = %v, want existing timestamp", params.LastVerifiedAt)
	}
	if params.LastError == nil || *params.LastError != lastError {
		t.Fatalf("last error = %v, want preserved error", params.LastError)
	}
}

func TestSEOSettingsIntegrationParamsConnectsExplicitCredentialRef(t *testing.T) {
	projectID := uuid.New()
	now := time.Date(2026, 7, 7, 13, 0, 0, 0, time.UTC)
	params := seoSettingsIntegrationParams(projectID, seopkg.ProviderGA4, "GOOGLE_SERVICE_ACCOUNT_JSON", nil, now, nil)

	if params.Status != "connected" {
		t.Fatalf("status = %q, want connected", params.Status)
	}
	if params.CredentialRef == nil || *params.CredentialRef != "GOOGLE_SERVICE_ACCOUNT_JSON" {
		t.Fatalf("credential ref = %v, want GOOGLE_SERVICE_ACCOUNT_JSON", params.CredentialRef)
	}
	if !params.LastVerifiedAt.Valid || !params.LastVerifiedAt.Time.Equal(now) {
		t.Fatalf("last verified = %v, want now", params.LastVerifiedAt)
	}
	if params.LastError != nil {
		t.Fatalf("last error = %v, want nil after explicit credential", *params.LastError)
	}
}

func TestSEOSettingsIntegrationParamsUsesFallbackCredentialForGA4(t *testing.T) {
	projectID := uuid.New()
	verifiedAt := pgutil.TS(time.Date(2026, 7, 7, 14, 0, 0, 0, time.UTC))
	credentialRef := "seo_oauth_tokens:google_search_console"
	existingGSC := db.SeoIntegration{
		ProjectID:      projectID,
		Provider:       seopkg.ProviderGSC,
		Status:         "connected",
		CredentialRef:  &credentialRef,
		LastVerifiedAt: verifiedAt,
	}

	params := seoSettingsIntegrationParams(projectID, seopkg.ProviderGA4, "", nil, time.Now(), &existingGSC)

	if params.Status != "connected" {
		t.Fatalf("status = %q, want connected", params.Status)
	}
	if params.CredentialRef == nil || *params.CredentialRef != credentialRef {
		t.Fatalf("credential ref = %v, want fallback ref", params.CredentialRef)
	}
	if !params.LastVerifiedAt.Valid || !params.LastVerifiedAt.Time.Equal(verifiedAt.Time) {
		t.Fatalf("last verified = %v, want fallback timestamp", params.LastVerifiedAt)
	}
	if params.LastError != nil {
		t.Fatalf("last error = %v, want nil", *params.LastError)
	}
}

func TestShouldClearGA4ScopeErrorAfterAnalyticsScopeGranted(t *testing.T) {
	lastError := `google api status 403: {"error":{"message":"Request had insufficient authentication scopes.","details":[{"reason":"ACCESS_TOKEN_SCOPE_INSUFFICIENT"}]}}`
	integration := db.SeoIntegration{
		Provider:  seopkg.ProviderGA4,
		Status:    "error",
		LastError: &lastError,
	}

	if !shouldClearGA4ScopeErrorAfterOAuth(googledata.SearchConsoleOAuthScopeString(), integration) {
		t.Fatal("expected Analytics scope reconnect to clear a stale GA4 scope error")
	}
}

func TestShouldNotClearNonScopeGA4ErrorAfterAnalyticsScopeGranted(t *testing.T) {
	lastError := "google api status 404: property not found"
	integration := db.SeoIntegration{
		Provider:  seopkg.ProviderGA4,
		Status:    "error",
		LastError: &lastError,
	}

	if shouldClearGA4ScopeErrorAfterOAuth(googledata.SearchConsoleOAuthScopeString(), integration) {
		t.Fatal("non-scope GA4 errors should not be cleared by OAuth reconnect")
	}
}

func TestGA4OAuthValidationConnectsAndClearsStalePropertyError(t *testing.T) {
	projectID := uuid.New()
	now := time.Date(2026, 7, 8, 6, 10, 0, 0, time.UTC)
	credentialRef := "seo_oauth_tokens:google_search_console"
	lastError := `google api status 403: { "error": { "message": "User does not have sufficient permissions for this property.", "status": "PERMISSION_DENIED" } }`
	existing := []db.SeoIntegration{{
		ProjectID:     projectID,
		Provider:      seopkg.ProviderGA4,
		Status:        "error",
		CredentialRef: &credentialRef,
		LastError:     &lastError,
	}}

	params, ok := ga4OAuthValidationIntegrationParams(projectID, googledata.SearchConsoleOAuthScopeString(), "544348649", existing, nil, now)

	if !ok {
		t.Fatal("expected Analytics OAuth validation to update GA4 integration")
	}
	if params.Status != "connected" {
		t.Fatalf("status = %q, want connected", params.Status)
	}
	if params.CredentialRef == nil || *params.CredentialRef != credentialRef {
		t.Fatalf("credential ref = %v, want %q", params.CredentialRef, credentialRef)
	}
	if !params.LastVerifiedAt.Valid || !params.LastVerifiedAt.Time.Equal(now) {
		t.Fatalf("last verified = %v, want %v", params.LastVerifiedAt, now)
	}
	if params.LastError != nil {
		t.Fatalf("last error = %q, want nil", *params.LastError)
	}
}

func TestGA4OAuthValidationMapsPropertyDeniedProbe(t *testing.T) {
	projectID := uuid.New()
	now := time.Date(2026, 7, 8, 6, 11, 0, 0, time.UTC)
	err := errors.New(`google api status 403: { "error": { "code": 403, "message": "User does not have sufficient permissions for this property. To learn more about Property ID, see https://developers.google.com/analytics/devguides/reporting/data/v1/property-id.", "status": "PERMISSION_DENIED" } }`)

	params, ok := ga4OAuthValidationIntegrationParams(projectID, googledata.SearchConsoleOAuthScopeString(), "544348649", nil, err, now)

	if !ok {
		t.Fatal("expected Analytics OAuth validation to update GA4 integration")
	}
	if params.Status != "property_access_required" {
		t.Fatalf("status = %q, want property_access_required", params.Status)
	}
	want := "Google Analytics property access is missing. Confirm the numeric GA4 Property ID and grant the connected Google account Viewer access in GA4 Property Access Management, then run SEO sync again."
	if params.LastError == nil || *params.LastError != want {
		t.Fatalf("last error = %v, want %q", params.LastError, want)
	}
	if params.LastVerifiedAt.Valid {
		t.Fatalf("last verified = %v, want invalid after failed probe", params.LastVerifiedAt)
	}
}
