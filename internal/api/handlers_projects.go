package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	seopkg "github.com/citeloop/citeloop/internal/seo"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) projectID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "projectID"))
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	ownerID := s.ownerID(r)
	if ownerID == "" {
		writeErr(w, http.StatusForbidden, "project owner required")
		return
	}
	ps, err := s.Q.ListProjectsByOwner(r.Context(), ownerID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, emptySlice(ps))
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name    string `json:"name"`
		Slug    string `json:"slug"`
		Owner   string `json:"owner_id"`
		SiteURL string `json:"site_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ownerID := s.ownerID(r)
	if ownerID == "" {
		writeErr(w, http.StatusForbidden, "project owner required")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = slugifyProject(in.Slug)
	in.SiteURL = strings.TrimSpace(in.SiteURL)
	if in.Name == "" && in.SiteURL != "" {
		in.Name = projectNameFromURL(in.SiteURL)
	}
	if in.Slug == "" {
		in.Slug = slugifyProject(in.Name)
	}
	if in.Name == "" || in.Slug == "" {
		writeErr(w, http.StatusBadRequest, "project name or site_url required")
		return
	}
	normalizedSiteURL := ""
	if in.SiteURL != "" {
		normalized, err := seopkg.NormalizeURL(in.SiteURL, in.SiteURL, seopkg.URLNormalizationConfig{})
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad site_url")
			return
		}
		normalizedSiteURL = normalized
	}
	p, err := s.Q.CreateProject(r.Context(), db.CreateProjectParams{
		OwnerID: ownerID, Name: in.Name, Slug: in.Slug,
		Config: config.Default().JSON(),
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if normalizedSiteURL != "" {
		if _, err := s.Q.UpsertSEOProperty(r.Context(), db.UpsertSEOPropertyParams{
			ProjectID:              p.ID,
			SiteUrl:                normalizedSiteURL,
			UrlNormalizationConfig: json.RawMessage(`{}`),
		}); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		if _, err := s.Q.UpsertSEOIntegration(r.Context(), db.UpsertSEOIntegrationParams{
			ProjectID: p.ID,
			Provider:  seopkg.ProviderGSC,
			Status:    "missing",
		}); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
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
	p, err := s.Q.UpdateProjectConfigForOwner(r.Context(), db.UpdateProjectConfigForOwnerParams{
		ID: id, Config: cfg.JSON(), OwnerID: s.ownerID(r),
	})
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
	writeJSON(w, 200, emptySlice(items))
}

func (s *Server) updateInventory(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
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
		Evidence      json.RawMessage `json:"evidence_snippets"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if in.Topics == nil {
		in.Topics = json.RawMessage("[]")
	}
	if in.Evidence == nil {
		in.Evidence = json.RawMessage("[]")
	}
	item, err := s.Q.UpdateInventoryItem(r.Context(), db.UpdateInventoryItemParams{
		ID: itemID, Title: strPtr(in.Title), TargetKeyword: strPtr(in.TargetKeyword),
		Topics: in.Topics, Summary: strPtr(in.Summary), EvidenceSnippets: in.Evidence,
		ProjectID: projectID,
	})
	if err != nil {
		writeErr(w, 404, "inventory item not found")
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

func emptySlice[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func projectNameFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return strings.TrimSpace(raw)
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	return host
}

func slugifyProject(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
