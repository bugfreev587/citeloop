// Package api exposes the HTTP surface (Chi) for project/profile/inventory CRUD,
// agent runs, the review queue (the only human gate, §5.5), and manual ticks.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/citeloop/citeloop/internal/aicalls"
	"github.com/citeloop/citeloop/internal/articleassets"
	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/citeloop/citeloop/internal/scheduler"
	"github.com/citeloop/citeloop/internal/search"
	"github.com/citeloop/citeloop/internal/seo"
	"github.com/citeloop/citeloop/internal/sitefix"
	"github.com/clerk/clerk-sdk-go/v2"
	clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	Pool             *pgxpool.Pool
	Q                *db.Queries
	AICalls          aicalls.Store
	LLM              llm.Provider
	Search           search.Provider
	Blog             *publisher.BlogPublisher
	Sched            *scheduler.Scheduler
	Env              config.Env
	Log              *slog.Logger
	SEOData          seo.GoogleDataProvider
	ArticleAssets    *articleassets.Service
	SiteFixes        DoctorSiteFixService
	SiteFixLifecycle DoctorSiteFixLifecycleService
	SiteFixMigration siteFixMigrationService
	githubAppClient  githubAppAPI

	githubReadinessStore      githubPRReadinessStore
	githubReadinessChecker    githubPRReadinessChecker
	githubReadinessHTTPClient *http.Client
	githubReadinessAPIBase    string
	githubReadinessNow        func() time.Time
	canonicalSiteFixPRRunner  func(context.Context, uuid.UUID, uuid.UUID, githubPRReadinessTarget) (sitefix.ApplyResult, error)

	OnboardingRunner         projectOnboardingRunner
	InsightInventoryRunner   insightInventoryRunner
	SEOOnboardingRunner      seoOnboardingRunner
	DoctorOnboardingRunner   projectOnboardingRunner
	ContextOpportunityRunner contextOpportunityRunner

	// emailResolver is a test seam for resolving a Clerk subject to its email;
	// nil in production, where the Clerk Backend API is used.
	emailResolver func(ctx context.Context, subject string) string
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer)
	r.Use(corsMiddleware)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	r.Get("/assets/{token}", s.getPublicArticleAsset)

	r.Route("/api", func(r chi.Router) {
		if s.Env.ClerkSecretKey != "" {
			clerk.SetKey(s.Env.ClerkSecretKey)
			r.Use(clerkhttp.RequireHeaderAuthorization())
		}

		r.Get("/me", s.me)

		r.Group(func(r chi.Router) {
			r.Use(s.requireAdmin)
			r.Get("/admin/llm-credentials", s.getLLMCredentials)
			r.Put("/admin/llm-credentials", s.updateLLMCredentials)
			r.Post("/admin/llm-credentials/test", s.testLLMCredentials)
			r.Delete("/admin/llm-credentials", s.deleteLLMCredentials)
			r.Get("/admin/image-credentials", s.getImageCredentials)
			r.Put("/admin/image-credentials", s.updateImageCredentials)
			r.Delete("/admin/image-credentials", s.deleteImageCredentials)
			r.Get("/admin/geo-credentials", s.listAdminGEOCredentials)
			r.Put("/admin/geo-credentials/{scope}", s.updateAdminGEOCredentials)
			r.Post("/admin/geo-credentials/{scope}/test", s.testAdminGEOCredentials)
			r.Delete("/admin/geo-credentials/{scope}", s.deleteAdminGEOCredentials)
			r.Get("/admin/projects", s.listAdminProjects)
			r.Delete("/admin/projects/{projectID}", s.deleteAdminProject)
			r.Post("/admin/projects/{projectID}/discovery-shadow/run", s.runAdminDiscoveryShadow)
			r.Get("/admin/projects/{projectID}/discovery-shadow/report", s.getAdminDiscoveryShadowReport)
			r.Post("/admin/projects/{projectID}/discovery-arbitration/{candidateID}/prepare", s.prepareAdminDiscoveryArbitration)
			r.Get("/admin/projects/{projectID}/discovery-review", s.listAdminDiscoveryReview)
			r.Get("/admin/projects/{projectID}/discovery-review/{candidateID}", s.getAdminDiscoveryReview)
			r.Post("/admin/projects/{projectID}/discovery-review/{candidateID}/resolve", s.resolveAdminDiscoveryReview)
			r.Get("/admin/projects/{projectID}/discovery-semantic-evaluation", s.getAdminDiscoverySemanticEvaluation)
			r.Post("/admin/projects/{projectID}/discovery-semantic-evaluation/run", s.runAdminDiscoverySemanticEvaluation)
			r.Post("/admin/projects/{projectID}/site-fix-migration/dry-run", s.dryRunAdminSiteFixMigration)
			r.Post("/admin/projects/{projectID}/site-fix-migration/apply", s.applyAdminSiteFixMigration)
			r.Get("/admin/projects/{projectID}/site-fix-migration/reviews", s.listAdminMigrationReviews)
			r.Post("/admin/projects/{projectID}/site-fix-migration/reviews/{reviewID}/resolve", s.resolveAdminMigrationReview)
			r.Post("/admin/projects/{projectID}/site-fix-migration/{batchID}/rollback", s.rollbackAdminSiteFixMigration)
			r.Get("/admin/projects/{projectID}/site-fix-migration/{batchID}", s.getAdminSiteFixMigrationReport)
			r.Get("/admin/users", s.listAdminUsers)
			r.Delete("/admin/users/{ownerID}", s.deleteAdminUser)
		})

		r.Get("/projects", s.listProjects)
		r.Post("/projects", s.createProject)
		r.Route("/projects/{projectID}", func(r chi.Router) {
			r.Use(s.requireProjectOwner)
			r.Get("/", s.getProject)
			r.Delete("/", s.deleteProject)
			r.Put("/config", s.updateConfig)
			r.Get("/platform-contracts/capabilities", s.getPlatformContractCapabilities)
			r.Get("/platform-target-contexts", s.listPlatformTargetContexts)
			r.Post("/platform-target-contexts", s.confirmPlatformTargetContext)
			r.Post("/platform-target-contexts/{contextID}/reconfirm", s.reconfirmPlatformTargetContext)

			r.Post("/insight", s.runInsight)
			r.Post("/context/refresh", s.refreshContext)
			r.Get("/profile", s.getProfile)
			r.Put("/profile", s.updateProfile)

			r.Get("/inventory", s.listInventory)
			r.Put("/inventory/{itemID}", s.updateInventory)
			r.Delete("/inventory/{itemID}", s.deleteInventory)

			r.Get("/topics", s.listTopics)
			r.Put("/topics/{topicID}", s.updateTopic)
			r.Post("/topics/{topicID}/generate", s.generateTopic)
			r.Post("/topics/{topicID}/schedule", s.scheduleTopic)
			r.Post("/topics/{topicID}/archive", s.archiveTopic)

			r.Get("/review", s.listReview)
			r.Get("/articles", s.listArticles)
			r.Get("/articles/{articleID}", s.getProjectArticle)
			r.Put("/articles/{articleID}", s.editProjectArticle)
			r.Post("/articles/{articleID}/ai-fix", s.fixProjectArticle)
			r.Post("/articles/{articleID}/apply-fix", s.applyFixProjectArticle)
			r.Post("/articles/{articleID}/recheck", s.recheckProjectArticle)
			r.Post("/articles/{articleID}/approve", s.approveProjectArticle)
			r.Post("/articles/{articleID}/reject", s.rejectProjectArticle)
			r.Post("/articles/{articleID}/distributed", s.markProjectDistributed)
			r.Post("/articles/{articleID}/retry-publish", s.retryProjectPublish)
			r.Post("/articles/{articleID}/publish-now", s.publishProjectArticleNow)
			r.Get("/articles/{articleID}/assets", s.listProjectArticleAssets)
			r.Put("/articles/{articleID}/assets/{assetID}", s.editProjectArticleAsset)
			r.Post("/articles/{articleID}/assets/{assetID}/regenerate", s.regenerateProjectArticleAsset)
			r.Get("/distribute", s.listDistribute)
			r.Get("/runs", s.listRuns)
			r.Get("/runs/{runID}", s.getRun)
			r.Get("/publisher-connections", s.listPublisherConnections)
			r.Put("/publisher-connections/github-nextjs", s.upsertGitHubNextJSPublisherConnection)
			r.Put("/publisher-connections/dev-to", s.upsertDevToPublisherConnection)
			r.Delete("/publisher-connections/{connectionID}", s.deletePublisherConnection)
			r.Put("/publisher-connections/{connectionID}/enabled", s.setPublisherConnectionEnabled)
			r.Post("/publisher-connections/{connectionID}/test", s.testPublisherConnection)
			r.Put("/publisher-connections/{connectionID}/credential", s.upsertPublisherCredential)
			r.Delete("/publisher-connections/{connectionID}/credential", s.revokePublisherCredential)
			r.Get("/integrations/github", s.getGithubIntegration)
			r.Post("/integrations/github/installation", s.storeGithubInstallation)
			r.Get("/integrations/github/repos", s.listGithubRepos)
			r.Post("/integrations/github/select-repo", s.selectGithubRepo)
			r.Get("/integrations/github/pr-readiness", s.getGitHubPRReadiness)
			r.Post("/integrations/github/pr-readiness/check", s.checkGitHubPRReadiness)
			r.Get("/results/actions", s.listResultsActions)
			r.Get("/results/actions/{actionID}", s.getResultsAction)
			r.Post("/results/recompute", s.recomputeResults)
			// Opportunities is the canonical Growth loop surface. Keep the
			// legacy /seo routes below as compatibility aliases while new clients
			// use product-domain names that do not expose implementation details.
			r.Post("/opportunities/runs", s.runOpportunityFinding)
			r.Get("/opportunities/status", s.getOpportunityFindingStatus)
			r.Get("/opportunities", s.listSEOOpportunities)
			r.Get("/opportunities/radar", s.getGrowthRadarDiagnostics)
			r.Get("/opportunities/{opportunityID}", s.getSEOOpportunity)
			r.Get("/growth-actions", s.listResultsActions)
			r.Get("/growth-actions/{actionID}/measurement", s.getResultsAction)
			r.Get("/growth-learnings", s.listGrowthLearnings)
			r.Route("/seo", func(r chi.Router) {
				r.Get("/overview", s.getSEOOverview)
				r.Post("/sync", s.syncSEO)
				r.Post("/analyze", s.analyzeSEO)
				r.Get("/runs", s.listSEORuns)
				r.Get("/opportunity-finding/status", s.getOpportunityFindingStatus)
				r.Get("/opportunity-finding/radar", s.getGrowthRadarDiagnostics)
				r.Post("/opportunity-finding/run", s.runOpportunityFinding)
				r.Get("/visibility/summary", s.getVisibilitySummary)
				r.Get("/opportunities", s.listSEOOpportunities)
				r.Get("/opportunities/{opportunityID}", s.getSEOOpportunity)
				r.Post("/opportunities/{opportunityID}/accept", s.acceptSEOOpportunity)
				r.Post("/opportunities/{opportunityID}/dismiss", s.dismissSEOOpportunity)
				r.Post("/opportunities/{opportunityID}/snooze", s.snoozeSEOOpportunity)
				r.Post("/opportunities/{opportunityID}/unsnooze", s.unsnoozeSEOOpportunity)
				r.Post("/opportunities/{opportunityID}/watch", s.watchSEOOpportunity)
				r.Post("/opportunities/{opportunityID}/actions", s.createSEOContentAction)
				r.Get("/watchlist", s.listSEOWatchlist)
				r.Post("/watchlist/{watchlistItemID}/close", s.closeSEOWatchlistItem)
				r.Get("/actions", s.listSEOContentActions)
				r.Get("/actions/{actionID}", s.getSEOContentAction)
				r.Post("/actions/{actionID}/plan", s.planSEOContentAction)
				r.Post("/actions/{actionID}/return-to-opportunity", s.returnSEOContentActionToOpportunity)
				r.Post("/actions/{actionID}/site-fix-pr", s.createSiteFixGitHubPR)
				r.Post("/actions/{actionID}/page-update-drafts", s.createPageUpdateDraftForAction)
				r.Get("/page-update-drafts/{draftID}", s.getPageUpdateDraft)
				r.Post("/page-update-drafts/{draftID}/generate", s.generatePageUpdateDraft)
				r.Post("/page-update-drafts/{draftID}/approve", s.approvePageUpdateDraft)
				r.Post("/page-update-drafts/{draftID}/apply", s.applyPageUpdateDraft)
				r.Post("/page-update-drafts/{draftID}/verify", s.verifyPageUpdateDraft)
				r.Post("/actions/{actionID}/generate-draft", func(w http.ResponseWriter, r *http.Request) {
					s.updateSEOContentActionStatus(w, r, "ready_for_review")
				})
				r.Post("/actions/{actionID}/approve", func(w http.ResponseWriter, r *http.Request) {
					s.updateSEOContentActionStatus(w, r, "approved")
				})
				r.Post("/actions/{actionID}/publish", func(w http.ResponseWriter, r *http.Request) {
					s.updateSEOContentActionStatus(w, r, "measuring")
				})
				r.Post("/actions/{actionID}/verify", s.verifySEOContentAction)
				r.Post("/actions/{actionID}/dismiss", s.dismissSEOContentActionAndOpportunity)
				r.Get("/briefs/latest", s.getSEOBrief)
				s.registerDoctorRoutes(r, "/doctor")
				r.Get("/settings", s.getSEOSettings)
				r.Put("/settings", s.updateSEOSettings)
				r.Get("/gsc/connection", s.getGSCConnection)
				r.Post("/gsc/oauth/start", s.startGSCOAuth)
				r.Post("/gsc/oauth/complete", s.completeGSCOAuth)
				r.Post("/gsc/property", s.selectGSCProperty)
				r.Post("/gsc/revoke", s.revokeGSCConnection)
				r.Route("/autopilot", func(r chi.Router) {
					r.Get("/objectives", s.listSEOObjectives)
					r.Post("/objectives", s.createSEOObjective)
					r.Get("/policy", s.getSEOPolicy)
					r.Put("/policy", s.updateSEOPolicy)
					r.Get("/readiness", s.getAutopilotReadiness)
					r.Post("/plans/generate", s.generateAutopilotPlan)
					r.Get("/plans", s.listAutopilotPlans)
					r.Post("/plans/{planID}/execute", s.executeAutopilotPlan)
					r.Get("/safe-mode", s.listSafeModeEvents)
					r.Post("/safe-mode", s.enterSafeMode)
					r.Post("/safe-mode/{safeModeID}/exit", s.exitSafeMode)
				})
			})
			s.registerDoctorRoutes(r, "/doctor")
			s.registerCanonicalDoctorSiteFixRoutes(r, "/doctor")
			r.Route("/geo", func(r chi.Router) {
				r.Get("/overview", s.getGEOOverview)
				r.Post("/crawler-audit", s.runGEOCrawlerAudit)
				r.Get("/crawler-audit/latest", s.getLatestGEOCrawlerAudit)
				r.Post("/prompt-sets/generate", s.generateGEOPromptSet)
				r.Get("/prompt-sets", s.listGEOPromptSets)
				r.Put("/prompt-sets/{promptSetID}", s.updateGEOPromptSet)
				r.Put("/prompts/{promptID}", s.updateGEOPrompt)
				r.Put("/competitors/{competitorID}", s.updateGEOCompetitor)
				r.Get("/runs", s.listGEORuns)
				r.Post("/runs/observe", s.observeGEOManualFixtures)
				r.Post("/runs/observe-provider", s.observeGEOAnswerProvider)
				r.Get("/observations", s.listGEOObservations)
				r.Get("/external-surfaces", s.listGEOExternalSurfaces)
				r.Post("/external-surfaces", s.createGEOExternalSurface)
				r.Post("/external-surfaces/monitor", s.monitorGEOExternalSurfaces)
				r.Post("/opportunities/analyze", s.analyzeGEOOpportunities)
				r.Get("/asset-briefs", s.listGEOAssetBriefs)
				r.Post("/asset-briefs/{briefID}/accept", s.acceptGEOAssetBrief)
			})
			r.Get("/notifications/channels", s.listNotificationChannels)
			r.Post("/notifications/channels", s.createNotificationChannel)
			r.Patch("/notifications/channels/{channelID}", s.updateNotificationChannel)
			r.Post("/notifications/channels/{channelID}/test", s.testNotificationChannel)
			r.Delete("/notifications/channels/{channelID}", s.deleteNotificationChannel)
			r.Get("/notifications/events", s.listNotificationEvents)
			r.Get("/notifications/subscriptions", s.listNotificationSubscriptions)
			r.Put("/notifications/subscriptions", s.upsertNotificationSubscription)
			r.Get("/notifications/deliveries", s.listNotificationDeliveries)
			r.Post("/notifications/deliveries/{deliveryID}/retry", s.retryNotificationDelivery)
			r.Post("/publishing/reconcile", s.reconcilePublishing)

			r.Post("/tick/generate", s.tickGenerate)
			r.Post("/tick/publish", s.tickPublish)
		})
	})
	return r
}

// --- small response helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
