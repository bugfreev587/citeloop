// Package api exposes the HTTP surface (Chi) for project/profile/inventory CRUD,
// agent runs, the review queue (the only human gate, §5.5), and manual ticks.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/citeloop/citeloop/internal/scheduler"
	"github.com/citeloop/citeloop/internal/search"
	"github.com/citeloop/citeloop/internal/seo"
	"github.com/clerk/clerk-sdk-go/v2"
	clerkhttp "github.com/clerk/clerk-sdk-go/v2/http"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	Pool    *pgxpool.Pool
	Q       *db.Queries
	LLM     llm.Provider
	Search  search.Provider
	Blog    *publisher.BlogPublisher
	Sched   *scheduler.Scheduler
	Env     config.Env
	Log     *slog.Logger
	SEOData seo.GoogleDataProvider
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer)
	r.Use(corsMiddleware)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	r.Route("/api", func(r chi.Router) {
		if s.Env.ClerkSecretKey != "" {
			clerk.SetKey(s.Env.ClerkSecretKey)
			r.Use(clerkhttp.RequireHeaderAuthorization())
		}

		r.Get("/admin/llm-credentials", s.getLLMCredentials)
		r.Put("/admin/llm-credentials", s.updateLLMCredentials)

		r.Get("/projects", s.listProjects)
		r.Post("/projects", s.createProject)
		r.Route("/projects/{projectID}", func(r chi.Router) {
			r.Get("/", s.getProject)
			r.Put("/config", s.updateConfig)

			r.Post("/insight", s.runInsight)
			r.Get("/profile", s.getProfile)
			r.Put("/profile", s.updateProfile)

			r.Get("/inventory", s.listInventory)
			r.Put("/inventory/{itemID}", s.updateInventory)
			r.Delete("/inventory/{itemID}", s.deleteInventory)

			r.Post("/strategist", s.runStrategist)
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
			r.Post("/articles/{articleID}/approve", s.approveProjectArticle)
			r.Post("/articles/{articleID}/reject", s.rejectProjectArticle)
			r.Post("/articles/{articleID}/distributed", s.markProjectDistributed)
			r.Post("/articles/{articleID}/retry-publish", s.retryProjectPublish)
			r.Get("/distribute", s.listDistribute)
			r.Get("/runs", s.listRuns)
			r.Get("/runs/{runID}", s.getRun)
			r.Route("/seo", func(r chi.Router) {
				r.Get("/overview", s.getSEOOverview)
				r.Post("/sync", s.syncSEO)
				r.Post("/analyze", s.analyzeSEO)
				r.Get("/runs", s.listSEORuns)
				r.Get("/opportunities", s.listSEOOpportunities)
				r.Get("/opportunities/{opportunityID}", s.getSEOOpportunity)
				r.Post("/opportunities/{opportunityID}/accept", s.acceptSEOOpportunity)
				r.Post("/opportunities/{opportunityID}/dismiss", s.dismissSEOOpportunity)
				r.Post("/opportunities/{opportunityID}/actions", s.createSEOContentAction)
				r.Get("/actions", s.listSEOContentActions)
				r.Get("/actions/{actionID}", s.getSEOContentAction)
				r.Post("/actions/{actionID}/generate-draft", func(w http.ResponseWriter, r *http.Request) {
					s.updateSEOContentActionStatus(w, r, "ready_for_review")
				})
				r.Post("/actions/{actionID}/approve", func(w http.ResponseWriter, r *http.Request) {
					s.updateSEOContentActionStatus(w, r, "approved")
				})
				r.Post("/actions/{actionID}/publish", func(w http.ResponseWriter, r *http.Request) {
					s.updateSEOContentActionStatus(w, r, "measuring")
				})
				r.Get("/briefs/latest", s.getSEOBrief)
				r.Get("/settings", s.getSEOSettings)
				r.Put("/settings", s.updateSEOSettings)
				r.Route("/autopilot", func(r chi.Router) {
					r.Get("/objectives", s.listSEOObjectives)
					r.Post("/objectives", s.createSEOObjective)
					r.Get("/policy", s.getSEOPolicy)
					r.Put("/policy", s.updateSEOPolicy)
					r.Post("/plans/generate", s.generateAutopilotPlan)
					r.Get("/plans", s.listAutopilotPlans)
					r.Get("/safe-mode", s.listSafeModeEvents)
					r.Post("/safe-mode", s.enterSafeMode)
					r.Post("/safe-mode/{safeModeID}/exit", s.exitSafeMode)
				})
			})
			r.Get("/notifications/channels", s.listNotificationChannels)
			r.Post("/notifications/channels", s.createNotificationChannel)
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
