package geo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	AgentCrawlerAudit     = "geo_crawler_audit"
	ProviderHonestProbe   = "citeloop_honest_probe"
	OpportunityTypeAccess = "geo_crawler_access_blocked"
)

type Store interface {
	GetActiveProfile(ctx context.Context, projectID uuid.UUID) (db.ProductProfile, error)
	GetSEOPropertyForProject(ctx context.Context, projectID uuid.UUID) (db.SeoProperty, error)
	ListTopics(ctx context.Context, projectID uuid.UUID) ([]db.Topic, error)
	ListPublishedCanonicalArticlesForSEO(ctx context.Context, projectID uuid.UUID) ([]db.Article, error)
	StartGEORun(ctx context.Context, arg db.StartGEORunParams) (db.GeoRun, error)
	FinishGEORun(ctx context.Context, arg db.FinishGEORunParams) (db.GeoRun, error)
	UpsertAICrawlerAccessSnapshot(ctx context.Context, arg db.UpsertAICrawlerAccessSnapshotParams) (db.AiCrawlerAccessSnapshot, error)
	ListLatestAICrawlerAccessSnapshots(ctx context.Context, projectID uuid.UUID) ([]db.AiCrawlerAccessSnapshot, error)
	UpsertCrawlerAccessOpportunity(ctx context.Context, arg db.UpsertCrawlerAccessOpportunityParams) (db.UpsertCrawlerAccessOpportunityRow, error)
	CreateGEOPromptSet(ctx context.Context, arg db.CreateGEOPromptSetParams) (db.GeoPromptSet, error)
	ListGEOPromptSets(ctx context.Context, arg db.ListGEOPromptSetsParams) ([]db.GeoPromptSet, error)
	GetGEOPromptSetForProject(ctx context.Context, arg db.GetGEOPromptSetForProjectParams) (db.GeoPromptSet, error)
	UpdateGEOPromptSet(ctx context.Context, arg db.UpdateGEOPromptSetParams) (db.GeoPromptSet, error)
	CreateGEOPrompt(ctx context.Context, arg db.CreateGEOPromptParams) (db.GeoPrompt, error)
	ListGEOPrompts(ctx context.Context, arg db.ListGEOPromptsParams) ([]db.GeoPrompt, error)
	ListActiveGEOPrompts(ctx context.Context, projectID uuid.UUID) ([]db.GeoPrompt, error)
	UpdateGEOPrompt(ctx context.Context, arg db.UpdateGEOPromptParams) (db.GeoPrompt, error)
	UpsertGEOCompetitor(ctx context.Context, arg db.UpsertGEOCompetitorParams) (db.GeoCompetitor, error)
	ListGEOCompetitors(ctx context.Context, arg db.ListGEOCompetitorsParams) ([]db.GeoCompetitor, error)
	UpdateGEOCompetitor(ctx context.Context, arg db.UpdateGEOCompetitorParams) (db.GeoCompetitor, error)
	UpsertGEOExternalSurface(ctx context.Context, arg db.UpsertGEOExternalSurfaceParams) (db.GeoExternalSurface, error)
	ListGEOExternalSurfaces(ctx context.Context, arg db.ListGEOExternalSurfacesParams) ([]db.GeoExternalSurface, error)
	ListProjectOwnedGEOExternalSurfaces(ctx context.Context, projectID uuid.UUID) ([]db.GeoExternalSurface, error)
	CreateGEOObservation(ctx context.Context, arg db.CreateGEOObservationParams) (db.GeoObservation, error)
	ListGEOObservations(ctx context.Context, arg db.ListGEOObservationsParams) ([]db.GeoObservation, error)
	ListGEOObservationsForRun(ctx context.Context, arg db.ListGEOObservationsForRunParams) ([]db.GeoObservation, error)
	CreateGEOVisibilityScore(ctx context.Context, arg db.CreateGEOVisibilityScoreParams) (db.GeoVisibilityScore, error)
	GetLatestGEOVisibilityScore(ctx context.Context, projectID uuid.UUID) (db.GeoVisibilityScore, error)
	ListGEOVisibilityScores(ctx context.Context, arg db.ListGEOVisibilityScoresParams) ([]db.GeoVisibilityScore, error)
	UpsertGEOObservationOpportunity(ctx context.Context, arg db.UpsertGEOObservationOpportunityParams) (db.UpsertGEOObservationOpportunityRow, error)
	CreateGEOAssetBrief(ctx context.Context, arg db.CreateGEOAssetBriefParams) (db.GeoAssetBrief, error)
	ListGEOAssetBriefs(ctx context.Context, arg db.ListGEOAssetBriefsParams) ([]db.GeoAssetBrief, error)
	GetGEOAssetBriefForProject(ctx context.Context, arg db.GetGEOAssetBriefForProjectParams) (db.GeoAssetBrief, error)
	UpdateGEOAssetBriefStatus(ctx context.Context, arg db.UpdateGEOAssetBriefStatusParams) (db.GeoAssetBrief, error)
	CreateTopic(ctx context.Context, arg db.CreateTopicParams) (db.Topic, error)
}

type Service struct {
	Q              Store
	HTTPClient     *http.Client
	AnswerProvider AnswerProvider
	Now            func() time.Time
}

type CrawlerAuditRequest struct {
	SiteURL          string   `json:"site_url,omitempty"`
	URLs             []string `json:"urls,omitempty"`
	TargetUserAgents []string `json:"target_user_agents,omitempty"`
}

type CrawlerAuditResult struct {
	Run             db.GeoRun                    `json:"run"`
	Snapshots       []db.AiCrawlerAccessSnapshot `json:"snapshots"`
	CheckedURLs     int                          `json:"checked_urls"`
	CreatedBlockers int                          `json:"created_blockers"`
	DataSourceNotes []string                     `json:"data_source_notes"`
}

func (s Service) RunCrawlerAudit(ctx context.Context, projectID uuid.UUID, req CrawlerAuditRequest) (CrawlerAuditResult, error) {
	now := s.now()
	run, err := s.Q.StartGEORun(ctx, db.StartGEORunParams{
		ProjectID: projectID,
		Agent:     AgentCrawlerAudit,
		Provider:  ProviderHonestProbe,
		StartedAt: pgutil.TS(now),
		Input:     jsonBytes(req),
	})
	if err != nil {
		return CrawlerAuditResult{}, err
	}

	result := CrawlerAuditResult{
		Run:             run,
		DataSourceNotes: []string{"robots_static_authoritative", "http_waf_signals_inferred"},
	}
	finish := func(status string, output any, runErr error) (CrawlerAuditResult, error) {
		var errText *string
		if runErr != nil {
			message := runErr.Error()
			errText = &message
		}
		finished, finishErr := s.Q.FinishGEORun(ctx, db.FinishGEORunParams{
			ID:         run.ID,
			ProjectID:  projectID,
			Status:     status,
			FinishedAt: pgutil.TS(s.now()),
			Output:     jsonBytes(output),
			Error:      errText,
			CostUsd:    pgtype.Numeric{},
		})
		if finishErr == nil {
			result.Run = finished
		}
		if runErr != nil {
			return result, runErr
		}
		return result, finishErr
	}

	property, err := s.Q.GetSEOPropertyForProject(ctx, projectID)
	if err != nil {
		return finish("error", result, err)
	}
	siteURL := strings.TrimSpace(req.SiteURL)
	if siteURL == "" {
		siteURL = strings.TrimSpace(property.SiteUrl)
	}
	if siteURL == "" {
		return finish("error", result, errors.New("geo crawler audit requires a site URL"))
	}

	articles, err := s.Q.ListPublishedCanonicalArticlesForSEO(ctx, projectID)
	if err != nil {
		return finish("error", result, err)
	}
	urls := sampleCrawlerAuditURLs(siteURL, req.URLs, articles)
	if len(urls) == 0 {
		return finish("error", result, errors.New("geo crawler audit requires at least one URL"))
	}
	result.CheckedURLs = len(urls)

	auditResults := Auditor{HTTPClient: s.HTTPClient, Now: s.Now}.Audit(ctx, AuditRequest{
		SiteURL:          siteURL,
		URLs:             urls,
		TargetUserAgents: req.TargetUserAgents,
	})

	blockerURLs := map[string]struct{}{}
	for _, audited := range auditResults {
		snapshot, err := s.Q.UpsertAICrawlerAccessSnapshot(ctx, db.UpsertAICrawlerAccessSnapshotParams{
			ProjectID:         projectID,
			RunID:             run.ID,
			PageUrl:           audited.PageURL,
			NormalizedPageUrl: audited.NormalizedPageURL,
			TargetUserAgent:   audited.TargetUserAgent,
			ProbeUserAgent:    audited.ProbeUserAgent,
			EvidenceType:      audited.EvidenceType,
			RobotsState:       string(audited.RobotsState),
			HttpStatus:        audited.HTTPStatus,
			AccessState:       audited.AccessState,
			Confidence:        audited.Confidence,
			Inferred:          audited.Inferred,
			MetaRobotsState:   stringPtr(audited.MetaRobotsState),
			SitemapState:      stringPtr(audited.SitemapState),
			BodyExtractable:   audited.BodyExtractable,
			RawDetails:        jsonBytes(auditDetails(audited)),
			CheckedAt:         pgutil.TS(now),
		})
		if err != nil {
			return finish("error", result, err)
		}
		result.Snapshots = append(result.Snapshots, snapshot)

		if audited.RobotsState != RobotsDisallowed || audited.Confidence != ConfidenceHigh {
			continue
		}
		if _, ok := blockerURLs[audited.NormalizedPageURL]; ok {
			continue
		}
		blockerURLs[audited.NormalizedPageURL] = struct{}{}
		if err := s.upsertCrawlerAccessBlocker(ctx, projectID, run.ID, audited); err != nil {
			return finish("error", result, err)
		}
		result.CreatedBlockers++
	}

	return finish("ok", result, nil)
}

func (s Service) LatestCrawlerAudit(ctx context.Context, projectID uuid.UUID) ([]db.AiCrawlerAccessSnapshot, error) {
	return s.Q.ListLatestAICrawlerAccessSnapshots(ctx, projectID)
}

func (s Service) upsertCrawlerAccessBlocker(ctx context.Context, projectID, runID uuid.UUID, audited AuditResult) error {
	action := "Review robots.txt policy for search-related AI crawlers and allow this path when it matches the project's indexing policy."
	impact := "Restores a high-confidence crawlability precondition for AI answer-engine discovery and citation."
	_, err := s.Q.UpsertCrawlerAccessOpportunity(ctx, db.UpsertCrawlerAccessOpportunityParams{
		ProjectID:         projectID,
		Type:              OpportunityTypeAccess,
		Status:            "open",
		PriorityScore:     pgutil.Numeric(90),
		Confidence:        pgutil.Numeric(95),
		PageUrl:           &audited.PageURL,
		NormalizedPageUrl: audited.NormalizedPageURL,
		Evidence: jsonBytes(map[string]any{
			"run_id":             runID,
			"page_url":           audited.PageURL,
			"target_user_agent":  audited.TargetUserAgent,
			"probe_user_agent":   audited.ProbeUserAgent,
			"evidence_type":      audited.EvidenceType,
			"robots_state":       audited.RobotsState,
			"confidence":         audited.Confidence,
			"inferred":           audited.Inferred,
			"source_confidence":  "robots_static_authoritative",
			"opportunity_source": AgentCrawlerAudit,
		}),
		RecommendedAction: &action,
		ExpectedImpact:    &impact,
		Effort:            2,
		RiskLevel:         "medium",
	})
	return err
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func sampleCrawlerAuditURLs(siteURL string, requested []string, articles []db.Article) []string {
	values := []string{siteURL}
	values = append(values, requested...)
	for _, article := range articles {
		if article.CanonicalUrl == nil {
			continue
		}
		values = append(values, *article.CanonicalUrl)
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		resolved, ok := absoluteURL(value, siteURL)
		if !ok {
			continue
		}
		out = append(out, resolved)
	}
	return uniqueStrings(out)
}

func absoluteURL(rawURL, siteURL string) (string, bool) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", false
	}
	base, baseErr := url.Parse(strings.TrimSpace(siteURL))
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", false
	}
	if !parsed.IsAbs() {
		if baseErr != nil || base == nil {
			return "", false
		}
		parsed = base.ResolveReference(parsed)
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}
	if parsed.Host == "" {
		return "", false
	}
	return parsed.String(), true
}

func auditDetails(audited AuditResult) map[string]any {
	details := map[string]any{}
	for key, value := range audited.RawDetails {
		details[key] = value
	}
	details["target_user_agent"] = audited.TargetUserAgent
	details["probe_user_agent"] = audited.ProbeUserAgent
	details["evidence_type"] = audited.EvidenceType
	details["robots_state"] = audited.RobotsState
	details["access_state"] = audited.AccessState
	details["confidence"] = audited.Confidence
	details["inferred"] = audited.Inferred
	return details
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func jsonBytes(value any) json.RawMessage {
	b, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}
