package geo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/aicalls"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/evidence"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	AgentCrawlerAudit   = "geo_crawler_audit"
	ProviderHonestProbe = "citeloop_honest_probe"
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
	ListApplicableGrowthLearnings(ctx context.Context, arg db.ListApplicableGrowthLearningsParams) ([]db.ListApplicableGrowthLearningsRow, error)
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
	EvidenceStore  evidence.Store
	AICallStore    aicalls.Store
	GrowthWriter   GrowthOpportunityWriter
	HTTPClient     *http.Client
	AnswerProvider AnswerProvider
	Now            func() time.Time
}

type GrowthOpportunityWriter interface {
	CreateOpportunity(context.Context, db.CreateCanonicalGrowthOpportunityParams) (db.SeoOpportunity, error)
	EnsureOpportunityReserved(context.Context, uuid.UUID, uuid.UUID) error
	CanExecuteOpportunity(context.Context, uuid.UUID, uuid.UUID) (bool, error)
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
	SkippedURLs     []string                     `json:"skipped_urls"`
	DataSourceNotes []string                     `json:"data_source_notes"`
}

func (s Service) RunCrawlerAudit(ctx context.Context, projectID uuid.UUID, req CrawlerAuditRequest) (CrawlerAuditResult, error) {
	req.TargetUserAgents = effectiveTargetUserAgents(req.TargetUserAgents)
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
		Run: run,
		DataSourceNotes: []string{
			"robots_static_authoritative",
			"http_waf_signals_inferred",
			"crawler_access_observations_feed_doctor",
		},
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
	urls, omittedURLs := sampleCrawlerAuditURLs(siteURL, req.URLs, articles)
	if len(urls) == 0 {
		return finish("error", result, errors.New("geo crawler audit requires at least one URL"))
	}
	result.CheckedURLs = len(urls)
	result.SkippedURLs = omittedURLs
	if len(omittedURLs) > 0 {
		result.DataSourceNotes = append(result.DataSourceNotes, "crawler_audit_url_cap_coverage_gap")
	}

	auditResults, err := s.collectCrawlerAuditEvidence(ctx, projectID, run.ID, siteURL, urls, omittedURLs, req.TargetUserAgents, now)
	if err != nil {
		return finish("error", result, err)
	}

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
			CheckedAt:         pgutil.TS(firstNonZeroTime(audited.ObservedAt, now)),
		})
		if err != nil {
			return finish("error", result, err)
		}
		result.Snapshots = append(result.Snapshots, snapshot)

	}

	status := "ok"
	_, auditCompleteness, _, _, failedAuditURLs := crawlerEvidenceQuality(auditResults, len(omittedURLs))
	if auditCompleteness < 1 {
		status = "degraded"
		result.DataSourceNotes = append(result.DataSourceNotes, fmt.Sprintf("crawler_audit_coverage_gap:%d", len(failedAuditURLs)+len(omittedURLs)))
	}
	return finish(status, result, nil)
}

func effectiveTargetUserAgents(requested []string) []string {
	if len(requested) == 0 {
		return DefaultTargetUserAgents()
	}
	out := make([]string, 0, len(requested))
	seen := map[string]bool{}
	for _, value := range requested {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	if len(out) == 0 {
		return DefaultTargetUserAgents()
	}
	return out
}

func (s Service) LatestCrawlerAudit(ctx context.Context, projectID uuid.UUID) ([]db.AiCrawlerAccessSnapshot, error) {
	return s.Q.ListLatestAICrawlerAccessSnapshots(ctx, projectID)
}

func (s Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func sampleCrawlerAuditURLs(siteURL string, requested []string, articles []db.Article) ([]string, []string) {
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
	out = uniqueStrings(out)
	if len(out) > 50 {
		return out[:50], out[50:]
	}
	return out, []string{}
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Now().UTC()
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
