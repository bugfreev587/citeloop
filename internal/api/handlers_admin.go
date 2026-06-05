package api

import (
	"encoding/json"
	"net/http"

	"github.com/citeloop/citeloop/internal/admin"
)

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
