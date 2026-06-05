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
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	Pool   *pgxpool.Pool
	Q      *db.Queries
	LLM    llm.Provider
	Search search.Provider
	Blog   *publisher.BlogPublisher
	Sched  *scheduler.Scheduler
	Env    config.Env
	Log    *slog.Logger
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.Recoverer)
	r.Use(corsMiddleware)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	r.Route("/api", func(r chi.Router) {
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
			r.Post("/topics/{topicID}/generate", s.generateTopic)

			r.Get("/review", s.listReview)
			r.Get("/articles", s.listArticles)
			r.Get("/distribute", s.listDistribute)

			r.Post("/tick/generate", s.tickGenerate)
			r.Post("/tick/publish", s.tickPublish)
		})
		r.Route("/articles/{articleID}", func(r chi.Router) {
			r.Put("/", s.editArticle)
			r.Post("/approve", s.approveArticle)
			r.Post("/reject", s.rejectArticle)
			r.Post("/distributed", s.markDistributed)
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
