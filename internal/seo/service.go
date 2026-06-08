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
	"github.com/citeloop/citeloop/internal/googledata"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	ProviderGSC       = "google_search_console"
	ProviderGA4       = "google_analytics"
	DefaultBriefLimit = 10
)

type GoogleDataProvider interface {
	FetchSearchConsole(context.Context, googledata.SearchConsoleRequest) (googledata.SearchConsoleData, error)
	FetchAnalytics(context.Context, googledata.AnalyticsRequest) ([]googledata.AnalyticsPageRow, error)
}

// Service coordinates the Operations Loop backend workflow.
type Service struct {
	Q           *db.Queries
	HTTPClient  *http.Client
	BlogBaseURL string
	GoogleData  GoogleDataProvider
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
	SetupChecklist      []SetupChecklistItem         `json:"setup_checklist"`
	CapabilityMode      string                       `json:"capability_mode"`
	Last28Days          db.SEOOverviewStatsRow       `json:"last_28_days"`
	Technical           db.SEOTechnicalSummaryRow    `json:"technical"`
	OpportunitiesByType []db.SEOOpportunityCountsRow `json:"opportunities_by_type"`
	ActionsByStatus     []db.ContentActionCountsRow  `json:"actions_by_status"`
	ColdStart           bool                         `json:"cold_start"`
	HandoffReadyForAuto bool                         `json:"handoff_ready_for_autopilot"`
	DataSourceWarnings  []string                     `json:"data_source_warnings"`
}

type SetupChecklistItem struct {
	Key              string `json:"key"`
	Label            string `json:"label"`
	Status           string `json:"status"`
	WhyNeeded        string `json:"why_needed"`
	NextAction       string `json:"next_action"`
	CapabilityImpact string `json:"capability_impact"`
}

type setupChecklistInput struct {
	Integrations         []db.SeoIntegration
	PublisherConnections []db.PublisherConnection
	ColdStart            bool
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
	publisherConnections, err := s.Q.ListPublisherConnections(ctx, projectID)
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
	out.Integrations = nonNilSlice(integrations)
	out.Last28Days = stats
	out.Technical = technical
	out.OpportunitiesByType = nonNilSlice(opps)
	out.ActionsByStatus = nonNilSlice(actions)
	out.ColdStart = isColdStart(stats)
	out.SetupChecklist, out.CapabilityMode, out.HandoffReadyForAuto = buildSetupChecklist(setupChecklistInput{
		Integrations:         integrations,
		PublisherConnections: publisherConnections,
		ColdStart:            out.ColdStart,
	})
	if !hasConnectedGSC(integrations) {
		out.DataSourceWarnings = append(out.DataSourceWarnings, "Google Search Console service account is not connected; search metrics are unavailable.")
	}
	if !hasConnectedPublisher(publisherConnections) {
		out.DataSourceWarnings = append(out.DataSourceWarnings, "Publisher connection is not ready; CiteLoop can draft content but cannot auto-publish for this project.")
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
	result := SyncResult{RunID: run.ID, Status: "ok", DataSourceNotes: []string{}}
	finish := func(status string, output any, runErr error) (SyncResult, error) {
		result.Status = status
		var errText *string
		if runErr != nil {
			s := runErr.Error()
			errText = &s
		}
		finishCtx, cancel := finishRunContext(ctx)
		defer cancel()
		finished, err := s.Q.FinishSEORun(finishCtx, db.FinishSEORunParams{
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
	if !isProviderAttemptable(integrations, ProviderGSC) {
		result.DataSourceNotes = append(result.DataSourceNotes, "gsc_missing")
	} else if s.GoogleData == nil {
		result.DataSourceNotes = append(result.DataSourceNotes, "google_service_account_env_missing")
		result.ConnectedGSC = false
	} else if prop.GscSiteUrl == nil || strings.TrimSpace(*prop.GscSiteUrl) == "" {
		result.DataSourceNotes = append(result.DataSourceNotes, "gsc_site_url_missing")
		result.ConnectedGSC = false
	} else {
		if err := s.syncGoogleMetrics(ctx, projectID, prop, integrations, now, &result); err != nil {
			result.DataSourceNotes = append(result.DataSourceNotes, "google_metrics_error")
			result.ConnectedGSC = false
			errText := err.Error()
			_, _ = s.Q.UpsertSEOIntegration(ctx, db.UpsertSEOIntegrationParams{
				ProjectID:      projectID,
				Provider:       ProviderGSC,
				Status:         "error",
				CredentialRef:  integrationCredentialRef(integrations, ProviderGSC),
				LastVerifiedAt: pgtype.Timestamptz{},
				LastError:      &errText,
			})
		} else {
			result.ConnectedGSC = true
		}
	}
	articles, err := s.Q.ListPublishedCanonicalArticlesForSEO(ctx, projectID)
	if err != nil {
		return finish("error", result, err)
	}
	for _, article := range articles {
		if article.CanonicalUrl == nil || strings.TrimSpace(*article.CanonicalUrl) == "" {
			continue
		}
		normalized, status, err := s.recordTechnicalCheck(ctx, projectID, run.ID, prop, *article.CanonicalUrl, uuidToPG(article.ID), article.ContentHash, strings.Contains(strings.ToLower(article.ContentMd), "<script"))
		if err != nil {
			return finish("error", result, err)
		}
		result.CheckedURLs++
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
	if result.CheckedURLs == 0 && strings.TrimSpace(prop.SiteUrl) != "" {
		_, _, err := s.recordTechnicalCheck(ctx, projectID, run.ID, prop, prop.SiteUrl, pgtype.UUID{}, nil, false)
		if err != nil {
			return finish("error", result, err)
		}
		result.CheckedURLs++
		result.DataSourceNotes = append(result.DataSourceNotes, "public_site_checked")
	}
	overview, err := s.Overview(ctx, projectID)
	if err != nil {
		return finish("error", result, err)
	}
	result.ColdStart = overview.ColdStart
	status := "ok"
	if !result.ConnectedGSC {
		status = "degraded"
	} else if hasNote(result.DataSourceNotes, "google_metrics_error") {
		status = "degraded"
	}
	return finish(status, result, nil)
}

func (s Service) syncGoogleMetrics(ctx context.Context, projectID uuid.UUID, prop db.SeoProperty, integrations []db.SeoIntegration, now time.Time, result *SyncResult) error {
	if s.GoogleData == nil {
		return nil
	}
	if prop.GscSiteUrl == nil || strings.TrimSpace(*prop.GscSiteUrl) == "" {
		result.DataSourceNotes = append(result.DataSourceNotes, "gsc_site_url_missing")
		result.ConnectedGSC = false
		return nil
	}
	days, err := s.Q.SEODataDayCount(ctx, db.SEODataDayCountParams{ProjectID: projectID, PropertyID: prop.ID})
	if err != nil {
		return err
	}
	end := now.AddDate(0, 0, -2)
	window := 28
	if days == 0 {
		window = 90
	}
	start := end.AddDate(0, 0, -window+1)
	cfg := decodeNormalizationConfig(prop.UrlNormalizationConfig)
	gsc, err := s.GoogleData.FetchSearchConsole(ctx, googledata.SearchConsoleRequest{
		SiteURL:   *prop.GscSiteUrl,
		StartDate: start,
		EndDate:   end,
		RowLimit:  25000,
	})
	if err != nil {
		return err
	}
	for _, row := range gsc.PageRows {
		normalized, err := NormalizeURL(row.PageURL, prop.SiteUrl, cfg)
		if err != nil {
			continue
		}
		_, err = s.Q.UpsertPagePerformanceDaily(ctx, db.UpsertPagePerformanceDailyParams{
			ProjectID:         projectID,
			PropertyID:        prop.ID,
			Date:              pgDate(row.Date),
			PageUrl:           row.PageURL,
			NormalizedPageUrl: normalized,
			Clicks:            pgutil.Numeric(row.Clicks),
			Impressions:       pgutil.Numeric(row.Impressions),
			WeightedPosition:  pgutil.Numeric(row.Position),
			Ctr:               pgutil.Numeric(row.CTR),
			DataSourceNotes:   mustJSON(map[string]any{"gsc_status": "connected", "gsc_source": "searchanalytics.page"}),
		})
		if err != nil {
			return err
		}
	}
	for _, row := range gsc.QueryRows {
		normalized, err := NormalizeURL(row.PageURL, prop.SiteUrl, cfg)
		if err != nil {
			continue
		}
		_, err = s.Q.UpsertSearchPerformanceDaily(ctx, db.UpsertSearchPerformanceDailyParams{
			ProjectID:         projectID,
			PropertyID:        prop.ID,
			Date:              pgDate(row.Date),
			PageUrl:           row.PageURL,
			NormalizedPageUrl: normalized,
			Query:             row.Query,
			Country:           row.Country,
			Device:            row.Device,
			Clicks:            pgutil.Numeric(row.Clicks),
			Impressions:       pgutil.Numeric(row.Impressions),
			Ctr:               pgutil.Numeric(row.CTR),
			Position:          pgutil.Numeric(row.Position),
			QueryDataPartial:  true,
			Source:            "gsc_api",
		})
		if err != nil {
			return err
		}
	}
	for _, row := range gsc.AppearanceRows {
		_, err = s.Q.UpsertSearchAppearanceDaily(ctx, db.UpsertSearchAppearanceDailyParams{
			ProjectID:        projectID,
			PropertyID:       prop.ID,
			Date:             pgDate(row.Date),
			SearchAppearance: row.SearchAppearance,
			Clicks:           pgutil.Numeric(row.Clicks),
			Impressions:      pgutil.Numeric(row.Impressions),
			Ctr:              pgutil.Numeric(row.CTR),
			Position:         pgutil.Numeric(row.Position),
			Source:           "gsc_api",
		})
		if err != nil {
			return err
		}
	}
	result.DataSourceNotes = append(result.DataSourceNotes, fmt.Sprintf("gsc_rows:%d/%d/%d", len(gsc.PageRows), len(gsc.QueryRows), len(gsc.AppearanceRows)))
	_, _ = s.Q.UpsertSEOIntegration(ctx, db.UpsertSEOIntegrationParams{
		ProjectID:      projectID,
		Provider:       ProviderGSC,
		Status:         "connected",
		CredentialRef:  integrationCredentialRef(integrations, ProviderGSC),
		LastVerifiedAt: pgutil.TS(s.now()),
	})
	if prop.Ga4PropertyID == nil || strings.TrimSpace(*prop.Ga4PropertyID) == "" || !isProviderAttemptable(integrations, ProviderGA4) {
		if prop.Ga4PropertyID == nil || strings.TrimSpace(*prop.Ga4PropertyID) == "" {
			result.DataSourceNotes = append(result.DataSourceNotes, "ga4_property_missing")
		}
		return nil
	}
	ga4Rows, err := s.GoogleData.FetchAnalytics(ctx, googledata.AnalyticsRequest{
		PropertyID: *prop.Ga4PropertyID,
		StartDate:  start,
		EndDate:    end,
		RowLimit:   25000,
	})
	if err != nil {
		errText := err.Error()
		_, _ = s.Q.UpsertSEOIntegration(ctx, db.UpsertSEOIntegrationParams{
			ProjectID:      projectID,
			Provider:       ProviderGA4,
			Status:         "error",
			CredentialRef:  integrationCredentialRef(integrations, ProviderGA4),
			LastVerifiedAt: pgtype.Timestamptz{},
			LastError:      &errText,
		})
		result.DataSourceNotes = append(result.DataSourceNotes, "ga4_error")
		return nil
	}
	for _, row := range ga4Rows {
		rawURL := absolutePageURL(prop.SiteUrl, row.PagePath)
		normalized, err := NormalizeURL(rawURL, prop.SiteUrl, cfg)
		if err != nil {
			continue
		}
		_, err = s.Q.UpsertPagePerformanceDaily(ctx, db.UpsertPagePerformanceDailyParams{
			ProjectID:          projectID,
			PropertyID:         prop.ID,
			Date:               pgDate(row.Date),
			PageUrl:            rawURL,
			NormalizedPageUrl:  normalized,
			Ga4Sessions:        pgutil.Numeric(row.Sessions),
			Ga4EngagedSessions: pgutil.Numeric(row.EngagedSessions),
			Ga4Conversions:     pgutil.Numeric(row.KeyEvents),
			DataSourceNotes:    mustJSON(map[string]any{"ga4_status": "connected", "ga4_source": "runReport"}),
		})
		if err != nil {
			return err
		}
	}
	result.DataSourceNotes = append(result.DataSourceNotes, fmt.Sprintf("ga4_rows:%d", len(ga4Rows)))
	_, _ = s.Q.UpsertSEOIntegration(ctx, db.UpsertSEOIntegrationParams{
		ProjectID:      projectID,
		Provider:       ProviderGA4,
		Status:         "connected",
		CredentialRef:  integrationCredentialRef(integrations, ProviderGA4),
		LastVerifiedAt: pgutil.TS(s.now()),
	})
	return nil
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
	result := SyncResult{RunID: run.ID, Status: "ok", DataSourceNotes: []string{}}
	finish := func(status string, output any, runErr error) (SyncResult, error) {
		result.Status = status
		var errText *string
		if runErr != nil {
			s := runErr.Error()
			errText = &s
		}
		finishCtx, cancel := finishRunContext(ctx)
		defer cancel()
		_, err := s.Q.FinishSEORun(finishCtx, db.FinishSEORunParams{
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
		Actions:     nonNilSlice(opps),
		Blockers:    blockers,
		Measurement: []string{"No completed SEO measurement windows yet."},
	}, nil
}

func (s Service) recordTechnicalCheck(
	ctx context.Context,
	projectID uuid.UUID,
	runID uuid.UUID,
	prop db.SeoProperty,
	rawURL string,
	articleID pgtype.UUID,
	contentHash *string,
	unsafeMDX bool,
) (string, string, error) {
	normalized, err := NormalizeURL(rawURL, prop.SiteUrl, decodeNormalizationConfig(prop.UrlNormalizationConfig))
	if err != nil {
		return "", "", err
	}
	check := s.checkURL(ctx, rawURL, prop.SiteUrl)
	status := "unknown"
	if check.HTTPStatus != nil && *check.HTTPStatus >= 200 && *check.HTTPStatus < 300 {
		status = "ok"
	} else if check.HTTPStatus != nil {
		status = "http_error"
	}
	_, err = s.Q.UpsertTechnicalCheck(ctx, db.UpsertTechnicalCheckParams{
		ProjectID:             projectID,
		RunID:                 runID,
		PageUrl:               rawURL,
		NormalizedPageUrl:     normalized,
		ArticleID:             articleID,
		HttpStatus:            check.HTTPStatus,
		CanonicalStatus:       strPtr(check.CanonicalStatus),
		RobotsStatus:          strPtr(check.RobotsStatus),
		TitleStatus:           strPtr(check.TitleStatus),
		MetaDescriptionStatus: strPtr(check.MetaDescriptionStatus),
		H1Status:              strPtr(check.H1Status),
		StructuredDataStatus:  strPtr(check.StructuredDataStatus),
		InternalLinkCount:     &check.InternalLinkCount,
		OutboundLinkCount:     &check.OutboundLinkCount,
		ContentHash:           contentHash,
		UnsafeMdxDetected:     unsafeMDX,
		RawDetails:            mustJSON(check.RawDetails),
	})
	return normalized, status, err
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

func finishRunContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
}

func hasConnectedGSC(integrations []db.SeoIntegration) bool {
	return isProviderConnected(integrations, ProviderGSC)
}

func buildSetupChecklist(in setupChecklistInput) ([]SetupChecklistItem, string, bool) {
	searchConnected := hasConnectedGSC(in.Integrations)
	analyticsConnected := isProviderConnected(in.Integrations, ProviderGA4)
	publisherConnected := hasConnectedPublisher(in.PublisherConnections)
	publisherExists := len(in.PublisherConnections) > 0

	mode := "public_only"
	if searchConnected && publisherConnected {
		mode = "customer_site_connected"
	} else if searchConnected && publisherExists {
		mode = "customer_site_pending_verification"
	}
	ready := searchConnected && publisherConnected && !in.ColdStart

	publisherStatus := "blocked"
	publisherNextAction := "Connect a publisher target and save a scoped credential."
	if publisherConnected {
		publisherStatus = "connected"
		publisherNextAction = "No action needed."
	} else if publisherExists {
		publisherStatus = "in_progress"
		publisherNextAction = "Save the publisher credential, then run Test."
	}

	searchStatus := "blocked"
	searchNextAction := "Connect first-party Search Console data or continue in public-only mode."
	if searchConnected {
		searchStatus = "connected"
		searchNextAction = "No action needed."
	}

	analyticsStatus := "optional"
	analyticsNextAction := "Connect Analytics when conversion and engagement attribution is needed."
	if analyticsConnected {
		analyticsStatus = "connected"
		analyticsNextAction = "No action needed."
	}

	return []SetupChecklistItem{
		{
			Key:              "public_crawl",
			Label:            "Public crawl",
			Status:           "connected",
			WhyNeeded:        "CiteLoop needs public pages, robots, sitemap, and metadata before it can draft recommendations.",
			NextAction:       "No action needed.",
			CapabilityImpact: "Enables public-only crawl, technical checks, and cold-start briefs.",
		},
		{
			Key:              "search_data",
			Label:            "Search data",
			Status:           searchStatus,
			WhyNeeded:        "First-party search data lets CiteLoop prioritize opportunities using real queries, impressions, CTR, and position.",
			NextAction:       searchNextAction,
			CapabilityImpact: "Missing search data limits planning to public crawl and SERP signals.",
		},
		{
			Key:              "analytics_data",
			Label:            "Analytics data",
			Status:           analyticsStatus,
			WhyNeeded:        "Analytics data connects SEO actions to engagement and conversion outcomes.",
			NextAction:       analyticsNextAction,
			CapabilityImpact: "Missing analytics data removes conversion signals but does not block SEO drafting.",
		},
		{
			Key:              "publisher_write",
			Label:            "Publishing",
			Status:           publisherStatus,
			WhyNeeded:        "CiteLoop needs a scoped publisher connection before it can create or update content automatically.",
			NextAction:       publisherNextAction,
			CapabilityImpact: "Missing publishing keeps generated work in review/draft mode.",
		},
		{
			Key:              "policy",
			Label:            "Autopilot policy",
			Status:           "connected",
			WhyNeeded:        "Policy defines which low-risk actions can run automatically and which require review.",
			NextAction:       "Review policy before raising automation level.",
			CapabilityImpact: "Required before enabling higher automation levels.",
		},
		{
			Key:              "dry_run",
			Label:            "Dry run",
			Status:           "not_started",
			WhyNeeded:        "A dry run verifies permissions and publishing behavior before real writes.",
			NextAction:       "Run a dry-run publish check after connections are ready.",
			CapabilityImpact: "Blocks hands-off execution until permissions are proven.",
		},
	}, mode, ready
}

func hasConnectedPublisher(connections []db.PublisherConnection) bool {
	for _, connection := range connections {
		if connection.Kind == publisher.ConnectionKindGitHubNextJS &&
			connection.Status == "connected" &&
			connection.CredentialRef != nil &&
			strings.TrimSpace(*connection.CredentialRef) != "" {
			return true
		}
	}
	return false
}

func isProviderConnected(integrations []db.SeoIntegration, provider string) bool {
	for _, integration := range integrations {
		if integration.Provider == provider && integration.Status == "connected" {
			return true
		}
	}
	return false
}

func isProviderAttemptable(integrations []db.SeoIntegration, provider string) bool {
	for _, integration := range integrations {
		if integration.Provider != provider {
			continue
		}
		switch integration.Status {
		case "connected", "error", "expired":
			return true
		}
	}
	return false
}

func integrationCredentialRef(integrations []db.SeoIntegration, provider string) *string {
	for _, integration := range integrations {
		if integration.Provider == provider {
			return integration.CredentialRef
		}
	}
	return nil
}

func hasNote(notes []string, want string) bool {
	for _, note := range notes {
		if note == want {
			return true
		}
	}
	return false
}

func pgDate(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: !t.IsZero()}
}

func absolutePageURL(siteURL, path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || trimmed == "(not set)" {
		return strings.TrimRight(siteURL, "/") + "/"
	}
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		return trimmed
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	return strings.TrimRight(siteURL, "/") + trimmed
}

func decodeNormalizationConfig(raw json.RawMessage) URLNormalizationConfig {
	var cfg URLNormalizationConfig
	_ = json.Unmarshal(raw, &cfg)
	return cfg
}

func uuidToPG(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func nonNilSlice[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
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
