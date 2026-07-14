package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/citeloop/citeloop/internal/admin"
	"github.com/citeloop/citeloop/internal/articleassets"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) getPublicArticleAsset(w http.ResponseWriter, r *http.Request) {
	key, err := articleassets.DecodeStorageToken(chi.URLParam(r, "token"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	object, err := s.Q.GetArticleAssetObject(r.Context(), key)
	if err != nil || !strings.HasPrefix(object.MimeType, "image/") {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", object.MimeType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(object.Data)
}

func (s *Server) listProjectArticleAssets(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	articleID, err := s.articleID(r)
	if err != nil {
		writeErr(w, 400, "bad article id")
		return
	}
	if _, err = s.Q.GetArticleForProject(r.Context(), db.GetArticleForProjectParams{ID: articleID, ProjectID: projectID}); err != nil {
		writeErr(w, 404, "article not found")
		return
	}
	rows, err := s.Q.ListArticleAssetsForArticle(r.Context(), db.ListArticleAssetsForArticleParams{ProjectID: projectID, ArticleID: articleID})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, rows)
}

func (s *Server) editProjectArticleAsset(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	articleID, assetID, ok := assetRouteIDs(r)
	if !ok {
		writeErr(w, 400, "bad article or asset id")
		return
	}
	asset, err := s.Q.GetArticleAssetForProject(r.Context(), db.GetArticleAssetForProjectParams{ID: assetID, ProjectID: projectID})
	if err != nil || asset.ArticleID != articleID {
		writeErr(w, 404, "article asset not found")
		return
	}
	var input struct {
		AltText string `json:"alt_text"`
		Caption string `json:"caption"`
		Omitted bool   `json:"omitted"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeErr(w, 400, "invalid asset edit")
		return
	}
	updated, err := s.Q.UpdateArticleAssetEditorial(r.Context(), db.UpdateArticleAssetEditorialParams{AltText: strings.TrimSpace(input.AltText), Caption: strings.TrimSpace(input.Caption), Omitted: input.Omitted, ID: assetID, ProjectID: projectID})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, updated)
}

func (s *Server) regenerateProjectArticleAsset(w http.ResponseWriter, r *http.Request) {
	if s.ArticleAssets == nil {
		writeErr(w, 503, "article image generation is not configured")
		return
	}
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	articleID, assetID, ok := assetRouteIDs(r)
	if !ok {
		writeErr(w, 400, "bad article or asset id")
		return
	}
	article, err := s.Q.GetArticleForProject(r.Context(), db.GetArticleForProjectParams{ID: articleID, ProjectID: projectID})
	if err != nil {
		writeErr(w, 404, "article not found")
		return
	}
	asset, err := s.Q.GetArticleAssetForProject(r.Context(), db.GetArticleAssetForProjectParams{ID: assetID, ProjectID: projectID})
	if err != nil || asset.ArticleID != articleID {
		writeErr(w, 404, "article asset not found")
		return
	}
	var brief articleassets.Brief
	if json.Unmarshal(asset.Brief, &brief) != nil {
		writeErr(w, 409, "stored image brief is invalid")
		return
	}
	brief.Roles = []string{asset.Role}
	brief.Revision = asset.Revision + 1
	planned, err := s.ArticleAssets.Plan(r.Context(), article, brief)
	if err != nil || len(planned) != 1 {
		writeErr(w, 500, "could not plan image revision")
		return
	}
	generated, err := s.ArticleAssets.Generate(r.Context(), projectID, planned[0].ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, generated)
}

func assetRouteIDs(r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	articleID, e1 := uuid.Parse(chi.URLParam(r, "articleID"))
	assetID, e2 := uuid.Parse(chi.URLParam(r, "assetID"))
	return articleID, assetID, e1 == nil && e2 == nil
}

func (s *Server) getImageCredentials(w http.ResponseWriter, r *http.Request) {
	credentials, err := admin.LoadImageCredentials(r.Context(), s.Pool, s.Env.NotificationSecretKey)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, admin.ImageStatus(credentials))
}
func (s *Server) updateImageCredentials(w http.ResponseWriter, r *http.Request) {
	var input admin.ImageCredentialInput
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeErr(w, 400, "invalid image credential")
		return
	}
	credentials, err := admin.SaveImageCredentials(r.Context(), s.Pool, s.Env.NotificationSecretKey, input)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, admin.ImageStatus(credentials))
}
func (s *Server) deleteImageCredentials(w http.ResponseWriter, r *http.Request) {
	if err := admin.DeleteImageCredentials(r.Context(), s.Pool); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
