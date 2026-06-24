package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/googledata"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/secretbox"
	seopkg "github.com/citeloop/citeloop/internal/seo"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
)

const gscOAuthCredentialRef = "seo_oauth_tokens:google_search_console"

type gscConnectionResponse struct {
	Configured          bool                  `json:"configured"`
	Status              string                `json:"status"`
	SelectedProperty    *string               `json:"selected_property"`
	RecommendedProperty *string               `json:"recommended_property"`
	Properties          []gscPropertyResponse `json:"properties"`
	AccountEmail        *string               `json:"account_email,omitempty"`
	LastError           *string               `json:"last_error,omitempty"`
}

type gscPropertyResponse struct {
	SiteURL         string `json:"site_url"`
	PermissionLevel string `json:"permission_level"`
	Recommended     bool   `json:"recommended"`
}

func (s *Server) getGSCConnection(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	connection, err := s.gscConnection(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, connection)
}

func (s *Server) startGSCOAuth(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	if !s.gscOAuthConfigured() {
		writeErr(w, http.StatusPreconditionFailed, "google oauth is not configured")
		return
	}
	var in struct {
		RedirectURI string `json:"redirect_uri"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	redirectURI := strings.TrimSpace(in.RedirectURI)
	if err := s.validateGSCRedirectURI(projectID, redirectURI); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now().UTC()
	state, err := signGSCOAuthState(s.gscOAuthStateSecret(), gscOAuthStateClaims{
		ProjectID:   projectID,
		OwnerID:     s.ownerID(r),
		RedirectURI: redirectURI,
		Nonce:       uuid.NewString(),
		ExpiresAt:   now.Add(10 * time.Minute),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg := googledata.SearchConsoleOAuthConfig(s.Env.GoogleOAuthClientID, s.Env.GoogleOAuthClientSecret, redirectURI)
	writeJSON(w, http.StatusOK, map[string]string{
		"authorization_url": cfg.AuthCodeURL(
			state,
			oauth2.AccessTypeOffline,
			oauth2.SetAuthURLParam("prompt", "consent"),
			oauth2.SetAuthURLParam("include_granted_scopes", "true"),
		),
	})
}

func (s *Server) completeGSCOAuth(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database not configured")
		return
	}
	if !s.gscOAuthConfigured() {
		writeErr(w, http.StatusPreconditionFailed, "google oauth is not configured")
		return
	}
	var in struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	claims, err := parseGSCOAuthState(s.gscOAuthStateSecret(), strings.TrimSpace(in.State), projectID, s.ownerID(r), time.Now().UTC())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg := googledata.SearchConsoleOAuthConfig(s.Env.GoogleOAuthClientID, s.Env.GoogleOAuthClientSecret, claims.RedirectURI)
	token, err := cfg.Exchange(r.Context(), strings.TrimSpace(in.Code))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "google oauth exchange failed")
		return
	}
	sites, err := googledata.NewSearchConsoleOAuthClient(r.Context(), s.Env.GoogleOAuthClientID, s.Env.GoogleOAuthClientSecret, claims.RedirectURI, token).ListSearchConsoleSites(r.Context())
	if err != nil {
		writeErr(w, http.StatusBadGateway, "search console property lookup failed")
		return
	}
	properties := gscPropertiesFromSites(sites)
	siteURL := s.projectSiteURL(r.Context(), projectID)
	properties = markRecommendedGSCProperties(siteURL, properties)
	propertiesJSON, err := json.Marshal(properties)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	encryptedRefreshToken := ""
	if strings.TrimSpace(token.RefreshToken) != "" {
		encryptedRefreshToken, err = secretbox.EncryptString(token.RefreshToken, s.gscTokenSecret())
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else if existing, err := s.Q.GetActiveSEOOAuthToken(r.Context(), db.GetActiveSEOOAuthTokenParams{ProjectID: projectID, Provider: seopkg.ProviderGSC}); err == nil {
		encryptedRefreshToken = existing.EncryptedRefreshToken
	} else {
		writeErr(w, http.StatusBadGateway, "google oauth did not return a refresh token")
		return
	}
	tokenType := strings.TrimSpace(token.TokenType)
	if tokenType == "" {
		tokenType = "Bearer"
	}
	scope := googledata.ScopeSearchConsoleReadonly
	if rawScope, ok := token.Extra("scope").(string); ok && strings.TrimSpace(rawScope) != "" {
		scope = strings.TrimSpace(rawScope)
	}
	row, err := s.Q.UpsertSEOOAuthToken(r.Context(), db.UpsertSEOOAuthTokenParams{
		ProjectID:             projectID,
		Provider:              seopkg.ProviderGSC,
		EncryptedRefreshToken: encryptedRefreshToken,
		TokenType:             tokenType,
		Scope:                 scope,
		AccessTokenExpiresAt:  pgutil.TS(token.Expiry),
		AuthorizedProperties:  propertiesJSON,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	status := gscStatusForToken(row, properties)
	if _, err := s.Q.UpsertSEOIntegration(r.Context(), db.UpsertSEOIntegrationParams{
		ProjectID:      projectID,
		Provider:       seopkg.ProviderGSC,
		Status:         status,
		CredentialRef:  strPtrFrom(gscOAuthCredentialRef),
		LastVerifiedAt: pgutil.TS(time.Now().UTC()),
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.gscConnectionFromToken(projectID, row, siteURL, status))
}

func (s *Server) selectGSCProperty(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database not configured")
		return
	}
	var in struct {
		SiteURL string `json:"site_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	selected := strings.TrimSpace(in.SiteURL)
	if selected == "" {
		writeErr(w, http.StatusBadRequest, "site_url required")
		return
	}
	token, err := s.Q.GetActiveSEOOAuthToken(r.Context(), db.GetActiveSEOOAuthTokenParams{ProjectID: projectID, Provider: seopkg.ProviderGSC})
	if err != nil {
		writeErr(w, http.StatusNotFound, "search console connection not found")
		return
	}
	properties := decodeGSCProperties(token.AuthorizedProperties)
	siteURL := s.projectSiteURL(r.Context(), projectID)
	properties = markRecommendedGSCProperties(siteURL, properties)
	if !containsGSCProperty(properties, selected) {
		writeErr(w, http.StatusBadRequest, "selected property is not authorized")
		return
	}
	token, err = s.Q.UpdateSEOOAuthSelectedProperty(r.Context(), db.UpdateSEOOAuthSelectedPropertyParams{
		ProjectID:        projectID,
		Provider:         seopkg.ProviderGSC,
		SelectedProperty: &selected,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if siteURL == "" {
		siteURL = siteURLFromGSCProperty(selected)
	}
	if siteURL != "" {
		normalize := json.RawMessage(`{}`)
		var country, lang *string
		if existing, err := s.Q.GetSEOPropertyForProject(r.Context(), projectID); err == nil {
			normalize = existing.UrlNormalizationConfig
			country = existing.DefaultCountry
			lang = existing.DefaultLanguage
		}
		if _, err := s.Q.UpsertSEOProperty(r.Context(), db.UpsertSEOPropertyParams{
			ProjectID:              projectID,
			SiteUrl:                siteURL,
			GscSiteUrl:             &selected,
			UrlNormalizationConfig: normalize,
			DefaultCountry:         country,
			DefaultLanguage:        lang,
		}); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if _, err := s.Q.UpsertSEOIntegration(r.Context(), db.UpsertSEOIntegrationParams{
		ProjectID:      projectID,
		Provider:       seopkg.ProviderGSC,
		Status:         "connected",
		CredentialRef:  strPtrFrom(gscOAuthCredentialRef),
		LastVerifiedAt: pgutil.TS(time.Now().UTC()),
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.gscConnectionFromToken(projectID, token, siteURL, "connected"))
}

func (s *Server) revokeGSCConnection(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	if s.Q == nil {
		writeErr(w, http.StatusInternalServerError, "database not configured")
		return
	}
	_, _ = s.Q.RevokeSEOOAuthToken(r.Context(), db.RevokeSEOOAuthTokenParams{ProjectID: projectID, Provider: seopkg.ProviderGSC})
	if _, err := s.Q.UpsertSEOIntegration(r.Context(), db.UpsertSEOIntegrationParams{
		ProjectID: projectID,
		Provider:  seopkg.ProviderGSC,
		Status:    "revoked",
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, gscConnectionResponse{
		Configured: s.gscOAuthConfigured(),
		Status:     "revoked",
		Properties: []gscPropertyResponse{},
	})
}

func (s *Server) gscConnection(ctx context.Context, projectID uuid.UUID) (gscConnectionResponse, error) {
	out := gscConnectionResponse{
		Configured: s.gscOAuthConfigured(),
		Status:     "missing",
		Properties: []gscPropertyResponse{},
	}
	if s.Q == nil {
		return out, nil
	}
	if integrations, err := s.Q.ListSEOIntegrations(ctx, projectID); err == nil {
		for _, integration := range integrations {
			if integration.Provider == seopkg.ProviderGSC {
				out.Status = integration.Status
				out.LastError = integration.LastError
				break
			}
		}
	}
	token, err := s.Q.GetActiveSEOOAuthToken(ctx, db.GetActiveSEOOAuthTokenParams{ProjectID: projectID, Provider: seopkg.ProviderGSC})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	siteURL := s.projectSiteURL(ctx, projectID)
	status := out.Status
	if status == "" || status == "missing" {
		status = gscStatusForToken(token, decodeGSCProperties(token.AuthorizedProperties))
	}
	return s.gscConnectionFromToken(projectID, token, siteURL, status), nil
}

func (s *Server) gscConnectionFromToken(projectID uuid.UUID, token db.SeoOauthToken, siteURL string, status string) gscConnectionResponse {
	properties := markRecommendedGSCProperties(siteURL, decodeGSCProperties(token.AuthorizedProperties))
	recommended := recommendedGSCProperty(properties)
	if status == "" || status == "missing" {
		status = gscStatusForToken(token, properties)
	}
	return gscConnectionResponse{
		Configured:          s.gscOAuthConfigured(),
		Status:              status,
		SelectedProperty:    token.SelectedProperty,
		RecommendedProperty: recommended,
		Properties:          properties,
		AccountEmail:        token.AccountEmail,
		LastError:           token.LastError,
	}
}

func (s *Server) gscOAuthConfigured() bool {
	return strings.TrimSpace(s.Env.GoogleOAuthClientID) != "" &&
		strings.TrimSpace(s.Env.GoogleOAuthClientSecret) != "" &&
		strings.TrimSpace(s.Env.PublicAppURL) != ""
}

func (s *Server) gscOAuthStateSecret() string {
	for _, value := range []string{s.Env.NotificationSecretKey, s.Env.ClerkSecretKey, s.Env.GoogleOAuthClientSecret} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "citeloop-local-gsc-oauth-state"
}

func (s *Server) gscTokenSecret() string {
	for _, value := range []string{s.Env.NotificationSecretKey, s.Env.GoogleOAuthClientSecret, s.Env.ClerkSecretKey} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "citeloop-local-gsc-token"
}

func (s *Server) validateGSCRedirectURI(projectID uuid.UUID, redirectURI string) error {
	parsed, err := url.Parse(strings.TrimSpace(redirectURI))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("redirect_uri is invalid")
	}
	publicURL, err := url.Parse(strings.TrimRight(strings.TrimSpace(s.Env.PublicAppURL), "/"))
	if err != nil || publicURL.Scheme == "" || publicURL.Host == "" {
		return errors.New("public app url is invalid")
	}
	if parsed.Scheme != publicURL.Scheme || parsed.Host != publicURL.Host {
		return errors.New("redirect_uri must use the public app origin")
	}
	expectedPath := "/projects/" + projectID.String() + "/settings/gsc/callback"
	if parsed.Path != expectedPath {
		return errors.New("redirect_uri must use the project search console callback")
	}
	return nil
}

func (s *Server) projectSiteURL(ctx context.Context, projectID uuid.UUID) string {
	if s.Q == nil {
		return ""
	}
	if prop, err := s.Q.GetSEOPropertyForProject(ctx, projectID); err == nil {
		return strings.TrimSpace(prop.SiteUrl)
	}
	project, err := s.Q.GetProject(ctx, projectID)
	if err != nil {
		return ""
	}
	cfg, err := config.Parse(project.Config)
	if err != nil {
		return ""
	}
	if strings.TrimSpace(cfg.SiteURL) != "" {
		return strings.TrimSpace(cfg.SiteURL)
	}
	return strings.TrimSpace(project.Name)
}

func gscPropertiesFromSites(sites []googledata.SearchConsoleSite) []gscPropertyResponse {
	out := make([]gscPropertyResponse, 0, len(sites))
	for _, site := range sites {
		if strings.TrimSpace(site.SiteURL) == "" {
			continue
		}
		out = append(out, gscPropertyResponse{
			SiteURL:         strings.TrimSpace(site.SiteURL),
			PermissionLevel: strings.TrimSpace(site.PermissionLevel),
		})
	}
	return out
}

func decodeGSCProperties(raw json.RawMessage) []gscPropertyResponse {
	if len(raw) == 0 {
		return []gscPropertyResponse{}
	}
	var properties []gscPropertyResponse
	if err := json.Unmarshal(raw, &properties); err != nil {
		return []gscPropertyResponse{}
	}
	if properties == nil {
		return []gscPropertyResponse{}
	}
	return properties
}

func markRecommendedGSCProperties(siteURL string, properties []gscPropertyResponse) []gscPropertyResponse {
	recommended := recommendedGSCPropertyForSite(siteURL, properties)
	out := make([]gscPropertyResponse, 0, len(properties))
	for _, property := range properties {
		property.Recommended = recommended != nil && property.SiteURL == *recommended
		out = append(out, property)
	}
	return out
}

func recommendedGSCProperty(properties []gscPropertyResponse) *string {
	for _, property := range properties {
		if property.Recommended {
			value := property.SiteURL
			return &value
		}
	}
	return nil
}

func recommendedGSCPropertyForSite(siteURL string, properties []gscPropertyResponse) *string {
	if len(properties) == 0 {
		return nil
	}
	host := normalizedHost(siteURL)
	if host == "" {
		return nil
	}
	for _, property := range properties {
		if strings.EqualFold(property.SiteURL, "sc-domain:"+host) || strings.EqualFold(property.SiteURL, "sc-domain:"+strings.TrimPrefix(host, "www.")) {
			value := property.SiteURL
			return &value
		}
	}
	for _, property := range properties {
		if normalizedHost(property.SiteURL) == host || strings.TrimPrefix(normalizedHost(property.SiteURL), "www.") == strings.TrimPrefix(host, "www.") {
			value := property.SiteURL
			return &value
		}
	}
	return nil
}

func containsGSCProperty(properties []gscPropertyResponse, siteURL string) bool {
	for _, property := range properties {
		if property.SiteURL == siteURL {
			return true
		}
	}
	return false
}

func gscStatusForToken(token db.SeoOauthToken, properties []gscPropertyResponse) string {
	if token.SelectedProperty != nil && containsGSCProperty(properties, *token.SelectedProperty) {
		return "connected"
	}
	return "property_selection_required"
}

func normalizedHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "sc-domain:") {
		return strings.ToLower(strings.TrimPrefix(raw, "sc-domain:"))
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		parsed, err = url.Parse("https://" + raw)
		if err != nil {
			return ""
		}
	}
	return strings.ToLower(parsed.Hostname())
}

func siteURLFromGSCProperty(property string) string {
	property = strings.TrimSpace(property)
	if strings.HasPrefix(property, "sc-domain:") {
		host := strings.TrimPrefix(property, "sc-domain:")
		if host != "" {
			return "https://" + host
		}
	}
	if normalizedHost(property) != "" {
		return property
	}
	return ""
}
