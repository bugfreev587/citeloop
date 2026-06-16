package api

import (
	"context"
	"encoding/json"
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
// admin can confirm the key/base URL actually work before relying on them.
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

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	start := time.Now()
	resp, err := admin.ProviderFromCredentials(*cred, s.Env).Complete(ctx, llm.CompletionReq{
		System:    "You are a connectivity probe.",
		Prompt:    "Reply with the single word: pong",
		MaxTokens: 16,
	})
	latencyMs := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         false,
			"provider":   string(cred.Provider),
			"latency_ms": latencyMs,
			"error":      err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"provider":   string(cred.Provider),
		"model":      resp.Model,
		"latency_ms": latencyMs,
		"sample":     truncate(strings.TrimSpace(resp.Text), 80),
	})
}
