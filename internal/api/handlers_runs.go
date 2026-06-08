package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type generationRunDTO struct {
	ID           uuid.UUID           `json:"id"`
	ProjectID    uuid.UUID           `json:"project_id"`
	Agent        string              `json:"agent"`
	Input        any                 `json:"input"`
	Output       any                 `json:"output"`
	Model        *string             `json:"model"`
	Tokens       *int32              `json:"tokens"`
	CostUsd      pgtype.Numeric      `json:"cost_usd"`
	Status       string              `json:"status"`
	Error        *string             `json:"error"`
	CreatedAt    pgtype.Timestamptz  `json:"created_at"`
	RelatedLinks []runRelatedLinkDTO `json:"related_links,omitempty"`
	NextActions  []runRelatedLinkDTO `json:"next_actions,omitempty"`
}

type runRelatedLinkDTO struct {
	Label string `json:"label"`
	Href  string `json:"href"`
	Kind  string `json:"kind"`
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	limit := int32(50)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeErr(w, http.StatusBadRequest, "bad limit")
			return
		}
		if n > 100 {
			n = 100
		}
		limit = int32(n)
	}
	var cursor time.Time
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		cursor, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad cursor")
			return
		}
	}
	runs, err := s.Q.ListGenerationRuns(r.Context(), db.ListGenerationRunsParams{
		ProjectID:       projectID,
		Agent:           r.URL.Query().Get("agent"),
		Status:          r.URL.Query().Get("status"),
		CursorCreatedAt: generationRunCursorParam(cursor),
		LimitRows:       limit,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]generationRunDTO, 0, len(runs))
	for _, run := range runs {
		out = append(out, generationRunResponse(run, nil, runNextActions(projectID, run)))
	}
	writeJSON(w, http.StatusOK, emptySlice(out))
}

func generationRunCursorParam(cursor time.Time) pgtype.Timestamptz {
	if cursor.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgutil.TS(cursor)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad run id")
		return
	}
	run, err := s.Q.GetGenerationRun(r.Context(), db.GetGenerationRunParams{ID: runID, ProjectID: projectID})
	if err != nil {
		writeErr(w, http.StatusNotFound, "run not found")
		return
	}
	links := s.runRelatedLinks(r.Context(), projectID, run)
	writeJSON(w, http.StatusOK, generationRunResponse(run, links, runNextActions(projectID, run)))
}

func generationRunResponse(run db.GenerationRun, links []runRelatedLinkDTO, actions []runRelatedLinkDTO) generationRunDTO {
	return generationRunDTO{
		ID:           run.ID,
		ProjectID:    run.ProjectID,
		Agent:        run.Agent,
		Input:        sanitizeRunPayload(run.Input),
		Output:       sanitizeRunPayload(run.Output),
		Model:        run.Model,
		Tokens:       run.Tokens,
		CostUsd:      run.CostUsd,
		Status:       run.Status,
		Error:        sanitizeRunStringPtr(run.Error),
		CreatedAt:    run.CreatedAt,
		RelatedLinks: links,
		NextActions:  actions,
	}
}

func (s *Server) runRelatedLinks(ctx context.Context, projectID uuid.UUID, run db.GenerationRun) []runRelatedLinkDTO {
	links := []runRelatedLinkDTO{}
	input := runPayloadMap(run.Input)
	topicRaw, _ := input["topic"].(string)
	if topicRaw == "" {
		return links
	}
	topicID, err := uuid.Parse(topicRaw)
	if err != nil || s.Q == nil {
		return links
	}
	links = append(links, runRelatedLinkDTO{Label: "Open topic backlog", Href: activityProjectHref(projectID, "topics"), Kind: "topic"})
	if run.Agent != "writer" && run.Agent != "qa" {
		return links
	}
	articles, err := s.Q.ListArticlesByTopicForProject(ctx, db.ListArticlesByTopicForProjectParams{
		TopicID:   topicID,
		ProjectID: projectID,
	})
	if err != nil {
		return links
	}
	for _, article := range articles {
		if article.Status == "rejected" {
			continue
		}
		label := "Open draft"
		if article.Kind != "" {
			label += " (" + article.Kind + ")"
		}
		links = append(links, runRelatedLinkDTO{
			Label: label,
			Href:  activityProjectHref(projectID, "articles/"+article.ID.String()),
			Kind:  "article",
		})
		if len(links) >= 4 {
			break
		}
	}
	return links
}

func runNextActions(projectID uuid.UUID, run db.GenerationRun) []runRelatedLinkDTO {
	switch run.Agent {
	case "writer", "qa":
		return []runRelatedLinkDTO{
			{Label: "Open review", Href: activityProjectHref(projectID, "review"), Kind: "review"},
			{Label: "Open topics", Href: activityProjectHref(projectID, "topics"), Kind: "retry"},
		}
	case "insight":
		return []runRelatedLinkDTO{{Label: "Open Knowledge", Href: activityProjectHref(projectID, "knowledge"), Kind: "knowledge"}}
	case "strategist":
		return []runRelatedLinkDTO{{Label: "Open Topics", Href: activityProjectHref(projectID, "topics"), Kind: "topics"}}
	case "publisher", "notification":
		return []runRelatedLinkDTO{{Label: "Open Publishing", Href: activityProjectHref(projectID, "publishing"), Kind: "publishing"}}
	case "seo", "geo":
		return []runRelatedLinkDTO{{Label: "Open SEO", Href: activityProjectHref(projectID, "seo"), Kind: "seo"}}
	default:
		return nil
	}
}

func sanitizeRunPayload(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	var value any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return "[unreadable json]"
	}
	return redactRunValue(value)
}

func runPayloadMap(raw []byte) map[string]any {
	value := sanitizeRunPayload(raw)
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func redactRunValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, nested := range v {
			if isSensitiveRunKey(key) {
				out[key] = "[redacted]"
				continue
			}
			out[key] = redactRunValue(nested)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, nested := range v {
			out[i] = redactRunValue(nested)
		}
		return out
	case string:
		return sanitizeRunString(v)
	default:
		return value
	}
}

func isSensitiveRunKey(key string) bool {
	k := strings.ToLower(key)
	for _, marker := range []string{"token", "api_key", "apikey", "authorization", "secret", "password", "webhook", "deploy_hook", "credential"} {
		if strings.Contains(k, marker) {
			return true
		}
	}
	return false
}

func sanitizeRunStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	s := sanitizeRunString(*value)
	return &s
}

func sanitizeRunString(value string) string {
	lower := strings.ToLower(value)
	for _, marker := range []string{
		"hooks.slack.com/services/",
		"discord.com/api/webhooks/",
		"api.vercel.com/v1/integrations/deploy/",
		"ghp_",
		"github_pat_",
		"bearer ",
		"token=",
		"api_key=",
		"secret=",
		"password=",
	} {
		if strings.Contains(lower, marker) {
			return "[redacted]"
		}
	}
	return value
}
