package api

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/citeloop/citeloop/internal/agents"
	"github.com/citeloop/citeloop/internal/config"
	"github.com/google/uuid"
)

const projectOnboardingTimeout = 5 * time.Minute
const onboardingInventoryMaxPages = 20
const onboardingInventorySitemapURLCap = 80
const onboardingInventoryRequestTimeoutMs = 5000
const onboardingInventoryMinRateLimitRPS = 3

type projectOnboardingInput struct {
	ProjectID uuid.UUID
	SiteURL   string
}

type projectOnboardingRunner func(context.Context, projectOnboardingInput)

type insightInventoryInput struct {
	ProjectID  uuid.UUID
	LandingURL string
	Crawl      config.CrawlConfig
}

type insightInventoryRunner func(context.Context, insightInventoryInput)
type seoOnboardingRunner func(context.Context, projectOnboardingInput)
type contextOpportunityRunner func(context.Context, uuid.UUID)

func (s *Server) startProjectOnboarding(projectID uuid.UUID, siteURL string) {
	siteURL = strings.TrimSpace(siteURL)
	if siteURL == "" {
		return
	}
	runner := s.OnboardingRunner
	if runner == nil {
		runner = s.runProjectOnboarding
	}
	in := projectOnboardingInput{ProjectID: projectID, SiteURL: siteURL}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), projectOnboardingTimeout)
		defer cancel()
		runner(ctx, in)
	}()
}

func (s *Server) startInsightInventoryCrawl(projectID uuid.UUID, landingURL string, crawlCfg config.CrawlConfig) {
	landingURL = strings.TrimSpace(landingURL)
	if landingURL == "" {
		return
	}
	crawlCfg = boundedOnboardingCrawlConfig(crawlCfg)
	runner := s.InsightInventoryRunner
	if runner == nil {
		runner = s.runInsightInventoryCrawl
	}
	in := insightInventoryInput{ProjectID: projectID, LandingURL: landingURL, Crawl: crawlCfg}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), projectOnboardingTimeout)
		defer cancel()
		runner(ctx, in)
	}()
}

func (s *Server) startContextOpportunityDiscovery(projectID uuid.UUID) {
	if projectID == uuid.Nil {
		return
	}
	runner := s.ContextOpportunityRunner
	if runner == nil {
		runner = s.runContextOpportunityDiscovery
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), projectOnboardingTimeout)
		defer cancel()
		runner(ctx, projectID)
	}()
}

func boundedOnboardingCrawlConfig(crawlCfg config.CrawlConfig) config.CrawlConfig {
	if crawlCfg.MaxPages <= 0 || crawlCfg.MaxPages > onboardingInventoryMaxPages {
		crawlCfg.MaxPages = onboardingInventoryMaxPages
	}
	if crawlCfg.SitemapURLCap <= 0 || crawlCfg.SitemapURLCap > onboardingInventorySitemapURLCap {
		crawlCfg.SitemapURLCap = onboardingInventorySitemapURLCap
	}
	if crawlCfg.RequestTimeoutMs <= 0 || crawlCfg.RequestTimeoutMs > onboardingInventoryRequestTimeoutMs {
		crawlCfg.RequestTimeoutMs = onboardingInventoryRequestTimeoutMs
	}
	if crawlCfg.RateLimitRPS < onboardingInventoryMinRateLimitRPS {
		crawlCfg.RateLimitRPS = onboardingInventoryMinRateLimitRPS
	}
	return crawlCfg
}

func (s *Server) runProjectOnboarding(ctx context.Context, in projectOnboardingInput) {
	log := s.Log
	if log == nil {
		log = slog.Default()
	}
	if s.Q == nil {
		log.Warn("project onboarding skipped: database unavailable", "project_id", in.ProjectID)
		return
	}

	cfg, err := s.projectConfigByID(ctx, in.ProjectID)
	if err != nil {
		log.Warn("project onboarding config unavailable", "project_id", in.ProjectID, "err", err)
		return
	}

	s.startInsightInventoryCrawl(in.ProjectID, in.SiteURL, cfg.Crawl)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		ag := agents.NewInsight(agents.Deps{Q: s.Q, LLM: s.LLM, Search: s.Search}, log)
		if _, summary, err := ag.RunQuickProfile(ctx, in.ProjectID, in.SiteURL, cfg.Crawl); err != nil {
			log.Warn("project onboarding quick profile failed", "project_id", in.ProjectID, "err", err)
		} else {
			log.Info("project onboarding quick profile complete", "project_id", in.ProjectID, "landing", summary.LandingURL)
		}
	}()
	go func() {
		defer wg.Done()
		runner := s.SEOOnboardingRunner
		if runner == nil {
			runner = s.runProjectSEOOnboarding
		}
		runner(ctx, in)
	}()
	wg.Wait()
}

func (s *Server) runProjectSEOOnboarding(ctx context.Context, in projectOnboardingInput) {
	log := s.Log
	if log == nil {
		log = slog.Default()
	}
	svc := s.seoService()
	syncResult, err := svc.Sync(ctx, in.ProjectID, in.SiteURL)
	if err != nil {
		log.Warn("project onboarding seo sync failed", "project_id", in.ProjectID, "err", err)
		return
	}
	analyzeResult, err := svc.Analyze(ctx, in.ProjectID)
	if err != nil {
		log.Warn("project onboarding seo analyze failed", "project_id", in.ProjectID, "err", err)
		return
	}
	log.Info(
		"project onboarding seo complete",
		"project_id", in.ProjectID,
		"sync_status", syncResult.Status,
		"analyze_status", analyzeResult.Status,
	)
}

func (s *Server) runContextOpportunityDiscovery(ctx context.Context, projectID uuid.UUID) {
	log := s.Log
	if log == nil {
		log = slog.Default()
	}
	if s.Q == nil {
		log.Warn("context opportunity discovery skipped: database unavailable", "project_id", projectID)
		return
	}
	result, err := s.seoService().Analyze(ctx, projectID)
	if err != nil {
		log.Warn("context opportunity discovery failed", "project_id", projectID, "err", err)
		return
	}
	log.Info(
		"context opportunity discovery complete",
		"project_id", projectID,
		"status", result.Status,
		"generated", result.GeneratedAnomalies,
	)
}

func (s *Server) runInsightInventoryCrawl(ctx context.Context, in insightInventoryInput) {
	log := s.Log
	if log == nil {
		log = slog.Default()
	}
	if s.Q == nil {
		log.Warn("insight inventory crawl skipped: database unavailable", "project_id", in.ProjectID)
		return
	}
	ag := agents.NewInsight(agents.Deps{Q: s.Q, LLM: s.LLM, Search: s.Search}, log)
	count, summary, err := ag.RunInventoryFromCrawl(ctx, in.ProjectID, in.LandingURL, in.Crawl)
	if err != nil {
		log.Warn("insight inventory crawl failed", "project_id", in.ProjectID, "err", err)
		return
	}
	log.Info("insight inventory crawl complete", "project_id", in.ProjectID, "inventory_count", count, "fetched", summary.FetchedCount)
}

func (s *Server) projectConfigByID(ctx context.Context, projectID uuid.UUID) (config.ProjectConfig, error) {
	p, err := s.Q.GetProject(ctx, projectID)
	if err != nil {
		return config.ProjectConfig{}, err
	}
	return config.Parse(p.Config)
}
