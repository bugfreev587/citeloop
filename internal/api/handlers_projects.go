package api

import (
	"encoding/json"
	"net/http"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) projectID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "projectID"))
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	ps, err := s.Q.ListProjects(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ps)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string `json:"name"`
		Slug  string `json:"slug"`
		Owner string `json:"owner_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Owner == "" {
		in.Owner = "default"
	}
	p, err := s.Q.CreateProject(r.Context(), db.CreateProjectParams{
		OwnerID: in.Owner, Name: in.Name, Slug: in.Slug,
		Config: config.Default().JSON(),
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, p)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	p, err := s.Q.GetProject(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "not found")
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) updateConfig(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	var cfg config.ProjectConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	p, err := s.Q.UpdateProjectConfig(r.Context(), db.UpdateProjectConfigParams{ID: id, Config: cfg.JSON()})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	p, err := s.Q.GetActiveProfile(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "no active profile")
		return
	}
	writeJSON(w, 200, p)
}

func (s *Server) updateProfile(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	active, err := s.Q.GetActiveProfile(r.Context(), id)
	if err != nil {
		writeErr(w, 404, "no active profile")
		return
	}
	var in struct {
		Profile    json.RawMessage `json:"profile"`
		SourceUrls json.RawMessage `json:"source_urls"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.SourceUrls == nil {
		in.SourceUrls = active.SourceUrls
	}
	updated, err := s.Q.UpdateProfile(r.Context(), db.UpdateProfileParams{
		ID: active.ID, Profile: in.Profile, SourceUrls: in.SourceUrls,
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, updated)
}

func (s *Server) listInventory(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	items, err := s.Q.ListInventory(r.Context(), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, items)
}

func (s *Server) updateInventory(w http.ResponseWriter, r *http.Request) {
	itemID, err := uuid.Parse(chi.URLParam(r, "itemID"))
	if err != nil {
		writeErr(w, 400, "bad item id")
		return
	}
	var in struct {
		Title         string          `json:"title"`
		TargetKeyword string          `json:"target_keyword"`
		Topics        json.RawMessage `json:"topics"`
		Summary       string          `json:"summary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Topics == nil {
		in.Topics = json.RawMessage("[]")
	}
	item, err := s.Q.UpdateInventoryItem(r.Context(), db.UpdateInventoryItemParams{
		ID: itemID, Title: strPtr(in.Title), TargetKeyword: strPtr(in.TargetKeyword),
		Topics: in.Topics, Summary: strPtr(in.Summary),
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, item)
}

func (s *Server) deleteInventory(w http.ResponseWriter, r *http.Request) {
	itemID, err := uuid.Parse(chi.URLParam(r, "itemID"))
	if err != nil {
		writeErr(w, 400, "bad item id")
		return
	}
	if err := s.Q.DeleteInventoryItem(r.Context(), itemID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
