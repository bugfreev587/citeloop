// Package scheduler is the automatic operations core (PRD §5.4). A daily cron
// tick, per project: hold an advisory xact lock, enforce the monthly cost
// breaker, pick understocked topics with FOR UPDATE SKIP LOCKED, and generate
// into the review queue. A separate publish tick auto-publishes due canonicals
// and unlocks distributable variants (§5.6).
package scheduler

import (
	"context"
	"encoding/binary"
	"log/slog"
	"time"

	"github.com/citeloop/citeloop/internal/agents"
	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/publisher"
	"github.com/citeloop/citeloop/internal/search"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Scheduler struct {
	Pool    *pgxpool.Pool
	LLM     llm.Provider
	Search  search.Provider
	Blog    *publisher.BlogPublisher
	Log     *slog.Logger
	now     func() time.Time
	alert   func(projectID uuid.UUID, msg string)
}

func New(pool *pgxpool.Pool, llmP llm.Provider, searchP search.Provider, blog *publisher.BlogPublisher, log *slog.Logger) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	return &Scheduler{
		Pool: pool, LLM: llmP, Search: searchP, Blog: blog, Log: log,
		now:   time.Now,
		alert: func(p uuid.UUID, m string) { log.Warn("ALERT", "project", p, "msg", m) },
	}
}

// TickGenerate runs the daily generation pass across all projects (§5.4).
func (s *Scheduler) TickGenerate(ctx context.Context) {
	q := db.New(s.Pool)
	projects, err := q.ListProjects(ctx)
	if err != nil {
		s.Log.Error("list projects", "err", err)
		return
	}
	for _, p := range projects {
		if err := s.generateForProject(ctx, p); err != nil {
			s.Log.Error("generate tick failed", "project", p.ID, "err", err)
		}
	}
}

func (s *Scheduler) generateForProject(ctx context.Context, p db.Project) error {
	cfg, err := config.Parse(p.Config)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Per-project advisory lock prevents concurrent ticks double-generating.
	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock($1)", lockKey(p.ID)); err != nil {
		return err
	}
	q := db.New(tx)

	// Cost breaker (§5.4): stop before any LLM call if month's spend >= budget.
	spent, err := q.MonthlySpend(ctx, p.ID)
	if err != nil {
		return err
	}
	if pgutil.Float(spent) >= cfg.MonthlyBudgetUSD {
		s.alert(p.ID, "monthly budget reached; generation paused")
		return tx.Commit(ctx)
	}

	// Buffer-window stocking (§5.4): keep `buffer_days` worth of content in
	// flight. desired = cadence_per_week * buffer_days / 7 (rounded up). Generate
	// only the deficit vs. what's already stocked, so a stocked buffer does not
	// trigger more generation every tick.
	desired := ceilDiv(cfg.CadencePerWeek*cfg.BufferDays, 7)
	stocked, err := q.CountStockedCanonical(ctx, p.ID)
	if err != nil {
		return err
	}
	deficit := desired - int(stocked)
	if deficit <= 0 {
		s.Log.Info("buffer already stocked; skipping generation", "project", p.ID, "desired", desired, "stocked", stocked)
		return tx.Commit(ctx)
	}
	candidates, err := q.SelectGenerationCandidates(ctx, db.SelectGenerationCandidatesParams{
		ProjectID: p.ID,
		Limit:     int32(deficit),
	})
	if err != nil {
		return err
	}

	writer := agents.NewWriter(agents.Deps{Q: q, LLM: s.LLM, Search: s.Search}, s.Log)
	for _, t := range candidates {
		// Idempotency: skip if a non-rejected article already exists (§5.4).
		n, err := q.CountNonRejectedArticlesForTopic(ctx, t.ID)
		if err != nil || n > 0 {
			continue
		}
		if _, err := q.UpdateTopicStatus(ctx, db.UpdateTopicStatusParams{ID: t.ID, Status: "generating"}); err != nil {
			s.Log.Warn("mark generating failed", "topic", t.ID, "err", err)
			continue
		}
		if _, err := writer.Generate(ctx, p.ID, t); err != nil {
			s.alert(p.ID, "generation failed for topic "+t.Title+": "+err.Error())
			// leave topic in generating; next tick may retry. Do not block others.
			continue
		}
		s.Log.Info("generated into review queue", "project", p.ID, "topic", t.Title)
	}
	return tx.Commit(ctx)
}

// TickPublish auto-publishes due canonicals and unlocks distributable variants.
func (s *Scheduler) TickPublish(ctx context.Context) {
	q := db.New(s.Pool)
	projects, err := q.ListProjects(ctx)
	if err != nil {
		s.Log.Error("list projects", "err", err)
		return
	}
	for _, p := range projects {
		if err := s.publishForProject(ctx, p); err != nil {
			s.Log.Error("publish tick failed", "project", p.ID, "err", err)
		}
	}
	// variant unlock is project-independent in query; run once.
	s.unlockVariants(ctx)
}

func (s *Scheduler) publishForProject(ctx context.Context, p db.Project) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock($1)", lockKey(p.ID)); err != nil {
		return err
	}
	q := db.New(tx)

	due, err := q.SelectDueCanonical(ctx, p.ID)
	if err != nil {
		return err
	}
	for _, a := range due {
		res, err := s.Blog.Publish(ctx, &a)
		if err != nil {
			s.alert(p.ID, "publish failed for article "+a.ID.String()+": "+err.Error())
			continue // do not mark published on failure (§5.6)
		}
		pubResult := mustJSON(res)
		published, err := q.MarkPublished(ctx, db.MarkPublishedParams{
			ID:            a.ID,
			PublishResult: pubResult,
			CanonicalUrl:  &res.URL,
		})
		if err != nil {
			s.Log.Error("mark published failed", "article", a.ID, "err", err)
			continue
		}
		// Published canonical feeds back into inventory (§5.6).
		s.feedInventory(ctx, q, published, res.URL)
		s.Log.Info("auto-published canonical", "article", a.ID, "url", res.URL)
	}
	return tx.Commit(ctx)
}

func (s *Scheduler) unlockVariants(ctx context.Context) {
	q := db.New(s.Pool)
	variants, err := q.SelectUnlockableVariants(ctx)
	if err != nil {
		s.Log.Error("select unlockable variants", "err", err)
		return
	}
	for _, v := range variants {
		// The joined canonical_url is needed; re-read sibling canonical.
		sibs, err := q.ListArticlesByTopic(ctx, v.TopicID)
		if err != nil {
			continue
		}
		var realURL string
		for _, sib := range sibs {
			if sib.Kind == "canonical" && sib.CanonicalUrl != nil {
				realURL = *sib.CanonicalUrl
			}
		}
		if realURL == "" {
			continue // guard: never unlock before canonical URL exists (§5.6)
		}
		newContent := publisher.RewriteForDistribution(v.ContentMd, realURL)
		// Backfill the canonical placeholder in seo_meta too — canonical-capable
		// platforms (Dev.to/Hashnode) carry it in seo_meta.canonical_url (§5.6).
		newSEO := []byte(publisher.RewriteForDistribution(string(v.SeoMeta), realURL))
		if _, err := q.UnlockVariant(ctx, db.UnlockVariantParams{
			ID: v.ID, CanonicalUrl: &realURL, ContentMd: newContent, SeoMeta: newSEO,
		}); err != nil {
			s.Log.Error("unlock variant failed", "article", v.ID, "err", err)
			continue
		}
		s.Log.Info("variant ready to distribute", "article", v.ID, "platform", deref(v.Platform))
	}
}

func (s *Scheduler) feedInventory(ctx context.Context, q *db.Queries, a db.Article, url string) {
	seo := struct {
		Title         string `json:"title"`
		TargetKeyword string `json:"target_keyword"`
	}{}
	_ = jsonUnmarshal(a.SeoMeta, &seo)
	_, _ = q.UpsertInventory(ctx, db.UpsertInventoryParams{
		ProjectID:        a.ProjectID,
		Url:              url,
		Title:            ptr(seo.Title),
		TargetKeyword:    ptr(seo.TargetKeyword),
		Topics:           []byte("[]"),
		EvidenceSnippets: []byte("[]"),
		Source:           "generated",
	})
}

// lockKey derives a stable int64 advisory-lock key from a project UUID.
func lockKey(id uuid.UUID) int64 {
	b := id[:]
	return int64(binary.BigEndian.Uint64(b[:8]))
}

var _ = pgx.ErrNoRows // keep pgx import for callers that switch on it
