package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/admin"
	"github.com/citeloop/citeloop/internal/db"
	geopkg "github.com/citeloop/citeloop/internal/geo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) listAdminGEOCredentials(w http.ResponseWriter, r *http.Request) {
	if s.Pool == nil {
		writeErr(w, http.StatusInternalServerError, "database not configured")
		return
	}
	statuses, err := admin.ListGEOStatuses(r.Context(), s.Pool)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, statuses)
}

func (s *Server) updateAdminGEOCredentials(w http.ResponseWriter, r *http.Request) {
	scope, ok := adminGEOScope(w, r)
	if !ok {
		return
	}
	if s.Pool == nil {
		writeErr(w, http.StatusInternalServerError, "database not configured")
		return
	}
	var in admin.GEOUpdateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	credentials, err := admin.SaveGEOCredentials(r.Context(), s.Pool, scope, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, admin.GEOStatusFromCredential(credentials))
}

func (s *Server) deleteAdminGEOCredentials(w http.ResponseWriter, r *http.Request) {
	scope, ok := adminGEOScope(w, r)
	if !ok {
		return
	}
	if s.Pool == nil {
		writeErr(w, http.StatusInternalServerError, "database not configured")
		return
	}
	if err := admin.DeleteGEOCredentials(r.Context(), s.Pool, scope); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, admin.GEOStatusForScope(scope, nil))
}

func (s *Server) testAdminGEOCredentials(w http.ResponseWriter, r *http.Request) {
	scope, ok := adminGEOScope(w, r)
	if !ok {
		return
	}
	if s.Pool == nil {
		writeErr(w, http.StatusInternalServerError, "database not configured")
		return
	}
	credentials, err := admin.LoadGEOCredentials(r.Context(), s.Pool, scope)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if credentials == nil || strings.TrimSpace(credentials.APIKey) == "" || !credentials.Enabled {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "No enabled TokenGate GEO credential saved yet. Save a key first, then test."})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	start := time.Now()
	provider := tokenGateProviderFromGEOCredentials(*credentials)
	rows, costUSD, err := provider.Observe(ctx, []db.GeoPrompt{dbGeoPromptForAdminTest(scope)})
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         false,
			"provider":   provider.Name(),
			"model":      credentials.Model,
			"latency_ms": latencyMs,
			"error":      err.Error(),
		})
		return
	}
	sample := ""
	if len(rows) > 0 {
		sample = truncate(strings.TrimSpace(rows[0].AnswerSummary), 80)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"provider":   provider.Name(),
		"model":      credentials.Model,
		"latency_ms": latencyMs,
		"sample":     sample,
		"cost_usd":   costUSD,
	})
}

func adminGEOScope(w http.ResponseWriter, r *http.Request) (admin.GEOProviderScope, bool) {
	scope, err := admin.ParseGEOCredentialScope(chi.URLParam(r, "scope"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	return scope, true
}

func tokenGateProviderFromGEOCredentials(credentials admin.GEOCredentials) geopkg.TokenGateAnswerProvider {
	return geopkg.NewTokenGateAnswerProvider(geopkg.TokenGateAnswerProviderConfig{
		Scope:   string(credentials.Scope),
		APIKey:  credentials.APIKey,
		BaseURL: credentials.BaseURL,
		Model:   credentials.Model,
		Engine:  admin.GEOEngineForScope(credentials.Scope),
	}, nil)
}

func dbGeoPromptForAdminTest(_ admin.GEOProviderScope) db.GeoPrompt {
	return db.GeoPrompt{
		ID:         uuid.New(),
		PromptText: "Reply with one short sentence and cite a source if the selected model supports citations.",
		Locale:     "en-US",
	}
}
