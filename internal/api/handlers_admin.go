package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/admin"
	"github.com/citeloop/citeloop/internal/llm"
)

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func (s *Server) getLLMCredentials(w http.ResponseWriter, r *http.Request) {
	credentials, err := admin.LoadCredentials(r.Context(), s.Pool)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, admin.StatusFromCredentials(credentials))
}

func (s *Server) updateLLMCredentials(w http.ResponseWriter, r *http.Request) {
	var in admin.UpdateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	credentials, err := admin.SaveCredentials(r.Context(), s.Pool, in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, admin.StatusFromCredentials(credentials))
}

// deleteLLMCredentials removes the saved provider key; the runtime falls back to
// the server-environment provider afterwards.
func (s *Server) deleteLLMCredentials(w http.ResponseWriter, r *http.Request) {
	if err := admin.DeleteCredentials(r.Context(), s.Pool); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, admin.StatusFromCredentials(nil))
}

// testLLMCredentials runs a tiny live completion against the saved provider so an
// admin can confirm the key/base URL actually work before relying on them. The
// optional request body carries the routes currently selected in the UI so the
// probe exercises those selections even before they are saved.
func (s *Server) testLLMCredentials(w http.ResponseWriter, r *http.Request) {
	cred, err := admin.LoadCredentials(r.Context(), s.Pool)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cred == nil || cred.APIKey == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "No provider credentials saved yet. Save a key first, then test."})
		return
	}
	var in struct {
		Routes admin.ModelRoutes `json:"routes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	testCred := admin.CredentialsWithRouteOverrides(*cred, in.Routes)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	results := []map[string]any{}
	allOK := true
	firstModel := ""
	firstSample := ""
	var totalLatency int64
	provider := admin.ProviderFromCredentials(testCred, s.Env)
	for _, target := range admin.RuntimeProbeTargets(testCred, s.Env) {
		start := time.Now()
		resp, err := provider.Complete(ctx, llm.CompletionReq{
			System:                  "You are a connectivity probe.",
			Prompt:                  "Reply with the single word: pong",
			Purpose:                 target.Purpose,
			Model:                   target.ModelAlias,
			MaxTokens:               16,
			DisableProviderFallback: true,
		})
		latencyMs := time.Since(start).Milliseconds()
		totalLatency += latencyMs
		item := map[string]any{
			"role":             string(target.Role),
			"label":            target.Label,
			"provider":         string(cred.Provider),
			"primary_provider": string(target.Provider),
			"model_alias":      target.ModelAlias,
			"fallback_enabled": target.FallbackEnabled,
			"latency_ms":       latencyMs,
		}
		if err != nil {
			allOK = false
			item["ok"] = false
			item["error"] = err.Error()
		} else {
			item["ok"] = true
			item["model"] = resp.Model
			item["sample"] = truncate(strings.TrimSpace(resp.Text), 80)
			item["cost_usd"] = resp.CostUSD
			if firstModel == "" {
				firstModel = resp.Model
			}
			if firstSample == "" {
				firstSample = truncate(strings.TrimSpace(resp.Text), 80)
			}
		}
		results = append(results, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         allOK,
		"provider":   string(cred.Provider),
		"model":      firstModel,
		"latency_ms": totalLatency,
		"sample":     firstSample,
		"results":    results,
	})
}
