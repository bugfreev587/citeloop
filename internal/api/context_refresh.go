package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/contextmeta"
	"github.com/citeloop/citeloop/internal/db"
)

func (s *Server) refreshContext(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	cfg, err := s.projectConfig(r, projectID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	siteURL := strings.TrimSpace(cfg.SiteURL)
	if siteURL == "" {
		writeErr(w, http.StatusBadRequest, "configured domain required")
		return
	}
	active, err := s.Q.GetActiveProfile(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no active profile")
		return
	}
	if contextmeta.HasActiveCrawl(active.Profile) {
		writeErr(w, http.StatusConflict, "context refresh already running")
		return
	}
	now := time.Now().UTC()
	if contextmeta.ManualCooldownActive(active.Profile, now) {
		writeErr(w, http.StatusTooManyRequests, "context can be manually refreshed once every 24 hours")
		return
	}
	updatedProfile := contextmeta.StartedProfile(active.Profile, contextmeta.SourceManual, now)
	updated, err := s.Q.UpdateProfile(r.Context(), db.UpdateProfileParams{
		ID:         active.ID,
		Profile:    updatedProfile,
		SourceUrls: active.SourceUrls,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.startInsightInventoryCrawl(projectID, siteURL, cfg.Crawl)
	writeJSON(w, http.StatusAccepted, updated)
}

func configuredContextLanding(configuredSiteURL string, requestedLanding string) (string, error) {
	requestedLanding = strings.TrimSpace(requestedLanding)
	configuredSiteURL = strings.TrimSpace(configuredSiteURL)
	if configuredSiteURL == "" {
		if requestedLanding == "" {
			return "", errors.New("landing_url required")
		}
		return requestedLanding, nil
	}
	if !sameURLHost(configuredSiteURL, requestedLanding) {
		return "", errors.New("landing_url must match configured domain")
	}
	return configuredSiteURL, nil
}

func sameURLHost(a string, b string) bool {
	left, err := url.Parse(strings.TrimSpace(a))
	if err != nil || left.Host == "" {
		return false
	}
	right, err := url.Parse(strings.TrimSpace(b))
	if err != nil || right.Host == "" {
		return false
	}
	return strings.EqualFold(left.Host, right.Host)
}
