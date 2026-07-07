package api

import (
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
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
