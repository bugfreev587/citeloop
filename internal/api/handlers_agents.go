package api

import (
	"encoding/json"
	"net/http"

	"github.com/citeloop/citeloop/internal/agents"
	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/platform"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (s *Server) projectConfig(r *http.Request, id uuid.UUID) (config.ProjectConfig, error) {
	p, err := s.Q.GetProject(r.Context(), id)
	if err != nil {
		return config.ProjectConfig{}, err
	}
	return config.Parse(p.Config)
}

// runInsight crawls the landing URL and builds profile + inventory (§5.1).
func (s *Server) runInsight(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	var in struct {
		LandingURL string `json:"landing_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.LandingURL == "" {
		writeErr(w, 400, "landing_url required")
		return
	}
	cfg, err := s.projectConfig(r, id)
	if err != nil {
		writeErr(w, 404, "project not found")
		return
	}
	ag := agents.NewInsight(agents.Deps{Q: s.Q, LLM: s.LLM, Search: s.Search}, s.Log)
	profile, count, err := ag.Run(r.Context(), id, in.LandingURL, cfg.Crawl)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"profile": profile, "inventory_count": count})
}

// runStrategist produces the topic backlog (§5.2).
func (s *Server) runStrategist(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	cfg, err := s.projectConfig(r, id)
	if err != nil {
		writeErr(w, 404, "project not found")
		return
	}
	ag := agents.NewStrategist(agents.Deps{Q: s.Q, LLM: s.LLM, Search: s.Search}, s.Log)
	topics, err := ag.Run(r.Context(), id, cfg)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, topics)
}

func (s *Server) listTopics(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	topics, err := s.Q.ListTopics(r.Context(), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, topics)
}

// generateTopic runs Writer+QA for a single topic on demand (§5.3).
func (s *Server) generateTopic(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	topicID, err := uuid.Parse(chi.URLParam(r, "topicID"))
	if err != nil {
		writeErr(w, 400, "bad topic id")
		return
	}
	topic, err := s.Q.GetTopic(r.Context(), topicID)
	if err != nil {
		writeErr(w, 404, "topic not found")
		return
	}
	ag := agents.NewWriter(agents.Deps{Q: s.Q, LLM: s.LLM, Search: s.Search}, s.Log)
	arts, err := ag.Generate(r.Context(), id, topic)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, arts)
}

func (s *Server) tickGenerate(w http.ResponseWriter, r *http.Request) {
	s.Sched.TickGenerate(r.Context())
	writeJSON(w, 200, map[string]string{"status": "generate tick complete"})
}

func (s *Server) tickPublish(w http.ResponseWriter, r *http.Request) {
	s.Sched.TickPublish(r.Context())
	writeJSON(w, 200, map[string]string{"status": "publish tick complete"})
}

// listDistribute returns ready_to_distribute variants enriched with the target
// platform's compose URL so the UI can offer "copy variant + open compose page"
// (PRD §5.6).
func (s *Server) listDistribute(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	arts, err := s.Q.ListArticlesByStatus(r.Context(), db.ListArticlesByStatusParams{ProjectID: id, Status: "ready_to_distribute"})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(arts))
	for _, a := range arts {
		plat := ""
		if a.Platform != nil {
			plat = *a.Platform
		}
		out = append(out, map[string]any{
			"article":             a,
			"compose_url":         publisher.ComposeURL(plat),
			"supports_canonical":  platform.SupportsCanonical(platform.Platform(plat)),
		})
	}
	writeJSON(w, 200, out)
}

func (s *Server) listArticles(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "published"
	}
	arts, err := s.Q.ListArticlesByStatus(r.Context(), db.ListArticlesByStatusParams{ProjectID: id, Status: status})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, arts)
}
