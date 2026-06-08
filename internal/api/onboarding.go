package api

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/agents"
	"github.com/citeloop/citeloop/internal/config"
	"github.com/google/uuid"
)

const projectOnboardingTimeout = 5 * time.Minute

type projectOnboardingInput struct {
	ProjectID uuid.UUID
	SiteURL   string
}

type projectOnboardingRunner func(context.Context, projectOnboardingInput)

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

	ag := agents.NewInsight(agents.Deps{Q: s.Q, LLM: s.LLM, Search: s.Search}, log)
	if _, count, summary, err := ag.Run(ctx, in.ProjectID, in.SiteURL, cfg.Crawl); err != nil {
		log.Warn("project onboarding insight failed", "project_id", in.ProjectID, "err", err)
	} else {
		log.Info("project onboarding insight complete", "project_id", in.ProjectID, "inventory_count", count, "fetched", summary.FetchedCount)
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

func (s *Server) projectConfigByID(ctx context.Context, projectID uuid.UUID) (config.ProjectConfig, error) {
	p, err := s.Q.GetProject(ctx, projectID)
	if err != nil {
		return config.ProjectConfig{}, err
	}
	return config.Parse(p.Config)
}
