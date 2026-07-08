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
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/googledata"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/net/html"
)

const (
	ProviderGSC       = "google_search_console"
	ProviderGA4       = "google_analytics"
	DefaultBriefLimit = 10
)

const ga4ReconnectRequiredMessage = "Google Analytics permission is missing. Update Analytics access from Settings so CiteLoop can request Analytics read access, then run SEO sync again."

const ga4PropertyAccessRequiredMessage = "Google Analytics property access is missing. Confirm the numeric GA4 Property ID and grant the connected Google account Viewer access in GA4 Property Access Management, then run SEO sync again."

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
	Mode             string              `json:"mode"`
	Title            string              `json:"title"`
	GeneratedAt      time.Time           `json:"generated_at"`
	Actions          []db.SeoOpportunity `json:"actions"`
	Blockers         []string            `json:"blockers"`
	GEOBlockers      []string            `json:"geo_blockers"`
	GEOOpportunities []db.SeoOpportunity `json:"geo_opportunities"`
	Measurement      []string            `json:"measurement_updates"`
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
		status, errText, note := ga4IntegrationFailureForError(err)
		_, _ = s.Q.UpsertSEOIntegration(ctx, db.UpsertSEOIntegrationParams{
			ProjectID:      projectID,
			Provider:       ProviderGA4,
			Status:         status,
			CredentialRef:  integrationCredentialRef(integrations, ProviderGA4),
			LastVerifiedAt: pgtype.Timestamptz{},
			LastError:      &errText,
		})
		result.DataSourceNotes = append(result.DataSourceNotes, note)
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

func ga4IntegrationFailureForError(err error) (status string, message string, note string) {
	if googledata.IsInsufficientAuthenticationScopes(err) {
		return "reconnect_required", ga4ReconnectRequiredMessage, "ga4_reconnect_required"
	}
	if googledata.IsAnalyticsPropertyAccessDenied(err) {
		return "property_access_required", ga4PropertyAccessRequiredMessage, "ga4_property_access_required"
	}
	return "error", err.Error(), "ga4_error"
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
	metricGenerated, err := s.generateSearchMetricOpportunities(ctx, projectID, run.ID, prop)
	if err != nil {
		return finish("error", result, err)
	}
	if metricGenerated > 0 {
		result.GeneratedAnomalies += metricGenerated
		result.DataSourceNotes = append(result.DataSourceNotes, fmt.Sprintf("gsc_metric_opportunities:%d", metricGenerated))
	}
	actionableGenerated, err := s.generateActionableSEOOpportunities(ctx, projectID, run.ID, prop)
	if err != nil {
		return finish("error", result, err)
	}
	if actionableGenerated > 0 {
		result.GeneratedAnomalies += actionableGenerated
		result.DataSourceNotes = append(result.DataSourceNotes, fmt.Sprintf("actionable_seo_opportunities:%d", actionableGenerated))
	}
	overview, err := s.Overview(ctx, projectID)
	if err != nil {
		return finish("error", result, err)
	}
	result.ColdStart = overview.ColdStart
	if result.GeneratedAnomalies == 0 && result.ColdStart {
		generated, err := s.generateColdStartOpportunities(ctx, projectID, run.ID, prop)
		if err != nil {
			return finish("error", result, err)
		}
		if generated > 0 {
			result.GeneratedAnomalies += generated
			result.DataSourceNotes = append(result.DataSourceNotes, fmt.Sprintf("cold_start_opportunities:%d", generated))
		}
	}
	status := "ok"
	if result.ColdStart {
		status = "degraded"
	}
	return finish(status, result, nil)
}

func (s Service) generateSearchMetricOpportunities(ctx context.Context, projectID uuid.UUID, runID uuid.UUID, prop db.SeoProperty) (int, error) {
	if prop.ID == uuid.Nil {
		return 0, nil
	}
	queryRows, err := s.Q.ListSearchQueryOpportunityRollups(ctx, db.ListSearchQueryOpportunityRollupsParams{
		ProjectID:  projectID,
		PropertyID: prop.ID,
		Limit:      30,
	})
	if err != nil {
		return 0, err
	}
	decayRows, err := s.Q.ListPageDecayOpportunityRollups(ctx, db.ListPageDecayOpportunityRollupsParams{
		ProjectID:  projectID,
		PropertyID: prop.ID,
		Limit:      20,
	})
	if err != nil {
		return 0, err
	}
	candidates := searchMetricOpportunityCandidates(toSearchQueryRollups(queryRows), toPageDecayRollups(decayRows))
	generated := 0
	for _, candidate := range candidates {
		query := strings.TrimSpace(candidate.Query)
		var queryPtr *string
		if query != "" {
			queryPtr = &query
		}
		pageURL := strings.TrimSpace(candidate.PageURL)
		var pageURLPtr *string
		if pageURL != "" {
			pageURLPtr = &pageURL
		}
		action := candidate.RecommendedAction
		impact := candidate.ExpectedImpact
		_, err := s.Q.UpsertSEOOpportunity(ctx, db.UpsertSEOOpportunityParams{
			ProjectID:         projectID,
			Type:              candidate.Type,
			Status:            "open",
			PriorityScore:     pgutil.Numeric(candidate.PriorityScore),
			Confidence:        pgutil.Numeric(candidate.Confidence),
			PageUrl:           pageURLPtr,
			NormalizedPageUrl: candidate.NormalizedPageURL,
			Query:             queryPtr,
			Evidence:          mustJSON(candidate.Evidence),
			RecommendedAction: &action,
			ExpectedImpact:    &impact,
			Effort:            candidate.Effort,
			RiskLevel:         candidate.RiskLevel,
			CreatedByRunID:    uuidToPG(runID),
		})
		if err != nil {
			return generated, err
		}
		generated++
	}
	return generated, nil
}

func (s Service) generateActionableSEOOpportunities(ctx context.Context, projectID uuid.UUID, runID uuid.UUID, prop db.SeoProperty) (int, error) {
	checkRows, err := s.Q.ListLatestTechnicalChecks(ctx, db.ListLatestTechnicalChecksParams{
		ProjectID: projectID,
		LimitRows: 100,
	})
	if err != nil {
		return 0, err
	}
	inventoryRows, err := s.Q.ListInventory(ctx, projectID)
	if err != nil {
		return 0, err
	}
	queryRows := []db.ListSearchQueryOpportunityRollupsRow{}
	if prop.ID != uuid.Nil {
		queryRows, err = s.Q.ListSearchQueryOpportunityRollups(ctx, db.ListSearchQueryOpportunityRollupsParams{
			ProjectID:  projectID,
			PropertyID: prop.ID,
			Limit:      80,
		})
		if err != nil {
			return 0, err
		}
	}
	candidates := actionableSEOOpportunityCandidates(
		toTechnicalCheckRollups(checkRows),
		toInventoryEvidenceRollups(inventoryRows, prop),
		toSearchQueryRollups(queryRows),
	)
	generated := 0
	for _, candidate := range candidates {
		query := strings.TrimSpace(candidate.Query)
		var queryPtr *string
		if query != "" {
			queryPtr = &query
		}
		pageURL := strings.TrimSpace(candidate.PageURL)
		var pageURLPtr *string
		if pageURL != "" {
			pageURLPtr = &pageURL
		}
		action := candidate.RecommendedAction
		impact := candidate.ExpectedImpact
		_, err := s.Q.UpsertSEOOpportunity(ctx, db.UpsertSEOOpportunityParams{
			ProjectID:         projectID,
			Type:              candidate.Type,
			Status:            "open",
			PriorityScore:     pgutil.Numeric(candidate.PriorityScore),
			Confidence:        pgutil.Numeric(candidate.Confidence),
			PageUrl:           pageURLPtr,
			NormalizedPageUrl: candidate.NormalizedPageURL,
			Query:             queryPtr,
			Evidence:          mustJSON(candidate.Evidence),
			RecommendedAction: &action,
			ExpectedImpact:    &impact,
			Effort:            candidate.Effort,
			RiskLevel:         candidate.RiskLevel,
			CreatedByRunID:    uuidToPG(runID),
		})
		if err != nil {
			return generated, err
		}
		generated++
	}
	return generated, nil
}

func toTechnicalCheckRollups(rows []db.TechnicalCheck) []technicalCheckRollup {
	out := make([]technicalCheckRollup, 0, len(rows))
	for _, row := range rows {
		details := map[string]any{}
		if len(row.RawDetails) > 0 {
			_ = json.Unmarshal(row.RawDetails, &details)
		}
		out = append(out, technicalCheckRollup{
			PageURL:               row.PageUrl,
			NormalizedPageURL:     row.NormalizedPageUrl,
			HTTPStatus:            row.HttpStatus,
			CanonicalStatus:       stringPtrValue(row.CanonicalStatus),
			RobotsStatus:          stringPtrValue(row.RobotsStatus),
			TitleStatus:           stringPtrValue(row.TitleStatus),
			MetaDescriptionStatus: stringPtrValue(row.MetaDescriptionStatus),
			H1Status:              stringPtrValue(row.H1Status),
			StructuredDataStatus:  stringPtrValue(row.StructuredDataStatus),
			SitemapStatus:         stringPtrValue(row.SitemapStatus),
			InternalLinkCount:     row.InternalLinkCount,
			OutboundLinkCount:     row.OutboundLinkCount,
			RawDetails:            details,
		})
	}
	return out
}

func toInventoryEvidenceRollups(rows []db.ContentInventory, prop db.SeoProperty) []inventoryEvidenceRollup {
	out := make([]inventoryEvidenceRollup, 0, len(rows))
	normalization := decodeNormalizationConfig(prop.UrlNormalizationConfig)
	for _, row := range rows {
		normalized := strings.TrimSpace(row.Url)
		if value, err := NormalizeURL(row.Url, prop.SiteUrl, normalization); err == nil {
			normalized = value
		}
		summary := stringPtrValue(row.Summary)
		snippets := evidenceSnippets(row.EvidenceSnippets)
		out = append(out, inventoryEvidenceRollup{
			URL:               row.Url,
			NormalizedURL:     normalized,
			Title:             stringPtrValue(row.Title),
			Summary:           summary,
			EvidenceCount:     len(snippets),
			SummaryWordCount:  wordCount(summary),
			CapturedEvidence:  snippets,
			PrimarySourceType: row.Source,
		})
	}
	return out
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func toSearchQueryRollups(rows []db.ListSearchQueryOpportunityRollupsRow) []searchQueryRollup {
	out := make([]searchQueryRollup, 0, len(rows))
	for _, row := range rows {
		out = append(out, searchQueryRollup{
			PageURL:           row.PageUrl,
			NormalizedPageURL: row.NormalizedPageUrl,
			Query:             row.Query,
			Clicks:            pgutil.Float(row.Clicks28d),
			Impressions:       pgutil.Float(row.Impressions28d),
			CTR:               pgutil.Float(row.Ctr28d),
			Position:          pgutil.Float(row.Position28d),
			WindowStart:       dateFromPG(row.WindowStart),
			WindowEnd:         dateFromPG(row.WindowEnd),
		})
	}
	return out
}

func toPageDecayRollups(rows []db.ListPageDecayOpportunityRollupsRow) []pageDecayRollup {
	out := make([]pageDecayRollup, 0, len(rows))
	for _, row := range rows {
		out = append(out, pageDecayRollup{
			PageURL:             row.PageUrl,
			NormalizedPageURL:   row.NormalizedPageUrl,
			CurrentClicks:       pgutil.Float(row.CurrentClicks28d),
			PreviousClicks:      pgutil.Float(row.PreviousClicks28d),
			CurrentImpressions:  pgutil.Float(row.CurrentImpressions28d),
			PreviousImpressions: pgutil.Float(row.PreviousImpressions28d),
			WindowStart:         dateFromPG(row.WindowStart),
			WindowEnd:           dateFromPG(row.WindowEnd),
		})
	}
	return out
}

func dateFromPG(value pgtype.Date) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

type coldStartOpportunityCandidate struct {
	Type              string
	Query             string
	PageURL           string
	PriorityScore     float64
	Confidence        float64
	RecommendedAction string
	ExpectedImpact    string
	Effort            int32
	RiskLevel         string
	Evidence          map[string]any
}

func (s Service) generateColdStartOpportunities(ctx context.Context, projectID uuid.UUID, runID uuid.UUID, prop db.SeoProperty) (int, error) {
	profile, err := s.Q.GetActiveProfile(ctx, projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !profileHasContextConfirmation(profile.Profile) {
		return 0, nil
	}
	inventory, err := s.Q.ListInventory(ctx, projectID)
	if err != nil {
		return 0, err
	}
	candidates := coldStartOpportunityCandidates(profile.Profile, inventory, prop.SiteUrl)
	if len(candidates) == 0 {
		return 0, nil
	}
	existing, err := s.Q.ListSEOOpportunities(ctx, db.ListSEOOpportunitiesParams{
		ProjectID: projectID,
		LimitRows: 200,
	})
	if err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	for _, opportunity := range existing {
		if opportunity.Query == nil {
			continue
		}
		seen[strings.TrimSpace(*opportunity.Query)] = true
	}

	normalization := decodeNormalizationConfig(prop.UrlNormalizationConfig)
	generated := 0
	for _, candidate := range candidates {
		if seen[candidate.Query] {
			continue
		}
		normalized, err := NormalizeURL(candidate.PageURL, prop.SiteUrl, normalization)
		if err != nil {
			normalized, err = NormalizeURL(prop.SiteUrl, prop.SiteUrl, normalization)
			if err != nil {
				return generated, err
			}
		}
		pageURL := candidate.PageURL
		query := candidate.Query
		action := candidate.RecommendedAction
		impact := candidate.ExpectedImpact
		_, err = s.Q.UpsertSEOOpportunity(ctx, db.UpsertSEOOpportunityParams{
			ProjectID:         projectID,
			Type:              candidate.Type,
			Status:            "open",
			PriorityScore:     pgutil.Numeric(candidate.PriorityScore),
			Confidence:        pgutil.Numeric(candidate.Confidence),
			PageUrl:           strPtr(pageURL),
			NormalizedPageUrl: normalized,
			Evidence:          mustJSON(candidate.Evidence),
			Query:             &query,
			RecommendedAction: &action,
			ExpectedImpact:    &impact,
			Effort:            candidate.Effort,
			RiskLevel:         candidate.RiskLevel,
			CreatedByRunID:    uuidToPG(runID),
		})
		if err != nil {
			return generated, err
		}
		seen[candidate.Query] = true
		generated++
	}
	return generated, nil
}

func coldStartOpportunityCandidates(profileRaw json.RawMessage, inventory []db.ContentInventory, siteURL string) []coldStartOpportunityCandidate {
	var profile map[string]any
	if err := json.Unmarshal(profileRaw, &profile); err != nil {
		return nil
	}
	siteURL = strings.TrimSpace(siteURL)
	if siteURL == "" {
		siteURL = "https://unipost.dev"
	}
	positioning := firstProfileText(profile, "positioning", "summary", "description")
	icp := firstProfileText(profile, "icp", "ideal_customer_profile", "audience")
	valueProps := profileStringList(profile, "value_props", "value_propositions", "benefits")
	features := profileStringList(profile, "features", "capabilities")
	differentiators := profileStringList(profile, "differentiators", "why_us")
	competitors := profileStringList(profile, "competitors", "alternatives")
	keyTerms := profileStringList(profile, "key_terms", "keywords", "topics")
	source, evidenceCount := strongestEvidenceSource(inventory)

	baseEvidence := map[string]any{
		"source":         "context_confirmation",
		"positioning":    positioning,
		"icp":            icp,
		"value_props":    firstN(valueProps, 5),
		"features":       firstN(features, 5),
		"key_terms":      firstN(keyTerms, 8),
		"source_pages":   len(inventory),
		"evidence_count": evidenceCount,
	}
	candidates := []coldStartOpportunityCandidate{
		{
			Type:              "cold_start_context_plan",
			Query:             "cold-start:context-backed-use-case-pages",
			PageURL:           siteURL,
			PriorityScore:     72,
			Confidence:        68,
			RecommendedAction: "Plan the first context-backed use-case pages from the confirmed positioning",
			ExpectedImpact:    "Turns confirmed product facts and evidence into high-intent topics while Search Console data is missing or still too thin for confident query-level prioritization.",
			Effort:            3,
			RiskLevel:         "low",
			Evidence:          baseEvidence,
		},
	}
	if len(differentiators) > 0 || len(competitors) > 0 {
		candidates = append(candidates, coldStartOpportunityCandidate{
			Type:              "cold_start_competitive_gap",
			Query:             "cold-start:comparison-and-alternative-pages",
			PageURL:           siteURL,
			PriorityScore:     66,
			Confidence:        62,
			RecommendedAction: "Create comparison or alternative pages from confirmed differentiators",
			ExpectedImpact:    "Captures evaluation-stage demand with claims tied back to the confirmed Context profile.",
			Effort:            4,
			RiskLevel:         "medium",
			Evidence: map[string]any{
				"source":          "context_confirmation",
				"differentiators": firstN(differentiators, 6),
				"competitors":     firstN(competitors, 6),
			},
		})
	}
	if source.Url != "" && evidenceCount > 0 {
		candidates = append(candidates, coldStartOpportunityCandidate{
			Type:              "cold_start_evidence_page",
			Query:             "cold-start:evidence-led-source-page",
			PageURL:           source.Url,
			PriorityScore:     64,
			Confidence:        64,
			RecommendedAction: "Turn the strongest source page evidence into an opportunity brief",
			ExpectedImpact:    "Uses the most evidence-rich public page as the starting point for source-backed content planning.",
			Effort:            2,
			RiskLevel:         "low",
			Evidence: map[string]any{
				"source":             "context_confirmation",
				"source_page_url":    source.Url,
				"source_page_title":  source.Title,
				"evidence_count":     evidenceCount,
				"source_page_topics": rawJSONList(source.Topics, 6),
			},
		})
	}
	return candidates
}

func profileHasContextConfirmation(raw json.RawMessage) bool {
	var profile map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &profile) != nil {
		return false
	}
	for _, key := range []string{"context_confirmed_at", "confirmed_at"} {
		if value, ok := profile[key].(string); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func firstProfileText(profile map[string]any, keys ...string) string {
	for _, key := range keys {
		values := stringValues(profile[key], 1)
		if len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

func profileStringList(profile map[string]any, keys ...string) []string {
	for _, key := range keys {
		values := stringValues(profile[key], 12)
		if len(values) > 0 {
			return values
		}
	}
	return nil
}

func stringValues(value any, limit int) []string {
	out := []string{}
	var walk func(any)
	walk = func(current any) {
		if limit > 0 && len(out) >= limit {
			return
		}
		switch v := current.(type) {
		case string:
			for _, part := range splitProfileString(v) {
				if limit > 0 && len(out) >= limit {
					return
				}
				out = append(out, part)
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		case map[string]any:
			keys := make([]string, 0, len(v))
			for key := range v {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				walk(v[key])
			}
		}
	}
	walk(value)
	return compactStrings(out)
}

func splitProfileString(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	lines := strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ';'
	})
	if len(lines) <= 1 {
		return []string{value}
	}
	return compactStrings(lines)
}

func strongestEvidenceSource(inventory []db.ContentInventory) (db.ContentInventory, int) {
	var best db.ContentInventory
	bestCount := 0
	total := 0
	for _, item := range inventory {
		count := len(rawJSONList(item.EvidenceSnippets, 1000))
		total += count
		if count > bestCount {
			best = item
			bestCount = count
		}
	}
	return best, total
}

func rawJSONList(raw json.RawMessage, limit int) []string {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return stringValues(value, limit)
}

func firstN(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func compactStrings(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func (s Service) Brief(ctx context.Context, projectID uuid.UUID) (Brief, error) {
	overview, err := s.Overview(ctx, projectID)
	if err != nil {
		return Brief{}, err
	}
	opps, err := s.Q.ListSEOOpportunities(ctx, db.ListSEOOpportunitiesParams{
		ProjectID: projectID,
		Status:    "open",
		LimitRows: 50,
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
	geoBlockers, geoOpps := briefGEOSections(opps)
	return Brief{
		Mode:             mode,
		Title:            title,
		GeneratedAt:      s.now(),
		Actions:          firstSEOOpportunities(opps, DefaultBriefLimit),
		Blockers:         blockers,
		GEOBlockers:      geoBlockers,
		GEOOpportunities: geoOpps,
		Measurement:      []string{"No completed SEO measurement windows yet."},
	}, nil
}

func briefGEOSections(opps []db.SeoOpportunity) ([]string, []db.SeoOpportunity) {
	blockers := []string{}
	geoOpps := []db.SeoOpportunity{}
	for _, opp := range opps {
		if !strings.HasPrefix(opp.Type, "geo_") {
			continue
		}
		if opp.Type == "geo_crawler_access_blocked" {
			blockers = append(blockers, geoBlockerText(opp))
			continue
		}
		if len(geoOpps) < 5 {
			geoOpps = append(geoOpps, opp)
		}
	}
	return blockers, geoOpps
}

func geoBlockerText(opp db.SeoOpportunity) string {
	target := ""
	if opp.PageUrl != nil && strings.TrimSpace(*opp.PageUrl) != "" {
		target = strings.TrimSpace(*opp.PageUrl)
	} else if opp.Query != nil && strings.TrimSpace(*opp.Query) != "" {
		target = strings.TrimSpace(*opp.Query)
	} else {
		target = opp.Type
	}
	if opp.RecommendedAction != nil && strings.TrimSpace(*opp.RecommendedAction) != "" {
		return fmt.Sprintf("GEO crawler access blocker on %s: %s", target, strings.TrimSpace(*opp.RecommendedAction))
	}
	return "GEO crawler access blocker on " + target
}

func firstSEOOpportunities(opps []db.SeoOpportunity, limit int) []db.SeoOpportunity {
	if opps == nil {
		return []db.SeoOpportunity{}
	}
	if limit <= 0 || len(opps) <= limit {
		return opps
	}
	return opps[:limit]
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
	htmlStr := string(body)
	htmlLower := strings.ToLower(htmlStr)
	rawDetails := map[string]any{
		"status":     res.Status,
		"final_url":  res.Request.URL.String(),
		"body_bytes": len(body),
	}
	for key, value := range extractRepairMetadataFacts(htmlStr, res.Request.URL) {
		rawDetails[key] = value
	}
	return TechnicalResult{
		HTTPStatus:            &status,
		CanonicalStatus:       presenceStatus(htmlLower, `rel=["']canonical["']`),
		RobotsStatus:          robotsStatus(htmlLower),
		TitleStatus:           presenceStatus(htmlLower, `<title`),
		MetaDescriptionStatus: presenceStatus(htmlLower, `name=["']description["']`),
		H1Status:              presenceStatus(htmlLower, `<h1`),
		StructuredDataStatus:  presenceStatus(htmlLower, `application/ld\+json`),
		InternalLinkCount:     countLinks(htmlLower, siteURL, true),
		OutboundLinkCount:     countLinks(htmlLower, siteURL, false),
		RawDetails:            rawDetails,
	}
}

func extractRepairMetadataFacts(htmlStr string, baseURL *url.URL) map[string]any {
	out := map[string]any{"site_search_observed": false}
	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return out
	}
	logoCandidates := []string{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "html":
				if lang := strings.TrimSpace(attr(n, "lang")); lang != "" {
					out["html_lang"] = lang
				}
			case "title":
				if _, ok := out["page_title"]; !ok {
					if title := strings.TrimSpace(nodeText(n)); title != "" {
						out["page_title"] = title
					}
				}
			case "link":
				rel := strings.ToLower(attr(n, "rel"))
				href := strings.TrimSpace(attr(n, "href"))
				if href != "" && relHas(rel, "canonical") {
					if canonical := resolveURL(href, baseURL); canonical != "" {
						out["canonical_url"] = canonical
					}
				}
				if href != "" && (relHas(rel, "icon") || relHas(rel, "apple-touch-icon") || relHas(rel, "mask-icon")) {
					if logo := resolveURL(href, baseURL); logo != "" {
						logoCandidates = append(logoCandidates, logo)
					}
				}
			case "meta":
				key := strings.ToLower(firstNonEmpty(attr(n, "name"), attr(n, "property")))
				content := strings.TrimSpace(attr(n, "content"))
				if content == "" {
					break
				}
				switch key {
				case "description":
					out["meta_description"] = content
				case "application-name":
					out["application_name"] = content
				case "og:site_name":
					out["og_site_name"] = content
				case "og:title":
					out["og_title"] = content
				case "og:description":
					out["og_description"] = content
				case "og:url":
					if resolved := resolveURL(content, baseURL); resolved != "" {
						out["og_url"] = resolved
					}
				case "og:image":
					if image := resolveURL(content, baseURL); image != "" {
						out["og_image"] = image
					}
				}
			case "form":
				role := strings.ToLower(attr(n, "role"))
				action := strings.ToLower(attr(n, "action"))
				if role == "search" || strings.Contains(action, "search") {
					out["site_search_observed"] = true
					if resolved := resolveURL(attr(n, "action"), baseURL); resolved != "" {
						out["site_search_action_url"] = resolved
					}
				}
			case "input":
				inputType := strings.ToLower(attr(n, "type"))
				inputName := strings.ToLower(attr(n, "name"))
				if inputType == "search" || inputName == "q" || inputName == "query" || inputName == "search" {
					out["site_search_observed"] = true
				}
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	if logos := uniqueNonEmptyStrings(logoCandidates); len(logos) > 0 {
		out["logo_candidates"] = logos
	}
	return out
}

func attr(n *html.Node, name string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return b.String()
}

func relHas(rel, token string) bool {
	for _, part := range strings.Fields(rel) {
		if part == token {
			return true
		}
	}
	return false
}

func resolveURL(raw string, baseURL *url.URL) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || baseURL == nil {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return baseURL.ResolveReference(parsed).String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func uniqueNonEmptyStrings(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
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
			connection.Enabled &&
			hasPublisherWriteCredential(connection) {
			return true
		}
	}
	return false
}

func hasPublisherWriteCredential(connection db.PublisherConnection) bool {
	if connection.CredentialRef != nil && strings.TrimSpace(*connection.CredentialRef) != "" {
		return true
	}
	var cfg struct {
		InstallationID string `json:"installation_id"`
	}
	if len(connection.Config) > 0 && json.Unmarshal(connection.Config, &cfg) == nil {
		return strings.TrimSpace(cfg.InstallationID) != ""
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
		case "connected", "backfilling", "stale", "error", "expired":
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
