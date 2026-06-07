package seo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	ProviderGSC       = "google_search_console"
	DefaultBriefLimit = 10
)

// Service coordinates the Operations Loop backend workflow.
type Service struct {
	Q           *db.Queries
	HTTPClient  *http.Client
	BlogBaseURL string
	Now         func() time.Time
}

type SyncResult struct {
	RunID              uuid.UUID `json:"run_id"`
	Status             string    `json:"status"`
	CheckedURLs        int       `json:"checked_urls"`
	ConnectedGSC       bool      `json:"connected_gsc"`
	ColdStart          bool      `json:"cold_start"`
	DataSourceNotes    []string  `json:"data_source_notes"`
	GeneratedAnomalies int       `json:"generated_anomalies"`
}

type Overview struct {
	Property            *db.SeoProperty              `json:"property"`
	Integrations        []db.SeoIntegration          `json:"integrations"`
	Last28Days          db.SEOOverviewStatsRow       `json:"last_28_days"`
	Technical           db.SEOTechnicalSummaryRow    `json:"technical"`
	OpportunitiesByType []db.SEOOpportunityCountsRow `json:"opportunities_by_type"`
	ActionsByStatus     []db.ContentActionCountsRow  `json:"actions_by_status"`
	ColdStart           bool                         `json:"cold_start"`
	HandoffReadyForAuto bool                         `json:"handoff_ready_for_autopilot"`
	DataSourceWarnings  []string                     `json:"data_source_warnings"`
}

type Brief struct {
	Mode        string              `json:"mode"`
	Title       string              `json:"title"`
	GeneratedAt time.Time           `json:"generated_at"`
	Actions     []db.SeoOpportunity `json:"actions"`
	Blockers    []string            `json:"blockers"`
	Measurement []string            `json:"measurement_updates"`
}

type TechnicalResult struct {
	HTTPStatus            *int32         `json:"http_status,omitempty"`
	CanonicalStatus       string         `json:"canonical_status"`
	RobotsStatus          string         `json:"robots_status"`
	TitleStatus           string         `json:"title_status"`
	MetaDescriptionStatus string         `json:"meta_description_status"`
	H1Status              string         `json:"h1_status"`
	StructuredDataStatus  string         `json:"structured_data_status"`
	InternalLinkCount     int32          `json:"internal_link_count"`
	OutboundLinkCount     int32          `json:"outbound_link_count"`
	RawDetails            map[string]any `json:"raw_details"`
}

func (s Service) Overview(ctx context.Context, projectID uuid.UUID) (Overview, error) {
	var out Overview
	if prop, err := s.Q.GetSEOPropertyForProject(ctx, projectID); err == nil {
		out.Property = &prop
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	integrations, err := s.Q.ListSEOIntegrations(ctx, projectID)
	if err != nil {
		return out, err
	}
	stats, err := s.Q.SEOOverviewStats(ctx, projectID)
	if err != nil {
		return out, err
	}
	technical, err := s.Q.SEOTechnicalSummary(ctx, projectID)
	if err != nil {
		return out, err
	}
	opps, err := s.Q.SEOOpportunityCounts(ctx, projectID)
	if err != nil {
		return out, err
	}
	actions, err := s.Q.ContentActionCounts(ctx, projectID)
	if err != nil {
		return out, err
	}
	out.Integrations = integrations
	out.Last28Days = stats
	out.Technical = technical
	out.OpportunitiesByType = opps
	out.ActionsByStatus = actions
	out.ColdStart = isColdStart(stats)
	out.HandoffReadyForAuto = !out.ColdStart && hasConnectedGSC(integrations)
	if !hasConnectedGSC(integrations) {
		out.DataSourceWarnings = append(out.DataSourceWarnings, "Google Search Console service account is not connected; search metrics are unavailable.")
	}
	if out.ColdStart {
		out.DataSourceWarnings = append(out.DataSourceWarnings, "SEO data is below the Operations Loop minimum threshold; brief uses cold-start mode.")
	}
	return out, nil
}

func (s Service) Sync(ctx context.Context, projectID uuid.UUID, siteURL string) (SyncResult, error) {
	now := s.now()
	input := mustJSON(map[string]any{"site_url": siteURL})
	run, err := s.Q.StartSEORun(ctx, db.StartSEORunParams{
		ProjectID: projectID,
		Agent:     "seo_sync",
		StartedAt: pgutil.TS(now),
		Input:     input,
	})
	if err != nil {
		return SyncResult{}, err
	}
	result := SyncResult{RunID: run.ID, Status: "ok"}
	finish := func(status string, output any, runErr error) (SyncResult, error) {
		result.Status = status
		var errText *string
		if runErr != nil {
			s := runErr.Error()
			errText = &s
		}
		finished, err := s.Q.FinishSEORun(ctx, db.FinishSEORunParams{
			ID:         run.ID,
			ProjectID:  projectID,
			Status:     status,
			FinishedAt: pgutil.TS(s.now()),
			CostUsd:    pgtype.Numeric{},
			Output:     mustJSON(output),
			Error:      errText,
		})
		if err == nil {
			result.RunID = finished.ID
		}
		if runErr != nil {
			return result, runErr
		}
		return result, err
	}

	prop, err := s.ensureProperty(ctx, projectID, siteURL)
	if err != nil {
		return finish("error", result, err)
	}
	integrations, err := s.Q.ListSEOIntegrations(ctx, projectID)
	if err != nil {
		return finish("error", result, err)
	}
	result.ConnectedGSC = hasConnectedGSC(integrations)
	if !result.ConnectedGSC {
		result.DataSourceNotes = append(result.DataSourceNotes, "gsc_missing")
	}
	articles, err := s.Q.ListPublishedCanonicalArticlesForSEO(ctx, projectID)
	if err != nil {
		return finish("error", result, err)
	}
	for _, article := range articles {
		if article.CanonicalUrl == nil || strings.TrimSpace(*article.CanonicalUrl) == "" {
			continue
		}
		normalized, err := NormalizeURL(*article.CanonicalUrl, prop.SiteUrl, decodeNormalizationConfig(prop.UrlNormalizationConfig))
		if err != nil {
			return finish("error", result, err)
		}
		check := s.checkURL(ctx, *article.CanonicalUrl, prop.SiteUrl)
		result.CheckedURLs++
		status := "unknown"
		if check.HTTPStatus != nil && *check.HTTPStatus >= 200 && *check.HTTPStatus < 300 {
			status = "ok"
		} else if check.HTTPStatus != nil {
			status = "http_error"
		}
		_, err = s.Q.UpsertTechnicalCheck(ctx, db.UpsertTechnicalCheckParams{
			ProjectID:             projectID,
			RunID:                 run.ID,
			PageUrl:               *article.CanonicalUrl,
			NormalizedPageUrl:     normalized,
			ArticleID:             uuidToPG(article.ID),
			HttpStatus:            check.HTTPStatus,
			CanonicalStatus:       strPtr(check.CanonicalStatus),
			RobotsStatus:          strPtr(check.RobotsStatus),
			TitleStatus:           strPtr(check.TitleStatus),
			MetaDescriptionStatus: strPtr(check.MetaDescriptionStatus),
			H1Status:              strPtr(check.H1Status),
			StructuredDataStatus:  strPtr(check.StructuredDataStatus),
			InternalLinkCount:     &check.InternalLinkCount,
			OutboundLinkCount:     &check.OutboundLinkCount,
			ContentHash:           article.ContentHash,
			UnsafeMdxDetected:     strings.Contains(strings.ToLower(article.ContentMd), "<script"),
			RawDetails:            mustJSON(check.RawDetails),
		})
		if err != nil {
			return finish("error", result, err)
		}
		notes := map[string]any{"gsc_status": "missing", "metrics": "not_synced"}
		if result.ConnectedGSC {
			notes["gsc_status"] = "connected"
		}
		_, err = s.Q.UpsertPagePerformanceDaily(ctx, db.UpsertPagePerformanceDailyParams{
			ProjectID:         projectID,
			PropertyID:        prop.ID,
			Date:              pgtype.Date{Time: now, Valid: true},
			PageUrl:           *article.CanonicalUrl,
			NormalizedPageUrl: normalized,
			ArticleID:         uuidToPG(article.ID),
			TopicID:           uuidToPG(article.TopicID),
			TechnicalStatus:   &status,
			DataSourceNotes:   mustJSON(notes),
		})
		if err != nil {
			return finish("error", result, err)
		}
	}
	overview, err := s.Overview(ctx, projectID)
	if err != nil {
		return finish("error", result, err)
	}
	result.ColdStart = overview.ColdStart
	status := "ok"
	if !result.ConnectedGSC {
		status = "degraded"
	}
	return finish(status, result, nil)
}

func (s Service) Analyze(ctx context.Context, projectID uuid.UUID) (SyncResult, error) {
	now := s.now()
	run, err := s.Q.StartSEORun(ctx, db.StartSEORunParams{
		ProjectID: projectID,
		Agent:     "seo_analyzer",
		StartedAt: pgutil.TS(now),
		Input:     json.RawMessage(`{"source":"operations_loop"}`),
	})
	if err != nil {
		return SyncResult{}, err
	}
	result := SyncResult{RunID: run.ID, Status: "ok"}
	finish := func(status string, output any, runErr error) (SyncResult, error) {
		result.Status = status
		var errText *string
		if runErr != nil {
			s := runErr.Error()
			errText = &s
		}
		_, err := s.Q.FinishSEORun(ctx, db.FinishSEORunParams{
			ID:         run.ID,
			ProjectID:  projectID,
			Status:     status,
			FinishedAt: pgutil.TS(s.now()),
			Output:     mustJSON(output),
			Error:      errText,
		})
		if runErr != nil {
			return result, runErr
		}
		return result, err
	}
	articles, err := s.Q.ListPublishedCanonicalArticlesForSEO(ctx, projectID)
	if err != nil {
		return finish("error", result, err)
	}
	prop, _ := s.ensureProperty(ctx, projectID, "")
	for _, article := range articles {
		if article.CanonicalUrl == nil {
			continue
		}
		normalized, err := NormalizeURL(*article.CanonicalUrl, prop.SiteUrl, decodeNormalizationConfig(prop.UrlNormalizationConfig))
		if err != nil {
			return finish("error", result, err)
		}
		if article.CanonicalUrlVerifiedAt.Valid && article.Status == "published" {
			continue
		}
		evidence := map[string]any{
			"status":                    article.Status,
			"canonical_url_verified_at": article.CanonicalUrlVerifiedAt.Valid,
			"source":                    "articles_state",
		}
		action := "technical SEO fix task"
		impact := "Confirm canonical URL availability and indexing health before generating more query-level recommendations."
		_, err = s.Q.UpsertSEOOpportunity(ctx, db.UpsertSEOOpportunityParams{
			ProjectID:         projectID,
			Type:              "indexing_anomaly",
			Status:            "open",
			PriorityScore:     pgutil.Numeric(80),
			Confidence:        pgutil.Numeric(70),
			PageUrl:           article.CanonicalUrl,
			NormalizedPageUrl: normalized,
			ArticleID:         uuidToPG(article.ID),
			TopicID:           uuidToPG(article.TopicID),
			Evidence:          mustJSON(evidence),
			RecommendedAction: &action,
			ExpectedImpact:    &impact,
			Effort:            2,
			RiskLevel:         "low",
			CreatedByRunID:    uuidToPG(run.ID),
		})
		if err != nil {
			return finish("error", result, err)
		}
		result.GeneratedAnomalies++
	}
	overview, err := s.Overview(ctx, projectID)
	if err != nil {
		return finish("error", result, err)
	}
	result.ColdStart = overview.ColdStart
	status := "ok"
	if result.ColdStart {
		status = "degraded"
	}
	return finish(status, result, nil)
}

func (s Service) Brief(ctx context.Context, projectID uuid.UUID) (Brief, error) {
	overview, err := s.Overview(ctx, projectID)
	if err != nil {
		return Brief{}, err
	}
	opps, err := s.Q.ListSEOOpportunities(ctx, db.ListSEOOpportunitiesParams{
		ProjectID: projectID,
		Status:    "open",
		LimitRows: DefaultBriefLimit,
	})
	if err != nil {
		return Brief{}, err
	}
	mode := "opportunities"
	title := "SEO operating brief"
	blockers := []string{}
	if overview.ColdStart {
		mode = "cold_start"
		title = "SEO cold-start brief"
		blockers = append(blockers, "Insufficient GSC data for query/CTR/decay recommendations.")
	}
	if !hasConnectedGSC(overview.Integrations) {
		blockers = append(blockers, "Google Search Console service account is not connected.")
	}
	return Brief{
		Mode:        mode,
		Title:       title,
		GeneratedAt: s.now(),
		Actions:     opps,
		Blockers:    blockers,
		Measurement: []string{"No completed SEO measurement windows yet."},
	}, nil
}

func (s Service) ensureProperty(ctx context.Context, projectID uuid.UUID, siteURL string) (db.SeoProperty, error) {
	if prop, err := s.Q.GetSEOPropertyForProject(ctx, projectID); err == nil {
		return prop, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.SeoProperty{}, err
	}
	if strings.TrimSpace(siteURL) == "" {
		siteURL = s.BlogBaseURL
	}
	if strings.TrimSpace(siteURL) == "" {
		siteURL = "https://unipost.dev"
	}
	normalized, err := NormalizeURL(siteURL, siteURL, URLNormalizationConfig{})
	if err != nil {
		return db.SeoProperty{}, err
	}
	return s.Q.UpsertSEOProperty(ctx, db.UpsertSEOPropertyParams{
		ProjectID:              projectID,
		SiteUrl:                normalized,
		UrlNormalizationConfig: json.RawMessage(`{}`),
	})
}

func (s Service) checkURL(ctx context.Context, rawURL, siteURL string) TechnicalResult {
	client := s.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return failedTechnicalResult(err)
	}
	req.Header.Set("User-Agent", "CiteLoop SEO technical checker")
	res, err := client.Do(req)
	if err != nil {
		return failedTechnicalResult(err)
	}
	defer res.Body.Close()
	status := int32(res.StatusCode)
	limited := io.LimitReader(res.Body, 1<<20)
	body, _ := io.ReadAll(limited)
	html := strings.ToLower(string(body))
	return TechnicalResult{
		HTTPStatus:            &status,
		CanonicalStatus:       presenceStatus(html, `rel=["']canonical["']`),
		RobotsStatus:          robotsStatus(html),
		TitleStatus:           presenceStatus(html, `<title`),
		MetaDescriptionStatus: presenceStatus(html, `name=["']description["']`),
		H1Status:              presenceStatus(html, `<h1`),
		StructuredDataStatus:  presenceStatus(html, `application/ld\+json`),
		InternalLinkCount:     countLinks(html, siteURL, true),
		OutboundLinkCount:     countLinks(html, siteURL, false),
		RawDetails: map[string]any{
			"status":     res.Status,
			"final_url":  res.Request.URL.String(),
			"body_bytes": len(body),
		},
	}
}

func failedTechnicalResult(err error) TechnicalResult {
	return TechnicalResult{
		CanonicalStatus:       "unknown",
		RobotsStatus:          "unknown",
		TitleStatus:           "unknown",
		MetaDescriptionStatus: "unknown",
		H1Status:              "unknown",
		StructuredDataStatus:  "unknown",
		RawDetails:            map[string]any{"error": err.Error()},
	}
}

func presenceStatus(html, pattern string) string {
	ok, _ := regexp.MatchString(pattern, html)
	if ok {
		return "present"
	}
	return "missing"
}

func robotsStatus(html string) string {
	if strings.Contains(html, "noindex") {
		return "noindex"
	}
	if strings.Contains(html, `name="robots"`) || strings.Contains(html, `name='robots'`) {
		return "present"
	}
	return "missing"
}

func countLinks(html, siteURL string, internal bool) int32 {
	re := regexp.MustCompile(`href=["']([^"']+)["']`)
	matches := re.FindAllStringSubmatch(html, -1)
	host := ""
	if normalized, err := NormalizeURL(siteURL, siteURL, URLNormalizationConfig{}); err == nil {
		host = strings.TrimPrefix(strings.TrimPrefix(normalized, "https://"), "http://")
		host = strings.TrimSuffix(strings.Split(host, "/")[0], "/")
	}
	var count int32
	for _, match := range matches {
		href := strings.ToLower(match[1])
		isInternal := strings.HasPrefix(href, "/") || (host != "" && strings.Contains(href, host))
		if isInternal == internal {
			count++
		}
	}
	return count
}

func isColdStart(stats db.SEOOverviewStatsRow) bool {
	return stats.GscDays28d < 14 || pgutil.Float(stats.Impressions28d) < 500 || pgutil.Float(stats.Clicks28d) < 30
}

func hasConnectedGSC(integrations []db.SeoIntegration) bool {
	for _, integration := range integrations {
		if integration.Provider == ProviderGSC && integration.Status == "connected" {
			return true
		}
	}
	return false
}

func decodeNormalizationConfig(raw json.RawMessage) URLNormalizationConfig {
	var cfg URLNormalizationConfig
	_ = json.Unmarshal(raw, &cfg)
	return cfg
}

func uuidToPG(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func strPtr(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

func HashContent(content string, seoMeta json.RawMessage) string {
	sum := sha256.Sum256([]byte(content + string(seoMeta)))
	return hex.EncodeToString(sum[:])
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

func ErrBadStatus(status string) error {
	return fmt.Errorf("unsupported SEO status %q", status)
}
