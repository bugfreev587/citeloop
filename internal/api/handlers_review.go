package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/citeloop/citeloop/internal/agents"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// listReview returns pending_review articles grouped by topic so a canonical and
// its variants appear together (§5.5).
func (s *Server) listReview(w http.ResponseWriter, r *http.Request) {
	id, err := s.projectID(r)
	if err != nil {
		writeErr(w, 400, "bad project id")
		return
	}
	arts, err := s.Q.ListPendingReview(r.Context(), id)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// group by topic
	groups := map[uuid.UUID][]db.Article{}
	order := []uuid.UUID{}
	for _, a := range arts {
		if _, ok := groups[a.TopicID]; !ok {
			order = append(order, a.TopicID)
		}
		groups[a.TopicID] = append(groups[a.TopicID], a)
	}
	out := make([]map[string]any, 0, len(order))
	for _, tid := range order {
		out = append(out, map[string]any{"topic_id": tid, "articles": groups[tid]})
	}
	writeJSON(w, 200, out)
}

func (s *Server) articleID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "articleID"))
}

// editArticle updates content/seo. When the content changes it re-runs QA so
// qa_blocking is recomputed from the actual (edited) text — the ONLY way to
// clear the blocking gate. There is no flag-flip shortcut: a reviewer must edit
// the unsupported claim out (or map it to evidence) for QA to unblock it (§5.5).
func (s *Server) editArticle(w http.ResponseWriter, r *http.Request) {
	aid, err := s.articleID(r)
	if err != nil {
		writeErr(w, 400, "bad article id")
		return
	}
	var in struct {
		ContentMd string          `json:"content_md"`
		SeoMeta   json.RawMessage `json:"seo_meta"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	cur, err := s.Q.GetArticle(r.Context(), aid)
	if err != nil {
		writeErr(w, 404, "article not found")
		return
	}
	contentChanged := in.ContentMd != "" && in.ContentMd != cur.ContentMd
	if in.SeoMeta == nil {
		in.SeoMeta = cur.SeoMeta
	}
	if in.ContentMd == "" {
		in.ContentMd = cur.ContentMd
	}
	updated, err := s.Q.UpdateArticleContent(r.Context(), db.UpdateArticleContentParams{
		ID: aid, ContentMd: in.ContentMd, SeoMeta: in.SeoMeta,
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// Re-qualify on content change so the gate reflects the edited text. Without
	// a content change QA cannot have changed, so we skip the LLM call.
	if contentChanged {
		qaAgent := agents.NewQA(agents.Deps{Q: s.Q, LLM: s.LLM, Search: s.Search}, s.Log)
		requalified, qerr := qaAgent.Requalify(r.Context(), cur.ProjectID, aid)
		if qerr != nil {
			writeErr(w, 500, "re-qa failed: "+qerr.Error())
			return
		}
		updated = requalified
	}
	writeJSON(w, 200, updated)
}

// approveArticle is the human gate. canonical → approved + scheduled_at written
// (inherit topic.scheduled_at or now()+buffer_days). variant → approved (waits
// for canonical publish). Blocking articles cannot be approved (§5.5).
func (s *Server) approveArticle(w http.ResponseWriter, r *http.Request) {
	aid, err := s.articleID(r)
	if err != nil {
		writeErr(w, 400, "bad article id")
		return
	}
	var in struct {
		ReviewedBy string `json:"reviewed_by"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.ReviewedBy == "" {
		in.ReviewedBy = "reviewer"
	}
	a, err := s.Q.GetArticle(r.Context(), aid)
	if err != nil {
		writeErr(w, 404, "article not found")
		return
	}
	if a.QaBlocking {
		writeErr(w, 409, "article has unresolved qa_blocking issues; resolve before approving")
		return
	}

	var schedAt time.Time
	if a.Kind == "canonical" {
		// scheduled_at single source of truth (§3): inherit topic or now+buffer.
		topic, err := s.Q.GetTopic(r.Context(), a.TopicID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		cfg, _ := s.projectConfig(r, a.ProjectID)
		if topic.ScheduledAt.Valid {
			schedAt = topic.ScheduledAt.Time
		} else {
			schedAt = time.Now().Add(time.Duration(cfg.BufferDays) * 24 * time.Hour)
		}
	}

	updated, err := s.Q.ApproveArticle(r.Context(), db.ApproveArticleParams{
		ID:          aid,
		Status:      "approved",
		ScheduledAt: pgutil.TS(schedAt), // zero time for variants is harmless; unlock is gated on canonical publish
		ReviewedBy:  &in.ReviewedBy,
	})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, updated)
}

func (s *Server) rejectArticle(w http.ResponseWriter, r *http.Request) {
	aid, err := s.articleID(r)
	if err != nil {
		writeErr(w, 400, "bad article id")
		return
	}
	var in struct {
		ReviewedBy string `json:"reviewed_by"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.ReviewedBy == "" {
		in.ReviewedBy = "reviewer"
	}
	a, err := s.Q.RejectArticle(r.Context(), db.RejectArticleParams{ID: aid, ReviewedBy: &in.ReviewedBy})
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	// topic back to backlog so it can be re-picked (§5.5).
	_, _ = s.Q.UpdateTopicStatus(r.Context(), db.UpdateTopicStatusParams{ID: a.TopicID, Status: "backlog"})
	writeJSON(w, 200, a)
}

func (s *Server) markDistributed(w http.ResponseWriter, r *http.Request) {
	aid, err := s.articleID(r)
	if err != nil {
		writeErr(w, 400, "bad article id")
		return
	}
	a, err := s.Q.MarkDistributed(r.Context(), aid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, a)
}
